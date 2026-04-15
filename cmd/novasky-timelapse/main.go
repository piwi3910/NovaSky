package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/piwi3910/NovaSky/internal/db"
	novaskyRedis "github.com/piwi3910/NovaSky/internal/redis"
)

const consumerGroup = "timelapse"

func main() {
	log.Println("[timelapse] Starting...")
	db.Init()
	novaskyRedis.Init()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { c := make(chan os.Signal, 1); signal.Notify(c, syscall.SIGINT, syscall.SIGTERM); <-c; cancel() }()

	novaskyRedis.StartHealthReporter(ctx, "timelapse")

	novaskyRedis.CreateConsumerGroup(ctx, novaskyRedis.StreamFramesTimelapse, consumerGroup)

	// Accumulated frames for timelapse
	var frameFiles []string
	var lastGenerateTime time.Time

	log.Println("[timelapse] Worker started — collecting frames")

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		streams, err := novaskyRedis.ReadFromGroup(ctx, novaskyRedis.StreamFramesTimelapse, consumerGroup, "timelapse-1", 1)
		if err != nil {
			continue
		}

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				jpegPath, _ := msg.Values["jpegPath"].(string)
				if jpegPath != "" {
					frameFiles = append(frameFiles, jpegPath)
				}
				novaskyRedis.AckMessage(ctx, novaskyRedis.StreamFramesTimelapse, consumerGroup, msg.ID)
				novaskyRedis.ReportHealth(ctx, "timelapse")
			}
		}

		// Generate timelapse every 100 frames or every hour
		if len(frameFiles) >= 100 || (len(frameFiles) > 10 && time.Since(lastGenerateTime) > time.Hour) {
			generateTimelapse(frameFiles)
			frameFiles = nil
			lastGenerateTime = time.Now()
		}
	}
}

func generateTimelapse(frames []string) {
	if len(frames) == 0 {
		return
	}

	outDir := "/home/piwi/novasky-data/timelapse"
	os.MkdirAll(outDir, 0755)

	// Create a file list for ffmpeg
	listFile := filepath.Join(outDir, "frames.txt")
	f, err := os.Create(listFile)
	if err != nil {
		log.Printf("[timelapse] Failed to create frame list: %v", err)
		return
	}
	for _, frame := range frames {
		fmt.Fprintf(f, "file '%s'\nduration 0.1\n", frame)
	}
	f.Close()

	outputFile := filepath.Join(outDir, fmt.Sprintf("timelapse_%s.mp4", time.Now().Format("20060102_150405")))

	cmd := exec.Command("ffmpeg", "-y", "-f", "concat", "-safe", "0", "-i", listFile,
		"-vf", "scale=1920:-1", "-c:v", "libx264", "-preset", "fast", "-crf", "23",
		"-pix_fmt", "yuv420p", outputFile)

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[timelapse] ffmpeg failed: %v\n%s", err, string(output))
		return
	}

	log.Printf("[timelapse] Generated: %s (%d frames)", outputFile, len(frames))
	os.Remove(listFile)
}
