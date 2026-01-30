---
paths: "telegram-bot/**/*"
---

# Telegram Bot

Go-based Telegram bot for remote VPN Director management.

## Architecture

```
telegram-bot/
├── cmd/bot/main.go           # Entry point, signal handling, DI setup
├── internal/
│   ├── bot/                  # Core bot orchestration
│   │   ├── bot.go            # Bot struct, Run(), message dispatch
│   │   └── auth.go           # Username-based authorization
│   ├── config/               # Configuration
│   │   └── config.go         # Bot config (token, users, log_level)
│   ├── devmode/              # Development mode
│   │   └── executor.go       # Mock executor for safe dev testing
│   ├── handler/              # Command handlers
│   │   ├── handler.go        # Deps struct, handler registration
│   │   ├── misc.go           # /start, /version, /ip, /logs
│   │   ├── status.go         # /status, /restart, /stop
│   │   ├── servers.go        # /servers
│   │   ├── import.go         # /import
│   │   ├── update.go         # /update (self-update from GitHub)
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
│   ├── updater/              # Self-update logic
│   │   ├── updater.go        # GitHub API, lock file, downloads
│   │   ├── release.go        # Release fetching
│   │   ├── download.go       # Asset downloading
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
| `/servers` | `ServersHandler.HandleServers` | Server list (paginated) |
| `/import <url>` | `ImportHandler.HandleImport` | Import VLESS subscription |
| `/configure` | `WizardHandler.HandleConfigure` | Configuration wizard |
| `/restart` | `StatusHandler.HandleRestart` | Restart VPN Director |
| `/stop` | `StatusHandler.HandleStop` | Stop VPN Director |
| `/logs [bot\|vpn\|all] [N]` | `MiscHandler.HandleLogs` | Recent logs (default: all, 20 lines) |
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

Downloads latest release from GitHub and applies it:

1. Checks for newer version via GitHub API
2. Creates lock file (`/tmp/vpn-director-update/lock`)
3. Downloads release assets to `/tmp/vpn-director-update/files/`
4. Generates and runs `update.sh` script
5. Script copies files, restarts bot, sends notification

**Dev mode**: Command disabled when `DEV=true` environment variable is set.

## Config File

`/opt/vpn-director/telegram-bot.json`:
```json
{
  "bot_token": "123456:ABC...",
  "allowed_users": ["username1", "username2"],
  "log_level": "info"
}
```

**Log levels**: `debug`, `info`, `warn`, `error` (default: `info`)

Setup: `./setup_telegram_bot.sh`

## Build

```bash
cd telegram-bot

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
cd telegram-bot && go test ./...

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

**Dev mode**: `DEV=true` enables mock executor (safe commands only, blocks destructive ops)

## Dependencies

- `github.com/go-telegram-bot-api/telegram-bot-api/v5` — Telegram API client

## Logging

Uses Go's `log/slog` package:

- Output: stdout + `/tmp/telegram-bot.log` (via `io.MultiWriter`)
- Format: `time=2026-01-30T15:04:05.000+03:00 level=INFO source=main.go:42 msg="Bot started"`
- Levels: `DEBUG`, `INFO`, `WARN`, `ERROR` (configurable via `log_level` in config)
- Rotation: Log file truncated at 200KB (checked every minute)

**Runtime level change**: Call `logger.SetLevel("debug")` to adjust without restart
