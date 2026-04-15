package detection

import (
	"math"

	"gocv.io/x/gocv"
)

// Meteor represents a detected meteor streak
type Meteor struct {
	X1     float64 `json:"x1"`
	Y1     float64 `json:"y1"`
	X2     float64 `json:"x2"`
	Y2     float64 `json:"y2"`
	Length float64 `json:"length"`
}

// DetectMeteors finds fast-moving streaks by comparing two consecutive frames.
// Returns detected meteor-like line segments.
func DetectMeteors(prev, curr gocv.Mat) []Meteor {
	if prev.Empty() || curr.Empty() {
		return nil
	}

	// Convert to grayscale 8-bit
	g1 := gocv.NewMat()
	defer g1.Close()
	g2 := gocv.NewMat()
	defer g2.Close()

	if prev.Channels() > 1 {
		gocv.CvtColor(prev, &g1, gocv.ColorBGRToGray)
	} else {
		prev.CopyTo(&g1)
	}
	if curr.Channels() > 1 {
		gocv.CvtColor(curr, &g2, gocv.ColorBGRToGray)
	} else {
		curr.CopyTo(&g2)
	}

	if g1.Type() != gocv.MatTypeCV8UC1 {
		tmp := gocv.NewMat()
		g1.ConvertToWithParams(&tmp, gocv.MatTypeCV8UC1, 1.0/256.0, 0)
		tmp.CopyTo(&g1)
		tmp.Close()
	}
	if g2.Type() != gocv.MatTypeCV8UC1 {
		tmp := gocv.NewMat()
		g2.ConvertToWithParams(&tmp, gocv.MatTypeCV8UC1, 1.0/256.0, 0)
		tmp.CopyTo(&g2)
		tmp.Close()
	}

	// Frame difference
	diff := gocv.NewMat()
	defer diff.Close()
	gocv.AbsDiff(g1, g2, &diff)

	// Threshold
	thresh := gocv.NewMat()
	defer thresh.Close()
	gocv.Threshold(diff, &thresh, 50, 255, gocv.ThresholdBinary)

	// Hough line detection for streaks
	lines := gocv.NewMat()
	defer lines.Close()
	gocv.HoughLinesPWithParams(thresh, &lines, 1, math.Pi/180, 50, 20, 10)

	var meteors []Meteor
	for i := 0; i < lines.Rows(); i++ {
		x1 := float64(lines.GetIntAt(i, 0))
		y1 := float64(lines.GetIntAt(i, 1))
		x2 := float64(lines.GetIntAt(i, 2))
		y2 := float64(lines.GetIntAt(i, 3))
		dx := x2 - x1
		dy := y2 - y1
		length := math.Sqrt(dx*dx + dy*dy)
		if length > 30 { // minimum streak length
			meteors = append(meteors, Meteor{X1: x1, Y1: y1, X2: x2, Y2: y2, Length: length})
		}
	}
	return meteors
}
