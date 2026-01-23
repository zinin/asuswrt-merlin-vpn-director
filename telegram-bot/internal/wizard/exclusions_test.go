package wizard

import (
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestFormatExclusionButton(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		selected bool
		wantMark string
		wantCode string
		wantName string
	}{
		{
			name:     "selected ru",
			code:     "ru",
			selected: true,
			wantMark: "\u2705", // checkmark emoji
			wantCode: "ru",
			wantName: "Russia",
		},
		{
			name:     "unselected ru",
			code:     "ru",
			selected: false,
			wantMark: "\U0001F532", // white square emoji
			wantCode: "ru",
			wantName: "Russia",
		},
		{
			name:     "selected de",
			code:     "de",
			selected: true,
			wantMark: "\u2705",
			wantCode: "de",
			wantName: "Germany",
		},
		{
			name:     "unknown code",
			code:     "xx",
			selected: false,
			wantMark: "\U0001F532",
			wantCode: "xx",
			wantName: "xx", // falls back to code when name unknown
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatExclusionButton(tt.code, tt.selected)

			if !strings.Contains(result, tt.wantMark) {
				t.Errorf("expected mark %q in result %q", tt.wantMark, result)
			}
			if !strings.Contains(result, tt.wantCode) {
				t.Errorf("expected code %q in result %q", tt.wantCode, result)
			}
			if !strings.Contains(result, tt.wantName) {
				t.Errorf("expected name %q in result %q", tt.wantName, result)
			}
		})
	}
}

func TestExclusionsStep_Render(t *testing.T) {
	t.Run("renders exclusion selection with keyboard", func(t *testing.T) {
		sender := &mockSender{}
		deps := &StepDeps{
			Sender: sender,
		}

		nextCalled := false
		step := NewExclusionsStep(deps, func(chatID int64, state *State) {
			nextCalled = true
		})

		state := &State{
			ChatID:     123,
			Step:       StepExclusions,
			Exclusions: make(map[string]bool),
		}

		step.Render(123, state)

		if sender.lastChatID != 123 {
			t.Errorf("expected chatID 123, got %d", sender.lastChatID)
		}

		if sender.lastKeyboard == nil {
			t.Fatal("expected keyboard to be sent")
		}

		// Should have exclusion buttons in rows + Done/Cancel row
		// DefaultExclusions has 10 items, 2 per row = 5 rows + 1 control row = 6 rows
		if len(sender.lastKeyboard.InlineKeyboard) != 6 {
			t.Errorf("expected 6 rows, got %d", len(sender.lastKeyboard.InlineKeyboard))
		}

		// Verify first row has 2 buttons
		if len(sender.lastKeyboard.InlineKeyboard[0]) != 2 {
			t.Errorf("expected 2 buttons in first row, got %d", len(sender.lastKeyboard.InlineKeyboard[0]))
		}

		// Verify Done and Cancel buttons in last row
		lastRow := sender.lastKeyboard.InlineKeyboard[len(sender.lastKeyboard.InlineKeyboard)-1]
		if len(lastRow) != 2 {
			t.Errorf("expected 2 buttons in last row, got %d", len(lastRow))
		}
		if lastRow[0].Text != "Done" {
			t.Errorf("expected Done button, got '%s'", lastRow[0].Text)
		}
		if lastRow[1].Text != "Cancel" {
			t.Errorf("expected Cancel button, got '%s'", lastRow[1].Text)
		}

		// Text should contain step info
		if !strings.Contains(sender.lastText, "Step 2/4") {
			t.Errorf("expected step 2/4 in text, got '%s'", sender.lastText)
		}

		if nextCalled {
			t.Error("next callback should not be called on render")
		}
	})

	t.Run("shows selected exclusions in text", func(t *testing.T) {
		sender := &mockSender{}
		deps := &StepDeps{
			Sender: sender,
		}

		step := NewExclusionsStep(deps, nil)

		state := &State{
			ChatID: 123,
			Step:   StepExclusions,
			Exclusions: map[string]bool{
				"ru": true,
				"ua": true,
			},
		}

		step.Render(123, state)

		// Text should list selected exclusions
		if !strings.Contains(sender.lastText, "ru") || !strings.Contains(sender.lastText, "ua") {
			t.Errorf("expected selected exclusions in text, got '%s'", sender.lastText)
		}
	})

	t.Run("shows none when no exclusions selected", func(t *testing.T) {
		sender := &mockSender{}
		deps := &StepDeps{
			Sender: sender,
		}

		step := NewExclusionsStep(deps, nil)

		state := &State{
			ChatID:     123,
			Step:       StepExclusions,
			Exclusions: make(map[string]bool),
		}

		step.Render(123, state)

		if !strings.Contains(sender.lastText, "none") {
			t.Errorf("expected 'none' in text when no exclusions, got '%s'", sender.lastText)
		}
	})
}

func TestExclusionsStep_HandleCallback_Toggle(t *testing.T) {
	t.Run("toggles exclusion and updates message", func(t *testing.T) {
		sender := &mockSender{}
		deps := &StepDeps{
			Sender: sender,
		}

		step := NewExclusionsStep(deps, nil)

		state := &State{
			ChatID:     123,
			Step:       StepExclusions,
			Exclusions: make(map[string]bool),
		}

		cb := &tgbotapi.CallbackQuery{
			Data: "excl:de",
			Message: &tgbotapi.Message{
				MessageID: 456,
				Chat:      &tgbotapi.Chat{ID: 123},
			},
		}

		step.HandleCallback(cb, state)

		// Should toggle exclusion
		exclusions := state.GetExclusions()
		if !exclusions["de"] {
			t.Error("expected 'de' exclusion to be toggled on")
		}

		// Should call EditMessage (not SendWithKeyboard)
		if sender.lastChatID != 123 {
			t.Errorf("expected EditMessage with chatID 123, got %d", sender.lastChatID)
		}

		if sender.lastKeyboard == nil {
			t.Fatal("expected keyboard in edited message")
		}
	})

	t.Run("toggles off already selected exclusion", func(t *testing.T) {
		sender := &mockSender{}
		deps := &StepDeps{
			Sender: sender,
		}

		step := NewExclusionsStep(deps, nil)

		state := &State{
			ChatID: 123,
			Step:   StepExclusions,
			Exclusions: map[string]bool{
				"ru": true,
			},
		}

		cb := &tgbotapi.CallbackQuery{
			Data: "excl:ru",
			Message: &tgbotapi.Message{
				MessageID: 456,
				Chat:      &tgbotapi.Chat{ID: 123},
			},
		}

		step.HandleCallback(cb, state)

		exclusions := state.GetExclusions()
		if exclusions["ru"] {
			t.Error("expected 'ru' exclusion to be toggled off")
		}
	})
}

func TestExclusionsStep_HandleCallback_Done(t *testing.T) {
	t.Run("advances to clients step on done", func(t *testing.T) {
		sender := &mockSender{}
		deps := &StepDeps{
			Sender: sender,
		}

		var nextChatID int64
		var nextState *State
		step := NewExclusionsStep(deps, func(chatID int64, state *State) {
			nextChatID = chatID
			nextState = state
		})

		state := &State{
			ChatID: 123,
			Step:   StepExclusions,
			Exclusions: map[string]bool{
				"ru": true,
			},
		}

		cb := &tgbotapi.CallbackQuery{
			Data: "excl:done",
			Message: &tgbotapi.Message{
				Chat: &tgbotapi.Chat{ID: 123},
			},
		}

		step.HandleCallback(cb, state)

		// Should advance to clients step
		if state.GetStep() != StepClients {
			t.Errorf("expected step %s, got %s", StepClients, state.GetStep())
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

func TestExclusionsStep_HandleCallback_IgnoresUnrelatedCallbacks(t *testing.T) {
	t.Run("ignores non-excl callbacks", func(t *testing.T) {
		sender := &mockSender{}
		deps := &StepDeps{
			Sender: sender,
		}

		nextCalled := false
		step := NewExclusionsStep(deps, func(chatID int64, state *State) {
			nextCalled = true
		})

		state := &State{
			ChatID:     123,
			Step:       StepExclusions,
			Exclusions: make(map[string]bool),
		}

		cb := &tgbotapi.CallbackQuery{
			Data: "other:data",
			Message: &tgbotapi.Message{
				Chat: &tgbotapi.Chat{ID: 123},
			},
		}

		step.HandleCallback(cb, state)

		// Should not advance step
		if state.GetStep() != StepExclusions {
			t.Errorf("expected step to remain %s, got %s", StepExclusions, state.GetStep())
		}

		// Should not call next
		if nextCalled {
			t.Error("next callback should not be called for non-excl callback")
		}
	})
}

func TestExclusionsStep_HandleMessage(t *testing.T) {
	t.Run("returns false for any text input", func(t *testing.T) {
		sender := &mockSender{}
		deps := &StepDeps{
			Sender: sender,
		}

		step := NewExclusionsStep(deps, nil)

		state := &State{
			ChatID:     123,
			Step:       StepExclusions,
			Exclusions: make(map[string]bool),
		}

		msg := &tgbotapi.Message{
			Text: "some text",
			Chat: &tgbotapi.Chat{ID: 123},
		}

		handled := step.HandleMessage(msg, state)

		if handled {
			t.Error("expected HandleMessage to return false")
		}
	})
}
