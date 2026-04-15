package redis

import (
	"context"
	"log"
	"os"

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

// GetStreamLength returns the number of pending messages in a stream
func GetStreamLength(ctx context.Context, stream string) (int64, error) {
	return Client.XLen(ctx, stream).Result()
}

// CreateConsumerGroup creates a consumer group for a stream (idempotent)
func CreateConsumerGroup(ctx context.Context, stream, group string) error {
	err := Client.XGroupCreateMkStream(ctx, stream, group, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return err
	}
	return nil
}

// ReadFromGroup reads messages from a consumer group
func ReadFromGroup(ctx context.Context, stream, group, consumer string, count int64) ([]redis.XStream, error) {
	return Client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    group,
		Consumer: consumer,
		Streams:  []string{stream, ">"},
		Count:    count,
		Block:    0,
	}).Result()
}

// AckMessage acknowledges a message in a consumer group
func AckMessage(ctx context.Context, stream, group string, ids ...string) error {
	return Client.XAck(ctx, stream, group, ids...).Err()
}
