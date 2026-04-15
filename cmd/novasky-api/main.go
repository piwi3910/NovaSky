package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"

	"github.com/piwi3910/NovaSky/internal/config"
	"github.com/piwi3910/NovaSky/internal/db"
	"github.com/piwi3910/NovaSky/internal/diskmanager"
	"github.com/piwi3910/NovaSky/internal/models"
	"github.com/piwi3910/NovaSky/internal/platesolve"
	novaskyRedis "github.com/piwi3910/NovaSky/internal/redis"
)

// WebSocket clients
var (
	wsClients   = make(map[*websocket.Conn]bool)
	wsClientsMu sync.Mutex
)

func main() {
	log.Println("[api] Starting...")

	db.Init()
	novaskyRedis.Init()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := config.NewManager()
	cfg.Subscribe(ctx)

	app := fiber.New(fiber.Config{
		BodyLimit: 50 * 1024 * 1024, // 50MB for JPEG frames
	})
	app.Use(cors.New())

	// Health check
	app.Get("/api/health", func(c *fiber.Ctx) error {
		var services []models.ServiceHealth
		db.GetDB().Find(&services)
		return c.JSON(services)
	})

	// Status — latest safety, analysis, frame
	app.Get("/api/status", func(c *fiber.Ctx) error {
		var safety models.SafetyState
		var analysis models.AnalysisResult
		var frame models.Frame

		db.GetDB().Order("evaluated_at DESC").First(&safety)
		db.GetDB().Order("analyzed_at DESC").First(&analysis)
		db.GetDB().Order("created_at DESC").First(&frame)

		return c.JSON(fiber.Map{
			"safety":    safety,
			"analysis":  analysis,
			"frame":     frame,
			"timestamp": time.Now(),
		})
	})

	// Frames
	app.Get("/api/frames", func(c *fiber.Ctx) error {
		limit := c.QueryInt("limit", 20)
		offset := c.QueryInt("offset", 0)
		var frames []models.Frame
		db.GetDB().Order("created_at DESC").Limit(limit).Offset(offset).Find(&frames)
		return c.JSON(fiber.Map{"frames": frames, "limit": limit, "offset": offset})
	})

	app.Get("/api/frames/:id", func(c *fiber.Ctx) error {
		var frame models.Frame
		if err := db.GetDB().First(&frame, "id = ?", c.Params("id")).Error; err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "Frame not found"})
		}
		var analysis models.AnalysisResult
		db.GetDB().First(&analysis, "frame_id = ?", frame.ID)
		return c.JSON(fiber.Map{"frame": frame, "analysis": analysis})
	})

	app.Get("/api/frames/:id/jpeg", func(c *fiber.Ctx) error {
		var frame models.Frame
		if err := db.GetDB().First(&frame, "id = ?", c.Params("id")).Error; err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "Frame not found"})
		}
		if frame.JpegPath == nil {
			return c.Status(404).JSON(fiber.Map{"error": "JPEG not found"})
		}
		return c.SendFile(*frame.JpegPath)
	})

	// Analysis
	app.Get("/api/analysis", func(c *fiber.Ctx) error {
		limit := c.QueryInt("limit", 50)
		offset := c.QueryInt("offset", 0)
		var results []models.AnalysisResult
		db.GetDB().Order("analyzed_at DESC").Limit(limit).Offset(offset).Find(&results)
		return c.JSON(fiber.Map{"results": results})
	})

	// Sensors
	app.Get("/api/sensors", func(c *fiber.Ctx) error {
		var readings []models.SensorReading
		db.GetDB().Raw(`SELECT DISTINCT ON (sensor_type) * FROM sensor_readings ORDER BY sensor_type, recorded_at DESC`).Scan(&readings)
		return c.JSON(fiber.Map{"sensors": readings})
	})

	// Safety history
	app.Get("/api/safety-history", func(c *fiber.Ctx) error {
		limit := c.QueryInt("limit", 100)
		var history []models.SafetyState
		db.GetDB().Order("evaluated_at DESC").Limit(limit).Find(&history)
		return c.JSON(fiber.Map{"history": history})
	})

	// Config
	app.Get("/api/config", func(c *fiber.Ctx) error {
		return c.JSON(cfg.GetAll())
	})

	app.Get("/api/config/:key", func(c *fiber.Ctx) error {
		raw := cfg.GetRaw(c.Params("key"))
		if raw == nil {
			return c.Status(404).JSON(fiber.Map{"error": "Config not found"})
		}
		return c.JSON(fiber.Map{"key": c.Params("key"), "value": json.RawMessage(raw)})
	})

	app.Put("/api/config/:key", func(c *fiber.Ctx) error {
		var body struct {
			Value interface{} `json:"value"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid body"})
		}
		if err := cfg.Set(c.Params("key"), body.Value); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"key": c.Params("key"), "value": body.Value})
	})

	// Plate solve status
	app.Get("/api/platesolve", func(c *fiber.Ctx) error {
		wcs := platesolve.GetCachedWCS()
		if wcs == nil {
			return c.JSON(fiber.Map{"solved": false})
		}
		return c.JSON(fiber.Map{
			"solved": true,
			"ra":     wcs.CRVAL1, "dec": wcs.CRVAL2,
			"crpix1": wcs.CRPIX1, "crpix2": wcs.CRPIX2,
		})
	})

	// Devices (proxy to INDI — return from DB config for now)
	app.Get("/api/devices", func(c *fiber.Ctx) error {
		var device string
		cfg.Get("camera.device", &device)
		devices := []string{}
		if device != "" {
			devices = append(devices, device)
		}
		return c.JSON(fiber.Map{"devices": devices})
	})

	// GPSD
	app.Get("/api/gpsd", func(c *fiber.Ctx) error {
		result := queryGPSD()
		return c.JSON(result)
	})

	// Capture (manual trigger — publish to stream)
	app.Post("/api/capture", func(c *fiber.Ctx) error {
		// Publish a capture request — ingest-camera will pick it up
		return c.JSON(fiber.Map{"status": "capture requested"})
	})

	// Disk usage
	app.Get("/api/disk", func(c *fiber.Ctx) error {
		spoolDir := os.Getenv("FRAME_SPOOL_DIR")
		if spoolDir == "" {
			spoolDir = "/home/piwi/novasky-data/frames"
		}
		total, used, free := diskmanager.GetUsage(spoolDir)
		return c.JSON(fiber.Map{
			"totalGB": math.Round(total*10) / 10,
			"usedGB":  math.Round(used*10) / 10,
			"freeGB":  math.Round(free*10) / 10,
			"path":    spoolDir,
		})
	})

	// Prometheus metrics
	app.Get("/metrics", func(c *fiber.Ctx) error {
		var frameCount int64
		db.GetDB().Model(&models.Frame{}).Count(&frameCount)
		var safetyCount int64
		db.GetDB().Model(&models.SafetyState{}).Count(&safetyCount)

		metrics := "# HELP novasky_frames_total Total captured frames\n"
		metrics += "# TYPE novasky_frames_total counter\n"
		metrics += "novasky_frames_total " + itoa(int(frameCount)) + "\n"
		metrics += "# HELP novasky_safety_evaluations_total Total safety evaluations\n"
		metrics += "# TYPE novasky_safety_evaluations_total counter\n"
		metrics += "novasky_safety_evaluations_total " + itoa(int(safetyCount)) + "\n"

		c.Set("Content-Type", "text/plain; charset=utf-8")
		return c.SendString(metrics)
	})

	// Pipeline status — health, latency, queue depths
	app.Get("/api/pipeline", func(c *fiber.Ctx) error {
		ctx := context.Background()

		// Pending counts (unacknowledged messages — actual backpressure)
		qProcessing, _ := novaskyRedis.GetPendingCount(ctx, novaskyRedis.StreamFramesProcessing, "processing")
		qDetection, _ := novaskyRedis.GetPendingCount(ctx, novaskyRedis.StreamFramesDetection, "detection")
		qOverlay, _ := novaskyRedis.GetPendingCount(ctx, novaskyRedis.StreamFramesOverlay, "overlay")
		qExport, _ := novaskyRedis.GetPendingCount(ctx, novaskyRedis.StreamFramesExport, "export")
		qTimelapse, _ := novaskyRedis.GetPendingCount(ctx, novaskyRedis.StreamFramesTimelapse, "timelapse")
		qPolicy, _ := novaskyRedis.GetPendingCount(ctx, novaskyRedis.StreamPolicyEvaluate, "policy")
		qAlerts, _ := novaskyRedis.GetPendingCount(ctx, novaskyRedis.StreamAlertsDispatch, "alerts")

		// Service health from Redis heartbeats
		getHealth := func(name string) string {
			return novaskyRedis.GetServiceHealth(ctx, name)
		}

		// Read actual per-frame processing latencies from Redis
		var processLatency, detectLatency float64
		if val, err := novaskyRedis.Client.Get(ctx, "novasky:latency:processing").Float64(); err == nil {
			processLatency = val
		}
		if val, err := novaskyRedis.Client.Get(ctx, "novasky:latency:detection").Float64(); err == nil {
			detectLatency = val
		}

		type ServiceNode struct {
			Name       string  `json:"name"`
			Status     string  `json:"status"`
			QueueDepth int64   `json:"queueDepth"`
			Latency    float64 `json:"latency"`
		}

		pipeline := []ServiceNode{
			{Name: "ingest-camera", Status: getHealth("ingest-camera"), QueueDepth: 0, Latency: 0},
			{Name: "processing", Status: getHealth("processing"), QueueDepth: qProcessing, Latency: processLatency},
			{Name: "detection", Status: getHealth("detection"), QueueDepth: qDetection, Latency: detectLatency},
			{Name: "policy", Status: getHealth("policy"), QueueDepth: qPolicy, Latency: 0},
			{Name: "overlay", Status: getHealth("overlay"), QueueDepth: qOverlay, Latency: 0},
			{Name: "export", Status: getHealth("export"), QueueDepth: qExport, Latency: 0},
			{Name: "timelapse", Status: getHealth("timelapse"), QueueDepth: qTimelapse, Latency: 0},
			{Name: "alerts", Status: getHealth("alerts"), QueueDepth: qAlerts, Latency: 0},
			{Name: "api", Status: "running", QueueDepth: 0, Latency: 0},
			{Name: "alpaca", Status: getHealth("alpaca"), QueueDepth: 0, Latency: 0},
			{Name: "stream", Status: getHealth("stream"), QueueDepth: 0, Latency: 0},
		}

		return c.JSON(fiber.Map{"services": pipeline})
	})

	// Chart data for in-app graphs
	app.Get("/api/charts/cloud-cover", func(c *fiber.Ctx) error {
		hours := c.QueryInt("hours", 24)
		var results []models.AnalysisResult
		since := time.Now().Add(-time.Duration(hours) * time.Hour)
		db.GetDB().Where("analyzed_at > ?", since).Order("analyzed_at ASC").Find(&results)
		type Point struct {
			Time  string  `json:"time"`
			Value float64 `json:"value"`
		}
		points := make([]Point, len(results))
		for i, r := range results {
			points[i] = Point{Time: r.AnalyzedAt.Format(time.RFC3339), Value: r.CloudCover * 100}
		}
		return c.JSON(points)
	})

	app.Get("/api/charts/exposure", func(c *fiber.Ctx) error {
		hours := c.QueryInt("hours", 24)
		var frames []models.Frame
		since := time.Now().Add(-time.Duration(hours) * time.Hour)
		db.GetDB().Where("created_at > ?", since).Order("created_at ASC").Find(&frames)
		type Point struct {
			Time  string  `json:"time"`
			Value float64 `json:"value"`
		}
		points := make([]Point, len(frames))
		for i, f := range frames {
			points[i] = Point{Time: f.CreatedAt.Format(time.RFC3339), Value: f.ExposureMs}
		}
		return c.JSON(points)
	})

	// Focus mode
	var focusMode bool
	app.Post("/api/focus/start", func(c *fiber.Ctx) error {
		focusMode = true
		// Publish focus mode change
		novaskyRedis.Publish(context.Background(), "novasky:focus-mode", "start")
		return c.JSON(fiber.Map{"focusMode": true})
	})

	app.Post("/api/focus/stop", func(c *fiber.Ctx) error {
		focusMode = false
		novaskyRedis.Publish(context.Background(), "novasky:focus-mode", "stop")
		return c.JSON(fiber.Map{"focusMode": false})
	})

	app.Get("/api/focus/status", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"focusMode": focusMode})
	})

	// Weather forecast from Open-Meteo
	app.Get("/api/weather", func(c *fiber.Ctx) error {
		var loc struct {
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		}
		cfg.Get("location", &loc)
		if loc.Latitude == 0 && loc.Longitude == 0 {
			return c.JSON(fiber.Map{"error": "location not configured"})
		}

		url := fmt.Sprintf("https://api.open-meteo.com/v1/forecast?latitude=%.4f&longitude=%.4f&hourly=cloud_cover,temperature_2m,relative_humidity_2m,wind_speed_10m&forecast_days=1&timezone=auto", loc.Latitude, loc.Longitude)

		resp, err := http.Get(url)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		defer resp.Body.Close()

		var weather interface{}
		json.NewDecoder(resp.Body).Decode(&weather)
		return c.JSON(weather)
	})

	// Timelapse list
	app.Get("/api/timelapse", func(c *fiber.Ctx) error {
		dir := "/home/piwi/novasky-data/timelapse"
		entries, _ := filepath.Glob(filepath.Join(dir, "*.mp4"))
		type TL struct {
			Name string `json:"name"`
			Path string `json:"path"`
			Size int64  `json:"size"`
		}
		var timelapses []TL
		for _, e := range entries {
			fi, _ := os.Stat(e)
			if fi != nil {
				timelapses = append(timelapses, TL{
					Name: filepath.Base(e),
					Path: e,
					Size: fi.Size(),
				})
			}
		}
		return c.JSON(fiber.Map{"timelapses": timelapses})
	})

	// Serve timelapse video
	app.Get("/api/timelapse/:name", func(c *fiber.Ctx) error {
		path := filepath.Join("/home/piwi/novasky-data/timelapse", c.Params("name"))
		return c.SendFile(path)
	})

	// WebSocket
	app.Use("/ws", func(c *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})

	app.Get("/ws", websocket.New(func(c *websocket.Conn) {
		wsClientsMu.Lock()
		wsClients[c] = true
		wsClientsMu.Unlock()
		log.Printf("[api:ws] Client connected (%d total)", len(wsClients))

		defer func() {
			wsClientsMu.Lock()
			delete(wsClients, c)
			wsClientsMu.Unlock()
			c.Close()
			log.Printf("[api:ws] Client disconnected (%d total)", len(wsClients))
		}()

		for {
			if _, _, err := c.ReadMessage(); err != nil {
				break
			}
		}
	}))

	// Subscribe to Redis pub/sub and fan out to WebSocket clients
	go subscribeAndBroadcast(ctx)

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	port := os.Getenv("API_PORT")
	if port == "" {
		port = "3000"
	}

	go func() {
		if err := app.Listen(":" + port); err != nil {
			log.Fatalf("[api] Server error: %v", err)
		}
	}()

	log.Printf("[api] Server started on port %s", port)

	<-sigCh
	log.Println("[api] Shutting down...")
	cancel()
	app.Shutdown()
}

func subscribeAndBroadcast(ctx context.Context) {
	sub := novaskyRedis.Client.Subscribe(ctx,
		novaskyRedis.ChannelSafetyState,
		novaskyRedis.ChannelFrameNew,
		novaskyRedis.ChannelFrameProcessed,
		novaskyRedis.ChannelConfigChanged,
		novaskyRedis.ChannelAutoExposure,
		novaskyRedis.ChannelBackpressure,
	)
	ch := sub.Channel()

	for msg := range ch {
		// Extract type from channel name
		msgType := msg.Channel
		if idx := len("novasky:"); idx < len(msgType) {
			msgType = msgType[idx:]
		}

		payload, _ := json.Marshal(map[string]interface{}{
			"type": msgType,
			"data": json.RawMessage(msg.Payload),
		})

		wsClientsMu.Lock()
		for client := range wsClients {
			client.WriteMessage(websocket.TextMessage, payload)
		}
		wsClientsMu.Unlock()
	}
}

func queryGPSD() map[string]interface{} {
	conn, err := net.DialTimeout("tcp", "localhost:2947", 3*time.Second)
	if err != nil {
		return map[string]interface{}{"available": false}
	}
	defer conn.Close()

	conn.Write([]byte(`?WATCH={"enable":true,"json":true}` + "\n"))
	conn.SetReadDeadline(time.Now().Add(8 * time.Second))

	buf := make([]byte, 0, 65536)
	tmp := make([]byte, 4096)
	deadline := time.Now().Add(8 * time.Second)

	for time.Now().Before(deadline) {
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, err := conn.Read(tmp)
		if err != nil {
			break
		}
		buf = append(buf, tmp[:n]...)

		// Parse lines looking for TPV
		for {
			idx := indexOf(buf, '\n')
			if idx == -1 {
				break
			}
			line := buf[:idx]
			buf = buf[idx+1:]

			var msg map[string]interface{}
			if err := json.Unmarshal(line, &msg); err != nil {
				continue
			}
			if msg["class"] == "TPV" {
				mode, _ := msg["mode"].(float64)
				if mode >= 2 {
					lat, _ := msg["lat"].(float64)
					lon, _ := msg["lon"].(float64)
					alt, _ := msg["altMSL"].(float64)
					if alt == 0 {
						alt, _ = msg["alt"].(float64)
					}
					if lat != 0 || lon != 0 {
						return map[string]interface{}{
							"available": true,
							"latitude":  lat,
							"longitude": lon,
							"elevation": math.Round(alt*10) / 10,
						}
					}
				}
			}
		}
	}
	return map[string]interface{}{"available": false}
}

func indexOf(data []byte, b byte) int {
	for i, v := range data {
		if v == b {
			return i
		}
	}
	return -1
}

func itoa(i int) string {
	return fmt.Sprintf("%d", i)
}

