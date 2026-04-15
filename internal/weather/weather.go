package weather

import "fmt"

// WeatherData holds current weather conditions from any provider.
type WeatherData struct {
	Temperature float64 // °C
	Humidity    float64 // %
	WindSpeed   float64 // km/h
	WindGusts   float64 // km/h
	CloudCover  float64 // %
	DewPoint    float64 // °C
	Pressure    float64 // hPa
}

// FetchWeather dispatches to the configured weather provider and returns current conditions.
func FetchWeather(source string, lat, lon float64) (*WeatherData, error) {
	switch source {
	case "openmeteo":
		return FetchOpenMeteo(lat, lon)
	case "ecowitt":
		// Ecowitt is push-based; this should not be called directly.
		return nil, fmt.Errorf("ecowitt is push-based, use the HTTP listener instead")
	case "none", "":
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown weather source: %s", source)
	}
}
