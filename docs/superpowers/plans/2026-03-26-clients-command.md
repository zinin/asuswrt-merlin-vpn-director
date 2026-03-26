# /clients Command Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `/clients` Telegram bot command for listing, pausing/resuming, adding, and removing VPN clients without full reconfiguration.

**Architecture:** New `paused_clients` top-level field in vpn-director.json acts as a filter mask — shell scripts subtract it from all `clients` arrays at load time. Bot gets a new `ClientsHandler` following the existing ExcludeHandler pattern (callbacks + text input state). IPs are stored in normalized form (single hosts without `/32`), ensuring consistent matching across xray and tunnel_director sections.

**Tech Stack:** Go (telegram bot), Bash/jq (router scripts), Bats (shell tests)

**Spec:** `docs/superpowers/specs/2026-03-26-clients-command-design.md`

---

## File Map

| Action | File | Responsibility |
|--------|------|----------------|
| Modify | `telegram-bot/internal/vpnconfig/vpnconfig.go` | Add `PausedClients` field, `ClientInfo` type, `CollectClients` function |
| Modify | `telegram-bot/internal/vpnconfig/vpnconfig_test.go` | Tests for PausedClients serialization and CollectClients |
| Create | `telegram-bot/internal/handler/clients.go` | ClientsHandler: list, pause, resume, remove, add |
| Create | `telegram-bot/internal/handler/clients_test.go` | Handler unit tests |
| Modify | `telegram-bot/internal/bot/router.go` | Add ClientsRouterHandler interface, routing |
| Modify | `telegram-bot/internal/bot/router_test.go` | Router dispatch tests for /clients and clients: callbacks |
| Modify | `telegram-bot/internal/bot/bot.go` | Create ClientsHandler, pass to Router, register command |
| Modify | `router/opt/vpn-director/lib/config.sh` | Filter paused_clients from XRAY_CLIENTS and TUN_DIR_TUNNELS_JSON |
| Modify | `router/test/config.bats` | Tests for paused_clients filtering |
| Modify | `router/test/fixtures/vpn-director.json` | Add paused_clients to test fixture |
| Modify | `telegram-bot/testdata/dev/vpn-director.json` | Add paused_clients for dev mode |

---

### Task 1: Add PausedClients field to VPNDirectorConfig

**Files:**
- Modify: `telegram-bot/internal/vpnconfig/vpnconfig.go:16-21`
- Modify: `telegram-bot/internal/vpnconfig/vpnconfig_test.go`

- [ ] **Step 1: Write test for PausedClients round-trip serialization**

Add to `telegram-bot/internal/vpnconfig/vpnconfig_test.go` (add `"strings"` to the import block):

```go
func TestLoadVPNDirectorConfig_PausedClients(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "vpn-director.json")

	jsonContent := `{
		"paused_clients": ["192.168.50.10", "192.168.50.20/32"],
		"tunnel_director": {"tunnels": {}},
		"xray": {"clients": ["192.168.50.10"], "servers": [], "exclude_sets": []}
	}`

	err := os.WriteFile(path, []byte(jsonContent), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cfg, err := LoadVPNDirectorConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.PausedClients) != 2 {
		t.Fatalf("expected 2 paused clients, got %d", len(cfg.PausedClients))
	}
	if cfg.PausedClients[0] != "192.168.50.10" {
		t.Errorf("expected '192.168.50.10', got '%s'", cfg.PausedClients[0])
	}
	if cfg.PausedClients[1] != "192.168.50.20/32" {
		t.Errorf("expected '192.168.50.20/32', got '%s'", cfg.PausedClients[1])
	}
}

func TestSaveVPNDirectorConfig_PausedClients_OmitEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "vpn-director.json")

	cfg := &VPNDirectorConfig{
		DataDir:        "/data",
		TunnelDirector: TunnelDirectorConfig{Tunnels: map[string]TunnelConfig{}},
		Xray:           XrayConfig{Clients: []string{}, Servers: []string{}, ExcludeSets: []string{}},
	}

	if err := SaveVPNDirectorConfig(path, cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read: %v", err)
	}

	content := string(data)
	if strings.Contains(content, "paused_clients") {
		t.Error("expected paused_clients to be omitted when empty")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd telegram-bot && go test -run "TestLoadVPNDirectorConfig_PausedClients|TestSaveVPNDirectorConfig_PausedClients_OmitEmpty" ./internal/vpnconfig/ -v`

Expected: FAIL — `PausedClients` field doesn't exist.

- [ ] **Step 3: Add PausedClients field to VPNDirectorConfig**

In `telegram-bot/internal/vpnconfig/vpnconfig.go`, modify the struct:

```go
type VPNDirectorConfig struct {
	DataDir        string                 `json:"data_dir"`
	PausedClients  []string               `json:"paused_clients,omitempty"`
	TunnelDirector TunnelDirectorConfig   `json:"tunnel_director"`
	Xray           XrayConfig             `json:"xray"`
	Advanced       map[string]interface{} `json:"advanced,omitempty"`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd telegram-bot && go test -run "TestLoadVPNDirectorConfig_PausedClients|TestSaveVPNDirectorConfig_PausedClients_OmitEmpty" ./internal/vpnconfig/ -v`

Expected: PASS

- [ ] **Step 5: Run all vpnconfig tests to check nothing broke**

Run: `cd telegram-bot && go test ./internal/vpnconfig/ -v`

Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add telegram-bot/internal/vpnconfig/vpnconfig.go telegram-bot/internal/vpnconfig/vpnconfig_test.go
git commit -m "feat(vpnconfig): add PausedClients field to VPNDirectorConfig"
```

---

### Task 2: Add CollectClients helper function

**Files:**
- Modify: `telegram-bot/internal/vpnconfig/vpnconfig.go`
- Modify: `telegram-bot/internal/vpnconfig/vpnconfig_test.go`

- [ ] **Step 1: Write tests for CollectClients**

Add to `telegram-bot/internal/vpnconfig/vpnconfig_test.go`:

```go
func TestCollectClients_MixedRoutes(t *testing.T) {
	cfg := &VPNDirectorConfig{
		Xray: XrayConfig{
			Clients: []string{"192.168.50.10", "192.168.50.20"},
		},
		TunnelDirector: TunnelDirectorConfig{
			Tunnels: map[string]TunnelConfig{
				"wgc1": {
					Clients: []string{"192.168.50.30/32", "192.168.50.40/32"},
				},
				"ovpnc1": {
					Clients: []string{"192.168.1.5/32"},
				},
			},
		},
		PausedClients: []string{"192.168.50.20", "192.168.50.40/32"},
	}

	clients := CollectClients(cfg)

	if len(clients) != 5 {
		t.Fatalf("expected 5 clients, got %d", len(clients))
	}

	// Check xray clients
	found := findClient(clients, "192.168.50.10", "xray")
	if found == nil {
		t.Error("expected 192.168.50.10 -> xray")
	} else if found.Paused {
		t.Error("expected 192.168.50.10 to be active")
	}

	found = findClient(clients, "192.168.50.20", "xray")
	if found == nil {
		t.Error("expected 192.168.50.20 -> xray")
	} else if !found.Paused {
		t.Error("expected 192.168.50.20 to be paused")
	}

	// Check tunnel clients
	found = findClient(clients, "192.168.50.30/32", "wgc1")
	if found == nil {
		t.Error("expected 192.168.50.30/32 -> wgc1")
	} else if found.Paused {
		t.Error("expected 192.168.50.30/32 to be active")
	}

	found = findClient(clients, "192.168.50.40/32", "wgc1")
	if found == nil {
		t.Error("expected 192.168.50.40/32 -> wgc1")
	} else if !found.Paused {
		t.Error("expected 192.168.50.40/32 to be paused")
	}

	found = findClient(clients, "192.168.1.5/32", "ovpnc1")
	if found == nil {
		t.Error("expected 192.168.1.5/32 -> ovpnc1")
	}
}

func TestCollectClients_Empty(t *testing.T) {
	cfg := &VPNDirectorConfig{
		Xray:           XrayConfig{},
		TunnelDirector: TunnelDirectorConfig{},
	}

	clients := CollectClients(cfg)
	if len(clients) != 0 {
		t.Errorf("expected 0 clients, got %d", len(clients))
	}
}

func TestCollectClients_NoPausedClients(t *testing.T) {
	cfg := &VPNDirectorConfig{
		Xray: XrayConfig{Clients: []string{"192.168.50.10"}},
	}

	clients := CollectClients(cfg)
	if len(clients) != 1 {
		t.Fatalf("expected 1 client, got %d", len(clients))
	}
	if clients[0].Paused {
		t.Error("expected client to be active")
	}
}

func findClient(clients []ClientInfo, ip, route string) *ClientInfo {
	for i := range clients {
		if clients[i].IP == ip && clients[i].Route == route {
			return &clients[i]
		}
	}
	return nil
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd telegram-bot && go test -run "TestCollectClients" ./internal/vpnconfig/ -v`

Expected: FAIL — `ClientInfo` and `CollectClients` don't exist.

- [ ] **Step 3: Implement ClientInfo and CollectClients**

Add to `telegram-bot/internal/vpnconfig/vpnconfig.go`:

```go
// ClientInfo represents a VPN client with its route and pause status.
type ClientInfo struct {
	IP     string
	Route  string
	Paused bool
}

// CollectClients builds a unified list of all clients from xray and tunnel_director sections.
func CollectClients(cfg *VPNDirectorConfig) []ClientInfo {
	paused := make(map[string]bool, len(cfg.PausedClients))
	for _, ip := range cfg.PausedClients {
		paused[ip] = true
	}

	var clients []ClientInfo

	for _, ip := range cfg.Xray.Clients {
		clients = append(clients, ClientInfo{
			IP:     ip,
			Route:  "xray",
			Paused: paused[ip],
		})
	}

	// Sort tunnel names for deterministic order
	tunnelNames := make([]string, 0, len(cfg.TunnelDirector.Tunnels))
	for name := range cfg.TunnelDirector.Tunnels {
		tunnelNames = append(tunnelNames, name)
	}
	sort.Strings(tunnelNames)

	for _, name := range tunnelNames {
		tunnel := cfg.TunnelDirector.Tunnels[name]
		for _, ip := range tunnel.Clients {
			clients = append(clients, ClientInfo{
				IP:     ip,
				Route:  name,
				Paused: paused[ip],
			})
		}
	}

	return clients
}
```

Add `"sort"` to imports.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd telegram-bot && go test -run "TestCollectClients" ./internal/vpnconfig/ -v`

Expected: All PASS

- [ ] **Step 5: Run all vpnconfig tests**

Run: `cd telegram-bot && go test ./internal/vpnconfig/ -v`

Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add telegram-bot/internal/vpnconfig/vpnconfig.go telegram-bot/internal/vpnconfig/vpnconfig_test.go
git commit -m "feat(vpnconfig): add CollectClients helper for unified client listing"
```

---

### Task 3: Shell config.sh — filter paused_clients

**Files:**
- Modify: `router/opt/vpn-director/lib/config.sh:40-55`
- Modify: `router/test/fixtures/vpn-director.json`
- Modify: `router/test/config.bats`

- [ ] **Step 1: Write bats tests for paused_clients filtering**

Add to `router/test/config.bats` at the end:

```bash
# ============================================================================
# config.sh: paused_clients filtering
# ============================================================================

@test "config: paused_clients filters XRAY_CLIENTS" {
    local tmp_cfg="/tmp/bats_test_paused_config.json"
    jq '.paused_clients = ["192.168.1.100"]' "$TEST_ROOT/fixtures/vpn-director.json" > "$tmp_cfg"
    export VPD_CONFIG_FILE="$tmp_cfg"
    source "$LIB_DIR/config.sh"
    [[ "$XRAY_CLIENTS" != *"192.168.1.100"* ]]
    rm -f "$tmp_cfg"
}

@test "config: paused_clients filters TUN_DIR_TUNNELS_JSON clients" {
    local tmp_cfg="/tmp/bats_test_paused_td_config.json"
    jq '.paused_clients = ["192.168.50.0/24"]' "$TEST_ROOT/fixtures/vpn-director.json" > "$tmp_cfg"
    export VPD_CONFIG_FILE="$tmp_cfg"
    source "$LIB_DIR/config.sh"
    local clients
    clients=$(printf '%s\n' "$TUN_DIR_TUNNELS_JSON" | jq -r '.wgc1.clients[]')
    [[ "$clients" != *"192.168.50.0/24"* ]]
    rm -f "$tmp_cfg"
}

@test "config: empty paused_clients changes nothing" {
    local tmp_cfg="/tmp/bats_test_empty_paused_config.json"
    jq '.paused_clients = []' "$TEST_ROOT/fixtures/vpn-director.json" > "$tmp_cfg"
    export VPD_CONFIG_FILE="$tmp_cfg"
    source "$LIB_DIR/config.sh"
    [[ "$XRAY_CLIENTS" == *"192.168.1.100"* ]]
    rm -f "$tmp_cfg"
}

@test "config: missing paused_clients changes nothing" {
    load_config
    [[ "$XRAY_CLIENTS" == *"192.168.1.100"* ]]
    local clients
    clients=$(printf '%s\n' "$TUN_DIR_TUNNELS_JSON" | jq -r '.wgc1.clients[]')
    [[ "$clients" == *"192.168.50.0/24"* ]]
}

@test "config: paused_clients filtering preserves tunnel exclude field" {
    local tmp_cfg="/tmp/bats_test_paused_exclude_config.json"
    jq '.paused_clients = ["192.168.50.0/24"]' "$TEST_ROOT/fixtures/vpn-director.json" > "$tmp_cfg"
    export VPD_CONFIG_FILE="$tmp_cfg"
    source "$LIB_DIR/config.sh"
    local exclude
    exclude=$(printf '%s\n' "$TUN_DIR_TUNNELS_JSON" | jq -r '.wgc1.exclude[]')
    [[ "$exclude" == *"ru"* ]]
    rm -f "$tmp_cfg"
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `bats router/test/config.bats --filter "paused_clients"`

Expected: First two tests FAIL (filtering not implemented), last two PASS.

- [ ] **Step 3: Implement paused_clients filtering in config.sh**

Modify `router/opt/vpn-director/lib/config.sh`. After the helper functions section (line 41), add paused_clients loading. Then modify lines 46 and 52 to use filtered versions:

Replace the sections:

```bash
###################################################################################################
# 3. Helper functions
###################################################################################################
_cfg() { jq -r "$1 // empty" "$VPD_CONFIG_FILE"; }
_cfg_arr() { jq -r "$1 // [] | .[]" "$VPD_CONFIG_FILE" | tr '\n' ' ' | sed 's/ $//'; }

###################################################################################################
# 4. Tunnel Director variables
###################################################################################################
TUN_DIR_TUNNELS_JSON=$(_cfg '.tunnel_director.tunnels // {}')
```

With:

```bash
###################################################################################################
# 3. Helper functions
###################################################################################################
_cfg() { jq -r "$1 // empty" "$VPD_CONFIG_FILE"; }
_cfg_arr() { jq -r "$1 // [] | .[]" "$VPD_CONFIG_FILE" | tr '\n' ' ' | sed 's/ $//'; }

# Paused clients mask: filtered from all clients arrays
_PAUSED_CLIENTS_JSON=$(jq -c '.paused_clients // []' "$VPD_CONFIG_FILE")

# Array loader that subtracts paused_clients
_cfg_arr_active() {
    jq -r --argjson p "$_PAUSED_CLIENTS_JSON" \
        "($1 // []) - \$p | .[]" "$VPD_CONFIG_FILE" | tr '\n' ' ' | sed 's/ $//';
}

###################################################################################################
# 4. Tunnel Director variables
###################################################################################################
TUN_DIR_TUNNELS_JSON=$(jq --argjson p "$_PAUSED_CLIENTS_JSON" \
    '(.tunnel_director.tunnels // {})
     | to_entries
     | map(.value.clients = ((.value.clients // []) - $p))
     | from_entries' "$VPD_CONFIG_FILE")
```

Then replace line 52:

```bash
XRAY_CLIENTS=$(_cfg_arr '.xray.clients')
```

With:

```bash
XRAY_CLIENTS=$(_cfg_arr_active '.xray.clients')
```

- [ ] **Step 4: Run paused_clients tests to verify they pass**

Run: `bats router/test/config.bats --filter "paused_clients"`

Expected: All PASS

- [ ] **Step 5: Run ALL config tests to check nothing broke**

Run: `bats router/test/config.bats`

Expected: All PASS

- [ ] **Step 6: Run all router tests**

Run: `bats router/test/`

Expected: All PASS (or same failures as before this change)

- [ ] **Step 7: Commit**

```bash
git add router/opt/vpn-director/lib/config.sh router/test/config.bats
git commit -m "feat(config): filter paused_clients from all clients arrays at load time"
```

---

### Task 4: ClientsHandler — list rendering and keyboard

**Files:**
- Create: `telegram-bot/internal/handler/clients.go`
- Create: `telegram-bot/internal/handler/clients_test.go`

- [ ] **Step 1: Write test for client list rendering**

Create `telegram-bot/internal/handler/clients_test.go`:

```go
package handler

import (
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/vpnconfig"
)

type mockSenderClients struct {
	lastChatID   int64
	lastText     string
	lastKeyboard tgbotapi.InlineKeyboardMarkup
	editChatID   int64
	editMsgID    int
	editText     string
	editKeyboard tgbotapi.InlineKeyboardMarkup
	plainTexts   []string
}

func (m *mockSenderClients) Send(chatID int64, text string) error {
	m.lastChatID = chatID
	m.lastText = text
	return nil
}
func (m *mockSenderClients) SendPlain(chatID int64, text string) error {
	m.plainTexts = append(m.plainTexts, text)
	m.lastChatID = chatID
	return nil
}
func (m *mockSenderClients) SendLongPlain(chatID int64, text string) error { return nil }
func (m *mockSenderClients) SendWithKeyboard(chatID int64, text string, kb tgbotapi.InlineKeyboardMarkup) error {
	m.lastChatID = chatID
	m.lastText = text
	m.lastKeyboard = kb
	return nil
}
func (m *mockSenderClients) SendCodeBlock(chatID int64, header, content string) error { return nil }
func (m *mockSenderClients) EditMessage(chatID int64, msgID int, text string, kb tgbotapi.InlineKeyboardMarkup) error {
	m.editChatID = chatID
	m.editMsgID = msgID
	m.editText = text
	m.editKeyboard = kb
	return nil
}
func (m *mockSenderClients) AckCallback(callbackID string) error { return nil }

type mockConfigClients struct {
	vpnConfig   *vpnconfig.VPNDirectorConfig
	loadErr     error
	savedConfig *vpnconfig.VPNDirectorConfig
	saveErr     error
}

func (m *mockConfigClients) LoadVPNConfig() (*vpnconfig.VPNDirectorConfig, error) {
	return m.vpnConfig, m.loadErr
}
func (m *mockConfigClients) SaveVPNConfig(cfg *vpnconfig.VPNDirectorConfig) error {
	m.savedConfig = cfg
	return m.saveErr
}
func (m *mockConfigClients) LoadServers() ([]vpnconfig.Server, error)   { return nil, nil }
func (m *mockConfigClients) SaveServers(s []vpnconfig.Server) error     { return nil }
func (m *mockConfigClients) DataDir() (string, error)                   { return "/data", nil }
func (m *mockConfigClients) DataDirOrDefault() string                   { return "/data" }
func (m *mockConfigClients) ScriptsDir() string                         { return "/scripts" }

type mockVPNClients struct {
	applyErr error
}

func (m *mockVPNClients) Status() (string, error) { return "", nil }
func (m *mockVPNClients) Apply() error            { return m.applyErr }
func (m *mockVPNClients) Restart() error          { return nil }
func (m *mockVPNClients) RestartXray() error      { return nil }
func (m *mockVPNClients) Stop() error             { return nil }

func TestClientsHandler_HandleClients_WithClients(t *testing.T) {
	sender := &mockSenderClients{}
	config := &mockConfigClients{
		vpnConfig: &vpnconfig.VPNDirectorConfig{
			Xray: vpnconfig.XrayConfig{
				Clients: []string{"192.168.50.10"},
			},
			TunnelDirector: vpnconfig.TunnelDirectorConfig{
				Tunnels: map[string]vpnconfig.TunnelConfig{
					"wgc1": {Clients: []string{"192.168.50.20/32"}},
				},
			},
			PausedClients: []string{"192.168.50.20/32"},
		},
	}
	deps := &Deps{Sender: sender, Config: config}
	h := NewClientsHandler(deps)

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 100}}
	h.HandleClients(msg)

	if sender.lastChatID != 100 {
		t.Errorf("expected chatID 100, got %d", sender.lastChatID)
	}
	// Should show both clients in message text
	if !strings.Contains(sender.lastText, "192.168.50.10") {
		t.Error("expected message to contain 192.168.50.10")
	}
	if !strings.Contains(sender.lastText, "192.168.50.20") {
		t.Error("expected message to contain 192.168.50.20/32")
	}
	// 2 clients × 2 buttons + 1 Add row = 3 rows
	if len(sender.lastKeyboard.InlineKeyboard) != 3 {
		t.Errorf("expected 3 keyboard rows, got %d", len(sender.lastKeyboard.InlineKeyboard))
	}
}

func TestClientsHandler_HandleClients_Empty(t *testing.T) {
	sender := &mockSenderClients{}
	config := &mockConfigClients{
		vpnConfig: &vpnconfig.VPNDirectorConfig{
			Xray:           vpnconfig.XrayConfig{},
			TunnelDirector: vpnconfig.TunnelDirectorConfig{},
		},
	}
	deps := &Deps{Sender: sender, Config: config}
	h := NewClientsHandler(deps)

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 100}}
	h.HandleClients(msg)

	// Should have Add button only
	if len(sender.lastKeyboard.InlineKeyboard) != 1 {
		t.Errorf("expected 1 keyboard row (Add), got %d", len(sender.lastKeyboard.InlineKeyboard))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd telegram-bot && go test -run "TestClientsHandler_HandleClients" ./internal/handler/ -v`

Expected: FAIL — `ClientsHandler` doesn't exist.

- [ ] **Step 3: Implement ClientsHandler with list rendering**

Create `telegram-bot/internal/handler/clients.go`:

```go
package handler

import (
	"fmt"
	"strings"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/telegram"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/vpnconfig"
)

// ClientsHandler handles the /clients command for managing VPN clients.
type ClientsHandler struct {
	deps *Deps
	mu   sync.Mutex
	// addState tracks chat IDs in "add client" flow.
	// Key: chatID. Value: pending IP ("" = waiting for IP text, non-empty = waiting for route).
	addState map[int64]string
}

// NewClientsHandler creates a new ClientsHandler.
func NewClientsHandler(deps *Deps) *ClientsHandler {
	return &ClientsHandler{
		deps:     deps,
		addState: make(map[int64]string),
	}
}

// ClearState removes any pending add-client state for a chat.
func (h *ClientsHandler) ClearState(chatID int64) {
	h.mu.Lock()
	delete(h.addState, chatID)
	h.mu.Unlock()
}

// HandleClients handles the /clients command — shows client list with actions.
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
			status := "\u25b6"  // ▶
			if c.Paused {
				status = "\u23f8" // ⏸
			}
			sb.WriteString(telegram.EscapeMarkdownV2(fmt.Sprintf("%s  %s → %s", status, c.IP, c.Route)) + "\n")

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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd telegram-bot && go test -run "TestClientsHandler_HandleClients" ./internal/handler/ -v`

Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add telegram-bot/internal/handler/clients.go telegram-bot/internal/handler/clients_test.go
git commit -m "feat(telegram-bot): add ClientsHandler with /clients list rendering"
```

---

### Task 5: Pause/Resume callbacks

**Files:**
- Modify: `telegram-bot/internal/handler/clients.go`
- Modify: `telegram-bot/internal/handler/clients_test.go`

- [ ] **Step 1: Write tests for pause and resume**

Add to `telegram-bot/internal/handler/clients_test.go`:

```go
func TestClientsHandler_HandlePause(t *testing.T) {
	sender := &mockSenderClients{}
	cfg := &vpnconfig.VPNDirectorConfig{
		Xray: vpnconfig.XrayConfig{Clients: []string{"192.168.50.10"}},
	}
	config := &mockConfigClients{vpnConfig: cfg}
	vpn := &mockVPNClients{}
	deps := &Deps{Sender: sender, Config: config, VPN: vpn}
	h := NewClientsHandler(deps)

	cb := &tgbotapi.CallbackQuery{
		Data:    "clients:pause:192.168.50.10",
		Message: &tgbotapi.Message{MessageID: 42, Chat: &tgbotapi.Chat{ID: 100}},
	}
	h.HandleCallback(cb)

	if config.savedConfig == nil {
		t.Fatal("expected config to be saved")
	}
	if len(config.savedConfig.PausedClients) != 1 || config.savedConfig.PausedClients[0] != "192.168.50.10" {
		t.Errorf("expected paused_clients=[192.168.50.10], got %v", config.savedConfig.PausedClients)
	}
	if sender.editMsgID != 42 {
		t.Errorf("expected message 42 to be edited, got %d", sender.editMsgID)
	}
}

func TestClientsHandler_HandleResume(t *testing.T) {
	sender := &mockSenderClients{}
	cfg := &vpnconfig.VPNDirectorConfig{
		Xray:          vpnconfig.XrayConfig{Clients: []string{"192.168.50.10"}},
		PausedClients: []string{"192.168.50.10"},
	}
	config := &mockConfigClients{vpnConfig: cfg}
	vpn := &mockVPNClients{}
	deps := &Deps{Sender: sender, Config: config, VPN: vpn}
	h := NewClientsHandler(deps)

	cb := &tgbotapi.CallbackQuery{
		Data:    "clients:resume:192.168.50.10",
		Message: &tgbotapi.Message{MessageID: 42, Chat: &tgbotapi.Chat{ID: 100}},
	}
	h.HandleCallback(cb)

	if config.savedConfig == nil {
		t.Fatal("expected config to be saved")
	}
	if len(config.savedConfig.PausedClients) != 0 {
		t.Errorf("expected empty paused_clients, got %v", config.savedConfig.PausedClients)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd telegram-bot && go test -run "TestClientsHandler_Handle(Pause|Resume)" ./internal/handler/ -v`

Expected: FAIL — `HandleCallback` doesn't exist.

- [ ] **Step 3: Implement HandleCallback with pause/resume**

Add to `telegram-bot/internal/handler/clients.go`:

```go
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
		// Client was removed — refresh list silently
		text, kb := h.buildClientList(cfg)
		h.deps.Sender.EditMessage(chatID, msgID, text, kb)
		return
	}

	if pause {
		// Add to paused_clients if not already there
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
		// Remove from paused_clients
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
```

- [ ] **Step 4: Add stub methods to satisfy compilation**

Add empty stub methods for remove/add that will be implemented in later tasks:

```go
func (h *ClientsHandler) handleRemoveConfirm(chatID int64, msgID int, ip string) {}
func (h *ClientsHandler) handleRemove(chatID int64, msgID int, ip string)        {}
func (h *ClientsHandler) handleAddStart(chatID int64)                            {}
func (h *ClientsHandler) handleAddRoute(chatID int64, msgID int, route string)   {}

// HandleTextInput handles text messages for the add-client flow.
func (h *ClientsHandler) HandleTextInput(msg *tgbotapi.Message) {}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd telegram-bot && go test -run "TestClientsHandler_Handle(Pause|Resume)" ./internal/handler/ -v`

Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add telegram-bot/internal/handler/clients.go telegram-bot/internal/handler/clients_test.go
git commit -m "feat(telegram-bot): add pause/resume client callbacks"
```

---

### Task 6: Remove client with confirmation

**Files:**
- Modify: `telegram-bot/internal/handler/clients.go`
- Modify: `telegram-bot/internal/handler/clients_test.go`

- [ ] **Step 1: Write tests for remove flow**

Add to `telegram-bot/internal/handler/clients_test.go`:

```go
func TestClientsHandler_HandleRemoveConfirm(t *testing.T) {
	sender := &mockSenderClients{}
	cfg := &vpnconfig.VPNDirectorConfig{
		Xray: vpnconfig.XrayConfig{Clients: []string{"192.168.50.10"}},
	}
	config := &mockConfigClients{vpnConfig: cfg}
	deps := &Deps{Sender: sender, Config: config}
	h := NewClientsHandler(deps)

	cb := &tgbotapi.CallbackQuery{
		Data:    "clients:remove:192.168.50.10",
		Message: &tgbotapi.Message{MessageID: 42, Chat: &tgbotapi.Chat{ID: 100}},
	}
	h.HandleCallback(cb)

	// Should show confirmation with Yes/Cancel buttons
	if sender.editMsgID != 42 {
		t.Errorf("expected message 42 to be edited, got %d", sender.editMsgID)
	}
	if len(sender.editKeyboard.InlineKeyboard) != 1 {
		t.Errorf("expected 1 keyboard row, got %d", len(sender.editKeyboard.InlineKeyboard))
	}
}

func TestClientsHandler_HandleRemoveYes_Xray(t *testing.T) {
	sender := &mockSenderClients{}
	cfg := &vpnconfig.VPNDirectorConfig{
		Xray:          vpnconfig.XrayConfig{Clients: []string{"192.168.50.10", "192.168.50.20"}},
		PausedClients: []string{"192.168.50.10"},
	}
	config := &mockConfigClients{vpnConfig: cfg}
	vpn := &mockVPNClients{}
	deps := &Deps{Sender: sender, Config: config, VPN: vpn}
	h := NewClientsHandler(deps)

	cb := &tgbotapi.CallbackQuery{
		Data:    "clients:rm_yes:192.168.50.10",
		Message: &tgbotapi.Message{MessageID: 42, Chat: &tgbotapi.Chat{ID: 100}},
	}
	h.HandleCallback(cb)

	if config.savedConfig == nil {
		t.Fatal("expected config to be saved")
	}
	// Client should be removed from xray.clients
	if len(config.savedConfig.Xray.Clients) != 1 || config.savedConfig.Xray.Clients[0] != "192.168.50.20" {
		t.Errorf("expected xray.clients=[192.168.50.20], got %v", config.savedConfig.Xray.Clients)
	}
	// Client should be removed from paused_clients
	if len(config.savedConfig.PausedClients) != 0 {
		t.Errorf("expected empty paused_clients, got %v", config.savedConfig.PausedClients)
	}
}

func TestClientsHandler_HandleRemoveYes_Tunnel(t *testing.T) {
	sender := &mockSenderClients{}
	cfg := &vpnconfig.VPNDirectorConfig{
		TunnelDirector: vpnconfig.TunnelDirectorConfig{
			Tunnels: map[string]vpnconfig.TunnelConfig{
				"wgc1": {Clients: []string{"192.168.50.30/32", "192.168.50.40/32"}, Exclude: []string{"ru"}},
			},
		},
	}
	config := &mockConfigClients{vpnConfig: cfg}
	vpn := &mockVPNClients{}
	deps := &Deps{Sender: sender, Config: config, VPN: vpn}
	h := NewClientsHandler(deps)

	cb := &tgbotapi.CallbackQuery{
		Data:    "clients:rm_yes:192.168.50.30/32",
		Message: &tgbotapi.Message{MessageID: 42, Chat: &tgbotapi.Chat{ID: 100}},
	}
	h.HandleCallback(cb)

	if config.savedConfig == nil {
		t.Fatal("expected config to be saved")
	}
	wgc1 := config.savedConfig.TunnelDirector.Tunnels["wgc1"]
	if len(wgc1.Clients) != 1 || wgc1.Clients[0] != "192.168.50.40/32" {
		t.Errorf("expected wgc1.clients=[192.168.50.40/32], got %v", wgc1.Clients)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd telegram-bot && go test -run "TestClientsHandler_HandleRemove" ./internal/handler/ -v`

Expected: FAIL — stubs don't do anything.

- [ ] **Step 3: Implement handleRemoveConfirm and handleRemove**

Replace the stub methods in `telegram-bot/internal/handler/clients.go`:

```go
func (h *ClientsHandler) handleRemoveConfirm(chatID int64, msgID int, ip string) {
	cfg, err := h.deps.Config.LoadVPNConfig()
	if err != nil {
		h.deps.Sender.SendPlain(chatID, fmt.Sprintf("Config load error: %v", err))
		return
	}

	// Find the route for this IP
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

	// Remove from xray.clients
	cfg.Xray.Clients = removeString(cfg.Xray.Clients, ip)

	// Remove from all tunnel clients
	for name, tunnel := range cfg.TunnelDirector.Tunnels {
		tunnel.Clients = removeString(tunnel.Clients, ip)
		cfg.TunnelDirector.Tunnels[name] = tunnel
	}

	// Remove from paused_clients
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd telegram-bot && go test -run "TestClientsHandler_HandleRemove" ./internal/handler/ -v`

Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add telegram-bot/internal/handler/clients.go telegram-bot/internal/handler/clients_test.go
git commit -m "feat(telegram-bot): add remove client with confirmation"
```

---

### Task 7: Add client flow (text input + route selection)

**Files:**
- Modify: `telegram-bot/internal/handler/clients.go`
- Modify: `telegram-bot/internal/handler/clients_test.go`

- [ ] **Step 1: Write tests for add flow**

Add to `telegram-bot/internal/handler/clients_test.go`:

```go
func TestClientsHandler_HandleAdd_SetsState(t *testing.T) {
	sender := &mockSenderClients{}
	config := &mockConfigClients{
		vpnConfig: &vpnconfig.VPNDirectorConfig{
			Xray:           vpnconfig.XrayConfig{Clients: []string{}},
			TunnelDirector: vpnconfig.TunnelDirectorConfig{Tunnels: map[string]vpnconfig.TunnelConfig{}},
		},
	}
	deps := &Deps{Sender: sender, Config: config}
	h := NewClientsHandler(deps)

	cb := &tgbotapi.CallbackQuery{
		Data:    "clients:add",
		Message: &tgbotapi.Message{MessageID: 42, Chat: &tgbotapi.Chat{ID: 100}},
	}
	h.HandleCallback(cb)

	// Should ask for IP
	if len(sender.plainTexts) == 0 {
		t.Fatal("expected a plain text message")
	}
}

func TestClientsHandler_HandleTextInput_ValidIP(t *testing.T) {
	sender := &mockSenderClients{}
	config := &mockConfigClients{
		vpnConfig: &vpnconfig.VPNDirectorConfig{
			Xray: vpnconfig.XrayConfig{Clients: []string{}},
			TunnelDirector: vpnconfig.TunnelDirectorConfig{
				Tunnels: map[string]vpnconfig.TunnelConfig{
					"wgc1": {Clients: []string{}, Exclude: []string{"ru"}},
				},
			},
		},
	}
	deps := &Deps{Sender: sender, Config: config}
	h := NewClientsHandler(deps)

	// Set add state
	h.mu.Lock()
	h.addState[100] = ""
	h.mu.Unlock()

	msg := &tgbotapi.Message{
		Text: "192.168.50.10",
		Chat: &tgbotapi.Chat{ID: 100},
	}
	h.HandleTextInput(msg)

	// Should show route selection keyboard
	if sender.lastChatID != 100 {
		t.Errorf("expected chatID 100, got %d", sender.lastChatID)
	}
	// Should have route buttons (xray + wgc1 = at least 1 row)
	if len(sender.lastKeyboard.InlineKeyboard) == 0 {
		t.Error("expected route selection keyboard")
	}
}

func TestClientsHandler_HandleTextInput_InvalidIP(t *testing.T) {
	sender := &mockSenderClients{}
	config := &mockConfigClients{
		vpnConfig: &vpnconfig.VPNDirectorConfig{},
	}
	deps := &Deps{Sender: sender, Config: config}
	h := NewClientsHandler(deps)

	h.mu.Lock()
	h.addState[100] = ""
	h.mu.Unlock()

	msg := &tgbotapi.Message{
		Text: "not-an-ip",
		Chat: &tgbotapi.Chat{ID: 100},
	}
	h.HandleTextInput(msg)

	// Should show error
	if len(sender.plainTexts) == 0 {
		t.Fatal("expected error message")
	}
}

func TestClientsHandler_HandleTextInput_DuplicateIP(t *testing.T) {
	sender := &mockSenderClients{}
	config := &mockConfigClients{
		vpnConfig: &vpnconfig.VPNDirectorConfig{
			Xray: vpnconfig.XrayConfig{Clients: []string{"192.168.50.10"}},
		},
	}
	deps := &Deps{Sender: sender, Config: config}
	h := NewClientsHandler(deps)

	h.mu.Lock()
	h.addState[100] = ""
	h.mu.Unlock()

	msg := &tgbotapi.Message{
		Text: "192.168.50.10",
		Chat: &tgbotapi.Chat{ID: 100},
	}
	h.HandleTextInput(msg)

	if len(sender.plainTexts) == 0 {
		t.Fatal("expected duplicate error message")
	}
}

func TestClientsHandler_HandleAddRoute_Xray(t *testing.T) {
	sender := &mockSenderClients{}
	cfg := &vpnconfig.VPNDirectorConfig{
		Xray: vpnconfig.XrayConfig{Clients: []string{}},
	}
	config := &mockConfigClients{vpnConfig: cfg}
	vpn := &mockVPNClients{}
	deps := &Deps{Sender: sender, Config: config, VPN: vpn}
	h := NewClientsHandler(deps)

	// Set pending IP
	h.mu.Lock()
	h.addState[100] = "192.168.50.10"
	h.mu.Unlock()

	cb := &tgbotapi.CallbackQuery{
		Data:    "clients:route:xray",
		Message: &tgbotapi.Message{MessageID: 42, Chat: &tgbotapi.Chat{ID: 100}},
	}
	h.HandleCallback(cb)

	if config.savedConfig == nil {
		t.Fatal("expected config to be saved")
	}
	if len(config.savedConfig.Xray.Clients) != 1 || config.savedConfig.Xray.Clients[0] != "192.168.50.10" {
		t.Errorf("expected xray.clients=[192.168.50.10], got %v", config.savedConfig.Xray.Clients)
	}
}

func TestClientsHandler_HandleAddRoute_Tunnel(t *testing.T) {
	sender := &mockSenderClients{}
	cfg := &vpnconfig.VPNDirectorConfig{
		TunnelDirector: vpnconfig.TunnelDirectorConfig{
			Tunnels: map[string]vpnconfig.TunnelConfig{
				"wgc1": {Clients: []string{}, Exclude: []string{"ru"}},
			},
		},
	}
	config := &mockConfigClients{vpnConfig: cfg}
	vpn := &mockVPNClients{}
	deps := &Deps{Sender: sender, Config: config, VPN: vpn}
	h := NewClientsHandler(deps)

	h.mu.Lock()
	h.addState[100] = "192.168.50.30"
	h.mu.Unlock()

	cb := &tgbotapi.CallbackQuery{
		Data:    "clients:route:wgc1",
		Message: &tgbotapi.Message{MessageID: 42, Chat: &tgbotapi.Chat{ID: 100}},
	}
	h.HandleCallback(cb)

	if config.savedConfig == nil {
		t.Fatal("expected config to be saved")
	}
	wgc1 := config.savedConfig.TunnelDirector.Tunnels["wgc1"]
	// Should be normalized (no /32 suffix)
	if len(wgc1.Clients) != 1 || wgc1.Clients[0] != "192.168.50.30" {
		t.Errorf("expected wgc1.clients=[192.168.50.30], got %v", wgc1.Clients)
	}
}

func TestClientsHandler_HandleTextInput_NotInAddState(t *testing.T) {
	sender := &mockSenderClients{}
	deps := &Deps{Sender: sender}
	h := NewClientsHandler(deps)

	msg := &tgbotapi.Message{
		Text: "192.168.50.10",
		Chat: &tgbotapi.Chat{ID: 100},
	}
	h.HandleTextInput(msg)

	// Should not send anything — not in add state
	if sender.lastChatID != 0 {
		t.Error("expected no message sent when not in add state")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd telegram-bot && go test -run "TestClientsHandler_Handle(Add|TextInput|AddRoute)" ./internal/handler/ -v`

Expected: FAIL — stubs are empty.

- [ ] **Step 3: Implement add flow methods**

Replace the stub methods in `telegram-bot/internal/handler/clients.go`:

```go
func (h *ClientsHandler) handleAddStart(chatID int64) {
	h.mu.Lock()
	h.addState[chatID] = ""
	h.mu.Unlock()

	h.deps.Sender.SendPlain(chatID, "Enter client IP address (e.g. 192.168.50.10 or 192.168.50.0/24):")
}

// HandleTextInput handles text messages for the add-client flow.
func (h *ClientsHandler) HandleTextInput(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID

	h.mu.Lock()
	pendingIP, inAddState := h.addState[chatID]
	h.mu.Unlock()

	if !inAddState || pendingIP != "" {
		return // Not waiting for IP input
	}

	input := strings.TrimSpace(msg.Text)
	if input == "" {
		return
	}

	if !isValidIPOrCIDR(input) {
		h.deps.Sender.SendPlain(chatID, "Invalid format. Enter IPv4 (192.168.50.10) or CIDR (192.168.50.0/24):")
		return
	}

	// Check for duplicates
	cfg, err := h.deps.Config.LoadVPNConfig()
	if err != nil {
		h.deps.Sender.SendPlain(chatID, fmt.Sprintf("Config load error: %v", err))
		return
	}

	// Normalize for duplicate check
	normalized := normalizeIP(input)
	clients := vpnconfig.CollectClients(cfg)
	for _, c := range clients {
		if normalizeIP(c.IP) == normalized {
			h.deps.Sender.SendPlain(chatID, fmt.Sprintf("This IP is already configured for %s", c.Route))
			return
		}
	}

	// Save pending IP and show route selection
	h.mu.Lock()
	h.addState[chatID] = input
	h.mu.Unlock()

	h.showRouteSelection(chatID, input, cfg)
}

func (h *ClientsHandler) showRouteSelection(chatID int64, ip string, cfg *vpnconfig.VPNDirectorConfig) {
	kb := telegram.NewKeyboard()

	kb.Button("xray", "clients:route:xray").Row()

	for name := range cfg.TunnelDirector.Tunnels {
		kb.Button(name, fmt.Sprintf("clients:route:%s", name)).Row()
	}

	kb.Button("Cancel", "clients:route:cancel").Row()

	text := telegram.EscapeMarkdownV2(fmt.Sprintf("Select route for %s:", ip))
	h.deps.Sender.SendWithKeyboard(chatID, text, kb.Build())
}

func (h *ClientsHandler) handleAddRoute(chatID int64, msgID int, route string) {
	if route == "cancel" {
		h.ClearState(chatID)
		h.handleRefreshList(chatID, msgID)
		return
	}

	h.mu.Lock()
	ip, ok := h.addState[chatID]
	delete(h.addState, chatID)
	h.mu.Unlock()

	if !ok || ip == "" {
		return
	}

	cfg, err := h.deps.Config.LoadVPNConfig()
	if err != nil {
		h.deps.Sender.SendPlain(chatID, fmt.Sprintf("Config load error: %v", err))
		return
	}

	// Normalize IP: strip /32 for consistent storage
	ip = normalizeIP(ip)

	if route == "xray" {
		cfg.Xray.Clients = append(cfg.Xray.Clients, ip)
	} else {
		tunnel := cfg.TunnelDirector.Tunnels[route]
		tunnel.Clients = append(tunnel.Clients, ip)
		if cfg.TunnelDirector.Tunnels == nil {
			cfg.TunnelDirector.Tunnels = make(map[string]vpnconfig.TunnelConfig)
		}
		cfg.TunnelDirector.Tunnels[route] = tunnel
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
```

Also add the IP validation and normalization functions (to avoid importing wizard package):

```go
func isValidIPOrCIDR(s string) bool {
	if strings.Contains(s, "/") {
		ip, _, err := net.ParseCIDR(s)
		if err != nil {
			return false
		}
		return ip.To4() != nil
	}
	ip := net.ParseIP(s)
	return ip != nil && ip.To4() != nil
}

// normalizeIP strips /32 suffix from single-host addresses for consistent storage.
func normalizeIP(ip string) string {
	return strings.TrimSuffix(ip, "/32")
}
```

Add `"net"` to imports.

Also update the `HandleCallback` switch to handle `route:cancel`:

The existing `case strings.HasPrefix(action, "route:")` already handles this — `handleAddRoute` checks for "cancel" route.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd telegram-bot && go test -run "TestClientsHandler" ./internal/handler/ -v`

Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add telegram-bot/internal/handler/clients.go telegram-bot/internal/handler/clients_test.go
git commit -m "feat(telegram-bot): add client add flow with IP input and route selection"
```

---

### Task 8: Router integration and command registration

**Files:**
- Modify: `telegram-bot/internal/bot/router.go`
- Modify: `telegram-bot/internal/bot/router_test.go`
- Modify: `telegram-bot/internal/bot/bot.go`

- [ ] **Step 1: Write router test for /clients dispatch**

Add to `telegram-bot/internal/bot/router_test.go`:

First, add a mock for the clients handler. Follow the existing pattern of mock handlers in this test file:

```go
type mockClientsHandler struct {
	clientsCalled    bool
	callbackCalled   bool
	textInputCalled  bool
	clearStateCalled bool
}

func (m *mockClientsHandler) HandleClients(msg *tgbotapi.Message)        { m.clientsCalled = true }
func (m *mockClientsHandler) HandleCallback(cb *tgbotapi.CallbackQuery)  { m.callbackCalled = true }
func (m *mockClientsHandler) HandleTextInput(msg *tgbotapi.Message)      { m.textInputCalled = true }
func (m *mockClientsHandler) ClearState(chatID int64)                    { m.clearStateCalled = true }
```

Then add the tests:

```go
func TestRouter_RouteMessage_Clients(t *testing.T) {
	h := &mockClientsHandler{}
	router := &Router{clients: h}
	router.RouteMessage(msgWithCommand("/clients"))

	if !h.clientsCalled {
		t.Error("expected HandleClients to be called")
	}
}

func TestRouter_RouteCallback_Clients(t *testing.T) {
	h := &mockClientsHandler{}
	router := &Router{clients: h}
	cb := &tgbotapi.CallbackQuery{
		Data:    "clients:pause:192.168.50.10",
		Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}},
	}
	router.RouteCallback(cb)

	if !h.callbackCalled {
		t.Error("expected HandleCallback to be called")
	}
}

func TestRouter_RouteMessage_Clients_ClearsOtherStates(t *testing.T) {
	clients := &mockClientsHandler{}
	exclude := &mockExcludeHandler{}
	wizard := &mockWizardHandler{}
	router := &Router{clients: clients, exclude: exclude, wizard: wizard}
	router.RouteMessage(msgWithCommand("/clients"))

	if !exclude.clearStateCalled {
		t.Error("expected exclude ClearState to be called")
	}
	if !wizard.clearStateCalled {
		t.Error("expected wizard ClearState to be called")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd telegram-bot && go test -run "TestRouter_Route.*Clients" ./internal/bot/ -v`

Expected: FAIL — `ClientsRouterHandler` and `clients` field don't exist.

- [ ] **Step 3: Add ClientsRouterHandler interface and routing**

Modify `telegram-bot/internal/bot/router.go`:

Add the interface after `ExcludeRouterHandler`:

```go
// ClientsRouterHandler defines methods for clients command
type ClientsRouterHandler interface {
	HandleClients(msg *tgbotapi.Message)
	ClearState(chatID int64)
	HandleCallback(cb *tgbotapi.CallbackQuery)
	HandleTextInput(msg *tgbotapi.Message)
}
```

Add `clients` field to the Router struct:

```go
type Router struct {
	status  StatusRouterHandler
	servers ServersRouterHandler
	import_ ImportRouterHandler
	misc    MiscRouterHandler
	update  UpdateRouterHandler
	wizard  WizardRouterHandler
	xray    XrayRouterHandler
	exclude ExcludeRouterHandler
	clients ClientsRouterHandler
}
```

Update `NewRouter` to accept the new handler:

```go
func NewRouter(
	status StatusRouterHandler,
	servers ServersRouterHandler,
	import_ ImportRouterHandler,
	misc MiscRouterHandler,
	update UpdateRouterHandler,
	wizard WizardRouterHandler,
	xray XrayRouterHandler,
	exclude ExcludeRouterHandler,
	clients ClientsRouterHandler,
) *Router {
	return &Router{
		status:  status,
		servers: servers,
		import_: import_,
		misc:    misc,
		update:  update,
		wizard:  wizard,
		xray:    xray,
		exclude: exclude,
		clients: clients,
	}
}
```

Add to `RouteMessage`:

```go
	case "clients":
		r.exclude.ClearState(msg.Chat.ID)
		r.wizard.ClearState(msg.Chat.ID)
		r.clients.HandleClients(msg)
```

Update `configure` and `exclude` cases to also clear clients state:

```go
	case "configure":
		r.exclude.ClearState(msg.Chat.ID)
		r.clients.ClearState(msg.Chat.ID)
		r.wizard.Start(msg.Chat.ID)
	case "exclude":
		r.wizard.ClearState(msg.Chat.ID)
		r.clients.ClearState(msg.Chat.ID)
		r.exclude.HandleExclude(msg)
```

Add to `default` text handler:

```go
	default:
		r.clients.HandleTextInput(msg)
		r.exclude.HandleTextInput(msg)
		r.wizard.HandleTextInput(msg)
```

Add to `RouteCallback`, before the wizard fallthrough:

```go
	if strings.HasPrefix(cb.Data, "clients:") {
		r.clients.HandleCallback(cb)
		return
	}
```

- [ ] **Step 4: Fix existing router tests that call NewRouter**

Update all existing `NewRouter` calls in `router_test.go` to pass the new `clients` parameter. Most tests create a `Router` struct directly (not via `NewRouter`), so this may only affect a few places. Add a `clients` mock where needed, or pass `nil` for tests that don't test clients routing.

Search the test file for `NewRouter(` calls and add the `clients` parameter. Also update any direct `Router{...}` struct literals to include `clients` field if the test exercises RouteMessage default case or RouteCallback.

- [ ] **Step 5: Update bot.go — create ClientsHandler and pass to NewRouter**

In `telegram-bot/internal/bot/bot.go`, add the handler import and initialization.

After `excludeHandler := handler.NewExcludeHandler(deps)`:

```go
clientsHandler := handler.NewClientsHandler(deps)
```

Update the `NewRouter` call to include `clientsHandler`:

```go
router := NewRouter(statusHandler, serversHandler, importHandler, miscHandler, updateHandler, wizardHandler, xrayHandler, excludeHandler, clientsHandler)
```

Add `/clients` to `RegisterCommands`:

```go
{Command: "clients", Description: "Manage VPN clients"},
```

- [ ] **Step 6: Run router tests**

Run: `cd telegram-bot && go test ./internal/bot/ -v`

Expected: All PASS

- [ ] **Step 7: Run all bot tests to check nothing broke**

Run: `cd telegram-bot && go test ./... -v`

Expected: All PASS

- [ ] **Step 8: Commit**

```bash
git add telegram-bot/internal/bot/router.go telegram-bot/internal/bot/router_test.go telegram-bot/internal/bot/bot.go
git commit -m "feat(telegram-bot): integrate ClientsHandler into router and bot"
```

---

### Task 9: Update dev fixture and final verification

**Files:**
- Modify: `telegram-bot/testdata/dev/vpn-director.json`

- [ ] **Step 1: Update dev fixture**

Add `paused_clients` field to `telegram-bot/testdata/dev/vpn-director.json`. Read the current file first, then add `"paused_clients": []` at the top level (after `data_dir`).

- [ ] **Step 2: Run all Go tests**

Run: `cd telegram-bot && go test ./...`

Expected: All PASS

- [ ] **Step 3: Run all shell tests**

Run: `bats router/test/`

Expected: All PASS (or same as before)

- [ ] **Step 4: Commit**

```bash
git add telegram-bot/testdata/dev/vpn-director.json
git commit -m "chore: add paused_clients to dev fixture"
```
