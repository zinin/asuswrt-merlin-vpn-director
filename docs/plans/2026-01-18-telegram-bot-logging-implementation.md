# Telegram Bot File Logging Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add file logging to `/tmp/telegram-bot.log` with stdout duplication.

**Architecture:** Initialize `io.MultiWriter` at start of `main()` to write logs to both stdout and file. Use standard Go log flags for date/time/file:line format.

**Tech Stack:** Go standard library (`log`, `io`, `os`)

---

## Task 1: Add File Logging to main.go

**Files:**
- Modify: `telegram-bot/cmd/bot/main.go:3-17` (imports and start of main)

**Step 1: Add `io` import**

In the import block, add `"io"`:

```go
import (
	"context"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/bot"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/config"
)
```

**Step 2: Add log file initialization at start of main()**

Insert after `func main() {` and before config loading:

```go
func main() {
	// Initialize logging to file + stdout
	logFile, err := os.OpenFile("/tmp/telegram-bot.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("[ERROR] Failed to open log file: %v", err)
	}
	defer logFile.Close()

	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.SetOutput(io.MultiWriter(os.Stdout, logFile))

	cfg, err := config.Load(configPath)
	// ... rest of main()
```

**Step 3: Build and verify**

Run:
```bash
cd /opt/github/zinin/asuswrt-merlin-vpn-director/telegram-bot && make build
```

Expected: Build succeeds with no errors.

**Step 4: Test log output format**

Run bot briefly (Ctrl+C to stop):
```bash
./bin/telegram-bot-* 2>&1 | head -5
```

Expected output format:
```
2026/01/18 15:04:05 main.go:25: [INFO] Config not found, run setup_telegram_bot.sh first
```

**Step 5: Verify log file creation**

```bash
cat /tmp/telegram-bot.log
```

Expected: Same log entries as stdout.

**Step 6: Commit**

```bash
git add telegram-bot/cmd/bot/main.go
git commit -m "feat(telegram-bot): add file logging to /tmp/telegram-bot.log"
```

---

## Task 2: Update Documentation

**Files:**
- Modify: `.claude/rules/telegram-bot.md` (Logging section already mentions the file)

**Step 1: Verify documentation is accurate**

The existing docs already mention `/tmp/telegram-bot.log`. No changes needed if accurate.

**Step 2: Commit design doc removal (optional cleanup)**

The design doc `docs/plans/2026-01-18-telegram-bot-logging-design.md` can remain for history.

---

## Final Verification

```bash
# Build
cd /opt/github/zinin/asuswrt-merlin-vpn-director/telegram-bot && make build

# Run briefly (will exit if no config)
./bin/telegram-bot-*

# Check log file exists and has entries
ls -la /tmp/telegram-bot.log
cat /tmp/telegram-bot.log
```

Expected: Log file created with date/time/file:line format entries.
