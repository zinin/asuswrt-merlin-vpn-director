package handler

import (
	"fmt"
	"strings"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/telegram"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/vpnconfig"
)

type ClientsHandler struct {
	deps     *Deps
	mu       sync.Mutex
	addState map[int64]string
}

func NewClientsHandler(deps *Deps) *ClientsHandler {
	return &ClientsHandler{
		deps:     deps,
		addState: make(map[int64]string),
	}
}

func (h *ClientsHandler) ClearState(chatID int64) {
	h.mu.Lock()
	delete(h.addState, chatID)
	h.mu.Unlock()
}

func (h *ClientsHandler) HandleClients(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	h.ClearState(chatID)

	cfg, err := h.deps.Config.LoadVPNConfig()
	if err != nil {
		h.deps.Sender.Send(chatID, telegram.EscapeMarkdownV2(fmt.Sprintf("Config load error: %v", err)))
		return
	}

	text, kb := h.buildClientList(cfg)
	h.deps.Sender.SendWithKeyboard(chatID, text, kb)
}

func (h *ClientsHandler) buildClientList(cfg *vpnconfig.VPNDirectorConfig) (string, tgbotapi.InlineKeyboardMarkup) {
	clients := vpnconfig.CollectClients(cfg)
	kb := telegram.NewKeyboard()

	var sb strings.Builder
	if len(clients) == 0 {
		sb.WriteString(telegram.EscapeMarkdownV2("No clients configured."))
	} else {
		sb.WriteString(telegram.EscapeMarkdownV2("Clients:") + "\n\n")
		for _, c := range clients {
			status := "\u25b6"
			if c.Paused {
				status = "\u23f8"
			}
			// Escape non-IP parts; IPs and route names are safe plain text
			sb.WriteString(telegram.EscapeMarkdownV2(status+"  ") + c.IP + telegram.EscapeMarkdownV2(" \u2192 ") + c.Route + "\n")

			if c.Paused {
				kb.Button(fmt.Sprintf("\u25b6 %s", c.IP), fmt.Sprintf("clients:resume:%s", c.IP))
			} else {
				kb.Button(fmt.Sprintf("\u23f8 %s", c.IP), fmt.Sprintf("clients:pause:%s", c.IP))
			}
			kb.Button(fmt.Sprintf("\U0001f5d1 %s", c.IP), fmt.Sprintf("clients:remove:%s", c.IP))
			kb.Row()
		}
	}

	kb.Button("\u2795 Add client", "clients:add").Row()

	return sb.String(), kb.Build()
}

// HandleCallback handles all clients: callback queries.
func (h *ClientsHandler) HandleCallback(cb *tgbotapi.CallbackQuery) {
	data := cb.Data
	if !strings.HasPrefix(data, "clients:") {
		return
	}

	chatID := cb.Message.Chat.ID
	msgID := cb.Message.MessageID
	action := strings.TrimPrefix(data, "clients:")

	switch {
	case strings.HasPrefix(action, "pause:"):
		ip := strings.TrimPrefix(action, "pause:")
		h.handlePauseResume(chatID, msgID, ip, true)
	case strings.HasPrefix(action, "resume:"):
		ip := strings.TrimPrefix(action, "resume:")
		h.handlePauseResume(chatID, msgID, ip, false)
	case strings.HasPrefix(action, "remove:"):
		ip := strings.TrimPrefix(action, "remove:")
		h.handleRemoveConfirm(chatID, msgID, ip)
	case strings.HasPrefix(action, "rm_yes:"):
		ip := strings.TrimPrefix(action, "rm_yes:")
		h.handleRemove(chatID, msgID, ip)
	case action == "rm_no":
		h.handleRefreshList(chatID, msgID)
	case action == "add":
		h.handleAddStart(chatID)
	case strings.HasPrefix(action, "route:"):
		route := strings.TrimPrefix(action, "route:")
		h.handleAddRoute(chatID, msgID, route)
	}
}

func (h *ClientsHandler) handlePauseResume(chatID int64, msgID int, ip string, pause bool) {
	cfg, err := h.deps.Config.LoadVPNConfig()
	if err != nil {
		h.deps.Sender.SendPlain(chatID, fmt.Sprintf("Config load error: %v", err))
		return
	}

	// Verify IP still exists in config (stale keyboard protection)
	clients := vpnconfig.CollectClients(cfg)
	exists := false
	for _, c := range clients {
		if c.IP == ip {
			exists = true
			break
		}
	}
	if !exists {
		text, kb := h.buildClientList(cfg)
		h.deps.Sender.EditMessage(chatID, msgID, text, kb)
		return
	}

	if pause {
		found := false
		for _, p := range cfg.PausedClients {
			if p == ip {
				found = true
				break
			}
		}
		if !found {
			cfg.PausedClients = append(cfg.PausedClients, ip)
		}
	} else {
		filtered := cfg.PausedClients[:0]
		for _, p := range cfg.PausedClients {
			if p != ip {
				filtered = append(filtered, p)
			}
		}
		cfg.PausedClients = filtered
	}

	if err := h.deps.Config.SaveVPNConfig(cfg); err != nil {
		h.deps.Sender.SendPlain(chatID, fmt.Sprintf("Save error: %v", err))
		return
	}

	if err := h.deps.VPN.Apply(); err != nil {
		h.deps.Sender.SendPlain(chatID, fmt.Sprintf("Apply error: %v", err))
		return
	}

	text, kb := h.buildClientList(cfg)
	h.deps.Sender.EditMessage(chatID, msgID, text, kb)
}

func (h *ClientsHandler) handleRefreshList(chatID int64, msgID int) {
	cfg, err := h.deps.Config.LoadVPNConfig()
	if err != nil {
		h.deps.Sender.SendPlain(chatID, fmt.Sprintf("Config load error: %v", err))
		return
	}
	text, kb := h.buildClientList(cfg)
	h.deps.Sender.EditMessage(chatID, msgID, text, kb)
}

func (h *ClientsHandler) handleRemoveConfirm(chatID int64, msgID int, ip string) {
	cfg, err := h.deps.Config.LoadVPNConfig()
	if err != nil {
		h.deps.Sender.SendPlain(chatID, fmt.Sprintf("Config load error: %v", err))
		return
	}

	clients := vpnconfig.CollectClients(cfg)
	route := ""
	for _, c := range clients {
		if c.IP == ip {
			route = c.Route
			break
		}
	}
	if route == "" {
		h.handleRefreshList(chatID, msgID)
		return
	}

	text := telegram.EscapeMarkdownV2(fmt.Sprintf("Remove %s from %s?", ip, route))
	kb := telegram.NewKeyboard()
	kb.Button("Yes, remove", fmt.Sprintf("clients:rm_yes:%s", ip))
	kb.Button("Cancel", "clients:rm_no")
	kb.Row()

	h.deps.Sender.EditMessage(chatID, msgID, text, kb.Build())
}

func (h *ClientsHandler) handleRemove(chatID int64, msgID int, ip string) {
	cfg, err := h.deps.Config.LoadVPNConfig()
	if err != nil {
		h.deps.Sender.SendPlain(chatID, fmt.Sprintf("Config load error: %v", err))
		return
	}

	cfg.Xray.Clients = removeString(cfg.Xray.Clients, ip)

	for name, tunnel := range cfg.TunnelDirector.Tunnels {
		tunnel.Clients = removeString(tunnel.Clients, ip)
		cfg.TunnelDirector.Tunnels[name] = tunnel
	}

	cfg.PausedClients = removeString(cfg.PausedClients, ip)

	if err := h.deps.Config.SaveVPNConfig(cfg); err != nil {
		h.deps.Sender.SendPlain(chatID, fmt.Sprintf("Save error: %v", err))
		return
	}

	if err := h.deps.VPN.Apply(); err != nil {
		h.deps.Sender.SendPlain(chatID, fmt.Sprintf("Apply error: %v", err))
		return
	}

	text, kb := h.buildClientList(cfg)
	h.deps.Sender.EditMessage(chatID, msgID, text, kb)
}

func removeString(slice []string, s string) []string {
	result := slice[:0]
	for _, v := range slice {
		if v != s {
			result = append(result, v)
		}
	}
	return result
}
func (h *ClientsHandler) handleAddStart(chatID int64)                            {}
func (h *ClientsHandler) handleAddRoute(chatID int64, msgID int, route string)   {}

// HandleTextInput handles text messages for the add-client flow.
func (h *ClientsHandler) HandleTextInput(msg *tgbotapi.Message) {}
