// Package startup handles bot startup tasks like update notifications.
package startup

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/chatstore"
	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/telegram"
)

// Default paths for update notification files.
const (
	DefaultNotifyFile = "/tmp/vpn-director-update/notify.json"
	DefaultUpdateDir  = "/tmp/vpn-director-update"
)

// ChatStore is the part of chatstore.Store the notifier needs: who to tell
// about an update that was started somewhere without a chat of its own.
type ChatStore interface {
	GetActiveUsers() ([]chatstore.UserChat, error)
}

// UpdateNotification represents the JSON structure in notify.json.
type UpdateNotification struct {
	ChatID     int64  `json:"chat_id"` // 0 when the update came from the Web UI
	OldVersion string `json:"old_version"`
	NewVersion string `json:"new_version"`
	Status     string `json:"status"`    // "ok" or "failed"
	Initiator  string `json:"initiator"` // "bot" or "webui"
}

// CheckAndSendNotify checks for a pending update notification and sends it.
// A notification with chat_id 0 was written by a Web UI update and goes to
// every active chat; store may be nil (dev mode), in which case it is skipped
// and kept for the next start. A successful update clears the whole update
// directory; a failed one keeps update.log, because the message points at it.
func CheckAndSendNotify(sender telegram.MessageSender, store ChatStore, notifyFile, updateDir string) error {
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

	recipients, err := recipientsFor(n, store)
	if err != nil {
		return err
	}
	if recipients == nil {
		// Skipped: nothing to do now, the file waits for the next start.
		return nil
	}

	text := notificationText(n, updateDir)

	sent := 0
	for _, chatID := range recipients {
		if err := sender.SendPlain(chatID, text); err != nil {
			slog.Warn("Failed to send update notification",
				"chat_id", chatID,
				"old_version", n.OldVersion,
				"new_version", n.NewVersion,
				"error", err)
			continue
		}
		sent++
	}

	if sent == 0 && len(recipients) > 0 {
		return fmt.Errorf("send notification: no recipient could be reached")
	}

	cleanup(n, notifyFile, updateDir)
	return nil
}

// recipientsFor resolves the chats to notify. A nil slice means "skip for
// now"; an empty slice means "nobody to tell, but nothing is pending either".
func recipientsFor(n UpdateNotification, store ChatStore) ([]int64, error) {
	if n.ChatID != 0 {
		return []int64{n.ChatID}, nil
	}
	if store == nil {
		slog.Info("Update notification has no chat_id and no chat store, skipping",
			"initiator", n.Initiator)
		return nil, nil
	}
	users, err := store.GetActiveUsers()
	if err != nil {
		return nil, fmt.Errorf("get active users: %w", err)
	}
	// chatstore keys by lowercased username, so one person who renamed their
	// Telegram handle is two records with one ChatID. Telling them twice about
	// the same update is a bug the store cannot see.
	seen := make(map[int64]struct{}, len(users))
	chats := make([]int64, 0, len(users))
	for _, u := range users {
		if _, dup := seen[u.ChatID]; dup {
			continue
		}
		seen[u.ChatID] = struct{}{}
		chats = append(chats, u.ChatID)
	}
	if len(chats) == 0 {
		slog.Info("Update notification has no active chats to go to", "initiator", n.Initiator)
	}
	return chats, nil
}

// notificationText renders the message. A failed update points at the log the
// script left behind rather than repeating its exit code.
func notificationText(n UpdateNotification, updateDir string) string {
	if n.Status == "failed" {
		return "Update failed: see " + filepath.Join(updateDir, "update.log")
	}
	return fmt.Sprintf("Update complete: %s → %s", n.OldVersion, n.NewVersion)
}

// cleanup removes what the notification leaves behind. Errors are logged, not
// returned: the message is already delivered and cleanup is best-effort.
func cleanup(n UpdateNotification, notifyFile, updateDir string) {
	if n.Status == "failed" {
		if err := os.Remove(notifyFile); err != nil {
			slog.Warn("Failed to remove notify file", "path", notifyFile, "error", err)
		}
		return
	}
	if err := os.RemoveAll(updateDir); err != nil {
		slog.Warn("Failed to cleanup update directory", "dir", updateDir, "error", err)
	}
}
