package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

const consumerGroup = "alerts"

func main() {
	log.Println("[alerts] Starting...")
	db.Init()
	novaskyRedis.Init()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { c := make(chan os.Signal, 1); signal.Notify(c, syscall.SIGINT, syscall.SIGTERM); <-c; cancel() }()

	novaskyRedis.StartHealthReporter(ctx, "alerts")

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

				// Dispatch webhook if configured
				webhookURL := os.Getenv("ALERT_WEBHOOK_URL")
				if webhookURL != "" {
					payload, _ := json.Marshal(map[string]string{
						"type": alertType, "message": message,
						"timestamp": time.Now().Format(time.RFC3339),
					})
					go func() {
						resp, err := http.Post(webhookURL, "application/json", bytes.NewReader(payload))
						if err != nil {
							log.Printf("[alerts] Webhook failed: %v", err)
							return
						}
						resp.Body.Close()
						log.Printf("[alerts] Webhook sent to %s: %d", webhookURL, resp.StatusCode)
					}()
				}
				// Dispatch Telegram if configured
				telegramToken := os.Getenv("TELEGRAM_BOT_TOKEN")
				telegramChatID := os.Getenv("TELEGRAM_CHAT_ID")
				if telegramToken != "" && telegramChatID != "" {
					go func() {
						text := fmt.Sprintf("🔭 NovaSky: %s\n%s", alertType, message)
						url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", telegramToken)
						payload, _ := json.Marshal(map[string]string{
							"chat_id": telegramChatID,
							"text":    text,
						})
						resp, err := http.Post(url, "application/json", bytes.NewReader(payload))
						if err != nil {
							log.Printf("[alerts] Telegram failed: %v", err)
							return
						}
						resp.Body.Close()
						log.Printf("[alerts] Telegram sent to chat %s", telegramChatID)
					}()
				}
				novaskyRedis.AckMessage(ctx, novaskyRedis.StreamAlertsDispatch, consumerGroup, msg.ID)
				novaskyRedis.ReportHealth(ctx, "alerts")
			}
		}
	}
}
