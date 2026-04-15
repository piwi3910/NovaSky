package gpio

import (
	"fmt"
	"math"
	"os"
	"time"
)

// DewHeaterController implements a simple PID controller for a dew heater.
// It adjusts the duty cycle based on the difference between current temperature
// and dew point, aiming to keep the lens above the dew point.
type DewHeaterController struct {
	// PID gains
	Kp float64
	Ki float64
	Kd float64

	// Target margin above dew point in °C
	TargetDelta float64

	// Internal state
	integral  float64
	prevError float64
	lastTime  time.Time

	// PWM sysfs paths (set by Enable)
	pwmChipPath string
	pwmChannel  int
	enabled     bool
}

// NewDewHeaterController creates a new PID-controlled dew heater.
// targetDelta is the desired margin above dew point in °C.
func NewDewHeaterController(targetDelta float64) *DewHeaterController {
	return &DewHeaterController{
		Kp:          10.0, // proportional gain
		Ki:          0.5,  // integral gain
		Kd:          2.0,  // derivative gain
		TargetDelta: targetDelta,
	}
}

// Update calculates the new duty cycle (0-100) based on current temperature and dew point.
// Returns the duty cycle percentage.
func (c *DewHeaterController) Update(temp, dewPoint float64) int {
	now := time.Now()
	dt := 1.0 // default 1 second
	if !c.lastTime.IsZero() {
		dt = now.Sub(c.lastTime).Seconds()
		if dt <= 0 {
			dt = 1.0
		}
	}
	c.lastTime = now

	// Error: how far below the target margin we are
	// Positive error = lens is too close to dew point (needs more heat)
	margin := temp - dewPoint
	err := c.TargetDelta - margin

	// PID terms
	proportional := c.Kp * err

	c.integral += err * dt
	// Anti-windup: clamp integral
	if c.integral > 100 {
		c.integral = 100
	}
	if c.integral < -100 {
		c.integral = -100
	}
	integralTerm := c.Ki * c.integral

	derivative := 0.0
	if dt > 0 {
		derivative = c.Kd * (err - c.prevError) / dt
	}
	c.prevError = err

	output := proportional + integralTerm + derivative

	// Clamp to 0-100
	duty := int(math.Round(output))
	if duty < 0 {
		duty = 0
	}
	if duty > 100 {
		duty = 100
	}

	return duty
}

// EnablePWM sets up a hardware PWM channel via sysfs.
// chip is the PWM chip number (usually 0), channel is the PWM channel.
func (c *DewHeaterController) EnablePWM(chip, channel int) error {
	c.pwmChipPath = fmt.Sprintf("/sys/class/pwm/pwmchip%d", chip)
	c.pwmChannel = channel

	exportPath := c.pwmChipPath + "/export"
	if err := os.WriteFile(exportPath, []byte(fmt.Sprintf("%d", channel)), 0644); err != nil {
		// Ignore if already exported
		if !os.IsExist(err) {
			// Check if pwm directory already exists
			pwmDir := fmt.Sprintf("%s/pwm%d", c.pwmChipPath, channel)
			if _, statErr := os.Stat(pwmDir); statErr != nil {
				return fmt.Errorf("export PWM channel: %w", err)
			}
		}
	}

	pwmDir := fmt.Sprintf("%s/pwm%d", c.pwmChipPath, channel)

	// Set period to 1ms (1000000 ns) — 1kHz PWM frequency
	if err := os.WriteFile(pwmDir+"/period", []byte("1000000"), 0644); err != nil {
		return fmt.Errorf("set PWM period: %w", err)
	}

	// Enable PWM
	if err := os.WriteFile(pwmDir+"/enable", []byte("1"), 0644); err != nil {
		return fmt.Errorf("enable PWM: %w", err)
	}

	c.enabled = true
	return nil
}

// SetDutyCycle writes the duty cycle (0-100) to the PWM sysfs interface.
func (c *DewHeaterController) SetDutyCycle(duty int) error {
	if !c.enabled {
		return fmt.Errorf("PWM not enabled")
	}

	// duty_cycle in nanoseconds; period is 1000000 ns
	ns := duty * 10000 // 1% = 10000 ns
	pwmDir := fmt.Sprintf("%s/pwm%d", c.pwmChipPath, c.pwmChannel)
	return os.WriteFile(pwmDir+"/duty_cycle", []byte(fmt.Sprintf("%d", ns)), 0644)
}

// Disable turns off the PWM output.
func (c *DewHeaterController) Disable() error {
	if !c.enabled {
		return nil
	}
	pwmDir := fmt.Sprintf("%s/pwm%d", c.pwmChipPath, c.pwmChannel)
	c.enabled = false
	return os.WriteFile(pwmDir+"/enable", []byte("0"), 0644)
}
