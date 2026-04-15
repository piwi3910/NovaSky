package weather

import (
	"fmt"
	"net/http"
	"strconv"
)

// ParseEcowittRequest parses an Ecowitt-compatible weather station HTTP POST.
// Ecowitt pushes form-encoded data to a configurable URL at regular intervals.
// Values arrive in imperial units and are converted to metric.
func ParseEcowittRequest(r *http.Request) (*WeatherData, error) {
	if err := r.ParseForm(); err != nil {
		return nil, fmt.Errorf("ecowitt form parse error: %w", err)
	}

	data := &WeatherData{}

	// Temperature: Fahrenheit -> Celsius
	if v, err := parseFloat(r.FormValue("tempf")); err == nil {
		data.Temperature = (v - 32.0) * 5.0 / 9.0
	}

	// Humidity: already percentage
	if v, err := parseFloat(r.FormValue("humidity")); err == nil {
		data.Humidity = v
	}

	// Wind speed: mph -> km/h
	if v, err := parseFloat(r.FormValue("windspeedmph")); err == nil {
		data.WindSpeed = v * 1.60934
	}

	// Wind gusts: mph -> km/h
	if v, err := parseFloat(r.FormValue("windgustmph")); err == nil {
		data.WindGusts = v * 1.60934
	}

	// Dew point: Fahrenheit -> Celsius
	if v, err := parseFloat(r.FormValue("dewptf")); err == nil {
		data.DewPoint = (v - 32.0) * 5.0 / 9.0
	}

	// Barometric pressure: inHg -> hPa
	if v, err := parseFloat(r.FormValue("baromrelin")); err == nil {
		data.Pressure = v * 33.8639
	}

	return data, nil
}

func parseFloat(s string) (float64, error) {
	if s == "" {
		return 0, fmt.Errorf("empty value")
	}
	return strconv.ParseFloat(s, 64)
}
