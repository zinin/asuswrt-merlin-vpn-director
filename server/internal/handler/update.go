package handler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/telegram"
	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/updateflow"
)

// UpdateFlow is the update orchestration behind /update. Declared here so the
// command's tests can drive every outcome without a fake GitHub.
type UpdateFlow interface {
	Start(ctx context.Context, initiator string, chatID int64, progress func(string)) (updateflow.StartResult, error)
}

// UpdateHandler handles the /update command. It is an adapter: every decision
// lives in updateflow, this type only turns outcomes into chat messages.
type UpdateHandler struct {
	sender  telegram.MessageSender
	flow    UpdateFlow
	version string
}

// NewUpdateHandler creates a new update handler.
func NewUpdateHandler(sender telegram.MessageSender, flow UpdateFlow, version string) *UpdateHandler {
	return &UpdateHandler{sender: sender, flow: flow, version: version}
}

// HandleUpdate processes the /update command: it starts an update of both
// daemons and reports progress into the chat it came from.
func (h *UpdateHandler) HandleUpdate(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID

	res, err := h.flow.Start(context.Background(), "bot", chatID, func(line string) {
		h.send(chatID, line)
	})

	current := res.From
	if current == "" {
		current = h.version
	}

	var ghErr *updateflow.GitHubError
	switch {
	case err == nil:
		h.send(chatID, fmt.Sprintf("Starting update %s → %s...", res.From, res.To))
	case errors.Is(err, updateflow.ErrDevMode):
		h.send(chatID, "Command /update is not available in dev mode")
	case errors.Is(err, updateflow.ErrDevVersion):
		h.send(chatID, "Cannot check updates for dev build")
	case errors.Is(err, updateflow.ErrInProgress):
		h.send(chatID, "Update is already in progress, please wait...")
	case errors.Is(err, updateflow.ErrUpToDate):
		h.send(chatID, fmt.Sprintf("Already running the latest version: %s", current))
	case errors.As(err, &ghErr):
		h.send(chatID, fmt.Sprintf("Failed to check for updates: %v", ghErr.Err))
	default:
		h.send(chatID, fmt.Sprintf("Failed to start update: %v", err))
	}
}

// HandleCallback handles update callbacks from inline buttons.
func (h *UpdateHandler) HandleCallback(cb *tgbotapi.CallbackQuery) {
	if cb.Data != "update:run" {
		return
	}
	// cb.Message can be nil for inline callbacks
	if cb.Message == nil {
		slog.Warn("Callback without message, cannot process update:run")
		return
	}
	// Create a message-like structure to reuse HandleUpdate logic
	msg := &tgbotapi.Message{
		Chat: cb.Message.Chat,
		From: cb.From,
	}
	h.HandleUpdate(msg)
}

// send sends a plain text message. Errors are logged but not returned.
func (h *UpdateHandler) send(chatID int64, text string) {
	_ = h.sender.SendPlain(chatID, text)
}
