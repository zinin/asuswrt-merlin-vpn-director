package handler

import (
	"context"
	"fmt"
	"log/slog"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/telegram"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/updater"
)

// UpdateHandler handles the /update command.
type UpdateHandler struct {
	sender  telegram.MessageSender
	updater updater.Updater
	devMode bool
	version string
}

// NewUpdateHandler creates a new update handler.
func NewUpdateHandler(sender telegram.MessageSender, upd updater.Updater, devMode bool, version string) *UpdateHandler {
	return &UpdateHandler{
		sender:  sender,
		updater: upd,
		devMode: devMode,
		version: version,
	}
}

// HandleUpdate processes the /update command.
// Checks for updates and starts the update process if a newer version is available.
func (h *UpdateHandler) HandleUpdate(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID

	// 1. Dev mode check (--dev flag)
	if h.devMode {
		h.send(chatID, "Command /update is not available in dev mode")
		return
	}

	// 2. Dev version check (unparseable version like "dev")
	if h.version == "dev" {
		h.send(chatID, "Cannot check updates for dev build")
		return
	}

	// 3. Lock check - is another update already in progress?
	if h.updater.IsUpdateInProgress() {
		h.send(chatID, "Update is already in progress, please wait...")
		return
	}

	// 4. Get latest release from GitHub
	ctx := context.Background()
	release, err := h.updater.GetLatestRelease(ctx)
	if err != nil {
		h.send(chatID, fmt.Sprintf("Failed to check for updates: %v", err))
		return
	}

	// 5. Validate current version for shell safety
	if !updater.IsValidVersion(h.version) {
		h.send(chatID, fmt.Sprintf("Invalid current version: %s", h.version))
		return
	}

	// 6. Validate release tag for shell safety
	if !updater.IsValidVersion(release.TagName) {
		h.send(chatID, fmt.Sprintf("Invalid release version: %s", release.TagName))
		return
	}

	// 7. Compare versions
	shouldUpdate, err := h.updater.ShouldUpdate(h.version, release.TagName)
	if err != nil {
		h.send(chatID, fmt.Sprintf("Failed to parse version: %v", err))
		return
	}
	if !shouldUpdate {
		h.send(chatID, fmt.Sprintf("Already running the latest version: %s", h.version))
		return
	}

	// 8. Create lock to prevent concurrent updates
	if err := h.updater.CreateLock(); err != nil {
		h.send(chatID, fmt.Sprintf("Failed to start update: %v", err))
		return
	}

	// 9. Notify user that update is starting
	h.send(chatID, fmt.Sprintf("Starting update %s → %s...", h.version, release.TagName))

	// 10. Download and update in goroutine to keep bot responsive
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("Update goroutine panicked", "error", r)
				h.send(chatID, "Update failed unexpectedly. Check logs.")
				h.updater.CleanFiles()
				h.updater.RemoveLock()
			}
		}()
		h.downloadAndUpdate(chatID, release)
	}()
}

// downloadAndUpdate handles the download and update process in background.
func (h *UpdateHandler) downloadAndUpdate(chatID int64, release *updater.Release) {
	ctx := context.Background()

	// Download all files
	err := h.updater.DownloadRelease(ctx, release)
	if err != nil {
		h.updater.CleanFiles()
		h.updater.RemoveLock()
		h.send(chatID, fmt.Sprintf("Download failed: %v", err))
		return
	}

	// Notify user files are ready
	h.send(chatID, "Files downloaded, starting update...")

	// Run update script (detached, will restart the bot)
	if err := h.updater.RunUpdateScript(chatID, h.version, release.TagName); err != nil {
		h.updater.CleanFiles()
		h.updater.RemoveLock()
		h.send(chatID, fmt.Sprintf("Failed to run update script: %v", err))
		return
	}

	// Notify user that script is running
	// This message may or may not be delivered before the bot stops
	h.send(chatID, "Update script started, bot will restart in a few seconds...")
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
