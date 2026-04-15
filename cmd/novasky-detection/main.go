package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"gocv.io/x/gocv"

	"github.com/piwi3910/NovaSky/internal/db"
	"github.com/piwi3910/NovaSky/internal/detection"
	"github.com/piwi3910/NovaSky/internal/fits"
	"github.com/piwi3910/NovaSky/internal/models"
	"github.com/piwi3910/NovaSky/internal/platesolve"
	novaskyRedis "github.com/piwi3910/NovaSky/internal/redis"
)

const consumerGroup = "detection"

func main() {
	log.Println("[detection] Starting...")
	db.Init()
	novaskyRedis.Init()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { c := make(chan os.Signal, 1); signal.Notify(c, syscall.SIGINT, syscall.SIGTERM); <-c; cancel() }()

	novaskyRedis.StartHealthReporter(ctx, "detection")

	novaskyRedis.CreateConsumerGroup(ctx, novaskyRedis.StreamFramesDetection, consumerGroup)
	log.Println("[detection] Worker started")

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		streams, err := novaskyRedis.ReadFromGroup(ctx, novaskyRedis.StreamFramesDetection, consumerGroup, "detection-1", 1)
		if err != nil {
			continue
		}

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				frameID := msg.Values["frameId"].(string)
				filePath := msg.Values["filePath"].(string)

				// Read FITS and analyze
				data, err := os.ReadFile(filePath)
				if err != nil {
					log.Printf("[detection] Read error: %v", err)
					novaskyRedis.AckMessage(ctx, novaskyRedis.StreamFramesDetection, consumerGroup, msg.ID)
					continue
				}

				startTime := time.Now()
				brightness, cloudCover := analyzeFrame(data)
				skyQuality := classifyQuality(cloudCover)

				// Compute SQM from background brightness (simplified)
				// SQM ≈ -2.5 * log10(brightness * scale_factor) + zero_point
				// For a rough estimate: darker = higher SQM (better sky)
				var sqm *float64
				if brightness > 0 && brightness < 0.3 { // Only meaningful for dark-ish frames
					sqmVal := -2.5*math.Log10(brightness) + 20.0 // rough mag/arcsec²
					sqm = &sqmVal
				}

				result := models.AnalysisResult{
					FrameID:    frameID,
					CloudCover: cloudCover,
					Brightness: brightness,
					SkyQuality: skyQuality,
					SQM:        sqm,
				}
				db.GetDB().Create(&result)

				// Star detection (night frames only)
				if brightness < 0.2 {
					header := fits.ParseHeader(data)
					pixels := fits.ReadPixels16(data, header)
					if len(pixels) > 0 && header.NAXIS1 > 0 && header.NAXIS2 > 0 {
						mat, err := gocv.NewMatFromBytes(header.NAXIS2, header.NAXIS1, gocv.MatTypeCV16UC1, uint16ToBytes(pixels))
						if err == nil {
							stars := detection.DetectStars(mat, 200)
							mat.Close()
							if len(stars) > 0 {
								starsJSON, _ := json.Marshal(stars)
								db.GetDB().Create(&models.Detection{
									FrameID: frameID,
									Type:    "stars",
									Data:    starsJSON,
								})
								log.Printf("[detection] Found %d stars", len(stars))
							}
						}
					}
				}

				// Fetch TLE data periodically (non-blocking)
				go detection.FetchTLEs() //nolint:errcheck

				// Plate solve (once, or periodically)
				if platesolve.GetCachedWCS() == nil {
					go func(fp string) {
						log.Printf("[detection] Attempting plate solve on %s", fp)
						wcs, err := platesolve.Solve(fp, 180)
						if err != nil {
							log.Printf("[detection] Plate solve failed: %v", err)
							return
						}
						if wcs.Solved {
							platesolve.CacheWCS(wcs)
							log.Printf("[detection] Plate solved! RA=%.4f Dec=%.4f", wcs.CRVAL1, wcs.CRVAL2)
						}
					}(filePath)
				}

				// Trigger policy evaluation
				novaskyRedis.PublishToStream(ctx, novaskyRedis.StreamPolicyEvaluate, map[string]interface{}{
					"trigger": "detection", "sourceId": result.ID,
				})

				novaskyRedis.AckMessage(ctx, novaskyRedis.StreamFramesDetection, consumerGroup, msg.ID)
				novaskyRedis.ReportHealth(ctx, "detection")

				elapsed := time.Since(startTime)
				novaskyRedis.Client.Set(ctx, "novasky:latency:detection", fmt.Sprintf("%.3f", elapsed.Seconds()), 0)
				log.Printf("[detection] Frame %s: cloud=%.0f%% sky=%s (%.1fs)", frameID, cloudCover*100, skyQuality, elapsed.Seconds())
			}
		}
	}
}

func uint16ToBytes(data []uint16) []byte {
	buf := make([]byte, len(data)*2)
	for i, v := range data {
		buf[i*2] = byte(v)
		buf[i*2+1] = byte(v >> 8)
	}
	return buf
}

func analyzeFrame(fitsData []byte) (brightness, cloudCover float64) {
	// Find data section
	headerEnd := 0
	for i := 0; i < len(fitsData)-80; i += 80 {
		if string(fitsData[i:i+3]) == "END" {
			headerEnd = ((i/80 + 1) * 80)
			headerEnd = ((headerEnd + 2879) / 2880) * 2880
			break
		}
	}
	if headerEnd == 0 || headerEnd >= len(fitsData) {
		return 0, 0
	}

	pixelData := fitsData[headerEnd:]
	step := 1
	nPixels := len(pixelData) / 2
	if nPixels > 50000 {
		step = nPixels / 50000
	}

	var values []uint16
	for i := 0; i < len(pixelData)-1; i += 2 * step {
		values = append(values, binary.BigEndian.Uint16(pixelData[i:i+2]))
	}
	if len(values) == 0 {
		return 0, 0
	}

	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	median := float64(values[len(values)/2])
	brightness = median / 65535.0
	cloudCover = math.Min(1.0, brightness*2)
	return
}

func classifyQuality(cloudCover float64) string {
	if cloudCover > 0.8 {
		return "UNUSABLE"
	}
	if cloudCover > 0.5 {
		return "POOR"
	}
	if cloudCover > 0.2 {
		return "GOOD"
	}
	return "EXCELLENT"
}
