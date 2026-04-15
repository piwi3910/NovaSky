package config

import (
	"encoding/json"

	"github.com/piwi3910/NovaSky/internal/db"
	"github.com/piwi3910/NovaSky/internal/models"
)

type ImagingProfile struct {
	Gain          int     `json:"gain"`
	MinExposureMs float64 `json:"minExposureMs"`
	MaxExposureMs float64 `json:"maxExposureMs"`
	ADUTarget     float64 `json:"aduTarget"`
	Stretch       string  `json:"stretch"`
}

type LocationConfig struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Elevation float64 `json:"elevation"`
}

var defaults = map[string]interface{}{
	"camera.driver":    "indi_asi_ccd",
	"camera.device":    "",
	"imaging.day":      ImagingProfile{Gain: 0, MinExposureMs: 0.032, MaxExposureMs: 5000, ADUTarget: 30, Stretch: "none"},
	"imaging.night":    ImagingProfile{Gain: 300, MinExposureMs: 1000, MaxExposureMs: 30000, ADUTarget: 30, Stretch: "auto"},
	"imaging.twilight": map[string]interface{}{"sunAltitude": -6.0, "transitionSpeed": 1},
	"imaging.mask":     map[string]interface{}{"centerX": 1776, "centerY": 1776, "radius": 1700, "enabled": false},
	"location":         LocationConfig{Latitude: 0, Longitude: 0, Elevation: 0},
	"location.gpsd":    map[string]interface{}{"enabled": false},
	"autoexposure.buffer":  2.0,
	"autoexposure.history": 10,
}

func SeedDefaults() {
	for key, value := range defaults {
		data, _ := json.Marshal(value)
		db.GetDB().FirstOrCreate(&models.Config{Key: key}, models.Config{
			Key:   key,
			Value: data,
		})
	}
}
