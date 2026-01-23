// internal/bot/bot.go
package bot

import (
	"context"
	"log"

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

// New creates a new Bot with full dependency injection
func New(cfg *config.Config, p paths.Paths, version string) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(cfg.BotToken)
	if err != nil {
		return nil, err
	}

	log.Printf("[INFO] Authorized as @%s", api.Self.UserName)

	// Create sender
	sender := telegram.NewSender(api)

	// Create services
	configSvc := service.NewConfigService(p.ScriptsDir, p.DefaultDataDir)
	vpnSvc := service.NewVPNDirectorService(p.ScriptsDir, nil)
	xraySvc := service.NewXrayService(p.XrayTemplate, p.XrayConfig)
	networkSvc := service.NewNetworkService(nil)
	logSvc := service.NewLogService(nil)

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

	log.Printf("[INFO] Registered %d bot commands", len(commands))
	return nil
}

// Run starts the bot and processes updates until context is cancelled
func (b *Bot) Run(ctx context.Context) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := b.api.GetUpdatesChan(u)

	log.Println("[INFO] Bot started, waiting for messages...")

	for {
		select {
		case <-ctx.Done():
			log.Println("[INFO] Shutting down bot...")
			b.api.StopReceivingUpdates()
			return
		case update := <-updates:
			if msg := update.Message; msg != nil {
				// Skip messages without sender (channel posts, service messages)
				if msg.From == nil {
					continue
				}
				username := msg.From.UserName
				if !b.auth.IsAuthorized(username) {
					log.Printf("[WARN] Unauthorized: %s", username)
					b.sender.SendPlain(msg.Chat.ID, "Access denied")
					continue
				}
				log.Printf("[INFO] Command from %s: %s", username, msg.Text)
				b.router.RouteMessage(msg)
			}
			if cb := update.CallbackQuery; cb != nil {
				// Skip callbacks without sender (should not happen, but be defensive)
				if cb.From == nil {
					continue
				}
				username := cb.From.UserName
				if !b.auth.IsAuthorized(username) {
					log.Printf("[WARN] Unauthorized callback: %s", username)
					continue
				}
				log.Printf("[INFO] Callback from %s: %s", username, cb.Data)
				b.router.RouteCallback(cb)
			}
		}
	}
}
