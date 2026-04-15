package processing

import (
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/piwi3910/NovaSky/internal/fits"
)

// ProcessResult contains the output of frame processing
type ProcessResult struct {
	JpegPath string
}

// ProcessFrame debayers, stretches, and saves a JPEG from a raw FITS file
func ProcessFrame(fitsPath string, stretch string, maskCfg *MaskConfig) (*ProcessResult, error) {
	// Read FITS
	data, err := os.ReadFile(fitsPath)
	if err != nil {
		return nil, err
	}

	// Parse FITS header using shared package
	header := fits.ParseHeader(data)
	width := header.NAXIS1
	height := header.NAXIS2
	bayerPat := header.BayerPat

	if width == 0 || height == 0 {
		return nil, fmt.Errorf("invalid FITS dimensions: %dx%d", width, height)
	}

	// Read raw pixel values as uint16 (applying BZERO for proper unsigned interpretation)
	pixels := fits.ReadPixels16(data, header)

	// Debayer
	var img *image.RGBA
	if bayerPat != "" {
		img = debayer(pixels, width, height, bayerPat)
	} else {
		img = monoToRGBA(pixels, width, height)
	}

	// Apply mask
	if maskCfg != nil && maskCfg.Enabled {
		applyMask(img, maskCfg.CenterX, maskCfg.CenterY, maskCfg.Radius)
	}

	// Apply stretch
	applyStretch(img, stretch)

	// Save JPEG
	jpegPath := strings.TrimSuffix(fitsPath, filepath.Ext(fitsPath)) + ".jpg"
	f, err := os.Create(jpegPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	err = jpeg.Encode(f, img, &jpeg.Options{Quality: 90})
	if err != nil {
		return nil, err
	}

	return &ProcessResult{JpegPath: jpegPath}, nil
}

// MaskConfig defines a circular mask to apply to processed images
type MaskConfig struct {
	CenterX int  `json:"centerX"`
	CenterY int  `json:"centerY"`
	Radius  int  `json:"radius"`
	Enabled bool `json:"enabled"`
}

// debayer implements simple bilinear Bayer demosaicing
func debayer(raw []uint16, width, height int, pattern string) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Bayer pattern mapping: FITS BAYERPAT → Go debayer positions.
	// Verified empirically by testing all 4 OpenCV BayerXX2RGB codes against
	// actual camera data (ASI676MC). The correct mapping for FITS conventions:
	//
	// FITS BAYERPAT → OpenCV equivalent → Go positions
	// RGGB → BayerGB2RGB → R at (0,1), B at (1,0)
	// BGGR → BayerRG2RGB → R at (1,0), B at (0,1)
	// GRBG → BayerGR2RGB → R at (1,1), B at (0,0)
	// GBRG → BayerBG2RGB → R at (0,0), B at (1,1)
	var redX, redY, blueX, blueY int
	switch pattern {
	case "RGGB":
		redX, redY = 0, 1
		blueX, blueY = 1, 0
	case "BGGR":
		redX, redY = 1, 0
		blueX, blueY = 0, 1
	case "GRBG":
		redX, redY = 1, 1
		blueX, blueY = 0, 0
	case "GBRG":
		redX, redY = 0, 0
		blueX, blueY = 1, 1
	default:
		// Unknown pattern, try to detect from data (Green pair detection is reliable)
		redX, redY, blueX, blueY = detectBayerLayout(raw, width, height, pattern)
	}

	isRed := func(x, y int) bool {
		return x%2 == redX && y%2 == redY
	}
	isBlue := func(x, y int) bool {
		return x%2 == blueX && y%2 == blueY
	}

	getPixel := func(x, y int) uint16 {
		if x < 0 {
			x = 0
		}
		if y < 0 {
			y = 0
		}
		if x >= width {
			x = width - 1
		}
		if y >= height {
			y = height - 1
		}
		return raw[y*width+x]
	}

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			var r, g, b uint16
			val := getPixel(x, y)

			if isRed(x, y) {
				r = val
				g = (getPixel(x-1, y) + getPixel(x+1, y) + getPixel(x, y-1) + getPixel(x, y+1)) / 4
				b = (getPixel(x-1, y-1) + getPixel(x+1, y-1) + getPixel(x-1, y+1) + getPixel(x+1, y+1)) / 4
			} else if isBlue(x, y) {
				b = val
				g = (getPixel(x-1, y) + getPixel(x+1, y) + getPixel(x, y-1) + getPixel(x, y+1)) / 4
				r = (getPixel(x-1, y-1) + getPixel(x+1, y-1) + getPixel(x-1, y+1) + getPixel(x+1, y+1)) / 4
			} else {
				g = val
				if isRed(x-1, y) || isRed(x+1, y) {
					r = (getPixel(x-1, y) + getPixel(x+1, y)) / 2
					b = (getPixel(x, y-1) + getPixel(x, y+1)) / 2
				} else {
					b = (getPixel(x-1, y) + getPixel(x+1, y)) / 2
					r = (getPixel(x, y-1) + getPixel(x, y+1)) / 2
				}
			}

			// Store as 16-bit color (using RGBA64 would be ideal but we convert at the end)
			// For "none" stretch: map based on actual data range
			// For now, simple >> 8 — stretch function handles the rest
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(r >> 8),
				G: uint8(g >> 8),
				B: uint8(b >> 8),
				A: 255,
			})
		}
	}
	return img
}

func monoToRGBA(raw []uint16, width, height int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			v := uint8(raw[y*width+x] >> 8)
			img.SetRGBA(x, y, color.RGBA{R: v, G: v, B: v, A: 255})
		}
	}
	return img
}

func applyStretch(img *image.RGBA, mode string) {
	if mode == "none" || mode == "" {
		return // Already linear 16→8 bit from debayer
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	if mode == "linear" {
		// Collect all pixel values for percentile calculation
		var allVals []uint8
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				r, g, b, _ := img.At(x, y).RGBA()
				allVals = append(allVals, uint8(r>>8), uint8(g>>8), uint8(b>>8))
			}
		}
		sort.Slice(allVals, func(i, j int) bool { return allVals[i] < allVals[j] })
		p5 := float64(allVals[len(allVals)*5/100])
		p95 := float64(allVals[len(allVals)*95/100])
		if p95 <= p5 {
			return
		}

		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				c := img.RGBAAt(x, y)
				c.R = stretchVal(c.R, p5, p95)
				c.G = stretchVal(c.G, p5, p95)
				c.B = stretchVal(c.B, p5, p95)
				img.SetRGBA(x, y, c)
			}
		}
	} else if mode == "auto" {
		// Per-channel stretch
		for ch := 0; ch < 3; ch++ {
			var vals []uint8
			for y := 0; y < h; y++ {
				for x := 0; x < w; x++ {
					c := img.RGBAAt(x, y)
					switch ch {
					case 0:
						vals = append(vals, c.R)
					case 1:
						vals = append(vals, c.G)
					case 2:
						vals = append(vals, c.B)
					}
				}
			}
			sort.Slice(vals, func(i, j int) bool { return vals[i] < vals[j] })
			p2 := float64(vals[len(vals)*2/100])
			p98 := float64(vals[len(vals)*98/100])
			if p98 <= p2 {
				continue
			}

			for y := 0; y < h; y++ {
				for x := 0; x < w; x++ {
					c := img.RGBAAt(x, y)
					switch ch {
					case 0:
						c.R = stretchVal(c.R, p2, p98)
					case 1:
						c.G = stretchVal(c.G, p2, p98)
					case 2:
						c.B = stretchVal(c.B, p2, p98)
					}
					img.SetRGBA(x, y, c)
				}
			}
		}
	}
}

func stretchVal(v uint8, low, high float64) uint8 {
	f := (float64(v) - low) / (high - low) * 255.0
	if f < 0 {
		f = 0
	}
	if f > 255 {
		f = 255
	}
	return uint8(f)
}

func applyMask(img *image.RGBA, cx, cy, radius int) {
	bounds := img.Bounds()
	r2 := float64(radius * radius)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			dx := float64(x - cx)
			dy := float64(y - cy)
			if dx*dx+dy*dy > r2 {
				img.SetRGBA(x, y, color.RGBA{0, 0, 0, 255})
			}
		}
	}
}

// detectBayerLayout analyzes the raw pixel data to determine which positions
// in the 2x2 superpixel are R, G, G, B. Works with any camera regardless of
// FITS header conventions.
//
// Method: compute mean value for each of the 4 positions in the 2x2 grid.
// The two positions with the closest means are Green (there are 2 green pixels).
// Of the remaining two, the dimmer one is Red (works for most sky images).
//
// Returns: redX, redY, blueX, blueY (each 0 or 1, position within 2x2 block)
func detectBayerLayout(raw []uint16, width, height int, headerPattern string) (int, int, int, int) {
	// Sample the center 25% for speed and to avoid edge artifacts
	startX := width / 4
	startY := height / 4
	endX := width * 3 / 4
	endY := height * 3 / 4

	var sums [2][2]float64
	var counts [2][2]float64

	for y := startY; y < endY; y++ {
		for x := startX; x < endX; x++ {
			bx := x % 2
			by := y % 2
			sums[by][bx] += float64(raw[y*width+x])
			counts[by][bx]++
		}
	}

	// Compute means
	type pos struct{ x, y int }
	var positions [4]pos
	var meanVals [4]float64
	idx := 0
	for by := 0; by < 2; by++ {
		for bx := 0; bx < 2; bx++ {
			if counts[by][bx] > 0 {
				meanVals[idx] = sums[by][bx] / counts[by][bx]
			}
			positions[idx] = pos{bx, by}
			idx++
		}
	}

	// Find the two Green positions (closest pair of means)
	minDiff := meanVals[0] + meanVals[1] + meanVals[2] + meanVals[3]
	greenA, greenB := 0, 1
	for i := 0; i < 4; i++ {
		for j := i + 1; j < 4; j++ {
			diff := meanVals[i] - meanVals[j]
			if diff < 0 {
				diff = -diff
			}
			if diff < minDiff {
				minDiff = diff
				greenA, greenB = i, j
			}
		}
	}

	// The other two positions are R and B
	var nonGreen [2]int
	ni := 0
	for i := 0; i < 4; i++ {
		if i != greenA && i != greenB {
			nonGreen[ni] = i
			ni++
		}
	}

	// Dimmer non-green is Red (works for sky images)
	rIdx, bIdx := nonGreen[0], nonGreen[1]
	if meanVals[rIdx] > meanVals[bIdx] {
		rIdx, bIdx = bIdx, rIdx
	}

	return positions[rIdx].x, positions[rIdx].y, positions[bIdx].x, positions[bIdx].y
}
