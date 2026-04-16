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

func TestMoonPhase_Illumination(t *testing.T) {
	// Test that illumination is always 0-1 and phase name is non-empty
	dates := []time.Time{
		time.Date(2024, 1, 11, 12, 0, 0, 0, time.UTC), // known new moon
		time.Date(2024, 1, 25, 12, 0, 0, 0, time.UTC), // known full moon
		time.Date(2024, 3, 10, 12, 0, 0, 0, time.UTC), // new moon
		time.Date(2024, 3, 25, 12, 0, 0, 0, time.UTC), // full moon
	}
	for _, d := range dates {
		illum, phase := MoonPhase(d)
		if illum < 0 || illum > 1 {
			t.Errorf("illumination out of range [0,1]: got %.4f for %s", illum, d)
		}
		if phase == "" {
			t.Errorf("phase name should not be empty for %s", d)
		}
	}
}

func TestMoonPhase_Cycle(t *testing.T) {
	// Over a 30-day cycle, illumination should have both a minimum and maximum
	start := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	minIllum := 1.0
	maxIllum := 0.0
	for i := 0; i < 30; i++ {
		illum, _ := MoonPhase(start.AddDate(0, 0, i))
		if illum < minIllum { minIllum = illum }
		if illum > maxIllum { maxIllum = illum }
	}
	if minIllum > 0.15 {
		t.Errorf("minimum illumination over 30 days should be near 0, got %.4f", minIllum)
	}
	if maxIllum < 0.85 {
		t.Errorf("maximum illumination over 30 days should be near 1, got %.4f", maxIllum)
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
	// Use equinox date at mid-latitude where all twilight types exist
	date := time.Date(2024, 3, 20, 12, 0, 0, 0, time.UTC)
	times := CalculateSunTimes(date, 25.0, 55.0) // Dubai — all twilights exist

	// Sunrise must be before sunset
	if !times.Sunrise.Before(times.Sunset) {
		t.Errorf("sunrise (%v) should be before sunset (%v)", times.Sunrise, times.Sunset)
	}

	// Civil dawn before sunrise (skip if equal — means no solution at this latitude)
	if !times.CivilDawn.Equal(times.Sunrise) && !times.CivilDawn.Before(times.Sunrise) {
		t.Errorf("civil dawn (%v) should be before sunrise (%v)", times.CivilDawn, times.Sunrise)
	}

	// Nautical dawn before civil dawn
	if !times.NauticalDawn.Equal(times.CivilDawn) && !times.NauticalDawn.Before(times.CivilDawn) {
		t.Errorf("nautical dawn (%v) should be before civil dawn (%v)", times.NauticalDawn, times.CivilDawn)
	}

	// Astronomical dawn before nautical dawn
	if !times.AstronomicalDawn.Equal(times.NauticalDawn) && !times.AstronomicalDawn.Before(times.NauticalDawn) {
		t.Errorf("astronomical dawn (%v) should be before nautical dawn (%v)", times.AstronomicalDawn, times.NauticalDawn)
	}

	// Sunrise at equinox in Dubai should be around 2:00-3:00 UTC (6:00-7:00 local UTC+4)
	if times.Sunrise.Hour() < 1 || times.Sunrise.Hour() > 4 {
		t.Errorf("sunrise hour should be 1-4 UTC for Dubai at equinox, got %d", times.Sunrise.Hour())
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
