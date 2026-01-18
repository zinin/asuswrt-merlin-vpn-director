package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/bot"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/config"
)

const configPath = "/jffs/scripts/vpn-director/telegram-bot.json"

func main() {
	cfg, err := config.Load(configPath)
	if os.IsNotExist(err) {
		os.Exit(0)
	}
	if err != nil {
		log.Fatalf("[ERROR] Failed to load config: %v", err)
	}

	if strings.TrimSpace(cfg.BotToken) == "" {
		os.Exit(0)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	b, err := bot.New(cfg)
	if err != nil {
		log.Fatalf("[ERROR] Failed to create bot: %v", err)
	}

	log.Println("[INFO] Bot started")
	b.Run(ctx)
	log.Println("[INFO] Bot stopped")
}
