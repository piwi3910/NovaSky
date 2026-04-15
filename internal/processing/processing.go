package processing

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
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

// Bayer pattern mapping — verified against camera's own RGB24 output.
// Tested all 4 OpenCV BayerXX codes; BayerGB matches camera RGB exactly for RGGB.
var cfaMap = map[string]gocv.ColorConversionCode{
	"RGGB": gocv.ColorBayerGBToBGR,
	"GRBG": gocv.ColorBayerBGToBGR,
	"BGGR": gocv.ColorBayerGRToBGR,
	"GBRG": gocv.ColorBayerRGToBGR,
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
		// Debayer using OpenCV — output is BGR (OpenCV native)
		rgb = gocv.NewMat()
		gocv.CvtColor(mat, &rgb, code)
		// Keep in BGR — we'll use IMWrite which expects BGR
	} else {
		// Mono camera — convert to 3-channel
		rgb = gocv.NewMat()
		gocv.CvtColor(mat, &rgb, gocv.ColorGrayToBGR)
	}
	defer rgb.Close()

	// Apply SCNR (remove green cast from color cameras)
	if bayerPat != "" {
		applySCNR(&rgb)
	}

	// Apply noise reduction (currently hardcoded to off)
	applyNoiseReduction(&rgb, "off", 3)

	// Convert 16-bit to 8-bit based on stretch mode
	out := gocv.NewMat()
	defer out.Close()

	switch stretch {
	case "none":
		// Linear 16→8 bit: scale by 1/256
		rgb.ConvertToWithParams(&out, gocv.MatTypeCV8UC3, 1.0/256.0, 0)
	case "linear":
		applyLinearStretch(&rgb, &out)
	case "auto":
		applyAutoStretch(&rgb, &out)
	case "adaptive":
		applyAdaptiveStretch(&rgb, &out)
	case "ghs":
		applyGHSStretch(&rgb, &out, 2.0, 0.25, 0.0, 0.0)
	default:
		rgb.ConvertToWithParams(&out, gocv.MatTypeCV8UC3, 1.0/256.0, 0)
	}

	// Apply mask
	if maskCfg != nil && maskCfg.Enabled {
		applyMaskCV(&out, maskCfg.CenterX, maskCfg.CenterY, maskCfg.Radius)
	}

	// Save JPEG using OpenCV IMWrite (handles BGR natively)
	jpegPath := strings.TrimSuffix(fitsPath, filepath.Ext(fitsPath)) + ".jpg"
	if ok := gocv.IMWriteWithParams(jpegPath, out, []int{gocv.IMWriteJpegQuality, 90}); !ok {
		return nil, fmt.Errorf("failed to write JPEG: %s", jpegPath)
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
			ch.MultiplyFloat(float32(gain))
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

// applySCNR applies Subtractive Chromatic Noise Reduction (average neutral method).
// Constrains G channel to not exceed the average of R and B channels.
// Works on BGR 16-bit Mat.
func applySCNR(img *gocv.Mat) {
	// Split into B, G, R channels
	channels := gocv.Split(*img)
	defer func() {
		for _, ch := range channels {
			ch.Close()
		}
	}()

	if len(channels) != 3 {
		return
	}

	// B=channels[0], G=channels[1], R=channels[2] (BGR order)
	// m = (R + B) / 2
	m := gocv.NewMat()
	defer m.Close()
	gocv.AddWeighted(channels[2], 0.5, channels[0], 0.5, 0, &m)

	// G = min(G, m)
	gocv.Min(channels[1], m, &channels[1])

	gocv.Merge(channels, img)
}

// applyNoiseReduction applies spatial noise filtering.
func applyNoiseReduction(img *gocv.Mat, filterType string, kernelSize int) {
	if filterType == "" || filterType == "off" {
		return
	}
	if kernelSize <= 0 {
		kernelSize = 3
	}
	// Ensure odd kernel size
	if kernelSize%2 == 0 {
		kernelSize++
	}

	switch filterType {
	case "gaussian":
		gocv.GaussianBlur(*img, img, image.Pt(kernelSize, kernelSize), 0, 0, gocv.BorderDefault)
	case "bilateral":
		dst := gocv.NewMat()
		defer dst.Close()
		gocv.BilateralFilter(*img, &dst, kernelSize, float64(kernelSize)*2, float64(kernelSize)*2)
		dst.CopyTo(img)
	case "median":
		gocv.MedianBlur(*img, img, kernelSize)
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

// StackFrames combines multiple frames by averaging (mean stacking).
// All frames must be the same dimensions. Works on 16-bit BGR Mats.
func StackFrames(frames []gocv.Mat) gocv.Mat {
	if len(frames) == 0 {
		return gocv.NewMat()
	}
	if len(frames) == 1 {
		result := gocv.NewMat()
		frames[0].CopyTo(&result)
		return result
	}

	// Accumulate in float64
	acc := gocv.NewMat()
	defer acc.Close()
	frames[0].ConvertTo(&acc, gocv.MatTypeCV64FC3)

	for i := 1; i < len(frames); i++ {
		tmp := gocv.NewMat()
		frames[i].ConvertTo(&tmp, gocv.MatTypeCV64FC3)
		gocv.Add(acc, tmp, &acc)
		tmp.Close()
	}

	// Divide by count
	n := float64(len(frames))
	acc.DivideFloat(float32(n))

	result := gocv.NewMat()
	acc.ConvertTo(&result, gocv.MatTypeCV16UC3)
	return result
}

// applyAdaptiveStretch applies a non-linear stretch preserving star colors.
// Uses midtone transfer function (MTF) similar to PixInsight.
func applyAdaptiveStretch(src *gocv.Mat, dst *gocv.Mat) {
	// Convert to float
	srcF := gocv.NewMat()
	defer srcF.Close()
	src.ConvertToWithParams(&srcF, gocv.MatTypeCV32FC3, 1.0/65535.0, 0) // normalize to 0-1

	// Compute median for midtone balance
	gray := gocv.NewMat()
	defer gray.Close()
	gocv.CvtColor(srcF, &gray, gocv.ColorBGRToGray)
	median := gray.Mean().Val1

	// MTF: midtones transfer function
	// f(x) = (m-1)*x / ((2*m-1)*x - m) where m is the midtones balance
	m := 0.25 // target midtone
	if median > 0 && median < 1 {
		m = median
	}

	// Apply MTF via LUT (build 256-entry lookup for 8-bit output)
	// First scale to 8-bit
	srcF.ConvertToWithParams(dst, gocv.MatTypeCV8UC3, 255.0, 0)

	// Apply MTF curve using LUT
	lut := gocv.NewMatWithSize(1, 256, gocv.MatTypeCV8UC1)
	defer lut.Close()
	for i := 0; i < 256; i++ {
		x := float64(i) / 255.0
		var y float64
		if x == 0 {
			y = 0
		} else if x == 1 {
			y = 1
		} else {
			y = (m - 1) * x / ((2*m-1)*x - m)
		}
		if y < 0 {
			y = 0
		}
		if y > 1 {
			y = 1
		}
		lut.SetUCharAt(0, i, uint8(y*255))
	}

	// Apply LUT to each channel
	channels := gocv.Split(*dst)
	for i, ch := range channels {
		gocv.LUT(ch, lut, &channels[i])
	}
	gocv.Merge(channels, dst)
	for _, ch := range channels {
		ch.Close()
	}
}

// asinh returns the inverse hyperbolic sine of v.
func asinh(v float64) float64 {
	return math.Log(v + math.Sqrt(v*v+1))
}

// applyGHSStretch applies Generalized Hyperbolic Stretch (arcsinh-based).
// Parameters: D (stretch factor), b (symmetry point), SP (shadow protection), HP (highlight protection)
func applyGHSStretch(src *gocv.Mat, dst *gocv.Mat, D, b, SP, HP float64) {
	if D <= 0 {
		D = 2.0
	}
	if b <= 0 {
		b = 0.25
	}

	// Convert to float normalized 0-1
	srcF := gocv.NewMat()
	defer srcF.Close()
	src.ConvertToWithParams(&srcF, gocv.MatTypeCV32FC3, 1.0/65535.0, 0)

	// Build GHS LUT (256 entries for 8-bit output)
	lut := gocv.NewMatWithSize(1, 256, gocv.MatTypeCV8UC1)
	defer lut.Close()

	for i := 0; i < 256; i++ {
		x := float64(i) / 255.0
		// Arcsinh stretch: y = asinh(D * (x - b)) / asinh(D * (1 - b))
		num := asinh(D * (x - b))
		den := asinh(D * (1 - b))
		var y float64
		if den != 0 {
			y = num / den
		}
		// Shadow protection
		if SP > 0 && x < SP {
			y = y * (x / SP)
		}
		// Highlight protection
		if HP > 0 && x > (1-HP) {
			y = 1 - (1-y)*((1-x)/HP)
		}
		if y < 0 {
			y = 0
		}
		if y > 1 {
			y = 1
		}
		lut.SetUCharAt(0, i, uint8(y*255))
	}

	// Scale to 8-bit first
	srcF.ConvertToWithParams(dst, gocv.MatTypeCV8UC3, 255.0, 0)

	// Apply LUT to each channel
	channels := gocv.Split(*dst)
	for i, ch := range channels {
		gocv.LUT(ch, lut, &channels[i])
	}
	gocv.Merge(channels, dst)
	for _, ch := range channels {
		ch.Close()
	}
}

// applySkyglowReduction removes light pollution gradient by subtracting
// a smoothed background model from the image.
func applySkyglowReduction(img *gocv.Mat, aggressiveness int) {
	if aggressiveness <= 0 {
		aggressiveness = 64
	}

	// Create heavily blurred version as background model
	bg := gocv.NewMat()
	defer bg.Close()
	// Use very large kernel to capture only the gradient
	kernelSize := aggressiveness*2 + 1
	if kernelSize > 255 {
		kernelSize = 255
	}
	// Kernel size must be odd
	if kernelSize%2 == 0 {
		kernelSize++
	}
	gocv.GaussianBlur(*img, &bg, image.Pt(kernelSize, kernelSize), 0, 0, gocv.BorderDefault)

	// Subtract background model
	gocv.Subtract(*img, bg, img)
}

// SubtractDarkFrame subtracts a dark frame from the image.
// Both must be same dimensions and type.
func SubtractDarkFrame(img *gocv.Mat, dark gocv.Mat) {
	if img.Rows() != dark.Rows() || img.Cols() != dark.Cols() {
		return
	}
	gocv.Subtract(*img, dark, img)
}

// ApplyFlatCorrection divides image by normalized flat frame to correct vignetting.
func ApplyFlatCorrection(img *gocv.Mat, flat gocv.Mat) {
	if img.Rows() != flat.Rows() || img.Cols() != flat.Cols() {
		return
	}
	// Normalize flat to mean = 1.0
	flatF := gocv.NewMat()
	defer flatF.Close()
	flat.ConvertTo(&flatF, gocv.MatTypeCV32FC3)
	mean := flatF.Mean()
	avgMean := (mean.Val1 + mean.Val2 + mean.Val3) / 3.0
	if avgMean > 0 {
		flatF.DivideFloat(float32(avgMean))
	}

	// Divide image by normalized flat
	imgF := gocv.NewMat()
	defer imgF.Close()
	img.ConvertTo(&imgF, gocv.MatTypeCV32FC3)
	gocv.Divide(imgF, flatF, &imgF)
	imgF.ConvertTo(img, img.Type())
}

// InterpolateBadPixels replaces bad pixels with the median of their neighbors.
// badPixels is a list of (x, y) coordinates.
func InterpolateBadPixels(img *gocv.Mat, badPixels [][2]int) {
	for _, bp := range badPixels {
		x, y := bp[0], bp[1]
		if x <= 0 || y <= 0 || x >= img.Cols()-1 || y >= img.Rows()-1 {
			continue
		}
		// 3x3 median filter on the bad pixel location
		roi := img.Region(image.Rect(x-1, y-1, x+2, y+2))
		median := gocv.NewMat()
		gocv.MedianBlur(roi, &median, 3)
		// Copy center pixel back
		centerVal := median.GetVecbAt(1, 1)
		img.SetUCharAt(y, x*3, centerVal[0])
		img.SetUCharAt(y, x*3+1, centerVal[1])
		img.SetUCharAt(y, x*3+2, centerVal[2])
		roi.Close()
		median.Close()
	}
}

