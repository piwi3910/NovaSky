package platesolve

import (
	"fmt"
	"log"
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

var (
	cachedWCS *WCS
	wcsMu     sync.RWMutex
)

// Solve runs ASTAP plate solver on a FITS file and returns the WCS solution.
func Solve(fitsPath string, searchRadius float64) (*WCS, error) {
	if searchRadius <= 0 {
		searchRadius = 180 // full sky search for all-sky cameras
	}

	cmd := exec.Command("astap_cli",
		"-f", fitsPath,
		"-r", fmt.Sprintf("%.1f", searchRadius),
		"-z", "0", // downsample
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("astap_cli failed: %w\nOutput: %s", err, string(output))
	}

	// Parse the .wcs file that ASTAP creates alongside the FITS
	wcsPath := strings.TrimSuffix(fitsPath, ".fits") + ".wcs"
	return parseWCSFile(wcsPath)
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
