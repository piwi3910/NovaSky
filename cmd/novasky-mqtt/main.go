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
	novaskyRedis "github.com/piwi3910/NovaSky/internal/redis"
)

func main() {
	log.Println("[mqtt] Starting...")
	db.Init()
	novaskyRedis.Init()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { c := make(chan os.Signal, 1); signal.Notify(c, syscall.SIGINT, syscall.SIGTERM); <-c; cancel() }()

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

	if !mqttCfg.Enabled {
		log.Println("[mqtt] MQTT integration disabled, waiting for config change...")
		// Wait for config to enable it
		cfg.OnChange(func(key string) {
			if key == "mqtt" {
				cfg.Get("mqtt", &mqttCfg)
				if mqttCfg.Enabled {
					log.Println("[mqtt] MQTT enabled via config change")
				}
			}
		})
	}

	// Subscribe to safety state changes
	sub := novaskyRedis.Client.Subscribe(ctx, novaskyRedis.ChannelSafetyState)
	ch := sub.Channel()

	log.Println("[mqtt] Service started (stub — logging MQTT publishes)")

	// Periodic sensor publish
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			var safety models.SafetyState
			json.Unmarshal([]byte(msg.Payload), &safety)
			log.Printf("[mqtt] Would publish safety state: %s to homeassistant/binary_sensor/novasky_safety/state", safety.State)
			// TODO: actual MQTT publish using paho.mqtt.golang
		case <-ticker.C:
			// Publish sensor readings
			var readings []models.SensorReading
			db.GetDB().Raw("SELECT DISTINCT ON (sensor_type) * FROM sensor_readings ORDER BY sensor_type, recorded_at DESC").Scan(&readings)
			for _, r := range readings {
				topic := fmt.Sprintf("homeassistant/sensor/novasky_%s/state", r.SensorType)
				log.Printf("[mqtt] Would publish %s: %.1f %s", topic, r.Value, r.Unit)
			}
		}
	}
}
