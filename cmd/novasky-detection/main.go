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
	"github.com/piwi3910/NovaSky/internal/platesolve"
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

				// Read FITS and analyze
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

				// Night frame analysis
				if brightness < 0.3 {
					header := fits.ParseHeader(data)
					imageSize := header.NAXIS1
					if imageSize <= 0 {
						imageSize = 1304
					}

					// Star detection
					if detCfg.StarsEnabled {
						pixels := fits.ReadPixels16(data, header)
						if len(pixels) > 0 && header.NAXIS1 > 0 && header.NAXIS2 > 0 {
							mat, err := gocv.NewMatFromBytes(header.NAXIS2, header.NAXIS1, gocv.MatTypeCV16UC1, uint16ToBytes(pixels))
							if err == nil {
								stars := detection.DetectStars(mat, detCfg.StarMinBrightness)
								mat.Close()
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

							if detCfg.ConstellationsEnabled {
								projected := detection.ProjectConstellations(lst, loc.Latitude, imageSize)
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

				// Plate solve (once, or periodically)
				if detCfg.PlateSolveEnabled && platesolve.GetCachedWCS() == nil {
					go func(fp string) {
						log.Printf("[detection] Attempting plate solve on %s", fp)
						wcs, err := platesolve.Solve(fp, 180)
						if err != nil {
							log.Printf("[detection] Plate solve failed: %v", err)
							return
						}
						if wcs.Solved {
							platesolve.CacheWCS(wcs)
							log.Printf("[detection] Plate solved! RA=%.4f Dec=%.4f", wcs.CRVAL1, wcs.CRVAL2)
						}
					}(filePath)
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

	// Cloud detection heuristic:
	// At night: bright + low contrast = clouds. Dark + high contrast = clear.
	// During day: always bright, use contrast ratio.
	// Normalize: contrast below 0.02 suggests uniform (cloudy), above 0.05 suggests structure (clear)
	contrastScore := math.Min(1.0, stddev/0.05) // 0=uniform, 1=high contrast

	if brightness > 0.4 {
		// Daytime or very bright — cloud cover from inverse contrast
		cloudCover = math.Max(0, 1.0-contrastScore)
	} else {
		// Night — combine brightness and contrast
		// Bright + uniform = clouds. Dark + varied = clear.
		brightnessScore := math.Min(1.0, brightness/0.15) // 0=dark, 1=bright
		cloudCover = brightnessScore * (1.0 - contrastScore*0.5)
	}
	cloudCover = math.Max(0, math.Min(1.0, cloudCover))
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
