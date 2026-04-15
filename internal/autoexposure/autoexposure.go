package autoexposure

import (
	"log"
	"math"
	"time"

	"github.com/piwi3910/NovaSky/internal/astronomy"
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
		transitionSpeed = 25
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
	return astronomy.SunAltitude(time.Now(), e.latitude, e.longitude)
}

// Sun altitude calculation moved to internal/astronomy.SunAltitude()

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

	if errorPct > 20.0 {
		// SLEW mode: large error, apply capped ratio correction
		e.phase = "slew"
		ratio := targetPixel / medianADU
		// Cap max change to 2x per cycle to avoid wild swings
		ratio = clamp(ratio, 0.5, 2.0)

		newExposure := e.currentExposure * ratio
		e.currentExposure = clamp(newExposure, profile.MinExposureMs, profile.MaxExposureMs)
	} else if errorPct > e.bufferPct {
		// CONVERGE mode: close to target, use damped correction
		e.phase = "converge"
		error := (targetPixel - medianADU) / targetPixel
		// Apply 30% of error — fast enough to converge, slow enough to not oscillate
		correction := 1.0 + (error * 0.3)
		e.currentExposure *= correction
		e.currentExposure = clamp(e.currentExposure, profile.MinExposureMs, profile.MaxExposureMs)

		// Apply predictive correction if we have history
		prediction := e.predictTrend()
		if prediction != 0 {
			e.currentExposure *= (1.0 + prediction*0.3)
			e.currentExposure = clamp(e.currentExposure, profile.MinExposureMs, profile.MaxExposureMs)
			e.phase = "predict"
		}
	} else {
		// TRACK mode: within buffer zone, very gentle corrections only
		e.phase = "track"
		error := (targetPixel - medianADU) / targetPixel
		// 5% of error — barely nudge, let it settle naturally
		correction := 1.0 + (error * 0.05)
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

// NeedsRapidCapture returns true when ADU is far off target (>20% error)
// and we should skip the normal capture interval to converge faster.
// Does NOT rapid-capture if at exposure limits (can't improve anyway).
func (e *Engine) NeedsRapidCapture() bool {
	if e.lastMedianADU <= 0 {
		return true // No data yet, capture fast
	}
	profile := e.ActiveProfile()

	// Already at limits — rapid capture won't help
	if e.currentExposure <= profile.MinExposureMs || e.currentExposure >= profile.MaxExposureMs {
		return false
	}

	targetPixel := (profile.ADUTarget / 100.0) * 65535.0
	errorPct := math.Abs(e.lastMedianADU-targetPixel) / targetPixel * 100.0
	return errorPct > 20.0
}

// IsConverged returns true when the frame is good enough to send to the pipeline.
// Uses a wider threshold (20%) than the PID buffer zone (2%).
// Frames within 20% of target are sent to pipeline; only frames > 20% off are discarded.
// The tight 2% buffer only controls PID vs slew mode, not pipeline gating.
func (e *Engine) IsConverged() bool {
	if e.lastMedianADU <= 0 {
		return false
	}
	profile := e.ActiveProfile()
	targetPixel := (profile.ADUTarget / 100.0) * 65535.0
	errorPct := math.Abs(e.lastMedianADU-targetPixel) / targetPixel * 100.0

	// Within 20% of target — good enough for pipeline
	if errorPct <= 20.0 {
		return true
	}

	// At exposure limits — can't do better, accept it
	if e.currentExposure <= profile.MinExposureMs || e.currentExposure >= profile.MaxExposureMs {
		return true
	}

	return false
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
