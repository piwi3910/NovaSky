package astronomy

import (
	"math"
	"time"
)

// MoonPhase returns moon illumination (0-1) and phase name
func MoonPhase(t time.Time) (illumination float64, phase string) {
	// Simplified moon phase from Julian date
	y, m, d := t.Date()
	if m <= 2 {
		y--
		m += 12
	}
	jd := float64(int(365.25*float64(y+4716))) + float64(int(30.6001*float64(m+1))) + float64(d) - 1524.5

	// Days since known new moon (Jan 6, 2000)
	daysSinceNew := jd - 2451550.1
	cycles := daysSinceNew / 29.530588853
	phase_frac := cycles - math.Floor(cycles)

	// Illumination (0 at new, 1 at full)
	illumination = 0.5 * (1 - math.Cos(2*math.Pi*phase_frac))

	switch {
	case phase_frac < 0.0625:
		phase = "New Moon"
	case phase_frac < 0.1875:
		phase = "Waxing Crescent"
	case phase_frac < 0.3125:
		phase = "First Quarter"
	case phase_frac < 0.4375:
		phase = "Waxing Gibbous"
	case phase_frac < 0.5625:
		phase = "Full Moon"
	case phase_frac < 0.6875:
		phase = "Waning Gibbous"
	case phase_frac < 0.8125:
		phase = "Last Quarter"
	case phase_frac < 0.9375:
		phase = "Waning Crescent"
	default:
		phase = "New Moon"
	}
	return
}

// SQMToBortle converts Sky Quality Meter reading to Bortle class
func SQMToBortle(sqm float64) int {
	switch {
	case sqm >= 21.99:
		return 1
	case sqm >= 21.89:
		return 2
	case sqm >= 21.69:
		return 3
	case sqm >= 20.49:
		return 4
	case sqm >= 19.50:
		return 5
	case sqm >= 18.94:
		return 6
	case sqm >= 18.38:
		return 7
	case sqm >= 17.80:
		return 8
	default:
		return 9
	}
}

// BortleDescription returns human-readable Bortle class description
func BortleDescription(bortle int) string {
	descriptions := map[int]string{
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
	if d, ok := descriptions[bortle]; ok {
		return d
	}
	return "Unknown"
}

// SunTimes holds calculated sun event times for a given date and location
type SunTimes struct {
	Sunrise          time.Time `json:"sunrise"`
	Sunset           time.Time `json:"sunset"`
	CivilDawn        time.Time `json:"civilDawn"`
	CivilDusk        time.Time `json:"civilDusk"`
	NauticalDawn     time.Time `json:"nauticalDawn"`
	NauticalDusk     time.Time `json:"nauticalDusk"`
	AstronomicalDawn time.Time `json:"astronomicalDawn"`
	AstronomicalDusk time.Time `json:"astronomicalDusk"`
}

// CalculateSunTimes computes sunrise/sunset and twilight times
func CalculateSunTimes(date time.Time, lat, lon float64) SunTimes {
	times := SunTimes{}

	// Use Julian day number from Unix timestamp (simple and correct)
	date = date.UTC()
	jd := float64(date.Unix())/86400.0 + 2440587.5

	n := jd - 2451545.0 // days since J2000.0

	// Mean solar noon
	jStar := n - lon/360.0

	// Solar mean anomaly
	M := math.Mod(357.5291+0.98560028*jStar, 360.0)
	Mrad := M * math.Pi / 180.0

	// Equation of center
	C := 1.9148*math.Sin(Mrad) + 0.0200*math.Sin(2*Mrad) + 0.0003*math.Sin(3*Mrad)

	// Ecliptic longitude
	lambda := math.Mod(M+C+180.0+102.9372, 360.0)
	lambdaRad := lambda * math.Pi / 180.0

	// Declination
	sinDec := math.Sin(lambdaRad) * math.Sin(23.4393*math.Pi/180.0)
	dec := math.Asin(sinDec)

	latRad := lat * math.Pi / 180.0

	calcTime := func(angle float64) (time.Time, time.Time) {
		cosHA := (math.Sin(angle*math.Pi/180.0) - math.Sin(latRad)*math.Sin(dec)) / (math.Cos(latRad) * math.Cos(dec))
		if cosHA < -1 || cosHA > 1 {
			return date, date // no rise/set
		}
		HA := math.Acos(cosHA) * 180.0 / math.Pi

		// Transit
		jTransit := 2451545.0 + jStar + 0.0053*math.Sin(Mrad) - 0.0069*math.Sin(2*lambdaRad)

		rise := jTransit - HA/360.0
		set := jTransit + HA/360.0

		riseTime := julianToTime(rise, date.Location())
		setTime := julianToTime(set, date.Location())
		return riseTime, setTime
	}

	times.Sunrise, times.Sunset = calcTime(-0.833)
	times.CivilDawn, times.CivilDusk = calcTime(-6.0)
	times.NauticalDawn, times.NauticalDusk = calcTime(-12.0)
	times.AstronomicalDawn, times.AstronomicalDusk = calcTime(-18.0)

	return times
}

func julianToTime(jd float64, loc *time.Location) time.Time {
	// Convert Julian date to Unix timestamp
	unix := (jd - 2440587.5) * 86400.0
	return time.Unix(int64(unix), 0).In(loc)
}
