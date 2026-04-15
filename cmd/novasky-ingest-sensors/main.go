package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/piwi3910/NovaSky/internal/db"
	"github.com/piwi3910/NovaSky/internal/models"
	novaskyRedis "github.com/piwi3910/NovaSky/internal/redis"
)

func main() {
	log.Println("[ingest-sensors] Starting...")
	db.Init()
	novaskyRedis.Init()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { c := make(chan os.Signal, 1); signal.Notify(c, syscall.SIGINT, syscall.SIGTERM); <-c; cancel() }()

	apiURL := os.Getenv("API_URL")
	if apiURL == "" {
		apiURL = "http://localhost:3000"
	}

	log.Println("[ingest-sensors] Sensor loop started")

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Read GPSD for location-based data
		resp, err := http.Get(apiURL + "/api/gpsd")
		if err == nil {
			var gpsd struct {
				Available bool    `json:"available"`
				Latitude  float64 `json:"latitude"`
				Longitude float64 `json:"longitude"`
				Elevation float64 `json:"elevation"`
			}
			json.NewDecoder(resp.Body).Decode(&gpsd)
			resp.Body.Close()
		}

		// Camera temperature is read by ingest-camera via INDI
		// We can read it from the status endpoint
		resp, err = http.Get(apiURL + "/api/status")
		if err == nil {
			resp.Body.Close()
		}

		// For now, log a heartbeat
		health := models.ServiceHealth{Name: "ingest-sensors", LastSeen: time.Now(), Status: "running"}
		db.GetDB().Save(&health)

		time.Sleep(15 * time.Second)
	}
}
