package wizard

import (
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestIsValidLANIP(t *testing.T) {
	tests := []struct {
		name  string
		ip    string
		valid bool
	}{
		// Valid 192.168.x.x
		{"192.168.1.1", "192.168.1.1", true},
		{"192.168.0.0", "192.168.0.0", true},
		{"192.168.255.255", "192.168.255.255", true},
		{"192.168.50.100", "192.168.50.100", true},

		// Valid 10.x.x.x
		{"10.0.0.1", "10.0.0.1", true},
		{"10.255.255.255", "10.255.255.255", true},
		{"10.10.10.10", "10.10.10.10", true},

		// Valid 172.16-31.x.x
		{"172.16.0.1", "172.16.0.1", true},
		{"172.31.255.255", "172.31.255.255", true},
		{"172.20.5.10", "172.20.5.10", true},

		// Invalid 172.x.x.x (outside 16-31)
		{"172.15.0.1", "172.15.0.1", false},
		{"172.32.0.1", "172.32.0.1", false},
		{"172.0.0.1", "172.0.0.1", false},

		// Invalid public IPs
		{"8.8.8.8", "8.8.8.8", false},
		{"1.1.1.1", "1.1.1.1", false},
		{"203.0.113.1", "203.0.113.1", false},

		// Malformed IPs
		{"not an ip", "not an ip", false},
		{"192.168.1", "192.168.1", false},
		{"192.168.1.1.1", "192.168.1.1.1", false},
		{"192.168.1.256", "192.168.1.256", false},
		{"192.168.1.-1", "192.168.1.-1", false},
		{"", "", false},
		{"...", "...", false},
		{"a.b.c.d", "a.b.c.d", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidLANIP(tt.ip)
			if result != tt.valid {
				t.Errorf("isValidLANIP(%q) = %v, want %v", tt.ip, result, tt.valid)
			}
		})
	}
}

func TestClientsStep_Render(t *testing.T) {
	t.Run("renders empty clients list", func(t *testing.T) {
		sender := &mockSender{}
		deps := &StepDeps{
			Sender: sender,
		}

		nextCalled := false
		step := NewClientsStep(deps, func(chatID int64, state *State) {
			nextCalled = true
		})

		state := &State{
			ChatID:     123,
			Step:       StepClients,
			Exclusions: make(map[string]bool),
			Clients:    []ClientRoute{},
		}

		step.Render(123, state)

		if sender.lastChatID != 123 {
			t.Errorf("expected chatID 123, got %d", sender.lastChatID)
		}

		if sender.lastKeyboard == nil {
			t.Fatal("expected keyboard to be sent")
		}

		// Should have 2 rows: Add/Remove and Done/Cancel
		if len(sender.lastKeyboard.InlineKeyboard) != 2 {
			t.Errorf("expected 2 rows, got %d", len(sender.lastKeyboard.InlineKeyboard))
		}

		// First row should have Add and Remove
		if len(sender.lastKeyboard.InlineKeyboard[0]) != 2 {
			t.Errorf("expected 2 buttons in first row, got %d", len(sender.lastKeyboard.InlineKeyboard[0]))
		}
		if sender.lastKeyboard.InlineKeyboard[0][0].Text != "Add" {
			t.Errorf("expected Add button, got '%s'", sender.lastKeyboard.InlineKeyboard[0][0].Text)
		}
		if sender.lastKeyboard.InlineKeyboard[0][1].Text != "Remove" {
			t.Errorf("expected Remove button, got '%s'", sender.lastKeyboard.InlineKeyboard[0][1].Text)
		}

		// Text should indicate no clients
		if !strings.Contains(sender.lastText, "none") {
			t.Errorf("expected 'none' in text when no clients, got '%s'", sender.lastText)
		}

		// Text should indicate step 3/4
		if !strings.Contains(sender.lastText, "Step 3/4") {
			t.Errorf("expected 'Step 3/4' in text, got '%s'", sender.lastText)
		}

		if nextCalled {
			t.Error("next callback should not be called on render")
		}
	})

	t.Run("renders clients list with entries", func(t *testing.T) {
		sender := &mockSender{}
		deps := &StepDeps{
			Sender: sender,
		}

		step := NewClientsStep(deps, nil)

		state := &State{
			ChatID:     123,
			Step:       StepClients,
			Exclusions: make(map[string]bool),
			Clients: []ClientRoute{
				{IP: "192.168.1.100", Route: "xray"},
				{IP: "192.168.1.101", Route: "wgc1"},
			},
		}

		step.Render(123, state)

		// Text should list clients (IPs contain dots which are escaped in MarkdownV2)
		// 192.168.1.100 becomes 192\.168\.1\.100
		if !strings.Contains(sender.lastText, "192\\.168\\.1\\.100") {
			t.Errorf("expected first client IP in text, got '%s'", sender.lastText)
		}
		if !strings.Contains(sender.lastText, "xray") {
			t.Errorf("expected first client route in text, got '%s'", sender.lastText)
		}
		if !strings.Contains(sender.lastText, "192\\.168\\.1\\.101") {
			t.Errorf("expected second client IP in text, got '%s'", sender.lastText)
		}
		if !strings.Contains(sender.lastText, "wgc1") {
			t.Errorf("expected second client route in text, got '%s'", sender.lastText)
		}
	})
}

func TestClientsStep_HandleCallback_Add(t *testing.T) {
	t.Run("sets step to StepClientIP and sends prompt", func(t *testing.T) {
		sender := &mockSender{}
		deps := &StepDeps{
			Sender: sender,
		}

		step := NewClientsStep(deps, nil)

		state := &State{
			ChatID:     123,
			Step:       StepClients,
			Exclusions: make(map[string]bool),
			Clients:    []ClientRoute{},
		}

		cb := &tgbotapi.CallbackQuery{
			Data: "client:add",
			Message: &tgbotapi.Message{
				Chat: &tgbotapi.Chat{ID: 123},
			},
		}

		step.HandleCallback(cb, state)

		// Should set step to StepClientIP
		if state.GetStep() != StepClientIP {
			t.Errorf("expected step %s, got %s", StepClientIP, state.GetStep())
		}

		// Should send IP prompt message
		if sender.lastChatID != 123 {
			t.Errorf("expected message to chatID 123, got %d", sender.lastChatID)
		}

		// Message should contain IP prompt
		if !strings.Contains(sender.lastText, "IP") {
			t.Errorf("expected IP prompt in text, got '%s'", sender.lastText)
		}
	})
}

func TestClientsStep_HandleCallback_Del(t *testing.T) {
	t.Run("removes last client and re-renders", func(t *testing.T) {
		sender := &mockSender{}
		deps := &StepDeps{
			Sender: sender,
		}

		step := NewClientsStep(deps, nil)

		state := &State{
			ChatID:     123,
			Step:       StepClients,
			Exclusions: make(map[string]bool),
			Clients: []ClientRoute{
				{IP: "192.168.1.100", Route: "xray"},
				{IP: "192.168.1.101", Route: "wgc1"},
			},
		}

		cb := &tgbotapi.CallbackQuery{
			Data: "client:del",
			Message: &tgbotapi.Message{
				Chat: &tgbotapi.Chat{ID: 123},
			},
		}

		step.HandleCallback(cb, state)

		// Should remove last client
		clients := state.GetClients()
		if len(clients) != 1 {
			t.Errorf("expected 1 client, got %d", len(clients))
		}

		if clients[0].IP != "192.168.1.100" {
			t.Errorf("expected first client IP 192.168.1.100, got %s", clients[0].IP)
		}

		// Should re-render (send message with keyboard)
		if sender.lastKeyboard == nil {
			t.Fatal("expected keyboard to be sent on re-render")
		}
	})

	t.Run("handles empty clients list gracefully", func(t *testing.T) {
		sender := &mockSender{}
		deps := &StepDeps{
			Sender: sender,
		}

		step := NewClientsStep(deps, nil)

		state := &State{
			ChatID:     123,
			Step:       StepClients,
			Exclusions: make(map[string]bool),
			Clients:    []ClientRoute{},
		}

		cb := &tgbotapi.CallbackQuery{
			Data: "client:del",
			Message: &tgbotapi.Message{
				Chat: &tgbotapi.Chat{ID: 123},
			},
		}

		// Should not panic
		step.HandleCallback(cb, state)

		// Should still render
		if sender.lastKeyboard == nil {
			t.Fatal("expected keyboard to be sent")
		}
	})
}

func TestClientsStep_HandleCallback_Done(t *testing.T) {
	t.Run("advances to StepConfirm and calls next", func(t *testing.T) {
		sender := &mockSender{}
		deps := &StepDeps{
			Sender: sender,
		}

		var nextChatID int64
		var nextState *State
		step := NewClientsStep(deps, func(chatID int64, state *State) {
			nextChatID = chatID
			nextState = state
		})

		state := &State{
			ChatID:     123,
			Step:       StepClients,
			Exclusions: make(map[string]bool),
			Clients: []ClientRoute{
				{IP: "192.168.1.100", Route: "xray"},
			},
		}

		cb := &tgbotapi.CallbackQuery{
			Data: "client:done",
			Message: &tgbotapi.Message{
				Chat: &tgbotapi.Chat{ID: 123},
			},
		}

		step.HandleCallback(cb, state)

		// Should set step to StepConfirm
		if state.GetStep() != StepConfirm {
			t.Errorf("expected step %s, got %s", StepConfirm, state.GetStep())
		}

		// Should call next callback
		if nextChatID != 123 {
			t.Errorf("expected next callback with chatID 123, got %d", nextChatID)
		}

		if nextState != state {
			t.Error("expected next callback with same state")
		}
	})
}

func TestClientsStep_HandleCallback_Route(t *testing.T) {
	t.Run("adds client with route and returns to StepClients", func(t *testing.T) {
		sender := &mockSender{}
		deps := &StepDeps{
			Sender: sender,
		}

		step := NewClientsStep(deps, nil)

		state := &State{
			ChatID:     123,
			Step:       StepClientRoute,
			Exclusions: make(map[string]bool),
			Clients:    []ClientRoute{},
			PendingIP:  "192.168.1.100",
		}

		cb := &tgbotapi.CallbackQuery{
			Data: "route:xray",
			Message: &tgbotapi.Message{
				Chat: &tgbotapi.Chat{ID: 123},
			},
		}

		step.HandleCallback(cb, state)

		// Should add client
		clients := state.GetClients()
		if len(clients) != 1 {
			t.Errorf("expected 1 client, got %d", len(clients))
		}

		if clients[0].IP != "192.168.1.100" {
			t.Errorf("expected client IP 192.168.1.100, got %s", clients[0].IP)
		}

		if clients[0].Route != "xray" {
			t.Errorf("expected client route xray, got %s", clients[0].Route)
		}

		// Should clear pending IP
		if state.GetPendingIP() != "" {
			t.Errorf("expected pending IP to be cleared, got %s", state.GetPendingIP())
		}

		// Should return to StepClients
		if state.GetStep() != StepClients {
			t.Errorf("expected step %s, got %s", StepClients, state.GetStep())
		}

		// Should re-render clients list
		if sender.lastKeyboard == nil {
			t.Fatal("expected keyboard to be sent on re-render")
		}
	})

	t.Run("handles ovpn routes", func(t *testing.T) {
		sender := &mockSender{}
		deps := &StepDeps{
			Sender: sender,
		}

		step := NewClientsStep(deps, nil)

		state := &State{
			ChatID:     123,
			Step:       StepClientRoute,
			Exclusions: make(map[string]bool),
			Clients:    []ClientRoute{},
			PendingIP:  "10.0.0.5",
		}

		cb := &tgbotapi.CallbackQuery{
			Data: "route:ovpnc3",
			Message: &tgbotapi.Message{
				Chat: &tgbotapi.Chat{ID: 123},
			},
		}

		step.HandleCallback(cb, state)

		clients := state.GetClients()
		if len(clients) != 1 {
			t.Fatalf("expected 1 client, got %d", len(clients))
		}

		if clients[0].Route != "ovpnc3" {
			t.Errorf("expected client route ovpnc3, got %s", clients[0].Route)
		}
	})

	t.Run("handles wg routes", func(t *testing.T) {
		sender := &mockSender{}
		deps := &StepDeps{
			Sender: sender,
		}

		step := NewClientsStep(deps, nil)

		state := &State{
			ChatID:     123,
			Step:       StepClientRoute,
			Exclusions: make(map[string]bool),
			Clients:    []ClientRoute{},
			PendingIP:  "172.16.0.10",
		}

		cb := &tgbotapi.CallbackQuery{
			Data: "route:wgc5",
			Message: &tgbotapi.Message{
				Chat: &tgbotapi.Chat{ID: 123},
			},
		}

		step.HandleCallback(cb, state)

		clients := state.GetClients()
		if len(clients) != 1 {
			t.Fatalf("expected 1 client, got %d", len(clients))
		}

		if clients[0].Route != "wgc5" {
			t.Errorf("expected client route wgc5, got %s", clients[0].Route)
		}
	})
}

func TestClientsStep_HandleCallback_IgnoresUnrelated(t *testing.T) {
	t.Run("ignores non-client and non-route callbacks", func(t *testing.T) {
		sender := &mockSender{}
		deps := &StepDeps{
			Sender: sender,
		}

		step := NewClientsStep(deps, nil)

		state := &State{
			ChatID:     123,
			Step:       StepClients,
			Exclusions: make(map[string]bool),
			Clients:    []ClientRoute{},
		}

		cb := &tgbotapi.CallbackQuery{
			Data: "other:data",
			Message: &tgbotapi.Message{
				Chat: &tgbotapi.Chat{ID: 123},
			},
		}

		step.HandleCallback(cb, state)

		// Should not change step
		if state.GetStep() != StepClients {
			t.Errorf("expected step to remain %s, got %s", StepClients, state.GetStep())
		}
	})
}

func TestClientsStep_HandleMessage_ValidIP(t *testing.T) {
	t.Run("accepts valid IP and moves to StepClientRoute", func(t *testing.T) {
		sender := &mockSender{}
		deps := &StepDeps{
			Sender: sender,
		}

		step := NewClientsStep(deps, nil)

		state := &State{
			ChatID:     123,
			Step:       StepClientIP,
			Exclusions: make(map[string]bool),
			Clients:    []ClientRoute{},
		}

		msg := &tgbotapi.Message{
			Text: "192.168.1.100",
			Chat: &tgbotapi.Chat{ID: 123},
		}

		handled := step.HandleMessage(msg, state)

		if !handled {
			t.Error("expected HandleMessage to return true")
		}

		// Should set pending IP
		if state.GetPendingIP() != "192.168.1.100" {
			t.Errorf("expected pending IP 192.168.1.100, got %s", state.GetPendingIP())
		}

		// Should advance to StepClientRoute
		if state.GetStep() != StepClientRoute {
			t.Errorf("expected step %s, got %s", StepClientRoute, state.GetStep())
		}

		// Should send route selection keyboard
		if sender.lastKeyboard == nil {
			t.Fatal("expected keyboard to be sent")
		}

		// Keyboard should have route options
		// Row 1: Xray, Row 2: ovpnc1-5, Row 3: wgc1-5, Row 4: Cancel
		if len(sender.lastKeyboard.InlineKeyboard) != 4 {
			t.Errorf("expected 4 rows in route selection, got %d", len(sender.lastKeyboard.InlineKeyboard))
		}
	})

	t.Run("trims whitespace from IP", func(t *testing.T) {
		sender := &mockSender{}
		deps := &StepDeps{
			Sender: sender,
		}

		step := NewClientsStep(deps, nil)

		state := &State{
			ChatID:     123,
			Step:       StepClientIP,
			Exclusions: make(map[string]bool),
			Clients:    []ClientRoute{},
		}

		msg := &tgbotapi.Message{
			Text: "  10.0.0.1  ",
			Chat: &tgbotapi.Chat{ID: 123},
		}

		handled := step.HandleMessage(msg, state)

		if !handled {
			t.Error("expected HandleMessage to return true")
		}

		if state.GetPendingIP() != "10.0.0.1" {
			t.Errorf("expected trimmed IP 10.0.0.1, got %s", state.GetPendingIP())
		}
	})
}

func TestClientsStep_HandleMessage_InvalidIP(t *testing.T) {
	t.Run("rejects invalid IP with error message", func(t *testing.T) {
		sender := &mockSender{}
		deps := &StepDeps{
			Sender: sender,
		}

		step := NewClientsStep(deps, nil)

		state := &State{
			ChatID:     123,
			Step:       StepClientIP,
			Exclusions: make(map[string]bool),
			Clients:    []ClientRoute{},
		}

		msg := &tgbotapi.Message{
			Text: "8.8.8.8",
			Chat: &tgbotapi.Chat{ID: 123},
		}

		handled := step.HandleMessage(msg, state)

		if !handled {
			t.Error("expected HandleMessage to return true for invalid IP")
		}

		// Should NOT advance step
		if state.GetStep() != StepClientIP {
			t.Errorf("expected step to remain %s, got %s", StepClientIP, state.GetStep())
		}

		// Should NOT set pending IP
		if state.GetPendingIP() != "" {
			t.Errorf("expected pending IP to be empty, got %s", state.GetPendingIP())
		}

		// Should send error message
		if !strings.Contains(sender.lastText, "Invalid") {
			t.Errorf("expected error message about invalid IP, got '%s'", sender.lastText)
		}
	})

	t.Run("rejects malformed IP", func(t *testing.T) {
		sender := &mockSender{}
		deps := &StepDeps{
			Sender: sender,
		}

		step := NewClientsStep(deps, nil)

		state := &State{
			ChatID:     123,
			Step:       StepClientIP,
			Exclusions: make(map[string]bool),
			Clients:    []ClientRoute{},
		}

		msg := &tgbotapi.Message{
			Text: "not-an-ip",
			Chat: &tgbotapi.Chat{ID: 123},
		}

		handled := step.HandleMessage(msg, state)

		if !handled {
			t.Error("expected HandleMessage to return true for malformed IP")
		}

		if state.GetStep() != StepClientIP {
			t.Errorf("expected step to remain %s, got %s", StepClientIP, state.GetStep())
		}
	})
}

func TestClientsStep_HandleMessage_WrongStep(t *testing.T) {
	t.Run("returns false if not StepClientIP", func(t *testing.T) {
		sender := &mockSender{}
		deps := &StepDeps{
			Sender: sender,
		}

		step := NewClientsStep(deps, nil)

		state := &State{
			ChatID:     123,
			Step:       StepClients, // Not StepClientIP
			Exclusions: make(map[string]bool),
			Clients:    []ClientRoute{},
		}

		msg := &tgbotapi.Message{
			Text: "192.168.1.100",
			Chat: &tgbotapi.Chat{ID: 123},
		}

		handled := step.HandleMessage(msg, state)

		if handled {
			t.Error("expected HandleMessage to return false for wrong step")
		}
	})

	t.Run("returns false for StepClientRoute", func(t *testing.T) {
		sender := &mockSender{}
		deps := &StepDeps{
			Sender: sender,
		}

		step := NewClientsStep(deps, nil)

		state := &State{
			ChatID:     123,
			Step:       StepClientRoute, // Text input not expected here
			Exclusions: make(map[string]bool),
			Clients:    []ClientRoute{},
		}

		msg := &tgbotapi.Message{
			Text: "192.168.1.100",
			Chat: &tgbotapi.Chat{ID: 123},
		}

		handled := step.HandleMessage(msg, state)

		if handled {
			t.Error("expected HandleMessage to return false for StepClientRoute")
		}
	})
}

func TestClientsStep_RouteSelectionKeyboard(t *testing.T) {
	t.Run("route selection shows all options", func(t *testing.T) {
		sender := &mockSender{}
		deps := &StepDeps{
			Sender: sender,
		}

		step := NewClientsStep(deps, nil)

		state := &State{
			ChatID:     123,
			Step:       StepClientIP,
			Exclusions: make(map[string]bool),
			Clients:    []ClientRoute{},
		}

		msg := &tgbotapi.Message{
			Text: "192.168.1.100",
			Chat: &tgbotapi.Chat{ID: 123},
		}

		step.HandleMessage(msg, state)

		if sender.lastKeyboard == nil {
			t.Fatal("expected keyboard to be sent")
		}

		kb := sender.lastKeyboard.InlineKeyboard

		// Row 0: Xray
		if kb[0][0].Text != "Xray" {
			t.Errorf("expected Xray button, got '%s'", kb[0][0].Text)
		}

		// Row 1: ovpnc1-5 (5 buttons)
		if len(kb[1]) != 5 {
			t.Errorf("expected 5 ovpn buttons, got %d", len(kb[1]))
		}
		if kb[1][0].Text != "ovpnc1" {
			t.Errorf("expected ovpnc1 button, got '%s'", kb[1][0].Text)
		}
		if kb[1][4].Text != "ovpnc5" {
			t.Errorf("expected ovpnc5 button, got '%s'", kb[1][4].Text)
		}

		// Row 2: wgc1-5 (5 buttons)
		if len(kb[2]) != 5 {
			t.Errorf("expected 5 wg buttons, got %d", len(kb[2]))
		}
		if kb[2][0].Text != "wgc1" {
			t.Errorf("expected wgc1 button, got '%s'", kb[2][0].Text)
		}
		if kb[2][4].Text != "wgc5" {
			t.Errorf("expected wgc5 button, got '%s'", kb[2][4].Text)
		}

		// Row 3: Cancel
		if kb[3][0].Text != "Cancel" {
			t.Errorf("expected Cancel button, got '%s'", kb[3][0].Text)
		}
	})
}
