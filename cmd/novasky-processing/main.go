package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/piwi3910/NovaSky/internal/config"
	"github.com/piwi3910/NovaSky/internal/db"
	"github.com/piwi3910/NovaSky/internal/models"
	"github.com/piwi3910/NovaSky/internal/processing"
	novaskyRedis "github.com/piwi3910/NovaSky/internal/redis"
)

const (
	consumerGroup = "processing"
	consumerName  = "processing-1"
)

func main() {
	log.Println("[processing] Starting...")

	db.Init()
	novaskyRedis.Init()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	novaskyRedis.StartHealthReporter(ctx, "processing")

	// Load config for mask
	cfg := config.NewManager()
	cfg.Subscribe(ctx)

	// Create consumer group
	novaskyRedis.CreateConsumerGroup(ctx, novaskyRedis.StreamFramesProcessing, consumerGroup)

	// Stacking buffer
	type pendingFrame struct {
		frameID  string
		filePath string
		stretch  string
	}
	var stackBuffer []pendingFrame

	log.Println("[processing] Worker started, waiting for frames...")

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Read from stream
		streams, err := novaskyRedis.ReadFromGroup(ctx, novaskyRedis.StreamFramesProcessing, consumerGroup, consumerName, 1)
		if err != nil {
			log.Printf("[processing] Read error: %v", err)
			continue
		}

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				frameID := msg.Values["frameId"].(string)
				filePath := msg.Values["filePath"].(string)
				stretch := "none"
				if s, ok := msg.Values["stretch"].(string); ok {
					stretch = s
				}

				// Read stacking config
				var stackCfg struct {
					Enabled bool `json:"enabled"`
					Count   int  `json:"count"`
				}
				cfg.Get("imaging.stacking", &stackCfg)
				if stackCfg.Count <= 0 {
					stackCfg.Count = 5
				}

				// Get processing configs
				var maskCfg processing.MaskConfig
				cfg.Get("imaging.mask", &maskCfg)
				var mask *processing.MaskConfig
				if maskCfg.Enabled {
					mask = &maskCfg
				}
				var skyglow bool
				cfg.Get("imaging.skyglow", &skyglow)
				var cal struct {
					NorthAngle float64 `json:"northAngle"`
					Solved     bool    `json:"solved"`
				}
				cfg.Get("camera.calibration", &cal)
				rotation := 0.0
				if cal.Solved {
					rotation = cal.NorthAngle
				}

				// Acknowledge immediately so queue doesn't back up
				novaskyRedis.AckMessage(ctx, novaskyRedis.StreamFramesProcessing, consumerGroup, msg.ID)

				if stackCfg.Enabled && stackCfg.Count > 1 {
					// Stacking mode: buffer frames, process when full
					stackBuffer = append(stackBuffer, pendingFrame{frameID, filePath, stretch})
					log.Printf("[processing] Stacking: buffered frame %d/%d", len(stackBuffer), stackCfg.Count)

					if len(stackBuffer) < stackCfg.Count {
						continue // wait for more frames
					}

					// Buffer full — stack and process
					startTime := time.Now()
					var paths []string
					for _, f := range stackBuffer {
						paths = append(paths, f.filePath)
					}
					lastFrame := stackBuffer[len(stackBuffer)-1]
					log.Printf("[processing] Stacking %d frames...", len(paths))

					result, err := processing.ProcessStackedFrames(paths, stretch, mask, skyglow, rotation)
					stackBuffer = nil // clear buffer
					if err != nil {
						log.Printf("[processing] Stacking failed: %v", err)
						continue
					}

					// Update last frame in DB with the stacked JPEG
					db.GetDB().Model(&models.Frame{}).Where("id = ?", lastFrame.frameID).Update("jpeg_path", result.JpegPath)

					streamData := map[string]interface{}{
						"frameId": lastFrame.frameID, "filePath": lastFrame.filePath, "jpegPath": result.JpegPath,
					}
					novaskyRedis.PublishToStream(ctx, novaskyRedis.StreamFramesDetection, streamData)
					novaskyRedis.PublishToStream(ctx, novaskyRedis.StreamFramesOverlay, streamData)
					novaskyRedis.PublishToStream(ctx, novaskyRedis.StreamFramesExport, streamData)
					novaskyRedis.PublishToStream(ctx, novaskyRedis.StreamFramesTimelapse, streamData)

					event, _ := json.Marshal(map[string]interface{}{"id": lastFrame.frameID, "jpegPath": result.JpegPath})
					novaskyRedis.Publish(ctx, novaskyRedis.ChannelFrameProcessed, string(event))

					elapsed := time.Since(startTime)
					novaskyRedis.Client.Set(ctx, "novasky:latency:processing", fmt.Sprintf("%.3f", elapsed.Seconds()), 0)
					log.Printf("[processing] Stacked %d frames in %.1fs → %s", len(paths), elapsed.Seconds(), result.JpegPath)
				} else {
					// Single frame mode
					startTime := time.Now()
					result, err := processing.ProcessFrame(filePath, stretch, mask, skyglow, rotation)
					if err != nil {
						log.Printf("[processing] Failed to process %s: %v", frameID, err)
						continue
					}

					db.GetDB().Model(&models.Frame{}).Where("id = ?", frameID).Update("jpeg_path", result.JpegPath)

					streamData := map[string]interface{}{
						"frameId": frameID, "filePath": filePath, "jpegPath": result.JpegPath,
					}
					novaskyRedis.PublishToStream(ctx, novaskyRedis.StreamFramesDetection, streamData)
					novaskyRedis.PublishToStream(ctx, novaskyRedis.StreamFramesOverlay, streamData)
					novaskyRedis.PublishToStream(ctx, novaskyRedis.StreamFramesExport, streamData)
					novaskyRedis.PublishToStream(ctx, novaskyRedis.StreamFramesTimelapse, streamData)

					event, _ := json.Marshal(map[string]interface{}{"id": frameID, "jpegPath": result.JpegPath})
					novaskyRedis.Publish(ctx, novaskyRedis.ChannelFrameProcessed, string(event))

					elapsed := time.Since(startTime)
					novaskyRedis.Client.Set(ctx, "novasky:latency:processing", fmt.Sprintf("%.3f", elapsed.Seconds()), 0)
					log.Printf("[processing] Frame %s processed in %.1fs → %s", frameID, elapsed.Seconds(), result.JpegPath)
				}

				novaskyRedis.ReportHealth(ctx, "processing")
			}
		}
	}
}
