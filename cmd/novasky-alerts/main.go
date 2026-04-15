package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/piwi3910/NovaSky/internal/db"
	"github.com/piwi3910/NovaSky/internal/models"
	novaskyRedis "github.com/piwi3910/NovaSky/internal/redis"
)

const consumerGroup = "alerts"

func main() {
	log.Println("[alerts] Starting...")
	db.Init()
	novaskyRedis.Init()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { c := make(chan os.Signal, 1); signal.Notify(c, syscall.SIGINT, syscall.SIGTERM); <-c; cancel() }()

	novaskyRedis.CreateConsumerGroup(ctx, novaskyRedis.StreamAlertsDispatch, consumerGroup)
	log.Println("[alerts] Worker started")

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		streams, err := novaskyRedis.ReadFromGroup(ctx, novaskyRedis.StreamAlertsDispatch, consumerGroup, "alerts-1", 1)
		if err != nil {
			continue
		}

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				alertType := msg.Values["type"].(string)
				message := msg.Values["message"].(string)

				alert := models.Alert{Type: alertType, Message: message}
				db.GetDB().Create(&alert)
				log.Printf("[alerts] %s: %s", alertType, message)

				// TODO: webhook, telegram, email dispatch
				novaskyRedis.AckMessage(ctx, novaskyRedis.StreamAlertsDispatch, consumerGroup, msg.ID)
			}
		}
	}
}
