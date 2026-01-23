package wizard

import (
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/telegram"
)

// ClientsStep handles Step 3: client management (including IP input and route selection)
// This step handles StepClients, StepClientIP, and StepClientRoute as they form one logical flow.
type ClientsStep struct {
	deps *StepDeps
	next func(chatID int64, state *State) // callback to render next step (confirm)
}

// NewClientsStep creates a new ClientsStep handler
func NewClientsStep(deps *StepDeps, next func(chatID int64, state *State)) *ClientsStep {
	return &ClientsStep{
		deps: deps,
		next: next,
	}
}

// Render displays the clients list with management buttons
func (s *ClientsStep) Render(chatID int64, state *State) {
	clients := state.GetClients()

	var sb strings.Builder
	sb.WriteString(telegram.EscapeMarkdownV2("Step 3/4: Clients") + "\n\n")

	if len(clients) == 0 {
		sb.WriteString(telegram.EscapeMarkdownV2("(none yet)") + "\n")
	} else {
		for _, c := range clients {
			sb.WriteString(telegram.EscapeMarkdownV2(fmt.Sprintf("* %s -> %s", c.IP, c.Route)) + "\n")
		}
	}

	kb := telegram.NewKeyboard()
	kb.Button("Add", "client:add").Button("Remove", "client:del").Row()
	kb.Button("Done", "client:done").Button("Cancel", "cancel").Row()

	s.deps.Sender.SendWithKeyboard(chatID, sb.String(), kb.Build())
}

// HandleCallback processes callback button presses for client management
func (s *ClientsStep) HandleCallback(cb *tgbotapi.CallbackQuery, state *State) {
	data := cb.Data
	chatID := cb.Message.Chat.ID

	// Handle client: prefix callbacks
	if strings.HasPrefix(data, "client:") {
		action := strings.TrimPrefix(data, "client:")
		switch action {
		case "add":
			state.SetStep(StepClientIP)
			s.sendIPPrompt(chatID)
		case "del":
			state.RemoveLastClient()
			s.Render(chatID, state)
		case "done":
			state.SetStep(StepConfirm)
			if s.next != nil {
				s.next(chatID, state)
			}
		}
		return
	}

	// Handle route: prefix callbacks
	if strings.HasPrefix(data, "route:") {
		route := strings.TrimPrefix(data, "route:")
		state.AddClient(ClientRoute{
			IP:    state.GetPendingIP(),
			Route: route,
		})
		state.SetPendingIP("")
		state.SetStep(StepClients)
		s.Render(chatID, state)
		return
	}
}

// HandleMessage processes text input for IP addresses
// Returns true if the message was handled, false otherwise
func (s *ClientsStep) HandleMessage(msg *tgbotapi.Message, state *State) bool {
	// Only handle text input when expecting an IP address
	if state.GetStep() != StepClientIP {
		return false
	}

	ip := strings.TrimSpace(msg.Text)
	if !isValidLANIP(ip) {
		s.deps.Sender.Send(msg.Chat.ID, telegram.EscapeMarkdownV2("Invalid IP. Enter IP from range 192.168.x.x, 10.x.x.x or 172.16-31.x.x"))
		return true
	}

	state.SetPendingIP(ip)
	state.SetStep(StepClientRoute)
	s.sendRouteSelection(msg.Chat.ID, ip)
	return true
}

// sendIPPrompt sends a message prompting for IP address input
func (s *ClientsStep) sendIPPrompt(chatID int64) {
	s.deps.Sender.Send(chatID, telegram.EscapeMarkdownV2("Enter client IP address\n(e.g.: 192.168.1.100)"))
}

// sendRouteSelection sends a keyboard with route options for the given IP
func (s *ClientsStep) sendRouteSelection(chatID int64, ip string) {
	kb := telegram.NewKeyboard()

	// Xray option
	kb.Button("Xray", "route:xray").Row()

	// OpenVPN options (5 buttons in one row)
	for i := 1; i <= 5; i++ {
		kb.Button(fmt.Sprintf("ovpnc%d", i), fmt.Sprintf("route:ovpnc%d", i))
	}
	kb.Row()

	// WireGuard options (5 buttons in one row)
	for i := 1; i <= 5; i++ {
		kb.Button(fmt.Sprintf("wgc%d", i), fmt.Sprintf("route:wgc%d", i))
	}
	kb.Row()

	kb.Button("Cancel", "cancel").Row()

	text := telegram.EscapeMarkdownV2(fmt.Sprintf("Where to route traffic for %s?", ip))
	s.deps.Sender.SendWithKeyboard(chatID, text, kb.Build())
}

// isValidLANIP validates that the IP is a valid LAN (private) IP address
func isValidLANIP(ip string) bool {
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return false
	}

	// Validate each part is a number 0-255
	nums := make([]int, 4)
	for i, p := range parts {
		var n int
		if _, err := fmt.Sscanf(p, "%d", &n); err != nil || n < 0 || n > 255 {
			return false
		}
		nums[i] = n
	}

	// Check for private IP ranges
	// 192.168.0.0/16
	if nums[0] == 192 && nums[1] == 168 {
		return true
	}

	// 10.0.0.0/8
	if nums[0] == 10 {
		return true
	}

	// 172.16.0.0/12 (172.16-31.x.x)
	if nums[0] == 172 && nums[1] >= 16 && nums[1] <= 31 {
		return true
	}

	return false
}
