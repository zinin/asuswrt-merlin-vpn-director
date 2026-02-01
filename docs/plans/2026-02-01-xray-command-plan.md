# /xray Command Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add `/xray` command to quickly switch Xray server without going through full `/configure` wizard.

**Architecture:** New handler (`XrayHandler`) following existing patterns. Uses `Deps` struct for DI. Callback format `xray:select:{index}`. UI shows all servers in 2-column grid.

**Tech Stack:** Go, telegram-bot-api/v5, existing service interfaces (ConfigStore, XrayGenerator, VPNDirector)

---

## Task 1: Create XrayHandler with HandleXray

**Files:**
- Create: `telegram-bot/internal/handler/xray.go`
- Test: `telegram-bot/internal/handler/xray_test.go`

**Step 1: Write the failing test for HandleXray**

```go
// internal/handler/xray_test.go
package handler

import (
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/vpnconfig"
)

func TestXrayHandler_HandleXray_WithServers(t *testing.T) {
	sender := &mockSenderWithKeyboard{}
	servers := []vpnconfig.Server{
		{Name: "Germany, Berlin", Address: "de.example.com", IP: "1.1.1.1"},
		{Name: "USA, New York", Address: "us.example.com", IP: "2.2.2.2"},
	}
	config := &mockConfigStore{servers: servers}

	deps := &Deps{Sender: sender, Config: config}
	h := NewXrayHandler(deps)

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}}
	h.HandleXray(msg)

	if sender.lastChatID != 123 {
		t.Errorf("expected chatID 123, got %d", sender.lastChatID)
	}
	if !strings.Contains(sender.lastText, "сервер") {
		t.Errorf("expected 'сервер' in text, got %q", sender.lastText)
	}
	// Should have 2 rows: servers in 2 columns
	if len(sender.lastKeyboard.InlineKeyboard) < 1 {
		t.Error("expected keyboard to have rows")
	}
	// First row should have server buttons
	if len(sender.lastKeyboard.InlineKeyboard[0]) == 0 {
		t.Error("expected buttons in first row")
	}
}

func TestXrayHandler_HandleXray_Empty(t *testing.T) {
	sender := &mockSenderWithKeyboard{}
	config := &mockConfigStore{servers: []vpnconfig.Server{}}

	deps := &Deps{Sender: sender, Config: config}
	h := NewXrayHandler(deps)

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 456}}
	h.HandleXray(msg)

	if !strings.Contains(sender.lastText, "не найден") || !strings.Contains(sender.lastText, "/import") {
		t.Errorf("expected 'не найдены' and '/import' in text, got %q", sender.lastText)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd telegram-bot && go test -v ./internal/handler/ -run TestXrayHandler_HandleXray`
Expected: FAIL with "NewXrayHandler not defined"

**Step 3: Write minimal implementation**

```go
// internal/handler/xray.go
package handler

import (
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/telegram"
)

// XrayHandler handles /xray command for quick server switching
type XrayHandler struct {
	deps *Deps
}

// NewXrayHandler creates a new XrayHandler
func NewXrayHandler(deps *Deps) *XrayHandler {
	return &XrayHandler{deps: deps}
}

// HandleXray handles /xray command - shows server selection keyboard
func (h *XrayHandler) HandleXray(msg *tgbotapi.Message) {
	servers, err := h.deps.Config.LoadServers()
	if err != nil {
		h.deps.Sender.Send(msg.Chat.ID, telegram.EscapeMarkdownV2(fmt.Sprintf("Ошибка: %v", err)))
		return
	}

	if len(servers) == 0 {
		h.deps.Sender.Send(msg.Chat.ID, telegram.EscapeMarkdownV2("Серверы не найдены. Используйте /import для импорта"))
		return
	}

	kb := telegram.NewKeyboard()
	for i, srv := range servers {
		btnText := fmt.Sprintf("%d. %s", i+1, srv.Name)
		kb.Button(btnText, fmt.Sprintf("xray:select:%d", i))
	}
	kb.Columns(2)

	text := telegram.EscapeMarkdownV2("Выберите сервер:")
	h.deps.Sender.SendWithKeyboard(msg.Chat.ID, text, kb.Build())
}
```

**Step 4: Run test to verify it passes**

Run: `cd telegram-bot && go test -v ./internal/handler/ -run TestXrayHandler_HandleXray`
Expected: PASS

**Step 5: Commit**

```bash
git add telegram-bot/internal/handler/xray.go telegram-bot/internal/handler/xray_test.go
git commit -m "$(cat <<'EOF'
feat(telegram-bot): add XrayHandler with HandleXray

Shows server selection keyboard for quick switching.
EOF
)"
```

---

## Task 2: Add HandleCallback for server selection

**Files:**
- Modify: `telegram-bot/internal/handler/xray.go`
- Modify: `telegram-bot/internal/handler/xray_test.go`

**Step 1: Write the failing test for HandleCallback**

Add to `xray_test.go`:

```go
// mockXrayGenerator for testing
type mockXrayGenerator struct {
	lastServer vpnconfig.Server
	err        error
}

func (m *mockXrayGenerator) GenerateConfig(server vpnconfig.Server) error {
	m.lastServer = server
	return m.err
}

func TestXrayHandler_HandleCallback_Success(t *testing.T) {
	sender := &mockSenderWithKeyboard{}
	servers := []vpnconfig.Server{
		{Name: "Germany, Berlin", Address: "de.example.com", Port: 443, UUID: "uuid1", IP: "1.1.1.1"},
		{Name: "USA, New York", Address: "us.example.com", Port: 443, UUID: "uuid2", IP: "2.2.2.2"},
	}
	config := &mockConfigStore{servers: servers}
	xray := &mockXrayGenerator{}
	vpn := &mockVPNDirector{}

	deps := &Deps{Sender: sender, Config: config, Xray: xray, VPN: vpn}
	h := NewXrayHandler(deps)

	cb := &tgbotapi.CallbackQuery{
		ID:   "cb123",
		Data: "xray:select:1",
		Message: &tgbotapi.Message{
			MessageID: 42,
			Chat:      &tgbotapi.Chat{ID: 100},
		},
	}
	h.HandleCallback(cb)

	// Should generate config for server index 1 (USA)
	if xray.lastServer.Name != "USA, New York" {
		t.Errorf("expected server 'USA, New York', got %q", xray.lastServer.Name)
	}
	// Should send success message
	if !strings.Contains(sender.lastText, "Переключено") || !strings.Contains(sender.lastText, "USA") {
		t.Errorf("expected success message with server name, got %q", sender.lastText)
	}
}

func TestXrayHandler_HandleCallback_InvalidIndex(t *testing.T) {
	sender := &mockSenderWithKeyboard{}
	servers := []vpnconfig.Server{
		{Name: "Germany, Berlin"},
	}
	config := &mockConfigStore{servers: servers}

	deps := &Deps{Sender: sender, Config: config}
	h := NewXrayHandler(deps)

	cb := &tgbotapi.CallbackQuery{
		ID:   "cb456",
		Data: "xray:select:99", // invalid index
		Message: &tgbotapi.Message{
			MessageID: 10,
			Chat:      &tgbotapi.Chat{ID: 200},
		},
	}
	h.HandleCallback(cb)

	if !strings.Contains(sender.lastText, "Ошибка") {
		t.Errorf("expected error message, got %q", sender.lastText)
	}
}

func TestXrayHandler_HandleCallback_GenerateError(t *testing.T) {
	sender := &mockSenderWithKeyboard{}
	servers := []vpnconfig.Server{{Name: "Server1"}}
	config := &mockConfigStore{servers: servers}
	xray := &mockXrayGenerator{err: errors.New("template not found")}

	deps := &Deps{Sender: sender, Config: config, Xray: xray}
	h := NewXrayHandler(deps)

	cb := &tgbotapi.CallbackQuery{
		ID:   "cb789",
		Data: "xray:select:0",
		Message: &tgbotapi.Message{
			MessageID: 5,
			Chat:      &tgbotapi.Chat{ID: 300},
		},
	}
	h.HandleCallback(cb)

	if !strings.Contains(sender.lastText, "template not found") {
		t.Errorf("expected error message, got %q", sender.lastText)
	}
}

func TestXrayHandler_HandleCallback_RestartError(t *testing.T) {
	sender := &mockSenderWithKeyboard{}
	servers := []vpnconfig.Server{{Name: "Server1"}}
	config := &mockConfigStore{servers: servers}
	xray := &mockXrayGenerator{}
	vpn := &mockVPNDirector{restartXrayErr: errors.New("xray not running")}

	deps := &Deps{Sender: sender, Config: config, Xray: xray, VPN: vpn}
	h := NewXrayHandler(deps)

	cb := &tgbotapi.CallbackQuery{
		ID:   "cb_restart",
		Data: "xray:select:0",
		Message: &tgbotapi.Message{
			MessageID: 7,
			Chat:      &tgbotapi.Chat{ID: 400},
		},
	}
	h.HandleCallback(cb)

	if !strings.Contains(sender.lastText, "xray not running") {
		t.Errorf("expected restart error message, got %q", sender.lastText)
	}
}
```

Also update `mockVPNDirector` in `status_test.go` to add `restartXrayErr`:

```go
type mockVPNDirector struct {
	statusOutput   string
	statusErr      error
	restartErr     error
	restartXrayErr error
	stopErr        error
}

func (m *mockVPNDirector) RestartXray() error { return m.restartXrayErr }
```

**Step 2: Run test to verify it fails**

Run: `cd telegram-bot && go test -v ./internal/handler/ -run TestXrayHandler_HandleCallback`
Expected: FAIL with "HandleCallback not defined"

**Step 3: Write minimal implementation**

Add to `xray.go`:

```go
import (
	"strconv"
	"strings"
)

// HandleCallback handles xray:select:{index} callbacks
func (h *XrayHandler) HandleCallback(cb *tgbotapi.CallbackQuery) {
	if cb.Message == nil {
		return
	}

	chatID := cb.Message.Chat.ID
	data := cb.Data

	// Parse server index from "xray:select:N"
	if !strings.HasPrefix(data, "xray:select:") {
		return
	}

	idxStr := strings.TrimPrefix(data, "xray:select:")
	idx, err := strconv.Atoi(idxStr)
	if err != nil {
		h.deps.Sender.Send(chatID, telegram.EscapeMarkdownV2("Ошибка: неверный индекс"))
		return
	}

	// Load servers
	servers, err := h.deps.Config.LoadServers()
	if err != nil {
		h.deps.Sender.Send(chatID, telegram.EscapeMarkdownV2(fmt.Sprintf("Ошибка: %v", err)))
		return
	}

	if idx < 0 || idx >= len(servers) {
		h.deps.Sender.Send(chatID, telegram.EscapeMarkdownV2("Ошибка: сервер не найден"))
		return
	}

	server := servers[idx]

	// Generate Xray config
	if err := h.deps.Xray.GenerateConfig(server); err != nil {
		h.deps.Sender.Send(chatID, telegram.EscapeMarkdownV2(fmt.Sprintf("Ошибка: %v", err)))
		return
	}

	// Restart Xray
	if err := h.deps.VPN.RestartXray(); err != nil {
		h.deps.Sender.Send(chatID, telegram.EscapeMarkdownV2(fmt.Sprintf("Ошибка перезапуска: %v", err)))
		return
	}

	// Send success message
	h.deps.Sender.Send(chatID, telegram.EscapeMarkdownV2(fmt.Sprintf("✓ Переключено на %s", server.Name)))
}
```

**Step 4: Run test to verify it passes**

Run: `cd telegram-bot && go test -v ./internal/handler/ -run TestXrayHandler_HandleCallback`
Expected: PASS

**Step 5: Commit**

```bash
git add telegram-bot/internal/handler/xray.go telegram-bot/internal/handler/xray_test.go telegram-bot/internal/handler/status_test.go
git commit -m "$(cat <<'EOF'
feat(telegram-bot): add XrayHandler.HandleCallback

Generates config and restarts Xray on server selection.
EOF
)"
```

---

## Task 3: Add XrayRouterHandler interface to router

**Files:**
- Modify: `telegram-bot/internal/bot/router.go`

**Step 1: Write the failing test**

No separate test needed - we'll verify integration in Task 5.

**Step 2: Add interface and routing**

Add to `router.go`:

```go
// XrayRouterHandler defines methods for xray command
type XrayRouterHandler interface {
	HandleXray(msg *tgbotapi.Message)
	HandleCallback(cb *tgbotapi.CallbackQuery)
}
```

Update Router struct:

```go
type Router struct {
	status  StatusRouterHandler
	servers ServersRouterHandler
	import_ ImportRouterHandler
	misc    MiscRouterHandler
	update  UpdateRouterHandler
	wizard  WizardRouterHandler
	xray    XrayRouterHandler // add this
}
```

Update NewRouter:

```go
func NewRouter(
	status StatusRouterHandler,
	servers ServersRouterHandler,
	import_ ImportRouterHandler,
	misc MiscRouterHandler,
	update UpdateRouterHandler,
	wizard WizardRouterHandler,
	xray XrayRouterHandler, // add this
) *Router {
	return &Router{
		status:  status,
		servers: servers,
		import_: import_,
		misc:    misc,
		update:  update,
		wizard:  wizard,
		xray:    xray, // add this
	}
}
```

Add case in RouteMessage:

```go
case "xray":
	r.xray.HandleXray(msg)
```

Add routing in RouteCallback (before wizard fallback):

```go
if strings.HasPrefix(cb.Data, "xray:") {
	r.xray.HandleCallback(cb)
	return
}
```

**Step 3: Run existing tests to verify no regressions**

Run: `cd telegram-bot && go test -v ./internal/bot/...`
Expected: May fail if bot_test.go exists and expects old NewRouter signature

**Step 4: Commit**

```bash
git add telegram-bot/internal/bot/router.go
git commit -m "$(cat <<'EOF'
feat(telegram-bot): add XrayRouterHandler to router

Routes /xray command and xray:* callbacks.
EOF
)"
```

---

## Task 4: Wire XrayHandler in bot.go

**Files:**
- Modify: `telegram-bot/internal/bot/bot.go`

**Step 1: Add XrayHandler creation and wire to Router**

In `New()` function, after other handlers:

```go
// Create handlers
statusHandler := handler.NewStatusHandler(deps)
serversHandler := handler.NewServersHandler(deps)
importHandler := handler.NewImportHandler(deps)
miscHandler := handler.NewMiscHandler(deps)
updateHandler := handler.NewUpdateHandler(sender, b.updater, b.devMode, version)
wizardHandler := wizard.NewHandler(sender, configSvc, vpnSvc, xraySvc)
xrayHandler := handler.NewXrayHandler(deps) // add this

// Create router - update call to include xrayHandler
router := NewRouter(statusHandler, serversHandler, importHandler, miscHandler, updateHandler, wizardHandler, xrayHandler)
```

**Step 2: Register /xray command**

Add to `RegisterCommands()`:

```go
commands := []tgbotapi.BotCommand{
	{Command: "status", Description: "Xray status"},
	{Command: "xray", Description: "Switch Xray server"},  // add this
	{Command: "servers", Description: "Server list"},
	// ... rest
}
```

**Step 3: Run build to verify compilation**

Run: `cd telegram-bot && go build ./...`
Expected: PASS

**Step 4: Commit**

```bash
git add telegram-bot/internal/bot/bot.go
git commit -m "$(cat <<'EOF'
feat(telegram-bot): wire XrayHandler and register /xray command
EOF
)"
```

---

## Task 5: Integration test

**Files:**
- Test: `telegram-bot/internal/handler/xray_test.go`

**Step 1: Add full flow test**

```go
func TestXrayHandler_FullFlow(t *testing.T) {
	sender := &mockSenderWithKeyboard{}
	servers := []vpnconfig.Server{
		{Name: "Germany, Berlin", Address: "de.example.com", Port: 443, UUID: "uuid1", IP: "1.1.1.1"},
		{Name: "USA, New York", Address: "us.example.com", Port: 443, UUID: "uuid2", IP: "2.2.2.2"},
		{Name: "Japan, Tokyo", Address: "jp.example.com", Port: 443, UUID: "uuid3", IP: "3.3.3.3"},
	}
	config := &mockConfigStore{servers: servers}
	xray := &mockXrayGenerator{}
	vpn := &mockVPNDirector{}

	deps := &Deps{Sender: sender, Config: config, Xray: xray, VPN: vpn}
	h := NewXrayHandler(deps)

	// Step 1: User sends /xray
	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 100}}
	h.HandleXray(msg)

	// Should show keyboard with 3 servers in 2 columns
	if len(sender.lastKeyboard.InlineKeyboard) != 2 { // 2 rows (2+1)
		t.Errorf("expected 2 keyboard rows, got %d", len(sender.lastKeyboard.InlineKeyboard))
	}
	// First row should have 2 buttons
	if len(sender.lastKeyboard.InlineKeyboard[0]) != 2 {
		t.Errorf("expected 2 buttons in first row, got %d", len(sender.lastKeyboard.InlineKeyboard[0]))
	}
	// Verify callback data format
	btn := sender.lastKeyboard.InlineKeyboard[0][0]
	if btn.CallbackData == nil || *btn.CallbackData != "xray:select:0" {
		t.Errorf("expected callback data 'xray:select:0', got %v", btn.CallbackData)
	}

	// Step 2: User clicks on USA server (index 1)
	cb := &tgbotapi.CallbackQuery{
		ID:   "cb",
		Data: "xray:select:1",
		Message: &tgbotapi.Message{
			MessageID: 42,
			Chat:      &tgbotapi.Chat{ID: 100},
		},
	}
	h.HandleCallback(cb)

	// Verify correct server was used
	if xray.lastServer.Address != "us.example.com" {
		t.Errorf("expected address 'us.example.com', got %q", xray.lastServer.Address)
	}
	// Verify success message
	if !strings.Contains(sender.lastText, "USA, New York") {
		t.Errorf("expected success message with 'USA, New York', got %q", sender.lastText)
	}
}
```

**Step 2: Run all xray tests**

Run: `cd telegram-bot && go test -v ./internal/handler/ -run TestXrayHandler`
Expected: PASS

**Step 3: Run all bot tests**

Run: `cd telegram-bot && go test -v ./...`
Expected: PASS

**Step 4: Commit**

```bash
git add telegram-bot/internal/handler/xray_test.go
git commit -m "$(cat <<'EOF'
test(telegram-bot): add integration test for /xray flow
EOF
)"
```

---

## Task 6: Update documentation

**Files:**
- Modify: `.claude/rules/telegram-bot.md`

**Step 1: Add /xray to commands table**

Find the Commands table and add:

```markdown
| `/xray` | `XrayHandler.HandleXray` | Quick server switch |
```

**Step 2: Commit**

```bash
git add .claude/rules/telegram-bot.md
git commit -m "$(cat <<'EOF'
docs: add /xray command to telegram-bot docs
EOF
)"
```

---

## Summary

| Task | Description | Files |
|------|-------------|-------|
| 1 | Create XrayHandler with HandleXray | `handler/xray.go`, `handler/xray_test.go` |
| 2 | Add HandleCallback for server selection | `handler/xray.go`, `handler/xray_test.go`, `handler/status_test.go` |
| 3 | Add XrayRouterHandler interface to router | `bot/router.go` |
| 4 | Wire XrayHandler in bot.go | `bot/bot.go` |
| 5 | Integration test | `handler/xray_test.go` |
| 6 | Update documentation | `.claude/rules/telegram-bot.md` |

Total: 6 tasks, ~30 steps
