package main

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/piwi3910/NovaSky/internal/config"
	"github.com/piwi3910/NovaSky/internal/db"
	"github.com/piwi3910/NovaSky/internal/gpio"
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
		BME280 struct {
			Enabled bool   `json:"enabled"`
			Device  string `json:"device"`
			Address uint8  `json:"address"`
		} `json:"bme280"`
		RainPin    int `json:"rainPin"`
		DewHeater  struct {
			Enabled    bool    `json:"enabled"`
			PWMChip    int     `json:"pwmChip"`
			PWMChannel int     `json:"pwmChannel"`
			DeltaTemp  float64 `json:"deltaTemp"`
		} `json:"dewHeater"`
	}
	cfg.Get("gpio", &gpioCfg)

	// Set defaults
	if gpioCfg.BME280.Device == "" {
		gpioCfg.BME280.Device = "/dev/i2c-1"
	}
	if gpioCfg.BME280.Address == 0 {
		gpioCfg.BME280.Address = 0x76
	}
	if gpioCfg.DewHeater.DeltaTemp == 0 {
		gpioCfg.DewHeater.DeltaTemp = 3.0
	}

	// Check if I2C is available
	i2cAvailable := false
	if _, err := os.Stat(gpioCfg.BME280.Device); err == nil {
		i2cAvailable = true
		log.Printf("[gpio] I2C bus detected at %s", gpioCfg.BME280.Device)
	}

	// Initialize dew heater controller
	dewCtrl := gpio.NewDewHeaterController(gpioCfg.DewHeater.DeltaTemp)
	dewHeaterActive := false
	if gpioCfg.DewHeater.Enabled {
		if err := dewCtrl.EnablePWM(gpioCfg.DewHeater.PWMChip, gpioCfg.DewHeater.PWMChannel); err != nil {
			log.Printf("[gpio] failed to enable dew heater PWM: %v", err)
		} else {
			dewHeaterActive = true
			log.Printf("[gpio] Dew heater active (chip %d, channel %d, delta %.1f°C)",
				gpioCfg.DewHeater.PWMChip, gpioCfg.DewHeater.PWMChannel, gpioCfg.DewHeater.DeltaTemp)
		}
	}

	// BME280 reads every 10s, health updates every 15s
	bmeTicker := time.NewTicker(10 * time.Second)
	defer bmeTicker.Stop()
	healthTicker := time.NewTicker(15 * time.Second)
	defer healthTicker.Stop()

	// Track latest readings for dew heater PID
	var lastTemp, lastDewPoint float64
	hasDewPoint := false

	log.Printf("[gpio] Service ready (I2C: %v, BME280: %v, DewHeater: %v)",
		i2cAvailable, gpioCfg.BME280.Enabled, dewHeaterActive)

	for {
		select {
		case <-ctx.Done():
			if dewHeaterActive {
				dewCtrl.Disable()
			}
			return

		case <-bmeTicker.C:
			cfg.Get("gpio", &gpioCfg)

			if gpioCfg.BME280.Enabled && i2cAvailable {
				temp, humidity, pressure, err := gpio.ReadBME280(gpioCfg.BME280.Device, gpioCfg.BME280.Address)
				if err != nil {
					log.Printf("[gpio] BME280 read error: %v", err)
					continue
				}

				now := time.Now()
				readings := []models.SensorReading{
					{SensorType: "temperature", Value: temp, Unit: "°C", Source: "bme280", RecordedAt: now},
					{SensorType: "humidity", Value: humidity, Unit: "%", Source: "bme280", RecordedAt: now},
					{SensorType: "pressure", Value: pressure, Unit: "hPa", Source: "bme280", RecordedAt: now},
				}

				for _, r := range readings {
					if err := db.GetDB().Create(&r).Error; err != nil {
						log.Printf("[gpio] failed to store %s: %v", r.SensorType, err)
					}
				}

				// Publish sensor data to Redis
				event, _ := json.Marshal(map[string]any{
					"source":      "bme280",
					"temperature": temp,
					"humidity":    humidity,
					"pressure":    pressure,
					"timestamp":   now.Unix(),
				})
				novaskyRedis.Publish(ctx, "novasky:sensor-update", string(event))

				log.Printf("[gpio] BME280: %.1f°C, %.0f%% RH, %.1f hPa", temp, humidity, pressure)

				lastTemp = temp

				// Calculate dew point from BME280 readings using Magnus formula
				// Td = (b * alpha) / (a - alpha), where alpha = (a * T) / (b + T) + ln(RH/100)
				const a = 17.27
				const b = 237.7
				alpha := (a*temp)/(b+temp) + logRH(humidity)
				lastDewPoint = (b * alpha) / (a - alpha)
				hasDewPoint = true
			}

			// Run dew heater PID
			if dewHeaterActive && hasDewPoint {
				duty := dewCtrl.Update(lastTemp, lastDewPoint)
				if err := dewCtrl.SetDutyCycle(duty); err != nil {
					log.Printf("[gpio] dew heater set duty error: %v", err)
				} else {
					log.Printf("[gpio] Dew heater: temp=%.1f°C dew=%.1f°C duty=%d%%", lastTemp, lastDewPoint, duty)
				}
			}

			if gpioCfg.RainPin > 0 {
				log.Printf("[gpio] Would read rain sensor on GPIO %d", gpioCfg.RainPin)
			}

		case <-healthTicker.C:
			health := models.ServiceHealth{Name: "gpio", LastSeen: time.Now(), Status: "running"}
			db.GetDB().Save(&health)
		}
	}
}

// logRH returns ln(RH/100), clamping RH to avoid log(0).
func logRH(rh float64) float64 {
	if rh <= 0 {
		rh = 0.1
	}
	if rh > 100 {
		rh = 100
	}
	return math.Log(rh / 100.0)
}
