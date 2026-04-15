package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/piwi3910/NovaSky/internal/db"
	novaskyRedis "github.com/piwi3910/NovaSky/internal/redis"
)

const consumerGroup = "timelapse"

func main() {
	log.Println("[timelapse] Starting...")
	db.Init()
	novaskyRedis.Init()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { c := make(chan os.Signal, 1); signal.Notify(c, syscall.SIGINT, syscall.SIGTERM); <-c; cancel() }()

	novaskyRedis.CreateConsumerGroup(ctx, novaskyRedis.StreamFramesTimelapse, consumerGroup)
	log.Println("[timelapse] Worker started (stub — collecting frames)")

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		streams, err := novaskyRedis.ReadFromGroup(ctx, novaskyRedis.StreamFramesTimelapse, consumerGroup, "timelapse-1", 1)
		if err != nil {
			continue
		}

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				frameID := msg.Values["frameId"].(string)
				log.Printf("[timelapse] Frame %s collected", frameID)
				// TODO: accumulate frames, generate timelapse at dawn
				novaskyRedis.AckMessage(ctx, novaskyRedis.StreamFramesTimelapse, consumerGroup, msg.ID)
			}
		}
	}
}
