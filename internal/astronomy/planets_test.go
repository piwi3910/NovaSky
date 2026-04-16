package astronomy

import (
	"testing"
	"time"
)

func TestPlanetPositions_Count(t *testing.T) {
	// Should always return 5 planets (Mercury, Venus, Mars, Jupiter, Saturn)
	tt := time.Date(2024, 6, 21, 0, 0, 0, 0, time.UTC)
	planets := PlanetPositions(tt, 51.0, 4.0)

	if len(planets) != 5 {
		t.Fatalf("expected 5 planets, got %d", len(planets))
	}

	expected := map[string]bool{
		"Mercury": false,
		"Venus":   false,
		"Mars":    false,
		"Jupiter": false,
		"Saturn":  false,
	}
	for _, p := range planets {
		if _, ok := expected[p.Name]; !ok {
			t.Errorf("unexpected planet: %s", p.Name)
		}
		expected[p.Name] = true
	}
	for name, found := range expected {
		if !found {
			t.Errorf("missing planet: %s", name)
		}
	}
}

func TestPlanetPositions_AltRange(t *testing.T) {
	tt := time.Date(2024, 6, 21, 0, 0, 0, 0, time.UTC)
	planets := PlanetPositions(tt, 51.0, 4.0)

	for _, p := range planets {
		if p.Alt < -90 || p.Alt > 90 {
			t.Errorf("%s altitude out of range [-90,90]: got %.2f", p.Name, p.Alt)
		}
		if p.Az < 0 || p.Az > 360 {
			t.Errorf("%s azimuth out of range [0,360]: got %.2f", p.Name, p.Az)
		}
	}
}

func TestPlanetPositions_RADecRange(t *testing.T) {
	tt := time.Date(2024, 3, 15, 12, 0, 0, 0, time.UTC)
	planets := PlanetPositions(tt, 51.0, 4.0)

	for _, p := range planets {
		if p.RA < 0 || p.RA > 24 {
			t.Errorf("%s RA out of range [0,24]: got %.4f", p.Name, p.RA)
		}
		if p.Dec < -90 || p.Dec > 90 {
			t.Errorf("%s Dec out of range [-90,90]: got %.4f", p.Name, p.Dec)
		}
	}
}

func TestPlanetPositions_Visibility(t *testing.T) {
	// Test multiple times — at least one planet should be visible at some point
	times := []time.Time{
		time.Date(2024, 1, 15, 22, 0, 0, 0, time.UTC),
		time.Date(2024, 4, 15, 22, 0, 0, 0, time.UTC),
		time.Date(2024, 7, 15, 22, 0, 0, 0, time.UTC),
		time.Date(2024, 10, 15, 22, 0, 0, 0, time.UTC),
	}

	anyVisible := false
	for _, tt := range times {
		planets := PlanetPositions(tt, 51.0, 4.0)
		for _, p := range planets {
			if p.Visible {
				anyVisible = true
			}
		}
	}

	if !anyVisible {
		t.Error("expected at least one planet to be visible across 4 different dates")
	}
}

func TestPlanetPositions_VisibleFlag(t *testing.T) {
	tt := time.Date(2024, 6, 21, 0, 0, 0, 0, time.UTC)
	planets := PlanetPositions(tt, 51.0, 4.0)

	for _, p := range planets {
		if p.Alt > 0 && !p.Visible {
			t.Errorf("%s has alt=%.2f but Visible=false", p.Name, p.Alt)
		}
		if p.Alt <= 0 && p.Visible {
			t.Errorf("%s has alt=%.2f but Visible=true", p.Name, p.Alt)
		}
	}
}

func TestEquatorialToHorizontal(t *testing.T) {
	// Polaris: RA ~2.53h, Dec ~89.26 deg
	// At latitude 51N, Polaris altitude should be close to the latitude
	// Use LST = RA*15 so HA = 0 (on meridian)
	polarisRA := 2.53   // hours
	polarisDec := 89.26 // degrees
	lat := 51.0
	lst := polarisRA * 15.0 // degrees, so HA = 0

	alt, _ := equatorialToHorizontal(polarisRA, polarisDec, lat, lst)

	// Polaris on meridian: alt should be very close to latitude for dec~90
	// More precisely: sin(alt) = sin(lat)*sin(dec) + cos(lat)*cos(dec)*cos(0)
	// For dec=89.26, this gives alt ~ 51 +/- a few degrees
	if alt < 40 || alt > 65 {
		t.Errorf("Polaris altitude at lat 51N should be ~51deg, got %.2f", alt)
	}
}
