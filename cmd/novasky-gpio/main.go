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
	novaskyRedis.StartHealthReporter(ctx, "gpio")

	cfg := config.NewManager()
	cfg.Subscribe(ctx)

	var gpioCfg struct {
		I2CEnabled    bool   `json:"i2cEnabled"`
		I2CDevice     string `json:"i2cDevice"`
		BME280Address int    `json:"bme280Address"`
		RainPin       int    `json:"rainPin"`
		DewHeater     struct {
			Enabled   bool    `json:"enabled"`
			Pin       int     `json:"pin"`
			DeltaTemp float64 `json:"deltaTemp"`
		} `json:"dewHeater"`
	}
	cfg.Get("gpio", &gpioCfg)

	// Check if I2C is available
	i2cAvailable := false
	if _, err := os.Stat("/dev/i2c-1"); err == nil {
		i2cAvailable = true
		log.Println("[gpio] I2C bus detected at /dev/i2c-1")
	}

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	log.Printf("[gpio] Service ready (I2C: %v)", i2cAvailable)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cfg.Get("gpio", &gpioCfg)

			if gpioCfg.I2CEnabled && i2cAvailable {
				// Read BME280 sensor (would use periph.io/x/conn/v3/i2c in production)
				// For now, log that we would read
				log.Println("[gpio] Would read BME280 sensor via I2C")
			}

			if gpioCfg.RainPin > 0 {
				// Read rain sensor GPIO
				// Would use periph.io/x/host/v3/rpi
				log.Printf("[gpio] Would read rain sensor on GPIO %d", gpioCfg.RainPin)
			}

			if gpioCfg.DewHeater.Enabled {
				// PID control for dew heater
				// Read current temp + humidity, calculate dew point, adjust PWM
				log.Printf("[gpio] Dew heater active, target delta: +%.1f°C", gpioCfg.DewHeater.DeltaTemp)
			}

			// Report health
			health := models.ServiceHealth{Name: "gpio", LastSeen: time.Now(), Status: "running"}
			db.GetDB().Save(&health)
		}
	}
}
