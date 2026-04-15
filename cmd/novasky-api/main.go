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
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"

	"github.com/piwi3910/NovaSky/internal/astronomy"
	"github.com/piwi3910/NovaSky/internal/config"
	"github.com/piwi3910/NovaSky/internal/db"
	"github.com/piwi3910/NovaSky/internal/diskmanager"
	"github.com/piwi3910/NovaSky/internal/models"
	"github.com/piwi3910/NovaSky/internal/platesolve"
	"github.com/piwi3910/NovaSky/internal/processing"
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

	// Register debayer function for plate solving
	platesolve.DebayerFunc = processing.DebayerToGray

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
		// Return stored calibration from config
		var cal platesolve.Calibration
		cfg.Get("camera.calibration", &cal)
		return c.JSON(cal)
	})

	// Manual plate solve calibration — triggered by user to determine camera rotation
	app.Post("/api/platesolve/calibrate", func(c *fiber.Ctx) error {
		// Clear previous log
		novaskyRedis.Client.Del(ctx, "novasky:calibration:log")
		novaskyRedis.Client.Del(ctx, "novasky:calibration:status")

		calLog := func(msg string) {
			novaskyRedis.Client.RPush(ctx, "novasky:calibration:log", msg)
			log.Printf("[calibrate] %s", msg)
		}

		// Get latest frame
		var frame models.Frame
		if err := db.GetDB().Where("file_path IS NOT NULL").Order("created_at DESC").First(&frame).Error; err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "No frames available"})
		}

		// Get optics config for FoV calculation
		var optics struct {
			FocalLength float64 `json:"focalLength"`
		}
		cfg.Get("camera.optics", &optics)
		if optics.FocalLength <= 0 {
			return c.Status(400).JSON(fiber.Map{"error": "Set focal length in Camera Settings first"})
		}

		pixelSize := 2.0
		fov := platesolve.CalcFoV(optics.FocalLength, pixelSize, 3552)

		// Use raw FITS — ASTAP handles 16-bit FITS natively with better dynamic range than JPEG
		imagePath := frame.FilePath

		// Run plate solve in background with logging
		go func() {
			calLog(fmt.Sprintf("Starting calibration on frame %s", frame.ID))
			calLog(fmt.Sprintf("Focal length: %.1fmm, Pixel size: %.1fµm", optics.FocalLength, pixelSize))
			calLog(fmt.Sprintf("Full frame FoV: %.1f°", fov))
			calLog(fmt.Sprintf("Image: %s", imagePath))

			// Check file exists
			if _, err := os.Stat(imagePath); err != nil {
				calLog(fmt.Sprintf("ERROR: Image file not found: %v", err))
				novaskyRedis.Client.Set(ctx, "novasky:calibration:status", "failed", 0)
				return
			}
			calLog("Image file found, cropping center region to reduce FoV...")

			// Get location for zenith hint
			var loc struct {
				Latitude  float64 `json:"latitude"`
				Longitude float64 `json:"longitude"`
			}
			cfg.Get("location", &loc)
			calLog(fmt.Sprintf("Observer location: lat=%.4f lon=%.4f", loc.Latitude, loc.Longitude))

			cal, err := platesolve.Calibrate(imagePath, fov, 3552, loc.Latitude, loc.Longitude, calLog)
			if err != nil {
				calLog(fmt.Sprintf("ERROR: Plate solve failed: %v", err))
				novaskyRedis.Client.Set(ctx, "novasky:calibration:status", "failed", 0)
				return
			}
			if !cal.Solved {
				calLog("FAILED: No star match found. Is it night time with a clear sky?")
				novaskyRedis.Client.Set(ctx, "novasky:calibration:status", "failed", 0)
				return
			}

			calLog(fmt.Sprintf("Solved! North angle: %.1f°", cal.NorthAngle))
			calLog(fmt.Sprintf("Center: RA=%.4f° Dec=%.4f°", cal.RA, cal.Dec))
			calLog(fmt.Sprintf("Pixel scale: %.2f arcsec/pixel", cal.PixelScale))
			cfg.Set("camera.calibration", cal)
			calLog("Calibration saved successfully")
			novaskyRedis.Client.Set(ctx, "novasky:calibration:status", "success", 0)
		}()

		return c.JSON(fiber.Map{"status": "started", "fov": fov})
	})

	// Calibration log polling
	app.Get("/api/platesolve/log", func(c *fiber.Ctx) error {
		logs, _ := novaskyRedis.Client.LRange(ctx, "novasky:calibration:log", 0, -1).Result()
		status, _ := novaskyRedis.Client.Get(ctx, "novasky:calibration:status").Result()
		return c.JSON(fiber.Map{"logs": logs, "status": status})
	})

	// Devices — query INDI sidecar or return config fallback
	app.Get("/api/devices", func(c *fiber.Ctx) error {
		var sidecarURL string
		cfg.Get("indi.sidecarUrl", &sidecarURL)

		if sidecarURL != "" {
			// Query INDI sidecar for device list
			httpClient := &http.Client{Timeout: 5 * time.Second}
			resp, err := httpClient.Get(sidecarURL + "/devices")
			if err == nil {
				defer resp.Body.Close()
				var result interface{}
				if json.NewDecoder(resp.Body).Decode(&result) == nil {
					return c.JSON(fiber.Map{"devices": result, "source": "sidecar"})
				}
			}
			// Sidecar unreachable — fall through to config
		}

		// Try connecting to INDI server directly
		var indiHost string
		var indiPort int
		cfg.Get("indi.host", &indiHost)
		cfg.Get("indi.port", &indiPort)

		if indiHost != "" && indiPort > 0 {
			conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", indiHost, indiPort), 3*time.Second)
			if err == nil {
				conn.Close()
				// INDI server is reachable — use the configured device
				var device string
				cfg.Get("camera.device", &device)
				devices := []string{}
				if device != "" {
					devices = append(devices, device)
				}
				return c.JSON(fiber.Map{"devices": devices, "source": "indi-server"})
			}
		}

		// Fallback — return configured device if any
		var device string
		cfg.Get("camera.device", &device)
		devices := []string{}
		if device != "" {
			devices = append(devices, device)
		}
		return c.JSON(fiber.Map{"devices": devices, "source": "config", "note": "INDI sidecar not configured or unreachable"})
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

	// Disk usage with retention info
	app.Get("/api/disk", func(c *fiber.Ctx) error {
		spoolDir := os.Getenv("FRAME_SPOOL_DIR")
		if spoolDir == "" {
			spoolDir = "/home/piwi/novasky-data/frames"
		}
		total, used, free := diskmanager.GetUsage(spoolDir)

		// Retention config
		var retentionDays int
		cfg.Get("storage.retention.days", &retentionDays)
		if retentionDays == 0 {
			retentionDays = 30
		}
		var retentionMaxGB float64
		cfg.Get("storage.retention.maxGB", &retentionMaxGB)
		if retentionMaxGB == 0 {
			retentionMaxGB = 50
		}

		// Frame file stats
		oldest, newest, frameSizeGB, fileCount := diskmanager.DirStats(spoolDir)

		result := fiber.Map{
			"totalGB":    math.Round(total*10) / 10,
			"usedGB":     math.Round(used*10) / 10,
			"freeGB":     math.Round(free*10) / 10,
			"path":       spoolDir,
			"frameFiles": fileCount,
			"frameSizeGB": math.Round(frameSizeGB*100) / 100,
			"retention": fiber.Map{
				"days":  retentionDays,
				"maxGB": retentionMaxGB,
			},
		}
		if fileCount > 0 {
			result["oldestFrame"] = oldest.Format(time.RFC3339)
			result["newestFrame"] = newest.Format(time.RFC3339)
		}
		return c.JSON(result)
	})

	// Public sharing page — simple read-only view
	app.Get("/public", func(c *fiber.Ctx) error {
		var frame models.Frame
		db.GetDB().Where("jpeg_path IS NOT NULL").Order("created_at DESC").First(&frame)

		var safety models.SafetyState
		db.GetDB().Order("evaluated_at DESC").First(&safety)

		var analysis models.AnalysisResult
		db.GetDB().Order("analyzed_at DESC").First(&analysis)

		html := fmt.Sprintf(`<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>NovaSky Observatory</title>
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta http-equiv="refresh" content="30">
<style>
body{margin:0;background:#111;color:#eee;font-family:sans-serif;text-align:center}
img{max-width:100%%;max-height:80vh;border-radius:8px;margin:20px auto}
.info{display:flex;gap:20px;justify-content:center;flex-wrap:wrap;padding:10px}
.badge{padding:4px 12px;border-radius:4px;font-weight:bold}
.safe{background:#4caf50}.unsafe{background:#f44336}.unknown{background:#ff9800}
h1{margin:10px 0;font-size:1.5em}
</style></head><body>
<h1>NovaSky All-Sky Camera</h1>
<img src="/api/frames/%s/jpeg" alt="Latest sky frame">
<div class="info">
<span class="badge %s">%s</span>
<span>Cloud: %.0f%%</span>
<span>Sky: %s</span>
<span>Updated: %s</span>
</div>
</body></html>`,
			frame.ID,
			strings.ToLower(safety.State), safety.State,
			analysis.CloudCover*100, analysis.SkyQuality,
			frame.CapturedAt.Format("15:04:05"),
		)
		c.Set("Content-Type", "text/html")
		return c.SendString(html)
	})

	// Prometheus metrics
	app.Get("/metrics", func(c *fiber.Ctx) error {
		ctx := context.Background()

		var frameCount int64
		db.GetDB().Model(&models.Frame{}).Count(&frameCount)
		var safetyCount int64
		db.GetDB().Model(&models.SafetyState{}).Count(&safetyCount)

		// Latest safety state
		var safety models.SafetyState
		db.GetDB().Order("evaluated_at DESC").First(&safety)
		safetyGauge := 1 // UNKNOWN
		if safety.State == "SAFE" {
			safetyGauge = 2
		}
		if safety.State == "UNSAFE" {
			safetyGauge = 0
		}

		// Latest analysis
		var analysis models.AnalysisResult
		db.GetDB().Order("analyzed_at DESC").First(&analysis)

		// Latest frame
		var frame models.Frame
		db.GetDB().Order("created_at DESC").First(&frame)

		// Queue depths
		qProcessing, _ := novaskyRedis.GetStreamLength(ctx, novaskyRedis.StreamFramesProcessing)

		// Disk
		total, used, free := diskmanager.GetUsage(os.Getenv("FRAME_SPOOL_DIR"))

		m := ""
		m += "# HELP novasky_frames_total Total captured frames\n# TYPE novasky_frames_total counter\n"
		m += fmt.Sprintf("novasky_frames_total %d\n", frameCount)
		m += "# HELP novasky_safety_state Current safety state (0=UNSAFE,1=UNKNOWN,2=SAFE)\n# TYPE novasky_safety_state gauge\n"
		m += fmt.Sprintf("novasky_safety_state %d\n", safetyGauge)
		m += "# HELP novasky_cloud_cover Current cloud cover percentage\n# TYPE novasky_cloud_cover gauge\n"
		m += fmt.Sprintf("novasky_cloud_cover %.1f\n", analysis.CloudCover*100)
		m += "# HELP novasky_exposure_ms Current exposure in milliseconds\n# TYPE novasky_exposure_ms gauge\n"
		m += fmt.Sprintf("novasky_exposure_ms %.3f\n", frame.ExposureMs)
		m += "# HELP novasky_gain Current camera gain\n# TYPE novasky_gain gauge\n"
		m += fmt.Sprintf("novasky_gain %d\n", frame.Gain)
		m += "# HELP novasky_queue_depth Processing queue depth\n# TYPE novasky_queue_depth gauge\n"
		m += fmt.Sprintf("novasky_queue_depth %d\n", qProcessing)
		m += "# HELP novasky_disk_free_gb Free disk space in GB\n# TYPE novasky_disk_free_gb gauge\n"
		m += fmt.Sprintf("novasky_disk_free_gb %.1f\n", free)
		m += "# HELP novasky_disk_used_gb Used disk space in GB\n# TYPE novasky_disk_used_gb gauge\n"
		m += fmt.Sprintf("novasky_disk_used_gb %.1f\n", used)
		m += "# HELP novasky_disk_total_gb Total disk space in GB\n# TYPE novasky_disk_total_gb gauge\n"
		m += fmt.Sprintf("novasky_disk_total_gb %.1f\n", total)

		if analysis.SQM != nil {
			m += "# HELP novasky_sqm Sky Quality Meter reading\n# TYPE novasky_sqm gauge\n"
			m += fmt.Sprintf("novasky_sqm %.2f\n", *analysis.SQM)
		}

		c.Set("Content-Type", "text/plain; charset=utf-8")
		return c.SendString(m)
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
		db.GetDB().Where("captured_at > ?", since).Order("captured_at ASC").Find(&frames)
		type Point struct {
			Time     string  `json:"time"`
			Exposure float64 `json:"exposure"`
			Gain     int     `json:"gain"`
		}
		points := make([]Point, len(frames))
		for i, f := range frames {
			points[i] = Point{
				Time:     f.CapturedAt.Format(time.RFC3339),
				Exposure: f.ExposureMs,
				Gain:     f.Gain,
			}
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

	// Keograms list
	app.Get("/api/keograms", func(c *fiber.Ctx) error {
		dir := "/home/piwi/novasky-data/keograms"
		entries, _ := filepath.Glob(filepath.Join(dir, "*.jpg"))
		type Item struct {
			Name string `json:"name"`
			Path string `json:"path"`
			Size int64  `json:"size"`
		}
		var items []Item
		for _, e := range entries {
			fi, _ := os.Stat(e)
			if fi != nil {
				items = append(items, Item{
					Name: filepath.Base(e),
					Path: e,
					Size: fi.Size(),
				})
			}
		}
		return c.JSON(fiber.Map{"keograms": items})
	})

	// Serve keogram image
	app.Get("/api/keograms/:name", func(c *fiber.Ctx) error {
		path := filepath.Join("/home/piwi/novasky-data/keograms", c.Params("name"))
		return c.SendFile(path)
	})

	// Star trails list
	app.Get("/api/startrails", func(c *fiber.Ctx) error {
		dir := "/home/piwi/novasky-data/startrails"
		entries, _ := filepath.Glob(filepath.Join(dir, "*.jpg"))
		type Item struct {
			Name string `json:"name"`
			Path string `json:"path"`
			Size int64  `json:"size"`
		}
		var items []Item
		for _, e := range entries {
			fi, _ := os.Stat(e)
			if fi != nil {
				items = append(items, Item{
					Name: filepath.Base(e),
					Path: e,
					Size: fi.Size(),
				})
			}
		}
		return c.JSON(fiber.Map{"startrails": items})
	})

	// Serve star trails image
	app.Get("/api/startrails/:name", func(c *fiber.Ctx) error {
		path := filepath.Join("/home/piwi/novasky-data/startrails", c.Params("name"))
		return c.SendFile(path)
	})

	// Panoramic list
	app.Get("/api/panoramic", func(c *fiber.Ctx) error {
		dir := "/home/piwi/novasky-data/panoramic"
		entries, _ := filepath.Glob(filepath.Join(dir, "*.jpg"))
		type Item struct {
			Name string `json:"name"`
			Path string `json:"path"`
			Size int64  `json:"size"`
		}
		var items []Item
		for _, e := range entries {
			fi, _ := os.Stat(e)
			if fi != nil {
				items = append(items, Item{
					Name: filepath.Base(e),
					Path: e,
					Size: fi.Size(),
				})
			}
		}
		return c.JSON(fiber.Map{"panoramic": items})
	})

	// Serve panoramic image
	app.Get("/api/panoramic/:name", func(c *fiber.Ctx) error {
		path := filepath.Join("/home/piwi/novasky-data/panoramic", c.Params("name"))
		return c.SendFile(path)
	})

	// Overlay layer config
	app.Get("/api/overlay/config", func(c *fiber.Ctx) error {
		raw := cfg.GetRaw("overlay.layers")
		if raw == nil {
			// Return defaults — all enabled
			return c.JSON(fiber.Map{
				"compass":            true,
				"grid":               true,
				"timestamp":          true,
				"moonPhase":          true,
				"safetyState":        true,
				"sensorData":         true,
				"starLabels":         true,
				"constellationLines": true,
			})
		}
		return c.Send(raw)
	})

	app.Put("/api/overlay/config", func(c *fiber.Ctx) error {
		var layers struct {
			Compass            *bool `json:"compass"`
			Grid               *bool `json:"grid"`
			Timestamp          *bool `json:"timestamp"`
			MoonPhase          *bool `json:"moonPhase"`
			SafetyState        *bool `json:"safetyState"`
			SensorData         *bool `json:"sensorData"`
			StarLabels         *bool `json:"starLabels"`
			ConstellationLines *bool `json:"constellationLines"`
		}
		if err := c.BodyParser(&layers); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid body"})
		}
		if err := cfg.Set("overlay.layers", layers); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(layers)
	})

	// Overlay layouts CRUD
	app.Get("/api/overlay/layouts", func(c *fiber.Ctx) error {
		var layouts []models.OverlayLayout
		db.GetDB().Order("created_at DESC").Find(&layouts)
		return c.JSON(layouts)
	})

	app.Post("/api/overlay/layouts", func(c *fiber.Ctx) error {
		var layout models.OverlayLayout
		if err := c.BodyParser(&layout); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid body"})
		}
		if layout.Name == "" {
			return c.Status(400).JSON(fiber.Map{"error": "Name is required"})
		}
		if err := db.GetDB().Create(&layout).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(201).JSON(layout)
	})

	app.Put("/api/overlay/layouts/:id", func(c *fiber.Ctx) error {
		var layout models.OverlayLayout
		if err := db.GetDB().First(&layout, "id = ?", c.Params("id")).Error; err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "Layout not found"})
		}
		if err := c.BodyParser(&layout); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid body"})
		}
		db.GetDB().Save(&layout)
		return c.JSON(layout)
	})

	app.Delete("/api/overlay/layouts/:id", func(c *fiber.Ctx) error {
		if err := db.GetDB().Delete(&models.OverlayLayout{}, "id = ?", c.Params("id")).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"deleted": true})
	})

	app.Put("/api/overlay/layouts/:id/activate", func(c *fiber.Ctx) error {
		// Deactivate all, then activate this one
		db.GetDB().Model(&models.OverlayLayout{}).Where("1=1").Update("is_active", false)
		db.GetDB().Model(&models.OverlayLayout{}).Where("id = ?", c.Params("id")).Update("is_active", true)
		return c.JSON(fiber.Map{"activated": c.Params("id")})
	})

	// Overlay data for a frame — returns all detections (stars, meteors, planes, satellites)
	app.Get("/api/frames/:id/overlay", func(c *fiber.Ctx) error {
		var detections []models.Detection
		db.GetDB().Where("frame_id = ?", c.Params("id")).Find(&detections)
		return c.JSON(fiber.Map{"overlays": detections})
	})

	// Latest detected stars
	app.Get("/api/stars", func(c *fiber.Ctx) error {
		var det models.Detection
		db.GetDB().Where("type = ?", "stars").Order("created_at DESC").First(&det)
		return c.JSON(det)
	})

	// Nightly summaries
	app.Get("/api/summaries", func(c *fiber.Ctx) error {
		limit := c.QueryInt("limit", 30)
		var summaries []models.NightlySummary
		db.GetDB().Order("date DESC").Limit(limit).Find(&summaries)
		return c.JSON(fiber.Map{"summaries": summaries})
	})

	// Process preview for tuner
	app.Get("/api/process-preview", func(c *fiber.Ctx) error {
		frameID := c.Query("frameId")
		stretch := c.Query("stretch", "none")

		var frame models.Frame
		if err := db.GetDB().First(&frame, "id = ?", frameID).Error; err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "Frame not found"})
		}

		// Process with custom params
		result, err := processing.ProcessFrame(frame.FilePath, stretch, nil, false)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		return c.SendFile(result.JpegPath)
	})

	// Astronomy data
	app.Get("/api/astronomy", func(c *fiber.Ctx) error {
		var loc struct {
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		}
		cfg.Get("location", &loc)

		now := time.Now()
		moonIllum, moonPhase := astronomy.MoonPhase(now)
		sunTimes := astronomy.CalculateSunTimes(now, loc.Latitude, loc.Longitude)

		// Get latest SQM for Bortle
		var analysis models.AnalysisResult
		db.GetDB().Where("sqm IS NOT NULL").Order("analyzed_at DESC").First(&analysis)
		var bortle int
		var bortleDesc string
		if analysis.SQM != nil {
			bortle = astronomy.SQMToBortle(*analysis.SQM)
			bortleDesc = astronomy.BortleDescription(bortle)
		}

		return c.JSON(fiber.Map{
			"moon": fiber.Map{
				"illumination": math.Round(moonIllum*1000) / 10,
				"phase":        moonPhase,
			},
			"sun": sunTimes,
			"bortle": fiber.Map{
				"class":       bortle,
				"description": bortleDesc,
				"sqm":         analysis.SQM,
			},
		})
	})

	// Planet positions
	app.Get("/api/planets", func(c *fiber.Ctx) error {
		var loc struct {
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		}
		cfg.Get("location", &loc)
		if loc.Latitude == 0 && loc.Longitude == 0 {
			return c.JSON(fiber.Map{"error": "location not configured"})
		}

		planets := astronomy.PlanetPositions(time.Now(), loc.Latitude, loc.Longitude)
		return c.JSON(fiber.Map{"planets": planets})
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

	// Stale heartbeat monitoring
	go func() {
		for {
			time.Sleep(60 * time.Second)
			ctx := context.Background()
			services := []string{"ingest-camera", "processing", "detection", "policy", "alerts", "overlay", "export", "timelapse"}
			for _, svc := range services {
				status := novaskyRedis.GetServiceHealth(ctx, svc)
				if status == "unknown" {
					log.Printf("[api] WARNING: Service %s not reporting health", svc)
					novaskyRedis.PublishToStream(ctx, novaskyRedis.StreamAlertsDispatch, map[string]interface{}{
						"type": "service_down", "message": fmt.Sprintf("Service %s is not responding", svc),
					})
				}
			}
		}
	}()

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

