package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/bot"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/chatstore"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/config"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/devmode"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/logging"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/paths"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/updatechecker"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/updater"
)

var (
	Version     = "dev"
	VersionFull = "dev"
	Commit      = "unknown"
	BuildDate   = "unknown"
)

func versionString() string {
	return fmt.Sprintf("%s (%s, %s)", VersionFull, Commit, BuildDate)
}

const maxLogSize = 200 * 1024

func main() {
	devFlag := flag.Bool("dev", false, "Run in development mode (local testing)")
	flag.Parse()

	var p paths.Paths
	var opts []bot.Option

	if *devFlag {
		p = paths.DevPaths()
		opts = append(opts, bot.WithDevMode(devmode.NewExecutor()))
		// Validate testdata/dev exists before proceeding
		if _, err := os.Stat(p.ScriptsDir); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Error: %s not found\n", p.ScriptsDir)
			fmt.Fprintf(os.Stderr, "Run from telegram-bot/ directory: cd telegram-bot && go run ./cmd/bot --dev\n")
			os.Exit(1)
		}
	} else {
		p = paths.Default()
	}

	// Always add updater service
	opts = append(opts, bot.WithUpdater(updater.New()))

	// Initialize logger BEFORE config load (default INFO level)
	slogger, logger, err := logging.NewSlogLogger(p.BotLogPath)
	if err != nil {
		// Can't write to log file - fall back to stderr and exit
		fmt.Fprintf(os.Stderr, "Failed to initialize logging: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()

	slog.SetDefault(slogger)

	if *devFlag {
		slog.Info("Running in DEVELOPMENT mode", "config", p.BotConfigPath)
	}

	// Now load config - errors will be logged to file
	cfg, err := config.Load(p.BotConfigPath)
	if os.IsNotExist(err) {
		if *devFlag {
			slog.Info("Config not found", "hint", "copy testdata/dev/telegram-bot.json.example to testdata/dev/telegram-bot.json")
		} else {
			slog.Info("Config not found, run setup_telegram_bot.sh first")
		}
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

	// Create chat store for update notifications (not in dev mode)
	var store *chatstore.Store
	if !*devFlag {
		store = chatstore.New(p.DefaultDataDir + "/chats.json")
		opts = append(opts, bot.WithChatStore(store))
	}

	b, err := bot.New(cfg, p, Version, VersionFull, Commit, BuildDate, opts...)
	if err != nil {
		slog.Error("Failed to create bot", "error", err)
		os.Exit(1)
	}

	if err := b.RegisterCommands(); err != nil {
		slog.Warn("Failed to register commands", "error", err)
	}

	// Start update checker if configured (not in dev mode, not dev version)
	if cfg.UpdateCheckInterval > 0 && !*devFlag && Version != "dev" {
		checker := updatechecker.New(
			updater.New(),
			store,
			b.Sender(),
			b.Auth(),
			Version,
		)
		go checker.Run(ctx, cfg.UpdateCheckInterval)
	}

	slog.Info("Telegram Bot started", "version", versionString())
	b.Run(ctx)
	slog.Info("Bot stopped")
}
