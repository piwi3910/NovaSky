package platesolve

import (
	"fmt"
	"math"
	"time"
)

// BrightStar is a catalog star with known RA/Dec
type BrightStar struct {
	Name string
	RA   float64 // hours
	Dec  float64 // degrees
	Mag  float64
}

// Bright star catalog — navigational stars visible from most latitudes
var brightStars = []BrightStar{
	{"Sirius", 6.752, -16.716, -1.46},
	{"Canopus", 6.399, -52.696, -0.74},
	{"Arcturus", 14.261, 19.182, -0.05},
	{"Vega", 18.616, 38.784, 0.03},
	{"Capella", 5.278, 45.998, 0.08},
	{"Rigel", 5.242, -8.202, 0.13},
	{"Procyon", 7.655, 5.225, 0.34},
	{"Betelgeuse", 5.919, 7.407, 0.42},
	{"Altair", 19.846, 8.868, 0.76},
	{"Aldebaran", 4.599, 16.509, 0.86},
	{"Spica", 13.420, -11.161, 0.97},
	{"Antares", 16.490, -26.432, 1.04},
	{"Pollux", 7.755, 28.026, 1.14},
	{"Fomalhaut", 22.961, -29.622, 1.16},
	{"Deneb", 20.690, 45.280, 1.25},
	{"Regulus", 10.140, 11.967, 1.35},
	{"Castor", 7.577, 31.888, 1.58},
	{"Bellatrix", 5.419, 6.350, 1.64},
	{"Alnilam", 5.603, -1.202, 1.69},
	{"Dubhe", 11.062, 61.751, 1.79},
	{"Menkalinan", 5.992, 44.948, 1.90},
	{"Merak", 11.031, 56.382, 2.37},
	{"Alioth", 12.900, 55.960, 1.77},
	{"Alkaid", 13.792, 49.314, 1.86},
	{"Cor Caroli", 12.934, 38.318, 2.89},
	{"Denebola", 11.818, 14.572, 2.14},
	{"Mizar", 13.399, 54.925, 2.27},
	{"Rasalhague", 17.582, 12.560, 2.07},
	{"Alphecca", 15.578, 26.715, 2.23},
	{"Kochab", 14.845, 74.156, 2.08},
}

// DetectedStar is a star found in the image with pixel coordinates
type DetectedStar struct {
	X, Y       float64
	Brightness float64
}

// SolveRotation finds the camera rotation angle by matching detected stars
// against the bright star catalog. Returns the angle in degrees where
// North is "up" in the image.
//
// Method: for each candidate rotation (0-359° in 1° steps):
//   - Project catalog stars to pixel coordinates using equidistant fisheye projection
//   - Count how many catalog stars have a detected star within tolerance
//   - The rotation with the most matches wins
func SolveRotation(detected []DetectedStar, lat, lon float64, imageWidth, imageHeight int, fovDeg float64, t time.Time) (*Calibration, error) {
	if len(detected) < 5 {
		return nil, fmt.Errorf("need at least 5 detected stars, got %d", len(detected))
	}

	// Compute LST
	t = t.UTC()
	jd := float64(t.Unix())/86400.0 + 2440587.5
	gmst := math.Mod(280.46061837+360.98564736629*(jd-2451545.0), 360.0)
	if gmst < 0 {
		gmst += 360.0
	}
	lst := math.Mod(gmst+lon, 360.0) // degrees

	// Filter catalog to stars above horizon
	latRad := lat * math.Pi / 180.0
	var visible []struct {
		alt, az float64
		star    BrightStar
	}
	for _, s := range brightStars {
		alt, az := raDecToAltAz(s.RA*15.0, s.Dec, lst, latRad) // RA hours→degrees
		if alt > 5 { // at least 5° above horizon
			visible = append(visible, struct {
				alt, az float64
				star    BrightStar
			}{alt, az, s})
		}
	}

	if len(visible) < 3 {
		return nil, fmt.Errorf("only %d catalog stars above horizon — need at least 3", len(visible))
	}

	cx := float64(imageWidth) / 2.0
	cy := float64(imageHeight) / 2.0
	maxRadius := float64(imageWidth) / 2.0
	halfFov := fovDeg / 2.0

	// Match tolerance in pixels (~2% of image width)
	tolerance := float64(imageWidth) * 0.03

	bestAngle := 0.0
	bestCount := 0

	for angleDeg := 0; angleDeg < 360; angleDeg++ {
		angleRad := float64(angleDeg) * math.Pi / 180.0
		matches := 0

		for _, v := range visible {
			// Equidistant fisheye projection:
			// zenith distance = 90 - altitude
			// r = (zenithDist / halfFov) * maxRadius
			zenithDist := 90.0 - v.alt
			if zenithDist > halfFov {
				continue // outside FoV
			}
			r := (zenithDist / halfFov) * maxRadius

			// Azimuth + rotation → pixel position
			azRad := v.az * math.Pi / 180.0
			px := cx + r*math.Sin(azRad+angleRad)
			py := cy - r*math.Cos(azRad+angleRad)

			// Check if any detected star is within tolerance
			for _, d := range detected {
				dx := d.X - px
				dy := d.Y - py
				if dx*dx+dy*dy < tolerance*tolerance {
					matches++
					break
				}
			}
		}

		if matches > bestCount {
			bestCount = matches
			bestAngle = float64(angleDeg)
		}
	}

	if bestCount < 3 {
		return nil, fmt.Errorf("best match only %d stars — not confident enough (need 3+)", bestCount)
	}

	// Compute pixel scale
	pixelScale := fovDeg * 3600.0 / float64(imageWidth) // arcsec/pixel

	return &Calibration{
		NorthAngle: bestAngle,
		RA:         lst / 15.0, // zenith RA in hours→degrees
		Dec:        lat,
		PixelScale: pixelScale,
		Solved:     true,
	}, nil
}

func raDecToAltAz(raDeg, decDeg, lstDeg, latRad float64) (alt, az float64) {
	ha := (lstDeg - raDeg) * math.Pi / 180.0
	decRad := decDeg * math.Pi / 180.0

	sinAlt := math.Sin(latRad)*math.Sin(decRad) + math.Cos(latRad)*math.Cos(decRad)*math.Cos(ha)
	alt = math.Asin(sinAlt) * 180.0 / math.Pi

	cosAz := (math.Sin(decRad) - math.Sin(latRad)*sinAlt) / (math.Cos(latRad) * math.Cos(alt*math.Pi/180.0))
	cosAz = math.Max(-1, math.Min(1, cosAz))
	az = math.Acos(cosAz) * 180.0 / math.Pi
	if math.Sin(ha) > 0 {
		az = 360.0 - az
	}
	return
}
