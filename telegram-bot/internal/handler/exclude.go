package handler

import (
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/telegram"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/wizard"
)

// ExcludeHandler handles the /exclude command for managing excluded IPs
type ExcludeHandler struct {
	deps    *Deps
	manager *wizard.Manager
	sender  telegram.MessageSender
}

// NewExcludeHandler creates a new ExcludeHandler
func NewExcludeHandler(deps *Deps) *ExcludeHandler {
	return &ExcludeHandler{
		deps:    deps,
		manager: wizard.NewManager(),
		sender:  deps.Sender,
	}
}

// HandleExclude handles the /exclude command
func (h *ExcludeHandler) HandleExclude(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID

	cfg, err := h.deps.Config.LoadVPNConfig()
	if err != nil {
		h.sender.Send(chatID, telegram.EscapeMarkdownV2("Config load error: "+err.Error()))
		return
	}

	state := h.manager.Start(chatID)
	state.SetStep(wizard.StepExcludeIPs)
	if len(cfg.Xray.ExcludeIPs) > 0 {
		state.SetExcludeIPs(cfg.Xray.ExcludeIPs)
	}

	h.renderUI(chatID, state)
}

// HandleCallback handles callback button presses for exclude
func (h *ExcludeHandler) HandleCallback(cb *tgbotapi.CallbackQuery) {
	if cb.Message == nil || cb.Message.Chat == nil {
		return
	}
	chatID := cb.Message.Chat.ID
	state := h.manager.Get(chatID)
	if state == nil {
		return
	}

	data := cb.Data
	if !strings.HasPrefix(data, "exclip:") {
		return
	}

	action := strings.TrimPrefix(data, "exclip:")

	switch {
	case action == "cancel":
		h.manager.Clear(chatID)
		h.sender.SendPlain(chatID, "Cancelled")
		return

	case action == "done":
		h.saveAndApply(chatID, state)
		return

	case action == "skip":
		h.manager.Clear(chatID)
		h.sender.SendPlain(chatID, "No changes")
		return

	case action == "add":
		h.sender.SendPlain(chatID, "Enter IP address or CIDR (e.g., 1.2.3.4 or 10.0.0.0/8):")
		return

	case strings.HasPrefix(action, "rm:"):
		var idx int
		if _, err := fmt.Sscanf(strings.TrimPrefix(action, "rm:"), "%d", &idx); err == nil {
			state.RemoveExcludeIP(idx)
		}
	}

	text, keyboard := h.buildUI(state)
	h.sender.EditMessage(chatID, cb.Message.MessageID, text, keyboard)
}

// HandleTextInput handles text input for exclude
func (h *ExcludeHandler) HandleTextInput(msg *tgbotapi.Message) {
	state := h.manager.Get(msg.Chat.ID)
	if state == nil {
		return
	}

	input := strings.TrimSpace(msg.Text)
	if input == "" {
		return
	}

	if !wizard.IsValidIPOrCIDR(input) {
		h.sender.SendPlain(msg.Chat.ID,
			"Invalid format. Enter IPv4 (1.2.3.4) or CIDR (10.0.0.0/8):")
		return
	}

	// Check for duplicate
	for _, existing := range state.GetExcludeIPs() {
		if existing == input {
			h.sender.SendPlain(msg.Chat.ID, "This IP/CIDR is already in the list")
			return
		}
	}

	state.AddExcludeIP(input)
	h.renderUI(msg.Chat.ID, state)
}

func (h *ExcludeHandler) renderUI(chatID int64, state *wizard.State) {
	text, keyboard := h.buildUI(state)
	h.sender.SendWithKeyboard(chatID, text, keyboard)
}

func (h *ExcludeHandler) buildUI(state *wizard.State) (string, tgbotapi.InlineKeyboardMarkup) {
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
	kb.Button("Cancel", "exclip:cancel").Row()

	var sb strings.Builder
	sb.WriteString(telegram.EscapeMarkdownV2("Manage excluded IPs"))
	sb.WriteString("\n")
	if len(ips) > 0 {
		sb.WriteString(telegram.EscapeMarkdownV2(
			fmt.Sprintf("Current: %s", strings.Join(ips, ", "))))
	} else {
		sb.WriteString(telegram.EscapeMarkdownV2("No IPs configured"))
	}

	return sb.String(), kb.Build()
}

func (h *ExcludeHandler) saveAndApply(chatID int64, state *wizard.State) {
	defer h.manager.Clear(chatID)

	cfg, err := h.deps.Config.LoadVPNConfig()
	if err != nil {
		h.sender.SendPlain(chatID, fmt.Sprintf("Config load error: %v", err))
		return
	}

	cfg.Xray.ExcludeIPs = state.GetExcludeIPs()

	if err := h.deps.Config.SaveVPNConfig(cfg); err != nil {
		h.sender.SendPlain(chatID, fmt.Sprintf("Save error: %v", err))
		return
	}
	h.sender.SendPlain(chatID, "vpn-director.json updated")

	if err := h.deps.VPN.Apply(); err != nil {
		h.sender.SendPlain(chatID, fmt.Sprintf("Apply error: %v", err))
		return
	}
	h.sender.SendPlain(chatID, "Done!")
}
