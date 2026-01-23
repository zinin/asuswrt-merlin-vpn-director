// internal/bot/bot.go
package bot

import (
	"context"
	"log/slog"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/config"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/handler"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/paths"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/service"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/telegram"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/wizard"
)

// Bot is the main Telegram bot struct with DI
type Bot struct {
	api    *tgbotapi.BotAPI
	auth   *Auth
	router *Router
	sender telegram.MessageSender
}

// New creates a new Bot with full dependency injection.
// If executor is nil, services use their default executors.
func New(cfg *config.Config, p paths.Paths, version string, executor service.ShellExecutor) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(cfg.BotToken)
	if err != nil {
		return nil, err
	}

	slog.Info("Authorized", "username", api.Self.UserName)

	// Create sender
	sender := telegram.NewSender(api)

	// Create services
	configSvc := service.NewConfigService(p.ScriptsDir, p.DefaultDataDir)
	vpnSvc := service.NewVPNDirectorService(p.ScriptsDir, executor)
	xraySvc := service.NewXrayService(p.XrayTemplate, p.XrayConfig)
	networkSvc := service.NewNetworkService(executor)
	logSvc := service.NewLogService(executor)

	// Create handler dependencies
	deps := &handler.Deps{
		Sender:  sender,
		Config:  configSvc,
		VPN:     vpnSvc,
		Xray:    xraySvc,
		Network: networkSvc,
		Logs:    logSvc,
		Paths:   p,
		Version: version,
	}

	// Create handlers
	statusHandler := handler.NewStatusHandler(deps)
	serversHandler := handler.NewServersHandler(deps)
	importHandler := handler.NewImportHandler(deps)
	miscHandler := handler.NewMiscHandler(deps)
	wizardHandler := wizard.NewHandler(sender, configSvc, vpnSvc, xraySvc)

	// Create router
	router := NewRouter(statusHandler, serversHandler, importHandler, miscHandler, wizardHandler)

	return &Bot{
		api:    api,
		auth:   NewAuth(cfg.AllowedUsers),
		router: router,
		sender: sender,
	}, nil
}

// RegisterCommands registers bot commands with Telegram
func (b *Bot) RegisterCommands() error {
	commands := []tgbotapi.BotCommand{
		{Command: "status", Description: "Xray status"},
		{Command: "servers", Description: "Server list"},
		{Command: "import", Description: "Import servers from URL"},
		{Command: "configure", Description: "Configuration wizard"},
		{Command: "restart", Description: "Restart VPN Director"},
		{Command: "stop", Description: "Stop VPN Director"},
		{Command: "logs", Description: "Recent logs"},
		{Command: "ip", Description: "External IP"},
		{Command: "version", Description: "Bot version"},
	}

	cfg := tgbotapi.NewSetMyCommands(commands...)
	_, err := b.api.Request(cfg)
	if err != nil {
		return err
	}

	slog.Info("Registered bot commands", "count", len(commands))
	return nil
}

// Run starts the bot and processes updates until context is cancelled
func (b *Bot) Run(ctx context.Context) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := b.api.GetUpdatesChan(u)

	slog.Info("Bot started, waiting for messages")

	for {
		select {
		case <-ctx.Done():
			slog.Info("Shutting down bot")
			b.api.StopReceivingUpdates()
			return
		case update, ok := <-updates:
			if !ok {
				slog.Warn("Updates channel closed, stopping bot")
				return
			}
			if msg := update.Message; msg != nil {
				// Skip messages without sender (channel posts, service messages)
				if msg.From == nil {
					continue
				}
				username := msg.From.UserName
				if !b.auth.IsAuthorized(username) {
					slog.Warn("Unauthorized access attempt", "username", username)
					b.sender.SendPlain(msg.Chat.ID, "Access denied")
					continue
				}
				// Log command without arguments for sensitive commands (import may contain tokens)
				slog.Info("Command received", "username", username, "command", sanitizeLogMessage(msg))
				b.router.RouteMessage(msg)
			}
			if cb := update.CallbackQuery; cb != nil {
				// Skip callbacks without sender (should not happen, but be defensive)
				if cb.From == nil {
					continue
				}
				// Acknowledge callback to prevent UI spinner hanging
				b.sender.AckCallback(cb.ID)
				username := cb.From.UserName
				if !b.auth.IsAuthorized(username) {
					slog.Warn("Unauthorized callback", "username", username)
					continue
				}
				slog.Info("Callback received", "username", username, "data", cb.Data)
				b.router.RouteCallback(cb)
			}
		}
	}
}

// sanitizeLogMessage returns a safe-to-log representation of the message.
// Sensitive commands (like /import) have their arguments redacted.
func sanitizeLogMessage(msg *tgbotapi.Message) string {
	if msg.IsCommand() {
		cmd := msg.Command()
		// Redact arguments for commands that may contain sensitive data (URLs with tokens)
		switch cmd {
		case "import":
			return "/" + cmd + " [REDACTED]"
		}
	}
	return msg.Text
}
