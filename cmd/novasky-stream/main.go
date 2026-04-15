package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/piwi3910/NovaSky/internal/db"
	"github.com/piwi3910/NovaSky/internal/models"
	novaskyRedis "github.com/piwi3910/NovaSky/internal/redis"
)

func main() {
	log.Println("[stream] Starting...")
	db.Init()
	novaskyRedis.Init()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { c := make(chan os.Signal, 1); signal.Notify(c, syscall.SIGINT, syscall.SIGTERM); <-c; cancel() }()

	go func() {
		for {
			novaskyRedis.ReportHealth(context.Background(), "stream")
			time.Sleep(30 * time.Second)
		}
	}()

	app := fiber.New(fiber.Config{DisableStartupMessage: true})

	// MJPEG stream — returns latest JPEG (poll for live view)
	app.Get("/stream/latest", func(c *fiber.Ctx) error {
		var frame models.Frame
		if err := db.GetDB().Where("jpeg_path IS NOT NULL").Order("created_at DESC").First(&frame).Error; err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "no frames"})
		}
		if frame.JpegPath == nil {
			return c.Status(404).JSON(fiber.Map{"error": "no JPEG"})
		}
		return c.SendFile(*frame.JpegPath)
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
