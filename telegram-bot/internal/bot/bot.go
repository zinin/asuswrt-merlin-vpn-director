package bot

import (
	"context"
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/config"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/wizard"
)

type Bot struct {
	api    *tgbotapi.BotAPI
	config *config.Config
	wizard *wizard.Manager
}

func New(cfg *config.Config) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(cfg.BotToken)
	if err != nil {
		return nil, err
	}

	log.Printf("[INFO] Authorized as @%s", api.Self.UserName)

	return &Bot{
		api:    api,
		config: cfg,
		wizard: wizard.NewManager(),
	}, nil
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
