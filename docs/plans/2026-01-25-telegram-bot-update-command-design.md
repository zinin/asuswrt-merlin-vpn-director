# Telegram Bot /update Command Design

## Overview

Implement `/update` command for the Telegram bot that updates all VPN Director components (scripts, templates, bot binary) to the latest GitHub release.

## Requirements

- Update all components: scripts, templates, and bot binary
- Source: GitHub Releases API for version info, raw.githubusercontent.com for scripts by tag, release assets for binary
- Compare versions semantically (major.minor.patch) - only update if latest > current
- Command disabled in dev mode (CLI flag `--dev`)
- Lock file prevents concurrent updates
- Bot remains responsive during downloads (async), external script handles restart

## Design Decisions

Decisions made during design review:

| Topic | Decision | Rationale |
|-------|----------|-----------|
| **Integrity verification** | Trust HTTPS/GitHub | Same approach as install.sh; checksums/signatures can be added later |
| **Download blocking** | Run in goroutine | Bot stays responsive during downloads |
| **Version string** | Use raw `Version`, not `versionString()` | Handler receives clean tag (e.g., "v1.2.0"), not formatted string with commit/date |
| **Script errors** | Fail-fast with `set -e` | If something fails, user investigates manually |
| **File list sync** | Manual duplication | List duplicated in install.sh and bot; sync manually when adding files |
| **Missing binary** | Fail entirely | Only arm64/arm supported, assets always present in releases |
| **Lock file** | Contains PID | Check if process alive to detect stale locks |
| **pgrep dependency** | Required | Part of project dependencies (procps-ng-pgrep) |
| **jffs/scripts/** | Overwrite | Same as install.sh; user customizations will be lost |
| **VPN Director restart** | Not automatic | User restarts manually if needed after update |
| **Dev mode signal** | CLI flag `--dev` only | Not based on Version value |
| **Semver format** | Strict `vX.Y.Z` only | No pre-release suffixes; GitHub releases/latest excludes pre-releases anyway |

## Flow

```
User: /update
       │
       ▼
┌─────────────────────────────────────────────────────────────────┐
│  1. Checks                                                      │
│     - Dev mode (--dev flag)? → "Command /update is not          │
│       available in dev mode"                                    │
│     - Lock exists AND process alive? → "Update is already       │
│       in progress..."                                           │
│     - Lock exists BUT process dead? → Remove stale lock         │
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
│     Parse error? → Fail with error message                      │
└─────────────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────────────────┐
│  4. Create lock (with PID), send "Starting update..."           │
└─────────────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────────────────┐
│  5. Download all files in GOROUTINE                             │
│     - Scripts from raw.githubusercontent.com/refs/tags/{tag}/   │
│     - Binary from release assets (arm64 or arm)                 │
│     - Missing binary asset → Fail entirely                      │
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
│     Bot writes chat_id, versions into script body via template  │
└─────────────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────────────────┐
│  8. Send "Update script started, bot will restart..."           │
│     (bot continues running, script will kill it)                │
└─────────────────────────────────────────────────────────────────┘
```

## Shell Script Flow

Script uses `set -e` for fail-fast behavior. If any step fails, script exits immediately.

```
┌─────────────────────────────────────────────────────────────────┐
│  0. set -e (fail-fast on any error)                             │
└─────────────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────────────────┐
│  1. Unmonitor in monit (if available)                           │
│     command -v monit && monit unmonitor telegram-bot            │
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
│     Overwrites jffs/scripts/* (user customizations lost)        │
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
│     command -v monit && monit monitor telegram-bot              │
└─────────────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────────────────┐
│  7. Start bot via init.d                                        │
│     /opt/etc/init.d/S98telegram-bot start                       │
│     (VPN Director NOT restarted - user does manually if needed) │
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
├── lock                        # Lock file (contains PID)
├── notify.json                 # Notification for bot after restart
├── update.sh                   # Generated update script
├── update.log                  # Script execution log
└── files/                      # Downloaded files
    ├── opt/vpn-director/       # Scripts
    ├── opt/etc/xray/           # Templates
    ├── jffs/scripts/           # Hooks
    └── telegram-bot            # Binary
```

## Lock File Format

Lock file contains PID of the bot process that created it:

```
12345
```

When checking lock:
1. If lock file doesn't exist → no update in progress
2. If lock file exists, read PID
3. Check if process with that PID is alive (`kill -0 $PID`)
4. If alive → update in progress, reject new update
5. If dead → stale lock, remove and proceed

## Version Comparison

Strict semver only (`vX.Y.Z` or `X.Y.Z`). Pre-release tags not supported.

```go
type Version struct {
    Major int
    Minor int
    Patch int
    Raw   string
}

// ParseVersion parses "v1.2.3" or "1.2.3"
// Returns error for invalid formats (e.g., "v1.2.3-rc1", "dev")
func ParseVersion(s string) (Version, error)

// IsOlderThan returns true if v < other
func (v Version) IsOlderThan(other Version) bool
```

**Important:** Handler must use raw `Version` variable (e.g., "v1.2.0"), NOT `versionString()` which includes commit and date.

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
- If asset not found → fail update entirely (don't proceed with scripts-only)

**Note:** File list is duplicated in install.sh. Keep in sync manually when adding new files.

## User Messages (English)

| Situation | Message |
|-----------|---------|
| Dev mode | `Command /update is not available in dev mode` |
| Lock exists (active) | `Update is already in progress, please wait...` |
| GitHub API error | `Failed to check for updates: {error}` |
| Version parse error | `Failed to parse version: {error}` |
| Already latest | `Already running the latest version: v1.2.0` |
| Starting | `Starting update v1.1.0 → v1.2.0...` |
| Download failed | `Download failed: {error}` |
| Binary not found | `Binary for architecture {arch} not found in release` |
| Files ready | `Files downloaded, starting update...` |
| Script started | `Update script started, bot will restart in a few seconds...` |
| After restart | `Update complete: v1.1.0 → v1.2.0` |

## Error Handling

- All downloads complete before stopping bot (minimizes risk)
- Downloads run in goroutine (bot stays responsive)
- Lock file contains PID to detect stale locks from crashed updates
- Shell script uses `set -e` for fail-fast behavior
- If bot fails to start after update: user must investigate manually (monit may restart)
- Missing binary asset for architecture → fail entirely, don't proceed with scripts-only

## New Files

| File | Description |
|------|-------------|
| `internal/updater/updater.go` | Service struct, interface, constructor |
| `internal/updater/version.go` | Semver parsing and comparison (strict format) |
| `internal/updater/github.go` | GitHub API client |
| `internal/updater/downloader.go` | File download logic (runs in goroutine) |
| `internal/updater/script.go` | Script generation, lock file with PID |
| `internal/updater/update_script.sh.tmpl` | Shell script template (with set -e) |
| `internal/handler/update.go` | /update command handler |
| `internal/startup/notify.go` | Startup notification check |

## Modified Files

| File | Changes |
|------|---------|
| `Makefile` | `--abbrev=0` for VERSION |
| `internal/handler/handler.go` | Add Updater and Version (raw) to Deps |
| `internal/bot/router.go` | Route for /update |
| `internal/bot/bot.go` | Register command with BotFather |
| `cmd/bot/main.go` | Init updater, call startup.CheckAndSendNotify, pass raw Version to Deps |

## Implementation Order

1. Modify Makefile (VERSION with --abbrev=0)
2. Create `updater/version.go` (strict semver parsing)
3. Create `updater/updater.go` (struct, interface)
4. Create `updater/github.go` (API client)
5. Create `updater/downloader.go` (downloading in goroutine)
6. Create `updater/update_script.sh.tmpl` (template with set -e)
7. Create `updater/script.go` (script generation, lock with PID)
8. Create `startup/notify.go` (startup notification)
9. Create `handler/update.go` (handler)
10. Modify `handler/handler.go` (Deps with raw Version)
11. Modify `bot/router.go` and `bot/bot.go` (registration)
12. Modify `cmd/bot/main.go` (integration)
