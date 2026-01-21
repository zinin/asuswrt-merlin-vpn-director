package bot

import (
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/shell"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/vpnconfig"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/wizard"
)

// handleConfigure starts the configuration wizard
func (b *Bot) handleConfigure(msg *tgbotapi.Message) {
	state := b.wizard.Start(msg.Chat.ID)
	b.sendServerSelection(msg.Chat.ID, state)
}

// handleWizardInput handles text input during wizard (e.g., IP address entry)
func (b *Bot) handleWizardInput(msg *tgbotapi.Message) {
	state := b.wizard.Get(msg.Chat.ID)
	if state == nil {
		return
	}

	if state.GetStep() == wizard.StepClientIP {
		ip := strings.TrimSpace(msg.Text)
		if !isValidLANIP(ip) {
			b.sendMessage(msg.Chat.ID, escapeMarkdownV2("Invalid IP. Enter IP from range 192.168.x.x, 10.x.x.x or 172.16-31.x.x"))
			return
		}
		state.SetPendingIP(ip)
		state.SetStep(wizard.StepClientRoute)
		b.sendRouteSelection(msg.Chat.ID, ip)
	}
}

// handleWizardCallback handles inline button callbacks
func (b *Bot) handleWizardCallback(cb *tgbotapi.CallbackQuery) {
	chatID := cb.Message.Chat.ID
	data := cb.Data
	state := b.wizard.Get(chatID)

	// Acknowledge callback
	if _, err := b.api.Send(tgbotapi.NewCallback(cb.ID, "")); err != nil {
		log.Printf("[ERROR] Failed to acknowledge callback: %v", err)
	}

	if data == "cancel" {
		b.wizard.Clear(chatID)
		b.sendMessage(chatID, "Configuration cancelled")
		return
	}

	if state == nil {
		b.sendMessage(chatID, escapeMarkdownV2("No active session. Use /configure"))
		return
	}

	switch {
	case strings.HasPrefix(data, "server:"):
		idx := 0
		fmt.Sscanf(data, "server:%d", &idx)

		// Validate server index bounds
		dataDir := scriptsDir + "/data"
		if vpnCfg, _ := vpnconfig.LoadVPNDirectorConfig(scriptsDir + "/vpn-director.json"); vpnCfg != nil && vpnCfg.DataDir != "" {
			dataDir = vpnCfg.DataDir
		}
		servers, _ := vpnconfig.LoadServers(dataDir + "/servers.json")

		if idx < 0 || idx >= len(servers) {
			b.sendMessage(chatID, "Invalid server index")
			return
		}

		state.SetServerIndex(idx)
		state.SetStep(wizard.StepExclusions)
		// Default: include ru in exclusions
		state.SetExclusion("ru", true)
		b.sendExclusionsSelection(chatID, state)

	case strings.HasPrefix(data, "excl:"):
		ex := strings.TrimPrefix(data, "excl:")
		if ex == "done" {
			state.SetStep(wizard.StepClients)
			b.sendClientsSelection(chatID, state)
		} else {
			state.ToggleExclusion(ex)
			b.updateExclusionsMessage(cb.Message, state)
		}

	case strings.HasPrefix(data, "client:"):
		action := strings.TrimPrefix(data, "client:")
		switch action {
		case "add":
			state.SetStep(wizard.StepClientIP)
			b.sendClientIPPrompt(chatID)
		case "del":
			state.RemoveLastClient()
			b.sendClientsSelection(chatID, state)
		case "done":
			state.SetStep(wizard.StepConfirm)
			b.sendConfirmation(chatID, state)
		}

	case strings.HasPrefix(data, "route:"):
		route := strings.TrimPrefix(data, "route:")
		state.AddClient(wizard.ClientRoute{
			IP:    state.GetPendingIP(),
			Route: route,
		})
		state.SetPendingIP("")
		state.SetStep(wizard.StepClients)
		b.sendClientsSelection(chatID, state)

	case data == "apply":
		b.applyConfig(chatID, state)
	}
}

// Step 1: Server selection
func (b *Bot) sendServerSelection(chatID int64, _ *wizard.State) {
	vpnCfg, err := vpnconfig.LoadVPNDirectorConfig(scriptsDir + "/vpn-director.json")
	if err != nil {
		b.sendMessage(chatID, "Config load error")
		b.wizard.Clear(chatID)
		return
	}

	dataDir := vpnCfg.DataDir
	if dataDir == "" {
		dataDir = scriptsDir + "/data"
	}

	servers, err := vpnconfig.LoadServers(dataDir + "/servers.json")
	if err != nil || len(servers) == 0 {
		b.sendMessage(chatID, escapeMarkdownV2("No servers found. Use /import"))
		b.wizard.Clear(chatID)
		return
	}

	cols := getServerGridColumns(len(servers))

	var rows [][]tgbotapi.InlineKeyboardButton
	var row []tgbotapi.InlineKeyboardButton
	for i, s := range servers {
		btnText := fmt.Sprintf("%d. %s", i+1, s.Name)
		btn := tgbotapi.NewInlineKeyboardButtonData(btnText, fmt.Sprintf("server:%d", i))
		row = append(row, btn)
		if len(row) == cols {
			rows = append(rows, row)
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("Cancel", "cancel"),
	))

	msg := tgbotapi.NewMessage(chatID, escapeMarkdownV2(fmt.Sprintf("Step 1/4: Select Xray server (%d available)", len(servers))))
	msg.ParseMode = "MarkdownV2"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("[ERROR] Failed to send server selection: %v", err)
	}
}

// Step 2: Exclusions selection
func (b *Bot) sendExclusionsSelection(chatID int64, state *wizard.State) {
	defaultExclusions := wizard.DefaultExclusions
	stateExclusions := state.GetExclusions()

	var rows [][]tgbotapi.InlineKeyboardButton
	var row []tgbotapi.InlineKeyboardButton
	for _, ex := range defaultExclusions {
		btnText := formatExclusionButton(ex, stateExclusions[ex])
		btn := tgbotapi.NewInlineKeyboardButtonData(btnText, "excl:"+ex)
		row = append(row, btn)
		if len(row) == 2 {
			rows = append(rows, row)
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("Done", "excl:done"),
		tgbotapi.NewInlineKeyboardButtonData("Cancel", "cancel"),
	))

	var selected []string
	for k, v := range stateExclusions {
		if v {
			selected = append(selected, k)
		}
	}
	sort.Strings(selected)
	text := escapeMarkdownV2("Step 2/4: Exclude from proxy") + "\n"
	if len(selected) > 0 {
		text += escapeMarkdownV2(fmt.Sprintf("Selected: %s", strings.Join(selected, ", ")))
	} else {
		text += escapeMarkdownV2("Selected: (none)")
	}

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "MarkdownV2"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("[ERROR] Failed to send exclusions selection: %v", err)
	}
}

func (b *Bot) updateExclusionsMessage(msg *tgbotapi.Message, state *wizard.State) {
	defaultExclusions := wizard.DefaultExclusions
	stateExclusions := state.GetExclusions()

	var rows [][]tgbotapi.InlineKeyboardButton
	var row []tgbotapi.InlineKeyboardButton
	for _, ex := range defaultExclusions {
		btnText := formatExclusionButton(ex, stateExclusions[ex])
		btn := tgbotapi.NewInlineKeyboardButtonData(btnText, "excl:"+ex)
		row = append(row, btn)
		if len(row) == 2 {
			rows = append(rows, row)
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("Done", "excl:done"),
		tgbotapi.NewInlineKeyboardButtonData("Cancel", "cancel"),
	))

	var selected []string
	for k, v := range stateExclusions {
		if v {
			selected = append(selected, k)
		}
	}
	sort.Strings(selected)
	text := escapeMarkdownV2("Step 2/4: Exclude from proxy") + "\n"
	if len(selected) > 0 {
		text += escapeMarkdownV2(fmt.Sprintf("Selected: %s", strings.Join(selected, ", ")))
	} else {
		text += escapeMarkdownV2("Selected: (none)")
	}

	edit := tgbotapi.NewEditMessageTextAndMarkup(
		msg.Chat.ID, msg.MessageID, text,
		tgbotapi.NewInlineKeyboardMarkup(rows...),
	)
	edit.ParseMode = "MarkdownV2"
	if _, err := b.api.Send(edit); err != nil {
		log.Printf("[ERROR] Failed to update exclusions message: %v", err)
	}
}

// Step 3: Clients management
func (b *Bot) sendClientsSelection(chatID int64, state *wizard.State) {
	clients := state.GetClients()

	var sb strings.Builder
	sb.WriteString(escapeMarkdownV2("Step 3/4: Clients") + "\n\n")

	if len(clients) == 0 {
		sb.WriteString(escapeMarkdownV2("(none yet)") + "\n")
	} else {
		for _, c := range clients {
			sb.WriteString(escapeMarkdownV2(fmt.Sprintf("* %s -> %s", c.IP, c.Route)) + "\n")
		}
	}

	rows := [][]tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Add", "client:add"),
			tgbotapi.NewInlineKeyboardButtonData("Remove", "client:del"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Done", "client:done"),
			tgbotapi.NewInlineKeyboardButtonData("Cancel", "cancel"),
		),
	}

	msg := tgbotapi.NewMessage(chatID, sb.String())
	msg.ParseMode = "MarkdownV2"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("[ERROR] Failed to send clients selection: %v", err)
	}
}

func (b *Bot) sendClientIPPrompt(chatID int64) {
	msg := tgbotapi.NewMessage(chatID, "Enter client IP address\n(e.g.: 192.168.1.100)")
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("[ERROR] Failed to send client IP prompt: %v", err)
	}
}

func (b *Bot) sendRouteSelection(chatID int64, ip string) {
	var rows [][]tgbotapi.InlineKeyboardButton

	// Xray option
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("Xray", "route:xray"),
	))

	// OpenVPN options
	var ovpnRow []tgbotapi.InlineKeyboardButton
	for i := 1; i <= 5; i++ {
		ovpnRow = append(ovpnRow, tgbotapi.NewInlineKeyboardButtonData(
			fmt.Sprintf("ovpnc%d", i), fmt.Sprintf("route:ovpnc%d", i)))
	}
	rows = append(rows, ovpnRow)

	// WireGuard options
	var wgRow []tgbotapi.InlineKeyboardButton
	for i := 1; i <= 5; i++ {
		wgRow = append(wgRow, tgbotapi.NewInlineKeyboardButtonData(
			fmt.Sprintf("wgc%d", i), fmt.Sprintf("route:wgc%d", i)))
	}
	rows = append(rows, wgRow)

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("Cancel", "cancel"),
	))

	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Where to route traffic for %s?", ip))
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("[ERROR] Failed to send route selection: %v", err)
	}
}

// Step 4: Confirmation
func (b *Bot) sendConfirmation(chatID int64, state *wizard.State) {
	vpnCfg, _ := vpnconfig.LoadVPNDirectorConfig(scriptsDir + "/vpn-director.json")
	dataDir := scriptsDir + "/data"
	if vpnCfg != nil && vpnCfg.DataDir != "" {
		dataDir = vpnCfg.DataDir
	}
	servers, _ := vpnconfig.LoadServers(dataDir + "/servers.json")

	serverIndex := state.GetServerIndex()
	exclusions := state.GetExclusions()
	clients := state.GetClients()

	var sb strings.Builder
	sb.WriteString("Step 4/4: Confirmation\n\n")

	if serverIndex < len(servers) {
		s := servers[serverIndex]
		sb.WriteString(fmt.Sprintf("Xray server: %s (%s)\n", s.Name, s.IP))
	}

	var excl []string
	for k, v := range exclusions {
		if v {
			excl = append(excl, k)
		}
	}
	if len(excl) > 0 {
		sb.WriteString(fmt.Sprintf("Exclusions: %s\n", strings.Join(excl, ", ")))
	}

	sb.WriteString("\nClients:\n")
	if len(clients) == 0 {
		sb.WriteString("(none)\n")
	} else {
		for _, c := range clients {
			sb.WriteString(fmt.Sprintf("* %s -> %s\n", c.IP, c.Route))
		}
	}

	rows := [][]tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Apply", "apply"),
			tgbotapi.NewInlineKeyboardButtonData("Cancel", "cancel"),
		),
	}

	msg := tgbotapi.NewMessage(chatID, sb.String())
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("[ERROR] Failed to send confirmation: %v", err)
	}
}

// Step 5: Apply configuration
func (b *Bot) applyConfig(chatID int64, state *wizard.State) {
	b.sendMessage(chatID, "Applying configuration...")

	// Load current config
	vpnCfg, err := vpnconfig.LoadVPNDirectorConfig(scriptsDir + "/vpn-director.json")
	if err != nil {
		b.sendMessage(chatID, fmt.Sprintf("Config load error: %v", err))
		return
	}

	dataDir := vpnCfg.DataDir
	if dataDir == "" {
		dataDir = scriptsDir + "/data"
	}
	servers, err := vpnconfig.LoadServers(dataDir + "/servers.json")
	if err != nil && !os.IsNotExist(err) {
		b.sendMessage(chatID, fmt.Sprintf("Server load error: %v", err))
		return
	}

	// Get state data with thread-safe getters
	clients := state.GetClients()
	exclusions := state.GetExclusions()
	serverIndex := state.GetServerIndex()

	// Build new configuration
	var xrayClients []string
	var tunDirRules []string

	for _, c := range clients {
		if c.Route == "xray" {
			xrayClients = append(xrayClients, c.IP)
		} else {
			// Tunnel Director rule: table:ip/32::any:exclusions
			var excl []string
			for k, v := range exclusions {
				if v {
					excl = append(excl, k)
				}
			}
			exclStr := strings.Join(excl, ",")
			if exclStr == "" {
				exclStr = "ru"
			}
			rule := fmt.Sprintf("%s:%s/32::any:%s", c.Route, c.IP, exclStr)
			tunDirRules = append(tunDirRules, rule)
		}
	}

	// Exclusions for Xray
	var xrayExclusions []string
	for k, v := range exclusions {
		if v {
			xrayExclusions = append(xrayExclusions, k)
		}
	}
	if len(xrayExclusions) == 0 {
		xrayExclusions = []string{"ru"}
	}

	// Server IPs (unique and sorted, like jq '[.[].ip] | unique')
	seen := make(map[string]bool)
	var serverIPs []string
	for _, s := range servers {
		if !seen[s.IP] {
			seen[s.IP] = true
			serverIPs = append(serverIPs, s.IP)
		}
	}
	sort.Strings(serverIPs)

	vpnCfg.Xray.Clients = xrayClients
	vpnCfg.Xray.ExcludeSets = xrayExclusions
	vpnCfg.Xray.Servers = serverIPs
	vpnCfg.TunnelDirector.Rules = tunDirRules

	if err := vpnconfig.SaveVPNDirectorConfig(scriptsDir+"/vpn-director.json", vpnCfg); err != nil {
		b.sendMessage(chatID, fmt.Sprintf("Save error: %v", err))
		return
	}
	b.sendMessage(chatID, "vpn-director.json updated")

	// Generate Xray config
	if serverIndex < len(servers) {
		s := servers[serverIndex]
		if err := b.generateXrayConfig(s); err != nil {
			b.sendMessage(chatID, fmt.Sprintf("Xray config generation error: %v", err))
		} else {
			b.sendMessage(chatID, "xray/config.json updated")
		}
	}

	// Apply configuration via vpn-director
	result, err := shell.Exec(scriptsDir+"/vpn-director.sh", "apply")
	if err != nil {
		b.sendMessage(chatID, fmt.Sprintf("vpn-director.sh apply exec error: %v", err))
		return
	}
	if result.ExitCode != 0 {
		b.sendMessage(chatID, fmt.Sprintf("vpn-director apply error (exit %d): %s", result.ExitCode, result.Output))
		return
	}
	b.sendMessage(chatID, "VPN Director applied")

	// Restart Xray to apply new config
	result, err = shell.Exec(scriptsDir+"/vpn-director.sh", "restart", "xray")
	if err != nil {
		b.sendMessage(chatID, fmt.Sprintf("vpn-director.sh restart exec error: %v", err))
		return
	}
	if result.ExitCode != 0 {
		b.sendMessage(chatID, fmt.Sprintf("vpn-director restart error (exit %d): %s", result.ExitCode, result.Output))
		return
	}
	b.sendMessage(chatID, "Xray restarted")

	b.sendMessage(chatID, "Done!")
	b.wizard.Clear(chatID)
}

func (b *Bot) generateXrayConfig(server vpnconfig.Server) error {
	templatePath := "/opt/etc/xray/config.json.template"
	outputPath := "/opt/etc/xray/config.json"

	template, err := os.ReadFile(templatePath)
	if err != nil {
		return err
	}

	config := string(template)
	config = strings.ReplaceAll(config, "{{XRAY_SERVER_ADDRESS}}", server.Address)
	config = strings.ReplaceAll(config, "{{XRAY_SERVER_PORT}}", fmt.Sprintf("%d", server.Port))
	config = strings.ReplaceAll(config, "{{XRAY_USER_UUID}}", server.UUID)

	return os.WriteFile(outputPath, []byte(config), 0644)
}

// isValidLANIP checks if the IP is in private range
func isValidLANIP(ip string) bool {
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return false
	}
	// Validate each part is a number 0-255
	for _, p := range parts {
		var n int
		if _, err := fmt.Sscanf(p, "%d", &n); err != nil || n < 0 || n > 255 {
			return false
		}
	}
	// Check for private IP ranges
	if strings.HasPrefix(ip, "192.168.") || strings.HasPrefix(ip, "10.") {
		return true
	}
	if strings.HasPrefix(ip, "172.") {
		// 172.16-31.x.x
		second := 0
		fmt.Sscanf(parts[1], "%d", &second)
		return second >= 16 && second <= 31
	}
	return false
}

// getServerGridColumns returns number of columns based on server count
func getServerGridColumns(count int) int {
	if count <= 10 {
		return 1
	}
	return 2
}

// formatExclusionButton formats exclusion button text with emoji and country name
func formatExclusionButton(code string, selected bool) string {
	mark := "🔲"
	if selected {
		mark = "✅"
	}
	name := code
	if n, ok := wizard.CountryNames[code]; ok {
		name = n
	}
	return fmt.Sprintf("%s %s %s", mark, code, name)
}
