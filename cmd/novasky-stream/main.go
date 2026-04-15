package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/piwi3910/NovaSky/internal/db"
	"github.com/piwi3910/NovaSky/internal/models"
)

func main() {
	log.Println("[stream] Starting...")
	db.Init()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { c := make(chan os.Signal, 1); signal.Notify(c, syscall.SIGINT, syscall.SIGTERM); <-c; cancel() }()

	app := fiber.New(fiber.Config{DisableStartupMessage: true})

	// MJPEG stream endpoint
	app.Get("/stream", func(c *fiber.Ctx) error {
		c.Set("Content-Type", "multipart/x-mixed-replace; boundary=frame")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")

		c.Context().SetBodyStreamWriter(func(w *fiber.StreamWriter) {
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}

				// Get latest frame with JPEG
				var frame models.Frame
				if err := db.GetDB().Where("jpeg_path IS NOT NULL").Order("created_at DESC").First(&frame).Error; err != nil {
					time.Sleep(1 * time.Second)
					continue
				}

				if frame.JpegPath != nil {
					jpegData, err := os.ReadFile(*frame.JpegPath)
					if err == nil {
						header := fmt.Sprintf("--frame\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\n\r\n", len(jpegData))
						w.Write([]byte(header))
						w.Write(jpegData)
						w.Write([]byte("\r\n"))
						w.Flush()
					}
				}

				// Frame rate ~1fps (adjustable)
				time.Sleep(1 * time.Second)
			}
		})

		return nil
	})

	// Status
	app.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"service": "novasky-stream",
			"type":    "MJPEG",
			"url":     "/stream",
		})
	})

	port := os.Getenv("STREAM_PORT")
	if port == "" {
		port = "8090"
	}

	go func() {
		if err := app.Listen(":" + port); err != nil {
			log.Fatalf("[stream] Server error: %v", err)
		}
	}()

	log.Printf("[stream] MJPEG stream on port %s", port)
	<-ctx.Done()
	app.Shutdown()
}
