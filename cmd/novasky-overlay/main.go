package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/piwi3910/NovaSky/internal/astronomy"
	"github.com/piwi3910/NovaSky/internal/config"
	"github.com/piwi3910/NovaSky/internal/db"
	"github.com/piwi3910/NovaSky/internal/models"
	novaskyRedis "github.com/piwi3910/NovaSky/internal/redis"
)

const consumerGroup = "overlay"

func main() {
	log.Println("[overlay] Starting...")
	db.Init()
	novaskyRedis.Init()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { c := make(chan os.Signal, 1); signal.Notify(c, syscall.SIGINT, syscall.SIGTERM); <-c; cancel() }()
	novaskyRedis.StartHealthReporter(ctx, "overlay")

	cfg := config.NewManager()
	cfg.Subscribe(ctx)

	novaskyRedis.CreateConsumerGroup(ctx, novaskyRedis.StreamFramesOverlay, consumerGroup)
	log.Println("[overlay] Worker started — computing overlay metadata")

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		streams, err := novaskyRedis.ReadFromGroup(ctx, novaskyRedis.StreamFramesOverlay, consumerGroup, "overlay-1", 1)
		if err != nil {
			continue
		}

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				frameID := msg.Values["frameId"].(string)

				// Compute overlay metadata
				now := time.Now()

				// Location from config
				var loc struct {
					Latitude  float64 `json:"latitude"`
					Longitude float64 `json:"longitude"`
				}
				cfg.Get("location", &loc)

				// Moon data
				moonIllum, moonPhase := astronomy.MoonPhase(now)

				// Latest frame info
				var frame models.Frame
				db.GetDB().First(&frame, "id = ?", frameID)

				// Latest safety
				var safety models.SafetyState
				db.GetDB().Order("evaluated_at DESC").First(&safety)

				// Latest sensor readings
				var tempReading, humidityReading models.SensorReading
				db.GetDB().Where("sensor_type = ?", "temperature").Order("recorded_at DESC").First(&tempReading)
				db.GetDB().Where("sensor_type = ?", "humidity").Order("recorded_at DESC").First(&humidityReading)

				// Latest analysis for SQM and cloud cover
				var analysis models.AnalysisResult
				db.GetDB().Where("frame_id = ?", frameID).First(&analysis)
				if analysis.ID == "" {
					db.GetDB().Order("analyzed_at DESC").First(&analysis)
				}

				// Compute Bortle from SQM
				var sqmVal float64
				var bortleClass int
				if analysis.SQM != nil {
					sqmVal = *analysis.SQM
					bortleClass = astronomy.SQMToBortle(sqmVal)
				}

				// Median ADU
				var aduVal float64
				if frame.MedianADU != nil {
					aduVal = *frame.MedianADU
				}

				// Build overlay metadata with text variables
				overlayData := map[string]any{
					"timestamp": now.Format("02/01/2006 15:04:05"),
					"moon": map[string]any{
						"phase":        moonPhase,
						"illumination": int(moonIllum * 100),
					},
					"camera": map[string]any{
						"exposure": frame.ExposureMs,
						"gain":     frame.Gain,
						"adu":      frame.MedianADU,
					},
					"safety": map[string]any{
						"state":   safety.State,
						"quality": safety.ImagingQuality,
					},
					"location": map[string]any{
						"lat": loc.Latitude,
						"lon": loc.Longitude,
					},
					// Text variables for frontend rendering
					"variables": map[string]any{
						"date":       now.Format("2006-01-02"),
						"time":       now.Format("15:04:05"),
						"exposure":   frame.ExposureMs,
						"gain":       frame.Gain,
						"adu":        aduVal,
						"temp":       tempReading.Value,
						"humidity":   humidityReading.Value,
						"sqm":        sqmVal,
						"bortle":     bortleClass,
						"moon":       moonPhase,
						"cloudcover": analysis.CloudCover * 100,
					},
				}

				data, _ := json.Marshal(overlayData)
				detection := models.Detection{
					FrameID: frameID,
					Type:    "overlay",
					Data:    data,
				}
				db.GetDB().Create(&detection)

				novaskyRedis.AckMessage(ctx, novaskyRedis.StreamFramesOverlay, consumerGroup, msg.ID)
				log.Printf("[overlay] Frame %s: overlay metadata stored", frameID)
			}
		}
	}
}
