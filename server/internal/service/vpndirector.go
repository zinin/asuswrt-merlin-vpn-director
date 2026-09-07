// internal/service/vpndirector.go
package service

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/shell"
)

// Per-command limits for vpn-director.sh. A limit is a safety net against a
// hung script, not an expected duration: apply and update download country
// ipsets and legitimately run for minutes on a throttled link. Each limit
// includes up to two minutes the script may spend in --wait for its lock.
const (
	// StatusTimeout bounds `status`, which only reads kernel state.
	StatusTimeout = 30 * time.Second
	// ApplyTimeout bounds apply, restart, stop and restart xray.
	ApplyTimeout = 5 * time.Minute
	// UpdateTimeout bounds `update`, which re-downloads every configured
	// country set from up to three sources.
	UpdateTimeout = 15 * time.Minute
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

// run executes vpn-director.sh with args under timeout.
func (s *VPNDirectorService) run(timeout time.Duration, args ...string) (*shell.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return s.executor.Exec(ctx, s.scriptPath(), args...)
}

// runChecked runs a mutating command and turns a non-zero exit into an error
// carrying the script output, so callers can show its last line. --wait makes
// the script queue for its own lock instead of exiting 0 when another
// instance is running, which used to turn a concurrent apply into a silent
// no-op reported as success.
func (s *VPNDirectorService) runChecked(timeout time.Duration, what string, args ...string) error {
	result, err := s.run(timeout, append([]string{"--wait"}, args...)...)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("%s failed (exit %d): %s", what, result.ExitCode, result.Output)
	}
	return nil
}

// Status returns VPN Director status
func (s *VPNDirectorService) Status() (string, error) {
	result, err := s.run(StatusTimeout, "status")
	if err != nil {
		return "", err
	}
	return result.Output, nil
}

// Apply applies VPN Director configuration
func (s *VPNDirectorService) Apply() error { return s.runChecked(ApplyTimeout, "apply", "apply") }

// Restart restarts VPN Director
func (s *VPNDirectorService) Restart() error { return s.runChecked(ApplyTimeout, "restart", "restart") }

// RestartXray restarts only Xray
func (s *VPNDirectorService) RestartXray() error {
	return s.runChecked(ApplyTimeout, "restart xray", "restart", "xray")
}

// Stop stops VPN Director
func (s *VPNDirectorService) Stop() error { return s.runChecked(ApplyTimeout, "stop", "stop") }

// Update downloads fresh ipsets and reapplies VPN Director configuration
func (s *VPNDirectorService) Update() error { return s.runChecked(UpdateTimeout, "update", "update") }
