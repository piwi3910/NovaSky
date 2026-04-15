package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"syscall"
	"time"

	"github.com/piwi3910/NovaSky/internal/autoexposure"
	"github.com/piwi3910/NovaSky/internal/config"
	"github.com/piwi3910/NovaSky/internal/db"
	"github.com/piwi3910/NovaSky/internal/indi"
	"github.com/piwi3910/NovaSky/internal/models"
	novaskyRedis "github.com/piwi3910/NovaSky/internal/redis"
)

const (
	defaultInterval = 10 * time.Second
	consumerGroup   = "ingest-camera"
)

func main() {
	log.Println("[ingest-camera] Starting...")

	// Init shared services
	db.Init()
	novaskyRedis.Init()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("[ingest-camera] Shutting down...")
		cancel()
	}()

	// Load config
	cfg := config.NewManager()
	cfg.Subscribe(ctx)

	// Get camera config
	var driver string
	cfg.Get("camera.driver", &driver)
	if driver == "" {
		driver = "indi_asi_ccd"
	}

	// Get imaging profiles
	var dayProfile, nightProfile autoexposure.Profile
	cfg.Get("imaging.day", &dayProfile)
	cfg.Get("imaging.night", &nightProfile)

	var twilightCfg struct {
		SunAltitude     float64 `json:"sunAltitude"`
		TransitionSpeed int     `json:"transitionSpeed"`
	}
	cfg.Get("imaging.twilight", &twilightCfg)
	if twilightCfg.SunAltitude == 0 {
		twilightCfg.SunAltitude = -6
	}
	if twilightCfg.TransitionSpeed == 0 {
		twilightCfg.TransitionSpeed = 1
	}

	var location config.LocationConfig
	cfg.Get("location", &location)

	var bufferPct float64
	cfg.Get("autoexposure.buffer", &bufferPct)
	var historySize int
	cfg.Get("autoexposure.history", &historySize)

	// Frame spool directory
	spoolDir := os.Getenv("FRAME_SPOOL_DIR")
	if spoolDir == "" {
		spoolDir = "/home/piwi/novasky-data/frames"
	}
	os.MkdirAll(spoolDir, 0755)

	// Start INDI server
	indiPort := 7624
	server := indi.NewServer(driver, indiPort)
	if err := server.Start(); err != nil {
		log.Fatalf("[ingest-camera] Failed to start INDI server: %v", err)
	}
	defer server.Stop()

	// Connect INDI client
	client := indi.NewClient()
	if err := client.Connect(ctx, "localhost", indiPort); err != nil {
		log.Fatalf("[ingest-camera] Failed to connect INDI client: %v", err)
	}
	defer client.Close()

	// Connect to first device
	devices := client.GetDevices()
	if len(devices) == 0 {
		log.Fatal("[ingest-camera] No INDI devices found")
	}
	deviceName := devices[0]
	if err := client.ConnectDevice(deviceName); err != nil {
		log.Fatalf("[ingest-camera] Failed to connect device: %v", err)
	}

	// Init auto-exposure
	ae := autoexposure.New(
		dayProfile, nightProfile,
		twilightCfg.SunAltitude, twilightCfg.TransitionSpeed,
		location.Latitude, location.Longitude,
		bufferPct, historySize,
	)

	// Restore last exposure/gain from DB
	var lastDrift models.Config
	if err := db.GetDB().First(&lastDrift, "key = ?", "autoexposure.drift").Error; err == nil {
		var drift struct {
			Exposure float64 `json:"exposure"`
			Gain     int     `json:"gain"`
		}
		json.Unmarshal([]byte(lastDrift.Value), &drift)
		ae.Resume(drift.Exposure, drift.Gain)
	}

	// Config change handler
	cfg.OnChange(func(key string) {
		switch {
		case key == "imaging.day" || key == "imaging.night" || key == "imaging.twilight" || key == "location":
			var dp, np autoexposure.Profile
			cfg.Get("imaging.day", &dp)
			cfg.Get("imaging.night", &np)
			var tw struct {
				SunAltitude     float64 `json:"sunAltitude"`
				TransitionSpeed int     `json:"transitionSpeed"`
			}
			cfg.Get("imaging.twilight", &tw)
			var loc config.LocationConfig
			cfg.Get("location", &loc)
			var bp float64
			cfg.Get("autoexposure.buffer", &bp)
			ae.UpdateConfig(dp, np, tw.SunAltitude, tw.TransitionSpeed, loc.Latitude, loc.Longitude, bp)
		}
	})

	// Backpressure monitoring
	backpressure := "normal"
	go func() {
		sub := novaskyRedis.Client.Subscribe(ctx, novaskyRedis.ChannelBackpressure)
		ch := sub.Channel()
		for msg := range ch {
			backpressure = msg.Payload
		}
	}()

	log.Printf("[ingest-camera] Starting capture loop: device=%s spool=%s", deviceName, spoolDir)

	// Main capture loop
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Check backpressure
		if backpressure == "paused" {
			log.Println("[ingest-camera] Paused (backpressure)")
			time.Sleep(2 * time.Second)
			continue
		}

		interval := defaultInterval
		if backpressure == "throttled" {
			interval *= 2
		}

		// Get exposure settings
		exposureMs := ae.ExposureMs()
		gain := ae.Gain()

		// Set gain
		client.SetGain(deviceName, gain)
		time.Sleep(100 * time.Millisecond)

		// Capture frame
		exposureSec := exposureMs / 1000.0
		timeout := time.Duration(exposureMs+30000) * time.Millisecond
		fitsData, err := client.Capture(deviceName, exposureSec, timeout)
		if err != nil {
			log.Printf("[ingest-camera] Capture failed: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		// Save FITS to disk
		timestamp := time.Now()
		filename := fmt.Sprintf("frame_%d.fits", timestamp.UnixMilli())
		filePath := filepath.Join(spoolDir, filename)
		if err := os.WriteFile(filePath, fitsData, 0644); err != nil {
			log.Printf("[ingest-camera] Failed to save FITS: %v", err)
			continue
		}

		// Compute median ADU from raw FITS data
		medianADU := computeMedianADU(fitsData)

		// Adjust auto-exposure
		ae.Adjust(medianADU)

		// Publish auto-exposure state (always, even during convergence)
		aeState, _ := json.Marshal(ae.GetState())
		novaskyRedis.Publish(ctx, novaskyRedis.ChannelAutoExposure, string(aeState))

		// Persist drift for restart recovery
		drift, _ := json.Marshal(map[string]interface{}{
			"exposure": ae.ExposureMs(), "gain": ae.Gain(),
		})
		db.GetDB().Save(&models.Config{Key: "autoexposure.drift", Value: drift})

		// Only send to pipeline when exposure has converged
		// During rapid convergence, frames are just for ADU measurement — don't process them
		if ae.IsConverged() {
			// Save frame to DB
			frame := models.Frame{
				FilePath:   filePath,
				CapturedAt: timestamp,
				ExposureMs: exposureMs,
				Gain:       gain,
				MedianADU:  &medianADU,
			}
			db.GetDB().Create(&frame)

			// Publish to processing stream
			novaskyRedis.PublishToStream(ctx, novaskyRedis.StreamFramesProcessing, map[string]interface{}{
				"frameId":  frame.ID,
				"filePath": filePath,
				"stretch":  ae.ActiveProfile().Stretch,
			})

			// Publish frame-new event
			frameEvent, _ := json.Marshal(map[string]interface{}{
				"id": frame.ID, "filePath": filePath,
				"capturedAt": timestamp, "exposureMs": exposureMs,
				"gain": gain, "medianAdu": medianADU,
			})
			novaskyRedis.Publish(ctx, novaskyRedis.ChannelFrameNew, string(frameEvent))

			// Check backpressure (queue depth)
			queueLen, _ := novaskyRedis.GetStreamLength(ctx, novaskyRedis.StreamFramesProcessing)
			if queueLen > 3 {
				novaskyRedis.Publish(ctx, novaskyRedis.ChannelBackpressure, "paused")
			} else if queueLen > 1 {
				novaskyRedis.Publish(ctx, novaskyRedis.ChannelBackpressure, "throttled")
			} else {
				novaskyRedis.Publish(ctx, novaskyRedis.ChannelBackpressure, "normal")
			}

			log.Printf("[ingest-camera] Frame captured: %s exp=%.3fms gain=%d adu=%.0f",
				frame.ID, exposureMs, gain, medianADU)
		} else {
			// Convergence frame — just for ADU measurement, delete the FITS
			os.Remove(filePath)
			log.Printf("[ingest-camera] Convergence frame: exp=%.3fms gain=%d adu=%.0f (not sent to pipeline)",
				exposureMs, gain, medianADU)
		}

		// Rapid capture when ADU is way off — skip the normal interval
		if ae.NeedsRapidCapture() {
			time.Sleep(500 * time.Millisecond) // Minimal delay for camera readout
		} else {
			time.Sleep(interval)
		}
	}
}

// computeMedianADU reads raw 16-bit pixel data from FITS and computes the median.
// Handles BZERO/BSCALE for proper unsigned 16-bit interpretation.
func computeMedianADU(fitsData []byte) float64 {
	// Parse BZERO from header (FITS with BITPIX=16 + BZERO=32768 = unsigned 16-bit)
	var bzero float64
	headerEnd := 0
	for i := 0; i < len(fitsData)-80; i += 80 {
		line := string(fitsData[i : i+80])
		if len(line) >= 3 && line[:3] == "END" {
			headerEnd = ((i/80 + 1) * 80)
			headerEnd = ((headerEnd + 2879) / 2880) * 2880
			break
		}
		key := ""
		if len(line) >= 8 {
			key = line[:8]
		}
		if key == "BZERO   " && len(line) > 10 && line[8] == '=' {
			fmt.Sscanf(line[10:], "%f", &bzero)
		}
	}

	if headerEnd == 0 || headerEnd >= len(fitsData) {
		return 0
	}

	pixelData := fitsData[headerEnd:]

	// Read as 16-bit big-endian, apply BZERO for true unsigned value
	nPixels := len(pixelData) / 2
	if nPixels == 0 {
		return 0
	}

	// Sample for performance (every Nth pixel for large images)
	step := 1
	if nPixels > 100000 {
		step = nPixels / 100000
	}

	values := make([]float64, 0, nPixels/step)
	for i := 0; i < len(pixelData)-1; i += 2 * step {
		raw := int16(binary.BigEndian.Uint16(pixelData[i : i+2]))
		// Apply BZERO: physical_value = raw + BZERO
		val := float64(raw) + bzero
		if val < 0 {
			val = 0
		}
		values = append(values, val)
	}

	if len(values) == 0 {
		return 0
	}

	// Sort and take median
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	return values[len(values)/2]
}
