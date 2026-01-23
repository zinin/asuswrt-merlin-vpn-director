package main

import (
	"context"
	"fmt"
	"log/slog"
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

	// Initialize logger BEFORE config load (default INFO level)
	slogger, logger, err := logging.NewSlogLogger(p.BotLogPath)
	if err != nil {
		// Can't write to log file - fall back to stderr and exit
		fmt.Fprintf(os.Stderr, "Failed to initialize logging: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()

	slog.SetDefault(slogger)

	// Now load config - errors will be logged to file
	cfg, err := config.Load(p.BotConfigPath)
	if os.IsNotExist(err) {
		slog.Info("Config not found, run setup_telegram_bot.sh first")
		os.Exit(0)
	}
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	// Update log level from config
	if cfg.LogLevel != "" {
		logger.SetLevel(cfg.LogLevel)
		slog.Debug("Log level set from config", "level", cfg.LogLevel)
	}

	if strings.TrimSpace(cfg.BotToken) == "" {
		slog.Info("Bot token not configured, skipping")
		os.Exit(0)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger.StartRotation(ctx, []string{p.BotLogPath, p.VPNLogPath}, maxLogSize, time.Minute)

	b, err := bot.New(cfg, p, versionString())
	if err != nil {
		slog.Error("Failed to create bot", "error", err)
		os.Exit(1)
	}

	if err := b.RegisterCommands(); err != nil {
		slog.Warn("Failed to register commands", "error", err)
	}

	slog.Info("Telegram Bot started", "version", versionString())
	b.Run(ctx)
	slog.Info("Bot stopped")
}
