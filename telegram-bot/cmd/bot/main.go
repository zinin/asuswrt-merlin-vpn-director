package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/bot"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/config"
)

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

func versionString() string {
	return fmt.Sprintf("%s (%s, %s)", Version, Commit, BuildDate)
}

const configPath = "/opt/vpn-director/telegram-bot.json"

const (
	botLogPath = "/tmp/telegram-bot.log"
	vpnLogPath = "/tmp/vpn-director.log"
	maxLogSize = 200 * 1024 // 200KB
)

func truncateLogIfNeeded(path string, maxSize int64) {
	info, err := os.Stat(path)
	if err != nil {
		return // File doesn't exist, ignore
	}
	if info.Size() > maxSize {
		if err := os.Truncate(path, 0); err != nil {
			log.Printf("[WARN] Failed to truncate %s: %v", path, err)
		} else {
			log.Printf("[INFO] Truncated log file %s (was %d bytes)", path, info.Size())
		}
	}
}

func startLogRotation(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				truncateLogIfNeeded(botLogPath, maxLogSize)
				truncateLogIfNeeded(vpnLogPath, maxLogSize)
			}
		}
	}()
}

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

	startLogRotation(ctx)

	b, err := bot.New(cfg, versionString())
	if err != nil {
		log.Fatalf("[ERROR] Failed to create bot: %v", err)
	}

	log.Printf("[INFO] Telegram Bot %s started", versionString())
	b.Run(ctx)
	log.Println("[INFO] Bot stopped")
}
