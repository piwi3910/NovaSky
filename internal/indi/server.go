package indi

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"
)

var validDrivers = map[string]bool{
	"indi_asi_ccd":         true,
	"indi_asi_single_ccd":  true,
	"indi_qhy_ccd":         true,
	"indi_webcam":          true,
	"indi_simulator_ccd":   true,
	"indi_sv305_ccd":       true,
	"indi_playerone_ccd":   true,
	"indi_toupbase_ccd":    true,
}

type Server struct {
	cmd    *exec.Cmd
	Driver string
	Port   int
}

func NewServer(driver string, port int) *Server {
	return &Server{
		Driver: driver,
		Port:   port,
	}
}

func (s *Server) Start() error {
	if !validDrivers[s.Driver] && !strings.HasPrefix(s.Driver, "indi_") {
		return fmt.Errorf("invalid INDI driver: %s", s.Driver)
	}

	binPath := "/usr/local/bin/indiserver"
	driverPath := fmt.Sprintf("/usr/local/bin/%s", s.Driver)

	s.cmd = exec.Command(binPath, "-p", fmt.Sprintf("%d", s.Port), driverPath)
	s.cmd.Stdout = nil
	s.cmd.Stderr = nil

	if err := s.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start indiserver: %w", err)
	}

	log.Printf("[indi] indiserver started with driver %s on port %d (pid %d)", s.Driver, s.Port, s.cmd.Process.Pid)

	// Give it time to initialize
	time.Sleep(5 * time.Second)
	return nil
}

func (s *Server) Stop() error {
	if s.cmd != nil && s.cmd.Process != nil {
		log.Println("[indi] Stopping indiserver")
		if err := s.cmd.Process.Kill(); err != nil {
			return err
		}
		s.cmd.Wait() //nolint:errcheck
		s.cmd = nil
	}
	return nil
}

func (s *Server) IsRunning() bool {
	return s.cmd != nil && s.cmd.Process != nil
}
