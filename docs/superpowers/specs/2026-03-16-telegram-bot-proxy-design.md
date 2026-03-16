# Telegram Bot Proxy Support

Route Telegram bot traffic through VPN (Xray SOCKS5) when Telegram is blocked on router's WAN.

## Problem

The Telegram bot runs on the router as a Go binary, connecting outbound to `api.telegram.org:443` via long polling. All router-generated traffic (OUTPUT chain) bypasses VPN routing (which only handles LAN client traffic via PREROUTING). If the ISP blocks Telegram, the bot cannot connect.

## Solution

Add SOCKS5 proxy support to the Telegram bot, using Xray's new SOCKS5 inbound as the proxy endpoint. Configurable fallback to direct connection when proxy is unavailable.

## Design

### 1. Xray SOCKS5 Inbound

Add a second inbound to `config.json.template` — SOCKS5 on loopback:

```json
{
  "port": 12346,
  "listen": "127.0.0.1",
  "protocol": "socks",
  "settings": { "udp": true },
  "tag": "socks-in"
}
```

Update routing rule to include both inbounds:

```json
"inboundTag": ["tproxy-in", "socks-in"]
```

Add `socks_port` to `vpn-director.json.template` in `advanced.xray` section:

```json
"xray": {
  "tproxy_port": 12345,
  "socks_port": 12346
}
```

Port is used during Xray config generation, same as `tproxy_port`.

### 2. Telegram Bot Configuration

Two new fields in `telegram-bot.json`:

```json
{
  "bot_token": "...",
  "allowed_users": ["..."],
  "proxy": "socks5://127.0.0.1:12346",
  "proxy_fallback_direct": true,
  "log_level": "info"
}
```

- `proxy` — SOCKS5 proxy URL. Empty or absent = direct connection (backward compatible).
- `proxy_fallback_direct` — if `true`, bot falls back to direct connection when proxy is unreachable. Default `false` (strict mode).

Format `socks5://host:port` is standard for Go, supported by `golang.org/x/net/proxy`. User can point to any SOCKS/HTTP proxy, not necessarily Xray.

### 3. Go Code Changes

**Config** (`internal/config/config.go`):

Add fields to both `rawConfig` (for JSON unmarshaling) and `Config` structs. Copy in `Load()`:

```go
// in rawConfig:
Proxy               string `json:"proxy"`
ProxyFallbackDirect bool   `json:"proxy_fallback_direct"`

// in Config:
Proxy               string
ProxyFallbackDirect bool
```

This follows the existing two-struct pattern where `rawConfig` handles JSON deserialization and `Config` is the application-facing struct.

**HTTP transport** (new file `internal/bot/transport.go`):

- Parse `config.Proxy` as URL
- Create `http.Transport` with SOCKS5 dialer via `golang.org/x/net/proxy`
- If `proxy_fallback_direct == true`: custom `http.RoundTripper` that tries proxy transport first, falls back to direct on connection errors (refused, timeout)
- Log via slog: "using proxy ...", "proxy failed, falling back to direct"

**Bot init** (`internal/bot/bot.go`):

- Build proxy-aware `http.Client` **before** creating tgbotapi instance
- Use `tgbotapi.NewBotAPIWithClient(token, tgbotapi.APIEndpoint, httpClient)` instead of `tgbotapi.NewBotAPI(token)`
- This is critical: `NewBotAPI` calls `GetMe()` during construction, which makes a network request. If Telegram is blocked and proxy isn't set yet, it will fail before proxy is ever applied.

**Startup retry logic** (`cmd/bot/main.go`):

- Wrap `bot.New()` in a retry loop with exponential backoff (5s, 10s, 20s, 40s, 60s, then 60s intervals)
- Handles the boot order race: bot (S98) starts before Xray (S99). When proxy is configured but Xray isn't running yet, `NewBotAPIWithClient` fails. Retry loop waits until Xray is up.
- Log each retry attempt: "failed to connect to Telegram API, retrying in Ns..."
- No max retry limit — bot keeps trying indefinitely (it's a long-running daemon)

**Dependency**: add `golang.org/x/net` to `go.mod` (for `proxy.SOCKS5`).

No changes to bot command logic — commands, wizard work through the same `http.Client`.

**Updater/UpdateChecker**: these create their own `http.Client` for GitHub API requests (`api.github.com`, `github.com`). Proxy is NOT propagated to them — they always connect directly. This is intentional: GitHub is rarely blocked when Telegram is, and the updater is non-critical. If needed in the future, the proxy-aware client can be passed to `updater.New()`.

### 4. Setup Script and Init Script

**`setup_telegram_bot.sh`** — ask during bot configuration:

```
Use proxy for Telegram API? (recommended if Telegram is blocked)
  1) Yes - use Xray SOCKS5 proxy (socks5://127.0.0.1:12346)
  2) No - direct connection
```

If "Yes" — write `proxy` and `proxy_fallback_direct: true` to `telegram-bot.json`. Port taken from `vpn-director.json` (`advanced.xray.socks_port`) if available, else default 12346.

**`S98telegram-bot`** — no changes to init script itself. Boot order S98 (bot) -> S99 (vpn-director) means bot starts before Xray. The startup retry logic in `main.go` (see section 3) handles this race: bot retries connecting until Xray is up and proxy is available.

**No changes to VPN Director shell scripts** (tproxy.sh, tunnel.sh, etc.) — SOCKS5 inbound in Xray config works independently of iptables rules.

## Files Changed

| File | Change |
|------|--------|
| `router/opt/etc/xray/config.json.template` | Add SOCKS5 inbound, update routing rule |
| `router/opt/vpn-director/vpn-director.json.template` | Add `socks_port` field |
| `telegram-bot/internal/config/config.go` | Add `Proxy`, `ProxyFallbackDirect` fields |
| `telegram-bot/internal/bot/transport.go` | New file: proxy transport with fallback |
| `telegram-bot/internal/bot/bot.go` | Use custom HTTP client |
| `telegram-bot/go.mod` / `go.sum` | Add `golang.org/x/net` dependency |
| `router/opt/vpn-director/setup_telegram_bot.sh` | Add proxy configuration prompt |
| `telegram-bot/testdata/dev/xray.template.json` | Mirror SOCKS5 inbound for tests |

## Known Limitations

- **Updater bypasses proxy**: `/update` command and automatic update checker connect to GitHub directly, not through proxy. If GitHub is also blocked by ISP, these features won't work. Can be addressed later by propagating proxy client to `updater.New()`.
- **Boot order race**: Bot starts (S98) before Xray (S99). Handled by retry logic, but first successful connection may take up to ~60s after boot.

## Out of Scope

- Routing other router-generated traffic through VPN (DNS, updates, etc.)
- iptables OUTPUT chain modifications
- OpenVPN/WireGuard proxy support (user can point `proxy` to any external SOCKS proxy manually)
- Propagating proxy to updater/update-checker HTTP clients
