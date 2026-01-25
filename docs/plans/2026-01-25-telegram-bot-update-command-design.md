# Telegram Bot /update Command Design

## Overview

Implement `/update` command for the Telegram bot that updates all VPN Director components to the latest GitHub release:
- Shell scripts (`vpn-director.sh`, `lib/*.sh`, etc.)
- Configuration templates (`*.template`)
- Init.d scripts and JFFS hooks
- Telegram bot binary

## Requirements

- Update all components in one operation
- Source: GitHub Releases API + raw.githubusercontent.com
- Semantic version comparison - only update if latest > current
- Command disabled in dev mode (CLI flag `--dev`)
- Lock file prevents concurrent updates
- Bot remains responsive during downloads
- External shell script handles bot restart

## Architecture Overview

### Why External Script?

Bot cannot replace its own binary while running. Solution:

1. Bot downloads all files to `/tmp/`
2. Bot generates and launches a shell script (detached)
3. Shell script stops the bot, replaces files, restarts bot
4. Bot reads notification file on startup and reports success

### Why Hybrid Download Approach?

GitHub releases contain only binary assets (`telegram-bot-arm64`, `telegram-bot-arm`). Scripts are in the repository.

Solution:
- **Binary**: Download from release assets (`releases/latest/download/telegram-bot-arm64`)
- **Scripts**: Download from raw.githubusercontent.com by tag (`refs/tags/v1.2.0/router/opt/...`)

This avoids changing CI to bundle scripts into release archives.

### Why Download Before Stop?

Downloading can fail (network issues, GitHub unavailable). If we stop the bot first and then download fails, user is left without a working bot.

Solution: Download everything to `/tmp/` first, only then stop and replace.

## Design Decisions

### Security: Integrity Verification

**Decision:** Trust HTTPS/GitHub (same as `install.sh`)

**Rationale:** HTTPS guarantees files come from GitHub, not a MITM attacker. This is the same trust model as the current installer. Adding checksums/signatures would protect against GitHub account compromise but adds complexity. Can be added later if needed.

### Concurrency: Download Blocking

**Decision:** Run downloads in a goroutine

**Rationale:** Downloads may take time on slow connections. Running synchronously would block all bot commands. With goroutine, bot stays responsive and can process other commands while downloading.

**Implementation:** Handler starts goroutine, sends progress messages back to chat.

### Handler Context

**Decision:** `HandleUpdate(msg *tgbotapi.Message)` without `context.Context`

**Rationale:** Current bot handlers don't use Context. HTTP timeouts are configured via `http.Client.Timeout`. Update runs in goroutine and script is detached — cancellation via Context provides no benefit. Use `context.Background()` internally for HTTP calls.

### Version Handling

**Decision:** Pass `Version`, `Commit`, `BuildDate` separately via ldflags, format where needed

**Rationale:** Semver parser expects clean version string. The `/version` command needs formatted output. Keeping them separate allows both use cases.

**Implementation:** `Deps` struct receives `Version` (raw tag), `Commit`, `BuildDate` separately. Handler formats for display, updater uses raw Version for comparison.

### Dependency Injection: Options Pattern

**Decision:** Use Options pattern for `bot.New()`

**Rationale:** `bot.New()` already has many parameters. Adding `devMode` and `updater` would make it unwieldy. Options pattern allows clean extension:

```go
b, err := bot.New(cfg, p, version,
    bot.WithDevMode(executor),  // sets devMode=true + custom executor
    bot.WithUpdater(updater.New()),
)
```

**Benefits:**
- Related settings grouped (devMode + executor)
- Easy to add new options without changing signature
- Idiomatic Go pattern

### Shell Script Error Handling

**Decision:** Strict fail-fast with `set -e`, no `|| true` on critical operations

**Rationale:** If any step fails (stop, copy, start), continuing could leave system in inconsistent state. All files in the download list are mandatory — if a file doesn't copy, it's a release error and update should fail.

**Exception:** `monit unmonitor/monitor` uses `|| true` because monit is optional.

### No Rollback on Script Failure

**Decision:** Accept risk of bot being down if script fails after stopping bot

**Rationale:** This is a rare edge case — files are already downloaded and validated before script runs. Adding rollback complexity (backup, restore) is not worth it. If script fails mid-copy, user must investigate manually. Lock file remains to prevent retry loops.

**Known risk:** If script fails between stop and start, bot is down with no notification. User must SSH to router and check `/tmp/vpn-director-update/update.log`.

### Lock File Format

**Decision:** Lock file contains PID of bot process

**Rationale:** If bot crashes after creating lock but before script runs, lock becomes stale. By storing PID, we can check if the process is still alive and remove stale locks automatically.

**Format:**
```
12345
```

**Check logic:**
1. Lock doesn't exist → proceed
2. Lock exists, read PID
3. `kill -0 $PID` succeeds → update in progress, reject
4. `kill -0 $PID` fails → stale lock, remove and proceed

**Known limitation:** If bot restarts during update (e.g., monit), new bot may remove lock while script is still running. This is unlikely — monit only restarts if process dies, and process dies from the script which then continues to completion.

### Download Cleanup

**Decision:** Clean `files/` directory before downloading and on error

**Rationale:** Failed downloads can leave partial files. Subsequent update attempts could use stale/corrupt files. Clean before download ensures fresh state. Clean on error ensures no garbage remains.

### File List Synchronization

**Decision:** Manual duplication between `install.sh` and bot

**Rationale:** Files rarely change. When adding new files, developer must update both places. A shared manifest or archive would add complexity (CI changes) for little benefit.

### Missing Binary Asset

**Decision:** Fail update entirely if binary not found for architecture

**Rationale:** Only arm64 and arm are supported. These assets will always be present in releases. Proceeding with scripts-only would leave bot at old version, which is confusing.

### Dependencies

**Decision:** Require `pgrep` (from `procps-ng-pgrep`)

**Rationale:** Already in project dependencies per README. Script uses `pgrep -f telegram-bot` to wait for process exit. No fallback to `pidof` needed.

### JFFS Scripts Handling

**Decision:** Overwrite `firewall-start` and `wan-event` without warning

**Rationale:** Same behavior as `install.sh`. Users who customize these files should know that updates will overwrite them. Merging user changes is too complex and error-prone.

### VPN Director Restart

**Decision:** Do not restart VPN Director/Xray after update

**Rationale:** Updated scripts/templates don't affect running configuration. User can restart manually if needed via `/restart` command or `vpn-director.sh restart`.

### Dev Mode Detection

**Decision:** Check CLI flag `--dev` only, not `Version == "dev"`

**Rationale:** Dev mode is an explicit choice when starting the bot. A release build without proper tag shouldn't accidentally enable dev mode. The `--dev` flag is already used throughout the bot for this purpose.

**Note:** If `Version == "dev"` (unparseable), `ShouldUpdate()` returns false and handler shows "Cannot check updates for dev build" message.

### Semver Format

**Decision:** Strict `vX.Y.Z` format only, no pre-release tags

**Rationale:** GitHub API `releases/latest` already excludes pre-releases. Parser only needs to handle stable versions. Simpler implementation.

### Access Control

**Decision:** All authorized users can run `/update`

**Rationale:** Consistent with other commands. No separate "admin" concept exists in the bot. If someone is in `allowed_users`, they have full control. No confirmation step needed.

### Monit Handling

**Decision:** Continue update if monit unmonitor/monitor fails

**Rationale:** Monit is optional. Its absence or failure to unmonitor shouldn't block updates. Use `|| true` for monit commands only.

### Progress Messages

**Decision:** Send multiple separate messages for progress

**Rationale:** Simpler than editing a single message. Three messages ("Starting...", "Files downloaded...", "Script started...") is acceptable UX.

### Testability

**Decision:** Make GitHub API base URL injectable for testing

**Rationale:** Allows using `httptest.NewServer()` in tests instead of skipping GitHub client tests.

## Flow

```
User: /update
       │
       ▼
┌─────────────────────────────────────────────────────────────────┐
│  1. Checks                                                      │
│     - Dev mode (--dev flag)? → "Command /update is not          │
│       available in dev mode"                                    │
│     - Version == "dev"? → "Cannot check updates for dev build"  │
│     - Lock exists AND process alive? → "Update is already       │
│       in progress..."                                           │
│     - Lock exists BUT process dead? → Remove stale lock         │
└─────────────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────────────────┐
│  2. Get latest release from GitHub API                          │
│     GET api.github.com/repos/zinin/asuswrt-merlin-vpn-director/ │
│         releases/latest                                         │
│     Extract: tag_name, assets[].browser_download_url            │
└─────────────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────────────────┐
│  3. Compare versions (semver)                                   │
│     Parse current version (from ldflags Version variable)       │
│     Parse latest version (from tag_name)                        │
│     current >= latest? → "Already running the latest version"   │
│     Parse error? → Fail with error message                      │
└─────────────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────────────────┐
│  4. Create lock file (write current PID)                        │
│     Clean files/ directory (remove stale downloads)             │
│     Send: "Starting update v1.1.0 → v1.2.0..."                  │
└─────────────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────────────────┐
│  5. Download all files in GOROUTINE to /tmp/vpn-director-update/│
│     - Scripts from raw.githubusercontent.com/refs/tags/{tag}/   │
│     - Binary from release assets (arm64 or arm by runtime.GOARCH│
│     - Missing binary asset? → Clean files/, remove lock, error  │
│     - Any download fails? → Clean files/, remove lock, error    │
└─────────────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────────────────┐
│  6. Send: "Files downloaded, starting update..."                │
└─────────────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────────────────┐
│  7. Generate update.sh from Go template                         │
│     Embed into script body:                                     │
│       CHAT_ID=123456789                                         │
│       OLD_VERSION="v1.1.0"                                      │
│       NEW_VERSION="v1.2.0"                                      │
│     Run script detached: nohup /bin/sh update.sh &              │
└─────────────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────────────────┐
│  8. Send: "Update script started, bot will restart in a few     │
│           seconds..."                                           │
│     (bot continues running, script will stop it)                │
└─────────────────────────────────────────────────────────────────┘
```

## Shell Script Flow

Script uses `set -e` for strict fail-fast behavior. Variables are embedded by bot when generating the script. No `|| true` on critical operations.

```
┌─────────────────────────────────────────────────────────────────┐
│  0. Header                                                      │
│     #!/bin/sh                                                   │
│     set -e                                                      │
│     CHAT_ID=123456789  (embedded by bot)                        │
│     OLD_VERSION="v1.1.0"                                        │
│     NEW_VERSION="v1.2.0"                                        │
│     UPDATE_DIR="/tmp/vpn-director-update"                       │
└─────────────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────────────────┐
│  1. Unmonitor in monit (if installed) — OPTIONAL, uses || true  │
│     Prevents monit from restarting bot during file replacement  │
│     command -v monit >/dev/null && monit unmonitor telegram-bot │
│       || true                                                   │
└─────────────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────────────────┐
│  2. Stop bot via init.d                                         │
│     /opt/etc/init.d/S98telegram-bot stop                        │
│     Wait for process to exit (poll via pgrep, max 30s)          │
│     If still running after 30s → pkill -9                       │
└─────────────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────────────────┐
│  3. Copy files to target locations — NO || true, fails on error │
│     cp files/opt/vpn-director/*.sh → /opt/vpn-director/         │
│     cp files/opt/vpn-director/lib/*.sh → /opt/vpn-director/lib/ │
│     cp files/*.template → respective locations                  │
│     cp files/telegram-bot → /opt/vpn-director/telegram-bot      │
│     cp files/jffs/scripts/* → /jffs/scripts/                    │
│     chmod +x on all scripts and binary                          │
│     NOTE: Overwrites any user customizations in jffs/scripts/   │
└─────────────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────────────────┐
│  4. Create notify.json for bot to read on startup               │
│     {"chat_id":123456789,                                       │
│      "old_version":"v1.1.0",                                    │
│      "new_version":"v1.2.0"}                                    │
└─────────────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────────────────┐
│  5. Remove lock file                                            │
│     rm -f $UPDATE_DIR/lock                                      │
└─────────────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────────────────┐
│  6. Re-monitor in monit (if was unmonitored) — uses || true     │
│     command -v monit >/dev/null && monit monitor telegram-bot   │
│       || true                                                   │
└─────────────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────────────────┐
│  7. Start bot via init.d                                        │
│     /opt/etc/init.d/S98telegram-bot start                       │
│     NOTE: VPN Director is NOT restarted automatically           │
│           User runs /restart or vpn-director.sh restart if needed│
└─────────────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────────────────┐
│  8. Cleanup — delete entire update directory                    │
│     rm -rf $UPDATE_DIR                                          │
└─────────────────────────────────────────────────────────────────┘
```

## Bot Startup Notification

On every startup, bot checks for pending update notification:

```go
// In cmd/bot/main.go, before starting message polling
if err := startup.CheckAndSendNotify(bot); err != nil {
    slog.Warn("Failed to send update notification", "error", err)
}
```

**Logic:**
1. Check if `/tmp/vpn-director-update/notify.json` exists
2. If not exists → normal startup, do nothing
3. If exists → read JSON, parse chat_id and versions
4. Send message: "Update complete: v1.1.0 → v1.2.0"
5. Delete `/tmp/vpn-director-update/` directory (entire cleanup)

**notify.json format:**
```json
{
  "chat_id": 123456789,
  "old_version": "v1.1.0",
  "new_version": "v1.2.0"
}
```

## Temporary Files

All update-related files in single directory for easy cleanup:

```
/tmp/vpn-director-update/
├── lock                        # Lock file (contains PID, e.g., "12345")
├── notify.json                 # Created by script, read by bot on startup
├── update.sh                   # Generated shell script
├── update.log                  # Script execution log (deleted on success)
└── files/                      # Downloaded files (mirrors target structure)
    ├── opt/
    │   ├── vpn-director/
    │   │   ├── vpn-director.sh
    │   │   ├── configure.sh
    │   │   ├── import_server_list.sh
    │   │   ├── setup_telegram_bot.sh
    │   │   ├── vpn-director.json.template
    │   │   └── lib/
    │   │       ├── common.sh
    │   │       ├── firewall.sh
    │   │       ├── config.sh
    │   │       ├── ipset.sh
    │   │       ├── tunnel.sh
    │   │       ├── tproxy.sh
    │   │       └── send-email.sh
    │   └── etc/
    │       ├── init.d/
    │       │   ├── S98telegram-bot
    │       │   └── S99vpn-director
    │       └── xray/
    │           └── config.json.template
    ├── jffs/
    │   └── scripts/
    │       ├── firewall-start
    │       └── wan-event
    └── telegram-bot            # Binary for current architecture
```

**Note:** On successful update, entire `/tmp/vpn-director-update/` is deleted including `update.log`. On failure, directory remains for debugging.

## Version Comparison

Strict semver only. Pre-release tags (v1.2.3-rc1) not supported.

```go
type Version struct {
    Major int
    Minor int
    Patch int
    Raw   string  // Original string for display ("v1.2.3")
}

// ParseVersion parses "v1.2.3" or "1.2.3"
// Returns error for:
//   - "dev" (dev builds)
//   - "v1.2.3-rc1" (pre-release)
//   - "v1.2" (incomplete)
//   - "invalid" (non-numeric)
func ParseVersion(s string) (Version, error)

// Compare returns:
//   -1 if v < other
//    0 if v == other
//    1 if v > other
func (v Version) Compare(other Version) int

// IsOlderThan returns true if v < other
func (v Version) IsOlderThan(other Version) bool
```

## Makefile Changes

Current Makefile produces version like "v1.2.0-5-gabc1234" when not exactly on a tag.

```makefile
# Before: includes -N-gHASH suffix after tag
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

# After: only the tag name, or "dev" if no tags
VERSION ?= $(shell git describe --tags --abbrev=0 2>/dev/null || echo "dev")
```

This ensures `Version` variable contains clean semver that can be parsed.

## Files to Download

### Scripts (from raw.githubusercontent.com)

URL pattern: `https://raw.githubusercontent.com/zinin/asuswrt-merlin-vpn-director/refs/tags/{tag}/{path}`

| Source Path | Target Path |
|-------------|-------------|
| `router/opt/vpn-director/vpn-director.sh` | `/opt/vpn-director/vpn-director.sh` |
| `router/opt/vpn-director/configure.sh` | `/opt/vpn-director/configure.sh` |
| `router/opt/vpn-director/import_server_list.sh` | `/opt/vpn-director/import_server_list.sh` |
| `router/opt/vpn-director/setup_telegram_bot.sh` | `/opt/vpn-director/setup_telegram_bot.sh` |
| `router/opt/vpn-director/vpn-director.json.template` | `/opt/vpn-director/vpn-director.json.template` |
| `router/opt/vpn-director/lib/common.sh` | `/opt/vpn-director/lib/common.sh` |
| `router/opt/vpn-director/lib/firewall.sh` | `/opt/vpn-director/lib/firewall.sh` |
| `router/opt/vpn-director/lib/config.sh` | `/opt/vpn-director/lib/config.sh` |
| `router/opt/vpn-director/lib/ipset.sh` | `/opt/vpn-director/lib/ipset.sh` |
| `router/opt/vpn-director/lib/tunnel.sh` | `/opt/vpn-director/lib/tunnel.sh` |
| `router/opt/vpn-director/lib/tproxy.sh` | `/opt/vpn-director/lib/tproxy.sh` |
| `router/opt/vpn-director/lib/send-email.sh` | `/opt/vpn-director/lib/send-email.sh` |
| `router/opt/etc/xray/config.json.template` | `/opt/etc/xray/config.json.template` |
| `router/opt/etc/init.d/S99vpn-director` | `/opt/etc/init.d/S99vpn-director` |
| `router/opt/etc/init.d/S98telegram-bot` | `/opt/etc/init.d/S98telegram-bot` |
| `router/jffs/scripts/firewall-start` | `/jffs/scripts/firewall-start` |
| `router/jffs/scripts/wan-event` | `/jffs/scripts/wan-event` |

### Binary (from release assets)

URL: From `assets[].browser_download_url` in releases/latest response

| Architecture (`runtime.GOARCH`) | Asset Name |
|--------------------------------|------------|
| `arm64` | `telegram-bot-arm64` |
| `arm` | `telegram-bot-arm` |

If asset for current architecture not found → fail update entirely.

**Note:** File list is duplicated in `install.sh`. Keep in sync manually when adding new files.

## User Messages (English)

| Situation | Message |
|-----------|---------|
| Dev mode (--dev flag) | `Command /update is not available in dev mode` |
| Dev version (unparseable) | `Cannot check updates for dev build` |
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

| Error | Handling |
|-------|----------|
| Dev mode enabled (--dev) | Reject immediately with message |
| Dev version (unparseable) | Reject with "Cannot check updates for dev build" |
| Lock exists, process alive | Reject with "already in progress" |
| Lock exists, process dead | Remove stale lock, proceed |
| GitHub API fails | Remove lock, report error |
| Version parse fails | Remove lock, report error |
| Download fails | Clean files/, remove lock, report error |
| Binary asset missing | Clean files/, remove lock, report error |
| Script generation fails | Clean files/, remove lock, report error |
| init.d stop fails | Script exits (set -e), lock remains, no notification |
| File copy fails | Script exits (set -e), lock remains, no notification |
| init.d start fails | Logged in update.log, user must investigate manually |

**Recovery from script failure:** If script fails after stopping bot, bot remains down. User must SSH to router, check `/tmp/vpn-director-update/update.log`, fix manually, and remove lock file. This is a rare edge case.

## New Files

| File | Description |
|------|-------------|
| `internal/updater/updater.go` | Service struct, interface, constructor |
| `internal/updater/version.go` | Semver parsing and comparison (strict format) |
| `internal/updater/github.go` | GitHub API client (releases/latest), injectable baseURL |
| `internal/updater/downloader.go` | File download logic (runs in goroutine) |
| `internal/updater/script.go` | Script generation, lock file with PID |
| `internal/updater/update_script.sh.tmpl` | Go template for shell script |
| `internal/handler/update.go` | /update command handler |
| `internal/startup/notify.go` | Startup notification check and send |

## Modified Files

| File | Changes |
|------|---------|
| `Makefile` | `--abbrev=0` for VERSION (clean tag only) |
| `internal/handler/handler.go` | Add `Updater`, `Version`, `Commit`, `BuildDate` to Deps |
| `internal/handler/misc.go` | Add `/update` to `/start` help text |
| `internal/bot/bot.go` | Options pattern: `WithDevMode()`, `WithUpdater()`. Register `/update` in BotFather commands |
| `internal/bot/router.go` | Add route for `/update` command, add `UpdateRouterHandler` interface |
| `cmd/bot/main.go` | Use Options pattern, init updater service, call `startup.CheckAndSendNotify` |

## Implementation Order

1. Modify `Makefile` (VERSION with --abbrev=0)
2. Create `internal/updater/version.go` (strict semver parsing with tests)
3. Create `internal/updater/updater.go` (struct, interface)
4. Create `internal/updater/github.go` (API client with injectable baseURL)
5. Create `internal/updater/downloader.go` (async downloading with cleanup)
6. Create `internal/updater/update_script.sh.tmpl` (Go template, strict set -e)
7. Create `internal/updater/script.go` (script generation, lock with PID)
8. Create `internal/startup/notify.go` (startup notification)
9. Create `internal/handler/update.go` (command handler without Context)
10. Refactor `internal/bot/bot.go` to use Options pattern
11. Modify `internal/handler/handler.go` (add to Deps)
12. Modify `internal/handler/misc.go` (add /update to /start help)
13. Modify `internal/bot/router.go` (add route and interface)
14. Modify `cmd/bot/main.go` (integration with Options)
