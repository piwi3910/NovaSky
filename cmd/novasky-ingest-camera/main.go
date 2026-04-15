package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/piwi3910/NovaSky/internal/autoexposure"
	"github.com/piwi3910/NovaSky/internal/config"
	"github.com/piwi3910/NovaSky/internal/db"
	"github.com/piwi3910/NovaSky/internal/diskmanager"
	"github.com/piwi3910/NovaSky/internal/fits"
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

	novaskyRedis.StartHealthReporter(ctx, "ingest-camera")

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
		twilightCfg.TransitionSpeed = 25
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
			if tw.TransitionSpeed == 0 {
				tw.TransitionSpeed = 25
			}
			var loc config.LocationConfig
			cfg.Get("location", &loc)
			var bp float64
			cfg.Get("autoexposure.buffer", &bp)
			ae.UpdateConfig(dp, np, tw.SunAltitude, tw.TransitionSpeed, loc.Latitude, loc.Longitude, bp)
		}
	})

	// Focus mode subscription
	focusMode := false
	go func() {
		sub := novaskyRedis.Client.Subscribe(ctx, "novasky:focus-mode")
		ch := sub.Channel()
		for msg := range ch {
			focusMode = msg.Payload == "start"
			if focusMode {
				log.Println("[ingest-camera] Focus mode STARTED — rapid capture, short exposure")
			} else {
				log.Println("[ingest-camera] Focus mode STOPPED — resuming auto-exposure")
			}
		}
	}()

	log.Printf("[ingest-camera] Starting capture loop: device=%s spool=%s", deviceName, spoolDir)

	frameCount := 0
	consecutiveFailures := 0

	// reconnectINDI restarts the INDI server and client when the connection dies
	reconnectINDI := func() {
		log.Println("[ingest-camera] Reconnecting INDI...")
		client.Close()
		server.Stop()
		time.Sleep(2 * time.Second)
		if err := server.Start(); err != nil {
			log.Printf("[ingest-camera] Failed to restart INDI server: %v", err)
			return
		}
		time.Sleep(3 * time.Second)
		if err := client.Connect(ctx, "localhost", indiPort); err != nil {
			log.Printf("[ingest-camera] Failed to reconnect INDI client: %v", err)
			return
		}
		devices := client.GetDevices()
		if len(devices) > 0 {
			deviceName = devices[0]
			client.ConnectDevice(deviceName)
		}
		consecutiveFailures = 0
		log.Println("[ingest-camera] INDI reconnected successfully")
	}

	// Main capture loop
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		interval := defaultInterval

		// Focus mode override — rapid capture with fixed short exposure
		if focusMode {
			client.SetGain(deviceName, 200)
			time.Sleep(50 * time.Millisecond)
			fitsData, err := client.Capture(deviceName, 0.5, 10*time.Second) // 500ms, fixed
			if err != nil {
				log.Printf("[ingest-camera] Focus capture failed: %v", err)
				consecutiveFailures++
				if consecutiveFailures >= 3 {
					reconnectINDI()
				}
				time.Sleep(500 * time.Millisecond)
				continue
			}
			consecutiveFailures = 0
			medianADU := fits.MedianADU(fitsData)
			timestamp := time.Now()
			filename := fmt.Sprintf("focus_%d.fits", timestamp.UnixMilli())
			filePath := filepath.Join(spoolDir, filename)
			os.WriteFile(filePath, fitsData, 0644)

			// Publish focus frame directly to processing
			frame := models.Frame{FilePath: filePath, CapturedAt: timestamp, ExposureMs: 500, Gain: 200, MedianADU: &medianADU}
			db.GetDB().Create(&frame)
			novaskyRedis.PublishToStream(ctx, novaskyRedis.StreamFramesProcessing, map[string]interface{}{
				"frameId": frame.ID, "filePath": filePath, "stretch": "auto",
			})

			log.Printf("[ingest-camera] Focus frame: %s adu=%.0f", frame.ID, medianADU)
			time.Sleep(500 * time.Millisecond)
			continue
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
			consecutiveFailures++
			if consecutiveFailures >= 3 {
				reconnectINDI()
			}
			time.Sleep(5 * time.Second)
			continue
		}
		consecutiveFailures = 0

		// Save FITS to disk
		timestamp := time.Now()
		filename := fmt.Sprintf("frame_%d.fits", timestamp.UnixMilli())
		filePath := filepath.Join(spoolDir, filename)
		if err := os.WriteFile(filePath, fitsData, 0644); err != nil {
			log.Printf("[ingest-camera] Failed to save FITS: %v", err)
			continue
		}

		// Compute median ADU from raw FITS data
		medianADU := fits.MedianADU(fitsData)

		// Adjust auto-exposure
		ae.Adjust(medianADU)

		// Report health
		novaskyRedis.ReportHealth(ctx, "ingest-camera")

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

			log.Printf("[ingest-camera] Frame captured: %s exp=%.3fms gain=%d adu=%.0f",
				frame.ID, exposureMs, gain, medianADU)
		} else {
			// Convergence frame — just for ADU measurement, delete the FITS
			os.Remove(filePath)
			log.Printf("[ingest-camera] Convergence frame: exp=%.3fms gain=%d adu=%.0f (not sent to pipeline)",
				exposureMs, gain, medianADU)
		}

		// Periodic disk check (every 10 frames)
		if frameCount%10 == 0 {
			diskmanager.CheckAndClean(spoolDir, 5.0) // keep 5GB free minimum

			// Retention policy cleanup
			var retentionDays int
			cfg.Get("storage.retention.days", &retentionDays)
			if retentionDays == 0 {
				retentionDays = 30
			}
			var retentionMaxGB float64
			cfg.Get("storage.retention.maxGB", &retentionMaxGB)
			if retentionMaxGB == 0 {
				retentionMaxGB = 50
			}
			diskmanager.CleanByRetention(spoolDir, retentionDays)
			diskmanager.CleanBySize(spoolDir, retentionMaxGB)
		}
		frameCount++

		// Rapid capture when ADU is way off — skip the normal interval
		if ae.NeedsRapidCapture() {
			time.Sleep(500 * time.Millisecond) // Minimal delay for camera readout
		} else {
			time.Sleep(interval)
		}
	}
}

// computeMedianADU is now in internal/fits package: fits.MedianADU()
