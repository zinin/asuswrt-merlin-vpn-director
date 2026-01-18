# Telegram Bot Logging Design

## Overview

Add file logging to telegram-bot while preserving stdout output.

## Requirements

- Write logs to `/tmp/telegram-bot.log`
- Duplicate output to stdout (for manual debugging)
- Use standard Go log format with file:line
- No log rotation (file in /tmp, cleared on reboot)
- No configuration (hardcoded path)

## Log Format

Standard Go log with `Ldate | Ltime | Lshortfile`:

```
2026/01/18 15:04:05 main.go:42: [INFO] Bot started
2026/01/18 15:04:05 bot.go:58: [WARN] Unauthorized: user123
2026/01/18 15:04:06 handlers.go:23: [ERROR] Failed to send message: timeout
```

## Implementation

### Changes

Single file: `telegram-bot/cmd/bot/main.go`

### Code

Add at the beginning of `main()`, before any logging:

```go
import (
    "io"
    // ... existing imports
)

func main() {
    // Initialize logging to file + stdout
    logFile, err := os.OpenFile("/tmp/telegram-bot.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
    if err != nil {
        log.Fatalf("[ERROR] Failed to open log file: %v", err)
    }
    defer logFile.Close()

    log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
    log.SetOutput(io.MultiWriter(os.Stdout, logFile))

    // ... rest of main()
}
```

### Behavior

- Log file created on first start (or appended if exists)
- Both stdout and file receive identical output
- File cleared on router reboot (lives in /tmp)
- No changes to existing log.Printf/log.Println calls

## Alternatives Considered

1. **Custom logger with vpn-director format** — rejected for simplicity
2. **Configurable log path** — rejected, YAGNI
3. **Log rotation** — rejected, low log volume + /tmp cleared on reboot
4. **File-only output** — rejected, stdout useful for manual debugging
