package autoexposure

import (
	"log"
	"math"
	"time"

	"github.com/sixdouglas/suncalc"
)

type Profile struct {
	Gain          int     `json:"gain"`
	MinExposureMs float64 `json:"minExposureMs"`
	MaxExposureMs float64 `json:"maxExposureMs"`
	ADUTarget     float64 `json:"aduTarget"`  // percentage 0-100
	Stretch       string  `json:"stretch"`
}

type State struct {
	Mode            string  `json:"mode"` // "day" or "night"
	SunAltitude     float64 `json:"sunAltitude"`
	CurrentExposure float64 `json:"currentExposureMs"`
	CurrentGain     int     `json:"currentGain"`
	TargetGain      int     `json:"targetGain"`
	LastMedianADU   float64 `json:"lastMedianAdu"`
	TargetADU       float64 `json:"targetAdu"`
	Phase           string  `json:"phase"` // "slew", "track", "predict"
	Backpressure    string  `json:"backpressure"` // "normal", "throttled", "paused"
}

type Engine struct {
	dayProfile    Profile
	nightProfile  Profile
	twilightAngle float64
	transitionSpd int // gain units per cycle during twilight transition
	latitude      float64
	longitude     float64
	bufferPct     float64 // ADU buffer zone percentage (default 2)
	historySize   int

	currentExposure float64
	currentGain     int
	targetGain      int
	lastMedianADU   float64
	phase           string

	// Rolling history for prediction
	history []historyEntry
}

type historyEntry struct {
	exposure  float64
	medianADU float64
	timestamp time.Time
}

func New(day, night Profile, twilightAngle float64, transitionSpeed int, lat, lon float64, bufferPct float64, historySize int) *Engine {
	if bufferPct <= 0 {
		bufferPct = 2.0
	}
	if historySize <= 0 {
		historySize = 10
	}
	if transitionSpeed <= 0 {
		transitionSpeed = 1
	}

	e := &Engine{
		dayProfile:    day,
		nightProfile:  night,
		twilightAngle: twilightAngle,
		transitionSpd: transitionSpeed,
		latitude:      lat,
		longitude:     lon,
		bufferPct:     bufferPct,
		historySize:   historySize,
		phase:         "slew",
	}

	// Start with active profile defaults
	p := e.ActiveProfile()
	e.currentExposure = p.MaxExposureMs
	e.currentGain = p.Gain
	e.targetGain = p.Gain

	return e
}

// Resume restores state from persisted values (startup recovery)
func (e *Engine) Resume(exposure float64, gain int) {
	if exposure > 0 {
		e.currentExposure = exposure
	}
	if gain >= 0 {
		e.currentGain = gain
	}
	e.targetGain = e.ActiveProfile().Gain
	log.Printf("[autoexposure] Resumed: exposure=%.3fms gain=%d", e.currentExposure, e.currentGain)
}

func (e *Engine) GetMode() string {
	alt := e.SunAltitude()
	if alt > e.twilightAngle {
		return "day"
	}
	return "night"
}

func (e *Engine) SunAltitude() float64 {
	now := time.Now()
	pos := suncalc.GetPosition(now, e.latitude, e.longitude)
	return pos.Altitude * (180.0 / math.Pi)
}

func (e *Engine) ActiveProfile() Profile {
	if e.GetMode() == "day" {
		return e.dayProfile
	}
	return e.nightProfile
}

func (e *Engine) ExposureMs() float64 {
	return e.currentExposure
}

func (e *Engine) Gain() int {
	return e.currentGain
}

func (e *Engine) Adjust(medianADU float64) {
	e.lastMedianADU = medianADU
	profile := e.ActiveProfile()

	// Update target gain (twilight ramping)
	e.targetGain = profile.Gain
	e.rampGain()

	// Convert ADU target from percentage to 16-bit pixel value
	targetPixel := (profile.ADUTarget / 100.0) * 65535.0

	// Add to history
	e.history = append(e.history, historyEntry{
		exposure:  e.currentExposure,
		medianADU: medianADU,
		timestamp: time.Now(),
	})
	if len(e.history) > e.historySize {
		e.history = e.history[1:]
	}

	if medianADU <= 0 {
		// No usable data, double exposure
		e.currentExposure = math.Min(e.currentExposure*2, profile.MaxExposureMs)
		e.phase = "slew"
		e.log(profile)
		return
	}

	// Calculate error percentage
	errorPct := math.Abs(medianADU-targetPixel) / targetPixel * 100.0

	if errorPct > e.bufferPct {
		// SLEW mode: full ratio correction
		e.phase = "slew"
		ratio := targetPixel / medianADU

		// Apply predictive correction if we have history
		prediction := e.predictTrend()
		if prediction != 0 {
			ratio *= (1.0 + prediction)
			e.phase = "predict"
		}

		newExposure := e.currentExposure * ratio
		e.currentExposure = clamp(newExposure, profile.MinExposureMs, profile.MaxExposureMs)
	} else {
		// TRACK mode: gentle PID correction
		e.phase = "track"
		error := (targetPixel - medianADU) / targetPixel
		// Damped proportional correction (10% of error)
		correction := 1.0 + (error * 0.1)
		e.currentExposure *= correction
		e.currentExposure = clamp(e.currentExposure, profile.MinExposureMs, profile.MaxExposureMs)
	}

	// Round to microsecond precision
	e.currentExposure = math.Round(e.currentExposure*1000) / 1000

	// Gain boost: if at max exposure and still under target
	if e.currentExposure >= profile.MaxExposureMs && medianADU < targetPixel {
		gainRatio := targetPixel / medianADU
		e.currentGain = min(int(float64(e.currentGain)*gainRatio), profile.Gain)
	}

	e.log(profile)
}

func (e *Engine) predictTrend() float64 {
	if len(e.history) < 3 {
		return 0
	}

	// Linear regression on recent exposure values to detect trend
	n := len(e.history)
	recent := e.history[n-3:]

	// Calculate slope of exposure changes
	var sumDelta float64
	for i := 1; i < len(recent); i++ {
		if recent[i-1].exposure > 0 {
			delta := (recent[i].exposure - recent[i-1].exposure) / recent[i-1].exposure
			sumDelta += delta
		}
	}
	avgDelta := sumDelta / float64(len(recent)-1)

	// Only apply prediction if trend is consistent (same direction)
	if math.Abs(avgDelta) < 0.01 {
		return 0 // No significant trend
	}

	return avgDelta * 0.5 // Apply 50% of predicted trend
}

func (e *Engine) rampGain() {
	if e.currentGain == e.targetGain {
		return
	}

	if e.currentGain < e.targetGain {
		e.currentGain = min(e.currentGain+e.transitionSpd, e.targetGain)
	} else {
		e.currentGain = max(e.currentGain-e.transitionSpd, e.targetGain)
	}
}

func (e *Engine) UpdateConfig(day, night Profile, twilightAngle float64, transitionSpeed int, lat, lon float64, bufferPct float64) {
	e.dayProfile = day
	e.nightProfile = night
	e.twilightAngle = twilightAngle
	e.transitionSpd = transitionSpeed
	e.latitude = lat
	e.longitude = lon
	if bufferPct > 0 {
		e.bufferPct = bufferPct
	}

	// Update target gain to new profile
	e.targetGain = e.ActiveProfile().Gain
	log.Printf("[autoexposure] Config updated: mode=%s targetGain=%d", e.GetMode(), e.targetGain)
}

func (e *Engine) GetState() State {
	return State{
		Mode:            e.GetMode(),
		SunAltitude:     math.Round(e.SunAltitude()*100) / 100,
		CurrentExposure: e.currentExposure,
		CurrentGain:     e.currentGain,
		TargetGain:      e.targetGain,
		LastMedianADU:   e.lastMedianADU,
		TargetADU:       e.ActiveProfile().ADUTarget,
		Phase:           e.phase,
	}
}

func (e *Engine) log(profile Profile) {
	log.Printf("[autoexposure] %s/%s exp=%.3fms gain=%d→%d adu=%.0f target=%.0f%%",
		e.GetMode(), e.phase, e.currentExposure, e.currentGain, e.targetGain,
		e.lastMedianADU, profile.ADUTarget)
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
