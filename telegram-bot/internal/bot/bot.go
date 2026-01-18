package bot

import (
	"context"
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/config"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/wizard"
)

type Bot struct {
	api     *tgbotapi.BotAPI
	config  *config.Config
	wizard  *wizard.Manager
	version string
}

func New(cfg *config.Config, version string) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(cfg.BotToken)
	if err != nil {
		return nil, err
	}

	log.Printf("[INFO] Authorized as @%s", api.Self.UserName)

	b := &Bot{
		api:     api,
		config:  cfg,
		wizard:  wizard.NewManager(),
		version: version,
	}

	if err := b.registerCommands(); err != nil {
		log.Printf("[WARN] Failed to register commands: %v", err)
	}

	return b, nil
}

func (b *Bot) registerCommands() error {
	commands := []tgbotapi.BotCommand{
		{Command: "status", Description: "Xray status"},
		{Command: "servers", Description: "Server list"},
		{Command: "import", Description: "Import servers from URL"},
		{Command: "configure", Description: "Configuration wizard"},
		{Command: "restart", Description: "Restart Xray"},
		{Command: "stop", Description: "Stop Xray"},
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

func (b *Bot) IsAuthorized(username string) bool {
	return isAuthorized(username, b.config.AllowedUsers)
}

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
			if update.Message != nil {
				b.handleMessage(update.Message)
			}
			if update.CallbackQuery != nil {
				b.handleCallback(update.CallbackQuery)
			}
		}
	}
}

func (b *Bot) handleMessage(msg *tgbotapi.Message) {
	username := msg.From.UserName

	if !b.IsAuthorized(username) {
		log.Printf("[WARN] Unauthorized: %s", username)
		b.sendMessage(msg.Chat.ID, "Access denied")
		return
	}

	log.Printf("[INFO] Command from %s: %s", username, msg.Text)

	cmd := msg.Command()
	switch cmd {
	case "start":
		b.handleStart(msg)
	case "status":
		b.handleStatus(msg)
	case "servers":
		b.handleServers(msg)
	case "restart":
		b.handleRestart(msg)
	case "stop":
		b.handleStop(msg)
	case "logs":
		b.handleLogs(msg)
	case "ip":
		b.handleIP(msg)
	case "import":
		b.handleImport(msg)
	case "configure":
		b.handleConfigure(msg)
	case "version":
		b.handleVersion(msg)
	default:
		b.handleWizardInput(msg)
	}
}

func (b *Bot) handleCallback(cb *tgbotapi.CallbackQuery) {
	username := cb.From.UserName

	if !b.IsAuthorized(username) {
		log.Printf("[WARN] Unauthorized callback: %s", username)
		return
	}

	log.Printf("[INFO] Callback from %s: %s", username, cb.Data)
	b.handleWizardCallback(cb)
}
