package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/bot"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/config"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/logging"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/paths"
)

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

func versionString() string {
	return fmt.Sprintf("%s (%s, %s)", Version, Commit, BuildDate)
}

const maxLogSize = 200 * 1024

func main() {
	p := paths.Default()

	logger, err := logging.New(p.BotLogPath)
	if err != nil {
		log.Fatalf("[ERROR] Failed to initialize logging: %v", err)
	}
	defer logger.Close()

	cfg, err := config.Load(p.BotConfigPath)
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

	logger.StartRotation(ctx, []string{p.BotLogPath, p.VPNLogPath}, maxLogSize, time.Minute)

	b, err := bot.New(cfg, p, versionString())
	if err != nil {
		log.Fatalf("[ERROR] Failed to create bot: %v", err)
	}

	if err := b.RegisterCommands(); err != nil {
		log.Printf("[WARN] Failed to register commands: %v", err)
	}

	log.Printf("[INFO] Telegram Bot %s started", versionString())
	b.Run(ctx)
	log.Println("[INFO] Bot stopped")
}
