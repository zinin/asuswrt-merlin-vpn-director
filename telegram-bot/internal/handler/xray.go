// internal/handler/xray.go
package handler

import (
	"fmt"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/telegram"
)

// XrayHandler handles /xray command for quick server switching
type XrayHandler struct {
	deps *Deps
}

// NewXrayHandler creates a new XrayHandler
func NewXrayHandler(deps *Deps) *XrayHandler {
	return &XrayHandler{deps: deps}
}

// HandleXray handles /xray command - shows server selection keyboard
func (h *XrayHandler) HandleXray(msg *tgbotapi.Message) {
	servers, err := h.deps.Config.LoadServers()
	if err != nil {
		h.deps.Sender.Send(msg.Chat.ID, telegram.EscapeMarkdownV2(fmt.Sprintf("Ошибка: %v", err)))
		return
	}

	if len(servers) == 0 {
		h.deps.Sender.Send(msg.Chat.ID, telegram.EscapeMarkdownV2("Серверы не найдены. Используйте /import для импорта"))
		return
	}

	kb := telegram.NewKeyboard()
	for i, srv := range servers {
		btnText := fmt.Sprintf("%d. %s", i+1, srv.Name)
		kb.Button(btnText, fmt.Sprintf("xray:select:%d", i))
	}
	kb.Columns(2)

	text := telegram.EscapeMarkdownV2("Выберите сервер:")
	h.deps.Sender.SendWithKeyboard(msg.Chat.ID, text, kb.Build())
}

// HandleCallback handles xray:select:{index} callbacks
func (h *XrayHandler) HandleCallback(cb *tgbotapi.CallbackQuery) {
	if cb.Message == nil {
		return
	}

	chatID := cb.Message.Chat.ID
	data := cb.Data

	// Parse server index from "xray:select:N"
	if !strings.HasPrefix(data, "xray:select:") {
		return
	}

	idxStr := strings.TrimPrefix(data, "xray:select:")
	idx, err := strconv.Atoi(idxStr)
	if err != nil {
		h.deps.Sender.Send(chatID, telegram.EscapeMarkdownV2("Ошибка: неверный индекс"))
		return
	}

	// Load servers
	servers, err := h.deps.Config.LoadServers()
	if err != nil {
		h.deps.Sender.Send(chatID, telegram.EscapeMarkdownV2(fmt.Sprintf("Ошибка: %v", err)))
		return
	}

	if idx < 0 || idx >= len(servers) {
		h.deps.Sender.Send(chatID, telegram.EscapeMarkdownV2("Ошибка: сервер не найден"))
		return
	}

	server := servers[idx]

	// Generate Xray config
	if err := h.deps.Xray.GenerateConfig(server); err != nil {
		h.deps.Sender.Send(chatID, telegram.EscapeMarkdownV2(fmt.Sprintf("Ошибка: %v", err)))
		return
	}

	// Restart Xray
	if err := h.deps.VPN.RestartXray(); err != nil {
		h.deps.Sender.Send(chatID, telegram.EscapeMarkdownV2(fmt.Sprintf("Ошибка перезапуска: %v", err)))
		return
	}

	// Send success message
	h.deps.Sender.Send(chatID, telegram.EscapeMarkdownV2(fmt.Sprintf("✓ Переключено на %s", server.Name)))
}
