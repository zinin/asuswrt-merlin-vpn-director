# Unified JSON Config Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Объединить config-xray.sh и config-tunnel-director.sh в единый vpn-director.json с загрузкой через jq.

**Architecture:** Создаём utils/config.sh, который читает JSON через jq и экспортирует переменные. Скрипты подключают config.sh вместо старых конфигов. Массивы преобразуются в строки через пробел для итерации через `for x in $VAR`.

**Tech Stack:** ash shell, jq 1.7.1, JSON

---

## Task 1: Создать utils/config.sh

**Files:**
- Create: `jffs/scripts/vpn-director/utils/config.sh`

**Step 1: Создать файл config.sh**

```bash
#!/usr/bin/env ash

###################################################################################################
# config.sh - load vpn-director.json and export variables
###################################################################################################

# shellcheck disable=SC2034

set -euo pipefail

###################################################################################################
# 1. Configuration file path
###################################################################################################
VPD_CONFIG_FILE="/jffs/scripts/vpn-director/vpn-director.json"

###################################################################################################
# 2. Validate config exists and is valid JSON
###################################################################################################
if [ ! -f "$VPD_CONFIG_FILE" ]; then
    echo "ERROR: Config not found: $VPD_CONFIG_FILE" >&2
    exit 1
fi

if ! jq empty "$VPD_CONFIG_FILE" 2>/dev/null; then
    echo "ERROR: Invalid JSON: $VPD_CONFIG_FILE" >&2
    exit 1
fi

###################################################################################################
# 3. Helper functions
###################################################################################################
_cfg() { jq -r "$1 // empty" "$VPD_CONFIG_FILE"; }
_cfg_arr() { jq -r "$1 // [] | .[]" "$VPD_CONFIG_FILE" | tr '\n' ' ' | sed 's/ $//'; }

###################################################################################################
# 4. Tunnel Director variables
###################################################################################################
TUN_DIR_RULES=$(_cfg_arr '.tunnel_director.rules')
IPS_BDR_DIR=$(_cfg '.tunnel_director.ipset_dump_dir')

###################################################################################################
# 5. Xray variables
###################################################################################################
XRAY_CLIENTS=$(_cfg_arr '.xray.clients')
XRAY_SERVERS=$(_cfg_arr '.xray.servers')
XRAY_EXCLUDE_SETS=$(_cfg_arr '.xray.exclude_sets')

###################################################################################################
# 6. Advanced: Xray
###################################################################################################
XRAY_TPROXY_PORT=$(_cfg '.advanced.xray.tproxy_port')
XRAY_ROUTE_TABLE=$(_cfg '.advanced.xray.route_table')
XRAY_RULE_PREF=$(_cfg '.advanced.xray.rule_pref')
XRAY_FWMARK=$(_cfg '.advanced.xray.fwmark')
XRAY_FWMARK_MASK=$(_cfg '.advanced.xray.fwmark_mask')
XRAY_CHAIN=$(_cfg '.advanced.xray.chain')
XRAY_CLIENTS_IPSET=$(_cfg '.advanced.xray.clients_ipset')
XRAY_SERVERS_IPSET=$(_cfg '.advanced.xray.servers_ipset')

###################################################################################################
# 7. Advanced: Tunnel Director
###################################################################################################
TUN_DIR_CHAIN_PREFIX=$(_cfg '.advanced.tunnel_director.chain_prefix')
TUN_DIR_PREF_BASE=$(_cfg '.advanced.tunnel_director.pref_base')
TUN_DIR_MARK_MASK=$(_cfg '.advanced.tunnel_director.mark_mask')
TUN_DIR_MARK_SHIFT=$(_cfg '.advanced.tunnel_director.mark_shift')

###################################################################################################
# 8. Advanced: Boot
###################################################################################################
MIN_BOOT_TIME=$(_cfg '.advanced.boot.min_time')
BOOT_WAIT_DELAY=$(_cfg '.advanced.boot.wait_delay')

###################################################################################################
# 9. Make all variables read-only
###################################################################################################
readonly \
    VPD_CONFIG_FILE \
    TUN_DIR_RULES IPS_BDR_DIR \
    XRAY_CLIENTS XRAY_SERVERS XRAY_EXCLUDE_SETS \
    XRAY_TPROXY_PORT XRAY_ROUTE_TABLE XRAY_RULE_PREF \
    XRAY_FWMARK XRAY_FWMARK_MASK XRAY_CHAIN \
    XRAY_CLIENTS_IPSET XRAY_SERVERS_IPSET \
    TUN_DIR_CHAIN_PREFIX TUN_DIR_PREF_BASE \
    TUN_DIR_MARK_MASK TUN_DIR_MARK_SHIFT \
    MIN_BOOT_TIME BOOT_WAIT_DELAY
```

**Step 2: Проверить синтаксис**

Run: `ash -n jffs/scripts/vpn-director/utils/config.sh`
Expected: No output (syntax OK)

**Step 3: Commit**

```bash
git add jffs/scripts/vpn-director/utils/config.sh
git commit -m "feat: add config.sh to load JSON config via jq"
```

---

## Task 2: Создать vpn-director.json.template

**Files:**
- Create: `jffs/scripts/vpn-director/vpn-director.json.template`

**Step 1: Создать шаблон**

```json
{
  "tunnel_director": {
    "rules": [],
    "ipset_dump_dir": "/jffs/ipset_builder"
  },
  "xray": {
    "clients": [],
    "servers": [],
    "exclude_sets": ["ru"]
  },
  "advanced": {
    "xray": {
      "tproxy_port": 12345,
      "route_table": 100,
      "rule_pref": 200,
      "fwmark": "0x100",
      "fwmark_mask": "0x100",
      "chain": "XRAY_TPROXY",
      "clients_ipset": "XRAY_CLIENTS",
      "servers_ipset": "XRAY_SERVERS"
    },
    "tunnel_director": {
      "chain_prefix": "TUN_DIR_",
      "pref_base": 16384,
      "mark_mask": "0x00ff0000",
      "mark_shift": 16
    },
    "boot": {
      "min_time": 120,
      "wait_delay": 60
    }
  }
}
```

**Step 2: Валидировать JSON**

Run: `jq empty jffs/scripts/vpn-director/vpn-director.json.template`
Expected: No output (valid JSON)

**Step 3: Commit**

```bash
git add jffs/scripts/vpn-director/vpn-director.json.template
git commit -m "feat: add vpn-director.json.template"
```

---

## Task 3: Обновить xray_tproxy.sh

**Files:**
- Modify: `jffs/scripts/vpn-director/xray_tproxy.sh:35` (source)
- Modify: `jffs/scripts/vpn-director/xray_tproxy.sh:140-146` (XRAY_CLIENTS iteration)
- Modify: `jffs/scripts/vpn-director/xray_tproxy.sh:165-171` (XRAY_SERVERS iteration)

**Step 1: Заменить source конфига**

Строка 35, заменить:
```bash
. /jffs/scripts/vpn-director/configs/config-xray.sh
```
на:
```bash
. /jffs/scripts/vpn-director/utils/config.sh
```

**Step 2: Изменить итерацию XRAY_CLIENTS в setup_clients_ipset()**

Строки 140-146, заменить:
```bash
    strip_comments "$XRAY_CLIENTS" | while IFS= read -r ip; do
        [ -n "$ip" ] || continue
        ipset add "$XRAY_CLIENTS_IPSET" "$ip" 2>/dev/null || {
            log -l warn "Failed to add $ip to $XRAY_CLIENTS_IPSET"
            warnings=1
        }
    done
```
на:
```bash
    for ip in $XRAY_CLIENTS; do
        [ -n "$ip" ] || continue
        ipset add "$XRAY_CLIENTS_IPSET" "$ip" 2>/dev/null || {
            log -l warn "Failed to add $ip to $XRAY_CLIENTS_IPSET"
        }
    done
```

**Step 3: Изменить итерацию XRAY_SERVERS в setup_servers_ipset()**

Строки 165-171, заменить:
```bash
    strip_comments "$XRAY_SERVERS" | while IFS= read -r ip; do
        [ -n "$ip" ] || continue
        ipset add "$XRAY_SERVERS_IPSET" "$ip" 2>/dev/null || {
            log -l warn "Failed to add $ip to $XRAY_SERVERS_IPSET"
            warnings=1
        }
    done
```
на:
```bash
    for ip in $XRAY_SERVERS; do
        [ -n "$ip" ] || continue
        ipset add "$XRAY_SERVERS_IPSET" "$ip" 2>/dev/null || {
            log -l warn "Failed to add $ip to $XRAY_SERVERS_IPSET"
        }
    done
```

**Step 4: Проверить синтаксис**

Run: `ash -n jffs/scripts/vpn-director/xray_tproxy.sh`
Expected: No output (syntax OK)

**Step 5: Commit**

```bash
git add jffs/scripts/vpn-director/xray_tproxy.sh
git commit -m "refactor(xray_tproxy): use unified JSON config"
```

---

## Task 4: Обновить tunnel_director.sh

**Files:**
- Modify: `jffs/scripts/vpn-director/tunnel_director.sh:58` (source)
- Modify: `jffs/scripts/vpn-director/tunnel_director.sh:181` (rules normalization)

**Step 1: Заменить source конфига**

Строка 58, заменить:
```bash
. /jffs/scripts/vpn-director/configs/config-tunnel-director.sh
```
на:
```bash
. /jffs/scripts/vpn-director/utils/config.sh
```

**Step 2: Изменить нормализацию правил**

Строка 181, заменить:
```bash
strip_comments "$TUN_DIR_RULES" | sed -E 's/[[:blank:]]+//g' > "$tun_dir_rules"
```
на:
```bash
printf '%s\n' $TUN_DIR_RULES > "$tun_dir_rules"
```

Примечание: `$TUN_DIR_RULES` без кавычек — shell разбивает по пробелам, `printf '%s\n'` пишет каждое правило на отдельной строке.

**Step 3: Проверить синтаксис**

Run: `ash -n jffs/scripts/vpn-director/tunnel_director.sh`
Expected: No output (syntax OK)

**Step 4: Commit**

```bash
git add jffs/scripts/vpn-director/tunnel_director.sh
git commit -m "refactor(tunnel_director): use unified JSON config"
```

---

## Task 5: Обновить ipset_builder.sh

**Files:**
- Modify: `jffs/scripts/vpn-director/ipset_builder.sh:63-69` (source configs)
- Modify: `jffs/scripts/vpn-director/ipset_builder.sh:223-224` (save_hashes)
- Modify: `jffs/scripts/vpn-director/ipset_builder.sh:341` (parse_country_codes call)
- Modify: `jffs/scripts/vpn-director/ipset_builder.sh:394-395` (XRAY_EXCLUDE_SETS)
- Modify: `jffs/scripts/vpn-director/ipset_builder.sh:450` (parse_combo_from_rules call)

**Step 1: Заменить source конфигов**

Строки 63-69, заменить:
```bash
. /jffs/scripts/vpn-director/configs/config-tunnel-director.sh

# Load Xray config for XRAY_EXCLUDE_SETS (optional)
XRAY_CONFIG="/jffs/scripts/vpn-director/configs/config-xray.sh"
if [ -f "$XRAY_CONFIG" ]; then
    . "$XRAY_CONFIG"
fi
```
на:
```bash
. /jffs/scripts/vpn-director/utils/config.sh
```

**Step 2: Изменить save_hashes()**

Строки 219-225, заменить:
```bash
save_hashes() {
    local tun_dir_rules
    tun_dir_rules=$(tmp_file)

    strip_comments "$TUN_DIR_RULES" | sed -E 's/[[:blank:]]+//g' > "$tun_dir_rules"
    printf '%s\n' "$(compute_hash "$tun_dir_rules")" > "$TUN_DIR_IPSETS_HASH"
}
```
на:
```bash
save_hashes() {
    local tun_dir_rules_file
    tun_dir_rules_file=$(tmp_file)

    printf '%s\n' $TUN_DIR_RULES > "$tun_dir_rules_file"
    printf '%s\n' "$(compute_hash "$tun_dir_rules_file")" > "$TUN_DIR_IPSETS_HASH"
}
```

**Step 3: Обновить parse_country_codes()**

Функция `parse_country_codes()` принимает многострочный текст. Нужно изменить вызов.

Строка 384, заменить:
```bash
    tun_cc="$(parse_country_codes "$TUN_DIR_RULES")"
```
на:
```bash
    tun_cc="$(printf '%s\n' $TUN_DIR_RULES | parse_country_codes)"
```

И изменить саму функцию (строки 338-379), заменить сигнатуру:
```bash
parse_country_codes() {
    local rules="$1"

    strip_comments "$rules" |
```
на:
```bash
parse_country_codes() {
    # Reads rules from stdin
```

**Step 4: Обновить использование XRAY_EXCLUDE_SETS**

Строки 393-395, заменить:
```bash
    xray_cc=""
    if [ -n "${XRAY_EXCLUDE_SETS:-}" ]; then
        xray_cc="$(printf '%s' "$XRAY_EXCLUDE_SETS" | tr ',' ' ')"
    fi
```
на:
```bash
    xray_cc="${XRAY_EXCLUDE_SETS:-}"
```

Примечание: XRAY_EXCLUDE_SETS теперь уже через пробелы, tr не нужен.

**Step 5: Обновить parse_combo_from_rules()**

Функция принимает многострочный текст. Нужно изменить вызов.

Строка 500, заменить:
```bash
    combo_ipsets="$(parse_combo_from_rules "$TUN_DIR_RULES" | awk 'NF')"
```
на:
```bash
    combo_ipsets="$(printf '%s\n' $TUN_DIR_RULES | parse_combo_from_rules | awk 'NF')"
```

И изменить саму функцию (строки 446-483), заменить сигнатуру:
```bash
parse_combo_from_rules() {
    local rules_text="$1"

    strip_comments "$rules_text" |
```
на:
```bash
parse_combo_from_rules() {
    # Reads rules from stdin
```

**Step 6: Проверить синтаксис**

Run: `ash -n jffs/scripts/vpn-director/ipset_builder.sh`
Expected: No output (syntax OK)

**Step 7: Commit**

```bash
git add jffs/scripts/vpn-director/ipset_builder.sh
git commit -m "refactor(ipset_builder): use unified JSON config"
```

---

## Task 6: Обновить configure.sh

**Files:**
- Modify: `jffs/scripts/vpn-director/configure.sh`

**Step 1: Обновить step_generate_configs()**

Заменить генерацию shell-конфигов на генерацию JSON.

Строки 417-461, заменить функцию `step_generate_configs()`:

```bash
step_generate_configs() {
    print_header "Step 7: Generating Configs"

    # Generate xray/config.json from template
    print_info "Generating Xray config..."

    sed "s|{{XRAY_SERVER_ADDRESS}}|$SELECTED_SERVER_ADDRESS|g" \
        /opt/etc/xray/config.json.template 2>/dev/null | \
        sed "s|{{XRAY_SERVER_PORT}}|$SELECTED_SERVER_PORT|g" | \
        sed "s|{{XRAY_USER_UUID}}|$SELECTED_SERVER_UUID|g" \
        > "$XRAY_CONFIG_DIR/config.json"
    print_success "Generated $XRAY_CONFIG_DIR/config.json"

    # Generate vpn-director.json
    print_info "Generating vpn-director.json..."

    # Build JSON arrays
    xray_clients_json="[]"
    if [ -n "$XRAY_CLIENTS_LIST" ]; then
        xray_clients_json=$(printf '%s' "$XRAY_CLIENTS_LIST" | grep -v '^$' | jq -R . | jq -s .)
    fi

    xray_servers_json="[]"
    if [ -n "$XRAY_SERVERS_IPS" ]; then
        xray_servers_json=$(printf '%s\n' $XRAY_SERVERS_IPS | jq -R . | jq -s .)
    fi

    xray_exclude_json='["ru"]'
    if [ -n "$XRAY_EXCLUDE_SETS_LIST" ]; then
        xray_exclude_json=$(printf '%s\n' ${XRAY_EXCLUDE_SETS_LIST//,/ } | jq -R . | jq -s .)
    fi

    tun_dir_rules_json="[]"
    if [ -n "$TUN_DIR_RULES_LIST" ]; then
        tun_dir_rules_json=$(printf '%s' "$TUN_DIR_RULES_LIST" | grep -v '^$' | jq -R . | jq -s .)
    fi

    # Read template and update with jq
    jq \
        --argjson clients "$xray_clients_json" \
        --argjson servers "$xray_servers_json" \
        --argjson exclude "$xray_exclude_json" \
        --argjson rules "$tun_dir_rules_json" \
        '.xray.clients = $clients |
         .xray.servers = $servers |
         .xray.exclude_sets = $exclude |
         .tunnel_director.rules = $rules' \
        "$JFFS_DIR/vpn-director.json.template" \
        > "$JFFS_DIR/vpn-director.json"

    print_success "Generated $JFFS_DIR/vpn-director.json"
}
```

**Step 2: Обновить проверку шаблона**

В step_generate_configs(), строка 423-427, удалить проверку старого шаблона:
```bash
    if [ ! -f "$JFFS_DIR/configs/config-xray.sh.template" ]; then
        print_error "Template not found: $JFFS_DIR/configs/config-xray.sh.template"
        print_info "Run install.sh first to download required files"
        exit 1
    fi
```

Добавить проверку нового шаблона в начале функции:
```bash
    if [ ! -f "$JFFS_DIR/vpn-director.json.template" ]; then
        print_error "Template not found: $JFFS_DIR/vpn-director.json.template"
        print_info "Run install.sh first to download required files"
        exit 1
    fi
```

**Step 3: Проверить синтаксис**

Run: `sh -n jffs/scripts/vpn-director/configure.sh`
Expected: No output (syntax OK)

**Step 4: Commit**

```bash
git add jffs/scripts/vpn-director/configure.sh
git commit -m "refactor(configure): generate JSON config instead of shell"
```

---

## Task 7: Удалить старые конфиги

**Files:**
- Delete: `jffs/scripts/vpn-director/configs/config-xray.sh.template`
- Delete: `jffs/scripts/vpn-director/configs/config-tunnel-director.sh.template`

**Step 1: Удалить файлы**

```bash
git rm jffs/scripts/vpn-director/configs/config-xray.sh.template
git rm jffs/scripts/vpn-director/configs/config-tunnel-director.sh.template
```

**Step 2: Удалить пустую директорию configs (если пуста)**

```bash
rmdir jffs/scripts/vpn-director/configs 2>/dev/null || true
```

**Step 3: Commit**

```bash
git commit -m "chore: remove old shell config templates"
```

---

## Task 8: Обновить документацию

**Files:**
- Modify: `CLAUDE.md`
- Modify: `.claude/rules/xray-tproxy.md`
- Modify: `.claude/rules/tunnel-director.md`
- Modify: `.claude/rules/ipset-builder.md`

**Step 1: Обновить CLAUDE.md**

В секции "Config Files (after install)", заменить:
```markdown
| Path | Purpose |
|------|---------|
| `/jffs/scripts/vpn-director/configs/config-tunnel-director.sh` | Tunnel Director rules & IPSet Builder settings |
| `/jffs/scripts/vpn-director/configs/config-xray.sh` | Xray TPROXY clients & servers |
| `/opt/etc/xray/config.json` | Xray server configuration |
```
на:
```markdown
| Path | Purpose |
|------|---------|
| `/jffs/scripts/vpn-director/vpn-director.json` | Unified config (Xray + Tunnel Director) |
| `/opt/etc/xray/config.json` | Xray server configuration |
```

**Step 2: Обновить .claude/rules/xray-tproxy.md**

Обновить секцию "Configuration" — указать, что конфиг теперь в vpn-director.json.

**Step 3: Обновить .claude/rules/tunnel-director.md**

Обновить ссылки на config.sh → vpn-director.json.

**Step 4: Обновить .claude/rules/ipset-builder.md**

Обновить ссылки на конфиги.

**Step 5: Commit**

```bash
git add CLAUDE.md .claude/rules/
git commit -m "docs: update for unified JSON config"
```

---

## Task 9: Интеграционное тестирование

**Step 1: Создать тестовый конфиг**

```bash
cp jffs/scripts/vpn-director/vpn-director.json.template /tmp/vpn-director.json
```

Отредактировать /tmp/vpn-director.json, добавить тестовые данные:
```json
{
  "tunnel_director": {
    "rules": ["wgc1:192.168.50.0/24::us,ca"],
    "ipset_dump_dir": "/jffs/ipset_builder"
  },
  "xray": {
    "clients": ["192.168.1.10", "192.168.1.11"],
    "servers": ["1.2.3.4"],
    "exclude_sets": ["ru"]
  },
  ...
}
```

**Step 2: Протестировать config.sh**

```bash
VPD_CONFIG_FILE=/tmp/vpn-director.json ash -c '
  . jffs/scripts/vpn-director/utils/config.sh
  echo "TUN_DIR_RULES: $TUN_DIR_RULES"
  echo "XRAY_CLIENTS: $XRAY_CLIENTS"
  echo "XRAY_TPROXY_PORT: $XRAY_TPROXY_PORT"
'
```

Expected output:
```
TUN_DIR_RULES: wgc1:192.168.50.0/24::us,ca
XRAY_CLIENTS: 192.168.1.10 192.168.1.11
XRAY_TPROXY_PORT: 12345
```

**Step 3: Финальный commit**

```bash
git add -A
git commit -m "feat: complete migration to unified JSON config"
```

---

## Summary

| Task | Description | Files |
|------|-------------|-------|
| 1 | Создать utils/config.sh | Create: utils/config.sh |
| 2 | Создать vpn-director.json.template | Create: vpn-director.json.template |
| 3 | Обновить xray_tproxy.sh | Modify: xray_tproxy.sh |
| 4 | Обновить tunnel_director.sh | Modify: tunnel_director.sh |
| 5 | Обновить ipset_builder.sh | Modify: ipset_builder.sh |
| 6 | Обновить configure.sh | Modify: configure.sh |
| 7 | Удалить старые конфиги | Delete: configs/*.template |
| 8 | Обновить документацию | Modify: CLAUDE.md, .claude/rules/*.md |
| 9 | Интеграционное тестирование | Test all components |
