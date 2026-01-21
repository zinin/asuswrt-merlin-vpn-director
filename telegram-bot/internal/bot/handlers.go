package bot

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
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
	defaultLogLines = 20
	serversPerPage  = 15
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

// extractCountry extracts country name from server name format "Country, City"
func extractCountry(name string) string {
	parts := strings.SplitN(name, ",", 2)
	country := strings.TrimSpace(parts[0])
	if country == "" {
		return "Other"
	}
	return country
}

// groupServersByCountry groups servers by country and returns formatted string
func groupServersByCountry(servers []vpnconfig.Server) string {
	if len(servers) == 0 {
		return ""
	}

	counts := make(map[string]int)
	for _, s := range servers {
		country := extractCountry(s.Name)
		counts[country]++
	}

	type countryCount struct {
		country string
		count   int
	}
	var sorted []countryCount
	for c, n := range counts {
		sorted = append(sorted, countryCount{c, n})
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].count != sorted[j].count {
			return sorted[i].count > sorted[j].count
		}
		return sorted[i].country < sorted[j].country
	})

	var parts []string
	maxShow := 10
	for i, cc := range sorted {
		if i >= maxShow {
			parts = append(parts, fmt.Sprintf("и ещё %d стран", len(sorted)-maxShow))
			break
		}
		parts = append(parts, fmt.Sprintf("%s (%d)", cc.country, cc.count))
	}
	return strings.Join(parts, ", ")
}

// buildCodeBlockText builds a code block message with optional truncation
func buildCodeBlockText(header, content string, maxLen int) string {
	// Calculate max content length: total - header - "\n```\n" (5) - "```" (3) - buffer
	maxContentLen := maxLen - len(header) - 10

	if len(content) > maxContentLen {
		// Truncate from beginning to show latest content
		content = content[len(content)-maxContentLen:]
		// Find first newline to avoid cutting mid-line
		if idx := strings.Index(content, "\n"); idx != -1 {
			content = content[idx+1:]
		}
		content = "...\n" + content
	}

	return fmt.Sprintf("%s\n```\n%s```", header, content)
}

// buildServersPage builds paginated server list with navigation keyboard
func buildServersPage(servers []vpnconfig.Server, page int) (string, tgbotapi.InlineKeyboardMarkup) {
	totalPages := (len(servers) + serversPerPage - 1) / serversPerPage
	if page < 0 {
		page = 0
	}
	if page >= totalPages {
		page = totalPages - 1
	}

	start := page * serversPerPage
	end := start + serversPerPage
	if end > len(servers) {
		end = len(servers)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🖥 *Servers* \\(%d\\), page %d/%d:\n",
		len(servers), page+1, totalPages))

	for i := start; i < end; i++ {
		s := servers[i]
		sb.WriteString(fmt.Sprintf("%d\\. %s — %s \\(%s\\)\n",
			i+1,
			escapeMarkdownV2(s.Name),
			escapeMarkdownV2(s.Address),
			escapeMarkdownV2(s.IP)))
	}

	// Navigation buttons
	var buttons []tgbotapi.InlineKeyboardButton

	if page > 0 {
		buttons = append(buttons,
			tgbotapi.NewInlineKeyboardButtonData("← Prev", fmt.Sprintf("servers:page:%d", page-1)))
	}

	buttons = append(buttons,
		tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("%d/%d", page+1, totalPages), "servers:noop"))

	if page < totalPages-1 {
		buttons = append(buttons,
			tgbotapi.NewInlineKeyboardButtonData("Next →", fmt.Sprintf("servers:page:%d", page+1)))
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(buttons)

	return sb.String(), keyboard
}

func (b *Bot) sendCodeBlock(chatID int64, header, content string) {
	text := buildCodeBlockText(header, content, maxMessageLength)
	b.sendMessage(chatID, text)
}

func (b *Bot) handleStart(msg *tgbotapi.Message) {
	text := `*VPN Director Bot*

Commands:
/status \- Xray status
/servers \- server list
/import <url> \- import servers
/configure \- configuration
/restart \- restart Xray
/stop \- stop Xray
/logs \- recent logs
/ip \- external IP
/version \- bot version`

	b.sendMessage(msg.Chat.ID, text)
}

func (b *Bot) handleStatus(msg *tgbotapi.Message) {
	result, err := shell.Exec(scriptsDir+"/vpn-director.sh", "status")
	if err != nil {
		b.sendMessage(msg.Chat.ID, escapeMarkdownV2(fmt.Sprintf("Error: %v", err)))
		return
	}
	text := fmt.Sprintf("📊 *VPN Director Status*:\n```\n%s```", result.Output)
	b.sendLongMessage(msg.Chat.ID, text)
}

func (b *Bot) handleServers(msg *tgbotapi.Message) {
	vpnCfg, err := vpnconfig.LoadVPNDirectorConfig(scriptsDir + "/vpn-director.json")
	if err != nil {
		b.sendMessage(msg.Chat.ID, escapeMarkdownV2(fmt.Sprintf("Config load error: %v", err)))
		return
	}

	dataDir := vpnCfg.DataDir
	if dataDir == "" {
		dataDir = scriptsDir + "/data"
	}

	servers, err := vpnconfig.LoadServers(dataDir + "/servers.json")
	if err != nil {
		b.sendMessage(msg.Chat.ID, escapeMarkdownV2(fmt.Sprintf("Error: %v", err)))
		return
	}

	if len(servers) == 0 {
		b.sendMessage(msg.Chat.ID, "No servers\\. Use /import to add servers\\.")
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🖥 *Servers* \\(%d\\):\n", len(servers)))
	for i, s := range servers {
		sb.WriteString(fmt.Sprintf("%d\\. %s — %s \\(%s\\)\n",
			i+1,
			escapeMarkdownV2(s.Name),
			escapeMarkdownV2(s.Address),
			escapeMarkdownV2(s.IP)))
	}
	b.sendLongMessage(msg.Chat.ID, sb.String())
}

func (b *Bot) handleRestart(msg *tgbotapi.Message) {
	b.sendMessage(msg.Chat.ID, "Restarting VPN Director\\.\\.\\.")
	result, err := shell.Exec(scriptsDir+"/vpn-director.sh", "restart")
	if err != nil {
		b.sendMessage(msg.Chat.ID, escapeMarkdownV2(fmt.Sprintf("Error: %v", err)))
		return
	}
	if result.ExitCode != 0 {
		b.sendMessage(msg.Chat.ID, fmt.Sprintf("Error \\(code %d\\):\n```\n%s```", result.ExitCode, result.Output))
		return
	}
	b.sendMessage(msg.Chat.ID, "✅ VPN Director restarted")
}

func (b *Bot) handleStop(msg *tgbotapi.Message) {
	result, err := shell.Exec(scriptsDir+"/vpn-director.sh", "stop")
	if err != nil {
		b.sendMessage(msg.Chat.ID, escapeMarkdownV2(fmt.Sprintf("Error: %v", err)))
		return
	}
	if result.ExitCode != 0 {
		b.sendMessage(msg.Chat.ID, fmt.Sprintf("Error \\(code %d\\):\n```\n%s```", result.ExitCode, result.Output))
		return
	}
	b.sendMessage(msg.Chat.ID, "⏹ VPN Director stopped")
}

func (b *Bot) handleLogs(msg *tgbotapi.Message) {
	args := strings.Fields(msg.CommandArguments())

	source := "all"
	lines := defaultLogLines

	// Parse arguments: /logs [source] [lines]
	if len(args) >= 1 {
		switch args[0] {
		case "bot", "vpn", "all":
			source = args[0]
		default:
			// Maybe it's a number
			if n, err := strconv.Atoi(args[0]); err == nil && n > 0 {
				lines = n
			} else {
				b.sendMessage(msg.Chat.ID, "Usage: `/logs [bot|vpn|all] [lines]`")
				return
			}
		}
	}

	if len(args) >= 2 {
		if n, err := strconv.Atoi(args[1]); err == nil && n > 0 {
			lines = n
		}
	}

	linesStr := strconv.Itoa(lines)

	if source == "bot" || source == "all" {
		b.sendLogFile(msg.Chat.ID, botLogPath, "Bot", linesStr)
	}

	if source == "vpn" || source == "all" {
		b.sendLogFile(msg.Chat.ID, vpnLogPath, "VPN Director", linesStr)
	}
}

func (b *Bot) sendLogFile(chatID int64, path, name, lines string) {
	result, err := shell.Exec("tail", "-n", lines, path)
	if err != nil {
		b.sendMessage(chatID, escapeMarkdownV2(fmt.Sprintf("Error reading %s logs: %v", name, err)))
		return
	}

	output := result.Output
	if output == "" {
		output = "(empty)"
	}

	text := fmt.Sprintf("📋 *%s logs* \\(last %s lines\\):\n```\n%s```",
		escapeMarkdownV2(name), lines, output)
	b.sendLongMessage(chatID, text)
}

func (b *Bot) handleIP(msg *tgbotapi.Message) {
	result, err := shell.Exec("curl", "-s", "--connect-timeout", "5", "--max-time", "10", "ifconfig.me")
	if err != nil {
		b.sendMessage(msg.Chat.ID, escapeMarkdownV2(fmt.Sprintf("Error: %v", err)))
		return
	}
	ip := strings.TrimSpace(result.Output)
	b.sendMessage(msg.Chat.ID, fmt.Sprintf("🌐 External IP: `%s`", ip))
}

func (b *Bot) handleVersion(msg *tgbotapi.Message) {
	b.sendMessage(msg.Chat.ID, escapeMarkdownV2(b.version))
}

func (b *Bot) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "MarkdownV2"
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("[ERROR] Failed to send message to %d: %v", chatID, err)
	}
}

func (b *Bot) sendPlainMessage(chatID int64, text string) {
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
		b.sendMessage(msg.Chat.ID, "Usage: `/import <url>`")
		return
	}

	// Validate URL scheme
	parsedURL, err := url.Parse(args)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		b.sendMessage(msg.Chat.ID, "Invalid URL\\. Use http:// or https://")
		return
	}

	b.sendMessage(msg.Chat.ID, "Loading server list\\.\\.\\.")

	// HTTP client with timeout
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(args)
	if err != nil {
		b.sendMessage(msg.Chat.ID, escapeMarkdownV2(fmt.Sprintf("Download error: %v", err)))
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
		b.sendMessage(msg.Chat.ID, escapeMarkdownV2(fmt.Sprintf("Read error: %v", err)))
		return
	}

	servers, parseErrors := vless.DecodeSubscription(string(body))
	if len(servers) == 0 {
		errMsg := "No VLESS servers found"
		if len(parseErrors) > 0 {
			errMsg += fmt.Sprintf("\nErrors: %v", parseErrors)
		}
		b.sendMessage(msg.Chat.ID, escapeMarkdownV2(errMsg))
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
		b.sendMessage(msg.Chat.ID, escapeMarkdownV2(fmt.Sprintf("Directory creation error: %v", err)))
		return
	}

	if err := vpnconfig.SaveServers(dataDir+"/servers.json", resolved); err != nil {
		b.sendMessage(msg.Chat.ID, escapeMarkdownV2(fmt.Sprintf("Save error: %v", err)))
		return
	}

	// Response with full stats
	var sb strings.Builder
	if resolveErrors > 0 {
		sb.WriteString(fmt.Sprintf("⚠️ Imported %d of %d \\(%d DNS errors\\):\n",
			len(resolved), totalParsed, resolveErrors))
	} else {
		sb.WriteString(fmt.Sprintf("✅ Imported %d servers:\n", len(resolved)))
	}
	for i, s := range resolved {
		sb.WriteString(fmt.Sprintf("%d\\. %s — %s\n", i+1, escapeMarkdownV2(s.Name), escapeMarkdownV2(s.Address)))
	}
	if len(parseErrors) > 0 {
		sb.WriteString(fmt.Sprintf("\n\\(%d with errors\\)", len(parseErrors)))
	}
	b.sendLongMessage(msg.Chat.ID, sb.String())
}
