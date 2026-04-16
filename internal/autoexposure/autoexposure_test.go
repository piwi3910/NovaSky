package autoexposure

import (
	"math"
	"testing"
)

// helpers

func dayProfile() Profile {
	return Profile{
		Gain:          0,
		MinExposureMs: 0.032,
		MaxExposureMs: 100,
		ADUTarget:     50,
		Stretch:       "linear",
	}
}

func nightProfile() Profile {
	return Profile{
		Gain:          300,
		MinExposureMs: 1,
		MaxExposureMs: 15000,
		ADUTarget:     25,
		Stretch:       "asinh",
	}
}

// newTestEngine creates an engine whose GetMode always returns the given mode.
// We achieve this by setting latitude/longitude and twilightAngle so that
// the current sun altitude is deterministically above or below the angle.
// Since we cannot control time.Now, we use a twilight angle of +90 (always night)
// or -90 (always day).
func newTestEngine(mode string) *Engine {
	tw := 90.0 // sun is never above 90 -> always "night"
	if mode == "day" {
		tw = -90.0 // sun is always above -90 -> always "day"
	}
	return New(dayProfile(), nightProfile(), tw, 25, 51.0, 4.0, 2.0, 10)
}

func TestNew_Defaults(t *testing.T) {
	e := New(dayProfile(), nightProfile(), -6.0, 0, 51.0, 4.0, 0, 0)
	if e.bufferPct != 2.0 {
		t.Errorf("bufferPct: got %f, want 2.0", e.bufferPct)
	}
	if e.historySize != 10 {
		t.Errorf("historySize: got %d, want 10", e.historySize)
	}
	if e.transitionSpd != 25 {
		t.Errorf("transitionSpd: got %d, want 25", e.transitionSpd)
	}
	if e.phase != "slew" {
		t.Errorf("phase: got %q, want \"slew\"", e.phase)
	}
}

func TestNew_CustomValues(t *testing.T) {
	e := New(dayProfile(), nightProfile(), -6.0, 50, 51.0, 4.0, 5.0, 20)
	if e.bufferPct != 5.0 {
		t.Errorf("bufferPct: got %f, want 5.0", e.bufferPct)
	}
	if e.historySize != 20 {
		t.Errorf("historySize: got %d, want 20", e.historySize)
	}
	if e.transitionSpd != 50 {
		t.Errorf("transitionSpd: got %d, want 50", e.transitionSpd)
	}
}

func TestGetMode(t *testing.T) {
	// twilightAngle = -90 means sun is always "above" it -> day
	eDay := newTestEngine("day")
	if got := eDay.GetMode(); got != "day" {
		t.Errorf("GetMode: got %q, want \"day\"", got)
	}

	// twilightAngle = 90 means sun is always "below" it -> night
	eNight := newTestEngine("night")
	if got := eNight.GetMode(); got != "night" {
		t.Errorf("GetMode: got %q, want \"night\"", got)
	}
}

func TestRampGain(t *testing.T) {
	tests := []struct {
		name       string
		current    int
		target     int
		speed      int
		wantAfter  int
	}{
		{"ramp up partial", 100, 200, 25, 125},
		{"ramp up exact", 100, 125, 25, 125},
		{"ramp up overshoot clamped", 100, 110, 25, 110},
		{"ramp down partial", 200, 100, 25, 175},
		{"ramp down exact", 125, 100, 25, 100},
		{"ramp down overshoot clamped", 110, 100, 25, 100},
		{"already at target", 100, 100, 25, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEngine("night")
			e.currentGain = tt.current
			e.targetGain = tt.target
			e.transitionSpd = tt.speed
			e.rampGain()
			if e.currentGain != tt.wantAfter {
				t.Errorf("rampGain: got %d, want %d", e.currentGain, tt.wantAfter)
			}
		})
	}
}

func TestAdjust_Slew(t *testing.T) {
	e := newTestEngine("night")
	profile := e.ActiveProfile()
	targetPixel := (profile.ADUTarget / 100.0) * 65535.0

	// Set an ADU that is > 20% below target
	medianADU := targetPixel * 0.5 // 50% of target -> error > 20%
	e.currentExposure = 1000.0

	e.Adjust(medianADU)

	if e.phase != "slew" {
		t.Errorf("phase: got %q, want \"slew\"", e.phase)
	}
	// Ratio = target/median = 2.0, capped at 2.0, so exposure should double
	if e.currentExposure < 1999.0 || e.currentExposure > 2001.0 {
		t.Errorf("exposure after slew: got %.3f, want ~2000", e.currentExposure)
	}
}

func TestAdjust_Slew_RatioCapped(t *testing.T) {
	e := newTestEngine("night")
	profile := e.ActiveProfile()
	targetPixel := (profile.ADUTarget / 100.0) * 65535.0

	// Set ADU very low so ratio would be > 2.0
	medianADU := targetPixel * 0.1
	e.currentExposure = 1000.0

	e.Adjust(medianADU)

	if e.phase != "slew" {
		t.Errorf("phase: got %q, want \"slew\"", e.phase)
	}
	// Ratio capped at 2.0
	if e.currentExposure < 1999.0 || e.currentExposure > 2001.0 {
		t.Errorf("exposure capped at 2x: got %.3f, want ~2000", e.currentExposure)
	}
}

func TestAdjust_Converge(t *testing.T) {
	e := newTestEngine("night")
	profile := e.ActiveProfile()
	targetPixel := (profile.ADUTarget / 100.0) * 65535.0

	// Error between 2% and 20% -> converge mode
	medianADU := targetPixel * 0.9 // 10% error
	e.currentExposure = 1000.0

	e.Adjust(medianADU)

	if e.phase != "converge" {
		t.Errorf("phase: got %q, want \"converge\"", e.phase)
	}
	// 30% correction of ~10% under -> small increase
	if e.currentExposure <= 1000.0 {
		t.Errorf("exposure should increase from 1000, got %.3f", e.currentExposure)
	}
}

func TestAdjust_Track(t *testing.T) {
	e := newTestEngine("night")
	profile := e.ActiveProfile()
	targetPixel := (profile.ADUTarget / 100.0) * 65535.0

	// Error < 2% -> track mode
	medianADU := targetPixel * 0.99 // 1% error
	e.currentExposure = 1000.0

	e.Adjust(medianADU)

	if e.phase != "track" {
		t.Errorf("phase: got %q, want \"track\"", e.phase)
	}
	// 5% correction of 1% error -> very small change
	diff := math.Abs(e.currentExposure - 1000.0)
	if diff > 10.0 {
		t.Errorf("track should make tiny adjustment, diff=%.3f", diff)
	}
}

func TestAdjust_ZeroADU(t *testing.T) {
	e := newTestEngine("night")
	e.currentExposure = 500.0

	e.Adjust(0)

	if e.phase != "slew" {
		t.Errorf("phase: got %q, want \"slew\"", e.phase)
	}
	if e.currentExposure != 1000.0 {
		t.Errorf("zero ADU should double exposure: got %.3f, want 1000", e.currentExposure)
	}
}

func TestAdjust_ExposureClamped(t *testing.T) {
	e := newTestEngine("night")
	profile := e.ActiveProfile()

	// Set exposure near max and provide low ADU to try doubling
	e.currentExposure = profile.MaxExposureMs - 100
	e.Adjust(0) // doubles, should clamp to max

	if e.currentExposure != profile.MaxExposureMs {
		t.Errorf("exposure should clamp to max %f, got %f", profile.MaxExposureMs, e.currentExposure)
	}

	// Set exposure near min with very high ADU
	targetPixel := (profile.ADUTarget / 100.0) * 65535.0
	e.currentExposure = profile.MinExposureMs + 0.5
	e.Adjust(targetPixel * 10) // way too bright, ratio < 0.5 capped

	if e.currentExposure < profile.MinExposureMs {
		t.Errorf("exposure should not go below min %f, got %f", profile.MinExposureMs, e.currentExposure)
	}
}

func TestIsConverged(t *testing.T) {
	tests := []struct {
		name     string
		aduFrac  float64 // fraction of targetPixel
		atLimit  bool
		want     bool
	}{
		{"within 20%", 0.85, false, true},
		{"exactly at target", 1.0, false, true},
		{"outside 20%", 0.5, false, false},
		{"outside but at max limit", 0.5, true, true},
		{"no ADU data", 0, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEngine("night")
			profile := e.ActiveProfile()
			targetPixel := (profile.ADUTarget / 100.0) * 65535.0

			e.lastMedianADU = targetPixel * tt.aduFrac
			if tt.atLimit {
				e.currentExposure = profile.MaxExposureMs
			} else {
				e.currentExposure = 1000.0 // mid-range
			}

			got := e.IsConverged()
			if got != tt.want {
				t.Errorf("IsConverged: got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNeedsRapidCapture(t *testing.T) {
	tests := []struct {
		name    string
		aduFrac float64
		atLimit bool
		want    bool
	}{
		{"no data yet", 0, false, true},
		{"large error not at limit", 0.3, false, true},
		{"large error at max limit", 0.3, true, false},
		{"small error", 0.95, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEngine("night")
			profile := e.ActiveProfile()
			targetPixel := (profile.ADUTarget / 100.0) * 65535.0

			e.lastMedianADU = targetPixel * tt.aduFrac
			if tt.atLimit {
				e.currentExposure = profile.MaxExposureMs
			} else {
				e.currentExposure = 1000.0
			}

			got := e.NeedsRapidCapture()
			if got != tt.want {
				t.Errorf("NeedsRapidCapture: got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResume(t *testing.T) {
	e := newTestEngine("night")
	e.Resume(5000.0, 200)

	if e.currentExposure != 5000.0 {
		t.Errorf("Resume exposure: got %f, want 5000", e.currentExposure)
	}
	if e.currentGain != 200 {
		t.Errorf("Resume gain: got %d, want 200", e.currentGain)
	}
	// targetGain should be set to active profile's gain
	profile := e.ActiveProfile()
	if e.targetGain != profile.Gain {
		t.Errorf("Resume targetGain: got %d, want %d", e.targetGain, profile.Gain)
	}
}

func TestResume_ZeroValues(t *testing.T) {
	e := newTestEngine("night")
	origExposure := e.currentExposure
	origGain := e.currentGain

	// Zero/negative values should not change current settings
	e.Resume(0, -1)
	if e.currentExposure != origExposure {
		t.Errorf("Resume(0,...) should not change exposure: got %f, want %f", e.currentExposure, origExposure)
	}
	if e.currentGain != origGain {
		t.Errorf("Resume(...,-1) should not change gain: got %d, want %d", e.currentGain, origGain)
	}
}

func TestUpdateConfig(t *testing.T) {
	e := newTestEngine("night")

	newDay := dayProfile()
	newDay.Gain = 10
	newNight := nightProfile()
	newNight.Gain = 350

	e.UpdateConfig(newDay, newNight, -8.0, 50, 52.0, 5.0, 3.0)

	if e.twilightAngle != -8.0 {
		t.Errorf("twilightAngle: got %f, want -8.0", e.twilightAngle)
	}
	if e.transitionSpd != 50 {
		t.Errorf("transitionSpd: got %d, want 50", e.transitionSpd)
	}
	if e.latitude != 52.0 {
		t.Errorf("latitude: got %f, want 52.0", e.latitude)
	}
	if e.longitude != 5.0 {
		t.Errorf("longitude: got %f, want 5.0", e.longitude)
	}
	if e.bufferPct != 3.0 {
		t.Errorf("bufferPct: got %f, want 3.0", e.bufferPct)
	}
}
