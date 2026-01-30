// Package updatechecker provides background update checking functionality.
package updatechecker

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/chatstore"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/telegram"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/updater"
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
