package processing

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/piwi3910/NovaSky/internal/fits"
	"gocv.io/x/gocv"
)

// ProcessResult contains the output of frame processing
type ProcessResult struct {
	JpegPath string
}

// MaskConfig defines a circular mask for the visible sky area
type MaskConfig struct {
	CenterX int  `json:"centerX"`
	CenterY int  `json:"centerY"`
	Radius  int  `json:"radius"`
	Enabled bool `json:"enabled"`
}

// Bayer pattern mapping following indi-allsky convention
// https://github.com/aaronwmorris/indi-allsky
// RGGB -> cv2.COLOR_BAYER_BG2BGR (OpenCV convention)
var cfaMap = map[string]gocv.ColorConversionCode{
	"RGGB": gocv.ColorBayerBGToBGR,
	"GRBG": gocv.ColorBayerGBToBGR,
	"BGGR": gocv.ColorBayerRGToBGR,
	"GBRG": gocv.ColorBayerGRToBGR,
}

// ProcessFrame debayers, white-balances, stretches, and saves a JPEG from a raw FITS file.
// Uses OpenCV (via GoCV) for debayering — proven correct with indi-allsky mapping.
func ProcessFrame(fitsPath string, stretch string, maskCfg *MaskConfig) (*ProcessResult, error) {
	// Read FITS
	rawData, err := os.ReadFile(fitsPath)
	if err != nil {
		return nil, err
	}

	header := fits.ParseHeader(rawData)
	width := header.NAXIS1
	height := header.NAXIS2
	bayerPat := header.BayerPat

	if width == 0 || height == 0 {
		return nil, fmt.Errorf("invalid FITS dimensions: %dx%d", width, height)
	}

	// Read pixel data applying BZERO
	pixels := fits.ReadPixels16(rawData, header)

	// Create OpenCV Mat from pixel data (16-bit single channel)
	mat, err := gocv.NewMatFromBytes(height, width, gocv.MatTypeCV16UC1, uint16ToBytes(pixels))
	if err != nil {
		return nil, fmt.Errorf("failed to create Mat: %w", err)
	}
	defer mat.Close()

	var rgb gocv.Mat

	if code, ok := cfaMap[bayerPat]; ok {
		// Debayer using OpenCV
		bgr := gocv.NewMat()
		defer bgr.Close()
		gocv.CvtColor(mat, &bgr, code)

		// BGR to RGB
		rgb = gocv.NewMat()
		gocv.CvtColor(bgr, &rgb, gocv.ColorBGRToRGB)
	} else {
		// Mono camera — convert to 3-channel
		rgb = gocv.NewMat()
		gocv.CvtColor(mat, &rgb, gocv.ColorGrayToBGR)
	}
	defer rgb.Close()

	// Auto white balance (gray world) on 16-bit data
	applyAutoWB(&rgb)

	// Convert 16-bit to 8-bit based on stretch mode
	out := gocv.NewMat()
	defer out.Close()

	switch stretch {
	case "none":
		// Linear 16→8 bit: divide by 256
		rgb.ConvertTo(&out, gocv.MatTypeCV8UC3)
	case "linear":
		applyLinearStretch(&rgb, &out)
	case "auto":
		applyAutoStretch(&rgb, &out)
	default:
		rgb.ConvertTo(&out, gocv.MatTypeCV8UC3)
	}

	// Apply mask
	if maskCfg != nil && maskCfg.Enabled {
		applyMaskCV(&out, maskCfg.CenterX, maskCfg.CenterY, maskCfg.Radius)
	}

	// Save JPEG
	jpegPath := strings.TrimSuffix(fitsPath, filepath.Ext(fitsPath)) + ".jpg"

	// Convert to Go image for JPEG encoding (GoCV's IMWrite could also work)
	img, err := out.ToImage()
	if err != nil {
		return nil, fmt.Errorf("failed to convert to image: %w", err)
	}

	f, err := os.Create(jpegPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 90}); err != nil {
		return nil, err
	}

	return &ProcessResult{JpegPath: jpegPath}, nil
}

// uint16ToBytes converts a uint16 slice to a byte slice (little-endian for OpenCV)
func uint16ToBytes(data []uint16) []byte {
	buf := make([]byte, len(data)*2)
	for i, v := range data {
		binary.LittleEndian.PutUint16(buf[i*2:], v)
	}
	return buf
}

// applyAutoWB applies gray world white balance on a 16-bit 3-channel Mat
func applyAutoWB(img *gocv.Mat) {
	channels := gocv.Split(*img)
	defer func() {
		for _, ch := range channels {
			ch.Close()
		}
	}()

	if len(channels) != 3 {
		return
	}

	means := make([]float64, 3)
	for i, ch := range channels {
		means[i] = ch.Mean().Val1
	}

	overall := (means[0] + means[1] + means[2]) / 3.0

	for i, ch := range channels {
		if means[i] > 0 {
			gain := overall / means[i]
			ch.MultiplyFloat(gain)
		}
	}

	gocv.Merge(channels, img)
}

// applyLinearStretch applies a percentile stretch (5th-95th) across all channels
func applyLinearStretch(src *gocv.Mat, dst *gocv.Mat) {
	// Convert to float for calculation
	srcF := gocv.NewMat()
	defer srcF.Close()
	src.ConvertTo(&srcF, gocv.MatTypeCV32FC3)

	// Get min/max percentiles (approximate via histogram)
	gray := gocv.NewMat()
	defer gray.Close()
	gocv.CvtColor(*src, &gray, gocv.ColorRGBToGray)

	minVal, maxVal, _, _ := gocv.MinMaxLoc(gray)
	p5 := minVal + (maxVal-minVal)*0.05
	p95 := minVal + (maxVal-minVal)*0.95

	if p95 <= p5 {
		src.ConvertTo(dst, gocv.MatTypeCV8UC3)
		return
	}

	// Scale: (pixel - p5) / (p95 - p5) * 255
	scale := 255.0 / (p95 - p5)
	srcF.AddFloat(-p5)
	srcF.MultiplyFloat(scale)

	srcF.ConvertTo(dst, gocv.MatTypeCV8UC3)
}

// applyAutoStretch applies per-channel percentile stretch
func applyAutoStretch(src *gocv.Mat, dst *gocv.Mat) {
	channels := gocv.Split(*src)
	defer func() {
		for _, ch := range channels {
			ch.Close()
		}
	}()

	outChannels := make([]gocv.Mat, len(channels))
	for i, ch := range channels {
		minVal, maxVal, _, _ := gocv.MinMaxLoc(ch)
		p2 := minVal + (maxVal-minVal)*0.02
		p98 := minVal + (maxVal-minVal)*0.98

		chF := gocv.NewMat()
		ch.ConvertTo(&chF, gocv.MatTypeCV32F)

		if p98 > p2 {
			scale := 255.0 / (p98 - p2)
			chF.AddFloat(-p2)
			chF.MultiplyFloat(scale)
		}

		outCh := gocv.NewMat()
		chF.ConvertTo(&outCh, gocv.MatTypeCV8U)
		chF.Close()
		outChannels[i] = outCh
	}

	gocv.Merge(outChannels, dst)
	for _, ch := range outChannels {
		ch.Close()
	}
}

// applyMaskCV blacks out pixels outside a circular region
func applyMaskCV(img *gocv.Mat, cx, cy, radius int) {
	mask := gocv.NewMatWithSize(img.Rows(), img.Cols(), gocv.MatTypeCV8UC1)
	defer mask.Close()

	center := image.Pt(cx, cy)
	gocv.Circle(&mask, center, radius, color.RGBA{255, 255, 255, 255}, -1)

	// Apply mask
	masked := gocv.NewMat()
	defer masked.Close()

	img.CopyToWithMask(&masked, mask)
	masked.CopyTo(img)
}

// Keep these for potential non-OpenCV fallback
var _ = math.Abs
var _ = image.Pt
