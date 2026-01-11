# Рефакторинг логгирования в import_server_list.sh

## Проблема

В `jffs/scripts/vpn-director/import_server_list.sh` остался устаревший механизм debug-логгирования:
- `LOG_FILE="/tmp/import_vless_debug.log"` — отдельный файл для debug-вывода
- Множество `printf "[DEBUG] ..." >> "$LOG_FILE"` вместо стандартного `log`

Это несовместимо с остальными скриптами проекта, которые используют `log -l debug` из `common.sh`.

## Решение

Заменить все debug-printf на `log -l debug`, удалить отдельный LOG_FILE.

## Изменения

### Удаляем (4 места)

| Строка | Код |
|--------|-----|
| 150 | `LOG_FILE="/tmp/import_vless_debug.log"` |
| 156 | `: > "$LOG_FILE"` |
| 157 | `log "Debug log: $LOG_FILE"` |
| 223-224 | `printf "[DEBUG] Final JSON:\n" >> "$LOG_FILE"` + `cat "$SERVERS_FILE" >> "$LOG_FILE"` |

### Заменяем printf → log -l debug (6 мест)

| Строка | Было | Станет |
|--------|------|--------|
| 162 | `printf "[DEBUG] URI: %.80s...\n" "$uri" >> "$LOG_FILE"` | `log -l debug "URI: ${uri%#*}"` |
| 171-172 | `printf "[DEBUG] Parsed: server=%s port=%s uuid=%s name=%s\n" ...` | `log -l debug "Parsed: server=$server port=$port uuid=$uuid name=$name"` |
| 176 | `printf "[DEBUG] SKIP: missing required field\n" >> "$LOG_FILE"` | `log -l debug "SKIP: missing required field"` |
| 183 | `printf "[DEBUG] SKIP: invalid port '%s'\n" "$port" >> "$LOG_FILE"` | `log -l debug "SKIP: invalid port '$port'"` |
| 192 | `printf "[DEBUG] SKIP: cannot resolve %s\n" "$server" >> "$LOG_FILE"` | `log -l debug "SKIP: cannot resolve $server"` |
| 197 | `printf "[DEBUG] Resolved: %s -> %s\n" "$server" "$ip" >> "$LOG_FILE"` | `log -l debug "Resolved: $server -> $ip"` |

## Результат

- Debug-сообщения идут в syslog (видны через `logread | grep DEBUG`)
- Нет отдельного файла `/tmp/import_vless_debug.log`
- Код консистентен с остальными скриптами проекта
