package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/piwi3910/NovaSky/internal/db"
	"github.com/piwi3910/NovaSky/internal/models"
	novaskyRedis "github.com/piwi3910/NovaSky/internal/redis"
)

func main() {
	log.Println("[gpio] Starting...")
	db.Init()
	novaskyRedis.Init()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { c := make(chan os.Signal, 1); signal.Notify(c, syscall.SIGINT, syscall.SIGTERM); <-c; cancel() }()

	log.Println("[gpio] Service started (stub — no GPIO hardware detected)")

	// Periodic heartbeat
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			health := models.ServiceHealth{Name: "gpio", LastSeen: time.Now(), Status: "running"}
			db.GetDB().Save(&health)
			// TODO: read BME280/BME680 via I2C, rain sensor via GPIO, dew heater PWM control
		}
	}
}
