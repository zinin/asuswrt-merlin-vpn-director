package wizard

import (
	"fmt"
	"net"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/telegram"
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
	if !strings.HasPrefix(data, "exclip:") {
		return
	}

	action := strings.TrimPrefix(data, "exclip:")

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

	state.AddExcludeIP(input)
	s.Render(msg.Chat.ID, state)
	return true
}

func (s *ExcludeIPsStep) buildUI(state *State) (string, tgbotapi.InlineKeyboardMarkup) {
	ips := state.GetExcludeIPs()
	kb := telegram.NewKeyboard()

	for i, ip := range ips {
		kb.Button(fmt.Sprintf("Remove %s", ip), fmt.Sprintf("exclip:rm:%d", i)).Row()
	}

	kb.Button("Add", "exclip:add")
	if len(ips) > 0 {
		kb.Button("Done", "exclip:done")
	} else {
		kb.Button("Skip", "exclip:skip")
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

// IsValidIPOrCIDR validates input as IPv4 or IPv4 CIDR
func IsValidIPOrCIDR(s string) bool {
	if strings.Contains(s, "/") {
		_, _, err := net.ParseCIDR(s)
		return err == nil
	}
	ip := net.ParseIP(s)
	return ip != nil && ip.To4() != nil
}
