package weather

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const openMeteoURL = "https://api.open-meteo.com/v1/forecast"

// openMeteoResponse represents the JSON response from Open-Meteo API.
type openMeteoResponse struct {
	Current struct {
		Temperature  float64 `json:"temperature_2m"`
		Humidity     float64 `json:"relative_humidity_2m"`
		WindSpeed    float64 `json:"wind_speed_10m"`
		WindGusts    float64 `json:"wind_gusts_10m"`
		CloudCover   float64 `json:"cloud_cover"`
		DewPoint     float64 `json:"dew_point_2m"`
		PressureMSL  float64 `json:"pressure_msl"`
	} `json:"current"`
}

// FetchOpenMeteo fetches current weather data from the Open-Meteo API (free, no key required).
func FetchOpenMeteo(lat, lon float64) (*WeatherData, error) {
	url := fmt.Sprintf(
		"%s?latitude=%.4f&longitude=%.4f&current=temperature_2m,relative_humidity_2m,wind_speed_10m,wind_gusts_10m,cloud_cover,dew_point_2m,pressure_msl",
		openMeteoURL, lat, lon,
	)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("open-meteo request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("open-meteo returned status %d: %s", resp.StatusCode, string(body))
	}

	var result openMeteoResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("open-meteo decode error: %w", err)
	}

	return &WeatherData{
		Temperature: result.Current.Temperature,
		Humidity:    result.Current.Humidity,
		WindSpeed:   result.Current.WindSpeed,
		WindGusts:   result.Current.WindGusts,
		CloudCover:  result.Current.CloudCover,
		DewPoint:    result.Current.DewPoint,
		Pressure:    result.Current.PressureMSL,
	}, nil
}
