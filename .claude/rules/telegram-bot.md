---
paths: "telegram-bot/**/*"
---

# Telegram Bot

Go-based Telegram bot for remote VPN Director management.

## Architecture

```
telegram-bot/
├── cmd/bot/main.go           # Entry point, signal handling
├── internal/
│   ├── bot/                  # Core bot logic
│   │   ├── bot.go            # Bot struct, Run(), message dispatch
│   │   ├── auth.go           # Username-based authorization
│   │   ├── handlers.go       # Command handlers (/status, /restart, etc.)
│   │   └── wizard_handlers.go # Configuration wizard (4-step)
│   ├── config/config.go      # Bot config (token, allowed users)
│   ├── shell/shell.go        # Command execution wrapper
│   ├── vless/parser.go       # VLESS URL parser, subscription decoder
│   ├── vpnconfig/vpnconfig.go # vpn-director.json, servers.json handlers
│   └── wizard/               # Wizard state machine
│       ├── state.go          # Thread-safe state storage
│       └── wizard.go         # Wizard manager
└── Makefile                  # Build targets
```

## Commands

| Command | Handler | Description |
|---------|---------|-------------|
| `/start` | `handleStart` | Show help |
| `/status` | `handleStatus` | Xray status via vpn-director.sh |
| `/servers` | `handleServers` | List imported servers |
| `/import <url>` | `handleImport` | Import VLESS subscription |
| `/configure` | `handleConfigure` | Start config wizard |
| `/restart` | `handleRestart` | Restart Xray |
| `/stop` | `handleStop` | Stop Xray |
| `/logs [bot\|vpn\|all] [N]` | `handleLogs` | Show logs (default: all, 20 lines) |
| `/ip` | `handleIP` | Show external IP |

## Configuration Wizard

4-step inline keyboard wizard:

1. **Server Selection** — choose Xray server from servers.json
2. **Exclusions** — select country sets to exclude (ru default)
3. **Clients** — add LAN clients with route (xray/ovpnc1-5/wgc1-5)
4. **Confirm** — review and apply

On apply:
- Updates vpn-director.json (clients, exclusions, rules)
- Generates /opt/etc/xray/config.json from template
- Runs `vpn-director.sh update`
- Restarts Xray

## Config File

`/jffs/scripts/vpn-director/telegram-bot.json`:
```json
{
  "bot_token": "123456:ABC...",
  "allowed_users": ["username1", "username2"]
}
```

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
```

Binary: `bin/telegram-bot-{arch}`

## Deployment

1. Build for target architecture
2. Copy binary to `/jffs/scripts/vpn-director/telegram-bot`
3. Run `./setup_telegram_bot.sh` to create config
4. Bot auto-starts if config exists and token is set

## Key Patterns

**Authorization**: Username whitelist in config, checked in `isAuthorized()`

**Shell execution**: All router commands via `shell.Exec()` with exit code handling

**State management**: Thread-safe wizard state with mutex, per-chat storage

**Graceful shutdown**: Context cancellation on SIGINT/SIGTERM

## Dependencies

- `github.com/go-telegram-bot-api/telegram-bot-api/v5` — Telegram API client

## Logging

- Output: stdout + `/tmp/telegram-bot.log` (always, via `io.MultiWriter`)
- Format: `2026/01/18 15:04:05 main.go:42: [INFO] Bot started`
- Levels: `[INFO]`, `[WARN]`, `[ERROR]` (manual prefixes)
- Rotation: Both log files truncated at 200KB (checked every minute)
