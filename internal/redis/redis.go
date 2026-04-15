package redis

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

var Client *redis.Client

func Init() {
	addr := os.Getenv("REDIS_URL")
	if addr == "" {
		addr = "localhost:6379"
	}

	Client = redis.NewClient(&redis.Options{
		Addr: addr,
	})

	ctx := context.Background()
	if err := Client.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
}

// Stream names
const (
	StreamFramesRaw        = "frames.raw"
	StreamFramesProcessing = "frames.processing"
	StreamFramesDetection  = "frames.detection"
	StreamFramesOverlay    = "frames.overlay"
	StreamFramesExport     = "frames.export"
	StreamFramesTimelapse  = "frames.timelapse"
	StreamPolicyEvaluate   = "policy.evaluate"
	StreamAlertsDispatch   = "alerts.dispatch"
)

// Pub/sub channels
const (
	ChannelSafetyState    = "novasky:safety-state"
	ChannelFrameNew       = "novasky:frame-new"
	ChannelFrameProcessed = "novasky:frame-processed"
	ChannelConfigChanged  = "novasky:config-changed"
	ChannelAutoExposure   = "novasky:autoexposure-state"
	ChannelBackpressure   = "novasky:backpressure"
)

// PublishToStream adds a message to a Redis Stream
func PublishToStream(ctx context.Context, stream string, values map[string]interface{}) (string, error) {
	return Client.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: values,
	}).Result()
}

// Publish sends a message to a pub/sub channel
func Publish(ctx context.Context, channel string, message interface{}) error {
	return Client.Publish(ctx, channel, message).Err()
}

// GetStreamLength returns the total number of messages in a stream
func GetStreamLength(ctx context.Context, stream string) (int64, error) {
	return Client.XLen(ctx, stream).Result()
}

// GetPendingCount returns the number of unacknowledged messages for a consumer group
func GetPendingCount(ctx context.Context, stream, group string) (int64, error) {
	pending, err := Client.XPending(ctx, stream, group).Result()
	if err != nil {
		return 0, err
	}
	return pending.Count, nil
}

// ReportHealth writes a service heartbeat to Redis (lightweight, no DB)
func ReportHealth(ctx context.Context, service string) {
	Client.Set(ctx, "novasky:health:"+service, "running", 60*time.Second)
}

// StartHealthReporter starts a background goroutine that reports health every 10 seconds
func StartHealthReporter(ctx context.Context, service string) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				ReportHealth(ctx, service)
				time.Sleep(10 * time.Second)
			}
		}
	}()
}

// GetHealth checks if a service is alive
func GetServiceHealth(ctx context.Context, service string) string {
	val, err := Client.Get(ctx, "novasky:health:"+service).Result()
	if err != nil {
		return "unknown"
	}
	return val
}

// CreateConsumerGroup creates a consumer group for a stream (idempotent)
func CreateConsumerGroup(ctx context.Context, stream, group string) error {
	err := Client.XGroupCreateMkStream(ctx, stream, group, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return err
	}
	return nil
}

// ReadFromGroup reads messages from a consumer group.
// Auto-creates the group if it doesn't exist (handles NOGROUP error).
func ReadFromGroup(ctx context.Context, stream, group, consumer string, count int64) ([]redis.XStream, error) {
	result, err := Client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    group,
		Consumer: consumer,
		Streams:  []string{stream, ">"},
		Count:    count,
		Block:    5 * time.Second, // block for 5s instead of forever (allows NOGROUP recovery)
	}).Result()

	if err != nil && (err.Error() == "NOGROUP No such key '"+stream+"' or consumer group '"+group+"' in XREADGROUP with GROUP option" ||
		strings.Contains(err.Error(), "NOGROUP")) {
		// Stream or group doesn't exist yet — create it and retry
		CreateConsumerGroup(ctx, stream, group)
		time.Sleep(1 * time.Second)
		return nil, nil // return empty, caller will retry
	}

	if err == redis.Nil {
		return nil, nil // timeout, no messages
	}

	return result, err
}

// AckMessage acknowledges a message in a consumer group
func AckMessage(ctx context.Context, stream, group string, ids ...string) error {
	return Client.XAck(ctx, stream, group, ids...).Err()
}
