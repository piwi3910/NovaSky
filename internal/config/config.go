package config

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/piwi3910/NovaSky/internal/db"
	"github.com/piwi3910/NovaSky/internal/models"
	novaskyRedis "github.com/piwi3910/NovaSky/internal/redis"
)

// ErrNotFound is returned by Get when the requested config key does not exist.
var ErrNotFound = fmt.Errorf("config key not found")

type Manager struct {
	mu       sync.RWMutex
	values   map[string]json.RawMessage
	onChange func(key string)
}

func NewManager() *Manager {
	m := &Manager{
		values: make(map[string]json.RawMessage),
	}
	m.loadAll()
	return m
}

func (m *Manager) loadAll() {
	var configs []models.Config
	db.GetDB().Find(&configs)

	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range configs {
		m.values[c.Key] = json.RawMessage(c.Value)
	}
}

func (m *Manager) Get(key string, dest any) error {
	m.mu.RLock()
	raw, ok := m.values[key]
	m.mu.RUnlock()

	if !ok {
		return ErrNotFound
	}
	return json.Unmarshal(raw, dest)
}

func (m *Manager) GetRaw(key string) json.RawMessage {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.values[key]
}

func (m *Manager) GetAll() map[string]json.RawMessage {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]json.RawMessage, len(m.values))
	for k, v := range m.values {
		result[k] = v
	}
	return result
}

func (m *Manager) Set(key string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	config := models.Config{Key: key, Value: data}
	result := db.GetDB().Save(&config)
	if result.Error != nil {
		return result.Error
	}

	m.mu.Lock()
	m.values[key] = data
	m.mu.Unlock()

	// Broadcast change
	ctx := context.Background()
	msg, _ := json.Marshal(map[string]string{"key": key})
	novaskyRedis.Publish(ctx, novaskyRedis.ChannelConfigChanged, string(msg))

	return nil
}

func (m *Manager) OnChange(fn func(key string)) {
	m.onChange = fn
}

// Subscribe listens for config changes and reloads
func (m *Manager) Subscribe(ctx context.Context) {
	sub := novaskyRedis.Client.Subscribe(ctx, novaskyRedis.ChannelConfigChanged)
	ch := sub.Channel()

	go func() {
		for msg := range ch {
			var payload struct {
				Key string `json:"key"`
			}
			if err := json.Unmarshal([]byte(msg.Payload), &payload); err != nil {
				continue
			}

			// Reload this key from DB
			var config models.Config
			if err := db.GetDB().First(&config, "key = ?", payload.Key).Error; err != nil {
				continue
			}

			m.mu.Lock()
			m.values[payload.Key] = json.RawMessage(config.Value)
			m.mu.Unlock()

			if m.onChange != nil {
				m.onChange(payload.Key)
			}

			log.Printf("[config] reloaded key: %s", payload.Key)
		}
	}()
}
