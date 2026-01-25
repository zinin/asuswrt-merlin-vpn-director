# Telegram Bot /update Command Design

## Overview

Implement `/update` command for the Telegram bot that updates all VPN Director components (scripts, templates, bot binary) to the latest GitHub release.

## Requirements

- Update all components: scripts, templates, and bot binary
- Source: GitHub Releases API for version info, raw.githubusercontent.com for scripts by tag, release assets for binary
- Compare versions semantically (major.minor.patch) - only update if latest > current
- Command disabled in dev mode
- Lock file prevents concurrent updates
- Bot continues working while downloading, external script handles restart

## Flow

```
User: /update
       │
       ▼
┌─────────────────────────────────────────────────────────────────┐
│  1. Checks                                                      │
│     - Dev mode? → "Command /update is not available in dev mode"│
│     - Lock exists? → "Update is already in progress..."         │
└─────────────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────────────────┐
│  2. Get latest release from GitHub API                          │
│     GET api.github.com/repos/.../releases/latest                │
└─────────────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────────────────┐
│  3. Compare versions (semver)                                   │
│     current >= latest? → "Already running the latest version"   │
└─────────────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────────────────┐
│  4. Create lock, send "Starting update v1.0.0 → v1.1.0..."      │
└─────────────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────────────────┐
│  5. Download all files to /tmp/vpn-director-update/files/       │
│     - Scripts from raw.githubusercontent.com/refs/tags/{tag}/   │
│     - Binary from release assets                                │
└─────────────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────────────────┐
│  6. Send "Files downloaded, starting update..."                 │
└─────────────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────────────────┐
│  7. Generate and run update.sh (detached)                       │
│     Bot writes chat_id, versions into script body               │
└─────────────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────────────────┐
│  8. Send "Update script started, bot will restart..."           │
│     (bot continues running, script will kill it)                │
└─────────────────────────────────────────────────────────────────┘
```

## Shell Script Flow

```
┌─────────────────────────────────────────────────────────────────┐
│  1. Unmonitor in monit (if available)                           │
│     monit unmonitor telegram-bot                                │
└─────────────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────────────────┐
│  2. Stop bot via init.d                                         │
│     /opt/etc/init.d/S98telegram-bot stop                        │
│     Wait for process to exit (poll via pgrep, max 30s)          │
└─────────────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────────────────┐
│  3. Copy files to target locations                              │
│     Set permissions (chmod +x)                                  │
└─────────────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────────────────┐
│  4. Create notify.json with chat_id and versions                │
└─────────────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────────────────┐
│  5. Remove lock file                                            │
└─────────────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────────────────┐
│  6. Re-monitor in monit (if available)                          │
│     monit monitor telegram-bot                                  │
└─────────────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────────────────┐
│  7. Start bot via init.d                                        │
│     /opt/etc/init.d/S98telegram-bot start                       │
└─────────────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────────────────┐
│  8. Cleanup: remove files/, update.sh                           │
└─────────────────────────────────────────────────────────────────┘
```

## Bot Startup

On startup, bot checks for `/tmp/vpn-director-update/notify.json`:
- If exists: read chat_id and versions, send "Update complete: v1.0.0 → v1.1.0", delete file and directory
- If not exists: normal startup

## Temporary Files

```
/tmp/vpn-director-update/
├── lock                        # Lock file
├── notify.json                 # Notification for bot after restart
├── update.sh                   # Generated update script
├── update.log                  # Script execution log
└── files/                      # Downloaded files
    ├── opt/vpn-director/       # Scripts
    ├── opt/etc/xray/           # Templates
    ├── jffs/scripts/           # Hooks
    └── telegram-bot            # Binary
```

## Version Comparison

```go
type Version struct {
    Major int
    Minor int
    Patch int
    Raw   string
}

// ParseVersion parses "v1.2.3" or "1.2.3"
func ParseVersion(s string) (Version, error)

// IsOlderThan returns true if v < other
func (v Version) IsOlderThan(other Version) bool
```

- "dev" version never updates (dev mode check)
- Only update if current.IsOlderThan(latest)

## Makefile Changes

```makefile
# Before: includes -N-gHASH suffix
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

# After: only tag name
VERSION ?= $(shell git describe --tags --abbrev=0 2>/dev/null || echo "dev")
```

## Files to Download

Scripts (from raw.githubusercontent.com by tag):
- `router/opt/vpn-director/vpn-director.sh`
- `router/opt/vpn-director/configure.sh`
- `router/opt/vpn-director/import_server_list.sh`
- `router/opt/vpn-director/setup_telegram_bot.sh`
- `router/opt/vpn-director/lib/common.sh`
- `router/opt/vpn-director/lib/firewall.sh`
- `router/opt/vpn-director/lib/config.sh`
- `router/opt/vpn-director/lib/ipset.sh`
- `router/opt/vpn-director/lib/tunnel.sh`
- `router/opt/vpn-director/lib/tproxy.sh`
- `router/opt/vpn-director/lib/send-email.sh`
- `router/opt/vpn-director/vpn-director.json.template`
- `router/opt/etc/xray/config.json.template`
- `router/opt/etc/init.d/S99vpn-director`
- `router/opt/etc/init.d/S98telegram-bot`
- `router/jffs/scripts/firewall-start`
- `router/jffs/scripts/wan-event`

Binary (from release assets):
- `telegram-bot-arm64` or `telegram-bot-arm` (by architecture)

## User Messages (English)

| Situation | Message |
|-----------|---------|
| Dev mode | `Command /update is not available in dev mode` |
| Lock exists | `Update is already in progress, please wait...` |
| GitHub API error | `Failed to check for updates: {error}` |
| Already latest | `Already running the latest version: v1.2.0` |
| Starting | `Starting update v1.1.0 → v1.2.0...` |
| Download failed | `Download failed: {error}` |
| Files ready | `Files downloaded, starting update...` |
| Script started | `Update script started, bot will restart in a few seconds...` |
| After restart | `Update complete: v1.1.0 → v1.2.0` |

## Error Handling

- All downloads complete before stopping bot (minimizes risk)
- Lock file prevents concurrent updates
- If bot fails to start after update: logged, user must investigate manually (monit may restart)
- init.d errors are logged but don't stop the script

## New Files

| File | Description |
|------|-------------|
| `internal/updater/updater.go` | Service struct, interface, constructor |
| `internal/updater/version.go` | Semver parsing and comparison |
| `internal/updater/github.go` | GitHub API client |
| `internal/updater/downloader.go` | File download logic |
| `internal/updater/script.go` | Script generation, lock file |
| `internal/updater/update_script.sh.tmpl` | Shell script template |
| `internal/handler/update.go` | /update command handler |
| `internal/startup/notify.go` | Startup notification check |

## Modified Files

| File | Changes |
|------|---------|
| `Makefile` | `--abbrev=0` for VERSION |
| `internal/handler/handler.go` | Add Updater and Version to Deps |
| `internal/bot/router.go` | Route for /update |
| `internal/bot/bot.go` | Register command with BotFather |
| `cmd/bot/main.go` | Init updater, call startup.CheckAndSendNotify |

## Implementation Order

1. Modify Makefile (VERSION with --abbrev=0)
2. Create `updater/version.go` (semver parsing)
3. Create `updater/updater.go` (struct, interface)
4. Create `updater/github.go` (API client)
5. Create `updater/downloader.go` (downloading)
6. Create `updater/update_script.sh.tmpl` (template)
7. Create `updater/script.go` (script generation)
8. Create `startup/notify.go` (startup notification)
9. Create `handler/update.go` (handler)
10. Modify `handler/handler.go` (Deps)
11. Modify `bot/router.go` and `bot/bot.go` (registration)
12. Modify `cmd/bot/main.go` (integration)
