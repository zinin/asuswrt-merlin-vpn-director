// internal/service/interfaces.go
package service

import (
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/shell"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/vpnconfig"
)

// ConfigStore, VPNDirector, XrayGenerator, NetworkInfo, LogReader interfaces are defined here
// in service/ rather than handler/ so both handler/ and wizard/ can
// import them without coupling handler <-> wizard.
//
// TODO: If the number of consumers grows or interfaces become complex,
// consider extracting to internal/contract/ for cleaner separation.

// ShellExecutor is the interface for executing shell commands
type ShellExecutor interface {
	Exec(name string, args ...string) (*shell.Result, error)
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

// defaultExecutor wraps shell.Exec
type defaultExecutor struct{}

func (e *defaultExecutor) Exec(name string, args ...string) (*shell.Result, error) {
	return shell.Exec(name, args...)
}

// DefaultExecutor returns the default shell executor
func DefaultExecutor() ShellExecutor {
	return &defaultExecutor{}
}
