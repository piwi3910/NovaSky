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

	"github.com/piwi3910/NovaSky/internal/config"
	"github.com/piwi3910/NovaSky/internal/db"
	"github.com/piwi3910/NovaSky/internal/models"
	novaskyRedis "github.com/piwi3910/NovaSky/internal/redis"
	"github.com/piwi3910/NovaSky/internal/weather"
)

func main() {
	log.Println("[ingest-sensors] Starting...")
	db.Init()
	novaskyRedis.Init()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { c := make(chan os.Signal, 1); signal.Notify(c, syscall.SIGINT, syscall.SIGTERM); <-c; cancel() }()

	novaskyRedis.StartHealthReporter(ctx, "ingest-sensors")

	cfg := config.NewManager()
	cfg.Subscribe(ctx)

	apiURL := os.Getenv("API_URL")
	if apiURL == "" {
		apiURL = "http://localhost:3000"
	}

	// Read weather config
	var weatherCfg struct {
		Source    string  `json:"source"`
		Latitude float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	}
	cfg.Get("weather", &weatherCfg)

	// Start Ecowitt HTTP listener if configured
	if weatherCfg.Source == "ecowitt" {
		go startEcowittListener(ctx)
	}

	log.Printf("[ingest-sensors] Sensor loop started (weather source: %s)", weatherCfg.Source)

	// Separate ticker for weather polling (5 min)
	weatherTicker := time.NewTicker(5 * time.Minute)
	defer weatherTicker.Stop()

	// Main sensor loop ticker (15 sec)
	sensorTicker := time.NewTicker(15 * time.Second)
	defer sensorTicker.Stop()

	// Fetch weather immediately on start if openmeteo
	if weatherCfg.Source == "openmeteo" {
		fetchAndStoreWeather(ctx, weatherCfg.Source, weatherCfg.Latitude, weatherCfg.Longitude)
	}

	for {
		select {
		case <-ctx.Done():
			return

		case <-weatherTicker.C:
			cfg.Get("weather", &weatherCfg)
			if weatherCfg.Source == "openmeteo" {
				fetchAndStoreWeather(ctx, weatherCfg.Source, weatherCfg.Latitude, weatherCfg.Longitude)
			}

		case <-sensorTicker.C:
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

			// Report health
			health := models.ServiceHealth{Name: "ingest-sensors", LastSeen: time.Now(), Status: "running"}
			db.GetDB().Save(&health)
		}
	}
}

// fetchAndStoreWeather fetches weather from the configured provider and stores as SensorReading records.
func fetchAndStoreWeather(ctx context.Context, source string, lat, lon float64) {
	data, err := weather.FetchWeather(source, lat, lon)
	if err != nil {
		log.Printf("[ingest-sensors] weather fetch error: %v", err)
		return
	}
	if data == nil {
		return
	}

	storeWeatherData(ctx, data, source)
}

// storeWeatherData saves a WeatherData as individual SensorReading records and publishes to Redis.
func storeWeatherData(ctx context.Context, data *weather.WeatherData, source string) {
	now := time.Now()
	readings := []models.SensorReading{
		{SensorType: "temperature", Value: data.Temperature, Unit: "°C", Source: source, RecordedAt: now},
		{SensorType: "humidity", Value: data.Humidity, Unit: "%", Source: source, RecordedAt: now},
		{SensorType: "wind_speed", Value: data.WindSpeed, Unit: "km/h", Source: source, RecordedAt: now},
		{SensorType: "wind_gusts", Value: data.WindGusts, Unit: "km/h", Source: source, RecordedAt: now},
		{SensorType: "cloud_cover", Value: data.CloudCover, Unit: "%", Source: source, RecordedAt: now},
		{SensorType: "dew_point", Value: data.DewPoint, Unit: "°C", Source: source, RecordedAt: now},
		{SensorType: "pressure", Value: data.Pressure, Unit: "hPa", Source: source, RecordedAt: now},
	}

	for _, r := range readings {
		if err := db.GetDB().Create(&r).Error; err != nil {
			log.Printf("[ingest-sensors] failed to store %s reading: %v", r.SensorType, err)
			continue
		}
	}

	// Publish weather event to Redis for other services
	event, _ := json.Marshal(data)
	novaskyRedis.Publish(ctx, "novasky:weather-update", string(event))

	// Trigger policy evaluation on new weather data
	novaskyRedis.PublishToStream(ctx, novaskyRedis.StreamPolicyEvaluate, map[string]interface{}{
		"trigger": "weather", "source": source,
	})

	log.Printf("[ingest-sensors] weather stored: %.1f°C, %.0f%% humidity, wind %.1f km/h (gusts %.1f), clouds %.0f%%",
		data.Temperature, data.Humidity, data.WindSpeed, data.WindGusts, data.CloudCover)
}

// startEcowittListener starts an HTTP server to receive Ecowitt weather station push data.
func startEcowittListener(ctx context.Context) {
	mux := http.NewServeMux()
	mux.HandleFunc("/data/report", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		data, err := weather.ParseEcowittRequest(r)
		if err != nil {
			log.Printf("[ingest-sensors] ecowitt parse error: %v", err)
			http.Error(w, "parse error", http.StatusBadRequest)
			return
		}

		storeWeatherData(ctx, data, "ecowitt")
		w.WriteHeader(http.StatusOK)
	})

	server := &http.Server{Addr: ":8085", Handler: mux}

	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		server.Shutdown(shutdownCtx)
	}()

	log.Println("[ingest-sensors] Ecowitt listener started on :8085")
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("[ingest-sensors] ecowitt listener error: %v", err)
	}
}
