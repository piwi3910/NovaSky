package platesolve

import (
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

// WCS holds the World Coordinate System solution
type WCS struct {
	CRVAL1 float64 // RA of reference pixel (degrees)
	CRVAL2 float64 // Dec of reference pixel (degrees)
	CRPIX1 float64 // Reference pixel X
	CRPIX2 float64 // Reference pixel Y
	CD1_1  float64 // CD matrix element
	CD1_2  float64
	CD2_1  float64
	CD2_2  float64
	Solved bool
}

// Calibration holds the camera orientation result from plate solving
type Calibration struct {
	NorthAngle float64 `json:"northAngle"` // degrees clockwise from image-up to North
	RA         float64 `json:"ra"`         // RA of image center (degrees)
	Dec        float64 `json:"dec"`        // Dec of image center (degrees)
	PixelScale float64 `json:"pixelScale"` // arcsec/pixel
	Solved     bool    `json:"solved"`
}

var (
	cachedWCS *WCS
	wcsMu     sync.RWMutex
)

// Solve runs ASTAP plate solver on a FITS file and returns the WCS solution.
// fov is the field of view in degrees (0 = let ASTAP guess).
func Solve(fitsPath string, searchRadius float64, fov float64) (*WCS, error) {
	if searchRadius <= 0 {
		searchRadius = 180
	}

	args := []string{
		"-f", fitsPath,
		"-r", fmt.Sprintf("%.1f", searchRadius),
		"-d", "/opt/astap",
		"-z", "0",
	}
	if fov > 0 {
		args = append(args, "-fov", fmt.Sprintf("%.1f", fov))
	}

	cmd := exec.Command("astap_cli", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("astap_cli failed: %w\nOutput: %s", err, string(output))
	}

	wcsPath := strings.TrimSuffix(fitsPath, ".fits") + ".wcs"
	return parseWCSFile(wcsPath)
}

// CalcFoV computes the field of view in degrees from focal length and sensor size.
// pixelSizeUm is the pixel size in micrometers, imageWidth is pixels across.
func CalcFoV(focalLengthMm float64, pixelSizeUm float64, imageWidth int) float64 {
	if focalLengthMm <= 0 || pixelSizeUm <= 0 || imageWidth <= 0 {
		return 0
	}
	// pixel scale in arcsec/pixel
	pixelScale := 206.265 * pixelSizeUm / focalLengthMm
	// FoV in degrees
	return pixelScale * float64(imageWidth) / 3600.0
}

// CalcNorthAngle extracts the rotation angle from the CD matrix.
// Returns degrees clockwise from image-up to celestial North.
func CalcNorthAngle(wcs *WCS) float64 {
	if !wcs.Solved {
		return 0
	}
	// North angle from CD matrix: angle of the Dec axis relative to image Y
	// atan2(CD2_1, CD2_2) gives the rotation of North from the Y-axis
	angle := math.Atan2(wcs.CD2_1, wcs.CD2_2) * 180.0 / math.Pi
	return angle
}

// CalcPixelScale returns the pixel scale in arcsec/pixel from the CD matrix.
func CalcPixelScale(wcs *WCS) float64 {
	if !wcs.Solved {
		return 0
	}
	// pixel scale = sqrt(CD1_1^2 + CD2_1^2) * 3600
	return math.Sqrt(wcs.CD1_1*wcs.CD1_1+wcs.CD2_1*wcs.CD2_1) * 3600.0
}

// CalibrateFunc is the signature for the log callback
type LogFunc func(msg string)

// Calibrate runs plate solving and returns the camera orientation calibration.
// It uses the JPEG (debayered) image and crops the center 20% to avoid fisheye
// distortion and to fit within the D05 database FoV limit (<20°).
// fullFov is the full-frame field of view in degrees.
func Calibrate(imagePath string, fullFov float64, logFn LogFunc) (*Calibration, error) {
	if logFn == nil {
		logFn = func(msg string) { log.Println(msg) }
	}

	// Crop ratio: use center 20% of the image
	// This reduces FoV proportionally: cropFov = fullFov * 0.20
	cropRatio := 0.20
	cropFov := fullFov * cropRatio

	// If crop FoV is still too large for D05, reduce further
	if cropFov > 18 {
		cropRatio = 18.0 / fullFov
		cropFov = 18.0
	}

	logFn(fmt.Sprintf("Full FoV: %.1f° → cropping center %.0f%% → crop FoV: %.1f°", fullFov, cropRatio*100, cropFov))

	// Use ImageMagick/convert to crop center of the image
	// This works on both FITS and JPEG
	cropPercent := int(cropRatio * 100)
	croppedPath := strings.TrimSuffix(imagePath, ".fits")
	croppedPath = strings.TrimSuffix(croppedPath, ".jpg") + "_crop.jpg"

	// Use convert (ImageMagick) to crop center
	cropGeom := fmt.Sprintf("%d%%x%d%%+0+0", cropPercent, cropPercent)
	cmd := exec.Command("convert", imagePath, "-gravity", "center", "-crop", cropGeom, "+repage", croppedPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Try with 'magick' (ImageMagick v7)
		cmd = exec.Command("magick", imagePath, "-gravity", "center", "-crop", cropGeom, "+repage", croppedPath)
		output, err = cmd.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("failed to crop image: %w\nOutput: %s", err, string(output))
		}
	}
	defer os.Remove(croppedPath)

	logFn(fmt.Sprintf("Cropped center to %s (%.1f° FoV)", croppedPath, cropFov))
	logFn("Running ASTAP plate solver on cropped image...")

	wcs, err := Solve(croppedPath, 180, cropFov)
	if err != nil {
		return nil, err
	}
	if !wcs.Solved {
		return &Calibration{Solved: false}, nil
	}

	cal := &Calibration{
		NorthAngle: CalcNorthAngle(wcs),
		RA:         wcs.CRVAL1,
		Dec:        wcs.CRVAL2,
		PixelScale: CalcPixelScale(wcs),
		Solved:     true,
	}

	CacheWCS(wcs)
	log.Printf("[platesolve] Calibration: North=%.1f° RA=%.4f Dec=%.4f scale=%.2f\"/px",
		cal.NorthAngle, cal.RA, cal.Dec, cal.PixelScale)
	return cal, nil
}

// GetCachedWCS returns the most recent plate solve result
func GetCachedWCS() *WCS {
	wcsMu.RLock()
	defer wcsMu.RUnlock()
	return cachedWCS
}

// CacheWCS stores a plate solve result for reuse
func CacheWCS(wcs *WCS) {
	wcsMu.Lock()
	defer wcsMu.Unlock()
	cachedWCS = wcs
	log.Printf("[platesolve] WCS cached: RA=%.4f Dec=%.4f", wcs.CRVAL1, wcs.CRVAL2)
}

func parseWCSFile(path string) (*WCS, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("WCS file not found: %w", err)
	}

	wcs := &WCS{}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if len(line) < 10 || line[8] != '=' {
			continue
		}
		key := strings.TrimSpace(line[:8])
		valStr := strings.TrimSpace(strings.Split(line[10:], "/")[0])
		valStr = strings.Trim(valStr, "' ")

		val, _ := strconv.ParseFloat(valStr, 64)

		switch key {
		case "CRVAL1":
			wcs.CRVAL1 = val
		case "CRVAL2":
			wcs.CRVAL2 = val
		case "CRPIX1":
			wcs.CRPIX1 = val
		case "CRPIX2":
			wcs.CRPIX2 = val
		case "CD1_1":
			wcs.CD1_1 = val
		case "CD1_2":
			wcs.CD1_2 = val
		case "CD2_1":
			wcs.CD2_1 = val
		case "CD2_2":
			wcs.CD2_2 = val
		}
	}

	wcs.Solved = wcs.CRVAL1 != 0 || wcs.CRVAL2 != 0
	return wcs, nil
}

// PixelToRaDec converts pixel coordinates to RA/Dec using the WCS solution
func (w *WCS) PixelToRaDec(x, y float64) (ra, dec float64) {
	if !w.Solved {
		return 0, 0
	}
	dx := x - w.CRPIX1
	dy := y - w.CRPIX2
	ra = w.CRVAL1 + w.CD1_1*dx + w.CD1_2*dy
	dec = w.CRVAL2 + w.CD2_1*dx + w.CD2_2*dy
	return
}

// RaDecToPixel converts RA/Dec to pixel coordinates using the WCS solution
func (w *WCS) RaDecToPixel(ra, dec float64) (x, y float64) {
	if !w.Solved {
		return 0, 0
	}
	// Inverse of the CD matrix
	det := w.CD1_1*w.CD2_2 - w.CD1_2*w.CD2_1
	if det == 0 {
		return 0, 0
	}
	dra := ra - w.CRVAL1
	ddec := dec - w.CRVAL2
	x = w.CRPIX1 + (w.CD2_2*dra-w.CD1_2*ddec)/det
	y = w.CRPIX2 + (-w.CD2_1*dra+w.CD1_1*ddec)/det
	return
}
