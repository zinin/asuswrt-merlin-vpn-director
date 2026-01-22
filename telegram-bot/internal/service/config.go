// internal/service/config.go
package service

import (
	"os"
	"path/filepath"

	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/vpnconfig"
)

// ConfigService handles vpn-director configuration operations
type ConfigService struct {
	scriptsDir     string
	defaultDataDir string
}

// Compile-time check that ConfigService implements ConfigStore
var _ ConfigStore = (*ConfigService)(nil)

// NewConfigService creates a new ConfigService
func NewConfigService(scriptsDir, defaultDataDir string) *ConfigService {
	return &ConfigService{
		scriptsDir:     scriptsDir,
		defaultDataDir: defaultDataDir,
	}
}

// ConfigPath returns the path to vpn-director.json
func (s *ConfigService) ConfigPath() string {
	return filepath.Join(s.scriptsDir, "vpn-director.json")
}

// ScriptsDir returns the scripts directory
func (s *ConfigService) ScriptsDir() string {
	return s.scriptsDir
}

// DataDir returns the data directory from config (no caching, returns error)
func (s *ConfigService) DataDir() (string, error) {
	cfg, err := s.LoadVPNConfig()
	if err != nil {
		return "", err
	}
	if cfg.DataDir != "" {
		return cfg.DataDir, nil
	}
	return s.defaultDataDir, nil
}

// DataDirOrDefault returns data directory, falling back to default on error
// Used by /import when vpn-director.json may not exist
func (s *ConfigService) DataDirOrDefault() string {
	dataDir, err := s.DataDir()
	if err != nil || dataDir == "" {
		return s.defaultDataDir
	}
	return dataDir
}

// LoadVPNConfig loads the VPN Director configuration
func (s *ConfigService) LoadVPNConfig() (*vpnconfig.VPNDirectorConfig, error) {
	return vpnconfig.LoadVPNDirectorConfig(s.ConfigPath())
}

// SaveVPNConfig saves the VPN Director configuration
func (s *ConfigService) SaveVPNConfig(cfg *vpnconfig.VPNDirectorConfig) error {
	return vpnconfig.SaveVPNDirectorConfig(s.ConfigPath(), cfg)
}

// LoadServers loads the servers list
func (s *ConfigService) LoadServers() ([]vpnconfig.Server, error) {
	dataDir, err := s.DataDir()
	if err != nil {
		return nil, err
	}
	return vpnconfig.LoadServers(filepath.Join(dataDir, "servers.json"))
}

// SaveServers saves the servers list (creates directory if needed)
func (s *ConfigService) SaveServers(servers []vpnconfig.Server) error {
	dataDir, err := s.DataDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return err
	}
	return vpnconfig.SaveServers(filepath.Join(dataDir, "servers.json"), servers)
}
