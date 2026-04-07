// internal/handler/xray_test.go
package handler

import (
	"errors"
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/vpnconfig"
)

// mockXrayGenerator for testing
type mockXrayGenerator struct {
	lastServer vpnconfig.Server
	err        error
}

func (m *mockXrayGenerator) GenerateConfig(server vpnconfig.Server) error {
	m.lastServer = server
	return m.err
}

func TestXrayHandler_HandleXray_WithServers(t *testing.T) {
	sender := &mockSenderWithKeyboard{}
	servers := []vpnconfig.Server{
		{Name: "Germany, Berlin", Address: "de.example.com", IPs: []string{"1.1.1.1"}},
		{Name: "USA, New York", Address: "us.example.com", IPs: []string{"2.2.2.2"}},
	}
	config := &mockConfigStore{servers: servers}

	deps := &Deps{Sender: sender, Config: config}
	h := NewXrayHandler(deps)

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}}
	h.HandleXray(msg)

	if sender.lastChatID != 123 {
		t.Errorf("expected chatID 123, got %d", sender.lastChatID)
	}
	if !strings.Contains(sender.lastText, "сервер") {
		t.Errorf("expected 'сервер' in text, got %q", sender.lastText)
	}
	// Should have 1 row with 2 buttons (2 servers, 2 columns)
	if len(sender.lastKeyboard.InlineKeyboard) < 1 {
		t.Error("expected keyboard to have rows")
	}
	// First row should have server buttons
	if len(sender.lastKeyboard.InlineKeyboard[0]) == 0 {
		t.Error("expected buttons in first row")
	}
}

func TestXrayHandler_HandleXray_Empty(t *testing.T) {
	sender := &mockSenderWithKeyboard{}
	config := &mockConfigStore{servers: []vpnconfig.Server{}}

	deps := &Deps{Sender: sender, Config: config}
	h := NewXrayHandler(deps)

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 456}}
	h.HandleXray(msg)

	if !strings.Contains(sender.lastText, "не найден") || !strings.Contains(sender.lastText, "/import") {
		t.Errorf("expected 'не найдены' and '/import' in text, got %q", sender.lastText)
	}
}

func TestXrayHandler_HandleXray_LoadError(t *testing.T) {
	sender := &mockSenderWithKeyboard{}
	config := &mockConfigStore{err: errors.New("config load failed")}

	deps := &Deps{Sender: sender, Config: config}
	h := NewXrayHandler(deps)

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 789}}
	h.HandleXray(msg)

	if !strings.Contains(sender.lastText, "config load failed") {
		t.Errorf("expected error message, got %q", sender.lastText)
	}
}

func TestXrayHandler_HandleCallback_Success(t *testing.T) {
	sender := &mockSenderWithKeyboard{}
	servers := []vpnconfig.Server{
		{Name: "Germany, Berlin", Address: "de.example.com", Port: 443, UUID: "uuid1", IPs: []string{"1.1.1.1"}},
		{Name: "USA, New York", Address: "us.example.com", Port: 443, UUID: "uuid2", IPs: []string{"2.2.2.2"}},
	}
	config := &mockConfigStore{servers: servers}
	xray := &mockXrayGenerator{}
	vpn := &mockVPNDirectorWithXray{}

	deps := &Deps{Sender: sender, Config: config, Xray: xray, VPN: vpn}
	h := NewXrayHandler(deps)

	cb := &tgbotapi.CallbackQuery{
		ID:   "cb123",
		Data: "xray:select:1",
		Message: &tgbotapi.Message{
			MessageID: 42,
			Chat:      &tgbotapi.Chat{ID: 100},
		},
	}
	h.HandleCallback(cb)

	// Should generate config for server index 1 (USA)
	if xray.lastServer.Name != "USA, New York" {
		t.Errorf("expected server 'USA, New York', got %q", xray.lastServer.Name)
	}
	// Should edit the original message (not send new one)
	if sender.lastMsgID != 42 {
		t.Errorf("expected message ID 42 to be edited, got %d", sender.lastMsgID)
	}
	// Should show success message
	if !strings.Contains(sender.lastText, "Переключено") || !strings.Contains(sender.lastText, "USA") {
		t.Errorf("expected success message with server name, got %q", sender.lastText)
	}
}

func TestXrayHandler_HandleCallback_InvalidIndex(t *testing.T) {
	sender := &mockSenderWithKeyboard{}
	servers := []vpnconfig.Server{
		{Name: "Germany, Berlin"},
	}
	config := &mockConfigStore{servers: servers}

	deps := &Deps{Sender: sender, Config: config}
	h := NewXrayHandler(deps)

	cb := &tgbotapi.CallbackQuery{
		ID:   "cb456",
		Data: "xray:select:99", // invalid index
		Message: &tgbotapi.Message{
			MessageID: 10,
			Chat:      &tgbotapi.Chat{ID: 200},
		},
	}
	h.HandleCallback(cb)

	if !strings.Contains(sender.lastText, "Ошибка") {
		t.Errorf("expected error message, got %q", sender.lastText)
	}
}

func TestXrayHandler_HandleCallback_GenerateError(t *testing.T) {
	sender := &mockSenderWithKeyboard{}
	servers := []vpnconfig.Server{{Name: "Server1"}}
	config := &mockConfigStore{servers: servers}
	xray := &mockXrayGenerator{err: errors.New("template not found")}

	deps := &Deps{Sender: sender, Config: config, Xray: xray}
	h := NewXrayHandler(deps)

	cb := &tgbotapi.CallbackQuery{
		ID:   "cb789",
		Data: "xray:select:0",
		Message: &tgbotapi.Message{
			MessageID: 5,
			Chat:      &tgbotapi.Chat{ID: 300},
		},
	}
	h.HandleCallback(cb)

	if !strings.Contains(sender.lastText, "template not found") {
		t.Errorf("expected error message, got %q", sender.lastText)
	}
}

func TestXrayHandler_HandleCallback_RestartError(t *testing.T) {
	sender := &mockSenderWithKeyboard{}
	servers := []vpnconfig.Server{{Name: "Server1"}}
	config := &mockConfigStore{servers: servers}
	xray := &mockXrayGenerator{}
	vpn := &mockVPNDirectorWithXray{restartXrayErr: errors.New("xray not running")}

	deps := &Deps{Sender: sender, Config: config, Xray: xray, VPN: vpn}
	h := NewXrayHandler(deps)

	cb := &tgbotapi.CallbackQuery{
		ID:   "cb_restart",
		Data: "xray:select:0",
		Message: &tgbotapi.Message{
			MessageID: 7,
			Chat:      &tgbotapi.Chat{ID: 400},
		},
	}
	h.HandleCallback(cb)

	if !strings.Contains(sender.lastText, "xray not running") {
		t.Errorf("expected restart error message, got %q", sender.lastText)
	}
}

func TestXrayHandler_HandleCallback_NilMessage(t *testing.T) {
	sender := &mockSenderWithKeyboard{}
	config := &mockConfigStore{}

	deps := &Deps{Sender: sender, Config: config}
	h := NewXrayHandler(deps)

	// Inline callbacks may have nil Message
	cb := &tgbotapi.CallbackQuery{
		ID:      "cb_nil",
		Data:    "xray:select:0",
		Message: nil,
	}

	// Should not panic
	h.HandleCallback(cb)

	// No message should be sent (because we returned early)
	if sender.lastChatID != 0 {
		t.Error("expected no message to be sent for nil Message")
	}
}

func TestXrayHandler_HandleCallback_InvalidData(t *testing.T) {
	sender := &mockSenderWithKeyboard{}
	config := &mockConfigStore{}

	deps := &Deps{Sender: sender, Config: config}
	h := NewXrayHandler(deps)

	cb := &tgbotapi.CallbackQuery{
		ID:   "cb_invalid",
		Data: "xray:select:abc", // non-numeric index
		Message: &tgbotapi.Message{
			MessageID: 1,
			Chat:      &tgbotapi.Chat{ID: 500},
		},
	}
	h.HandleCallback(cb)

	if !strings.Contains(sender.lastText, "неверный индекс") {
		t.Errorf("expected 'неверный индекс' error, got %q", sender.lastText)
	}
}

func TestXrayHandler_FullFlow(t *testing.T) {
	sender := &mockSenderWithKeyboard{}
	servers := []vpnconfig.Server{
		{Name: "Germany, Berlin", Address: "de.example.com", Port: 443, UUID: "uuid1", IPs: []string{"1.1.1.1"}},
		{Name: "USA, New York", Address: "us.example.com", Port: 443, UUID: "uuid2", IPs: []string{"2.2.2.2"}},
		{Name: "Japan, Tokyo", Address: "jp.example.com", Port: 443, UUID: "uuid3", IPs: []string{"3.3.3.3"}},
	}
	config := &mockConfigStore{servers: servers}
	xray := &mockXrayGenerator{}
	vpn := &mockVPNDirectorWithXray{}

	deps := &Deps{Sender: sender, Config: config, Xray: xray, VPN: vpn}
	h := NewXrayHandler(deps)

	// Step 1: User sends /xray
	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 100}}
	h.HandleXray(msg)

	// Should show keyboard with 3 servers in 2 columns (2 rows)
	if len(sender.lastKeyboard.InlineKeyboard) != 2 { // 2 rows (2+1)
		t.Errorf("expected 2 keyboard rows, got %d", len(sender.lastKeyboard.InlineKeyboard))
	}
	// First row should have 2 buttons
	if len(sender.lastKeyboard.InlineKeyboard[0]) != 2 {
		t.Errorf("expected 2 buttons in first row, got %d", len(sender.lastKeyboard.InlineKeyboard[0]))
	}
	// Verify callback data format
	btn := sender.lastKeyboard.InlineKeyboard[0][0]
	if btn.CallbackData == nil || *btn.CallbackData != "xray:select:0" {
		t.Errorf("expected callback data 'xray:select:0', got %v", btn.CallbackData)
	}

	// Step 2: User clicks on USA server (index 1)
	cb := &tgbotapi.CallbackQuery{
		ID:   "cb",
		Data: "xray:select:1",
		Message: &tgbotapi.Message{
			MessageID: 42,
			Chat:      &tgbotapi.Chat{ID: 100},
		},
	}
	h.HandleCallback(cb)

	// Verify correct server was used
	if xray.lastServer.Address != "us.example.com" {
		t.Errorf("expected address 'us.example.com', got %q", xray.lastServer.Address)
	}
	// Verify original message was edited (keyboard removed)
	if sender.lastMsgID != 42 {
		t.Errorf("expected message ID 42 to be edited, got %d", sender.lastMsgID)
	}
	// Verify success message
	if !strings.Contains(sender.lastText, "USA, New York") {
		t.Errorf("expected success message with 'USA, New York', got %q", sender.lastText)
	}
}

// mockVPNDirectorWithXray extends mockVPNDirector with restartXrayErr support
type mockVPNDirectorWithXray struct {
	statusOutput   string
	statusErr      error
	restartErr     error
	restartXrayErr error
	stopErr        error
}

func (m *mockVPNDirectorWithXray) Status() (string, error) { return m.statusOutput, m.statusErr }
func (m *mockVPNDirectorWithXray) Apply() error            { return nil }
func (m *mockVPNDirectorWithXray) Restart() error          { return m.restartErr }
func (m *mockVPNDirectorWithXray) RestartXray() error      { return m.restartXrayErr }
func (m *mockVPNDirectorWithXray) Stop() error             { return m.stopErr }
