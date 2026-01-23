package wizard

import (
	"fmt"
	"sort"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/telegram"
)

// ExclusionsStep handles Step 2: exclusions selection
type ExclusionsStep struct {
	deps *StepDeps
	next func(chatID int64, state *State) // callback to render next step
}

// NewExclusionsStep creates a new ExclusionsStep handler
func NewExclusionsStep(deps *StepDeps, next func(chatID int64, state *State)) *ExclusionsStep {
	return &ExclusionsStep{
		deps: deps,
		next: next,
	}
}

// Render displays the exclusions selection UI
func (s *ExclusionsStep) Render(chatID int64, state *State) {
	text, keyboard := s.buildUI(state)
	s.deps.Sender.SendWithKeyboard(chatID, text, keyboard)
}

// HandleCallback processes callback button presses for exclusions selection
func (s *ExclusionsStep) HandleCallback(cb *tgbotapi.CallbackQuery, state *State) {
	data := cb.Data

	if !strings.HasPrefix(data, "excl:") {
		return
	}

	ex := strings.TrimPrefix(data, "excl:")

	if ex == "done" {
		state.SetStep(StepClients)
		if s.next != nil {
			s.next(cb.Message.Chat.ID, state)
		}
		return
	}

	// Toggle the exclusion
	state.ToggleExclusion(ex)

	// Update the message with new state
	text, keyboard := s.buildUI(state)
	s.deps.Sender.EditMessage(cb.Message.Chat.ID, cb.Message.MessageID, text, keyboard)
}

// HandleMessage processes text input - exclusions step doesn't handle text input
func (s *ExclusionsStep) HandleMessage(msg *tgbotapi.Message, state *State) bool {
	return false
}

// buildUI constructs the text and keyboard for the exclusions selection
func (s *ExclusionsStep) buildUI(state *State) (string, tgbotapi.InlineKeyboardMarkup) {
	stateExclusions := state.GetExclusions()

	// Build exclusion buttons
	kb := telegram.NewKeyboard()
	for _, ex := range DefaultExclusions {
		btnText := formatExclusionButton(ex, stateExclusions[ex])
		kb.Button(btnText, "excl:"+ex)
	}
	kb.Columns(2)

	// Add Done and Cancel row
	kb.Button("Done", "excl:done").Button("Cancel", "cancel").Row()

	// Build selected list for display
	var selected []string
	for k, v := range stateExclusions {
		if v {
			selected = append(selected, k)
		}
	}
	sort.Strings(selected)

	text := telegram.EscapeMarkdownV2("Step 2/4: Exclude from proxy") + "\n"
	if len(selected) > 0 {
		text += telegram.EscapeMarkdownV2(fmt.Sprintf("Selected: %s", strings.Join(selected, ", ")))
	} else {
		text += telegram.EscapeMarkdownV2("Selected: (none)")
	}

	return text, kb.Build()
}

// formatExclusionButton formats a button label for an exclusion option
func formatExclusionButton(code string, selected bool) string {
	mark := "\U0001F532" // white square
	if selected {
		mark = "\u2705" // checkmark
	}
	name := code
	if n, ok := CountryNames[code]; ok {
		name = n
	}
	return fmt.Sprintf("%s %s %s", mark, code, name)
}
