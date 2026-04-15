package astronomy

import (
	"math"
	"time"
)

// PlanetPosition holds computed position data for a planet
type PlanetPosition struct {
	Name    string  `json:"name"`
	RA      float64 `json:"ra"`        // hours
	Dec     float64 `json:"dec"`       // degrees
	Alt     float64 `json:"alt"`       // degrees above horizon
	Az      float64 `json:"az"`        // azimuth degrees
	Mag     float64 `json:"magnitude"` // approximate visual magnitude
	Visible bool    `json:"visible"`   // above horizon
}

// Orbital elements for J2000.0 epoch with rates per century
type orbitalElements struct {
	name       string
	a0, aRate  float64 // semi-major axis (AU)
	e0, eRate  float64 // eccentricity
	i0, iRate  float64 // inclination (deg)
	L0, LRate  float64 // mean longitude (deg)
	w0, wRate  float64 // longitude of perihelion (deg)
	O0, ORate  float64 // longitude of ascending node (deg)
	magBase    float64 // base visual magnitude (approximate)
}

// Simplified orbital elements from Standish (JPL) for the five naked-eye planets
// Epoch J2000.0, rates per Julian century
var planetElements = []orbitalElements{
	{
		name: "Mercury",
		a0: 0.38709927, aRate: 0.00000037,
		e0: 0.20563593, eRate: 0.00001906,
		i0: 7.00497902, iRate: -0.00594749,
		L0: 252.25032350, LRate: 149472.67411175,
		w0: 77.45779628, wRate: 0.16047689,
		O0: 48.33076593, ORate: -0.12534081,
		magBase: -0.4,
	},
	{
		name: "Venus",
		a0: 0.72333566, aRate: 0.00000390,
		e0: 0.00677672, eRate: -0.00004107,
		i0: 3.39467605, iRate: -0.00078890,
		L0: 181.97909950, LRate: 58517.81538729,
		w0: 131.60246718, wRate: 0.00268329,
		O0: 76.67984255, ORate: -0.27769418,
		magBase: -4.4,
	},
	{
		name: "Mars",
		a0: 1.52371034, aRate: 0.00001847,
		e0: 0.09339410, eRate: 0.00007882,
		i0: 1.84969142, iRate: -0.00813131,
		L0: -4.55343205, LRate: 19140.30268499,
		w0: -23.94362959, wRate: 0.44441088,
		O0: 49.55953891, ORate: -0.29257343,
		magBase: -1.6,
	},
	{
		name: "Jupiter",
		a0: 5.20288700, aRate: -0.00011607,
		e0: 0.04838624, eRate: -0.00013253,
		i0: 1.30439695, iRate: -0.00183714,
		L0: 34.39644051, LRate: 3034.74612775,
		w0: 14.72847983, wRate: 0.21252668,
		O0: 100.47390909, ORate: 0.20469106,
		magBase: -2.9,
	},
	{
		name: "Saturn",
		a0: 9.53667594, aRate: -0.00125060,
		e0: 0.05386179, eRate: -0.00050991,
		i0: 2.48599187, iRate: 0.00193609,
		L0: 49.95424423, LRate: 1222.49362201,
		w0: 92.59887831, wRate: -0.41897216,
		O0: 113.66242448, ORate: -0.28867794,
		magBase: 0.5,
	},
}

// Earth orbital elements for computing geocentric positions
var earthElements = orbitalElements{
	name: "Earth",
	a0: 1.00000261, aRate: 0.00000562,
	e0: 0.01671123, eRate: -0.00004392,
	i0: -0.00001531, iRate: -0.01294668,
	L0: 100.46457166, LRate: 35999.37244981,
	w0: 102.93768193, wRate: 0.32327364,
	O0: 0.0, ORate: 0.0,
}

// PlanetPositions computes the RA/Dec and Alt/Az for the five naked-eye planets
func PlanetPositions(t time.Time, lat, lon float64) []PlanetPosition {
	// Julian centuries since J2000.0
	jd := julianDay(t)
	T := (jd - 2451545.0) / 36525.0

	// Compute Earth's heliocentric position
	earthX, earthY, earthZ := heliocentricXYZ(earthElements, T)

	// Local sidereal time (degrees)
	lst := localSiderealTime(jd, lon)

	results := make([]PlanetPosition, 0, len(planetElements))
	for _, p := range planetElements {
		// Heliocentric ecliptic coordinates of the planet
		px, py, pz := heliocentricXYZ(p, T)

		// Geocentric ecliptic coordinates
		gx := px - earthX
		gy := py - earthY
		gz := pz - earthZ

		// Distance from Earth (for magnitude estimation)
		distEarth := math.Sqrt(gx*gx + gy*gy + gz*gz)
		distSun := math.Sqrt(px*px + py*py + pz*pz)

		// Convert ecliptic to equatorial (RA/Dec)
		// Obliquity of ecliptic at J2000.0
		eps := degToRad(23.4393 - 0.0000004*(jd-2451545.0))

		// Rotate from ecliptic to equatorial
		eqX := gx
		eqY := gy*math.Cos(eps) - gz*math.Sin(eps)
		eqZ := gy*math.Sin(eps) + gz*math.Cos(eps)

		ra := math.Atan2(eqY, eqX)
		dec := math.Atan2(eqZ, math.Sqrt(eqX*eqX+eqY*eqY))

		// Convert RA to hours (0-24)
		raHours := radToDeg(ra) / 15.0
		if raHours < 0 {
			raHours += 24.0
		}
		decDeg := radToDeg(dec)

		// Convert to Alt/Az
		alt, az := equatorialToHorizontal(raHours, decDeg, lat, lst)

		// Approximate magnitude (simplified — ignores phase angle)
		mag := p.magBase + 5*math.Log10(distSun*distEarth)

		results = append(results, PlanetPosition{
			Name:    p.name,
			RA:      math.Round(raHours*10000) / 10000,
			Dec:     math.Round(decDeg*10000) / 10000,
			Alt:     math.Round(alt*100) / 100,
			Az:      math.Round(az*100) / 100,
			Mag:     math.Round(mag*10) / 10,
			Visible: alt > 0,
		})
	}
	return results
}

// heliocentricXYZ computes heliocentric ecliptic cartesian coordinates
func heliocentricXYZ(el orbitalElements, T float64) (x, y, z float64) {
	// Compute current elements
	a := el.a0 + el.aRate*T
	e := el.e0 + el.eRate*T
	I := degToRad(el.i0 + el.iRate*T)
	L := el.L0 + el.LRate*T
	wBar := el.w0 + el.wRate*T
	O := degToRad(el.O0 + el.ORate*T)

	// Argument of perihelion
	w := degToRad(wBar - (el.O0 + el.ORate*T))

	// Mean anomaly
	M := degToRad(math.Mod(L-wBar, 360.0))

	// Solve Kepler's equation: E - e*sin(E) = M
	E := solveKepler(M, e)

	// True anomaly
	sinV := math.Sqrt(1-e*e) * math.Sin(E) / (1 - e*math.Cos(E))
	cosV := (math.Cos(E) - e) / (1 - e*math.Cos(E))
	v := math.Atan2(sinV, cosV)

	// Radius
	r := a * (1 - e*math.Cos(E))

	// Heliocentric ecliptic coordinates
	cosO := math.Cos(O)
	sinO := math.Sin(O)
	cosI := math.Cos(I)
	sinI := math.Sin(I)
	cosWV := math.Cos(w + v)
	sinWV := math.Sin(w + v)

	x = r * (cosO*cosWV - sinO*sinWV*cosI)
	y = r * (sinO*cosWV + cosO*sinWV*cosI)
	z = r * (sinWV * sinI)
	return
}

// solveKepler solves Kepler's equation iteratively
func solveKepler(M, e float64) float64 {
	E := M
	for i := 0; i < 20; i++ {
		dE := (M - E + e*math.Sin(E)) / (1 - e*math.Cos(E))
		E += dE
		if math.Abs(dE) < 1e-12 {
			break
		}
	}
	return E
}

// equatorialToHorizontal converts RA/Dec to Alt/Az
func equatorialToHorizontal(raHours, decDeg, latDeg, lstDeg float64) (alt, az float64) {
	ha := degToRad(lstDeg - raHours*15.0)
	dec := degToRad(decDeg)
	lat := degToRad(latDeg)

	sinAlt := math.Sin(dec)*math.Sin(lat) + math.Cos(dec)*math.Cos(lat)*math.Cos(ha)
	alt = radToDeg(math.Asin(sinAlt))

	cosAz := (math.Sin(dec) - math.Sin(degToRad(alt))*math.Sin(lat)) / (math.Cos(degToRad(alt)) * math.Cos(lat))
	// Clamp for numerical safety
	if cosAz > 1 {
		cosAz = 1
	}
	if cosAz < -1 {
		cosAz = -1
	}
	az = radToDeg(math.Acos(cosAz))
	if math.Sin(ha) > 0 {
		az = 360 - az
	}
	return
}

// julianDay computes Julian Day from a time.Time
func julianDay(t time.Time) float64 {
	t = t.UTC()
	return float64(t.Unix())/86400.0 + 2440587.5
}

// localSiderealTime returns LST in degrees for a given Julian Day and longitude
func localSiderealTime(jd, lon float64) float64 {
	// Greenwich Mean Sidereal Time
	T := (jd - 2451545.0) / 36525.0
	gmst := 280.46061837 + 360.98564736629*(jd-2451545.0) + 0.000387933*T*T - T*T*T/38710000.0
	gmst = math.Mod(gmst, 360.0)
	if gmst < 0 {
		gmst += 360.0
	}
	lst := math.Mod(gmst+lon, 360.0)
	if lst < 0 {
		lst += 360.0
	}
	return lst
}

func degToRad(d float64) float64 {
	return d * math.Pi / 180.0
}

func radToDeg(r float64) float64 {
	return r * 180.0 / math.Pi
}
