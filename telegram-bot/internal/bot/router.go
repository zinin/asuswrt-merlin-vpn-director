// internal/bot/router.go
package bot

import (
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// StatusRouterHandler defines methods for status-related commands
type StatusRouterHandler interface {
	HandleStatus(msg *tgbotapi.Message)
	HandleRestart(msg *tgbotapi.Message)
	HandleStop(msg *tgbotapi.Message)
}

// ServersRouterHandler defines methods for server-related commands
type ServersRouterHandler interface {
	HandleServers(msg *tgbotapi.Message)
	HandleCallback(cb *tgbotapi.CallbackQuery)
}

// ImportRouterHandler defines methods for import command
type ImportRouterHandler interface {
	HandleImport(msg *tgbotapi.Message)
}

// MiscRouterHandler defines methods for misc commands
type MiscRouterHandler interface {
	HandleStart(msg *tgbotapi.Message)
	HandleIP(msg *tgbotapi.Message)
	HandleVersion(msg *tgbotapi.Message)
	HandleLogs(msg *tgbotapi.Message)
}

// UpdateRouterHandler defines methods for update command
type UpdateRouterHandler interface {
	HandleUpdate(msg *tgbotapi.Message)
}

// WizardRouterHandler defines methods for wizard
type WizardRouterHandler interface {
	Start(chatID int64)
	HandleCallback(cb *tgbotapi.CallbackQuery)
	HandleTextInput(msg *tgbotapi.Message)
}

// Router routes messages and callbacks to appropriate handlers
type Router struct {
	status  StatusRouterHandler
	servers ServersRouterHandler
	import_ ImportRouterHandler
	misc    MiscRouterHandler
	update  UpdateRouterHandler
	wizard  WizardRouterHandler
}

// NewRouter creates a new Router with all handlers
func NewRouter(
	status StatusRouterHandler,
	servers ServersRouterHandler,
	import_ ImportRouterHandler,
	misc MiscRouterHandler,
	update UpdateRouterHandler,
	wizard WizardRouterHandler,
) *Router {
	return &Router{
		status:  status,
		servers: servers,
		import_: import_,
		misc:    misc,
		update:  update,
		wizard:  wizard,
	}
}

// RouteMessage routes a message to the appropriate handler based on command
func (r *Router) RouteMessage(msg *tgbotapi.Message) {
	switch msg.Command() {
	case "start":
		r.misc.HandleStart(msg)
	case "status":
		r.status.HandleStatus(msg)
	case "restart":
		r.status.HandleRestart(msg)
	case "stop":
		r.status.HandleStop(msg)
	case "servers":
		r.servers.HandleServers(msg)
	case "import":
		r.import_.HandleImport(msg)
	case "logs":
		r.misc.HandleLogs(msg)
	case "ip":
		r.misc.HandleIP(msg)
	case "version":
		r.misc.HandleVersion(msg)
	case "update":
		r.update.HandleUpdate(msg)
	case "configure":
		r.wizard.Start(msg.Chat.ID)
	default:
		// Non-command messages go to wizard text handler
		r.wizard.HandleTextInput(msg)
	}
}

// RouteCallback routes a callback query to the appropriate handler
func (r *Router) RouteCallback(cb *tgbotapi.CallbackQuery) {
	if strings.HasPrefix(cb.Data, "servers:") {
		r.servers.HandleCallback(cb)
		return
	}
	r.wizard.HandleCallback(cb)
}
