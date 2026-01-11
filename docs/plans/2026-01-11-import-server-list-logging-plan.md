# Рефакторинг логгирования import_server_list.sh

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Заменить устаревший debug-логгинг через printf в отдельный файл на стандартный `log -l debug` из common.sh.

**Architecture:** Удаляем LOG_FILE и все printf в него, заменяем на вызовы `log -l debug`. Debug-сообщения будут идти в syslog.

**Tech Stack:** Shell (ash), common.sh utilities

---

## Task 1: Удалить LOG_FILE и связанные строки

**Files:**
- Modify: `jffs/scripts/vpn-director/import_server_list.sh:150-157`

**Step 1: Удалить объявление LOG_FILE и его использование**

Удалить строки 150, 156, 157:

```sh
# Строка 150 - удалить:
LOG_FILE="/tmp/import_vless_debug.log"

# Строка 156 - удалить:
: > "$LOG_FILE"

# Строка 157 - удалить:
log "Debug log: $LOG_FILE"
```

**Step 2: Удалить вывод финального JSON в debug-лог**

Удалить строки 223-224:

```sh
# Строки 223-224 - удалить:
printf "[DEBUG] Final JSON:\n" >> "$LOG_FILE"
cat "$SERVERS_FILE" >> "$LOG_FILE"
```

**Step 3: Проверить синтаксис**

Run: `ash -n jffs/scripts/vpn-director/import_server_list.sh`
Expected: нет вывода (синтаксис валиден)

**Step 4: Commit**

```bash
git add jffs/scripts/vpn-director/import_server_list.sh
git commit -m "refactor(import): remove separate LOG_FILE for debug output"
```

---

## Task 2: Заменить printf на log -l debug (первые 3 места)

**Files:**
- Modify: `jffs/scripts/vpn-director/import_server_list.sh`

**Step 1: Заменить debug-вывод URI (бывшая строка 162)**

Было:
```sh
printf "[DEBUG] URI: %.80s...\n" "$uri" >> "$LOG_FILE"
```

Станет:
```sh
log -l debug "URI: ${uri%%#*}"
```

Примечание: `${uri%%#*}` обрезает имя сервера после #, чтобы не логировать слишком длинную строку.

**Step 2: Заменить debug-вывод parsed (бывшие строки 171-172)**

Было:
```sh
printf "[DEBUG] Parsed: server=%s port=%s uuid=%s name=%s\n" \
    "$server" "$port" "$uuid" "$name" >> "$LOG_FILE"
```

Станет:
```sh
log -l debug "Parsed: server=$server port=$port uuid=$uuid name=$name"
```

**Step 3: Заменить debug-вывод skip missing field (бывшая строка 176)**

Было:
```sh
printf "[DEBUG] SKIP: missing required field\n" >> "$LOG_FILE"
```

Станет:
```sh
log -l debug "SKIP: missing required field"
```

**Step 4: Проверить синтаксис**

Run: `ash -n jffs/scripts/vpn-director/import_server_list.sh`
Expected: нет вывода

**Step 5: Commit**

```bash
git add jffs/scripts/vpn-director/import_server_list.sh
git commit -m "refactor(import): replace debug printf with log -l debug (part 1)"
```

---

## Task 3: Заменить printf на log -l debug (оставшиеся 3 места)

**Files:**
- Modify: `jffs/scripts/vpn-director/import_server_list.sh`

**Step 1: Заменить debug-вывод invalid port (бывшая строка 183)**

Было:
```sh
printf "[DEBUG] SKIP: invalid port '%s'\n" "$port" >> "$LOG_FILE"
```

Станет:
```sh
log -l debug "SKIP: invalid port '$port'"
```

**Step 2: Заменить debug-вывод cannot resolve (бывшая строка 192)**

Было:
```sh
printf "[DEBUG] SKIP: cannot resolve %s\n" "$server" >> "$LOG_FILE"
```

Станет:
```sh
log -l debug "SKIP: cannot resolve $server"
```

**Step 3: Заменить debug-вывод resolved (бывшая строка 197)**

Было:
```sh
printf "[DEBUG] Resolved: %s -> %s\n" "$server" "$ip" >> "$LOG_FILE"
```

Станет:
```sh
log -l debug "Resolved: $server -> $ip"
```

**Step 4: Проверить синтаксис**

Run: `ash -n jffs/scripts/vpn-director/import_server_list.sh`
Expected: нет вывода

**Step 5: Commit**

```bash
git add jffs/scripts/vpn-director/import_server_list.sh
git commit -m "refactor(import): replace debug printf with log -l debug (part 2)"
```

---

## Verification

**Финальная проверка синтаксиса:**
```bash
ash -n jffs/scripts/vpn-director/import_server_list.sh
```

**Проверка отсутствия LOG_FILE:**
```bash
grep -n "LOG_FILE" jffs/scripts/vpn-director/import_server_list.sh
# Ожидается: пусто
```

**Проверка отсутствия printf с DEBUG:**
```bash
grep -n "printf.*DEBUG" jffs/scripts/vpn-director/import_server_list.sh
# Ожидается: пусто
```

**Проверка наличия log -l debug:**
```bash
grep -n "log -l debug" jffs/scripts/vpn-director/import_server_list.sh
# Ожидается: 6 строк
```
