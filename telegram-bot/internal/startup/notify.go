// Package startup handles bot startup tasks like update notifications.
package startup

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/telegram"
)

// Default paths for update notification files.
const (
	DefaultNotifyFile = "/tmp/vpn-director-update/notify.json"
	DefaultUpdateDir  = "/tmp/vpn-director-update"
)

// UpdateNotification represents the JSON structure in notify.json.
type UpdateNotification struct {
	ChatID     int64  `json:"chat_id"`
	OldVersion string `json:"old_version"`
	NewVersion string `json:"new_version"`
}

// CheckAndSendNotify checks for pending update notification and sends it.
// Uses telegram.MessageSender interface, calling SendPlain for plain text.
// Cleans up the update directory only after a successful send.
// Returns nil if no notification pending or if send succeeds.
func CheckAndSendNotify(sender telegram.MessageSender, notifyFile, updateDir string) error {
	data, err := os.ReadFile(notifyFile)
	if os.IsNotExist(err) {
		// No notification pending - this is normal
		return nil
	}
	if err != nil {
		return fmt.Errorf("read notify file: %w", err)
	}

	var n UpdateNotification
	if err := json.Unmarshal(data, &n); err != nil {
		return fmt.Errorf("parse notify file: %w", err)
	}

	// Validate required fields
	if n.ChatID == 0 {
		return fmt.Errorf("invalid notify file: missing chat_id")
	}

	// Send plain text message (no Markdown to avoid escaping issues)
	text := fmt.Sprintf("Update complete: %s → %s", n.OldVersion, n.NewVersion)
	if err := sender.SendPlain(n.ChatID, text); err != nil {
		// Log for debugging - notify.json will be kept for retry on next startup
		slog.Warn("Failed to send update notification",
			"chat_id", n.ChatID,
			"old_version", n.OldVersion,
			"new_version", n.NewVersion,
			"error", err)
		return fmt.Errorf("send notification: %w", err)
	}

	// Cleanup entire update directory after successful send
	// Log errors for debugging, but don't fail - cleanup is best-effort
	if err := os.RemoveAll(updateDir); err != nil {
		slog.Warn("Failed to cleanup update directory", "dir", updateDir, "error", err)
	}

	return nil
}
