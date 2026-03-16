# Telegram Bot Proxy Support — Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Route Telegram bot traffic through SOCKS5 proxy (Xray) when Telegram is blocked on router WAN.

**Architecture:** Add SOCKS5 inbound to Xray config, add proxy config fields to bot, build custom HTTP transport with fallback logic, wrap bot startup in retry loop for boot-order race.

**Tech Stack:** Go 1.25, `golang.org/x/net/proxy` for SOCKS5, `go-telegram-bot-api/v5`, bash (setup script)

**Spec:** `docs/superpowers/specs/2026-03-16-telegram-bot-proxy-design.md`

---

## Chunk 1: Configuration Layer

### Task 1: Add SOCKS5 inbound to Xray config template

**Files:**
- Modify: `router/opt/etc/xray/config.json.template`
- Modify: `telegram-bot/testdata/dev/xray.template.json`

- [ ] **Step 1: Add SOCKS5 inbound to `config.json.template`**

Add second inbound after the existing dokodemo-door entry:

```json
    {
      "port": 12346,
      "listen": "127.0.0.1",
      "protocol": "socks",
      "settings": {
        "udp": true
      },
      "tag": "socks-in"
    }
```

Update the routing rule `inboundTag` from `["tproxy-in"]` to `["tproxy-in", "socks-in"]`.

- [ ] **Step 2: Mirror the same change in `telegram-bot/testdata/dev/xray.template.json`**

Identical changes as step 1.

- [ ] **Step 3: Commit**

```bash
git add router/opt/etc/xray/config.json.template telegram-bot/testdata/dev/xray.template.json
git commit -m "feat(xray): add SOCKS5 inbound on 127.0.0.1:12346 for local proxy access"
```

---

### Task 2: Add `socks_port` to VPN Director config template

**Files:**
- Modify: `router/opt/vpn-director/vpn-director.json.template`

- [ ] **Step 1: Add `socks_port` field**

In `advanced.xray` section, add `"socks_port": 12346` after `"tproxy_port": 12345`:

```json
"xray": {
  "tproxy_port": 12345,
  "socks_port": 12346,
  ...
}
```

- [ ] **Step 2: Commit**

```bash
git add router/opt/vpn-director/vpn-director.json.template
git commit -m "feat(config): add socks_port to advanced.xray config section"
```

---

### Task 3: Add proxy fields to bot config

**Files:**
- Modify: `telegram-bot/internal/config/config.go`
- Modify: `telegram-bot/internal/config/config_test.go`

- [ ] **Step 1: Write failing tests for proxy config fields**

Add to `config_test.go`:

```go
func TestLoad_WithProxy(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	jsonContent := `{
		"bot_token": "test-token",
		"allowed_users": ["user1"],
		"proxy": "socks5://127.0.0.1:12346",
		"proxy_fallback_direct": true
	}`

	err := os.WriteFile(configPath, []byte(jsonContent), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Proxy != "socks5://127.0.0.1:12346" {
		t.Errorf("expected proxy 'socks5://127.0.0.1:12346', got '%s'", cfg.Proxy)
	}
	if !cfg.ProxyFallbackDirect {
		t.Error("expected proxy_fallback_direct to be true")
	}
}

func TestLoad_ProxyDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	jsonContent := `{
		"bot_token": "test-token",
		"allowed_users": []
	}`

	err := os.WriteFile(configPath, []byte(jsonContent), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Proxy != "" {
		t.Errorf("expected empty proxy, got '%s'", cfg.Proxy)
	}
	if cfg.ProxyFallbackDirect {
		t.Error("expected proxy_fallback_direct to default to false")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd telegram-bot && go test ./internal/config/ -run "TestLoad_WithProxy|TestLoad_ProxyDefaults" -v`
Expected: FAIL — `cfg.Proxy` is empty, field doesn't exist yet.

- [ ] **Step 3: Add proxy fields to rawConfig and Config structs**

In `config.go`, add to `rawConfig`:

```go
type rawConfig struct {
	BotToken            string   `json:"bot_token"`
	AllowedUsers        []string `json:"allowed_users"`
	LogLevel            string   `json:"log_level"`
	UpdateCheckInterval string   `json:"update_check_interval"`
	Proxy               string   `json:"proxy"`
	ProxyFallbackDirect bool     `json:"proxy_fallback_direct"`
}
```

Add to `Config`:

```go
type Config struct {
	BotToken            string        `json:"bot_token"`
	AllowedUsers        []string      `json:"allowed_users"`
	LogLevel            string        `json:"log_level"`
	UpdateCheckInterval time.Duration `json:"-"`
	Proxy               string
	ProxyFallbackDirect bool
}
```

Add copy lines in `Load()` after existing field copies:

```go
cfg := &Config{
	BotToken:            raw.BotToken,
	AllowedUsers:        raw.AllowedUsers,
	LogLevel:            raw.LogLevel,
	Proxy:               raw.Proxy,
	ProxyFallbackDirect: raw.ProxyFallbackDirect,
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd telegram-bot && go test ./internal/config/ -v`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
git add telegram-bot/internal/config/config.go telegram-bot/internal/config/config_test.go
git commit -m "feat(config): add proxy and proxy_fallback_direct fields"
```

---

## Chunk 2: Proxy Transport

### Task 4: Create proxy transport with fallback

**Files:**
- Create: `telegram-bot/internal/bot/transport.go`
- Create: `telegram-bot/internal/bot/transport_test.go`
- Modify: `telegram-bot/go.mod` (add `golang.org/x/net`)

- [ ] **Step 1: Add `golang.org/x/net` dependency**

Run: `cd telegram-bot && go get golang.org/x/net`

- [ ] **Step 2: Write failing tests for `NewHTTPClient`**

Create `telegram-bot/internal/bot/transport_test.go`:

```go
package bot

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewHTTPClient_NoProxy(t *testing.T) {
	client, err := NewHTTPClient("", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	// Should be default client behavior (no custom transport)
	if client.Transport != nil {
		t.Error("expected nil transport for no-proxy client")
	}
}

func TestNewHTTPClient_InvalidProxyURL(t *testing.T) {
	_, err := NewHTTPClient("not-a-url://bad", false)
	if err == nil {
		t.Fatal("expected error for invalid proxy URL")
	}
}

func TestNewHTTPClient_ValidSOCKS5(t *testing.T) {
	// Just verify it creates a client without error.
	// Actual SOCKS5 connectivity is an integration concern.
	client, err := NewHTTPClient("socks5://127.0.0.1:12346", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.Transport == nil {
		t.Error("expected custom transport for proxy client")
	}
}

func TestNewHTTPClient_FallbackDirect(t *testing.T) {
	// With fallback enabled, should create a fallback transport
	client, err := NewHTTPClient("socks5://127.0.0.1:19999", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.Transport == nil {
		t.Fatal("expected custom transport")
	}

	// Verify fallback works: proxy is not running, so proxy attempt fails,
	// but with fallback it should reach the test server directly
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	resp, err := client.Get(ts.URL)
	if err != nil {
		t.Fatalf("expected fallback to direct connection, got error: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestNewHTTPClient_NoFallback_Fails(t *testing.T) {
	// Without fallback, connecting through dead proxy should fail
	client, err := NewHTTPClient("socks5://127.0.0.1:19999", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	_, err = client.Get(ts.URL)
	if err == nil {
		t.Fatal("expected error when proxy is down and fallback is disabled")
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd telegram-bot && go test ./internal/bot/ -run "TestNewHTTPClient" -v`
Expected: FAIL — `NewHTTPClient` not defined.

- [ ] **Step 4: Implement `NewHTTPClient` in `transport.go`**

Create `telegram-bot/internal/bot/transport.go`:

```go
package bot

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"

	"golang.org/x/net/proxy"
)

// NewHTTPClient creates an HTTP client optionally configured with a SOCKS5 proxy.
// If proxyURL is empty, returns a default http.Client.
// If fallbackDirect is true, falls back to direct connection on proxy errors.
func NewHTTPClient(proxyURL string, fallbackDirect bool) (*http.Client, error) {
	if proxyURL == "" {
		return &http.Client{}, nil
	}

	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy URL %q: %w", proxyURL, err)
	}

	if u.Scheme != "socks5" {
		return nil, fmt.Errorf("unsupported proxy scheme %q, only socks5 is supported", u.Scheme)
	}

	dialer, err := proxy.SOCKS5("tcp", u.Host, nil, proxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("failed to create SOCKS5 dialer: %w", err)
	}

	proxyTransport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.(proxy.ContextDialer).DialContext(ctx, network, addr)
		},
	}

	if !fallbackDirect {
		slog.Info("Using proxy for Telegram API", "proxy", proxyURL)
		return &http.Client{Transport: proxyTransport}, nil
	}

	slog.Info("Using proxy for Telegram API with direct fallback", "proxy", proxyURL)
	return &http.Client{
		Transport: &fallbackTransport{
			primary:  proxyTransport,
			fallback: http.DefaultTransport,
			proxyURL: proxyURL,
		},
	}, nil
}

// fallbackTransport tries the primary transport first, falls back on connection errors.
type fallbackTransport struct {
	primary  http.RoundTripper
	fallback http.RoundTripper
	proxyURL string
}

func (t *fallbackTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.primary.RoundTrip(req)
	if err == nil {
		return resp, nil
	}

	slog.Warn("Proxy connection failed, falling back to direct",
		"proxy", t.proxyURL, "error", err)
	return t.fallback.RoundTrip(req)
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd telegram-bot && go test ./internal/bot/ -run "TestNewHTTPClient" -v`
Expected: ALL PASS

- [ ] **Step 6: Commit**

```bash
git add telegram-bot/internal/bot/transport.go telegram-bot/internal/bot/transport_test.go telegram-bot/go.mod telegram-bot/go.sum
git commit -m "feat(bot): add SOCKS5 proxy transport with direct fallback"
```

---

## Chunk 3: Bot Integration and Startup Retry

### Task 5: Use proxy-aware HTTP client in bot.New()

**Files:**
- Modify: `telegram-bot/internal/bot/bot.go`

- [ ] **Step 1: Modify `bot.New()` to build HTTP client and use `NewBotAPIWithClient`**

In `bot.go`, change the `New` function. Replace lines 60-63:

```go
// Before:
api, err := tgbotapi.NewBotAPI(cfg.BotToken)
if err != nil {
    return nil, err
}
```

With:

```go
httpClient, err := NewHTTPClient(cfg.Proxy, cfg.ProxyFallbackDirect)
if err != nil {
    return nil, fmt.Errorf("failed to create HTTP client: %w", err)
}
api, err := tgbotapi.NewBotAPIWithClient(cfg.BotToken, tgbotapi.APIEndpoint, httpClient)
if err != nil {
    return nil, err
}
```

Add `"fmt"` to the imports.

- [ ] **Step 2: Run all bot tests to verify nothing breaks**

Run: `cd telegram-bot && go test ./internal/bot/ -v`
Expected: ALL PASS (existing tests should still work — they don't call `New()` with real API).

- [ ] **Step 3: Commit**

```bash
git add telegram-bot/internal/bot/bot.go
git commit -m "feat(bot): use proxy-aware HTTP client via NewBotAPIWithClient"
```

---

### Task 6: Add startup retry logic in main.go

**Files:**
- Modify: `telegram-bot/cmd/bot/main.go`

- [ ] **Step 1: Add retry loop around `bot.New()` call**

Replace lines 113-117 in `main.go`:

```go
// Before:
b, err := bot.New(cfg, p, Version, VersionFull, Commit, BuildDate, opts...)
if err != nil {
    slog.Error("Failed to create bot", "error", err)
    os.Exit(1)
}
```

With:

```go
var b *bot.Bot
{
    backoffs := []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second, 40 * time.Second, 60 * time.Second}
    for attempt := 0; ; attempt++ {
        b, err = bot.New(cfg, p, Version, VersionFull, Commit, BuildDate, opts...)
        if err == nil {
            break
        }
        delay := backoffs[len(backoffs)-1]
        if attempt < len(backoffs) {
            delay = backoffs[attempt]
        }
        slog.Warn("Failed to connect to Telegram API, retrying...",
            "error", err, "attempt", attempt+1, "retry_in", delay)
        select {
        case <-ctx.Done():
            slog.Error("Shutdown requested during startup retry", "error", err)
            os.Exit(1)
        case <-time.After(delay):
        }
    }
}
```

Move the `ctx` creation (lines 101-102) to before the retry loop — it's already there, so no change needed.

- [ ] **Step 2: Verify build compiles**

Run: `cd telegram-bot && go build ./cmd/bot/`
Expected: compiles without errors.

- [ ] **Step 3: Commit**

```bash
git add telegram-bot/cmd/bot/main.go
git commit -m "feat(bot): add startup retry with exponential backoff for proxy boot race"
```

---

## Chunk 4: Setup Script and Final Verification

### Task 7: Add proxy prompt to setup script

**Files:**
- Modify: `router/opt/vpn-director/setup_telegram_bot.sh`

- [ ] **Step 1: Add proxy configuration prompt**

After the users loop (after line 47 `fi`) and before the "Create JSON" comment (line 49), add:

```bash
# Proxy configuration
echo
printf "Use proxy for Telegram API? (recommended if Telegram is blocked)\n"
printf "  1) Yes - use Xray SOCKS5 proxy (socks5://127.0.0.1:12346)\n"
printf "  2) No - direct connection\n"
printf "Choice [2]: "
read -r PROXY_CHOICE < /dev/tty

PROXY_URL=""
PROXY_FALLBACK=false
if [[ "$PROXY_CHOICE" == "1" ]]; then
    # Try to read socks_port from vpn-director.json
    SOCKS_PORT=12346
    if [[ -f "$VPD_DIR/vpn-director.json" ]] && command -v jq &>/dev/null; then
        CONFIGURED_PORT=$(jq -r '.advanced.xray.socks_port // empty' "$VPD_DIR/vpn-director.json" 2>/dev/null)
        if [[ -n "$CONFIGURED_PORT" ]]; then
            SOCKS_PORT="$CONFIGURED_PORT"
        fi
    fi
    PROXY_URL="socks5://127.0.0.1:${SOCKS_PORT}"
    PROXY_FALLBACK=true
    echo "Proxy: $PROXY_URL (with direct fallback)"
fi
```

Update the jq command that creates the JSON (replace line 52-55):

```bash
jq -n \
    --arg token "$BOT_TOKEN" \
    --argjson users "$USERS_JSON" \
    --arg proxy "$PROXY_URL" \
    --argjson fallback "$PROXY_FALLBACK" \
    '{bot_token: $token, allowed_users: $users, log_level: "info"} +
     (if $proxy != "" then {proxy: $proxy, proxy_fallback_direct: $fallback} else {} end)' > "$CONFIG_FILE"
```

- [ ] **Step 2: Commit**

```bash
git add router/opt/vpn-director/setup_telegram_bot.sh
git commit -m "feat(setup): add proxy configuration prompt to Telegram bot setup"
```

---

### Task 8: Run full test suite

- [ ] **Step 1: Run all Go tests**

Run: `cd telegram-bot && go test ./... -v`
Expected: ALL PASS

- [ ] **Step 2: Run shell tests (if applicable)**

Run: `cd router && bats test/` (if bats is installed)
Expected: ALL PASS (no shell changes that affect existing tests)

- [ ] **Step 3: Final commit if any fixes needed**

---

### Task 9: Update CLAUDE.md rules for new config field

**Files:**
- Modify: `.claude/rules/telegram-bot.md`

- [ ] **Step 1: Update Config File section in `.claude/rules/telegram-bot.md`**

Update the config example to include new fields:

```json
{
  "bot_token": "123456:ABC...",
  "allowed_users": ["username1", "username2"],
  "proxy": "socks5://127.0.0.1:12346",
  "proxy_fallback_direct": true,
  "log_level": "info",
  "update_check_interval": "1h"
}
```

Add to **Fields** list:
- `proxy` — SOCKS5 proxy URL for Telegram API (optional, empty = direct connection)
- `proxy_fallback_direct` — Fall back to direct connection if proxy unavailable (default: `false`)

Update **Dependencies** section:
- `golang.org/x/net` — SOCKS5 proxy support

- [ ] **Step 2: Commit**

```bash
git add .claude/rules/telegram-bot.md
git commit -m "docs: update telegram-bot rules with proxy config fields"
```
