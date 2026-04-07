package handler

import (
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/vpnconfig"
)

type mockSenderClients struct {
	lastChatID   int64
	lastText     string
	lastKeyboard tgbotapi.InlineKeyboardMarkup
	editChatID   int64
	editMsgID    int
	editText     string
	editKeyboard tgbotapi.InlineKeyboardMarkup
	plainTexts   []string
}

func (m *mockSenderClients) Send(chatID int64, text string) error {
	m.lastChatID = chatID
	m.lastText = text
	return nil
}
func (m *mockSenderClients) SendPlain(chatID int64, text string) error {
	m.plainTexts = append(m.plainTexts, text)
	m.lastChatID = chatID
	return nil
}
func (m *mockSenderClients) SendLongPlain(chatID int64, text string) error { return nil }
func (m *mockSenderClients) SendWithKeyboard(chatID int64, text string, kb tgbotapi.InlineKeyboardMarkup) error {
	m.lastChatID = chatID
	m.lastText = text
	m.lastKeyboard = kb
	return nil
}
func (m *mockSenderClients) SendCodeBlock(chatID int64, header, content string) error { return nil }
func (m *mockSenderClients) EditMessage(chatID int64, msgID int, text string, kb tgbotapi.InlineKeyboardMarkup) error {
	m.editChatID = chatID
	m.editMsgID = msgID
	m.editText = text
	m.editKeyboard = kb
	return nil
}
func (m *mockSenderClients) AckCallback(callbackID string) error { return nil }

type mockConfigClients struct {
	vpnConfig   *vpnconfig.VPNDirectorConfig
	loadErr     error
	savedConfig *vpnconfig.VPNDirectorConfig
	saveErr     error
}

func (m *mockConfigClients) LoadVPNConfig() (*vpnconfig.VPNDirectorConfig, error) {
	return m.vpnConfig, m.loadErr
}
func (m *mockConfigClients) SaveVPNConfig(cfg *vpnconfig.VPNDirectorConfig) error {
	m.savedConfig = cfg
	return m.saveErr
}
func (m *mockConfigClients) LoadServers() ([]vpnconfig.Server, error)   { return nil, nil }
func (m *mockConfigClients) SaveServers(s []vpnconfig.Server) error     { return nil }
func (m *mockConfigClients) DataDir() (string, error)                   { return "/data", nil }
func (m *mockConfigClients) DataDirOrDefault() string                   { return "/data" }
func (m *mockConfigClients) ScriptsDir() string                         { return "/scripts" }

type mockVPNClients struct {
	applyErr error
}

func (m *mockVPNClients) Status() (string, error) { return "", nil }
func (m *mockVPNClients) Apply() error            { return m.applyErr }
func (m *mockVPNClients) Restart() error          { return nil }
func (m *mockVPNClients) RestartXray() error      { return nil }
func (m *mockVPNClients) Stop() error             { return nil }

func TestClientsHandler_HandleClients_WithClients(t *testing.T) {
	sender := &mockSenderClients{}
	config := &mockConfigClients{
		vpnConfig: &vpnconfig.VPNDirectorConfig{
			Xray: vpnconfig.XrayConfig{
				Clients: []string{"192.168.50.10"},
			},
			TunnelDirector: vpnconfig.TunnelDirectorConfig{
				Tunnels: map[string]vpnconfig.TunnelConfig{
					"wgc1": {Clients: []string{"192.168.50.20/32"}},
				},
			},
			PausedClients: []string{"192.168.50.20/32"},
		},
	}
	deps := &Deps{Sender: sender, Config: config}
	h := NewClientsHandler(deps)

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 100}}
	h.HandleClients(msg)

	if sender.lastChatID != 100 {
		t.Errorf("expected chatID 100, got %d", sender.lastChatID)
	}
	if !strings.Contains(sender.lastText, "192\\.168\\.50\\.10") {
		t.Errorf("expected message to contain escaped 192.168.50.10, got: %s", sender.lastText)
	}
	if !strings.Contains(sender.lastText, "192\\.168\\.50\\.20") {
		t.Errorf("expected message to contain escaped 192.168.50.20/32, got: %s", sender.lastText)
	}
	// 2 clients x 2 buttons + 1 Add row = 3 rows
	if len(sender.lastKeyboard.InlineKeyboard) != 3 {
		t.Errorf("expected 3 keyboard rows, got %d", len(sender.lastKeyboard.InlineKeyboard))
	}
}

func TestClientsHandler_HandleClients_Empty(t *testing.T) {
	sender := &mockSenderClients{}
	config := &mockConfigClients{
		vpnConfig: &vpnconfig.VPNDirectorConfig{
			Xray:           vpnconfig.XrayConfig{},
			TunnelDirector: vpnconfig.TunnelDirectorConfig{},
		},
	}
	deps := &Deps{Sender: sender, Config: config}
	h := NewClientsHandler(deps)

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 100}}
	h.HandleClients(msg)

	if len(sender.lastKeyboard.InlineKeyboard) != 1 {
		t.Errorf("expected 1 keyboard row (Add), got %d", len(sender.lastKeyboard.InlineKeyboard))
	}
}

func TestClientsHandler_HandlePause(t *testing.T) {
	sender := &mockSenderClients{}
	cfg := &vpnconfig.VPNDirectorConfig{
		Xray: vpnconfig.XrayConfig{Clients: []string{"192.168.50.10"}},
	}
	config := &mockConfigClients{vpnConfig: cfg}
	vpn := &mockVPNClients{}
	deps := &Deps{Sender: sender, Config: config, VPN: vpn}
	h := NewClientsHandler(deps)

	cb := &tgbotapi.CallbackQuery{
		Data:    "clients:pause:192.168.50.10",
		Message: &tgbotapi.Message{MessageID: 42, Chat: &tgbotapi.Chat{ID: 100}},
	}
	h.HandleCallback(cb)

	if config.savedConfig == nil {
		t.Fatal("expected config to be saved")
	}
	if len(config.savedConfig.PausedClients) != 1 || config.savedConfig.PausedClients[0] != "192.168.50.10" {
		t.Errorf("expected paused_clients=[192.168.50.10], got %v", config.savedConfig.PausedClients)
	}
	if sender.editMsgID != 42 {
		t.Errorf("expected message 42 to be edited, got %d", sender.editMsgID)
	}
}

func TestClientsHandler_HandleResume(t *testing.T) {
	sender := &mockSenderClients{}
	cfg := &vpnconfig.VPNDirectorConfig{
		Xray:          vpnconfig.XrayConfig{Clients: []string{"192.168.50.10"}},
		PausedClients: []string{"192.168.50.10"},
	}
	config := &mockConfigClients{vpnConfig: cfg}
	vpn := &mockVPNClients{}
	deps := &Deps{Sender: sender, Config: config, VPN: vpn}
	h := NewClientsHandler(deps)

	cb := &tgbotapi.CallbackQuery{
		Data:    "clients:resume:192.168.50.10",
		Message: &tgbotapi.Message{MessageID: 42, Chat: &tgbotapi.Chat{ID: 100}},
	}
	h.HandleCallback(cb)

	if config.savedConfig == nil {
		t.Fatal("expected config to be saved")
	}
	if len(config.savedConfig.PausedClients) != 0 {
		t.Errorf("expected empty paused_clients, got %v", config.savedConfig.PausedClients)
	}
}

func TestClientsHandler_HandleRemoveConfirm(t *testing.T) {
	sender := &mockSenderClients{}
	cfg := &vpnconfig.VPNDirectorConfig{
		Xray: vpnconfig.XrayConfig{Clients: []string{"192.168.50.10"}},
	}
	config := &mockConfigClients{vpnConfig: cfg}
	deps := &Deps{Sender: sender, Config: config}
	h := NewClientsHandler(deps)

	cb := &tgbotapi.CallbackQuery{
		Data:    "clients:remove:192.168.50.10",
		Message: &tgbotapi.Message{MessageID: 42, Chat: &tgbotapi.Chat{ID: 100}},
	}
	h.HandleCallback(cb)

	if sender.editMsgID != 42 {
		t.Errorf("expected message 42 to be edited, got %d", sender.editMsgID)
	}
	if len(sender.editKeyboard.InlineKeyboard) != 1 {
		t.Errorf("expected 1 keyboard row, got %d", len(sender.editKeyboard.InlineKeyboard))
	}
}

func TestClientsHandler_HandleRemoveYes_Xray(t *testing.T) {
	sender := &mockSenderClients{}
	cfg := &vpnconfig.VPNDirectorConfig{
		Xray:          vpnconfig.XrayConfig{Clients: []string{"192.168.50.10", "192.168.50.20"}},
		PausedClients: []string{"192.168.50.10"},
	}
	config := &mockConfigClients{vpnConfig: cfg}
	vpn := &mockVPNClients{}
	deps := &Deps{Sender: sender, Config: config, VPN: vpn}
	h := NewClientsHandler(deps)

	cb := &tgbotapi.CallbackQuery{
		Data:    "clients:rm_yes:192.168.50.10",
		Message: &tgbotapi.Message{MessageID: 42, Chat: &tgbotapi.Chat{ID: 100}},
	}
	h.HandleCallback(cb)

	if config.savedConfig == nil {
		t.Fatal("expected config to be saved")
	}
	if len(config.savedConfig.Xray.Clients) != 1 || config.savedConfig.Xray.Clients[0] != "192.168.50.20" {
		t.Errorf("expected xray.clients=[192.168.50.20], got %v", config.savedConfig.Xray.Clients)
	}
	if len(config.savedConfig.PausedClients) != 0 {
		t.Errorf("expected empty paused_clients, got %v", config.savedConfig.PausedClients)
	}
}

func TestClientsHandler_HandleRemoveYes_Tunnel(t *testing.T) {
	sender := &mockSenderClients{}
	cfg := &vpnconfig.VPNDirectorConfig{
		TunnelDirector: vpnconfig.TunnelDirectorConfig{
			Tunnels: map[string]vpnconfig.TunnelConfig{
				"wgc1": {Clients: []string{"192.168.50.30/32", "192.168.50.40/32"}, Exclude: []string{"ru"}},
			},
		},
	}
	config := &mockConfigClients{vpnConfig: cfg}
	vpn := &mockVPNClients{}
	deps := &Deps{Sender: sender, Config: config, VPN: vpn}
	h := NewClientsHandler(deps)

	cb := &tgbotapi.CallbackQuery{
		Data:    "clients:rm_yes:192.168.50.30/32",
		Message: &tgbotapi.Message{MessageID: 42, Chat: &tgbotapi.Chat{ID: 100}},
	}
	h.HandleCallback(cb)

	if config.savedConfig == nil {
		t.Fatal("expected config to be saved")
	}
	wgc1 := config.savedConfig.TunnelDirector.Tunnels["wgc1"]
	if len(wgc1.Clients) != 1 || wgc1.Clients[0] != "192.168.50.40/32" {
		t.Errorf("expected wgc1.clients=[192.168.50.40/32], got %v", wgc1.Clients)
	}
}

func TestClientsHandler_HandleAdd_SetsState(t *testing.T) {
	sender := &mockSenderClients{}
	config := &mockConfigClients{
		vpnConfig: &vpnconfig.VPNDirectorConfig{
			Xray:           vpnconfig.XrayConfig{Clients: []string{}},
			TunnelDirector: vpnconfig.TunnelDirectorConfig{Tunnels: map[string]vpnconfig.TunnelConfig{}},
		},
	}
	deps := &Deps{Sender: sender, Config: config}
	h := NewClientsHandler(deps)

	cb := &tgbotapi.CallbackQuery{
		Data:    "clients:add",
		Message: &tgbotapi.Message{MessageID: 42, Chat: &tgbotapi.Chat{ID: 100}},
	}
	h.HandleCallback(cb)

	if len(sender.plainTexts) == 0 {
		t.Fatal("expected a plain text message")
	}
}

func TestClientsHandler_HandleTextInput_ValidIP(t *testing.T) {
	sender := &mockSenderClients{}
	config := &mockConfigClients{
		vpnConfig: &vpnconfig.VPNDirectorConfig{
			Xray: vpnconfig.XrayConfig{Clients: []string{}},
			TunnelDirector: vpnconfig.TunnelDirectorConfig{
				Tunnels: map[string]vpnconfig.TunnelConfig{
					"wgc1": {Clients: []string{}, Exclude: []string{"ru"}},
				},
			},
		},
	}
	deps := &Deps{Sender: sender, Config: config}
	h := NewClientsHandler(deps)

	h.mu.Lock()
	h.addState[100] = ""
	h.mu.Unlock()

	msg := &tgbotapi.Message{
		Text: "192.168.50.10",
		Chat: &tgbotapi.Chat{ID: 100},
	}
	h.HandleTextInput(msg)

	if sender.lastChatID != 100 {
		t.Errorf("expected chatID 100, got %d", sender.lastChatID)
	}
	if len(sender.lastKeyboard.InlineKeyboard) == 0 {
		t.Error("expected route selection keyboard")
	}
}

func TestClientsHandler_HandleTextInput_InvalidIP(t *testing.T) {
	sender := &mockSenderClients{}
	config := &mockConfigClients{
		vpnConfig: &vpnconfig.VPNDirectorConfig{},
	}
	deps := &Deps{Sender: sender, Config: config}
	h := NewClientsHandler(deps)

	h.mu.Lock()
	h.addState[100] = ""
	h.mu.Unlock()

	msg := &tgbotapi.Message{
		Text: "not-an-ip",
		Chat: &tgbotapi.Chat{ID: 100},
	}
	h.HandleTextInput(msg)

	if len(sender.plainTexts) == 0 {
		t.Fatal("expected error message")
	}
}

func TestClientsHandler_HandleTextInput_DuplicateIP(t *testing.T) {
	sender := &mockSenderClients{}
	config := &mockConfigClients{
		vpnConfig: &vpnconfig.VPNDirectorConfig{
			Xray: vpnconfig.XrayConfig{Clients: []string{"192.168.50.10"}},
		},
	}
	deps := &Deps{Sender: sender, Config: config}
	h := NewClientsHandler(deps)

	h.mu.Lock()
	h.addState[100] = ""
	h.mu.Unlock()

	msg := &tgbotapi.Message{
		Text: "192.168.50.10",
		Chat: &tgbotapi.Chat{ID: 100},
	}
	h.HandleTextInput(msg)

	if len(sender.plainTexts) == 0 {
		t.Fatal("expected duplicate error message")
	}
}

func TestClientsHandler_HandleAddRoute_Xray(t *testing.T) {
	sender := &mockSenderClients{}
	cfg := &vpnconfig.VPNDirectorConfig{
		Xray: vpnconfig.XrayConfig{Clients: []string{}},
	}
	config := &mockConfigClients{vpnConfig: cfg}
	vpn := &mockVPNClients{}
	deps := &Deps{Sender: sender, Config: config, VPN: vpn}
	h := NewClientsHandler(deps)

	h.mu.Lock()
	h.addState[100] = "192.168.50.10"
	h.mu.Unlock()

	cb := &tgbotapi.CallbackQuery{
		Data:    "clients:route:xray",
		Message: &tgbotapi.Message{MessageID: 42, Chat: &tgbotapi.Chat{ID: 100}},
	}
	h.HandleCallback(cb)

	if config.savedConfig == nil {
		t.Fatal("expected config to be saved")
	}
	if len(config.savedConfig.Xray.Clients) != 1 || config.savedConfig.Xray.Clients[0] != "192.168.50.10" {
		t.Errorf("expected xray.clients=[192.168.50.10], got %v", config.savedConfig.Xray.Clients)
	}
}

func TestClientsHandler_HandleAddRoute_Tunnel(t *testing.T) {
	sender := &mockSenderClients{}
	cfg := &vpnconfig.VPNDirectorConfig{
		TunnelDirector: vpnconfig.TunnelDirectorConfig{
			Tunnels: map[string]vpnconfig.TunnelConfig{
				"wgc1": {Clients: []string{}, Exclude: []string{"ru"}},
			},
		},
	}
	config := &mockConfigClients{vpnConfig: cfg}
	vpn := &mockVPNClients{}
	deps := &Deps{Sender: sender, Config: config, VPN: vpn}
	h := NewClientsHandler(deps)

	h.mu.Lock()
	h.addState[100] = "192.168.50.30"
	h.mu.Unlock()

	cb := &tgbotapi.CallbackQuery{
		Data:    "clients:route:wgc1",
		Message: &tgbotapi.Message{MessageID: 42, Chat: &tgbotapi.Chat{ID: 100}},
	}
	h.HandleCallback(cb)

	if config.savedConfig == nil {
		t.Fatal("expected config to be saved")
	}
	wgc1 := config.savedConfig.TunnelDirector.Tunnels["wgc1"]
	// Should be normalized (no /32 suffix)
	if len(wgc1.Clients) != 1 || wgc1.Clients[0] != "192.168.50.30" {
		t.Errorf("expected wgc1.clients=[192.168.50.30], got %v", wgc1.Clients)
	}
}

func TestClientsHandler_HandleTextInput_NotInAddState(t *testing.T) {
	sender := &mockSenderClients{}
	deps := &Deps{Sender: sender}
	h := NewClientsHandler(deps)

	msg := &tgbotapi.Message{
		Text: "192.168.50.10",
		Chat: &tgbotapi.Chat{ID: 100},
	}
	h.HandleTextInput(msg)

	if sender.lastChatID != 0 {
		t.Error("expected no message sent when not in add state")
	}
}
