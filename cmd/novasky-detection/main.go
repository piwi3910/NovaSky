package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"gocv.io/x/gocv"

	"github.com/piwi3910/NovaSky/internal/astronomy"
	"github.com/piwi3910/NovaSky/internal/config"
	"github.com/piwi3910/NovaSky/internal/db"
	"github.com/piwi3910/NovaSky/internal/detection"
	"github.com/piwi3910/NovaSky/internal/fits"
	"github.com/piwi3910/NovaSky/internal/models"
	"github.com/piwi3910/NovaSky/internal/nightlysummary"
	novaskyRedis "github.com/piwi3910/NovaSky/internal/redis"
)

const consumerGroup = "detection"

func main() {
	log.Println("[detection] Starting...")
	db.Init()
	novaskyRedis.Init()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { c := make(chan os.Signal, 1); signal.Notify(c, syscall.SIGINT, syscall.SIGTERM); <-c; cancel() }()

	cfg := config.NewManager()
	cfg.Subscribe(ctx)

	novaskyRedis.StartHealthReporter(ctx, "detection")

	novaskyRedis.CreateConsumerGroup(ctx, novaskyRedis.StreamFramesDetection, consumerGroup)
	log.Println("[detection] Worker started")

	// Nightly summary generation at dawn
	go func() {
		for {
			now := time.Now()
			// Run at 6 AM local time
			next := time.Date(now.Year(), now.Month(), now.Day(), 6, 0, 0, 0, now.Location())
			if now.After(next) {
				next = next.AddDate(0, 0, 1)
			}
			time.Sleep(time.Until(next))
			nightlysummary.GenerateYesterday()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		streams, err := novaskyRedis.ReadFromGroup(ctx, novaskyRedis.StreamFramesDetection, consumerGroup, "detection-1", 1)
		if err != nil {
			continue
		}

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				frameID := msg.Values["frameId"].(string)
				filePath := msg.Values["filePath"].(string)
				jpegPath := ""
				if jp, ok := msg.Values["jpegPath"].(string); ok {
					jpegPath = jp
				}

				// Read FITS for cloud/brightness analysis
				data, err := os.ReadFile(filePath)
				if err != nil {
					log.Printf("[detection] Read error: %v", err)
					novaskyRedis.AckMessage(ctx, novaskyRedis.StreamFramesDetection, consumerGroup, msg.ID)
					continue
				}

				// Read detection config
				var detCfg struct {
					CloudEnabled         bool    `json:"cloudEnabled"`
					SqmEnabled           bool    `json:"sqmEnabled"`
					StarsEnabled         bool    `json:"starsEnabled"`
					StarMinBrightness    float64 `json:"starMinBrightness"`
					MeteorsEnabled       bool    `json:"meteorsEnabled"`
					PlanesEnabled        bool    `json:"planesEnabled"`
					PlanesURL            string  `json:"planesUrl"`
					SatellitesEnabled    bool    `json:"satellitesEnabled"`
					ConstellationsEnabled bool   `json:"constellationsEnabled"`
					PlanetsEnabled       bool    `json:"planetsEnabled"`
					PlateSolveEnabled    bool    `json:"plateSolveEnabled"`
				}
				cfg.Get("detection", &detCfg)
				// Defaults: everything on if not configured
				if !detCfg.CloudEnabled && !detCfg.StarsEnabled && !detCfg.MeteorsEnabled {
					// Not configured yet — enable defaults
					detCfg.CloudEnabled = true
					detCfg.SqmEnabled = true
					detCfg.StarsEnabled = true
					detCfg.ConstellationsEnabled = true
					detCfg.PlanetsEnabled = true
					detCfg.PlateSolveEnabled = true
				}
				if detCfg.StarMinBrightness <= 0 {
					detCfg.StarMinBrightness = 200
				}

				startTime := time.Now()
				brightness, cloudCover := analyzeFrame(data)
				skyQuality := classifyQuality(cloudCover)

				var sqm *float64
				if detCfg.SqmEnabled && brightness > 0 && brightness < 0.3 {
					sqmVal := -2.5*math.Log10(brightness) + 20.0
					sqm = &sqmVal
				}

				result := models.AnalysisResult{
					FrameID:    frameID,
					CloudCover: cloudCover,
					Brightness: brightness,
					SkyQuality: skyQuality,
					SQM:        sqm,
				}
				db.GetDB().Create(&result)

				// Night frame analysis — use processed JPEG (already rotated/stretched)
				if brightness < 0.3 {
					// Get image size from JPEG for constellation projection
					imageSize := 3552
					if jpegPath != "" {
						jpegMat := gocv.IMRead(jpegPath, gocv.IMReadGrayScale)
						if !jpegMat.Empty() {
							imageSize = jpegMat.Cols()

							// Star detection on processed JPEG — coords match displayed image
							if detCfg.StarsEnabled {
								stars := detection.DetectStars(jpegMat, detCfg.StarMinBrightness)

								// Filter out stars in the masked area (outside mask circle)
								var maskCfg struct {
									Enabled bool `json:"enabled"`
									CenterX int  `json:"centerX"`
									CenterY int  `json:"centerY"`
									Radius  int  `json:"radius"`
								}
								cfg.Get("imaging.mask", &maskCfg)
								if maskCfg.Enabled && maskCfg.Radius > 0 {
									var filtered []detection.Star
									cx := float64(maskCfg.CenterX)
									cy := float64(maskCfg.CenterY)
									r := float64(maskCfg.Radius)
									for _, s := range stars {
										dx := s.X - cx
										dy := s.Y - cy
										if dx*dx+dy*dy <= r*r {
											filtered = append(filtered, s)
										}
									}
									log.Printf("[detection] Mask filter: %d → %d stars", len(stars), len(filtered))
									stars = filtered
								}

								if len(stars) > 0 {
									starsJSON, _ := json.Marshal(stars)
									db.GetDB().Create(&models.Detection{
										FrameID: frameID,
										Type:    "stars",
										Data:    starsJSON,
									})
									log.Printf("[detection] Found %d stars", len(stars))
								}
							}
							jpegMat.Close()
						}
					}

					// Constellation projection and planet positions
					if detCfg.ConstellationsEnabled || detCfg.PlanetsEnabled {
						var loc struct {
							Latitude  float64 `json:"latitude"`
							Longitude float64 `json:"longitude"`
						}
						cfg.Get("location", &loc)
						if loc.Latitude != 0 || loc.Longitude != 0 {
							now := time.Now()
							jd := float64(now.UTC().Unix())/86400.0 + 2440587.5
							T := (jd - 2451545.0) / 36525.0
							gmst := math.Mod(280.46061837+360.98564736629*(jd-2451545.0)+0.000387933*T*T, 360.0)
							if gmst < 0 {
								gmst += 360.0
							}
							lst := math.Mod(gmst+loc.Longitude, 360.0) / 15.0

							// Get North rotation from calibration
							var calCfg struct {
								NorthAngle float64 `json:"northAngle"`
								Solved     bool    `json:"solved"`
							}
							cfg.Get("camera.calibration", &calCfg)
							northAngle := 0.0
							if calCfg.Solved {
								northAngle = calCfg.NorthAngle
							}

							if detCfg.ConstellationsEnabled {
								projected := detection.ProjectConstellations(lst, loc.Latitude, imageSize, northAngle)
								if len(projected) > 0 {
									constJSON, _ := json.Marshal(projected)
									db.GetDB().Create(&models.Detection{
										FrameID: frameID,
										Type:    "constellations",
										Data:    constJSON,
									})
									log.Printf("[detection] Projected %d constellations", len(projected))
								}
							}

							if detCfg.PlanetsEnabled {
								planets := astronomy.PlanetPositions(now, loc.Latitude, loc.Longitude)
								var visible []astronomy.PlanetPosition
								for _, p := range planets {
									if p.Visible {
										visible = append(visible, p)
									}
								}
								if len(visible) > 0 {
									planetsJSON, _ := json.Marshal(visible)
									db.GetDB().Create(&models.Detection{
										FrameID: frameID,
										Type:    "planets",
										Data:    planetsJSON,
									})
									log.Printf("[detection] %d planets visible", len(visible))
								}
							}
						}
					}
				}

				// Fetch TLE data periodically (non-blocking)
				if detCfg.SatellitesEnabled {
					go detection.FetchTLEs() //nolint:errcheck
				}

				// Trigger policy evaluation
				novaskyRedis.PublishToStream(ctx, novaskyRedis.StreamPolicyEvaluate, map[string]interface{}{
					"trigger": "detection", "sourceId": result.ID,
				})

				novaskyRedis.AckMessage(ctx, novaskyRedis.StreamFramesDetection, consumerGroup, msg.ID)
				novaskyRedis.ReportHealth(ctx, "detection")

				elapsed := time.Since(startTime)
				novaskyRedis.Client.Set(ctx, "novasky:latency:detection", fmt.Sprintf("%.3f", elapsed.Seconds()), 0)
				log.Printf("[detection] Frame %s: cloud=%.0f%% sky=%s (%.1fs)", frameID, cloudCover*100, skyQuality, elapsed.Seconds())
			}
		}
	}
}

func uint16ToBytes(data []uint16) []byte {
	buf := make([]byte, len(data)*2)
	for i, v := range data {
		buf[i*2] = byte(v)
		buf[i*2+1] = byte(v >> 8)
	}
	return buf
}

func analyzeFrame(fitsData []byte) (brightness, cloudCover float64) {
	// Use shared FITS parser (handles BZERO correctly)
	header := fits.ParseHeader(fitsData)
	pixels := fits.ReadPixels16(fitsData, header)
	if len(pixels) == 0 {
		return 0, 0
	}

	// Sample pixels for speed
	step := 1
	if len(pixels) > 50000 {
		step = len(pixels) / 50000
	}

	var sampled []uint16
	for i := 0; i < len(pixels); i += step {
		sampled = append(sampled, pixels[i])
	}

	sort.Slice(sampled, func(i, j int) bool { return sampled[i] < sampled[j] })
	median := float64(sampled[len(sampled)/2])
	brightness = median / 65535.0

	// Compute standard deviation to distinguish clouds from clear sky
	// Clouds scatter light → higher brightness + lower contrast (low stddev)
	// Clear sky at night → low brightness + higher contrast (stars = high stddev)
	var sum, sumSq float64
	for _, v := range sampled {
		f := float64(v) / 65535.0
		sum += f
		sumSq += f * f
	}
	n := float64(len(sampled))
	mean := sum / n
	variance := sumSq/n - mean*mean
	stddev := math.Sqrt(math.Max(0, variance))

	// Cloud detection using histogram spread.
	// Auto-exposure normalizes brightness, so median is NOT a cloud indicator.
	// Instead use the distribution shape:
	// - Clear night sky: bimodal (dark sky + bright stars) → high IQR relative to median
	// - Cloudy night sky: unimodal (uniform glow) → low IQR relative to median
	// - Clear day: bright with texture → moderate stddev
	// - Cloudy day: bright and flat → low stddev
	p25 := float64(sampled[len(sampled)/4]) / 65535.0
	p75 := float64(sampled[3*len(sampled)/4]) / 65535.0
	iqr := p75 - p25

	// Relative spread: IQR / mean — independent of auto-exposure level
	relativeSpread := 0.0
	if mean > 0.001 {
		relativeSpread = iqr / mean
	}

	// Higher relative spread = more structure = clearer sky
	// Typical values: clear night 0.3-1.0+, cloudy night 0.02-0.1, clear day 0.1-0.3, cloudy day 0.01-0.05
	// Map: spread > 0.2 = clear, spread < 0.05 = fully cloudy
	cloudCover = 1.0 - math.Min(1.0, math.Max(0, (relativeSpread-0.05)/0.20))

	log.Printf("[detection] brightness=%.3f mean=%.3f stddev=%.4f iqr=%.4f relSpread=%.3f cloud=%.0f%%",
		brightness, mean, stddev, iqr, relativeSpread, cloudCover*100)
	return
}

func classifyQuality(cloudCover float64) string {
	if cloudCover > 0.8 {
		return "UNUSABLE"
	}
	if cloudCover > 0.5 {
		return "POOR"
	}
	if cloudCover > 0.2 {
		return "GOOD"
	}
	return "EXCELLENT"
}
