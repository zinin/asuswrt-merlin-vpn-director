package handler

import (
	"errors"
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/vpnconfig"
)

// newExcludeHandlerWithState builds an ExcludeHandler whose /exclude session
// for chat 100 is already seeded from the given stored exclude IPs.
func newExcludeHandlerWithState(t *testing.T, sender *mockSenderClients, stored []string) *ExcludeHandler {
	t.Helper()
	cfg := &vpnconfig.VPNDirectorConfig{
		Xray: vpnconfig.XrayConfig{ExcludeIPs: stored},
	}
	deps := &Deps{Sender: sender, Config: &mockConfigClients{vpnConfig: cfg}, VPN: &mockVPNClients{}}
	h := NewExcludeHandler(deps)
	h.HandleExclude(&tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 100}})
	if h.manager.Get(100) == nil {
		t.Fatal("expected an active /exclude session")
	}
	return h
}

func excludeTextMessage(text string) *tgbotapi.Message {
	return &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 100}, Text: text}
}

func TestExcludeHandler_HandleTextInput_StoresNormalizedForm(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"strips /32", "1.2.3.4/32", "1.2.3.4"},
		{"masks host bits", "5.6.7.8/24", "5.6.7.0/24"},
		{"trims spaces", "  9.9.9.9  ", "9.9.9.9"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := &mockSenderClients{}
			h := newExcludeHandlerWithState(t, sender, nil)

			h.HandleTextInput(excludeTextMessage(tt.input))

			got := h.manager.Get(100).GetExcludeIPs()
			if len(got) != 1 {
				t.Fatalf("expected 1 exclude IP, got %v", got)
			}
			if got[0] != tt.want {
				t.Errorf("stored %q, want %q", got[0], tt.want)
			}
		})
	}
}

func TestExcludeHandler_HandleTextInput_RejectsDuplicateSpelling(t *testing.T) {
	sender := &mockSenderClients{}
	// The session was seeded from a config holding the legacy /32 spelling.
	h := newExcludeHandlerWithState(t, sender, []string{"1.2.3.4/32"})

	h.HandleTextInput(excludeTextMessage("1.2.3.4"))

	if got := h.manager.Get(100).GetExcludeIPs(); len(got) != 1 {
		t.Fatalf("expected the duplicate to be rejected, got %v", got)
	}
	if !strings.Contains(strings.Join(sender.plainTexts, "\n"), "already in the list") {
		t.Errorf("expected a duplicate warning, got %v", sender.plainTexts)
	}
}

func TestExcludeHandler_HandleTextInput_RejectsInvalidInput(t *testing.T) {
	sender := &mockSenderClients{}
	h := newExcludeHandlerWithState(t, sender, nil)

	h.HandleTextInput(excludeTextMessage("2001:db8::1"))

	if got := h.manager.Get(100).GetExcludeIPs(); len(got) != 0 {
		t.Fatalf("expected nothing to be stored, got %v", got)
	}
	if !strings.Contains(strings.Join(sender.plainTexts, "\n"), "Invalid format") {
		t.Errorf("expected an invalid-format message, got %v", sender.plainTexts)
	}
}

func TestExcludeHandler_Done_SaveErrorIsReported(t *testing.T) {
	sender := &mockSenderClients{}
	config := &mockConfigClients{
		vpnConfig: &vpnconfig.VPNDirectorConfig{Xray: vpnconfig.XrayConfig{ExcludeIPs: []string{"1.2.3.4"}}},
		saveErr:   errors.New("disk full"),
	}
	deps := &Deps{Sender: sender, Config: config, VPN: &mockVPNClients{}}
	h := NewExcludeHandler(deps)

	h.HandleExclude(&tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 100}})
	h.HandleCallback(&tgbotapi.CallbackQuery{
		Data:    "exclip:done",
		Message: &tgbotapi.Message{MessageID: 7, Chat: &tgbotapi.Chat{ID: 100}},
	})

	if n := len(sender.plainTexts); n == 0 || !strings.Contains(sender.plainTexts[n-1], "Save error: disk full") {
		t.Errorf("expected the save error to reach the user, got %v", sender.plainTexts)
	}
}
