package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/piwi3910/NovaSky/internal/db"
	"github.com/piwi3910/NovaSky/internal/models"
	novaskyRedis "github.com/piwi3910/NovaSky/internal/redis"
)

const consumerGroup = "policy"

var previousState string

func main() {
	log.Println("[policy] Starting...")
	db.Init()
	novaskyRedis.Init()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { c := make(chan os.Signal, 1); signal.Notify(c, syscall.SIGINT, syscall.SIGTERM); <-c; cancel() }()

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
				evaluate(ctx)
				novaskyRedis.AckMessage(ctx, novaskyRedis.StreamPolicyEvaluate, consumerGroup, msg.ID)
			}
		}
	}
}

func evaluate(ctx context.Context) {
	var analysis models.AnalysisResult
	db.GetDB().Order("analyzed_at DESC").First(&analysis)

	var rain models.SensorReading
	db.GetDB().Where("sensor_type = ?", "rain").Order("recorded_at DESC").First(&rain)

	state := "UNKNOWN"
	quality := "UNUSABLE"
	var reason *string

	if rain.Value > 0 {
		state = "UNSAFE"
		r := "rain detected"
		reason = &r
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
