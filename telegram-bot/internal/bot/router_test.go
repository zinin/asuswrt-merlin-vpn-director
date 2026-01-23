// internal/bot/router_test.go
package bot

import (
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Mock handlers for testing

type mockStatusHandler struct {
	statusCalled  bool
	restartCalled bool
	stopCalled    bool
}

func (m *mockStatusHandler) HandleStatus(msg *tgbotapi.Message)  { m.statusCalled = true }
func (m *mockStatusHandler) HandleRestart(msg *tgbotapi.Message) { m.restartCalled = true }
func (m *mockStatusHandler) HandleStop(msg *tgbotapi.Message)    { m.stopCalled = true }

type mockServersHandler struct {
	serversCalled  bool
	callbackCalled bool
}

func (m *mockServersHandler) HandleServers(msg *tgbotapi.Message)       { m.serversCalled = true }
func (m *mockServersHandler) HandleCallback(cb *tgbotapi.CallbackQuery) { m.callbackCalled = true }

type mockImportHandler struct {
	importCalled bool
}

func (m *mockImportHandler) HandleImport(msg *tgbotapi.Message) { m.importCalled = true }

type mockMiscHandler struct {
	startCalled   bool
	ipCalled      bool
	versionCalled bool
	logsCalled    bool
}

func (m *mockMiscHandler) HandleStart(msg *tgbotapi.Message)   { m.startCalled = true }
func (m *mockMiscHandler) HandleIP(msg *tgbotapi.Message)      { m.ipCalled = true }
func (m *mockMiscHandler) HandleVersion(msg *tgbotapi.Message) { m.versionCalled = true }
func (m *mockMiscHandler) HandleLogs(msg *tgbotapi.Message)    { m.logsCalled = true }

type mockWizardHandler struct {
	startCalled    bool
	startChatID    int64
	callbackCalled bool
	textCalled     bool
}

func (m *mockWizardHandler) Start(chatID int64)                          { m.startCalled = true; m.startChatID = chatID }
func (m *mockWizardHandler) HandleCallback(cb *tgbotapi.CallbackQuery)   { m.callbackCalled = true }
func (m *mockWizardHandler) HandleTextInput(msg *tgbotapi.Message)       { m.textCalled = true }

// Helper to create a message with command entity
func msgWithCommand(text string) *tgbotapi.Message {
	cmdLen := len(text)
	if idx := indexOf(text, ' '); idx > 0 {
		cmdLen = idx
	}
	return &tgbotapi.Message{
		Text:     text,
		Entities: []tgbotapi.MessageEntity{{Type: "bot_command", Offset: 0, Length: cmdLen}},
		Chat:     &tgbotapi.Chat{ID: 123},
	}
}

func indexOf(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

// Tests for RouteMessage

func TestRouter_RouteMessage_Status(t *testing.T) {
	h := &mockStatusHandler{}
	router := &Router{status: h}

	router.RouteMessage(msgWithCommand("/status"))

	if !h.statusCalled {
		t.Error("expected HandleStatus to be called")
	}
}

func TestRouter_RouteMessage_Restart(t *testing.T) {
	h := &mockStatusHandler{}
	router := &Router{status: h}

	router.RouteMessage(msgWithCommand("/restart"))

	if !h.restartCalled {
		t.Error("expected HandleRestart to be called")
	}
}

func TestRouter_RouteMessage_Stop(t *testing.T) {
	h := &mockStatusHandler{}
	router := &Router{status: h}

	router.RouteMessage(msgWithCommand("/stop"))

	if !h.stopCalled {
		t.Error("expected HandleStop to be called")
	}
}

func TestRouter_RouteMessage_Servers(t *testing.T) {
	h := &mockServersHandler{}
	router := &Router{servers: h}

	router.RouteMessage(msgWithCommand("/servers"))

	if !h.serversCalled {
		t.Error("expected HandleServers to be called")
	}
}

func TestRouter_RouteMessage_Import(t *testing.T) {
	h := &mockImportHandler{}
	router := &Router{import_: h}

	router.RouteMessage(msgWithCommand("/import https://example.com"))

	if !h.importCalled {
		t.Error("expected HandleImport to be called")
	}
}

func TestRouter_RouteMessage_Start(t *testing.T) {
	h := &mockMiscHandler{}
	router := &Router{misc: h}

	router.RouteMessage(msgWithCommand("/start"))

	if !h.startCalled {
		t.Error("expected HandleStart to be called")
	}
}

func TestRouter_RouteMessage_IP(t *testing.T) {
	h := &mockMiscHandler{}
	router := &Router{misc: h}

	router.RouteMessage(msgWithCommand("/ip"))

	if !h.ipCalled {
		t.Error("expected HandleIP to be called")
	}
}

func TestRouter_RouteMessage_Version(t *testing.T) {
	h := &mockMiscHandler{}
	router := &Router{misc: h}

	router.RouteMessage(msgWithCommand("/version"))

	if !h.versionCalled {
		t.Error("expected HandleVersion to be called")
	}
}

func TestRouter_RouteMessage_Logs(t *testing.T) {
	h := &mockMiscHandler{}
	router := &Router{misc: h}

	router.RouteMessage(msgWithCommand("/logs bot 50"))

	if !h.logsCalled {
		t.Error("expected HandleLogs to be called")
	}
}

func TestRouter_RouteMessage_Configure(t *testing.T) {
	h := &mockWizardHandler{}
	router := &Router{wizard: h}

	msg := msgWithCommand("/configure")
	router.RouteMessage(msg)

	if !h.startCalled {
		t.Error("expected wizard.Start to be called")
	}
	if h.startChatID != 123 {
		t.Errorf("expected chatID 123, got %d", h.startChatID)
	}
}

func TestRouter_RouteMessage_UnknownCommand_RoutesToWizardText(t *testing.T) {
	h := &mockWizardHandler{}
	router := &Router{wizard: h}

	// Plain text (no command entity) - should go to wizard text handler
	msg := &tgbotapi.Message{
		Text: "192.168.1.100",
		Chat: &tgbotapi.Chat{ID: 123},
	}
	router.RouteMessage(msg)

	if !h.textCalled {
		t.Error("expected wizard.HandleTextInput to be called for plain text")
	}
}

// Tests for RouteCallback

func TestRouter_RouteCallback_Servers(t *testing.T) {
	h := &mockServersHandler{}
	router := &Router{servers: h}

	cb := &tgbotapi.CallbackQuery{
		Data:    "servers:page:1",
		Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}},
	}
	router.RouteCallback(cb)

	if !h.callbackCalled {
		t.Error("expected HandleCallback to be called for servers:page:*")
	}
}

func TestRouter_RouteCallback_ServersNoop(t *testing.T) {
	h := &mockServersHandler{}
	router := &Router{servers: h}

	cb := &tgbotapi.CallbackQuery{
		Data:    "servers:noop",
		Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}},
	}
	router.RouteCallback(cb)

	if !h.callbackCalled {
		t.Error("expected HandleCallback to be called for servers:noop")
	}
}

func TestRouter_RouteCallback_Wizard(t *testing.T) {
	h := &mockWizardHandler{}
	router := &Router{wizard: h}

	cb := &tgbotapi.CallbackQuery{
		Data:    "server:1",
		Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}},
	}
	router.RouteCallback(cb)

	if !h.callbackCalled {
		t.Error("expected wizard.HandleCallback to be called")
	}
}

func TestRouter_RouteCallback_WizardApply(t *testing.T) {
	h := &mockWizardHandler{}
	router := &Router{wizard: h}

	cb := &tgbotapi.CallbackQuery{
		Data:    "apply",
		Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}},
	}
	router.RouteCallback(cb)

	if !h.callbackCalled {
		t.Error("expected wizard.HandleCallback to be called for apply")
	}
}

func TestRouter_RouteCallback_WizardCancel(t *testing.T) {
	h := &mockWizardHandler{}
	router := &Router{wizard: h}

	cb := &tgbotapi.CallbackQuery{
		Data:    "cancel",
		Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}},
	}
	router.RouteCallback(cb)

	if !h.callbackCalled {
		t.Error("expected wizard.HandleCallback to be called for cancel")
	}
}
