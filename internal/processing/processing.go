package processing

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

	// Parse FITS header
	header := parseFITSHeader(data)
	width := header.naxis1
	height := header.naxis2
	bitpix := header.bitpix
	bayerPat := header.bayerpat
	headerSize := header.dataOffset

	if width == 0 || height == 0 {
		return nil, fmt.Errorf("invalid FITS dimensions: %dx%d", width, height)
	}

	pixelData := data[headerSize:]

	// Read raw pixel values as uint16
	pixels := readPixels16(pixelData, width, height, bitpix)

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

type fitsHeader struct {
	naxis1     int
	naxis2     int
	bitpix     int
	bayerpat   string
	dataOffset int
}

func parseFITSHeader(data []byte) fitsHeader {
	h := fitsHeader{bitpix: 16}
	for i := 0; i < len(data)-80; i += 80 {
		line := string(data[i : i+80])
		key := strings.TrimSpace(line[:8])

		if key == "END" {
			h.dataOffset = ((i/80 + 1) * 80)
			h.dataOffset = ((h.dataOffset + 2879) / 2880) * 2880
			break
		}

		if len(line) > 10 && line[8] == '=' {
			valStr := strings.TrimSpace(strings.Split(line[10:], "/")[0])
			valStr = strings.Trim(valStr, "' ")

			switch key {
			case "NAXIS1":
				fmt.Sscanf(valStr, "%d", &h.naxis1)
			case "NAXIS2":
				fmt.Sscanf(valStr, "%d", &h.naxis2)
			case "BITPIX":
				fmt.Sscanf(valStr, "%d", &h.bitpix)
			case "BAYERPAT":
				h.bayerpat = valStr
			}
		}
	}
	return h
}

func readPixels16(data []byte, width, height, bitpix int) []uint16 {
	nPixels := width * height
	pixels := make([]uint16, nPixels)
	bytesPerPixel := abs(bitpix) / 8

	for i := 0; i < nPixels && i*bytesPerPixel+1 < len(data); i++ {
		if bytesPerPixel == 2 {
			// FITS is big-endian
			pixels[i] = binary.BigEndian.Uint16(data[i*2 : i*2+2])
		} else if bytesPerPixel == 1 {
			pixels[i] = uint16(data[i]) * 256 // scale 8-bit to 16-bit
		}
	}
	return pixels
}

// debayer implements simple bilinear Bayer demosaicing
func debayer(raw []uint16, width, height int, pattern string) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// RGGB pattern (most common, used by ASI676MC)
	// Row 0: R G R G ...
	// Row 1: G B G B ...
	isRed := func(x, y int) bool {
		switch pattern {
		case "RGGB":
			return x%2 == 0 && y%2 == 0
		case "BGGR":
			return x%2 == 1 && y%2 == 1
		case "GRBG":
			return x%2 == 1 && y%2 == 0
		case "GBRG":
			return x%2 == 0 && y%2 == 1
		}
		return false
	}
	isBlue := func(x, y int) bool {
		switch pattern {
		case "RGGB":
			return x%2 == 1 && y%2 == 1
		case "BGGR":
			return x%2 == 0 && y%2 == 0
		case "GRBG":
			return x%2 == 0 && y%2 == 1
		case "GBRG":
			return x%2 == 1 && y%2 == 0
		}
		return false
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

			// Scale 16-bit to 8-bit (will be adjusted by stretch)
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

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
