// Package startup handles bot startup tasks like update notifications.
package startup

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/chatstore"
	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/telegram"
	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/updater"
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
// currentVersion is the version this process runs; "" means unknown.
func CheckAndSendNotify(sender telegram.MessageSender, store ChatStore, notifyFile, updateDir, currentVersion string) error {
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

	if overtakenFailure(n, currentVersion) {
		slog.Info("Dropping an update failure the running version outlived",
			"failed_from", n.OldVersion, "failed_to", n.NewVersion, "running", currentVersion)
		cleanup(n, notifyFile, updateDir)
		return nil
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

// overtakenFailure reports whether a failed update has already been overtaken.
// A failed update leaves notify.json behind, and the bot reads that file on
// its next start. When that start is a build from neither end of the failed
// attempt - install.sh was re-run, or a later update landed - the daemons the
// failure describes are gone, and announcing it says something is broken when
// nothing is.
//
// Running NewVersion is not that case. The script copies both binaries into
// place before it starts anything, so a daemon that fails to start in its step
// 6 leaves this process running the new build with the other one down - and
// step 6 rewrites notify.json before starting the bot precisely so that
// failure gets reported. Reading it as old would silence the one report the
// script went out of its way to arrange.
//
// A successful notification is left alone whatever the version: it names what
// that update did, and staying quiet would be worse than saying it late.
func overtakenFailure(n UpdateNotification, currentVersion string) bool {
	return n.Status == "failed" && currentVersion != "" && n.OldVersion != "" &&
		currentVersion != n.OldVersion && currentVersion != n.NewVersion
}

// cleanup removes what the notification leaves behind. Errors are logged, not
// returned: the message is already delivered and cleanup is best-effort.
func cleanup(n UpdateNotification, notifyFile, updateDir string) {
	if n.Status == "failed" {
		if err := os.Remove(notifyFile); err != nil {
			slog.Warn("Failed to remove notify file", "path", notifyFile, "error", err)
		}
		// The download the attempt left behind is worthless - a retry wipes
		// files/ before writing into it - and it is 16 MB of a router's tmpfs
		// until the next update or reboot. update.log stays: the message
		// points at it.
		//
		// Unless the directory is spoken for. updateflow.Start claims it
		// before it downloads a byte, so a payload beside a claim can be an
		// attempt's, and removing it would break that attempt. The claim can
		// also still be the failed script's own: that script starts the
		// daemons back up before it drops its lock, so this can run in
		// between. Skipping then costs only the reclaimed space - the next
		// update wipes files/ before writing anyway - which is the harmless
		// way to be wrong.
		//
		// Claiming the directory is what makes that so, where looking whether
		// it is claimed would not. A look leaves the window between itself and
		// the removal: an attempt starting inside that window passes its own
		// check, takes the lock and begins filling files/, and what it
		// downloaded goes with the payload being cleared here - the retry the
		// user just started fails over files that were there a moment ago.
		// CreateLockAt is the same atomic claim Start makes, so one of the two
		// is refused instead.
		lockFile := filepath.Join(updateDir, updater.LockFileName)
		if err := updater.CreateLockAt(lockFile); err != nil {
			if errors.Is(err, updater.ErrLockExists) {
				slog.Info("An update holds the lock, leaving its payload alone", "dir", updateDir)
			} else {
				slog.Warn("Could not claim the update directory, leaving its payload alone",
					"dir", updateDir, "error", err)
			}
			return
		}
		// Held for the removal and no longer. The claim names this process,
		// which is alive, so one left behind has every later update refused as
		// one already in progress until the bot restarts.
		defer updater.RemoveLockAt(lockFile)

		filesDir := filepath.Join(updateDir, updater.FilesDirName)
		if err := os.RemoveAll(filesDir); err != nil {
			slog.Warn("Failed to remove update payload", "path", filesDir, "error", err)
		}
		return
	}
	if err := os.RemoveAll(updateDir); err != nil {
		slog.Warn("Failed to cleanup update directory", "dir", updateDir, "error", err)
	}
}
