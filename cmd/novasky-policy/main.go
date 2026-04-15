package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/piwi3910/NovaSky/internal/config"
	"github.com/piwi3910/NovaSky/internal/db"
	"github.com/piwi3910/NovaSky/internal/models"
	novaskyRedis "github.com/piwi3910/NovaSky/internal/redis"
)

const consumerGroup = "policy"

var previousState string

// windGustTracker keeps a sliding window of wind gust readings for the last 10 minutes.
type windGustTracker struct {
	mu       sync.Mutex
	readings []gustReading
	window   time.Duration
}

type gustReading struct {
	value float64
	at    time.Time
}

func newWindGustTracker() *windGustTracker {
	return &windGustTracker{
		window: 10 * time.Minute,
	}
}

// add records a new wind gust reading.
func (w *windGustTracker) add(value float64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.readings = append(w.readings, gustReading{value: value, at: time.Now()})
	w.prune()
}

// maxGust returns the maximum gust in the sliding window.
func (w *windGustTracker) maxGust() float64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.prune()

	max := 0.0
	for _, r := range w.readings {
		if r.value > max {
			max = r.value
		}
	}
	return max
}

// prune removes readings older than the window. Must be called with lock held.
func (w *windGustTracker) prune() {
	cutoff := time.Now().Add(-w.window)
	i := 0
	for i < len(w.readings) && w.readings[i].at.Before(cutoff) {
		i++
	}
	if i > 0 {
		w.readings = w.readings[i:]
	}
}

func main() {
	log.Println("[policy] Starting...")
	db.Init()
	novaskyRedis.Init()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { c := make(chan os.Signal, 1); signal.Notify(c, syscall.SIGINT, syscall.SIGTERM); <-c; cancel() }()

	novaskyRedis.StartHealthReporter(ctx, "policy")

	cfg := config.NewManager()
	cfg.Subscribe(ctx)

	gustTracker := newWindGustTracker()

	// Load wind gust history from DB on startup (last 10 minutes)
	var recentGusts []models.SensorReading
	db.GetDB().Where("sensor_type = ? AND recorded_at > ?", "wind_gusts", time.Now().Add(-10*time.Minute)).
		Order("recorded_at ASC").Find(&recentGusts)
	for _, r := range recentGusts {
		gustTracker.mu.Lock()
		gustTracker.readings = append(gustTracker.readings, gustReading{value: r.Value, at: r.RecordedAt})
		gustTracker.mu.Unlock()
	}
	if len(recentGusts) > 0 {
		log.Printf("[policy] loaded %d recent wind gust readings", len(recentGusts))
	}

	novaskyRedis.CreateConsumerGroup(ctx, novaskyRedis.StreamPolicyEvaluate, consumerGroup)
	log.Println("[policy] Engine started")

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		streams, err := novaskyRedis.ReadFromGroup(ctx, novaskyRedis.StreamPolicyEvaluate, consumerGroup, "policy-1", 1)
		if err != nil {
			continue
		}

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				// Update gust tracker from latest DB readings
				var latestGust models.SensorReading
				if err := db.GetDB().Where("sensor_type = ?", "wind_gusts").
					Order("recorded_at DESC").First(&latestGust).Error; err == nil {
					gustTracker.add(latestGust.Value)
				}

				evaluate(ctx, cfg, gustTracker)
				novaskyRedis.AckMessage(ctx, novaskyRedis.StreamPolicyEvaluate, consumerGroup, msg.ID)
				novaskyRedis.ReportHealth(ctx, "policy")
			}
		}
	}
}

func evaluate(ctx context.Context, cfg *config.Manager, gustTracker *windGustTracker) {
	var analysis models.AnalysisResult
	db.GetDB().Order("analyzed_at DESC").First(&analysis)

	var rain models.SensorReading
	db.GetDB().Where("sensor_type = ?", "rain").Order("recorded_at DESC").First(&rain)

	// Read safety thresholds from config
	var safetyCfg struct {
		WindGustLimit float64 `json:"windGustLimit"`
	}
	cfg.Get("safety", &safetyCfg)
	if safetyCfg.WindGustLimit == 0 {
		safetyCfg.WindGustLimit = 50.0 // default 50 km/h
	}

	state := "UNKNOWN"
	quality := "UNUSABLE"
	var reason *string

	if rain.Value > 0 {
		state = "UNSAFE"
		r := "rain detected"
		reason = &r
	} else if maxGust := gustTracker.maxGust(); maxGust > safetyCfg.WindGustLimit {
		state = "UNSAFE"
		r := "wind gusts exceeded threshold"
		reason = &r
		log.Printf("[policy] wind gust %.1f km/h exceeds limit %.1f km/h", maxGust, safetyCfg.WindGustLimit)
	} else if analysis.ID == "" {
		state = "UNKNOWN"
		r := "no analysis data"
		reason = &r
	} else if analysis.CloudCover > 0.8 {
		state = "UNSAFE"
		r := "cloud cover exceeds threshold"
		reason = &r
	} else {
		state = "SAFE"
		quality = analysis.SkyQuality
	}

	saved := models.SafetyState{State: state, ImagingQuality: quality, Reason: reason}
	db.GetDB().Create(&saved)

	event, _ := json.Marshal(saved)
	novaskyRedis.Publish(ctx, novaskyRedis.ChannelSafetyState, string(event))

	if previousState != "" && previousState != state {
		novaskyRedis.PublishToStream(ctx, novaskyRedis.StreamAlertsDispatch, map[string]interface{}{
			"type": "safety_" + state, "message": "State changed to " + state,
		})
	}
	previousState = state

	log.Printf("[policy] %s quality=%s", state, quality)
}
