---
paths: "server/**/*"
---

# Telegram Bot

Go-based Telegram bot for remote VPN Director management.

## Architecture

```
server/
├── cmd/bot/main.go           # Entry point, signal handling, DI setup
├── internal/
│   ├── bot/                  # Core bot orchestration
│   │   ├── bot.go            # Bot struct, Run(), message dispatch
│   │   ├── router.go         # Command and callback routing
│   │   └── auth.go           # Username-based authorization
│   ├── chatstore/            # Chat ID persistence
│   │   └── store.go          # Thread-safe chat storage for notifications
│   ├── config/               # Configuration
│   │   └── config.go         # Bot config (token, users, log_level, update_check_interval)
│   ├── devmode/              # Development mode
│   │   └── executor.go       # Mock executor for safe dev testing
│   ├── handler/              # Command handlers
│   │   ├── handler.go        # Deps struct, handler registration
│   │   ├── misc.go           # /start, /version, /ip, /logs
│   │   ├── status.go         # /status, /restart, /stop
│   │   ├── servers.go        # /servers
│   │   ├── import.go         # /import
│   │   ├── update.go         # /update (self-update from GitHub)
│   │   ├── xray.go           # /xray (quick server switch)
│   │   └── wizard_*.go       # /configure wizard handlers
│   ├── logging/              # Logging
│   │   ├── logger.go         # slog setup (stdout + file)
│   │   └── rotation.go       # Log file rotation (200KB)
│   ├── paths/                # Path constants
│   │   └── paths.go          # BotLogPath, VPNLogPath, etc.
│   ├── service/              # Business logic interfaces
│   │   └── interfaces.go     # ShellExecutor, Network, etc.
│   ├── shell/                # Shell command execution
│   │   └── shell.go          # Real command executor
│   ├── startup/              # Startup notifications
│   │   └── notify.go         # Post-update notification
│   ├── telegram/             # Telegram API helpers
│   │   └── sender.go         # Message sending, escaping
│   ├── updatechecker/        # Automatic update notifications
│   │   └── checker.go        # Background goroutine, per-user tracking
│   ├── updateflow/           # Update orchestration shared by the bot and the Web UI
│   │   ├── flow.go           # Check with a 30-minute cache, typed errors
│   │   └── start.go          # Pre-flight, lock, background download and script
│   ├── updater/              # Self-update logic
│   │   ├── updater.go        # Daemon table (asset names, binaries, init scripts), GitHub API, lock file
│   │   ├── github.go         # GitHub release fetching
│   │   ├── downloader.go     # Asset downloading
│   │   └── script.go         # Update script generation
│   ├── vless/                # VLESS protocol
│   │   └── parser.go         # VLESS URL parser, subscription decoder
│   ├── vpnconfig/            # VPN Director config
│   │   └── vpnconfig.go      # vpn-director.json, servers.json
│   └── wizard/               # Configuration wizard
│       ├── state.go          # Thread-safe state storage
│       └── wizard.go         # Wizard manager
└── Makefile                  # Build targets
```

## Commands

| Command | Handler | Description |
|---------|---------|-------------|
| `/start` | `MiscHandler.HandleStart` | Show help |
| `/status` | `StatusHandler.HandleStatus` | VPN Director status |
| `/xray` | `XrayHandler.HandleXray` | Quick server switch |
| `/servers` | `ServersHandler.HandleServers` | Server list (paginated) |
| `/import <url>` | `ImportHandler.HandleImport` | Import VLESS subscription |
| `/configure` | `WizardHandler.HandleConfigure` | Configuration wizard |
| `/restart` | `StatusHandler.HandleRestart` | Restart VPN Director |
| `/stop` | `StatusHandler.HandleStop` | Stop VPN Director |
| `/logs [bot\|vpn\|xray\|webui\|all] [N]` | `MiscHandler.HandleLogs` | Recent logs (default: all, 20 lines) |
| `/ip` | `MiscHandler.HandleIP` | External IP |
| `/update` | `UpdateHandler.HandleUpdate` | Self-update to latest GitHub release |
| `/version` | `MiscHandler.HandleVersion` | Bot version |

## Configuration Wizard

4-step inline keyboard wizard:

1. **Server Selection** — choose Xray server from servers.json
2. **Exclusions** — select country sets to exclude (user-configurable)
3. **Clients** — add LAN clients with route (xray/ovpnc1-5/wgc1-5)
4. **Confirm** — review and apply

On apply:
- Updates vpn-director.json (clients, exclusions, rules)
- Generates /opt/etc/xray/config.json from template
- Runs `vpn-director.sh update`
- Restarts Xray

## Self-Update (`/update`)

Both daemons — `telegram-bot` and `webui` — are updated together, from the bot or from the Web UI. The orchestration lives in `internal/updateflow`; the bot command and the Web UI handlers are adapters over it.

1. `Flow.Check` asks the GitHub API for the latest release (result cached for 30 minutes; a forced check pierces the cache at most once a minute)
2. `Flow.Start` creates the lock file (`/tmp/vpn-director-update/lock`)
3. Release assets go to `/tmp/vpn-director-update/files/`: every script from `scriptFiles` plus one binary per daemon (`telegram-bot-<arch>`, `webui-<arch>`). A release missing either binary is a download error.
4. `update.sh` is generated from the daemon table and run detached
5. The script remembers which daemons were running, stops them, copies everything, writes `notify.json` and starts back exactly those daemons
6. On failure an `EXIT` trap restarts the daemons that were running and writes `notify.json` with `"status": "failed"`

`notify.json`:

```json
{"chat_id": 0, "old_version": "v1.2.0", "new_version": "v1.3.0",
 "status": "ok", "initiator": "webui"}
```

`chat_id` 0 marks an update started from the Web UI: on its next start the bot notifies every active chat. A successful notification clears `/tmp/vpn-director-update`; a failed one keeps `update.log`, because the message points at it.

**Dev mode**: `/update` is disabled with `--dev` and for a `dev` build.

## Config File

`/opt/vpn-director/telegram-bot.json`:
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

**Fields:**
- `bot_token` — Telegram Bot API token (required)
- `allowed_users` — Array of Telegram usernames (required)
- `proxy` — SOCKS5 proxy URL for Telegram API (optional, empty = direct connection)
- `proxy_fallback_direct` — Fall back to direct connection if proxy unavailable (default: `false`)
- `log_level` — `debug`, `info`, `warn`, `error` (default: `info`)
- `update_check_interval` — Go duration (`1h`, `30m`, `24h`). If omitted or `"0"`, automatic update checking is disabled

Setup: `./setup_telegram_bot.sh`

## Automatic Update Notifications

When `update_check_interval` is set, the bot periodically checks GitHub for new releases and notifies users.

**How it works:**
1. Bot checks GitHub API at configured interval
2. If new version found, sends notification to all active users
3. Notification includes changelog and "🔄 Обновить" button
4. Each user is notified only once per version

**Data storage:**
- `/opt/vpn-director/data/chats.json` — stores chat IDs and notification history

**User tracking:**
- Chat ID recorded on first message from authorized user
- Users marked inactive if bot is blocked
- Reactivated automatically when user messages bot again

**Disabled in dev mode:** Update checker does not run when `--dev` flag is used or version is "dev".

## Build

```bash
cd server

# Native build
make build

# Cross-compile for router
make build-arm64   # ARM64 routers (AX86U, GT-AX6000, etc.)
make build-arm     # ARMv7 routers (older models)

# Tests
make test

# Run tests with coverage
go test ./... -cover
```

Binary: `bin/telegram-bot-{arch}`

## Test Commands

```bash
# Run all Go tests
cd server && go test ./...

# Run with verbose output
go test -v ./internal/bot/...

# Run specific test
go test -run TestHandleStatus ./internal/bot/
```

## Deployment

1. Build for target architecture
2. Copy binary to `/opt/vpn-director/telegram-bot`
3. Run `./setup_telegram_bot.sh` to create config
4. Bot auto-starts if config exists and token is set

## Key Patterns

**Dependency Injection**: All handlers receive `*handler.Deps` struct with services

**Authorization**: Username whitelist in config, checked in `bot.isAuthorized()`

**Shell execution**: All router commands via `service.ShellExecutor` interface (real or dev mock)

**State management**: Thread-safe wizard state with mutex, per-chat storage

**Graceful shutdown**: Context cancellation on SIGINT/SIGTERM

**Dev mode**: `--dev` enables mock executor (safe commands only, blocks destructive ops)

## Dependencies

- `github.com/go-telegram-bot-api/telegram-bot-api/v5` — Telegram API client
- `golang.org/x/net` — SOCKS5 proxy support

## Logging

Uses Go's `log/slog` package:

- Output: stdout + `/tmp/telegram-bot.log` (via `io.MultiWriter`)
- Format: `time=2026-01-30T15:04:05.000+03:00 level=INFO source=main.go:42 msg="Bot started"`
- Levels: `DEBUG`, `INFO`, `WARN`, `ERROR` (configurable via `log_level` in config)
- Rotation: Log file truncated at 200KB (checked every minute)

**Runtime level change**: Call `logger.SetLevel("debug")` to adjust without restart
