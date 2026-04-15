package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/piwi3910/NovaSky/internal/db"
	novaskyRedis "github.com/piwi3910/NovaSky/internal/redis"
)

const consumerGroup = "export"

func main() {
	log.Println("[export] Starting...")
	db.Init()
	novaskyRedis.Init()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { c := make(chan os.Signal, 1); signal.Notify(c, syscall.SIGINT, syscall.SIGTERM); <-c; cancel() }()

	novaskyRedis.StartHealthReporter(ctx, "export")

	novaskyRedis.CreateConsumerGroup(ctx, novaskyRedis.StreamFramesExport, consumerGroup)

	exportBase := os.Getenv("EXPORT_DIR")
	if exportBase == "" {
		exportBase = "/home/piwi/novasky-data/export"
	}

	log.Printf("[export] Worker started, export dir: %s", exportBase)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		streams, err := novaskyRedis.ReadFromGroup(ctx, novaskyRedis.StreamFramesExport, consumerGroup, "export-1", 1)
		if err != nil {
			continue
		}

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				filePath := msg.Values["filePath"].(string)
				jpegPath, _ := msg.Values["jpegPath"].(string)

				// Create date folder
				dateDir := filepath.Join(exportBase, time.Now().Format("2006-01-02"))
				os.MkdirAll(dateDir, 0755)

				ts := time.Now().Format("20060102_150405")

				// Copy FITS
				if filePath != "" {
					dst := filepath.Join(dateDir, fmt.Sprintf("novasky_%s.fits", ts))
					copyFile(filePath, dst)
				}

				// Copy JPEG
				if jpegPath != "" {
					dst := filepath.Join(dateDir, fmt.Sprintf("novasky_%s.jpg", ts))
					copyFile(jpegPath, dst)
				}

				novaskyRedis.AckMessage(ctx, novaskyRedis.StreamFramesExport, consumerGroup, msg.ID)
				novaskyRedis.ReportHealth(ctx, "export")
				log.Printf("[export] Exported frame to %s", dateDir)
			}
		}
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
