# Logging Format Redesign - Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Обновить формат логирования: ISO 8601 timestamp, уровни TRACE/DEBUG/INFO/WARN/ERROR, единый формат для syslog/stderr/файла.

**Architecture:** Обновляем функцию `log` в common.sh, затем заменяем все вызовы `log -l <old>` на `log -l <NEW>` по маппингу: err→ERROR, warn→WARN, notice→TRACE, debug→DEBUG.

**Tech Stack:** ash (BusyBox), syslog через logger

---

## Task 1: Обновить функцию log в common.sh

**Files:**
- Modify: `jffs/scripts/vpn-director/utils/common.sh:326-369`

**Step 1: Заменить функции log и log_to_file**

Найти и заменить блок с строки 326 по 369:

```bash
# Найти:
_log_tag="$(get_script_name -n)"
LOG_FILE="/tmp/vpn_director.log"
MAX_LOG_SIZE=102400  # 100KB

# -------------------------------------------------------------------------------------------------
# log_to_file - append timestamped message to log file with rotation
# -------------------------------------------------------------------------------------------------
log_to_file() {
    local msg="$(date '+%Y-%m-%d %H:%M:%S') [$_log_tag] $*"

    # Rotate if file exceeds limit
    if [ -f "$LOG_FILE" ] && [ "$(wc -c < "$LOG_FILE")" -gt "$MAX_LOG_SIZE" ]; then
        mv "$LOG_FILE" "${LOG_FILE}.old"
    fi

    printf '%s\n' "$msg" >> "$LOG_FILE"
}

log() {
    local level="info"    # default priority

    # Optional "-l <level>"
    if [ "$1" = "-l" ] && [ -n "$2" ]; then
        level=$2
        shift 2
    fi

    # Prefix table for non-default levels
    local prefix=""
    case "$level" in
        debug)   prefix="DEBUG: " ;;
        notice)  prefix="NOTICE: " ;;
        warn)    prefix="WARNING: " ;;
        err)     prefix="ERROR: " ;;
        crit)    prefix="CRITICAL: " ;;
        alert)   prefix="ALERT: " ;;
        emerg)   prefix="EMERGENCY: " ;;
        # info (default) gets no prefix
    esac

    logger -s -t "$_log_tag" -p "user.$level" "${prefix}$*"
    log_to_file "${prefix}$*"
}
```

```bash
# Заменить на:
_log_tag="$(get_script_name -n)"
LOG_FILE="/tmp/vpn_director.log"
MAX_LOG_SIZE=102400  # 100KB

# -------------------------------------------------------------------------------------------------
# log - unified logger with ISO 8601 timestamp
# -------------------------------------------------------------------------------------------------
# Usage:
#   log "message"              # INFO by default
#   log -l LEVEL "message"     # LEVEL = TRACE|DEBUG|INFO|WARN|ERROR
#
# Output:
#   stderr: 2026-01-11T12:35:16 INFO  [module] - message
#   file:   2026-01-11T12:35:16 INFO  [module] - message
#   syslog: INFO  [module] - message (timestamp added by syslog)
# -------------------------------------------------------------------------------------------------
log() {
    local level="INFO"

    # Parse -l LEVEL
    if [ "$1" = "-l" ] && [ -n "$2" ]; then
        level="$2"
        shift 2
    fi

    # Validate and format level (5 chars)
    local level_fmt syslog_pri
    case "$level" in
        TRACE) level_fmt="TRACE"; syslog_pri="notice" ;;
        DEBUG) level_fmt="DEBUG"; syslog_pri="debug" ;;
        INFO)  level_fmt="INFO "; syslog_pri="info" ;;
        WARN)  level_fmt="WARN "; syslog_pri="warning" ;;
        ERROR) level_fmt="ERROR"; syslog_pri="err" ;;
        *)     level_fmt="INFO "; syslog_pri="info" ;;
    esac

    local timestamp=$(date '+%Y-%m-%dT%H:%M:%S')
    local msg_syslog="$level_fmt [$_log_tag] - $*"
    local msg_full="$timestamp $msg_syslog"

    # Rotate log file if needed
    if [ -f "$LOG_FILE" ] && [ "$(wc -c < "$LOG_FILE")" -gt "$MAX_LOG_SIZE" ]; then
        mv "$LOG_FILE" "${LOG_FILE}.old"
    fi

    # Output to three destinations
    printf '%s\n' "$msg_full" >&2
    printf '%s\n' "$msg_full" >> "$LOG_FILE"
    logger -t "$_log_tag" -p "user.$syslog_pri" "$msg_syslog"
}
```

**Step 2: Обновить документацию функции в заголовке файла**

Найти строки 23-26:

```bash
#   log [-l <level>] <message...>
#       Lightweight syslog wrapper. Logs to both syslog (user facility) and stderr.
#       Supports priority levels with optional -l flag.
```

Заменить на:

```bash
#   log [-l <level>] <message...>
#       Unified logger with ISO 8601 timestamp. Outputs to syslog, stderr, and log file.
#       Levels: TRACE, DEBUG, INFO (default), WARN, ERROR
```

**Step 3: Проверить синтаксис**

Run: `ash -n jffs/scripts/vpn-director/utils/common.sh`
Expected: No output (success)

**Step 4: Commit**

```bash
git add jffs/scripts/vpn-director/utils/common.sh
git commit -m "refactor(log): new format with ISO 8601 timestamp and TRACE/DEBUG/INFO/WARN/ERROR levels"
```

---

## Task 2: Обновить import_server_list.sh

**Files:**
- Modify: `jffs/scripts/vpn-director/import_server_list.sh`

**Step 1: Заменить уровни логирования**

| Строка | Было | Стало |
|--------|------|-------|
| 80 | `log -l err` | `log -l ERROR` |
| 92 | `log -l notice` | `log -l TRACE` |
| 101 | `log -l err` | `log -l ERROR` |
| 110 | `log -l err` | `log -l ERROR` |
| 116 | `log -l err` | `log -l ERROR` |
| 125 | `log -l err` | `log -l ERROR` |
| 133 | `log -l err` | `log -l ERROR` |
| 146 | `log -l notice` | `log -l TRACE` |
| 156 | `log -l debug` | `log -l DEBUG` |
| 164 | `log -l debug` | `log -l DEBUG` |
| 168 | `log -l debug` | `log -l DEBUG` |
| 169 | `log -l warn` | `log -l WARN` |
| 175 | `log -l debug` | `log -l DEBUG` |
| 176 | `log -l warn` | `log -l WARN` |
| 184 | `log -l debug` | `log -l DEBUG` |
| 185 | `log -l warn` | `log -l WARN` |
| 189 | `log -l debug` | `log -l DEBUG` |
| 216 | `log -l err` | `log -l ERROR` |
| 224 | `log -l err` | `log -l ERROR` |
| 237 | `log -l notice` | `log -l TRACE` |
| 251 | `log -l err` | `log -l ERROR` |
| 273 | `log -l notice` | `log -l TRACE` |
| 280 | `log -l notice` | `log -l TRACE` |

Команды замены:

```bash
sed -i 's/log -l err/log -l ERROR/g' jffs/scripts/vpn-director/import_server_list.sh
sed -i 's/log -l notice/log -l TRACE/g' jffs/scripts/vpn-director/import_server_list.sh
sed -i 's/log -l debug/log -l DEBUG/g' jffs/scripts/vpn-director/import_server_list.sh
sed -i 's/log -l warn/log -l WARN/g' jffs/scripts/vpn-director/import_server_list.sh
```

**Step 2: Проверить результат**

Run: `grep -n "log -l [a-z]" jffs/scripts/vpn-director/import_server_list.sh`
Expected: No output (все lowercase уровни заменены)

**Step 3: Проверить синтаксис**

Run: `ash -n jffs/scripts/vpn-director/import_server_list.sh`
Expected: No output (success)

**Step 4: Commit**

```bash
git add jffs/scripts/vpn-director/import_server_list.sh
git commit -m "refactor(import): update log levels to new format"
```

---

## Task 3: Обновить tunnel_director.sh

**Files:**
- Modify: `jffs/scripts/vpn-director/tunnel_director.sh`

**Step 1: Заменить уровни логирования**

Все вызовы `log -l warn` → `log -l WARN` (11 мест: строки 317, 324, 339, 343, 350, 356, 368, 380, 395, 419, 464)

```bash
sed -i 's/log -l warn/log -l WARN/g' jffs/scripts/vpn-director/tunnel_director.sh
```

**Step 2: Проверить результат**

Run: `grep -n "log -l [a-z]" jffs/scripts/vpn-director/tunnel_director.sh`
Expected: No output

**Step 3: Проверить синтаксис**

Run: `ash -n jffs/scripts/vpn-director/tunnel_director.sh`
Expected: No output (success)

**Step 4: Commit**

```bash
git add jffs/scripts/vpn-director/tunnel_director.sh
git commit -m "refactor(tunnel): update log levels to new format"
```

---

## Task 4: Обновить ipset_builder.sh

**Files:**
- Modify: `jffs/scripts/vpn-director/ipset_builder.sh`

**Step 1: Заменить уровни логирования**

- `log -l err` → `log -l ERROR` (2 места: строки 116, 272)
- `log -l warn` → `log -l WARN` (4 места: строки 201, 287, 515, 527, 546)

```bash
sed -i 's/log -l err/log -l ERROR/g' jffs/scripts/vpn-director/ipset_builder.sh
sed -i 's/log -l warn/log -l WARN/g' jffs/scripts/vpn-director/ipset_builder.sh
```

**Step 2: Проверить результат**

Run: `grep -n "log -l [a-z]" jffs/scripts/vpn-director/ipset_builder.sh`
Expected: No output

**Step 3: Проверить синтаксис**

Run: `ash -n jffs/scripts/vpn-director/ipset_builder.sh`
Expected: No output (success)

**Step 4: Commit**

```bash
git add jffs/scripts/vpn-director/ipset_builder.sh
git commit -m "refactor(ipset): update log levels to new format"
```

---

## Task 5: Обновить xray_tproxy.sh

**Files:**
- Modify: `jffs/scripts/vpn-director/xray_tproxy.sh`

**Step 1: Заменить уровни логирования**

- `log -l err` → `log -l ERROR` (2 места: строки 54, 218)
- `log -l warn` → `log -l WARN` (5 мест: строки 88, 143, 167, 308, 309, 321)

```bash
sed -i 's/log -l err/log -l ERROR/g' jffs/scripts/vpn-director/xray_tproxy.sh
sed -i 's/log -l warn/log -l WARN/g' jffs/scripts/vpn-director/xray_tproxy.sh
```

**Step 2: Проверить результат**

Run: `grep -n "log -l [a-z]" jffs/scripts/vpn-director/xray_tproxy.sh`
Expected: No output

**Step 3: Проверить синтаксис**

Run: `ash -n jffs/scripts/vpn-director/xray_tproxy.sh`
Expected: No output (success)

**Step 4: Commit**

```bash
git add jffs/scripts/vpn-director/xray_tproxy.sh
git commit -m "refactor(xray): update log levels to new format"
```

---

## Task 6: Обновить utils/firewall.sh

**Files:**
- Modify: `jffs/scripts/vpn-director/utils/firewall.sh`

**Step 1: Заменить уровни логирования**

Все вызовы `log -l err` → `log -l ERROR` (18 мест)

```bash
sed -i 's/log -l err/log -l ERROR/g' jffs/scripts/vpn-director/utils/firewall.sh
```

**Step 2: Проверить результат**

Run: `grep -n "log -l [a-z]" jffs/scripts/vpn-director/utils/firewall.sh`
Expected: No output

**Step 3: Проверить синтаксис**

Run: `ash -n jffs/scripts/vpn-director/utils/firewall.sh`
Expected: No output (success)

**Step 4: Commit**

```bash
git add jffs/scripts/vpn-director/utils/firewall.sh
git commit -m "refactor(firewall): update log levels to new format"
```

---

## Task 7: Обновить utils/send-email.sh

**Files:**
- Modify: `jffs/scripts/vpn-director/utils/send-email.sh`

**Step 1: Заменить уровни логирования**

- `log -l err` → `log -l ERROR` (4 места: строки 48, 73, 79, 125)
- `log -l warn` → `log -l WARN` (1 место: строка 122)

```bash
sed -i 's/log -l err/log -l ERROR/g' jffs/scripts/vpn-director/utils/send-email.sh
sed -i 's/log -l warn/log -l WARN/g' jffs/scripts/vpn-director/utils/send-email.sh
```

**Step 2: Проверить результат**

Run: `grep -n "log -l [a-z]" jffs/scripts/vpn-director/utils/send-email.sh`
Expected: No output

**Step 3: Проверить синтаксис**

Run: `ash -n jffs/scripts/vpn-director/utils/send-email.sh`
Expected: No output (success)

**Step 4: Commit**

```bash
git add jffs/scripts/vpn-director/utils/send-email.sh
git commit -m "refactor(email): update log levels to new format"
```

---

## Task 8: Обновить utils/shared.sh

**Files:**
- Modify: `jffs/scripts/vpn-director/utils/shared.sh`

**Step 1: Заменить уровни логирования**

`log -l notice` → `log -l TRACE` (1 место: строка 38)

```bash
sed -i 's/log -l notice/log -l TRACE/g' jffs/scripts/vpn-director/utils/shared.sh
```

**Step 2: Проверить результат**

Run: `grep -n "log -l [a-z]" jffs/scripts/vpn-director/utils/shared.sh`
Expected: No output

**Step 3: Проверить синтаксис**

Run: `ash -n jffs/scripts/vpn-director/utils/shared.sh`
Expected: No output (success)

**Step 4: Commit**

```bash
git add jffs/scripts/vpn-director/utils/shared.sh
git commit -m "refactor(shared): update log levels to new format"
```

---

## Task 9: Обновить utils/common.sh (resolve_ip, resolve_lan_ip)

**Files:**
- Modify: `jffs/scripts/vpn-director/utils/common.sh:558,570,611,628`

**Step 1: Заменить оставшиеся уровни**

`log -l err` → `log -l ERROR` (4 места в resolve_ip и resolve_lan_ip)

```bash
sed -i 's/log -l err/log -l ERROR/g' jffs/scripts/vpn-director/utils/common.sh
```

**Step 2: Проверить результат**

Run: `grep -n "log -l [a-z]" jffs/scripts/vpn-director/utils/common.sh`
Expected: No output

**Step 3: Проверить синтаксис**

Run: `ash -n jffs/scripts/vpn-director/utils/common.sh`
Expected: No output (success)

**Step 4: Commit**

```bash
git add jffs/scripts/vpn-director/utils/common.sh
git commit -m "refactor(common): update remaining log levels to new format"
```

---

## Task 10: Обновить документацию

**Files:**
- Modify: `CLAUDE.md`
- Modify: `.claude/rules/shell-conventions.md`

**Step 1: Обновить CLAUDE.md**

Найти строку 62:
```markdown
- Logging: `log -l err|warn|info|notice "message"`
```

Заменить на:
```markdown
- Logging: `log -l ERROR|WARN|INFO|DEBUG|TRACE "message"`
```

**Step 2: Обновить shell-conventions.md**

Найти строку с `log -l err|warn|info|notice`:
```markdown
- Logging: `log -l err|warn|info|notice "message"`
```

Заменить на:
```markdown
- Logging: `log -l ERROR|WARN|INFO|DEBUG|TRACE "message"` (default: INFO)
```

**Step 3: Commit**

```bash
git add CLAUDE.md .claude/rules/shell-conventions.md
git commit -m "docs: update logging documentation for new format"
```

---

## Task 11: Финальная проверка

**Step 1: Убедиться, что нет старых уровней**

Run: `grep -rn "log -l [a-z]" jffs/scripts/vpn-director/`
Expected: No output

**Step 2: Проверить синтаксис всех скриптов**

```bash
for f in jffs/scripts/vpn-director/*.sh jffs/scripts/vpn-director/utils/*.sh; do
    ash -n "$f" || echo "FAIL: $f"
done
```
Expected: No output (all pass)

**Step 3: Тест функции log**

```bash
ash -c '. jffs/scripts/vpn-director/utils/common.sh; log "test info"; log -l ERROR "test error"; log -l TRACE "test trace"'
```
Expected: Три строки с правильным форматом на stderr

**Step 4: Финальный коммит (если были исправления)**

Если обнаружены проблемы и внесены исправления:
```bash
git add -A
git commit -m "fix: address issues found during final verification"
```
