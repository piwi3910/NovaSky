// Package fits provides shared FITS file reading utilities.
package fits

import (
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"strings"
)

// Header contains parsed FITS header values.
type Header struct {
	NAXIS1     int
	NAXIS2     int
	BITPIX     int
	BayerPat   string
	BZERO      float64
	BSCALE     float64
	DataOffset int
}

// ParseHeader reads the FITS header from raw file data.
func ParseHeader(data []byte) Header {
	h := Header{BITPIX: 16, BSCALE: 1.0}
	for i := 0; i < len(data)-80; i += 80 {
		line := string(data[i : i+80])
		key := ""
		if len(line) >= 8 {
			key = strings.TrimSpace(line[:8])
		}

		if key == "END" {
			h.DataOffset = ((i/80 + 1) * 80)
			h.DataOffset = ((h.DataOffset + 2879) / 2880) * 2880
			break
		}

		if len(line) > 10 && line[8] == '=' {
			valStr := strings.TrimSpace(strings.Split(line[10:], "/")[0])
			valStr = strings.Trim(valStr, "' ")

			switch key {
			case "NAXIS1":
				fmt.Sscanf(valStr, "%d", &h.NAXIS1)
			case "NAXIS2":
				fmt.Sscanf(valStr, "%d", &h.NAXIS2)
			case "BITPIX":
				fmt.Sscanf(valStr, "%d", &h.BITPIX)
			case "BAYERPAT":
				h.BayerPat = valStr
			case "BZERO":
				fmt.Sscanf(valStr, "%f", &h.BZERO)
			case "BSCALE":
				fmt.Sscanf(valStr, "%f", &h.BSCALE)
			}
		}
	}
	return h
}

// ReadPixels16 reads 16-bit pixel data from FITS, applying BZERO/BSCALE.
// Returns pixel values in the range 0-65535.
func ReadPixels16(data []byte, header Header) []uint16 {
	if header.DataOffset >= len(data) {
		return nil
	}
	pixelData := data[header.DataOffset:]
	nPixels := header.NAXIS1 * header.NAXIS2
	bytesPerPixel := abs(header.BITPIX) / 8
	pixels := make([]uint16, nPixels)

	for i := 0; i < nPixels && i*bytesPerPixel+1 < len(pixelData); i++ {
		if bytesPerPixel == 2 {
			raw := int16(binary.BigEndian.Uint16(pixelData[i*2 : i*2+2]))
			val := float64(raw)*header.BSCALE + header.BZERO
			pixels[i] = clampUint16(val)
		} else if bytesPerPixel == 1 {
			val := float64(pixelData[i])*header.BSCALE + header.BZERO
			pixels[i] = clampUint16(val * 256) // scale 8-bit to 16-bit range
		}
	}
	return pixels
}

// MedianADU computes the median pixel value from raw FITS data.
// Samples pixels for performance on large images.
func MedianADU(data []byte) float64 {
	header := ParseHeader(data)
	if header.DataOffset >= len(data) || header.NAXIS1 == 0 || header.NAXIS2 == 0 {
		return 0
	}

	pixelData := data[header.DataOffset:]
	nPixels := header.NAXIS1 * header.NAXIS2
	bytesPerPixel := abs(header.BITPIX) / 8

	// Sample for performance
	step := 1
	if nPixels > 100000 {
		step = nPixels / 100000
	}

	values := make([]float64, 0, nPixels/step)
	for i := 0; i < nPixels && i*bytesPerPixel+1 < len(pixelData); i += step {
		if bytesPerPixel == 2 {
			raw := int16(binary.BigEndian.Uint16(pixelData[i*2 : i*2+2]))
			val := float64(raw)*header.BSCALE + header.BZERO
			values = append(values, math.Max(0, val))
		} else if bytesPerPixel == 1 {
			values = append(values, float64(pixelData[i]))
		}
	}

	if len(values) == 0 {
		return 0
	}

	sort.Float64s(values)
	return values[len(values)/2]
}

func clampUint16(v float64) uint16 {
	if v < 0 {
		return 0
	}
	if v > 65535 {
		return 65535
	}
	return uint16(v)
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
