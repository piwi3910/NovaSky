package indi

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Client struct {
	conn       net.Conn
	mu         sync.RWMutex
	devices    map[string]bool
	properties map[string]map[string]float64 // device -> prop.element -> value
	blobCh     chan []byte
	deviceCh   chan string
	connected  bool
}

type CaptureResult struct {
	FilePath   string
	Data       []byte
	ExposureMs float64
	Gain       int
	MedianADU  float64
}

func NewClient() *Client {
	return &Client{
		devices:    make(map[string]bool),
		properties: make(map[string]map[string]float64),
		blobCh:     make(chan []byte, 1),
		deviceCh:   make(chan string, 1),
	}
}

func (c *Client) Connect(ctx context.Context, host string, port int) error {
	addr := fmt.Sprintf("%s:%d", host, port)
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("failed to connect to INDI server at %s: %w", addr, err)
	}
	c.conn = conn

	// Reset state for fresh connection
	c.mu.Lock()
	c.devices = make(map[string]bool)
	c.properties = make(map[string]map[string]float64)
	c.connected = false
	c.mu.Unlock()
	// Drain stale channels
	for len(c.blobCh) > 0 {
		<-c.blobCh
	}
	for len(c.deviceCh) > 0 {
		<-c.deviceCh
	}

	// Start reading in background
	go c.readLoop()

	// Request all properties
	c.send(`<getProperties version="1.7"/>`)
	c.send(`<enableBLOB device="">Also</enableBLOB>`)

	// Wait for first device
	select {
	case deviceName := <-c.deviceCh:
		log.Printf("[indi] Device discovered: %s", deviceName)
	case <-time.After(15 * time.Second):
		return fmt.Errorf("no INDI devices found within 15s")
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

func (c *Client) ConnectDevice(device string) error {
	log.Printf("[indi] Connecting to device: %s", device)
	c.setSwitch(device, "CONNECTION", "CONNECT", "On")
	c.send(fmt.Sprintf(`<enableBLOB device="%s">Also</enableBLOB>`, device))
	time.Sleep(3 * time.Second)

	// Set 16-bit capture format
	c.setSwitch(device, "CCD_CAPTURE_FORMAT", "ASI_IMG_RAW16", "On")
	time.Sleep(500 * time.Millisecond)

	c.mu.Lock()
	c.connected = true
	c.mu.Unlock()

	log.Printf("[indi] Device %s connected (16-bit mode)", device)
	return nil
}

func (c *Client) GetDevices() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	devices := make([]string, 0, len(c.devices))
	for d := range c.devices {
		devices = append(devices, d)
	}
	return devices
}

func (c *Client) GetNumber(device, prop, element string) (float64, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	key := prop + "." + element
	if devProps, ok := c.properties[device]; ok {
		if val, ok := devProps[key]; ok {
			return val, true
		}
	}
	return 0, false
}

func (c *Client) SetNumber(device, prop, element string, value float64) {
	c.send(fmt.Sprintf(
		`<newNumberVector device="%s" name="%s"><oneNumber name="%s">%f</oneNumber></newNumberVector>`,
		device, prop, element, value,
	))
}

func (c *Client) SetGain(device string, gain int) {
	c.SetNumber(device, "CCD_CONTROLS", "Gain", float64(gain))
}

func (c *Client) Capture(device string, exposureSec float64, timeout time.Duration) ([]byte, error) {
	// Clear any previous blob
	select {
	case <-c.blobCh:
	default:
	}

	// Set exposure (triggers capture)
	c.SetNumber(device, "CCD_EXPOSURE", "CCD_EXPOSURE_VALUE", exposureSec)

	// Wait for BLOB
	select {
	case data := <-c.blobCh:
		return data, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("capture timed out after %s", timeout)
	}
}

func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

func (c *Client) Close() error {
	c.mu.Lock()
	c.connected = false
	c.mu.Unlock()
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *Client) send(xmlStr string) {
	if c.conn != nil {
		c.conn.Write([]byte(xmlStr)) //nolint:errcheck
	}
}

func (c *Client) setSwitch(device, prop, element, state string) {
	c.send(fmt.Sprintf(
		`<newSwitchVector device="%s" name="%s"><oneSwitch name="%s">%s</oneSwitch></newSwitchVector>`,
		device, prop, element, state,
	))
}

func (c *Client) readLoop() {
	buf := make([]byte, 1024*1024) // 1MB read buffer
	var accumulated []byte
	inBlob := false
	var blobBuf []byte

	for {
		n, err := c.conn.Read(buf)
		if err != nil {
			if err != io.EOF {
				log.Printf("[indi] Read error: %v", err)
			}
			return
		}

		data := buf[:n]

		if inBlob {
			blobBuf = append(blobBuf, data...)
			endMarker := []byte("</setBLOBVector>")
			if idx := indexOf(blobBuf, endMarker); idx >= 0 {
				blobSection := blobBuf[:idx]
				remaining := blobBuf[idx+len(endMarker):]
				inBlob = false
				blobBuf = nil
				c.extractBlob(blobSection)
				if len(remaining) > 0 {
					accumulated = append(accumulated, remaining...)
					c.processBuffer(&accumulated)
				}
			}
			continue
		}

		accumulated = append(accumulated, data...)

		// Check for BLOB start
		blobStart := indexOf(accumulated, []byte("<setBLOBVector"))
		if blobStart >= 0 {
			// Process everything before the BLOB
			if blobStart > 0 {
				before := accumulated[:blobStart]
				c.processXML(before)
			}
			blobPart := accumulated[blobStart:]
			accumulated = nil

			endMarker := []byte("</setBLOBVector>")
			if idx := indexOf(blobPart, endMarker); idx >= 0 {
				blobSection := blobPart[len("<setBLOBVector"):idx]
				remaining := blobPart[idx+len(endMarker):]
				c.extractBlob(blobSection)
				if len(remaining) > 0 {
					accumulated = remaining
				}
			} else {
				inBlob = true
				blobBuf = blobPart
			}
			continue
		}

		c.processBuffer(&accumulated)
	}
}

func (c *Client) processBuffer(buf *[]byte) {
	// Try to extract complete XML elements
	text := string(*buf)
	processed := 0

	for {
		start := strings.Index(text[processed:], "<")
		if start == -1 {
			break
		}
		start += processed

		if strings.HasPrefix(text[start:], "<setBLOBVector") {
			break
		}

		// Find tag name
		re := regexp.MustCompile(`<(\w+)`)
		match := re.FindStringSubmatch(text[start:])
		if match == nil {
			processed = start + 1
			continue
		}
		tagName := match[1]

		// Self-closing?
		closeAngle := strings.Index(text[start:], ">")
		if closeAngle == -1 {
			break
		}
		if text[start+closeAngle-1] == '/' {
			processed = start + closeAngle + 1
			continue
		}

		// Find closing tag
		closing := fmt.Sprintf("</%s>", tagName)
		closePos := strings.Index(text[start+closeAngle:], closing)
		if closePos == -1 {
			break
		}
		closePos += start + closeAngle

		element := text[start : closePos+len(closing)]
		processed = closePos + len(closing)

		c.handleElement(element)
	}

	*buf = []byte(text[processed:])
}

func (c *Client) processXML(data []byte) {
	buf := data
	c.processBuffer(&buf)
}

func (c *Client) handleElement(xmlText string) {
	// Device discovery
	deviceRe := regexp.MustCompile(`device="([^"]+)"`)
	deviceMatch := deviceRe.FindStringSubmatch(xmlText)
	if deviceMatch == nil {
		return
	}
	device := deviceMatch[1]

	if strings.HasPrefix(xmlText, "<def") {
		c.mu.Lock()
		if !c.devices[device] {
			c.devices[device] = true
			c.mu.Unlock()
			select {
			case c.deviceCh <- device:
			default:
			}
		} else {
			c.mu.Unlock()
		}
	}

	// Track number properties
	if strings.Contains(xmlText, "NumberVector") {
		nameRe := regexp.MustCompile(`name="([^"]+)"`)
		nameMatch := nameRe.FindStringSubmatch(xmlText)
		if nameMatch == nil {
			return
		}
		propName := nameMatch[1]

		numRe := regexp.MustCompile(`<(?:defNumber|oneNumber)\s+name="([^"]+)"[^>]*>\s*([-\d.eE+]+)`)
		for _, numMatch := range numRe.FindAllStringSubmatch(xmlText, -1) {
			elemName := numMatch[1]
			val, err := strconv.ParseFloat(strings.TrimSpace(numMatch[2]), 64)
			if err == nil {
				c.mu.Lock()
				if c.properties[device] == nil {
					c.properties[device] = make(map[string]float64)
				}
				c.properties[device][propName+"."+elemName] = val
				c.mu.Unlock()
			}
		}
	}
}

func (c *Client) extractBlob(blobXML []byte) {
	text := string(blobXML)
	// Find base64 data between <oneBLOB ...> and </oneBLOB>
	startTag := strings.Index(text, "<oneBLOB")
	if startTag == -1 {
		return
	}
	startData := strings.Index(text[startTag:], ">")
	if startData == -1 {
		return
	}
	startData += startTag + 1

	endData := strings.Index(text[startData:], "</oneBLOB>")
	if endData == -1 {
		return
	}
	endData += startData

	b64Data := strings.TrimSpace(text[startData:endData])
	decoded, err := base64.StdEncoding.DecodeString(b64Data)
	if err != nil {
		log.Printf("[indi] Failed to decode BLOB: %v", err)
		return
	}

	log.Printf("[indi] BLOB received: %d bytes", len(decoded))

	select {
	case c.blobCh <- decoded:
	default:
		log.Println("[indi] Warning: BLOB channel full, dropping")
	}
}

func indexOf(data, sep []byte) int {
	for i := 0; i <= len(data)-len(sep); i++ {
		found := true
		for j := 0; j < len(sep); j++ {
			if data[i+j] != sep[j] {
				found = false
				break
			}
		}
		if found {
			return i
		}
	}
	return -1
}

