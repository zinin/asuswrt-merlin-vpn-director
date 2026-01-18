package main

import (
	"context"
	"io"
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
	// Initialize logging to file + stdout
	logFile, err := os.OpenFile("/tmp/telegram-bot.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("[ERROR] Failed to open log file: %v", err)
	}
	defer logFile.Close()

	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.SetOutput(io.MultiWriter(os.Stdout, logFile))

	cfg, err := config.Load(configPath)
	if os.IsNotExist(err) {
		log.Println("[INFO] Config not found, run setup_telegram_bot.sh first")
		os.Exit(0)
	}
	if err != nil {
		log.Fatalf("[ERROR] Failed to load config: %v", err)
	}

	if strings.TrimSpace(cfg.BotToken) == "" {
		log.Println("[INFO] Bot token not configured, skipping")
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
