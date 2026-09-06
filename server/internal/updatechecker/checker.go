// Package updatechecker provides background update checking functionality.
package updatechecker

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/chatstore"
	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/telegram"
	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/updater"
)

const maxChangelogLength = 500

// Sender is the interface for sending messages with inline keyboard.
type Sender interface {
	SendWithKeyboard(chatID int64, text string, keyboard tgbotapi.InlineKeyboardMarkup) error
}

// Authorizer checks if a user is authorized.
type Authorizer interface {
	IsAuthorized(username string) bool
}

// ChatStore is the interface for chat storage.
type ChatStore interface {
	GetActiveUsers() ([]chatstore.UserChat, error)
	IsNotified(username string, version string) bool
	MarkNotified(username string, version string) error
	SetInactive(username string) error
}

// Checker periodically checks for updates and notifies users.
type Checker struct {
	updater        updater.Updater
	store          ChatStore
	sender         Sender
	auth           Authorizer
	currentVersion string
}

// New creates a new Checker.
func New(
	upd updater.Updater,
	store ChatStore,
	sender Sender,
	auth Authorizer,
	currentVersion string,
) *Checker {
	return &Checker{
		updater:        upd,
		store:          store,
		sender:         sender,
		auth:           auth,
		currentVersion: currentVersion,
	}
}

// Run starts the checker loop. Blocks until ctx is cancelled.
func (c *Checker) Run(ctx context.Context, interval time.Duration) {
	slog.Info("Update checker started", "interval", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Initial check
	c.checkOnce(ctx)

	for {
		select {
		case <-ctx.Done():
			slog.Info("Update checker stopped")
			return
		case <-ticker.C:
			c.checkOnce(ctx)
		}
	}
}

// checkOnce performs a single update check.
func (c *Checker) checkOnce(ctx context.Context) {
	// Skip if current version is dev
	if c.currentVersion == "dev" {
		return
	}

	release, err := c.updater.GetLatestRelease(ctx)
	if err != nil {
		slog.Warn("Failed to check for updates", "error", err)
		return
	}

	shouldUpdate, err := c.updater.ShouldUpdate(c.currentVersion, release.TagName)
	if err != nil {
		slog.Warn("Failed to compare versions", "error", err)
		return
	}

	if !shouldUpdate {
		slog.Debug("No update available", "current", c.currentVersion, "latest", release.TagName)
		return
	}

	slog.Info("New version available", "current", c.currentVersion, "latest", release.TagName)

	c.notifyUsers(ctx, release)
}

// notifyUsers sends update notification to all active authorized users.
func (c *Checker) notifyUsers(ctx context.Context, release *updater.Release) {
	users, err := c.store.GetActiveUsers()
	if err != nil {
		slog.Warn("Failed to get active users", "error", err)
		return
	}

	// One ChatID can carry several usernames (a renamed handle), and the store
	// tracks "notified" per username. Send once per chat, but mark every
	// record, or the duplicate asks again on the next tick.
	notifiedChats := make(map[int64]struct{}, len(users))

	// A chat told on an earlier tick stays told, even though the record that
	// heard about it is not the one being renamed into existence now. Seed from
	// the store before sending anything: GetActiveUsers iterates a map, so the
	// already notified record is not necessarily visited first. Authorisation is
	// deliberately not consulted here - the question is whether this chat has
	// already been told about this version, and it has, whoever heard it.
	for _, user := range users {
		if c.store.IsNotified(user.Username, release.TagName) {
			notifiedChats[user.ChatID] = struct{}{}
		}
	}

	for _, user := range users {
		// Check for context cancellation (graceful shutdown)
		select {
		case <-ctx.Done():
			slog.Info("Update notification interrupted by shutdown")
			return
		default:
		}

		// Check if user is still authorized
		if !c.auth.IsAuthorized(user.Username) {
			continue
		}

		// Check if already notified
		if c.store.IsNotified(user.Username, release.TagName) {
			continue
		}

		// Same chat, different handle: mark this record and move on.
		if _, dup := notifiedChats[user.ChatID]; dup {
			_ = c.store.MarkNotified(user.Username, release.TagName)
			continue
		}

		// Send notification with keyboard (MarkdownV2 via SendWithKeyboard)
		msg, keyboard := c.formatNotification(release)
		err := c.sender.SendWithKeyboard(user.ChatID, msg, keyboard)

		if err != nil {
			if isBlockedError(err) {
				slog.Info("User blocked bot, marking inactive", "username", user.Username)
				_ = c.store.SetInactive(user.Username)
			} else {
				slog.Warn("Failed to send notification", "username", user.Username, "error", err)
			}
			continue
		}

		// Mark as notified
		_ = c.store.MarkNotified(user.Username, release.TagName)
		notifiedChats[user.ChatID] = struct{}{}
		slog.Info("Sent update notification", "username", user.Username, "version", release.TagName)
	}
}

// formatNotification creates the notification message and keyboard.
func (c *Checker) formatNotification(release *updater.Release) (string, tgbotapi.InlineKeyboardMarkup) {
	changelog := release.Body
	// Truncate by runes to preserve UTF-8 validity
	runes := []rune(changelog)
	if len(runes) > maxChangelogLength {
		changelog = string(runes[:maxChangelogLength]) + "..."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🆕 Доступна новая версия %s\n\n", telegram.EscapeMarkdownV2(release.TagName)))
	sb.WriteString(fmt.Sprintf("Текущая версия: %s\n\n", telegram.EscapeMarkdownV2(c.currentVersion)))

	if changelog != "" {
		sb.WriteString("📋 Что нового:\n")
		sb.WriteString(telegram.EscapeMarkdownV2(changelog))
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔄 Обновить", "update:run"),
		),
	)

	return sb.String(), keyboard
}

// isBlockedError checks if error indicates bot was blocked.
func isBlockedError(err error) bool {
	errStr := err.Error()
	return strings.Contains(errStr, "bot was blocked") ||
		strings.Contains(errStr, "chat not found") ||
		strings.Contains(errStr, "user is deactivated")
}
