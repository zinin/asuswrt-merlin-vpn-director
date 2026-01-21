package bot

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/shell"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/vless"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/vpnconfig"
)

const scriptsDir = "/jffs/scripts/vpn-director"
const maxMessageLength = 4000 // Telegram limit is 4096, leave margin

const (
	botLogPath      = "/tmp/telegram-bot.log"
	vpnLogPath      = "/tmp/vpn-director.log"
	maxLogSize      = 200 * 1024 // 200KB
	defaultLogLines = 20
)

// escapeMarkdownV2 escapes special characters for Telegram MarkdownV2
func escapeMarkdownV2(text string) string {
	replacer := strings.NewReplacer(
		"_", "\\_",
		"*", "\\*",
		"[", "\\[",
		"]", "\\]",
		"(", "\\(",
		")", "\\)",
		"~", "\\~",
		"`", "\\`",
		">", "\\>",
		"#", "\\#",
		"+", "\\+",
		"-", "\\-",
		"=", "\\=",
		"|", "\\|",
		"{", "\\{",
		"}", "\\}",
		".", "\\.",
		"!", "\\!",
	)
	return replacer.Replace(text)
}

func (b *Bot) handleStart(msg *tgbotapi.Message) {
	text := `VPN Director Bot

Commands:
/status - Xray status
/servers - server list
/import <url> - import servers
/configure - configuration
/restart - restart Xray
/stop - stop Xray
/logs - recent logs
/ip - external IP
/version - bot version`

	b.sendMessage(msg.Chat.ID, text)
}

func (b *Bot) handleStatus(msg *tgbotapi.Message) {
	result, err := shell.Exec(scriptsDir+"/vpn-director.sh", "status")
	if err != nil {
		b.sendMessage(msg.Chat.ID, fmt.Sprintf("Error: %v", err))
		return
	}
	b.sendMessage(msg.Chat.ID, result.Output)
}

func (b *Bot) handleServers(msg *tgbotapi.Message) {
	vpnCfg, err := vpnconfig.LoadVPNDirectorConfig(scriptsDir + "/vpn-director.json")
	if err != nil {
		b.sendMessage(msg.Chat.ID, fmt.Sprintf("Config load error: %v", err))
		return
	}

	dataDir := vpnCfg.DataDir
	if dataDir == "" {
		dataDir = scriptsDir + "/data"
	}

	servers, err := vpnconfig.LoadServers(dataDir + "/servers.json")
	if err != nil {
		b.sendMessage(msg.Chat.ID, fmt.Sprintf("Error: %v", err))
		return
	}

	if len(servers) == 0 {
		b.sendMessage(msg.Chat.ID, "No servers. Use /import to add servers.")
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Servers (%d):\n", len(servers)))
	for i, s := range servers {
		sb.WriteString(fmt.Sprintf("%d. %s — %s (%s)\n", i+1, s.Name, s.Address, s.IP))
	}
	b.sendLongMessage(msg.Chat.ID, sb.String())
}

func (b *Bot) handleRestart(msg *tgbotapi.Message) {
	b.sendMessage(msg.Chat.ID, "Restarting VPN Director...")
	result, err := shell.Exec(scriptsDir+"/vpn-director.sh", "restart")
	if err != nil {
		b.sendMessage(msg.Chat.ID, fmt.Sprintf("Error: %v", err))
		return
	}
	if result.ExitCode != 0 {
		b.sendMessage(msg.Chat.ID, fmt.Sprintf("Error (code %d):\n%s", result.ExitCode, result.Output))
		return
	}
	b.sendMessage(msg.Chat.ID, "VPN Director restarted")
}

func (b *Bot) handleStop(msg *tgbotapi.Message) {
	result, err := shell.Exec(scriptsDir+"/vpn-director.sh", "stop")
	if err != nil {
		b.sendMessage(msg.Chat.ID, fmt.Sprintf("Error: %v", err))
		return
	}
	if result.ExitCode != 0 {
		b.sendMessage(msg.Chat.ID, fmt.Sprintf("Error (code %d):\n%s", result.ExitCode, result.Output))
		return
	}
	b.sendMessage(msg.Chat.ID, "VPN Director stopped")
}

func (b *Bot) handleLogs(msg *tgbotapi.Message) {
	result, err := shell.Exec("tail", "-n", "20", "/tmp/vpn_director.log")
	if err != nil {
		b.sendMessage(msg.Chat.ID, fmt.Sprintf("Error: %v", err))
		return
	}
	if result.Output == "" {
		b.sendMessage(msg.Chat.ID, "Log is empty")
		return
	}
	b.sendMessage(msg.Chat.ID, "```\n"+result.Output+"```")
}

func (b *Bot) handleIP(msg *tgbotapi.Message) {
	result, err := shell.Exec("curl", "-s", "--connect-timeout", "5", "--max-time", "10", "ifconfig.me")
	if err != nil {
		b.sendMessage(msg.Chat.ID, fmt.Sprintf("Error: %v", err))
		return
	}
	b.sendMessage(msg.Chat.ID, fmt.Sprintf("External IP: %s", strings.TrimSpace(result.Output)))
}

func (b *Bot) handleVersion(msg *tgbotapi.Message) {
	b.sendMessage(msg.Chat.ID, b.version)
}

func (b *Bot) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("[ERROR] Failed to send message to %d: %v", chatID, err)
	}
}

func (b *Bot) sendLongMessage(chatID int64, text string) {
	if len(text) <= maxMessageLength {
		b.sendMessage(chatID, text)
		return
	}

	// Split by lines, respecting message limit
	lines := strings.Split(text, "\n")
	var chunk strings.Builder

	for _, line := range lines {
		if chunk.Len()+len(line)+1 > maxMessageLength {
			b.sendMessage(chatID, chunk.String())
			chunk.Reset()
		}
		if chunk.Len() > 0 {
			chunk.WriteString("\n")
		}
		chunk.WriteString(line)
	}

	if chunk.Len() > 0 {
		b.sendMessage(chatID, chunk.String())
	}
}

func (b *Bot) handleImport(msg *tgbotapi.Message) {
	args := msg.CommandArguments()
	if args == "" {
		b.sendMessage(msg.Chat.ID, "Usage: /import <url>")
		return
	}

	// Validate URL scheme
	parsedURL, err := url.Parse(args)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		b.sendMessage(msg.Chat.ID, "Invalid URL. Use http:// or https://")
		return
	}

	b.sendMessage(msg.Chat.ID, "Loading server list...")

	// HTTP client with timeout
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(args)
	if err != nil {
		b.sendMessage(msg.Chat.ID, fmt.Sprintf("Download error: %v", err))
		return
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		b.sendMessage(msg.Chat.ID, fmt.Sprintf("Error: HTTP %d", resp.StatusCode))
		return
	}

	// Limit body size (1MB max)
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		b.sendMessage(msg.Chat.ID, fmt.Sprintf("Read error: %v", err))
		return
	}

	servers, parseErrors := vless.DecodeSubscription(string(body))
	if len(servers) == 0 {
		errMsg := "No VLESS servers found"
		if len(parseErrors) > 0 {
			errMsg += fmt.Sprintf("\nErrors: %v", parseErrors)
		}
		b.sendMessage(msg.Chat.ID, errMsg)
		return
	}

	// Resolve IPs
	var resolved []vpnconfig.Server
	var resolveErrors int
	totalParsed := len(servers)

	for _, s := range servers {
		if err := s.ResolveIP(); err != nil {
			resolveErrors++
			continue
		}
		resolved = append(resolved, vpnconfig.Server{
			Address: s.Address,
			Port:    s.Port,
			UUID:    s.UUID,
			Name:    s.Name,
			IP:      s.IP,
		})
	}

	if len(resolved) == 0 {
		b.sendMessage(msg.Chat.ID, "Could not resolve IP for any server")
		return
	}

	vpnCfg, _ := vpnconfig.LoadVPNDirectorConfig(scriptsDir + "/vpn-director.json")
	dataDir := scriptsDir + "/data"
	if vpnCfg != nil && vpnCfg.DataDir != "" {
		dataDir = vpnCfg.DataDir
	}

	// Ensure data dir exists
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		b.sendMessage(msg.Chat.ID, fmt.Sprintf("Directory creation error: %v", err))
		return
	}

	if err := vpnconfig.SaveServers(dataDir+"/servers.json", resolved); err != nil {
		b.sendMessage(msg.Chat.ID, fmt.Sprintf("Save error: %v", err))
		return
	}

	// Response with full stats
	var sb strings.Builder
	if resolveErrors > 0 {
		sb.WriteString(fmt.Sprintf("⚠️ Imported %d of %d (%d DNS errors):\n",
			len(resolved), totalParsed, resolveErrors))
	} else {
		sb.WriteString(fmt.Sprintf("✓ Imported %d servers:\n", len(resolved)))
	}
	for i, s := range resolved {
		sb.WriteString(fmt.Sprintf("%d. %s — %s\n", i+1, s.Name, s.Address))
	}
	if len(parseErrors) > 0 {
		sb.WriteString(fmt.Sprintf("\n(%d with errors)", len(parseErrors)))
	}
	b.sendLongMessage(msg.Chat.ID, sb.String())
}
