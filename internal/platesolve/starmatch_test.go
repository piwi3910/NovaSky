package platesolve

import (
	"math"
	"testing"
	"time"
)

func TestSolveRotation_MinStars(t *testing.T) {
	// Fewer than 5 detected stars should error
	detected := []DetectedStar{
		{100, 100, 500},
		{200, 200, 400},
		{300, 300, 300},
		{400, 400, 200},
	}

	_, err := SolveRotation(detected, 51.0, 4.0, 1920, 1080, 180.0, time.Now())
	if err == nil {
		t.Error("expected error for < 5 detected stars, got nil")
	}
}

func TestSolveRotation_KnownPattern(t *testing.T) {
	// Create synthetic detected stars by projecting bright catalog stars
	// at a known rotation angle, then verify SolveRotation finds that angle.
	lat := 51.0
	lon := 4.0
	imgW := 1000
	imgH := 1000
	fov := 180.0
	testTime := time.Date(2024, 1, 15, 22, 0, 0, 0, time.UTC)

	knownAngle := 45.0 // degrees
	angleRad := knownAngle * math.Pi / 180.0

	// Compute LST
	jd := float64(testTime.Unix())/86400.0 + 2440587.5
	gmst := math.Mod(280.46061837+360.98564736629*(jd-2451545.0), 360.0)
	if gmst < 0 {
		gmst += 360.0
	}
	lst := math.Mod(gmst+lon, 360.0)
	latRad := lat * math.Pi / 180.0

	cx := float64(imgW) / 2.0
	cy := float64(imgH) / 2.0
	maxRadius := float64(imgW) / 2.0
	halfFov := fov / 2.0

	// Project catalog stars to pixel coordinates using the known rotation
	var detected []DetectedStar
	for _, s := range brightStars {
		alt, az := raDecToAltAz(s.RA*15.0, s.Dec, lst, latRad)
		if alt <= 5 {
			continue
		}
		zenithDist := 90.0 - alt
		if zenithDist > halfFov {
			continue
		}
		r := (zenithDist / halfFov) * maxRadius
		azRad := az * math.Pi / 180.0
		px := cx + r*math.Sin(azRad+angleRad)
		py := cy - r*math.Cos(azRad+angleRad)

		detected = append(detected, DetectedStar{X: px, Y: py, Brightness: 1000 - s.Mag*100})
	}

	if len(detected) < 5 {
		t.Skipf("not enough visible catalog stars (%d) for this test time", len(detected))
	}

	cal, err := SolveRotation(detected, lat, lon, imgW, imgH, fov, testTime)
	if err != nil {
		t.Fatalf("SolveRotation failed: %v", err)
	}

	if !cal.Solved {
		t.Error("expected Solved=true")
	}

	// The solved angle should be close to 45 degrees (within 2 degrees tolerance)
	diff := math.Abs(cal.NorthAngle - knownAngle)
	if diff > 180 {
		diff = 360 - diff
	}
	if diff > 2.0 {
		t.Errorf("NorthAngle: got %.1f, want ~%.1f (diff=%.1f)", cal.NorthAngle, knownAngle, diff)
	}
}

func TestRaDecToAltAz_Polaris(t *testing.T) {
	// Polaris: RA ~2.53h = 37.95 deg, Dec ~89.26 deg
	// At latitude 51N, altitude should be approximately equal to latitude
	polarisRA := 2.53 * 15.0  // convert hours to degrees
	polarisDec := 89.26
	lat := 51.0
	latRad := lat * math.Pi / 180.0
	lst := polarisRA // LST = RA so HA = 0 (on meridian)

	alt, _ := raDecToAltAz(polarisRA, polarisDec, lst, latRad)

	// For dec ~90, alt on meridian should be close to latitude
	if alt < 40 || alt > 65 {
		t.Errorf("Polaris altitude at lat 51N: got %.2f, want ~51", alt)
	}
}

func TestRaDecToAltAz_BelowHorizon(t *testing.T) {
	// A star at dec=-80 should be well below horizon at lat 51N
	// when on the meridian (HA=0)
	starRA := 100.0 // degrees
	starDec := -80.0
	lat := 51.0
	latRad := lat * math.Pi / 180.0
	lst := starRA // HA = 0

	alt, _ := raDecToAltAz(starRA, starDec, lst, latRad)

	if alt > 0 {
		t.Errorf("star at dec=-80 should be below horizon at lat 51N, got alt=%.2f", alt)
	}
}
