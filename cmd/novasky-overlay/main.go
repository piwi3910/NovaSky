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

const consumerGroup = "overlay"

func main() {
	log.Println("[overlay] Starting...")
	db.Init()
	novaskyRedis.Init()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { c := make(chan os.Signal, 1); signal.Notify(c, syscall.SIGINT, syscall.SIGTERM); <-c; cancel() }()

	novaskyRedis.CreateConsumerGroup(ctx, novaskyRedis.StreamFramesOverlay, consumerGroup)
	log.Println("[overlay] Worker started (stub — pass-through)")

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		streams, err := novaskyRedis.ReadFromGroup(ctx, novaskyRedis.StreamFramesOverlay, consumerGroup, "overlay-1", 1)
		if err != nil {
			continue
		}

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				frameID := msg.Values["frameId"].(string)
				log.Printf("[overlay] Frame %s received (stub)", frameID)
				novaskyRedis.AckMessage(ctx, novaskyRedis.StreamFramesOverlay, consumerGroup, msg.ID)
			}
		}
	}
}
