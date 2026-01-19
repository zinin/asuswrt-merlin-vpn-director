# Server List Columns Design

## Problem

Current server list display in wizard has issues:
- Server names get truncated too aggressively
- Column layout switches too early (3 columns for >10 servers)

## Solution

Simplify column logic and remove name truncation.

### Column Layout

| Server Count | Columns |
|--------------|---------|
| 1-10         | 1       |
| 11+          | 2       |

### Button Format

```
{number}. {full_server_name}
```

No truncation — let Telegram handle overflow if needed.

## Implementation

### 1. Update `getServerGridColumns`

```go
func getServerGridColumns(count int) int {
    if count <= 10 {
        return 1
    }
    return 2
}
```

### 2. Update `sendServerSelection`

Remove `maxNameLen` logic and `truncateServerName` call:

```go
btnText := fmt.Sprintf("%d. %s", i+1, s.Name)
```

### 3. Remove `truncateServerName`

Function no longer needed — delete it.

## Files Changed

- `telegram-bot/internal/bot/wizard_handlers.go`
