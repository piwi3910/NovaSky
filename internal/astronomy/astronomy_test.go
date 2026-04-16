package astronomy

import (
	"math"
	"testing"
	"time"
)

func TestSunAltitude_Daytime(t *testing.T) {
	// 2024-06-21 12:00 UTC, Brussels (50.85N, 4.35E) — summer solstice noon
	noon := time.Date(2024, 6, 21, 12, 0, 0, 0, time.UTC)
	alt := SunAltitude(noon, 50.85, 4.35)
	if alt <= 0 {
		t.Errorf("sun should be above horizon at noon on summer solstice, got alt=%.2f", alt)
	}
	if alt < 50 || alt > 70 {
		t.Errorf("sun altitude at Brussels noon summer solstice should be ~62deg, got %.2f", alt)
	}
}

func TestSunAltitude_Nighttime(t *testing.T) {
	// 2024-06-21 02:00 UTC, Brussels — middle of the night
	night := time.Date(2024, 6, 21, 2, 0, 0, 0, time.UTC)
	alt := SunAltitude(night, 50.85, 4.35)
	if alt > 0 {
		t.Errorf("sun should be below horizon at 2AM, got alt=%.2f", alt)
	}
}

func TestMoonPhase_NewMoon(t *testing.T) {
	// Jan 6, 2000 is the reference new moon in the algorithm
	newMoon := time.Date(2000, 1, 6, 18, 0, 0, 0, time.UTC)
	illum, phase := MoonPhase(newMoon)

	if illum > 0.05 {
		t.Errorf("illumination at new moon should be near 0, got %.4f", illum)
	}
	if phase != "New Moon" {
		t.Errorf("phase at new moon: got %q, want \"New Moon\"", phase)
	}
}

func TestMoonPhase_FullMoon(t *testing.T) {
	// ~14.76 days after new moon reference
	fullMoon := time.Date(2000, 1, 21, 4, 0, 0, 0, time.UTC)
	illum, phase := MoonPhase(fullMoon)

	if illum < 0.9 {
		t.Errorf("illumination at full moon should be near 1.0, got %.4f", illum)
	}
	if phase != "Full Moon" {
		t.Errorf("phase at full moon: got %q, want \"Full Moon\"", phase)
	}
}

func TestSQMToBortle(t *testing.T) {
	tests := []struct {
		sqm  float64
		want int
	}{
		{22.0, 1},
		{21.99, 1},
		{21.90, 2},
		{21.89, 2},
		{21.70, 3},
		{21.69, 3},
		{20.50, 4},
		{20.49, 4},
		{19.50, 5},
		{19.00, 6},
		{18.94, 6},
		{18.50, 7},
		{18.38, 7},
		{18.00, 8},
		{17.80, 8},
		{17.79, 9},
		{15.0, 9},
	}

	for _, tt := range tests {
		got := SQMToBortle(tt.sqm)
		if got != tt.want {
			t.Errorf("SQMToBortle(%.2f): got %d, want %d", tt.sqm, got, tt.want)
		}
	}
}

func TestBortleDescription(t *testing.T) {
	expected := map[int]string{
		1: "Excellent dark-sky site",
		2: "Typical truly dark site",
		3: "Rural sky",
		4: "Rural/suburban transition",
		5: "Suburban sky",
		6: "Bright suburban sky",
		7: "Suburban/urban transition",
		8: "City sky",
		9: "Inner-city sky",
	}

	for bortle, want := range expected {
		got := BortleDescription(bortle)
		if got != want {
			t.Errorf("BortleDescription(%d): got %q, want %q", bortle, got, want)
		}
	}

	// Out of range
	if got := BortleDescription(0); got != "Unknown" {
		t.Errorf("BortleDescription(0): got %q, want \"Unknown\"", got)
	}
	if got := BortleDescription(10); got != "Unknown" {
		t.Errorf("BortleDescription(10): got %q, want \"Unknown\"", got)
	}
}

func TestCalculateSunTimes(t *testing.T) {
	// 2024-06-21, Brussels
	date := time.Date(2024, 6, 21, 12, 0, 0, 0, time.UTC)
	times := CalculateSunTimes(date, 50.85, 4.35)

	// Sunrise must be before sunset
	if !times.Sunrise.Before(times.Sunset) {
		t.Errorf("sunrise (%v) should be before sunset (%v)", times.Sunrise, times.Sunset)
	}

	// Civil dawn before sunrise
	if !times.CivilDawn.Before(times.Sunrise) {
		t.Errorf("civil dawn (%v) should be before sunrise (%v)", times.CivilDawn, times.Sunrise)
	}

	// Nautical dawn before civil dawn
	if !times.NauticalDawn.Before(times.CivilDawn) {
		t.Errorf("nautical dawn (%v) should be before civil dawn (%v)", times.NauticalDawn, times.CivilDawn)
	}

	// Astronomical dawn before nautical dawn
	if !times.AstronomicalDawn.Before(times.NauticalDawn) {
		t.Errorf("astronomical dawn (%v) should be before nautical dawn (%v)", times.AstronomicalDawn, times.NauticalDawn)
	}

	// Sunrise should be roughly around 3:30-5:30 UTC for Brussels in June
	if times.Sunrise.Hour() < 3 || times.Sunrise.Hour() > 6 {
		t.Errorf("sunrise hour should be 3-6 UTC for Brussels in June, got %d", times.Sunrise.Hour())
	}
}

func TestSunAltitude_Range(t *testing.T) {
	// Sun altitude should always be between -90 and +90
	testTimes := []time.Time{
		time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 6, 21, 12, 0, 0, 0, time.UTC),
		time.Date(2024, 12, 21, 0, 0, 0, 0, time.UTC),
	}
	for _, tt := range testTimes {
		alt := SunAltitude(tt, 51.0, 4.0)
		if alt < -90 || alt > 90 {
			t.Errorf("SunAltitude out of range [-90,90]: got %.2f at %v", alt, tt)
		}
		if math.IsNaN(alt) {
			t.Errorf("SunAltitude returned NaN at %v", tt)
		}
	}
}
