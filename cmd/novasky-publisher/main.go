package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/piwi3910/NovaSky/internal/config"
	"github.com/piwi3910/NovaSky/internal/db"
	novaskyRedis "github.com/piwi3910/NovaSky/internal/redis"
)

func main() {
	log.Println("[publisher] Starting...")
	db.Init()
	novaskyRedis.Init()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { c := make(chan os.Signal, 1); signal.Notify(c, syscall.SIGINT, syscall.SIGTERM); <-c; cancel() }()

	cfg := config.NewManager()
	cfg.Subscribe(ctx)

	log.Println("[publisher] Service started (stub — waiting for dawn triggers)")

	// Check periodically if it's dawn and timelapses are ready
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// TODO: check if it's dawn, find generated timelapses, upload to YouTube
			// Uses YouTube Data API v3 with OAuth2
			log.Println("[publisher] Checking for timelapse uploads... (stub)")
			_ = cfg
		}
	}
}
