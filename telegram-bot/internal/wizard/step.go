// internal/wizard/step.go
package wizard

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/service"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/telegram"
)

// StepHandler defines the interface for wizard step handlers
type StepHandler interface {
	// HandleCallback processes callback button presses
	HandleCallback(cb *tgbotapi.CallbackQuery, state *State)

	// HandleMessage processes text input, returns true if handled
	HandleMessage(msg *tgbotapi.Message, state *State) bool

	// Render displays the step UI
	Render(chatID int64, state *State)
}

// TODO: If we ever split wizard steps into a subpackage, it will create an import cycle
// (wizard -> steps -> wizard.State). Consider extracting State/Manager into a neutral
// package (e.g., wizardstate) or introducing a narrow state interface before doing that.

// StepDeps holds dependencies for step handlers
type StepDeps struct {
	Sender telegram.MessageSender
	Config service.ConfigStore
}
