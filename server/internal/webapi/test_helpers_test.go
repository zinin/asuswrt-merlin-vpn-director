package webapi

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/auth"
	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/service"
	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/vpnconfig"
)

// mockVPN implements service.VPNDirector for testing.
type mockVPN struct {
	statusOutput string
	err          error
	applyCalls   int // number of Apply() calls, to assert auto-apply
	updateCalls  int // number of Update() calls
}

func (m *mockVPN) Status() (string, error) { return m.statusOutput, m.err }
func (m *mockVPN) Apply() error            { m.applyCalls++; return m.err }
func (m *mockVPN) Restart() error          { return m.err }
func (m *mockVPN) RestartXray() error      { return m.err }
func (m *mockVPN) Stop() error             { return m.err }
func (m *mockVPN) Update() error           { m.updateCalls++; return m.err }

// mockNetwork implements service.NetworkInfo for testing.
type mockNetwork struct {
	ip  string
	err error
}

func (m *mockNetwork) GetExternalIP() (string, error) { return m.ip, m.err }

// mockLogs implements service.LogReader for testing and records the paths read.
type mockLogs struct {
	output string
	err    error
	paths  []string
}

func (m *mockLogs) Read(path string, _ int) (string, error) {
	m.paths = append(m.paths, path)
	return m.output, m.err
}

// mockConfig implements service.ConfigStore for testing.
type mockConfig struct {
	cfg           *vpnconfig.VPNDirectorConfig
	servers       []vpnconfig.Server
	err           error
	saveVPNCfgErr error                        // independent error for the save step of UpdateVPNConfig
	savedCfg      *vpnconfig.VPNDirectorConfig // captured by UpdateVPNConfig
	savedServers  []vpnconfig.Server           // captured by SaveServers
	updateErr     error                        // returned by UpdateVPNConfig before fn runs, e.g. service.ErrConfigLockTimeout
}

func (m *mockConfig) LoadVPNConfig() (*vpnconfig.VPNDirectorConfig, error) {
	return m.cfg, m.err
}
func (m *mockConfig) LoadServers() ([]vpnconfig.Server, error) { return m.servers, m.err }
func (m *mockConfig) SaveServers(servers []vpnconfig.Server) error {
	m.savedServers = servers
	return m.err
}

// UpdateVPNConfig mirrors the real contract: a load failure (err or no cfg)
// comes back wrapped in service.ErrConfigLoad before fn runs; an fn error
// skips the save; otherwise the mutated cfg is recorded as savedCfg and
// saveVPNCfgErr, if set, is returned after it.
func (m *mockConfig) UpdateVPNConfig(fn func(*vpnconfig.VPNDirectorConfig) error) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	if m.err != nil {
		return fmt.Errorf("%w: %w", service.ErrConfigLoad, m.err)
	}
	if m.cfg == nil {
		return fmt.Errorf("%w: %w", service.ErrConfigLoad, os.ErrNotExist)
	}
	if err := fn(m.cfg); err != nil {
		return err
	}
	m.savedCfg = m.cfg
	return m.saveVPNCfgErr
}

func (m *mockConfig) DataDir() (string, error) { return "/tmp/test-data", m.err }
func (m *mockConfig) DataDirOrDefault() string { return "/tmp/test-data" }
func (m *mockConfig) ScriptsDir() string       { return "/tmp/test-scripts" }

// mockXray implements service.XrayGenerator for testing.
type mockXray struct {
	err error
}

func (m *mockXray) GenerateConfig(_ vpnconfig.Server) error { return m.err }

// mockShadow implements password verification for testing.
// It acts as a thin wrapper that allows tests to control Verify results.
type mockShadow struct {
	validUser string
	validPass string
}

func (m *mockShadow) verify(username, password string) (bool, error) {
	if username == m.validUser && password == m.validPass {
		return true, nil
	}
	return false, nil
}

// newTestDeps creates a Deps instance wired with test mocks.
func newTestDeps(t *testing.T) *Deps {
	t.Helper()

	// Create a real shadow file for testing is complex,
	// so we use a ShadowAuth that points to a non-existent file.
	// Tests that need login verification should use the full router
	// with a real shadow file fixture, or test at the handler level directly.
	shadow := auth.NewShadowAuth("/dev/null")

	jwt := auth.NewJWTService("test-secret-key-32bytes!!!!!!!!", 1*time.Hour)

	return &Deps{
		Config:  &mockConfig{},
		VPN:     &mockVPN{statusOutput: "all systems operational"},
		Xray:    &mockXray{},
		Network: &mockNetwork{ip: "203.0.113.42"},
		Logs:    &mockLogs{output: "log line 1\nlog line 2"},
		LogPaths: map[string]string{
			"bot":   "/tmp/test-telegram-bot.log",
			"vpn":   "/tmp/test-vpn-director.log",
			"xray":  "/tmp/test-xray-error.log",
			"webui": "/tmp/test-webui.log",
		},
		Shadow:       shadow,
		JWT:          jwt,
		Version:      "1.0.0-test",
		Commit:       "abc1234",
		OpMutex:      &sync.Mutex{},
		loginLimiter: newRateLimiter(5, 1*time.Minute, 30*time.Second),
	}
}
