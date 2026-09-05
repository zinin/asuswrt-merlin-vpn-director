package wizard

import (
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestIsValidIPOrCIDR(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"1.2.3.4", true},
		{"10.0.0.0/8", true},
		{"192.168.1.0/24", true},
		{"not-an-ip", false},
		{"256.1.1.1", false},
		{"", false},
		{"::1", false},
		{"::1/128", false},
		{"2001:db8::/32", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := IsValidIPOrCIDR(tt.input); got != tt.valid {
				t.Errorf("IsValidIPOrCIDR(%q) = %v, want %v", tt.input, got, tt.valid)
			}
		})
	}
}

// excludeIPsMessage builds a text message addressed to the given chat.
func excludeIPsMessage(chatID int64, text string) *tgbotapi.Message {
	return &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: chatID}, Text: text}
}

func TestExcludeIPsStep_HandleMessage_StoresNormalizedForm(t *testing.T) {
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
			sender := &mockSender{}
			step := NewExcludeIPsStep(&StepDeps{Sender: sender}, nil)
			state := &State{ChatID: 123, Step: StepExcludeIPs, ExcludeIPs: []string{}}

			if !step.HandleMessage(excludeIPsMessage(123, tt.input), state) {
				t.Fatal("expected the step to consume the message")
			}

			got := state.GetExcludeIPs()
			if len(got) != 1 {
				t.Fatalf("expected 1 exclude IP, got %v", got)
			}
			if got[0] != tt.want {
				t.Errorf("stored %q, want %q", got[0], tt.want)
			}
		})
	}
}

func TestExcludeIPsStep_HandleMessage_RejectsDuplicateSpelling(t *testing.T) {
	sender := &mockSender{}
	step := NewExcludeIPsStep(&StepDeps{Sender: sender}, nil)
	// Seeded from a config that stores the legacy /32 spelling.
	state := &State{ChatID: 123, Step: StepExcludeIPs, ExcludeIPs: []string{"1.2.3.4/32"}}

	if !step.HandleMessage(excludeIPsMessage(123, "1.2.3.4"), state) {
		t.Fatal("expected the step to consume the message")
	}

	if got := state.GetExcludeIPs(); len(got) != 1 {
		t.Fatalf("expected the duplicate to be rejected, got %v", got)
	}
	if !strings.Contains(sender.lastText, "already in the list") {
		t.Errorf("expected a duplicate warning, got %q", sender.lastText)
	}
}

func TestExcludeIPsStep_HandleMessage_RejectsInvalidInput(t *testing.T) {
	sender := &mockSender{}
	step := NewExcludeIPsStep(&StepDeps{Sender: sender}, nil)
	state := &State{ChatID: 123, Step: StepExcludeIPs, ExcludeIPs: []string{}}

	if !step.HandleMessage(excludeIPsMessage(123, "2001:db8::1"), state) {
		t.Fatal("expected the step to consume the message")
	}

	if got := state.GetExcludeIPs(); len(got) != 0 {
		t.Fatalf("expected nothing to be stored, got %v", got)
	}
	if !strings.Contains(sender.lastText, "Invalid format") {
		t.Errorf("expected an invalid-format message, got %q", sender.lastText)
	}
}
