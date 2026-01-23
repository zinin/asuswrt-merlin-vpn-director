package wizard

import (
	"errors"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/vpnconfig"
)

// mockVPNDirector for testing
type mockVPNDirector struct {
	applyCalled      bool
	restartXrayCalled bool
	applyErr         error
	restartXrayErr   error
}

func (m *mockVPNDirector) Status() (string, error) { return "", nil }
func (m *mockVPNDirector) Apply() error {
	m.applyCalled = true
	return m.applyErr
}
func (m *mockVPNDirector) Restart() error        { return nil }
func (m *mockVPNDirector) RestartXray() error {
	m.restartXrayCalled = true
	return m.restartXrayErr
}
func (m *mockVPNDirector) Stop() error { return nil }

// mockXrayGenerator for testing
type mockXrayGenerator struct {
	generateCalled bool
	generatedServer vpnconfig.Server
	generateErr    error
}

func (m *mockXrayGenerator) GenerateConfig(server vpnconfig.Server) error {
	m.generateCalled = true
	m.generatedServer = server
	return m.generateErr
}

// trackingConfigStore extends mockConfigStore to track saves
type trackingConfigStore struct {
	servers         []vpnconfig.Server
	vpnConfig       *vpnconfig.VPNDirectorConfig
	loadErr         error
	saveErr         error
	savedConfig     *vpnconfig.VPNDirectorConfig
	saveConfigCalled bool
}

func (m *trackingConfigStore) LoadServers() ([]vpnconfig.Server, error) {
	return m.servers, m.loadErr
}

func (m *trackingConfigStore) LoadVPNConfig() (*vpnconfig.VPNDirectorConfig, error) {
	return m.vpnConfig, m.loadErr
}

func (m *trackingConfigStore) SaveVPNConfig(cfg *vpnconfig.VPNDirectorConfig) error {
	m.saveConfigCalled = true
	m.savedConfig = cfg
	return m.saveErr
}

func (m *trackingConfigStore) SaveServers([]vpnconfig.Server) error {
	return m.saveErr
}

func (m *trackingConfigStore) DataDir() (string, error) {
	return "/opt/vpn-director/data", m.loadErr
}

func (m *trackingConfigStore) DataDirOrDefault() string {
	return "/opt/vpn-director/data"
}

func (m *trackingConfigStore) ScriptsDir() string {
	return "/opt/vpn-director"
}

// trackingSender extends mockSender to track messages
type trackingSender struct {
	messages []string
	sendErr  error
}

func (m *trackingSender) Send(chatID int64, text string) error {
	m.messages = append(m.messages, text)
	return m.sendErr
}

func (m *trackingSender) SendPlain(chatID int64, text string) error {
	m.messages = append(m.messages, text)
	return m.sendErr
}

func (m *trackingSender) SendLongPlain(chatID int64, text string) error {
	m.messages = append(m.messages, text)
	return m.sendErr
}

func (m *trackingSender) SendWithKeyboard(chatID int64, text string, kb tgbotapi.InlineKeyboardMarkup) error {
	m.messages = append(m.messages, text)
	return m.sendErr
}

func (m *trackingSender) SendCodeBlock(chatID int64, header, content string) error {
	m.messages = append(m.messages, header+"\n"+content)
	return m.sendErr
}

func (m *trackingSender) EditMessage(chatID int64, msgID int, text string, kb tgbotapi.InlineKeyboardMarkup) error {
	m.messages = append(m.messages, text)
	return m.sendErr
}

func (m *trackingSender) AckCallback(callbackID string) error {
	return nil
}

// trackingManager tracks Clear calls
type trackingManager struct {
	clearedChatIDs []int64
}

func (m *trackingManager) Clear(chatID int64) {
	m.clearedChatIDs = append(m.clearedChatIDs, chatID)
}

func TestApplier_Apply_Success(t *testing.T) {
	t.Run("successfully applies configuration and clears state", func(t *testing.T) {
		manager := &trackingManager{}
		sender := &trackingSender{}
		configStore := &trackingConfigStore{
			servers: []vpnconfig.Server{
				{Name: "Server1", IP: "1.2.3.4", Address: "srv1.example.com", Port: 443, UUID: "uuid-1"},
				{Name: "Server2", IP: "5.6.7.8", Address: "srv2.example.com", Port: 443, UUID: "uuid-2"},
			},
			vpnConfig: &vpnconfig.VPNDirectorConfig{
				DataDir: "/opt/vpn-director/data",
				Xray:    vpnconfig.XrayConfig{},
				TunnelDirector: vpnconfig.TunnelDirectorConfig{
					Tunnels: make(map[string]vpnconfig.TunnelConfig),
				},
			},
		}
		vpnDirector := &mockVPNDirector{}
		xrayGen := &mockXrayGenerator{}

		applier := NewApplier(manager, sender, configStore, vpnDirector, xrayGen)

		state := &State{
			ChatID:      123,
			Step:        StepConfirm,
			ServerIndex: 0,
			Exclusions:  map[string]bool{"ru": true, "by": true},
			Clients: []ClientRoute{
				{IP: "192.168.1.10", Route: "xray"},
				{IP: "192.168.1.20", Route: "wgc1"},
			},
		}

		err := applier.Apply(123, state)

		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}

		// Verify state was cleared
		if len(manager.clearedChatIDs) != 1 || manager.clearedChatIDs[0] != 123 {
			t.Errorf("expected state to be cleared for chatID 123, got: %v", manager.clearedChatIDs)
		}

		// Verify config was saved
		if !configStore.saveConfigCalled {
			t.Error("expected SaveVPNConfig to be called")
		}

		// Verify xray clients
		if len(configStore.savedConfig.Xray.Clients) != 1 {
			t.Errorf("expected 1 xray client, got %d", len(configStore.savedConfig.Xray.Clients))
		}
		if configStore.savedConfig.Xray.Clients[0] != "192.168.1.10" {
			t.Errorf("expected xray client '192.168.1.10', got '%s'", configStore.savedConfig.Xray.Clients[0])
		}

		// Verify tunnel config
		if len(configStore.savedConfig.TunnelDirector.Tunnels) != 1 {
			t.Errorf("expected 1 tunnel, got %d", len(configStore.savedConfig.TunnelDirector.Tunnels))
		}
		tunnel, ok := configStore.savedConfig.TunnelDirector.Tunnels["wgc1"]
		if !ok {
			t.Fatal("expected tunnel 'wgc1' to exist")
		}
		if len(tunnel.Clients) != 1 || tunnel.Clients[0] != "192.168.1.20/32" {
			t.Errorf("expected tunnel client '192.168.1.20/32', got %v", tunnel.Clients)
		}

		// Verify xray config was generated
		if !xrayGen.generateCalled {
			t.Error("expected GenerateConfig to be called")
		}
		if xrayGen.generatedServer.Name != "Server1" {
			t.Errorf("expected server 'Server1', got '%s'", xrayGen.generatedServer.Name)
		}

		// Verify VPN apply was called
		if !vpnDirector.applyCalled {
			t.Error("expected Apply to be called")
		}

		// Verify Xray restart was called
		if !vpnDirector.restartXrayCalled {
			t.Error("expected RestartXray to be called")
		}

		// Verify messages were sent
		if len(sender.messages) < 5 {
			t.Errorf("expected at least 5 messages, got %d", len(sender.messages))
		}
	})
}

func TestApplier_Apply_DefaultExclusions(t *testing.T) {
	t.Run("uses default exclusion 'ru' when none selected", func(t *testing.T) {
		manager := &trackingManager{}
		sender := &trackingSender{}
		configStore := &trackingConfigStore{
			servers: []vpnconfig.Server{
				{Name: "Server1", IP: "1.2.3.4"},
			},
			vpnConfig: &vpnconfig.VPNDirectorConfig{
				DataDir: "/opt/vpn-director/data",
				Xray:    vpnconfig.XrayConfig{},
				TunnelDirector: vpnconfig.TunnelDirectorConfig{
					Tunnels: make(map[string]vpnconfig.TunnelConfig),
				},
			},
		}
		vpnDirector := &mockVPNDirector{}
		xrayGen := &mockXrayGenerator{}

		applier := NewApplier(manager, sender, configStore, vpnDirector, xrayGen)

		state := &State{
			ChatID:      123,
			Step:        StepConfirm,
			ServerIndex: 0,
			Exclusions:  map[string]bool{}, // Empty
			Clients: []ClientRoute{
				{IP: "192.168.1.10", Route: "wgc1"},
			},
		}

		_ = applier.Apply(123, state)

		// Verify default exclusion
		if len(configStore.savedConfig.Xray.ExcludeSets) != 1 {
			t.Errorf("expected 1 exclusion, got %d", len(configStore.savedConfig.Xray.ExcludeSets))
		}
		if configStore.savedConfig.Xray.ExcludeSets[0] != "ru" {
			t.Errorf("expected default exclusion 'ru', got '%s'", configStore.savedConfig.Xray.ExcludeSets[0])
		}

		tunnel := configStore.savedConfig.TunnelDirector.Tunnels["wgc1"]
		if len(tunnel.Exclude) != 1 || tunnel.Exclude[0] != "ru" {
			t.Errorf("expected tunnel exclusion 'ru', got %v", tunnel.Exclude)
		}
	})
}

func TestApplier_Apply_ClearsStateOnError(t *testing.T) {
	t.Run("clears state even on config load error", func(t *testing.T) {
		manager := &trackingManager{}
		sender := &trackingSender{}
		configStore := &trackingConfigStore{
			loadErr: errors.New("load error"),
		}
		vpnDirector := &mockVPNDirector{}
		xrayGen := &mockXrayGenerator{}

		applier := NewApplier(manager, sender, configStore, vpnDirector, xrayGen)

		state := &State{
			ChatID:     123,
			Step:       StepConfirm,
			Exclusions: make(map[string]bool),
		}

		err := applier.Apply(123, state)

		// Should return error
		if err == nil {
			t.Error("expected error, got nil")
		}

		// State should still be cleared
		if len(manager.clearedChatIDs) != 1 || manager.clearedChatIDs[0] != 123 {
			t.Errorf("expected state to be cleared on error, got: %v", manager.clearedChatIDs)
		}
	})

	t.Run("clears state even on VPN apply error", func(t *testing.T) {
		manager := &trackingManager{}
		sender := &trackingSender{}
		configStore := &trackingConfigStore{
			servers: []vpnconfig.Server{
				{Name: "Server1", IP: "1.2.3.4"},
			},
			vpnConfig: &vpnconfig.VPNDirectorConfig{
				DataDir: "/opt/vpn-director/data",
				Xray:    vpnconfig.XrayConfig{},
				TunnelDirector: vpnconfig.TunnelDirectorConfig{
					Tunnels: make(map[string]vpnconfig.TunnelConfig),
				},
			},
		}
		vpnDirector := &mockVPNDirector{
			applyErr: errors.New("apply failed"),
		}
		xrayGen := &mockXrayGenerator{}

		applier := NewApplier(manager, sender, configStore, vpnDirector, xrayGen)

		state := &State{
			ChatID:      123,
			Step:        StepConfirm,
			ServerIndex: 0,
			Exclusions:  map[string]bool{"ru": true},
			Clients:     []ClientRoute{},
		}

		err := applier.Apply(123, state)

		// Should return error
		if err == nil {
			t.Error("expected error, got nil")
		}

		// State should still be cleared
		if len(manager.clearedChatIDs) != 1 || manager.clearedChatIDs[0] != 123 {
			t.Errorf("expected state to be cleared on error, got: %v", manager.clearedChatIDs)
		}
	})
}

func TestApplier_Apply_ServerIPs(t *testing.T) {
	t.Run("collects unique server IPs sorted", func(t *testing.T) {
		manager := &trackingManager{}
		sender := &trackingSender{}
		configStore := &trackingConfigStore{
			servers: []vpnconfig.Server{
				{Name: "Server1", IP: "5.6.7.8"},
				{Name: "Server2", IP: "1.2.3.4"},
				{Name: "Server3", IP: "5.6.7.8"}, // Duplicate
			},
			vpnConfig: &vpnconfig.VPNDirectorConfig{
				DataDir: "/opt/vpn-director/data",
				Xray:    vpnconfig.XrayConfig{},
				TunnelDirector: vpnconfig.TunnelDirectorConfig{
					Tunnels: make(map[string]vpnconfig.TunnelConfig),
				},
			},
		}
		vpnDirector := &mockVPNDirector{}
		xrayGen := &mockXrayGenerator{}

		applier := NewApplier(manager, sender, configStore, vpnDirector, xrayGen)

		state := &State{
			ChatID:      123,
			Step:        StepConfirm,
			ServerIndex: 0,
			Exclusions:  map[string]bool{"ru": true},
			Clients:     []ClientRoute{},
		}

		_ = applier.Apply(123, state)

		// Verify unique and sorted server IPs
		if len(configStore.savedConfig.Xray.Servers) != 2 {
			t.Errorf("expected 2 unique server IPs, got %d", len(configStore.savedConfig.Xray.Servers))
		}
		if configStore.savedConfig.Xray.Servers[0] != "1.2.3.4" {
			t.Errorf("expected first server IP '1.2.3.4', got '%s'", configStore.savedConfig.Xray.Servers[0])
		}
		if configStore.savedConfig.Xray.Servers[1] != "5.6.7.8" {
			t.Errorf("expected second server IP '5.6.7.8', got '%s'", configStore.savedConfig.Xray.Servers[1])
		}
	})
}

func TestApplier_Apply_MultipleClientsToSameTunnel(t *testing.T) {
	t.Run("groups multiple clients into same tunnel", func(t *testing.T) {
		manager := &trackingManager{}
		sender := &trackingSender{}
		configStore := &trackingConfigStore{
			servers: []vpnconfig.Server{
				{Name: "Server1", IP: "1.2.3.4"},
			},
			vpnConfig: &vpnconfig.VPNDirectorConfig{
				DataDir: "/opt/vpn-director/data",
				Xray:    vpnconfig.XrayConfig{},
				TunnelDirector: vpnconfig.TunnelDirectorConfig{
					Tunnels: make(map[string]vpnconfig.TunnelConfig),
				},
			},
		}
		vpnDirector := &mockVPNDirector{}
		xrayGen := &mockXrayGenerator{}

		applier := NewApplier(manager, sender, configStore, vpnDirector, xrayGen)

		state := &State{
			ChatID:      123,
			Step:        StepConfirm,
			ServerIndex: 0,
			Exclusions:  map[string]bool{"ru": true},
			Clients: []ClientRoute{
				{IP: "192.168.1.10", Route: "wgc1"},
				{IP: "192.168.1.20", Route: "wgc1"},
				{IP: "192.168.1.30/24", Route: "wgc1"}, // Already has subnet
			},
		}

		_ = applier.Apply(123, state)

		tunnel := configStore.savedConfig.TunnelDirector.Tunnels["wgc1"]
		if len(tunnel.Clients) != 3 {
			t.Errorf("expected 3 clients in tunnel, got %d", len(tunnel.Clients))
		}

		// Check that /32 is added only when needed
		expected := []string{"192.168.1.10/32", "192.168.1.20/32", "192.168.1.30/24"}
		for i, exp := range expected {
			if tunnel.Clients[i] != exp {
				t.Errorf("expected client[%d]='%s', got '%s'", i, exp, tunnel.Clients[i])
			}
		}
	})
}

func TestApplier_Apply_SkipsXrayGenForInvalidServerIndex(t *testing.T) {
	t.Run("skips Xray config generation for invalid server index", func(t *testing.T) {
		manager := &trackingManager{}
		sender := &trackingSender{}
		configStore := &trackingConfigStore{
			servers: []vpnconfig.Server{
				{Name: "Server1", IP: "1.2.3.4"},
			},
			vpnConfig: &vpnconfig.VPNDirectorConfig{
				DataDir: "/opt/vpn-director/data",
				Xray:    vpnconfig.XrayConfig{},
				TunnelDirector: vpnconfig.TunnelDirectorConfig{
					Tunnels: make(map[string]vpnconfig.TunnelConfig),
				},
			},
		}
		vpnDirector := &mockVPNDirector{}
		xrayGen := &mockXrayGenerator{}

		applier := NewApplier(manager, sender, configStore, vpnDirector, xrayGen)

		state := &State{
			ChatID:      123,
			Step:        StepConfirm,
			ServerIndex: 5, // Out of bounds
			Exclusions:  map[string]bool{"ru": true},
			Clients:     []ClientRoute{},
		}

		_ = applier.Apply(123, state)

		// Xray generation should be skipped
		if xrayGen.generateCalled {
			t.Error("expected GenerateConfig NOT to be called for invalid index")
		}

		// VPN apply should still be called
		if !vpnDirector.applyCalled {
			t.Error("expected Apply to be called even with invalid server index")
		}

		// Should have warning message about invalid server
		hasWarning := false
		for _, msg := range sender.messages {
			if msg == "Warning: Invalid server selection, Xray config not updated" {
				hasWarning = true
				break
			}
		}
		if !hasWarning {
			t.Error("expected warning message about invalid server selection")
		}
	})
}

func TestApplier_Apply_XrayGenerateError(t *testing.T) {
	t.Run("continues on Xray generation error", func(t *testing.T) {
		manager := &trackingManager{}
		sender := &trackingSender{}
		configStore := &trackingConfigStore{
			servers: []vpnconfig.Server{
				{Name: "Server1", IP: "1.2.3.4"},
			},
			vpnConfig: &vpnconfig.VPNDirectorConfig{
				DataDir: "/opt/vpn-director/data",
				Xray:    vpnconfig.XrayConfig{},
				TunnelDirector: vpnconfig.TunnelDirectorConfig{
					Tunnels: make(map[string]vpnconfig.TunnelConfig),
				},
			},
		}
		vpnDirector := &mockVPNDirector{}
		xrayGen := &mockXrayGenerator{
			generateErr: errors.New("template error"),
		}

		applier := NewApplier(manager, sender, configStore, vpnDirector, xrayGen)

		state := &State{
			ChatID:      123,
			Step:        StepConfirm,
			ServerIndex: 0,
			Exclusions:  map[string]bool{"ru": true},
			Clients:     []ClientRoute{},
		}

		err := applier.Apply(123, state)

		// Should succeed despite xray generation error
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}

		// Xray generation was attempted
		if !xrayGen.generateCalled {
			t.Error("expected GenerateConfig to be called")
		}

		// VPN apply should still be called
		if !vpnDirector.applyCalled {
			t.Error("expected Apply to be called despite xray error")
		}

		// Should have error message about xray generation
		hasError := false
		for _, msg := range sender.messages {
			if msg == "Xray config generation error: template error" {
				hasError = true
				break
			}
		}
		if !hasError {
			t.Error("expected error message about xray generation failure")
		}
	})
}

func TestApplier_Apply_SaveConfigError(t *testing.T) {
	t.Run("returns error on save config failure", func(t *testing.T) {
		manager := &trackingManager{}
		sender := &trackingSender{}
		configStore := &trackingConfigStore{
			servers: []vpnconfig.Server{
				{Name: "Server1", IP: "1.2.3.4"},
			},
			vpnConfig: &vpnconfig.VPNDirectorConfig{
				DataDir: "/opt/vpn-director/data",
				Xray:    vpnconfig.XrayConfig{},
				TunnelDirector: vpnconfig.TunnelDirectorConfig{
					Tunnels: make(map[string]vpnconfig.TunnelConfig),
				},
			},
			saveErr: errors.New("disk full"),
		}
		vpnDirector := &mockVPNDirector{}
		xrayGen := &mockXrayGenerator{}

		applier := NewApplier(manager, sender, configStore, vpnDirector, xrayGen)

		state := &State{
			ChatID:      123,
			Step:        StepConfirm,
			ServerIndex: 0,
			Exclusions:  map[string]bool{"ru": true},
			Clients:     []ClientRoute{},
		}

		err := applier.Apply(123, state)

		// Should return error
		if err == nil {
			t.Error("expected error, got nil")
		}

		// State should still be cleared
		if len(manager.clearedChatIDs) != 1 {
			t.Error("expected state to be cleared on save error")
		}

		// VPN apply should NOT be called
		if vpnDirector.applyCalled {
			t.Error("expected Apply NOT to be called on save error")
		}

		// Should have error message
		hasError := false
		for _, msg := range sender.messages {
			if msg == "Save error: disk full" {
				hasError = true
				break
			}
		}
		if !hasError {
			t.Error("expected error message about save failure")
		}
	})
}

func TestApplier_Apply_RestartXrayError(t *testing.T) {
	t.Run("returns error on Xray restart failure", func(t *testing.T) {
		manager := &trackingManager{}
		sender := &trackingSender{}
		configStore := &trackingConfigStore{
			servers: []vpnconfig.Server{
				{Name: "Server1", IP: "1.2.3.4"},
			},
			vpnConfig: &vpnconfig.VPNDirectorConfig{
				DataDir: "/opt/vpn-director/data",
				Xray:    vpnconfig.XrayConfig{},
				TunnelDirector: vpnconfig.TunnelDirectorConfig{
					Tunnels: make(map[string]vpnconfig.TunnelConfig),
				},
			},
		}
		vpnDirector := &mockVPNDirector{
			restartXrayErr: errors.New("xray not running"),
		}
		xrayGen := &mockXrayGenerator{}

		applier := NewApplier(manager, sender, configStore, vpnDirector, xrayGen)

		state := &State{
			ChatID:      123,
			Step:        StepConfirm,
			ServerIndex: 0,
			Exclusions:  map[string]bool{"ru": true},
			Clients:     []ClientRoute{},
		}

		err := applier.Apply(123, state)

		// Should return error
		if err == nil {
			t.Error("expected error, got nil")
		}

		// VPN apply was called
		if !vpnDirector.applyCalled {
			t.Error("expected Apply to be called before restart error")
		}

		// RestartXray was called
		if !vpnDirector.restartXrayCalled {
			t.Error("expected RestartXray to be called")
		}

		// State should still be cleared
		if len(manager.clearedChatIDs) != 1 {
			t.Error("expected state to be cleared on restart error")
		}
	})
}

func TestApplier_Apply_ExclusionsSorted(t *testing.T) {
	t.Run("exclusions are sorted for deterministic config", func(t *testing.T) {
		manager := &trackingManager{}
		sender := &trackingSender{}
		configStore := &trackingConfigStore{
			servers: []vpnconfig.Server{
				{Name: "Server1", IP: "1.2.3.4"},
			},
			vpnConfig: &vpnconfig.VPNDirectorConfig{
				DataDir: "/opt/vpn-director/data",
				Xray:    vpnconfig.XrayConfig{},
				TunnelDirector: vpnconfig.TunnelDirectorConfig{
					Tunnels: make(map[string]vpnconfig.TunnelConfig),
				},
			},
		}
		vpnDirector := &mockVPNDirector{}
		xrayGen := &mockXrayGenerator{}

		applier := NewApplier(manager, sender, configStore, vpnDirector, xrayGen)

		state := &State{
			ChatID:      123,
			Step:        StepConfirm,
			ServerIndex: 0,
			Exclusions:  map[string]bool{"ua": true, "by": true, "ru": true},
			Clients: []ClientRoute{
				{IP: "192.168.1.10", Route: "wgc1"},
			},
		}

		_ = applier.Apply(123, state)

		// Verify exclusions are sorted
		expected := []string{"by", "ru", "ua"}
		if len(configStore.savedConfig.Xray.ExcludeSets) != 3 {
			t.Fatalf("expected 3 exclusions, got %d", len(configStore.savedConfig.Xray.ExcludeSets))
		}
		for i, exp := range expected {
			if configStore.savedConfig.Xray.ExcludeSets[i] != exp {
				t.Errorf("expected exclusion[%d]='%s', got '%s'", i, exp, configStore.savedConfig.Xray.ExcludeSets[i])
			}
		}
	})
}

func TestApplier_Apply_FiltersEmptyServerIPs(t *testing.T) {
	t.Run("filters out empty server IPs", func(t *testing.T) {
		manager := &trackingManager{}
		sender := &trackingSender{}
		configStore := &trackingConfigStore{
			servers: []vpnconfig.Server{
				{Name: "Server1", IP: "1.2.3.4"},
				{Name: "Server2", IP: ""},          // Empty IP
				{Name: "Server3", IP: "5.6.7.8"},
			},
			vpnConfig: &vpnconfig.VPNDirectorConfig{
				DataDir: "/opt/vpn-director/data",
				Xray:    vpnconfig.XrayConfig{},
				TunnelDirector: vpnconfig.TunnelDirectorConfig{
					Tunnels: make(map[string]vpnconfig.TunnelConfig),
				},
			},
		}
		vpnDirector := &mockVPNDirector{}
		xrayGen := &mockXrayGenerator{}

		applier := NewApplier(manager, sender, configStore, vpnDirector, xrayGen)

		state := &State{
			ChatID:      123,
			Step:        StepConfirm,
			ServerIndex: 0,
			Exclusions:  map[string]bool{"ru": true},
			Clients:     []ClientRoute{},
		}

		_ = applier.Apply(123, state)

		// Should have only 2 server IPs (empty filtered out)
		if len(configStore.savedConfig.Xray.Servers) != 2 {
			t.Errorf("expected 2 server IPs, got %d: %v", len(configStore.savedConfig.Xray.Servers), configStore.savedConfig.Xray.Servers)
		}
	})
}

func TestApplier_Apply_SkipsInvalidRoutes(t *testing.T) {
	t.Run("skips clients with invalid routes", func(t *testing.T) {
		manager := &trackingManager{}
		sender := &trackingSender{}
		configStore := &trackingConfigStore{
			servers: []vpnconfig.Server{
				{Name: "Server1", IP: "1.2.3.4"},
			},
			vpnConfig: &vpnconfig.VPNDirectorConfig{
				DataDir: "/opt/vpn-director/data",
				Xray:    vpnconfig.XrayConfig{},
				TunnelDirector: vpnconfig.TunnelDirectorConfig{
					Tunnels: make(map[string]vpnconfig.TunnelConfig),
				},
			},
		}
		vpnDirector := &mockVPNDirector{}
		xrayGen := &mockXrayGenerator{}

		applier := NewApplier(manager, sender, configStore, vpnDirector, xrayGen)

		state := &State{
			ChatID:      123,
			Step:        StepConfirm,
			ServerIndex: 0,
			Exclusions:  map[string]bool{"ru": true},
			Clients: []ClientRoute{
				{IP: "192.168.1.10", Route: "xray"},
				{IP: "192.168.1.20", Route: "invalid_route"}, // Invalid
				{IP: "192.168.1.30", Route: "wgc1"},
			},
		}

		_ = applier.Apply(123, state)

		// Should have 1 xray client
		if len(configStore.savedConfig.Xray.Clients) != 1 {
			t.Errorf("expected 1 xray client, got %d", len(configStore.savedConfig.Xray.Clients))
		}

		// Should have 1 tunnel (wgc1 only, invalid_route skipped)
		if len(configStore.savedConfig.TunnelDirector.Tunnels) != 1 {
			t.Errorf("expected 1 tunnel, got %d", len(configStore.savedConfig.TunnelDirector.Tunnels))
		}
		if _, ok := configStore.savedConfig.TunnelDirector.Tunnels["wgc1"]; !ok {
			t.Error("expected tunnel 'wgc1' to exist")
		}
		if _, ok := configStore.savedConfig.TunnelDirector.Tunnels["invalid_route"]; ok {
			t.Error("expected tunnel 'invalid_route' NOT to exist")
		}
	})
}
