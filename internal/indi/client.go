package indi

import (
	"bytes"
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

// Package-level compiled regexps (Fix #4, #16)
var (
	reTagName   = regexp.MustCompile(`<(\w+)`)
	reDevice    = regexp.MustCompile(`device="([^"]+)"`)
	reName      = regexp.MustCompile(`name="([^"]+)"`)
	reNumberVal = regexp.MustCompile(`<(?:defNumber|oneNumber)\s+name="([^"]+)"[^>]*>\s*([-\d.eE+]+)`)
)

type Client struct {
	conn       net.Conn
	mu         sync.RWMutex
	devices    map[string]bool
	properties map[string]map[string]float64 // device -> prop.element -> value
	blobCh     chan []byte
	deviceCh   chan string
	connected  bool
	done       chan struct{}
}

func NewClient() *Client {
	return &Client{
		devices:    make(map[string]bool),
		properties: make(map[string]map[string]float64),
		blobCh:     make(chan []byte, 1),
		deviceCh:   make(chan string, 1),
		done:       make(chan struct{}),
	}
}

// xmlEscape escapes XML special characters in strings interpolated into XML messages.
func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
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

	// Close previous done channel and create a fresh one
	select {
	case <-c.done:
		// already closed
	default:
		close(c.done)
	}
	c.done = make(chan struct{})

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
	c.send(fmt.Sprintf(`<enableBLOB device="%s">Also</enableBLOB>`, xmlEscape(device)))
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
		xmlEscape(device), xmlEscape(prop), xmlEscape(element), value,
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

	// Signal readLoop to stop
	select {
	case <-c.done:
		// already closed
	default:
		close(c.done)
	}

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
		xmlEscape(device), xmlEscape(prop), xmlEscape(element), xmlEscape(state),
	))
}

func (c *Client) readLoop() {
	buf := make([]byte, 1024*1024) // 1MB read buffer
	var accumulated []byte
	inBlob := false
	var blobBuf []byte

	for {
		// Check if we should stop
		select {
		case <-c.done:
			return
		default:
		}

		// Set a read deadline so we can periodically check the done channel
		c.conn.SetReadDeadline(time.Now().Add(1 * time.Second)) //nolint:errcheck
		n, err := c.conn.Read(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue // deadline exceeded, loop back to check done
			}
			if err != io.EOF {
				log.Printf("[indi] Read error: %v", err)
			}
			return
		}

		data := buf[:n]

		if inBlob {
			blobBuf = append(blobBuf, data...)
			endMarker := []byte("</setBLOBVector>")
			if idx := bytes.Index(blobBuf, endMarker); idx >= 0 {
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
		blobStart := bytes.Index(accumulated, []byte("<setBLOBVector"))
		if blobStart >= 0 {
			// Process everything before the BLOB
			if blobStart > 0 {
				before := accumulated[:blobStart]
				c.processXML(before)
			}
			blobPart := accumulated[blobStart:]
			accumulated = nil

			endMarker := []byte("</setBLOBVector>")
			if idx := bytes.Index(blobPart, endMarker); idx >= 0 {
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
		match := reTagName.FindStringSubmatch(text[start:])
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
	deviceMatch := reDevice.FindStringSubmatch(xmlText)
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
		nameMatch := reName.FindStringSubmatch(xmlText)
		if nameMatch == nil {
			return
		}
		propName := nameMatch[1]

		for _, numMatch := range reNumberVal.FindAllStringSubmatch(xmlText, -1) {
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
