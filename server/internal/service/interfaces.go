// internal/service/interfaces.go
package service

import (
	"context"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/shell"
	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/vpnconfig"
)

// ConfigStore, VPNDirector, XrayGenerator, NetworkInfo, LogReader interfaces are defined here
// in service/ rather than handler/ so both handler/ and wizard/ can
// import them without coupling handler <-> wizard.
//
// TODO: If the number of consumers grows or interfaces become complex,
// consider extracting to internal/contract/ for cleaner separation.

// ShellExecutor is the interface for executing shell commands. ctx bounds the
// command: services build it from context.Background() with their own
// per-command timeout, never from an HTTP request, so a closed browser tab
// cannot kill an apply half-way through its iptables changes.
type ShellExecutor interface {
	Exec(ctx context.Context, name string, args ...string) (*shell.Result, error)
}

// ConfigStore is the interface for config operations
type ConfigStore interface {
	LoadVPNConfig() (*vpnconfig.VPNDirectorConfig, error)
	LoadServers() ([]vpnconfig.Server, error)
	SaveVPNConfig(*vpnconfig.VPNDirectorConfig) error
	SaveServers([]vpnconfig.Server) error
	DataDir() (string, error)
	DataDirOrDefault() string
	ScriptsDir() string
}

// VPNDirector is the interface for VPN Director operations
type VPNDirector interface {
	Status() (string, error)
	Apply() error
	Restart() error
	RestartXray() error
	Stop() error
	// Update downloads fresh ipsets and reapplies the configuration
	// (vpn-director.sh update). Apply reuses cached ipsets instead.
	Update() error
}

// XrayGenerator is the interface for Xray config generation
type XrayGenerator interface {
	GenerateConfig(server vpnconfig.Server) error
}

// NetworkInfo is the interface for network operations
type NetworkInfo interface {
	GetExternalIP() (string, error)
}

// LogReader is the interface for log reading
type LogReader interface {
	Read(path string, lines int) (string, error)
}

// defaultExecutor wraps shell.ExecContext
type defaultExecutor struct{}

func (e *defaultExecutor) Exec(ctx context.Context, name string, args ...string) (*shell.Result, error) {
	return shell.ExecContext(ctx, name, args...)
}

// DefaultExecutor returns the default shell executor
func DefaultExecutor() ShellExecutor {
	return &defaultExecutor{}
}
