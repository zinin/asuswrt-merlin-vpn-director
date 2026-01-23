// internal/handler/status.go
package handler

import (
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/telegram"
)

// StatusHandler handles status-related commands
type StatusHandler struct {
	deps *Deps
}

// NewStatusHandler creates a new StatusHandler
func NewStatusHandler(deps *Deps) *StatusHandler {
	return &StatusHandler{deps: deps}
}

// HandleStatus handles /status command
func (h *StatusHandler) HandleStatus(msg *tgbotapi.Message) {
	output, err := h.deps.VPN.Status()
	if err != nil {
		h.deps.Sender.Send(msg.Chat.ID, telegram.EscapeMarkdownV2(fmt.Sprintf("Error: %v", err)))
		return
	}
	h.deps.Sender.SendCodeBlock(msg.Chat.ID, "📊 *VPN Director Status*:", output)
}

// HandleRestart handles /restart command
func (h *StatusHandler) HandleRestart(msg *tgbotapi.Message) {
	h.deps.Sender.Send(msg.Chat.ID, "Restarting VPN Director\\.\\.\\.")
	if err := h.deps.VPN.Restart(); err != nil {
		h.deps.Sender.Send(msg.Chat.ID, telegram.EscapeMarkdownV2(fmt.Sprintf("Error: %v", err)))
		return
	}
	h.deps.Sender.Send(msg.Chat.ID, "✅ VPN Director restarted")
}

// HandleStop handles /stop command
func (h *StatusHandler) HandleStop(msg *tgbotapi.Message) {
	if err := h.deps.VPN.Stop(); err != nil {
		h.deps.Sender.Send(msg.Chat.ID, telegram.EscapeMarkdownV2(fmt.Sprintf("Error: %v", err)))
		return
	}
	h.deps.Sender.Send(msg.Chat.ID, "⏹ VPN Director stopped")
}
