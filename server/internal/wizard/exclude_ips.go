package wizard

import (
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/telegram"
	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/vpnconfig"
)

// ExcludeIPsStep handles the exclude IPs wizard step
type ExcludeIPsStep struct {
	deps *StepDeps
	next func(chatID int64, state *State)
}

// NewExcludeIPsStep creates a new ExcludeIPsStep handler
func NewExcludeIPsStep(deps *StepDeps, next func(chatID int64, state *State)) *ExcludeIPsStep {
	return &ExcludeIPsStep{deps: deps, next: next}
}

// Render displays the exclude IPs UI
func (s *ExcludeIPsStep) Render(chatID int64, state *State) {
	text, keyboard := s.buildUI(state)
	s.deps.Sender.SendWithKeyboard(chatID, text, keyboard)
}

// HandleCallback processes callback button presses for exclude IPs
func (s *ExcludeIPsStep) HandleCallback(cb *tgbotapi.CallbackQuery, state *State) {
	data := cb.Data
	if !strings.HasPrefix(data, "wexclip:") {
		return
	}

	action := strings.TrimPrefix(data, "wexclip:")

	switch {
	case action == "done" || action == "skip":
		state.SetStep(StepClients)
		if s.next != nil {
			s.next(cb.Message.Chat.ID, state)
		}
		return

	case action == "add":
		state.SetStep(StepExcludeIPs)
		s.deps.Sender.SendPlain(cb.Message.Chat.ID,
			"Enter IP address or CIDR (e.g., 1.2.3.4 or 10.0.0.0/8):")
		return

	case strings.HasPrefix(action, "rm:"):
		var idx int
		if _, err := fmt.Sscanf(strings.TrimPrefix(action, "rm:"), "%d", &idx); err == nil {
			state.RemoveExcludeIP(idx)
		}
	}

	// Refresh UI
	text, keyboard := s.buildUI(state)
	s.deps.Sender.EditMessage(cb.Message.Chat.ID, cb.Message.MessageID, text, keyboard)
}

// HandleMessage processes text input for exclude IPs
func (s *ExcludeIPsStep) HandleMessage(msg *tgbotapi.Message, state *State) bool {
	input := strings.TrimSpace(msg.Text)
	if input == "" {
		return false
	}

	if !IsValidIPOrCIDR(input) {
		s.deps.Sender.SendPlain(msg.Chat.ID,
			"Invalid format. Enter IPv4 address (1.2.3.4) or CIDR (10.0.0.0/8):")
		return true
	}

	// Check for duplicate
	for _, existing := range state.GetExcludeIPs() {
		if existing == input {
			s.deps.Sender.SendPlain(msg.Chat.ID, "This IP/CIDR is already in the list")
			return true
		}
	}

	state.AddExcludeIP(input)
	s.Render(msg.Chat.ID, state)
	return true
}

func (s *ExcludeIPsStep) buildUI(state *State) (string, tgbotapi.InlineKeyboardMarkup) {
	ips := state.GetExcludeIPs()
	kb := telegram.NewKeyboard()

	for i, ip := range ips {
		kb.Button(fmt.Sprintf("Remove %s", ip), fmt.Sprintf("wexclip:rm:%d", i)).Row()
	}

	kb.Button("Add", "wexclip:add")
	if len(ips) > 0 {
		kb.Button("Done", "wexclip:done")
	} else {
		kb.Button("Skip", "wexclip:skip")
	}
	kb.Row()
	kb.Button("Cancel", "cancel").Row()

	var sb strings.Builder
	sb.WriteString(telegram.EscapeMarkdownV2("Exclude IPs from proxy"))
	sb.WriteString("\n")
	if len(ips) > 0 {
		sb.WriteString(telegram.EscapeMarkdownV2(
			fmt.Sprintf("Current: %s", strings.Join(ips, ", "))))
	} else {
		sb.WriteString(telegram.EscapeMarkdownV2("No extra IPs configured"))
	}

	return sb.String(), kb.Build()
}

// IsValidIPOrCIDR reports whether s is an IPv4 address or IPv4 CIDR.
// Validation lives in vpnconfig.NormalizeClientAddr so the bot and the
// Web UI agree on what they accept.
func IsValidIPOrCIDR(s string) bool {
	_, err := vpnconfig.NormalizeClientAddr(s)
	return err == nil
}
