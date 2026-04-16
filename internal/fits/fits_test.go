package fits

import (
	"encoding/binary"
	"fmt"
	"math"
	"testing"
)

// makeFITSHeader creates a minimal valid FITS header block.
// FITS headers are 2880-byte blocks of 80-char records.
func makeFITSHeader(naxis1, naxis2, bitpix int, bzero, bscale float64, bayerPat string) []byte {
	records := []string{
		fmt.Sprintf("%-8s= %20s", "SIMPLE", "T"),
		fmt.Sprintf("%-8s= %20d", "BITPIX", bitpix),
		fmt.Sprintf("%-8s= %20d", "NAXIS", 2),
		fmt.Sprintf("%-8s= %20d", "NAXIS1", naxis1),
		fmt.Sprintf("%-8s= %20d", "NAXIS2", naxis2),
		fmt.Sprintf("%-8s= %20.1f", "BZERO", bzero),
		fmt.Sprintf("%-8s= %20.1f", "BSCALE", bscale),
	}
	if bayerPat != "" {
		records = append(records, fmt.Sprintf("%-8s= '%s'", "BAYERPAT", bayerPat))
	}
	records = append(records, fmt.Sprintf("%-8s", "END"))

	// Pad each record to 80 chars
	var header []byte
	for _, r := range records {
		padded := fmt.Sprintf("%-80s", r)
		header = append(header, []byte(padded)...)
	}
	// Pad to 2880-byte boundary
	for len(header)%2880 != 0 {
		header = append(header, ' ')
	}
	return header
}

func TestParseHeader(t *testing.T) {
	data := makeFITSHeader(1920, 1080, 16, 32768.0, 1.0, "RGGB")

	h := ParseHeader(data)

	if h.NAXIS1 != 1920 {
		t.Errorf("NAXIS1: got %d, want 1920", h.NAXIS1)
	}
	if h.NAXIS2 != 1080 {
		t.Errorf("NAXIS2: got %d, want 1080", h.NAXIS2)
	}
	if h.BITPIX != 16 {
		t.Errorf("BITPIX: got %d, want 16", h.BITPIX)
	}
	if h.BZERO != 32768.0 {
		t.Errorf("BZERO: got %f, want 32768.0", h.BZERO)
	}
	if h.BSCALE != 1.0 {
		t.Errorf("BSCALE: got %f, want 1.0", h.BSCALE)
	}
	if h.BayerPat != "RGGB" {
		t.Errorf("BayerPat: got %q, want \"RGGB\"", h.BayerPat)
	}
	if h.DataOffset != 2880 {
		t.Errorf("DataOffset: got %d, want 2880", h.DataOffset)
	}
}

func TestParseHeader_Empty(t *testing.T) {
	h := ParseHeader([]byte{})

	// Should return defaults without panicking
	if h.NAXIS1 != 0 {
		t.Errorf("NAXIS1: got %d, want 0", h.NAXIS1)
	}
	if h.NAXIS2 != 0 {
		t.Errorf("NAXIS2: got %d, want 0", h.NAXIS2)
	}
	// BITPIX defaults to 16 in the code
	if h.BITPIX != 16 {
		t.Errorf("BITPIX: got %d, want 16 (default)", h.BITPIX)
	}
	if h.BSCALE != 1.0 {
		t.Errorf("BSCALE: got %f, want 1.0 (default)", h.BSCALE)
	}
}

func TestReadPixels16(t *testing.T) {
	naxis1 := 4
	naxis2 := 2
	nPixels := naxis1 * naxis2

	header := makeFITSHeader(naxis1, naxis2, 16, 32768.0, 1.0, "")
	h := ParseHeader(header)

	// Create pixel data: FITS stores signed int16 in big-endian.
	// With BZERO=32768, a stored value of -32768 (0x8000) becomes 0,
	// and a stored value of -1 (0xFFFF) becomes 32767,
	// and a stored value of 0 becomes 32768,
	// and a stored value of 32767 becomes 65535.
	pixelData := make([]byte, nPixels*2)

	knownValues := []struct {
		stored int16
		want   uint16
	}{
		{-32768, 0},     // min: -32768 + 32768 = 0
		{-32767, 1},
		{0, 32768},
		{32767, 65535},  // max: 32767 + 32768 = 65535
		{-16384, 16384},
		{16383, 49151},
		{100, 32868},
		{-100, 32668},
	}

	for i, kv := range knownValues {
		binary.BigEndian.PutUint16(pixelData[i*2:], uint16(kv.stored))
	}

	data := append(header, pixelData...)
	pixels := ReadPixels16(data, h)

	if len(pixels) != nPixels {
		t.Fatalf("pixel count: got %d, want %d", len(pixels), nPixels)
	}

	for i, kv := range knownValues {
		if pixels[i] != kv.want {
			t.Errorf("pixel[%d]: stored=%d, got %d, want %d", i, kv.stored, pixels[i], kv.want)
		}
	}
}

func TestReadPixels16_InsufficientData(t *testing.T) {
	header := makeFITSHeader(100, 100, 16, 0, 1.0, "")
	h := ParseHeader(header)

	// No pixel data appended — DataOffset beyond data
	pixels := ReadPixels16(header, h)
	if pixels != nil {
		t.Errorf("expected nil for insufficient data, got %d pixels", len(pixels))
	}
}

func TestMedianADU(t *testing.T) {
	naxis1 := 5
	naxis2 := 1

	header := makeFITSHeader(naxis1, naxis2, 16, 32768.0, 1.0, "")

	// Create 5 pixels with known values: stored as signed int16
	// After BZERO: values will be 100, 200, 300, 400, 500
	// Stored = desired - BZERO = desired - 32768
	desired := []float64{100, 200, 300, 400, 500}
	pixelData := make([]byte, naxis1*2)
	for i, d := range desired {
		stored := int16(d - 32768)
		binary.BigEndian.PutUint16(pixelData[i*2:], uint16(stored))
	}

	data := append(header, pixelData...)
	median := MedianADU(data)

	// Median of [100, 200, 300, 400, 500] = 300
	if math.Abs(median-300) > 1.0 {
		t.Errorf("MedianADU: got %.1f, want 300", median)
	}
}

func TestMedianADU_Empty(t *testing.T) {
	median := MedianADU([]byte{})
	if median != 0 {
		t.Errorf("MedianADU empty: got %f, want 0", median)
	}
}

func TestMedianADU_NoPixelData(t *testing.T) {
	header := makeFITSHeader(100, 100, 16, 0, 1.0, "")
	median := MedianADU(header)
	if median != 0 {
		t.Errorf("MedianADU no pixel data: got %f, want 0", median)
	}
}
