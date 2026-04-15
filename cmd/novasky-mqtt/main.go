package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/piwi3910/NovaSky/internal/config"
	"github.com/piwi3910/NovaSky/internal/db"
	"github.com/piwi3910/NovaSky/internal/models"
	novaskyRedis "github.com/piwi3910/NovaSky/internal/redis"
)

var mqttConn net.Conn

func main() {
	log.Println("[mqtt] Starting...")
	db.Init()
	novaskyRedis.Init()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { c := make(chan os.Signal, 1); signal.Notify(c, syscall.SIGINT, syscall.SIGTERM); <-c; cancel() }()
	novaskyRedis.StartHealthReporter(ctx, "mqtt")

	cfg := config.NewManager()
	cfg.Subscribe(ctx)

	var mqttCfg struct {
		Broker   string `json:"broker"`
		Username string `json:"username"`
		Password string `json:"password"`
		Enabled  bool   `json:"enabled"`
	}
	cfg.Get("mqtt", &mqttCfg)

	cfg.OnChange(func(key string) {
		if key == "mqtt" {
			cfg.Get("mqtt", &mqttCfg)
			if mqttCfg.Enabled && mqttCfg.Broker != "" {
				connectMQTT(mqttCfg.Broker, mqttCfg.Username, mqttCfg.Password)
			}
		}
	})

	if mqttCfg.Enabled && mqttCfg.Broker != "" {
		connectMQTT(mqttCfg.Broker, mqttCfg.Username, mqttCfg.Password)
	}

	// Publish HA auto-discovery configs on startup
	go publishHADiscovery()

	// Subscribe to safety state changes
	sub := novaskyRedis.Client.Subscribe(ctx, novaskyRedis.ChannelSafetyState)
	ch := sub.Channel()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	log.Println("[mqtt] Service ready")

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			if mqttConn == nil {
				continue
			}
			var safety models.SafetyState
			json.Unmarshal([]byte(msg.Payload), &safety)
			state := "OFF"
			if safety.State == "SAFE" {
				state = "ON"
			}
			mqttPublish("novasky/safety/state", state)
			mqttPublish("novasky/safety/quality", safety.ImagingQuality)
		case <-ticker.C:
			if mqttConn == nil {
				continue
			}
			var readings []models.SensorReading
			db.GetDB().Raw("SELECT DISTINCT ON (sensor_type) * FROM sensor_readings ORDER BY sensor_type, recorded_at DESC").Scan(&readings)
			for _, r := range readings {
				mqttPublish(fmt.Sprintf("novasky/sensor/%s", r.SensorType), fmt.Sprintf("%.1f", r.Value))
			}
			// Publish camera status
			var frame models.Frame
			db.GetDB().Order("created_at DESC").First(&frame)
			if frame.ID != "" {
				mqttPublish("novasky/camera/exposure", fmt.Sprintf("%.3f", frame.ExposureMs))
				mqttPublish("novasky/camera/gain", fmt.Sprintf("%d", frame.Gain))
			}
		}
	}
}

func connectMQTT(broker, username, password string) {
	var err error
	mqttConn, err = net.DialTimeout("tcp", broker, 5*time.Second)
	if err != nil {
		log.Printf("[mqtt] Failed to connect to %s: %v", broker, err)
		mqttConn = nil
		return
	}

	// Send MQTT CONNECT packet
	clientID := "novasky"
	packet := buildConnectPacket(clientID, username, password)
	mqttConn.Write(packet)

	// Read CONNACK
	buf := make([]byte, 4)
	mqttConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	mqttConn.Read(buf)
	mqttConn.SetReadDeadline(time.Time{})

	log.Printf("[mqtt] Connected to broker %s", broker)
}

func mqttPublish(topic, payload string) {
	if mqttConn == nil {
		return
	}

	// MQTT PUBLISH packet (QoS 0)
	topicBytes := []byte(topic)
	payloadBytes := []byte(payload)

	remainLen := 2 + len(topicBytes) + len(payloadBytes)
	packet := make([]byte, 0, 2+remainLen)
	packet = append(packet, 0x30) // PUBLISH, QoS 0
	packet = append(packet, encodeRemainingLength(remainLen)...)
	// Topic length
	packet = append(packet, byte(len(topicBytes)>>8), byte(len(topicBytes)))
	packet = append(packet, topicBytes...)
	packet = append(packet, payloadBytes...)

	_, err := mqttConn.Write(packet)
	if err != nil {
		log.Printf("[mqtt] Publish failed: %v", err)
		mqttConn = nil // reconnect on next cycle
	}
}

func buildConnectPacket(clientID, username, password string) []byte {
	// Variable header
	varHeader := []byte{
		0x00, 0x04, 'M', 'Q', 'T', 'T', // Protocol name
		0x04,       // Protocol level (MQTT 3.1.1)
		0x02,       // Connect flags (clean session)
		0x00, 0x3C, // Keep alive (60s)
	}

	flags := byte(0x02) // clean session
	if username != "" {
		flags |= 0x80
	}
	if password != "" {
		flags |= 0x40
	}
	varHeader[7] = flags

	// Payload
	payload := encodeString(clientID)
	if username != "" {
		payload = append(payload, encodeString(username)...)
	}
	if password != "" {
		payload = append(payload, encodeString(password)...)
	}

	remainLen := len(varHeader) + len(payload)
	packet := []byte{0x10} // CONNECT
	packet = append(packet, encodeRemainingLength(remainLen)...)
	packet = append(packet, varHeader...)
	packet = append(packet, payload...)
	return packet
}

func encodeString(s string) []byte {
	b := make([]byte, 2+len(s))
	binary.BigEndian.PutUint16(b, uint16(len(s)))
	copy(b[2:], s)
	return b
}

func encodeRemainingLength(length int) []byte {
	var encoded []byte
	for {
		b := byte(length % 128)
		length /= 128
		if length > 0 {
			b |= 0x80
		}
		encoded = append(encoded, b)
		if length == 0 {
			break
		}
	}
	return encoded
}

func publishHADiscovery() {
	time.Sleep(5 * time.Second) // wait for connection
	if mqttConn == nil {
		return
	}

	// Safety binary sensor
	disc, _ := json.Marshal(map[string]interface{}{
		"name": "NovaSky Safety", "unique_id": "novasky_safety",
		"state_topic": "novasky/safety/state", "device_class": "safety",
		"payload_on": "ON", "payload_off": "OFF",
		"device": map[string]interface{}{"identifiers": []string{"novasky"}, "name": "NovaSky", "manufacturer": "NovaSky"},
	})
	mqttPublish("homeassistant/binary_sensor/novasky_safety/config", string(disc))

	// Temperature sensor
	disc, _ = json.Marshal(map[string]interface{}{
		"name": "NovaSky Temperature", "unique_id": "novasky_temperature",
		"state_topic": "novasky/sensor/temperature", "unit_of_measurement": "°C",
		"device_class": "temperature",
		"device": map[string]interface{}{"identifiers": []string{"novasky"}, "name": "NovaSky"},
	})
	mqttPublish("homeassistant/sensor/novasky_temperature/config", string(disc))

	log.Println("[mqtt] HA auto-discovery published")
}
