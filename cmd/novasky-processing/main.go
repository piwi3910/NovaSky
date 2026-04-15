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

				startTime := time.Now()
				log.Printf("[processing] Processing frame %s: stretch=%s", frameID, stretch)

				// Get mask config
				var maskCfg processing.MaskConfig
				cfg.Get("imaging.mask", &maskCfg)

				var mask *processing.MaskConfig
				if maskCfg.Enabled {
					mask = &maskCfg
				}

				// Get skyglow config
				var skyglow bool
				cfg.Get("imaging.skyglow", &skyglow)

				// Process frame using GoCV/OpenCV debayer
				result, err := processing.ProcessFrame(filePath, stretch, mask, skyglow)
				if err != nil {
					log.Printf("[processing] Failed to process %s: %v", frameID, err)
					novaskyRedis.AckMessage(ctx, novaskyRedis.StreamFramesProcessing, consumerGroup, msg.ID)
					continue
				}

				// Update frame in DB
				db.GetDB().Model(&models.Frame{}).Where("id = ?", frameID).Update("jpeg_path", result.JpegPath)

				// Publish to downstream streams
				streamData := map[string]interface{}{
					"frameId":  frameID,
					"filePath": filePath,
					"jpegPath": result.JpegPath,
				}
				novaskyRedis.PublishToStream(ctx, novaskyRedis.StreamFramesDetection, streamData)
				novaskyRedis.PublishToStream(ctx, novaskyRedis.StreamFramesOverlay, streamData)
				novaskyRedis.PublishToStream(ctx, novaskyRedis.StreamFramesExport, streamData)
				novaskyRedis.PublishToStream(ctx, novaskyRedis.StreamFramesTimelapse, streamData)

				// Publish processed event
				event, _ := json.Marshal(map[string]interface{}{
					"id": frameID, "jpegPath": result.JpegPath,
				})
				novaskyRedis.Publish(ctx, novaskyRedis.ChannelFrameProcessed, string(event))

				// Publish backpressure
				queueLen, _ := novaskyRedis.GetStreamLength(ctx, novaskyRedis.StreamFramesProcessing)
				bp := "normal"
				if queueLen > 3 {
					bp = "paused"
				} else if queueLen > 1 {
					bp = "throttled"
				}
				novaskyRedis.Publish(ctx, novaskyRedis.ChannelBackpressure, bp)

				// Acknowledge message
				novaskyRedis.AckMessage(ctx, novaskyRedis.StreamFramesProcessing, consumerGroup, msg.ID)

				novaskyRedis.ReportHealth(ctx, "processing")

				// Store processing latency in Redis for pipeline visualization
				elapsed := time.Since(startTime)
				novaskyRedis.Client.Set(ctx, "novasky:latency:processing", fmt.Sprintf("%.3f", elapsed.Seconds()), 0)

				log.Printf("[processing] Frame %s processed in %.1fs → %s", frameID, elapsed.Seconds(), result.JpegPath)
			}
		}
	}
}
