package detection

import (
	"math"
)

// ConstellationLine defines a stick-figure line between two stars
type ConstellationLine struct {
	Star1 string  `json:"star1"`
	Star2 string  `json:"star2"`
	RA1   float64 `json:"ra1"` // hours
	Dec1  float64 `json:"dec1"` // degrees
	RA2   float64 `json:"ra2"` // hours
	Dec2  float64 `json:"dec2"` // degrees
}

// ConstellationDef defines a constellation with its stick-figure lines
type ConstellationDef struct {
	Name  string              `json:"name"`
	Lines []ConstellationLine `json:"lines"`
}

// ProjectedPoint is a pixel coordinate on the image
type ProjectedPoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// ProjectedLine is a constellation line projected to pixel coordinates
type ProjectedLine struct {
	Star1 string         `json:"star1"`
	Star2 string         `json:"star2"`
	P1    ProjectedPoint `json:"p1"`
	P2    ProjectedPoint `json:"p2"`
}

// ProjectedConstellation is a constellation with its lines projected to image coordinates
type ProjectedConstellation struct {
	Name  string          `json:"name"`
	Lines []ProjectedLine `json:"lines"`
}

// Constellation catalog — brightest constellations with named star endpoints
// RA in hours, Dec in degrees (J2000.0)
var constellationCatalog = []ConstellationDef{
	{
		Name: "Orion",
		Lines: []ConstellationLine{
			{"Betelgeuse", "Bellatrix", 5.9195, 7.4071, 5.4188, 6.3497},
			{"Bellatrix", "Mintaka", 5.4188, 6.3497, 5.5335, -0.2991},
			{"Mintaka", "Alnilam", 5.5335, -0.2991, 5.6036, -1.2019},
			{"Alnilam", "Alnitak", 5.6036, -1.2019, 5.6793, -1.9425},
			{"Alnitak", "Saiph", 5.6793, -1.9425, 5.7954, -9.6697},
			{"Saiph", "Rigel", 5.7954, -9.6697, 5.2423, -8.2017},
			{"Rigel", "Mintaka", 5.2423, -8.2017, 5.5335, -0.2991},
			{"Betelgeuse", "Meissa", 5.9195, 7.4071, 5.5853, 9.9339},
		},
	},
	{
		Name: "Ursa Major",
		Lines: []ConstellationLine{
			{"Dubhe", "Merak", 11.0621, 61.7509, 11.0306, 56.3824},
			{"Merak", "Phecda", 11.0306, 56.3824, 11.8977, 53.6948},
			{"Phecda", "Megrez", 11.8977, 53.6948, 12.2571, 57.0326},
			{"Megrez", "Alioth", 12.2571, 57.0326, 12.9005, 55.9598},
			{"Alioth", "Mizar", 12.9005, 55.9598, 13.3988, 54.9254},
			{"Mizar", "Alkaid", 13.3988, 54.9254, 13.7924, 49.3133},
			{"Megrez", "Dubhe", 12.2571, 57.0326, 11.0621, 61.7509},
		},
	},
	{
		Name: "Cassiopeia",
		Lines: []ConstellationLine{
			{"Schedar", "Caph", 0.6751, 56.5374, 0.1528, 59.1498},
			{"Schedar", "Gamma Cas", 0.6751, 56.5374, 0.9451, 60.7167},
			{"Gamma Cas", "Ruchbah", 0.9451, 60.7167, 1.4303, 60.2353},
			{"Ruchbah", "Segin", 1.4303, 60.2353, 1.9065, 63.6700},
		},
	},
	{
		Name: "Leo",
		Lines: []ConstellationLine{
			{"Regulus", "Eta Leo", 10.1395, 11.9672, 10.1222, 16.7627},
			{"Eta Leo", "Algieba", 10.1222, 16.7627, 10.3328, 19.8415},
			{"Algieba", "Zosma", 10.3328, 19.8415, 11.2351, 20.5243},
			{"Zosma", "Denebola", 11.2351, 20.5243, 11.8177, 14.5720},
			{"Denebola", "Theta Leo", 11.8177, 14.5720, 11.2373, 15.4297},
			{"Theta Leo", "Regulus", 11.2373, 15.4297, 10.1395, 11.9672},
			{"Algieba", "Epsilon Leo", 10.3328, 19.8415, 9.7641, 23.7743},
			{"Epsilon Leo", "Mu Leo", 9.7641, 23.7743, 9.8794, 26.0069},
		},
	},
	{
		Name: "Scorpius",
		Lines: []ConstellationLine{
			{"Antares", "Sigma Sco", 16.4901, -26.4320, 16.3530, -25.5928},
			{"Sigma Sco", "Dschubba", 16.3530, -25.5928, 16.0055, -22.6217},
			{"Dschubba", "Acrab", 16.0055, -22.6217, 16.0913, -19.8053},
			{"Antares", "Tau Sco", 16.4901, -26.4320, 16.5981, -28.2160},
			{"Tau Sco", "Epsilon Sco", 16.5981, -28.2160, 16.8364, -34.2926},
			{"Epsilon Sco", "Mu1 Sco", 16.8364, -34.2926, 16.8647, -38.0474},
			{"Mu1 Sco", "Zeta Sco", 16.8647, -38.0474, 16.8969, -42.3618},
			{"Zeta Sco", "Eta Sco", 16.8969, -42.3618, 17.2027, -43.2391},
			{"Eta Sco", "Theta Sco", 17.2027, -43.2391, 17.6224, -42.9978},
			{"Theta Sco", "Shaula", 17.6224, -42.9978, 17.5603, -37.1038},
		},
	},
	{
		Name: "Cygnus",
		Lines: []ConstellationLine{
			{"Deneb", "Sadr", 20.6905, 45.2803, 20.3706, 40.2567},
			{"Sadr", "Albireo", 20.3706, 40.2567, 19.5120, 27.9597},
			{"Sadr", "Gienah", 20.3706, 40.2567, 20.7703, 33.9703},
			{"Sadr", "Delta Cyg", 20.3706, 40.2567, 19.7499, 45.1308},
		},
	},
	{
		Name: "Gemini",
		Lines: []ConstellationLine{
			{"Pollux", "Castor", 7.7553, 28.0262, 7.5766, 31.8884},
			{"Castor", "Mebsuta", 7.5766, 31.8884, 6.7325, 25.1311},
			{"Mebsuta", "Tejat", 6.7325, 25.1311, 6.3828, 22.5137},
			{"Pollux", "Kappa Gem", 7.7553, 28.0262, 7.7141, 24.3980},
			{"Kappa Gem", "Wasat", 7.7141, 24.3980, 7.3356, 21.9823},
			{"Wasat", "Alhena", 7.3356, 21.9823, 6.6286, 16.3993},
		},
	},
	{
		Name: "Taurus",
		Lines: []ConstellationLine{
			{"Aldebaran", "Theta2 Tau", 4.5988, 16.5093, 4.4740, 15.8708},
			{"Theta2 Tau", "Gamma Tau", 4.4740, 15.8708, 4.3297, 15.6275},
			{"Gamma Tau", "Delta Tau", 4.3297, 15.6275, 4.3825, 17.5425},
			{"Delta Tau", "Epsilon Tau", 4.3825, 17.5425, 4.4769, 19.1804},
			{"Epsilon Tau", "Aldebaran", 4.4769, 19.1804, 4.5988, 16.5093},
			{"Aldebaran", "Zeta Tau", 4.5988, 16.5093, 5.6274, 21.1426},
			{"Zeta Tau", "Elnath", 5.6274, 21.1426, 5.4382, 28.6075},
		},
	},
	{
		Name: "Canis Major",
		Lines: []ConstellationLine{
			{"Sirius", "Mirzam", -6.7525, -16.7161, 6.3786, -17.9559},
			{"Sirius", "Wezen", -6.7525, -16.7161, 7.1399, -26.3932},
			{"Wezen", "Aludra", 7.1399, -26.3932, 7.4016, -29.3031},
			{"Wezen", "Adhara", 7.1399, -26.3932, 6.9771, -28.9722},
			{"Adhara", "Furud", 6.9771, -28.9722, 6.3382, -30.0634},
		},
	},
	{
		Name: "Crux",
		Lines: []ConstellationLine{
			{"Acrux", "Gacrux", 12.4433, -63.0990, 12.5194, -57.1132},
			{"Mimosa", "Delta Cru", 12.7953, -59.6886, 12.2524, -58.7490},
		},
	},
}

// fixCMaRA corrects the Canis Major catalog entries that used negative RA
func init() {
	for ci := range constellationCatalog {
		if constellationCatalog[ci].Name == "Canis Major" {
			for li := range constellationCatalog[ci].Lines {
				// Sirius RA is 6.7525h, not negative
				if constellationCatalog[ci].Lines[li].RA1 < 0 {
					constellationCatalog[ci].Lines[li].RA1 = 6.7525
				}
				if constellationCatalog[ci].Lines[li].RA2 < 0 {
					constellationCatalog[ci].Lines[li].RA2 = 6.7525
				}
			}
		}
	}
}

// ProjectConstellations projects all visible constellations onto an all-sky fisheye image.
// lst is local sidereal time in hours, lat is observer latitude in degrees.
// imageSize is the diameter of the circular all-sky image in pixels.
func ProjectConstellations(lst, lat float64, imageSize int) []ProjectedConstellation {
	cx := float64(imageSize) / 2.0
	cy := float64(imageSize) / 2.0
	radius := float64(imageSize) / 2.0

	var result []ProjectedConstellation

	for _, c := range constellationCatalog {
		allVisible := true
		var projectedLines []ProjectedLine

		for _, line := range c.Lines {
			alt1, az1 := raDecToAltAz(line.RA1, line.Dec1, lst, lat)
			alt2, az2 := raDecToAltAz(line.RA2, line.Dec2, lst, lat)

			if alt1 <= 0 || alt2 <= 0 {
				allVisible = false
				break
			}

			p1 := altAzToPixel(alt1, az1, cx, cy, radius)
			p2 := altAzToPixel(alt2, az2, cx, cy, radius)

			projectedLines = append(projectedLines, ProjectedLine{
				Star1: line.Star1,
				Star2: line.Star2,
				P1:    p1,
				P2:    p2,
			})
		}

		if allVisible && len(projectedLines) > 0 {
			result = append(result, ProjectedConstellation{
				Name:  c.Name,
				Lines: projectedLines,
			})
		}
	}

	return result
}

// raDecToAltAz converts equatorial coordinates to horizontal coordinates
func raDecToAltAz(raHours, decDeg, lstHours, latDeg float64) (alt, az float64) {
	ha := (lstHours - raHours) * 15.0 // hour angle in degrees
	haRad := ha * math.Pi / 180.0
	decRad := decDeg * math.Pi / 180.0
	latRad := latDeg * math.Pi / 180.0

	sinAlt := math.Sin(decRad)*math.Sin(latRad) + math.Cos(decRad)*math.Cos(latRad)*math.Cos(haRad)
	alt = math.Asin(sinAlt) * 180.0 / math.Pi

	cosAz := (math.Sin(decRad) - math.Sin(alt*math.Pi/180.0)*math.Sin(latRad)) /
		(math.Cos(alt*math.Pi/180.0) * math.Cos(latRad))

	// Clamp for numerical safety
	if cosAz > 1 {
		cosAz = 1
	}
	if cosAz < -1 {
		cosAz = -1
	}

	az = math.Acos(cosAz) * 180.0 / math.Pi
	if math.Sin(haRad) > 0 {
		az = 360.0 - az
	}
	return
}

// altAzToPixel projects altitude/azimuth onto a fisheye all-sky image
// Zenith is at center, horizon at edge
func altAzToPixel(alt, az, cx, cy, imgRadius float64) ProjectedPoint {
	azRad := az * math.Pi / 180.0
	r := imgRadius * (90.0 - alt) / 90.0
	x := cx + r*math.Sin(azRad)
	y := cy - r*math.Cos(azRad)
	return ProjectedPoint{X: math.Round(x*10) / 10, Y: math.Round(y*10) / 10}
}

// GetConstellationCatalog returns the full constellation catalog for external use
func GetConstellationCatalog() []ConstellationDef {
	return constellationCatalog
}
