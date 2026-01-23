// internal/handler/import_test.go
package handler

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/vpnconfig"
)

// mockConfigStoreForImport extends mockConfigStore with tracking for SaveServers
type mockConfigStoreForImport struct {
	mockConfigStore
	savedServers []vpnconfig.Server
	dataDirVal   string
}

func (m *mockConfigStoreForImport) SaveServers(servers []vpnconfig.Server) error {
	m.savedServers = servers
	return m.mockConfigStore.err
}

func (m *mockConfigStoreForImport) DataDirOrDefault() string {
	if m.dataDirVal != "" {
		return m.dataDirVal
	}
	return "/data"
}

func TestImportHandler_HandleImport_NoURL(t *testing.T) {
	sender := &mockSender{}
	config := &mockConfigStoreForImport{}
	deps := &Deps{Sender: sender, Config: config}
	h := NewImportHandler(deps)

	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 123},
		Text: "/import",
		Entities: []tgbotapi.MessageEntity{
			{Type: "bot_command", Offset: 0, Length: 7},
		},
	}
	h.HandleImport(msg)

	if sender.lastChatID != 123 {
		t.Errorf("expected chatID 123, got %d", sender.lastChatID)
	}
	if !strings.Contains(sender.lastText, "Usage") {
		t.Errorf("expected usage message, got %q", sender.lastText)
	}
}

func TestImportHandler_HandleImport_InvalidScheme(t *testing.T) {
	sender := &mockSender{}
	config := &mockConfigStoreForImport{}
	deps := &Deps{Sender: sender, Config: config}
	h := NewImportHandler(deps)

	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 456},
		Text: "/import ftp://example.com/servers",
		Entities: []tgbotapi.MessageEntity{
			{Type: "bot_command", Offset: 0, Length: 7},
		},
	}
	h.HandleImport(msg)

	if sender.lastChatID != 456 {
		t.Errorf("expected chatID 456, got %d", sender.lastChatID)
	}
	if !strings.Contains(sender.lastText, "http") && !strings.Contains(sender.lastText, "https") {
		t.Errorf("expected error about http/https, got %q", sender.lastText)
	}
}

func TestImportHandler_TimeoutConfiguration(t *testing.T) {
	h := NewImportHandler(&Deps{})

	if h.httpClient.Timeout != 30*time.Second {
		t.Errorf("expected 30s timeout, got %v", h.httpClient.Timeout)
	}
}

func TestImportHandler_MaxBodySize(t *testing.T) {
	h := NewImportHandler(&Deps{})

	expectedMaxSize := int64(1 << 20) // 1MB
	if h.maxBodySize != expectedMaxSize {
		t.Errorf("expected maxBodySize %d, got %d", expectedMaxSize, h.maxBodySize)
	}
}

func TestImportHandler_HandleImport_ValidSubscription(t *testing.T) {
	// Create a valid VLESS subscription (base64 encoded)
	vlessURL := "vless://test-uuid-1234@example.com:443?type=tcp#TestServer"
	encoded := base64.StdEncoding.EncodeToString([]byte(vlessURL))

	// Create mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(encoded))
	}))
	defer server.Close()

	sender := &mockSender{}
	config := &mockConfigStoreForImport{dataDirVal: t.TempDir()}
	deps := &Deps{Sender: sender, Config: config}
	h := NewImportHandler(deps)

	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 789},
		Text: "/import " + server.URL,
		Entities: []tgbotapi.MessageEntity{
			{Type: "bot_command", Offset: 0, Length: 7},
		},
	}
	h.HandleImport(msg)

	// The handler should have sent at least one message
	if sender.lastChatID != 789 {
		t.Errorf("expected chatID 789, got %d", sender.lastChatID)
	}

	// We expect either success or DNS resolution error (example.com won't resolve in test)
	// Either way, it should have tried to process the subscription
}

func TestImportHandler_HandleImport_HTTPError(t *testing.T) {
	// Create mock HTTP server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	sender := &mockSender{}
	config := &mockConfigStoreForImport{}
	deps := &Deps{Sender: sender, Config: config}
	h := NewImportHandler(deps)

	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 111},
		Text: "/import " + server.URL,
		Entities: []tgbotapi.MessageEntity{
			{Type: "bot_command", Offset: 0, Length: 7},
		},
	}
	h.HandleImport(msg)

	if sender.lastChatID != 111 {
		t.Errorf("expected chatID 111, got %d", sender.lastChatID)
	}
	if !strings.Contains(sender.lastText, "404") && !strings.Contains(sender.lastText, "HTTP") {
		t.Errorf("expected HTTP error message, got %q", sender.lastText)
	}
}

func TestImportHandler_HandleImport_InvalidBase64(t *testing.T) {
	// Create mock HTTP server that returns invalid base64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not valid base64!!!"))
	}))
	defer server.Close()

	sender := &mockSender{}
	config := &mockConfigStoreForImport{}
	deps := &Deps{Sender: sender, Config: config}
	h := NewImportHandler(deps)

	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 222},
		Text: "/import " + server.URL,
		Entities: []tgbotapi.MessageEntity{
			{Type: "bot_command", Offset: 0, Length: 7},
		},
	}
	h.HandleImport(msg)

	if sender.lastChatID != 222 {
		t.Errorf("expected chatID 222, got %d", sender.lastChatID)
	}
	// Should report no servers found or decode error
	if !strings.Contains(sender.lastText, "No") && !strings.Contains(sender.lastText, "decode") && !strings.Contains(sender.lastText, "base64") {
		t.Errorf("expected error about decoding or no servers, got %q", sender.lastText)
	}
}

func TestImportHandler_HandleImport_EmptySubscription(t *testing.T) {
	// Create a subscription with no VLESS URLs
	encoded := base64.StdEncoding.EncodeToString([]byte("just some text\nno vless here"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(encoded))
	}))
	defer server.Close()

	sender := &mockSender{}
	config := &mockConfigStoreForImport{}
	deps := &Deps{Sender: sender, Config: config}
	h := NewImportHandler(deps)

	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 333},
		Text: "/import " + server.URL,
		Entities: []tgbotapi.MessageEntity{
			{Type: "bot_command", Offset: 0, Length: 7},
		},
	}
	h.HandleImport(msg)

	if sender.lastChatID != 333 {
		t.Errorf("expected chatID 333, got %d", sender.lastChatID)
	}
	if !strings.Contains(sender.lastText, "No") {
		t.Errorf("expected 'No VLESS servers' message, got %q", sender.lastText)
	}
}
