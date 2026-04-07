// internal/service/vpndirector.go
package service

import (
	"fmt"
	"path/filepath"
)

// Compile-time interface check
var _ VPNDirector = (*VPNDirectorService)(nil)

// VPNDirectorService handles VPN Director shell operations
type VPNDirectorService struct {
	scriptsDir string
	executor   ShellExecutor
}

// NewVPNDirectorService creates a new VPNDirectorService
func NewVPNDirectorService(scriptsDir string, executor ShellExecutor) *VPNDirectorService {
	if executor == nil {
		executor = DefaultExecutor()
	}
	return &VPNDirectorService{
		scriptsDir: scriptsDir,
		executor:   executor,
	}
}

func (s *VPNDirectorService) scriptPath() string {
	return filepath.Join(s.scriptsDir, "vpn-director.sh")
}

// Status returns VPN Director status
func (s *VPNDirectorService) Status() (string, error) {
	result, err := s.executor.Exec(s.scriptPath(), "status")
	if err != nil {
		return "", err
	}
	return result.Output, nil
}

// Apply applies VPN Director configuration
func (s *VPNDirectorService) Apply() error {
	result, err := s.executor.Exec(s.scriptPath(), "apply")
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("apply failed (exit %d): %s", result.ExitCode, result.Output)
	}
	return nil
}

// Restart restarts VPN Director
func (s *VPNDirectorService) Restart() error {
	result, err := s.executor.Exec(s.scriptPath(), "restart")
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("restart failed (exit %d): %s", result.ExitCode, result.Output)
	}
	return nil
}

// RestartXray restarts only Xray
func (s *VPNDirectorService) RestartXray() error {
	result, err := s.executor.Exec(s.scriptPath(), "restart", "xray")
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("restart xray failed (exit %d): %s", result.ExitCode, result.Output)
	}
	return nil
}

// Stop stops VPN Director
func (s *VPNDirectorService) Stop() error {
	result, err := s.executor.Exec(s.scriptPath(), "stop")
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("stop failed (exit %d): %s", result.ExitCode, result.Output)
	}
	return nil
}
