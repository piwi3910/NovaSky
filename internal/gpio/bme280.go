package gpio

import (
	"encoding/binary"
	"fmt"
	"os"
	"syscall"
)

const (
	i2cSlave = 0x0703 // ioctl I2C_SLAVE

	// BME280 register addresses
	bme280RegCalibT1    = 0x88
	bme280RegCalibH1    = 0xA1
	bme280RegCalibH2    = 0xE1
	bme280RegCtrlHum    = 0xF2
	bme280RegCtrlMeas   = 0xF4
	bme280RegConfig     = 0xF5
	bme280RegDataStart  = 0xF7
)

// bme280Calibration holds compensation parameters read from the sensor.
type bme280Calibration struct {
	T1                         uint16
	T2, T3                     int16
	P1                         uint16
	P2, P3, P4, P5, P6, P7, P8, P9 int16
	H1                         uint8
	H2                         int16
	H3                         uint8
	H4, H5                     int16
	H6                         int8
}

// ReadBME280 reads temperature, humidity, and pressure from a BME280 sensor via I2C.
// Uses raw syscall ioctl — no CGO required.
func ReadBME280(bus string, addr uint8) (temp, humidity, pressure float64, err error) {
	f, err := os.OpenFile(bus, os.O_RDWR, 0)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("open i2c bus %s: %w", bus, err)
	}
	defer f.Close()

	// Set I2C slave address
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), i2cSlave, uintptr(addr)); errno != 0 {
		return 0, 0, 0, fmt.Errorf("ioctl I2C_SLAVE: %w", errno)
	}

	// Read calibration data
	cal, err := readCalibration(f)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("read calibration: %w", err)
	}

	// Configure sensor: humidity oversampling x1
	if err := writeReg(f, bme280RegCtrlHum, 0x01); err != nil {
		return 0, 0, 0, err
	}
	// Config: no filter, standby 1000ms
	if err := writeReg(f, bme280RegConfig, 0xA0); err != nil {
		return 0, 0, 0, err
	}
	// Ctrl_meas: temp oversampling x1, pressure oversampling x1, forced mode
	if err := writeReg(f, bme280RegCtrlMeas, 0x25); err != nil {
		return 0, 0, 0, err
	}

	// Wait for measurement (forced mode completes quickly)
	// In forced mode with 1x oversampling, measurement time is ~10ms
	// We do a simple busy-wait by re-reading status
	raw := make([]byte, 8)
	if err := readRegs(f, bme280RegDataStart, raw); err != nil {
		return 0, 0, 0, fmt.Errorf("read data: %w", err)
	}

	// Parse raw ADC values (20-bit pressure, 20-bit temp, 16-bit humidity)
	adcP := int32(raw[0])<<12 | int32(raw[1])<<4 | int32(raw[2])>>4
	adcT := int32(raw[3])<<12 | int32(raw[4])<<4 | int32(raw[5])>>4
	adcH := int32(raw[6])<<8 | int32(raw[7])

	// Compensate temperature (returns value in 0.01 °C)
	tFine := compensateTemperature(adcT, cal)
	temp = float64(tFine) / 5120.0

	// Compensate pressure (returns value in Pa, convert to hPa)
	pressure = compensatePressure(adcP, tFine, cal) / 100.0

	// Compensate humidity (returns value in %RH * 1024)
	humidity = compensateHumidity(adcH, tFine, cal) / 1024.0

	return temp, humidity, pressure, nil
}

func readCalibration(f *os.File) (*bme280Calibration, error) {
	cal := &bme280Calibration{}

	// Read temperature and pressure calibration (0x88..0x9F, 26 bytes)
	tp := make([]byte, 26)
	if err := readRegs(f, bme280RegCalibT1, tp); err != nil {
		return nil, err
	}

	cal.T1 = binary.LittleEndian.Uint16(tp[0:2])
	cal.T2 = int16(binary.LittleEndian.Uint16(tp[2:4]))
	cal.T3 = int16(binary.LittleEndian.Uint16(tp[4:6]))
	cal.P1 = binary.LittleEndian.Uint16(tp[6:8])
	cal.P2 = int16(binary.LittleEndian.Uint16(tp[8:10]))
	cal.P3 = int16(binary.LittleEndian.Uint16(tp[10:12]))
	cal.P4 = int16(binary.LittleEndian.Uint16(tp[12:14]))
	cal.P5 = int16(binary.LittleEndian.Uint16(tp[14:16]))
	cal.P6 = int16(binary.LittleEndian.Uint16(tp[16:18]))
	cal.P7 = int16(binary.LittleEndian.Uint16(tp[18:20]))
	cal.P8 = int16(binary.LittleEndian.Uint16(tp[20:22]))
	cal.P9 = int16(binary.LittleEndian.Uint16(tp[22:24]))

	// Read H1 (0xA1, 1 byte)
	h1 := make([]byte, 1)
	if err := readRegs(f, bme280RegCalibH1, h1); err != nil {
		return nil, err
	}
	cal.H1 = h1[0]

	// Read H2..H6 (0xE1..0xE7, 7 bytes)
	hx := make([]byte, 7)
	if err := readRegs(f, bme280RegCalibH2, hx); err != nil {
		return nil, err
	}
	cal.H2 = int16(binary.LittleEndian.Uint16(hx[0:2]))
	cal.H3 = hx[2]
	cal.H4 = int16(hx[3])<<4 | int16(hx[4]&0x0F)
	cal.H5 = int16(hx[5])<<4 | int16(hx[4])>>4
	cal.H6 = int8(hx[6])

	return cal, nil
}

func compensateTemperature(adcT int32, cal *bme280Calibration) int32 {
	var1 := (((adcT >> 3) - int32(cal.T1)<<1) * int32(cal.T2)) >> 11
	var2 := (((((adcT >> 4) - int32(cal.T1)) * ((adcT >> 4) - int32(cal.T1))) >> 12) * int32(cal.T3)) >> 14
	return var1 + var2 // t_fine
}

func compensatePressure(adcP, tFine int32, cal *bme280Calibration) float64 {
	var1 := float64(tFine)/2.0 - 64000.0
	var2 := var1 * var1 * float64(cal.P6) / 32768.0
	var2 = var2 + var1*float64(cal.P5)*2.0
	var2 = var2/4.0 + float64(cal.P4)*65536.0
	var1 = (float64(cal.P3)*var1*var1/524288.0 + float64(cal.P2)*var1) / 524288.0
	var1 = (1.0 + var1/32768.0) * float64(cal.P1)
	if var1 == 0 {
		return 0
	}
	p := 1048576.0 - float64(adcP)
	p = (p - var2/4096.0) * 6250.0 / var1
	var1 = float64(cal.P9) * p * p / 2147483648.0
	var2 = p * float64(cal.P8) / 32768.0
	return p + (var1+var2+float64(cal.P7))/16.0
}

func compensateHumidity(adcH, tFine int32, cal *bme280Calibration) float64 {
	h := float64(tFine) - 76800.0
	if h == 0 {
		return 0
	}
	h = (float64(adcH) - (float64(cal.H4)*64.0 + float64(cal.H5)/16384.0*h)) *
		(float64(cal.H2) / 65536.0 * (1.0 + float64(cal.H6)/67108864.0*h*(1.0+float64(cal.H3)/67108864.0*h)))
	h = h * (1.0 - float64(cal.H1)*h/524288.0)
	if h > 100 {
		h = 100
	}
	if h < 0 {
		h = 0
	}
	return h * 1024.0 // caller divides by 1024
}

func writeReg(f *os.File, reg, val byte) error {
	_, err := f.Write([]byte{reg, val})
	return err
}

func readRegs(f *os.File, reg byte, buf []byte) error {
	if _, err := f.Write([]byte{reg}); err != nil {
		return err
	}
	_, err := f.Read(buf)
	return err
}
