package detection

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Satellite represents a predicted satellite pass
type Satellite struct {
	Name      string  `json:"name"`
	NoradID   int     `json:"noradId"`
	RA        float64 `json:"ra"`
	Dec       float64 `json:"dec"`
	Altitude  float64 `json:"altitude"` // degrees above horizon
	Azimuth   float64 `json:"azimuth"`
	Magnitude float64 `json:"magnitude"`
}

// TLECache holds cached TLE data
type TLECache struct {
	mu        sync.RWMutex
	tles      []TLEEntry
	lastFetch time.Time
}

// TLEEntry holds a single TLE record
type TLEEntry struct {
	Name  string
	Line1 string
	Line2 string
}

var tleCache = &TLECache{}

// FetchTLEs downloads TLE data from CelesTrak
func FetchTLEs() error {
	tleCache.mu.Lock()
	defer tleCache.mu.Unlock()

	// Only fetch every 24 hours
	if time.Since(tleCache.lastFetch) < 24*time.Hour && len(tleCache.tles) > 0 {
		return nil
	}

	resp, err := http.Get("https://celestrak.org/NORAD/elements/gp.php?GROUP=visual&FORMAT=tle")
	if err != nil {
		return fmt.Errorf("failed to fetch TLE: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	var entries []TLEEntry
	for i := 0; i+2 < len(lines); i += 3 {
		entries = append(entries, TLEEntry{
			Name:  strings.TrimSpace(lines[i]),
			Line1: strings.TrimSpace(lines[i+1]),
			Line2: strings.TrimSpace(lines[i+2]),
		})
	}

	tleCache.tles = entries
	tleCache.lastFetch = time.Now()
	log.Printf("[detection] Fetched %d TLE entries from CelesTrak", len(entries))
	return nil
}

// GetVisibleSatellites returns satellites currently above the horizon.
// Full SGP4 propagation would be needed for accurate predictions.
func GetVisibleSatellites(lat, lon float64, t time.Time) []Satellite {
	tleCache.mu.RLock()
	defer tleCache.mu.RUnlock()

	// Placeholder — full SGP4 implementation needed for real predictions
	var sats []Satellite
	for _, tle := range tleCache.tles {
		_ = tle // SGP4 propagation would go here
	}
	return sats
}

// Plane represents a detected aircraft from ADS-B
type Plane struct {
	Callsign string  `json:"callsign"`
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
	Altitude float64 `json:"altitude"` // feet
	Speed    float64 `json:"speed"`    // knots
	Heading  float64 `json:"heading"`
}

// FetchPlanes queries a tar1090 ADS-B API for nearby aircraft
func FetchPlanes(tar1090URL string) ([]Plane, error) {
	if tar1090URL == "" {
		return nil, nil
	}

	url := tar1090URL + "/data/aircraft.json"
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data struct {
		Aircraft []struct {
			Hex    string  `json:"hex"`
			Flight string  `json:"flight"`
			Lat    float64 `json:"lat"`
			Lon    float64 `json:"lon"`
			AltBaro float64 `json:"alt_baro"`
			GS     float64 `json:"gs"`
			Track  float64 `json:"track"`
		} `json:"aircraft"`
	}
	json.NewDecoder(resp.Body).Decode(&data) //nolint:errcheck

	var planes []Plane
	for _, ac := range data.Aircraft {
		if ac.Lat == 0 && ac.Lon == 0 {
			continue
		}
		planes = append(planes, Plane{
			Callsign: strings.TrimSpace(ac.Flight),
			Lat:      ac.Lat,
			Lon:      ac.Lon,
			Altitude: ac.AltBaro,
			Speed:    ac.GS,
			Heading:  ac.Track,
		})
	}
	return planes, nil
}

var _ = math.Pi // keep math import
