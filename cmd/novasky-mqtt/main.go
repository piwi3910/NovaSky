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
	log.Println("[mqtt] Starting...")
	db.Init()
	novaskyRedis.Init()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { c := make(chan os.Signal, 1); signal.Notify(c, syscall.SIGINT, syscall.SIGTERM); <-c; cancel() }()
	novaskyRedis.StartHealthReporter(ctx, "mqtt")

	cfg := config.NewManager()
	cfg.Subscribe(ctx)

	// Read MQTT config
	var mqttCfg struct {
		Broker   string `json:"broker"`
		Username string `json:"username"`
		Password string `json:"password"`
		Enabled  bool   `json:"enabled"`
	}
	cfg.Get("mqtt", &mqttCfg)

	if !mqttCfg.Enabled || mqttCfg.Broker == "" {
		log.Println("[mqtt] MQTT disabled or no broker configured. Waiting...")
		cfg.OnChange(func(key string) {
			if key == "mqtt" {
				cfg.Get("mqtt", &mqttCfg)
				log.Printf("[mqtt] Config changed: enabled=%v broker=%s", mqttCfg.Enabled, mqttCfg.Broker)
			}
		})
	}

	// Subscribe to safety state changes
	sub := novaskyRedis.Client.Subscribe(ctx, novaskyRedis.ChannelSafetyState)
	ch := sub.Channel()

	log.Println("[mqtt] Service ready")

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			if !mqttCfg.Enabled {
				continue
			}
			log.Printf("[mqtt] Safety state → %s", msg.Payload)
			// Would publish to: homeassistant/binary_sensor/novasky_safety/state
		case <-ticker.C:
			if !mqttCfg.Enabled {
				continue
			}
			var readings []models.SensorReading
			db.GetDB().Raw("SELECT DISTINCT ON (sensor_type) * FROM sensor_readings ORDER BY sensor_type, recorded_at DESC").Scan(&readings)
			for _, r := range readings {
				log.Printf("[mqtt] Sensor %s: %.1f %s", r.SensorType, r.Value, r.Unit)
			}
		}
	}
}
