package platesolve

import (
	"log"
	"math"
	"sync"
)

// Calibration holds the camera orientation result
type Calibration struct {
	NorthAngle float64 `json:"northAngle"` // degrees clockwise from image-up to North
	RA         float64 `json:"ra"`         // RA of zenith (degrees)
	Dec        float64 `json:"dec"`        // Dec of zenith (degrees)
	PixelScale float64 `json:"pixelScale"` // arcsec/pixel
	Solved     bool    `json:"solved"`
}

// LogFunc is the signature for the log callback
type LogFunc func(msg string)

var (
	cachedCal *Calibration
	calMu     sync.RWMutex
)

// GetCachedCalibration returns the most recent calibration result
func GetCachedCalibration() *Calibration {
	calMu.RLock()
	defer calMu.RUnlock()
	return cachedCal
}

// CacheCalibration stores a calibration result
func CacheCalibration(cal *Calibration) {
	calMu.Lock()
	defer calMu.Unlock()
	cachedCal = cal
	log.Printf("[platesolve] Calibration cached: North=%.1f°", cal.NorthAngle)
}

// CalcFoV computes the field of view in degrees from focal length and sensor size.
func CalcFoV(focalLengthMm float64, pixelSizeUm float64, imageWidth int) float64 {
	if focalLengthMm <= 0 || pixelSizeUm <= 0 || imageWidth <= 0 {
		return 0
	}
	pixelScale := 206.265 * pixelSizeUm / focalLengthMm
	return pixelScale * float64(imageWidth) / 3600.0
}

// CalcNorthAngle extracts the rotation angle from the CD matrix.
func CalcNorthAngle(cd11, cd12, cd21, cd22 float64) float64 {
	return math.Atan2(cd21, cd22) * 180.0 / math.Pi
}
