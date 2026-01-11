# Logging Format Redesign

**Goal:** Улучшить формат логирования в функции `log` из `common.sh` — ISO 8601 timestamp, фиксированные уровни, единый формат.

## Формат вывода

**Три места вывода:**

| Вывод | Формат | Пример |
|-------|--------|--------|
| stderr | полный | `2026-01-09T12:35:16 INFO  [module] - message` |
| файл | полный | `2026-01-09T12:35:16 INFO  [module] - message` |
| syslog | без timestamp | `INFO  [module] - message` |

**Компоненты:**

1. **Timestamp** — ISO 8601 с секундами: `2026-01-09T12:35:16`
2. **Level** — 5 символов с пробелами: `TRACE`, `DEBUG`, `INFO `, `WARN `, `ERROR`
3. **Module** — имя скрипта без расширения: `[ipset_builder]`
4. **Separator** — ` - `
5. **Message** — текст сообщения

## API

```bash
log "message"              # INFO по умолчанию
log -l TRACE "message"
log -l DEBUG "message"
log -l INFO "message"
log -l WARN "message"
log -l ERROR "message"
```

**Уровни и маппинг на syslog priority:**

| Уровень | Syslog priority | Назначение |
|---------|-----------------|------------|
| TRACE | user.notice | Информационные заметки, этапы выполнения |
| DEBUG | user.debug | Отладочная информация |
| INFO | user.info | Стандартные сообщения о ходе работы |
| WARN | user.warning | Предупреждения, некритичные проблемы |
| ERROR | user.err | Ошибки |

**Валидация:** неизвестный уровень → INFO с предупреждением в stderr.

## Реализация

```bash
log() {
    local level="INFO"

    # Парсинг -l LEVEL
    if [ "$1" = "-l" ] && [ -n "$2" ]; then
        level="$2"
        shift 2
    fi

    # Валидация и форматирование уровня (5 символов)
    local level_fmt syslog_pri
    case "$level" in
        TRACE) level_fmt="TRACE"; syslog_pri="notice" ;;
        DEBUG) level_fmt="DEBUG"; syslog_pri="debug" ;;
        INFO)  level_fmt="INFO "; syslog_pri="info" ;;
        WARN)  level_fmt="WARN "; syslog_pri="warning" ;;
        ERROR) level_fmt="ERROR"; syslog_pri="err" ;;
        *)     level_fmt="INFO "; syslog_pri="info" ;;  # fallback
    esac

    local timestamp=$(date '+%Y-%m-%dT%H:%M:%S')
    local msg_syslog="$level_fmt [$_log_tag] - $*"
    local msg_full="$timestamp $msg_syslog"

    # Вывод в три места
    printf '%s\n' "$msg_full" >&2              # stderr
    printf '%s\n' "$msg_full" >> "$LOG_FILE"   # файл
    logger -t "$_log_tag" -p "user.$syslog_pri" "$msg_syslog"  # syslog (без -s)
}
```

## Маппинг старых вызовов

| Старый вызов | Новый вызов |
|--------------|-------------|
| `log "message"` | `log "message"` |
| `log -l debug "..."` | `log -l DEBUG "..."` |
| `log -l info "..."` | `log -l INFO "..."` |
| `log -l notice "..."` | `log -l TRACE "..."` |
| `log -l warn "..."` | `log -l WARN "..."` |
| `log -l err "..."` | `log -l ERROR "..."` |
| `log -l crit/alert/emerg "..."` | `log -l ERROR "..."` |

## Файлы для обновления

1. `jffs/scripts/vpn-director/utils/common.sh` — функция `log`
2. Все скрипты с вызовами `log -l`:
   - `import_server_list.sh`
   - `ipset_builder.sh`
   - `tunnel_director.sh`
   - `xray_tproxy.sh`
   - `configure.sh`
   - Утилиты в `utils/`
3. Документация:
   - `CLAUDE.md`
   - `.claude/rules/shell-conventions.md`

## Пример до/после

```
# Было:
2026-01-11 01:51:28 [xray_tproxy] ERROR: Failed to load module

# Стало:
2026-01-11T01:51:28 ERROR [xray_tproxy] - Failed to load module
```
