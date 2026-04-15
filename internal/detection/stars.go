package detection

import (
	"math"

	"gocv.io/x/gocv"
)

// Star represents a detected star
type Star struct {
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	Brightness float64 `json:"brightness"`
	FWHM       float64 `json:"fwhm"`
	HFR        float64 `json:"hfr"`
}

// DetectStars finds point sources in a grayscale image
func DetectStars(img gocv.Mat, minBrightness float64) []Star {
	// Convert to grayscale if needed
	gray := gocv.NewMat()
	defer gray.Close()
	if img.Channels() > 1 {
		gocv.CvtColor(img, &gray, gocv.ColorBGRToGray)
	} else {
		img.CopyTo(&gray)
	}

	// Ensure 8-bit
	gray8 := gocv.NewMat()
	defer gray8.Close()
	if gray.Type() != gocv.MatTypeCV8UC1 {
		gray.ConvertToWithParams(&gray8, gocv.MatTypeCV8UC1, 1.0/256.0, 0)
	} else {
		gray.CopyTo(&gray8)
	}

	// Threshold to find bright points
	thresh := gocv.NewMat()
	defer thresh.Close()
	threshVal := minBrightness
	if threshVal <= 0 {
		threshVal = 200
	}
	gocv.Threshold(gray8, &thresh, threshVal, 255, gocv.ThresholdBinary)

	// Find contours
	contours := gocv.FindContours(thresh, gocv.RetrievalExternal, gocv.ChainApproxSimple)
	defer contours.Close()

	var stars []Star
	for i := 0; i < contours.Size(); i++ {
		c := contours.At(i)
		area := gocv.ContourArea(c)
		if area < 1 || area > 500 { // filter noise and large objects
			continue
		}

		moments := gocv.Moments(c, false)
		if moments["m00"] == 0 {
			continue
		}

		cx := moments["m10"] / moments["m00"]
		cy := moments["m01"] / moments["m00"]

		// Compute brightness at center
		brightness := float64(gray8.GetUCharAt(int(cy), int(cx)))

		// Estimate FWHM from contour area (rough approximation)
		radius := math.Sqrt(area / math.Pi)
		fwhm := radius * 2.355 // Gaussian FWHM ≈ 2.355 * sigma

		stars = append(stars, Star{
			X:          cx,
			Y:          cy,
			Brightness: brightness,
			FWHM:       fwhm,
			HFR:        radius,
		})
	}
	return stars
}
