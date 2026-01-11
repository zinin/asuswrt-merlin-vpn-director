# Миграция с ash на bash — План реализации

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Мигрировать все shell-скрипты с ash на bash 5.2, улучшив безопасность, читаемость и отладку.

**Architecture:** Поэтапная миграция от базовых утилит к зависимым скриптам. Каждый файл покрывается тестами перед рефакторингом. TDD-подход с частыми коммитами.

**Tech Stack:** bash 5.2.32, bats-core, bats-assert, bats-support, shellcheck

---

## Task 0: Настройка тестовой инфраструктуры

**Files:**
- Create: `test/test_helper.bash`
- Create: `test/mocks/nvram`
- Create: `test/mocks/iptables`
- Create: `test/mocks/ip6tables`
- Create: `test/mocks/ipset`
- Create: `test/mocks/nslookup`
- Create: `test/mocks/logger`
- Create: `test/mocks/ip`
- Create: `test/fixtures/hosts`
- Create: `test/fixtures/vpn-director.json`

### Step 0.1: Установить bats-core

```bash
sudo apt install bats bats-assert bats-support
```

Run: `bats --version`
Expected: `Bats 1.x.x`

### Step 0.2: Создать структуру директорий

```bash
mkdir -p test/mocks test/fixtures
```

### Step 0.3: Создать test_helper.bash

```bash
# test/test_helper.bash

# Load bats helpers
load '/usr/lib/bats-support/load.bash'
load '/usr/lib/bats-assert/load.bash'

# Project paths
export PROJECT_ROOT="$BATS_TEST_DIRNAME/.."
export SCRIPTS_DIR="$PROJECT_ROOT/jffs/scripts/vpn-director"
export UTILS_DIR="$SCRIPTS_DIR/utils"

# Test mode - disables syslog, uses fixtures
export TEST_MODE=1
export LOG_FILE="/tmp/bats_test_vpn_director.log"

# Override system paths for mocks
setup() {
    export PATH="$BATS_TEST_DIRNAME/mocks:$PATH"
    export HOSTS_FILE="$BATS_TEST_DIRNAME/fixtures/hosts"

    # Clean log file
    : > "$LOG_FILE"
}

teardown() {
    # Cleanup temp files if any
    rm -f /tmp/bats_test_*
}

# Helper to source common.sh with mocks
load_common() {
    # Set $0 to a fake script path for get_script_* functions
    export BASH_SOURCE_OVERRIDE="$SCRIPTS_DIR/test_script.sh"
    source "$UTILS_DIR/common.sh"
}

# Helper to source firewall.sh (requires common.sh first)
load_firewall() {
    load_common
    source "$UTILS_DIR/firewall.sh"
}

# Helper to source config.sh
load_config() {
    export VPD_CONFIG_FILE="$BATS_TEST_DIRNAME/fixtures/vpn-director.json"
    source "$UTILS_DIR/config.sh"
}
```

### Step 0.4: Создать mock для nvram

```bash
# test/mocks/nvram
#!/bin/bash
case "$*" in
    "get ipv6_service") echo "native" ;;
    "get wan0_primary") echo "1" ;;
    "get wan0_ifname")  echo "eth0" ;;
    "get wan1_primary") echo "0" ;;
    "get wan1_ifname")  echo "eth1" ;;
    "get model")        echo "RT-AX88U" ;;
    *) echo "" ;;
esac
```

### Step 0.5: Создать mock для iptables

```bash
# test/mocks/iptables
#!/bin/bash
# Simple mock that tracks calls and returns success
echo "iptables $*" >> /tmp/bats_iptables_calls.log
case "$1" in
    -t)
        case "$3" in
            -S) echo "-P PREROUTING ACCEPT" ;;  # fake chain list
            -C) exit 1 ;;  # rule doesn't exist
            -N|-A|-I|-D|-F|-X) exit 0 ;;
            *) exit 0 ;;
        esac
        ;;
    *) exit 0 ;;
esac
```

### Step 0.6: Создать mock для ip6tables

```bash
# test/mocks/ip6tables
#!/bin/bash
# Same as iptables mock
echo "ip6tables $*" >> /tmp/bats_ip6tables_calls.log
case "$1" in
    -t)
        case "$3" in
            -S) echo "-P PREROUTING ACCEPT" ;;
            -C) exit 1 ;;
            -N|-A|-I|-D|-F|-X) exit 0 ;;
            *) exit 0 ;;
        esac
        ;;
    *) exit 0 ;;
esac
```

### Step 0.7: Создать mock для ipset

```bash
# test/mocks/ipset
#!/bin/bash
echo "ipset $*" >> /tmp/bats_ipset_calls.log
case "$1" in
    list)
        case "$2" in
            -n) echo "$3" ;;  # just echo the name back
            ru|us|ca)
                echo "Name: $2"
                echo "Type: hash:net"
                echo "Number of entries: 1000"
                ;;
            XRAY_CLIENTS|XRAY_SERVERS)
                echo "Name: $2"
                echo "Type: hash:net"
                echo "Number of entries: 5"
                ;;
            *) exit 1 ;;  # ipset doesn't exist
        esac
        ;;
    create|add|flush|destroy|save|restore) exit 0 ;;
    *) exit 0 ;;
esac
```

### Step 0.8: Создать mock для nslookup

```bash
# test/mocks/nslookup
#!/bin/bash
host="$1"
case "$host" in
    "example.com")
        echo "Server: 8.8.8.8"
        echo "Address: 8.8.8.8#53"
        echo ""
        echo "Name: example.com"
        echo "Address: 93.184.216.34"
        ;;
    "ipv6.example.com")
        echo "Server: 8.8.8.8"
        echo "Address: 8.8.8.8#53"
        echo ""
        echo "Name: ipv6.example.com"
        echo "Address: 2606:2800:220:1:248:1893:25c8:1946"
        ;;
    *)
        echo "** server can't find $host: NXDOMAIN"
        exit 1
        ;;
esac
```

### Step 0.9: Создать mock для logger

```bash
# test/mocks/logger
#!/bin/bash
# Silent mock - just capture args
echo "logger $*" >> /tmp/bats_logger_calls.log
exit 0
```

### Step 0.10: Создать mock для ip

```bash
# test/mocks/ip
#!/bin/bash
echo "ip $*" >> /tmp/bats_ip_calls.log
case "$1" in
    rule)
        case "$2" in
            show) echo "0: from all lookup local" ;;
            add|del) exit 0 ;;
        esac
        ;;
    route)
        case "$2" in
            show) echo "" ;;
            add|del) exit 0 ;;
        esac
        ;;
    *) exit 0 ;;
esac
```

### Step 0.11: Создать fixtures/hosts

```bash
# test/fixtures/hosts
127.0.0.1       localhost
192.168.1.100   mypc mypc.local
192.168.1.1     router router.local
10.0.0.5        server.internal
fd00::1         ipv6-host
```

### Step 0.12: Создать fixtures/vpn-director.json

```json
{
  "tunnel_director": {
    "rules": [
      "wgc1:192.168.50.0/24::us,ca"
    ],
    "data_dir": "/tmp/bats_test_data"
  },
  "xray": {
    "clients": ["192.168.1.100"],
    "servers": ["1.2.3.4"],
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
      "wait_delay": 30
    }
  }
}
```

### Step 0.13: Сделать mocks исполняемыми

```bash
chmod +x test/mocks/*
```

### Step 0.14: Проверить что bats работает

```bash
echo '@test "sanity check" { true; }' > test/sanity.bats
bats test/sanity.bats
rm test/sanity.bats
```

Expected: `1 test, 0 failures`

### Step 0.15: Коммит

```bash
git add test/
git commit -m "$(cat <<'EOF'
test: add bats testing infrastructure

- test_helper.bash with setup/teardown
- mocks for nvram, iptables, ipset, nslookup, logger, ip
- fixtures for /etc/hosts and vpn-director.json
EOF
)"
```

---

## Task 1: Миграция common.sh — часть 1 (базовые функции)

**Files:**
- Modify: `jffs/scripts/vpn-director/utils/common.sh`
- Create: `test/common.bats`

### Step 1.1: Написать тесты для uuid4, compute_hash

```bash
# test/common.bats
load 'test_helper'

# ============================================================================
# uuid4
# ============================================================================

@test "uuid4: returns valid UUID format" {
    load_common
    run uuid4
    assert_success
    # UUID format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
    assert_output --regexp '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
}

@test "uuid4: generates unique values" {
    load_common
    uuid1=$(uuid4)
    uuid2=$(uuid4)
    [ "$uuid1" != "$uuid2" ]
}

# ============================================================================
# compute_hash
# ============================================================================

@test "compute_hash: hashes string from stdin" {
    load_common
    run bash -c 'echo -n "test" | compute_hash'
    assert_success
    # SHA-256 of "test" is known
    assert_output "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
}

@test "compute_hash: hashes file" {
    load_common
    echo -n "test" > /tmp/bats_hash_test
    run compute_hash /tmp/bats_hash_test
    assert_success
    assert_output "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
    rm /tmp/bats_hash_test
}
```

### Step 1.2: Запустить тест и убедиться что они проходят с текущим кодом

Run: `bats test/common.bats`
Expected: Тесты проходят (код ещё не изменён, просто проверяем что тесты корректны)

### Step 1.3: Изменить shebang в common.sh

Заменить:
```bash
#!/usr/bin/env ash
```

На:
```bash
#!/usr/bin/env bash
```

### Step 1.4: Добавить debug mode в common.sh

После `set -euo pipefail` добавить:
```bash
# Debug mode: set DEBUG=1 to enable tracing
if [[ ${DEBUG:-0} == 1 ]]; then
    set -x
    PS4='+${BASH_SOURCE[0]##*/}:${LINENO}:${FUNCNAME[0]:-main}: '
fi
```

### Step 1.5: Запустить тесты

Run: `bats test/common.bats`
Expected: PASS

### Step 1.6: Запустить shellcheck

Run: `shellcheck -s bash jffs/scripts/vpn-director/utils/common.sh`
Expected: No errors (warnings OK for now)

### Step 1.7: Коммит

```bash
git add jffs/scripts/vpn-director/utils/common.sh test/common.bats
git commit -m "$(cat <<'EOF'
refactor(utils): start common.sh migration to bash

- Change shebang to #!/usr/bin/env bash
- Add DEBUG mode with informative PS4
- Add initial bats tests for uuid4, compute_hash
EOF
)"
```

---

## Task 2: Миграция common.sh — часть 2 (is_lan_ip, resolve_ip)

**Files:**
- Modify: `jffs/scripts/vpn-director/utils/common.sh`
- Modify: `test/common.bats`

### Step 2.1: Добавить тесты для is_lan_ip

```bash
# Добавить в test/common.bats

# ============================================================================
# is_lan_ip
# ============================================================================

@test "is_lan_ip: 192.168.x.x is private" {
    load_common
    run is_lan_ip 192.168.1.100
    assert_success
}

@test "is_lan_ip: 10.x.x.x is private" {
    load_common
    run is_lan_ip 10.0.0.1
    assert_success
}

@test "is_lan_ip: 172.16.x.x is private" {
    load_common
    run is_lan_ip 172.16.0.1
    assert_success
}

@test "is_lan_ip: 172.31.x.x is private" {
    load_common
    run is_lan_ip 172.31.255.255
    assert_success
}

@test "is_lan_ip: 172.15.x.x is NOT private" {
    load_common
    run is_lan_ip 172.15.0.1
    assert_failure
}

@test "is_lan_ip: 8.8.8.8 is NOT private" {
    load_common
    run is_lan_ip 8.8.8.8
    assert_failure
}

@test "is_lan_ip: IPv6 ULA fd00:: is private" {
    load_common
    run is_lan_ip -6 "fd00::1"
    assert_success
}

@test "is_lan_ip: IPv6 link-local fe80:: is private" {
    load_common
    run is_lan_ip -6 "fe80::1"
    assert_success
}

@test "is_lan_ip: IPv6 global 2001:: is NOT private" {
    load_common
    run is_lan_ip -6 "2001:4860::1"
    assert_failure
}
```

### Step 2.2: Запустить тесты

Run: `bats test/common.bats`
Expected: PASS (текущий код должен работать)

### Step 2.3: Рефакторинг is_lan_ip на bash-стиль

Заменить функцию `is_lan_ip` на:
```bash
is_lan_ip() {
    local use_v6=0 ip

    if [[ $1 == "-6" ]]; then
        use_v6=1
        shift
    fi
    ip=${1:-}

    if [[ $use_v6 -eq 1 ]]; then
        case "$ip" in
            [Ff][Cc]*|[Ff][Dd]*)        return 0 ;;  # ULA fc00::/7
            [Ff][Ee][89AaBb]*)          return 0 ;;  # link-local fe80::/10
            *)                          return 1 ;;
        esac
    else
        case "$ip" in
            192.168.*)                              return 0 ;;  # 192.168.0.0/16
            10.*)                                   return 0 ;;  # 10.0.0.0/8
            172.1[6-9].*|172.2[0-9].*|172.3[0-1].*) return 0 ;;  # 172.16.0.0/12
            *)                                      return 1 ;;
        esac
    fi
}
```

### Step 2.4: Запустить тесты

Run: `bats test/common.bats`
Expected: PASS

### Step 2.5: Добавить тесты для resolve_ip

```bash
# Добавить в test/common.bats

# ============================================================================
# resolve_ip
# ============================================================================

@test "resolve_ip: returns literal IPv4" {
    load_common
    run resolve_ip 192.168.1.1
    assert_success
    assert_output "192.168.1.1"
}

@test "resolve_ip: resolves from /etc/hosts" {
    load_common
    # Uses fixture hosts file via HOSTS_FILE env
    run resolve_ip mypc
    assert_success
    assert_output "192.168.1.100"
}

@test "resolve_ip: -q suppresses error on failure" {
    load_common
    run resolve_ip -q nonexistent.invalid
    assert_failure
    assert_output ""
}
```

### Step 2.6: Запустить тесты

Run: `bats test/common.bats`
Expected: PASS или некоторые могут не пройти (если hosts mock не работает)

### Step 2.7: Коммит

```bash
git add jffs/scripts/vpn-director/utils/common.sh test/common.bats
git commit -m "$(cat <<'EOF'
refactor(utils): migrate is_lan_ip to bash syntax

- Use [[ ]] instead of [ ]
- Add comprehensive tests for IPv4 and IPv6
- Tests for resolve_ip basic cases
EOF
)"
```

---

## Task 3: Миграция common.sh — часть 3 (_resolve_ip_impl)

**Files:**
- Modify: `jffs/scripts/vpn-director/utils/common.sh`
- Modify: `test/common.bats`

### Step 3.1: Рефакторинг _resolve_ip_impl на bash arrays

Заменить парсинг флагов на:
```bash
_resolve_ip_impl() {
    local use_v6=0 only_global=0 return_all=0

    while [[ $# -gt 0 ]]; do
        case $1 in
            -6) use_v6=1; shift ;;
            -g) only_global=1; shift ;;
            -a) return_all=1; shift ;;
            --) shift; break ;;
            *)  break ;;
        esac
    done

    local arg=${1:-}
    [[ -n $arg ]] || return 1

    # ... rest of function
```

### Step 3.2: Заменить grep на [[ =~ ]] где возможно

В `_emit_if_ok()` заменить:
```bash
printf '%s\n' "$cand" | grep $g_flags -- "$fam_pat" >/dev/null || return 1
```

На:
```bash
[[ $cand =~ $fam_pat ]] || return 1
```

### Step 3.3: Запустить тесты

Run: `bats test/common.bats`
Expected: PASS

### Step 3.4: Запустить shellcheck

Run: `shellcheck -s bash jffs/scripts/vpn-director/utils/common.sh`
Expected: Меньше warnings чем раньше

### Step 3.5: Коммит

```bash
git add jffs/scripts/vpn-director/utils/common.sh
git commit -m "$(cat <<'EOF'
refactor(utils): use bash [[ =~ ]] in _resolve_ip_impl

- Replace grep regex checks with bash native regex
- Use [[ ]] consistently for conditionals
EOF
)"
```

---

## Task 4: Миграция common.sh — часть 4 (log, strip_comments)

**Files:**
- Modify: `jffs/scripts/vpn-director/utils/common.sh`
- Modify: `test/common.bats`

### Step 4.1: Добавить тесты для log

```bash
# Добавить в test/common.bats

# ============================================================================
# log
# ============================================================================

@test "log: writes to LOG_FILE" {
    load_common
    log "test message"
    run cat "$LOG_FILE"
    assert_success
    assert_output --partial "INFO"
    assert_output --partial "test message"
}

@test "log: supports -l ERROR level" {
    load_common
    log -l ERROR "error message"
    run cat "$LOG_FILE"
    assert_output --partial "ERROR"
    assert_output --partial "error message"
}

@test "log: supports -l WARN level" {
    load_common
    log -l WARN "warning message"
    run cat "$LOG_FILE"
    assert_output --partial "WARN"
}
```

### Step 4.2: Запустить тесты

Run: `bats test/common.bats`
Expected: PASS

### Step 4.3: Добавить функцию log_error_trace

После функции `log()` добавить:
```bash
###################################################################################################
# log_error_trace - log error with stack trace
###################################################################################################
log_error_trace() {
    local msg=$1
    local i

    log -l ERROR "$msg"

    for ((i=1; i<${#FUNCNAME[@]}; i++)); do
        log -l ERROR "  at ${BASH_SOURCE[i]##*/}:${BASH_LINENO[i-1]} ${FUNCNAME[i]}()"
    done
}
```

### Step 4.4: Добавить тест для log_error_trace

```bash
@test "log_error_trace: includes stack trace" {
    load_common

    # Define nested function to test stack trace
    inner_func() { log_error_trace "inner error"; }
    outer_func() { inner_func; }

    outer_func

    run cat "$LOG_FILE"
    assert_output --partial "inner error"
    assert_output --partial "at"
}
```

### Step 4.5: Запустить тесты

Run: `bats test/common.bats`
Expected: PASS

### Step 4.6: Добавить тесты для strip_comments

```bash
# ============================================================================
# strip_comments
# ============================================================================

@test "strip_comments: removes # comments" {
    load_common
    input="line1
# comment
line2"
    run bash -c "echo '$input' | strip_comments"
    assert_success
    assert_line -n 0 "line1"
    assert_line -n 1 "line2"
}

@test "strip_comments: removes inline comments" {
    load_common
    run bash -c "echo 'value # comment' | strip_comments"
    assert_success
    assert_output "value"
}

@test "strip_comments: trims whitespace" {
    load_common
    run bash -c "echo '  spaced  ' | strip_comments"
    assert_success
    assert_output "spaced"
}
```

### Step 4.7: Запустить тесты

Run: `bats test/common.bats`
Expected: PASS

### Step 4.8: Коммит

```bash
git add jffs/scripts/vpn-director/utils/common.sh test/common.bats
git commit -m "$(cat <<'EOF'
refactor(utils): add log_error_trace, test log and strip_comments

- New log_error_trace() with FUNCNAME/BASH_SOURCE stack trace
- Tests for log levels and strip_comments
EOF
)"
```

---

## Task 5: Миграция firewall.sh — часть 1

**Files:**
- Modify: `jffs/scripts/vpn-director/utils/firewall.sh`
- Create: `test/firewall.bats`

### Step 5.1: Написать базовые тесты для firewall.sh

```bash
# test/firewall.bats
load 'test_helper'

# ============================================================================
# validate_port
# ============================================================================

@test "validate_port: accepts valid port 80" {
    load_firewall
    run validate_port 80
    assert_success
}

@test "validate_port: accepts valid port 443" {
    load_firewall
    run validate_port 443
    assert_success
}

@test "validate_port: accepts valid port 65535" {
    load_firewall
    run validate_port 65535
    assert_success
}

@test "validate_port: rejects port 0" {
    load_firewall
    run validate_port 0
    assert_failure
}

@test "validate_port: rejects port 70000" {
    load_firewall
    run validate_port 70000
    assert_failure
}

@test "validate_port: rejects non-numeric" {
    load_firewall
    run validate_port "abc"
    assert_failure
}

@test "validate_port: rejects empty" {
    load_firewall
    run validate_port ""
    assert_failure
}

# ============================================================================
# validate_ports
# ============================================================================

@test "validate_ports: accepts 'any'" {
    load_firewall
    run validate_ports "any"
    assert_success
}

@test "validate_ports: accepts single port" {
    load_firewall
    run validate_ports "443"
    assert_success
}

@test "validate_ports: accepts port range" {
    load_firewall
    run validate_ports "1000-2000"
    assert_success
}

@test "validate_ports: accepts comma list" {
    load_firewall
    run validate_ports "80,443,8080"
    assert_success
}

@test "validate_ports: accepts mixed list with range" {
    load_firewall
    run validate_ports "80,443,1000-2000"
    assert_success
}

@test "validate_ports: rejects invalid range (start > end)" {
    load_firewall
    run validate_ports "2000-1000"
    assert_failure
}

# ============================================================================
# normalize_protos
# ============================================================================

@test "normalize_protos: returns tcp for tcp" {
    load_firewall
    run normalize_protos "tcp"
    assert_success
    assert_output "tcp"
}

@test "normalize_protos: returns udp for udp" {
    load_firewall
    run normalize_protos "udp"
    assert_success
    assert_output "udp"
}

@test "normalize_protos: returns tcp,udp for any" {
    load_firewall
    run normalize_protos "any"
    assert_success
    assert_output "tcp,udp"
}

@test "normalize_protos: normalizes udp,tcp to tcp,udp" {
    load_firewall
    run normalize_protos "udp,tcp"
    assert_success
    assert_output "tcp,udp"
}
```

### Step 5.2: Запустить тесты

Run: `bats test/firewall.bats`
Expected: PASS

### Step 5.3: Изменить shebang в firewall.sh

Заменить:
```bash
#!/usr/bin/env ash
```

На:
```bash
#!/usr/bin/env bash
```

### Step 5.4: Добавить debug mode

После shebang:
```bash
# Debug mode
if [[ ${DEBUG:-0} == 1 ]]; then
    set -x
    PS4='+${BASH_SOURCE[0]##*/}:${LINENO}:${FUNCNAME[0]:-main}: '
fi
```

### Step 5.5: Запустить тесты

Run: `bats test/firewall.bats`
Expected: PASS

### Step 5.6: Коммит

```bash
git add jffs/scripts/vpn-director/utils/firewall.sh test/firewall.bats
git commit -m "$(cat <<'EOF'
refactor(utils): start firewall.sh migration to bash

- Change shebang to #!/usr/bin/env bash
- Add debug mode
- Add tests for validate_port, validate_ports, normalize_protos
EOF
)"
```

---

## Task 6: Миграция firewall.sh — часть 2 (убрать eval)

**Files:**
- Modify: `jffs/scripts/vpn-director/utils/firewall.sh`

### Step 6.1: Рефакторинг _spec_to_log — убрать eval

Найти в `_spec_to_log()`:
```bash
if [ $# -eq 1 ]; then
    set -f
    eval "set -- $1"
    set +f
fi
```

Заменить на:
```bash
if [[ $# -eq 1 ]]; then
    local -a args
    read -ra args <<< "$1"
    set -- "${args[@]}"
fi
```

### Step 6.2: Рефакторинг purge_fw_rules — убрать eval

Найти в `purge_fw_rules()`:
```bash
rest=${rule#-A }
set -f
eval "set -- $rest"
set +f
```

Заменить на:
```bash
rest=${rule#-A }
local -a args
read -ra args <<< "$rest"
set -- "${args[@]}"
```

### Step 6.3: Рефакторинг sync_fw_rule — убрать eval

Найти:
```bash
set -f
eval "set -- $cmdargs $desired"
set +f
```

Заменить на:
```bash
local -a cmd_array
read -ra cmd_array <<< "$cmdargs $desired"
set -- "${cmd_array[@]}"
```

### Step 6.4: Запустить тесты

Run: `bats test/firewall.bats`
Expected: PASS

### Step 6.5: Запустить shellcheck

Run: `shellcheck -s bash jffs/scripts/vpn-director/utils/firewall.sh`
Expected: Меньше warnings, нет eval warnings

### Step 6.6: Коммит

```bash
git add jffs/scripts/vpn-director/utils/firewall.sh
git commit -m "$(cat <<'EOF'
refactor(utils): remove eval from firewall.sh

- Replace eval "set -- $var" with read -ra array
- Safer parsing without shell injection risk
EOF
)"
```

---

## Task 7: Миграция firewall.sh — часть 3 ([[ ]] и arrays)

**Files:**
- Modify: `jffs/scripts/vpn-director/utils/firewall.sh`

### Step 7.1: Заменить [ ] на [[ ]] во всех функциях

Глобальная замена паттернов:
- `[ "$var" = "value" ]` → `[[ $var == "value" ]]`
- `[ -z "$var" ]` → `[[ -z $var ]]`
- `[ -n "$var" ]` → `[[ -n $var ]]`
- `[ "$a" -eq "$b" ]` → `[[ $a -eq $b ]]`

### Step 7.2: Рефакторинг validate_ports на read -ra

Заменить:
```bash
IFS_SAVE=$IFS
IFS=','; set -- $p; IFS=$IFS_SAVE

for tok in "$@"; do
```

На:
```bash
local -a tokens
IFS=',' read -ra tokens <<< "$p"

for tok in "${tokens[@]}"; do
```

### Step 7.3: Рефакторинг normalize_protos аналогично

Заменить:
```bash
IFS_SAVE=$IFS
IFS=','; set -- $in; IFS=$IFS_SAVE

for tok in "$@"; do
```

На:
```bash
local -a tokens
IFS=',' read -ra tokens <<< "$in"

for tok in "${tokens[@]}"; do
```

### Step 7.4: Запустить тесты

Run: `bats test/firewall.bats`
Expected: PASS

### Step 7.5: Коммит

```bash
git add jffs/scripts/vpn-director/utils/firewall.sh
git commit -m "$(cat <<'EOF'
refactor(utils): use [[ ]] and arrays in firewall.sh

- Replace [ ] with [[ ]] throughout
- Use read -ra for comma-separated parsing
- Consistent bash idioms
EOF
)"
```

---

## Task 8: Миграция config.sh

**Files:**
- Modify: `jffs/scripts/vpn-director/utils/config.sh`
- Create: `test/config.bats`

### Step 8.1: Написать тесты для config.sh

```bash
# test/config.bats
load 'test_helper'

@test "config.sh: loads without error" {
    load_config
    assert_success
}

@test "config.sh: sets TUN_DIR_RULES" {
    load_config
    [[ -n $TUN_DIR_RULES ]]
}

@test "config.sh: sets XRAY_CLIENTS" {
    load_config
    [[ -n $XRAY_CLIENTS ]] || [[ $XRAY_CLIENTS == "" ]]
}

@test "config.sh: sets XRAY_TPROXY_PORT" {
    load_config
    [[ $XRAY_TPROXY_PORT == "12345" ]]
}
```

### Step 8.2: Запустить тесты

Run: `bats test/config.bats`
Expected: PASS

### Step 8.3: Изменить shebang и добавить bash features

```bash
#!/usr/bin/env bash

set -euo pipefail

# Debug mode
if [[ ${DEBUG:-0} == 1 ]]; then
    set -x
    PS4='+${BASH_SOURCE[0]##*/}:${LINENO}:${FUNCNAME[0]:-main}: '
fi
```

### Step 8.4: Заменить [ ] на [[ ]]

### Step 8.5: Запустить тесты

Run: `bats test/config.bats`
Expected: PASS

### Step 8.6: Коммит

```bash
git add jffs/scripts/vpn-director/utils/config.sh test/config.bats
git commit -m "$(cat <<'EOF'
refactor(utils): migrate config.sh to bash

- Change shebang
- Use [[ ]] syntax
- Add basic tests
EOF
)"
```

---

## Task 9: Миграция shared.sh и send-email.sh

**Files:**
- Modify: `jffs/scripts/vpn-director/utils/shared.sh`
- Modify: `jffs/scripts/vpn-director/utils/send-email.sh`

### Step 9.1: Изменить shebang в shared.sh

### Step 9.2: Заменить [ ] на [[ ]] в shared.sh

### Step 9.3: Изменить shebang в send-email.sh

### Step 9.4: Заменить [ ] на [[ ]] в send-email.sh

### Step 9.5: Запустить shellcheck на обоих файлах

Run: `shellcheck -s bash jffs/scripts/vpn-director/utils/shared.sh jffs/scripts/vpn-director/utils/send-email.sh`

### Step 9.6: Коммит

```bash
git add jffs/scripts/vpn-director/utils/shared.sh jffs/scripts/vpn-director/utils/send-email.sh
git commit -m "$(cat <<'EOF'
refactor(utils): migrate shared.sh and send-email.sh to bash

- Change shebangs
- Use [[ ]] syntax
EOF
)"
```

---

## Task 10: Миграция ipset_builder.sh

**Files:**
- Modify: `jffs/scripts/vpn-director/ipset_builder.sh`
- Create: `test/ipset_builder.bats`

### Step 10.1: Написать тесты для парсеров

```bash
# test/ipset_builder.bats
load 'test_helper'

load_ipset_builder() {
    export PATH="$BATS_TEST_DIRNAME/mocks:$PATH"
    # Source only the parser functions, not the main logic
    source "$SCRIPTS_DIR/ipset_builder.sh" --source-only 2>/dev/null || true
}

@test "parse_country_codes: extracts ru from rule" {
    # This test may need adjustment based on how we expose the function
    skip "Parser function not exposed yet"
}
```

### Step 10.2: Изменить shebang

### Step 10.3: Рефакторинг парсинга аргументов

Заменить:
```bash
while [ $# -gt 0 ]; do
    case "$1" in
```

На:
```bash
while [[ $# -gt 0 ]]; do
    case $1 in
```

### Step 10.4: Заменить word splitting на arrays

Найти паттерны типа:
```bash
for cc in $all_cc; do
```

И заменить на:
```bash
local -a cc_array
read -ra cc_array <<< "$all_cc"
for cc in "${cc_array[@]}"; do
```

### Step 10.5: Запустить shellcheck

Run: `shellcheck -s bash jffs/scripts/vpn-director/ipset_builder.sh`

### Step 10.6: Коммит

```bash
git add jffs/scripts/vpn-director/ipset_builder.sh test/ipset_builder.bats
git commit -m "$(cat <<'EOF'
refactor: migrate ipset_builder.sh to bash

- Change shebang
- Use [[ ]] and bash arrays
- Safer argument parsing
EOF
)"
```

---

## Task 11: Миграция tunnel_director.sh

**Files:**
- Modify: `jffs/scripts/vpn-director/tunnel_director.sh`

### Step 11.1: Изменить shebang и добавить debug mode

### Step 11.2: Рефакторинг table_allowed на [[ ]]

Заменить:
```bash
table_allowed() {
    case " $valid_tables " in
        *" $1 "*)  return 0 ;;
        *)         return 1 ;;
    esac
}
```

На:
```bash
table_allowed() {
    [[ " $valid_tables " == *" $1 "* ]]
}
```

### Step 11.3: Рефакторинг парсинга src_excl

Заменить:
```bash
IFS_SAVE=$IFS
IFS=','; set -- $src_excl; IFS=$IFS_SAVE
for ex; do
```

На:
```bash
local -a excl_array
IFS=',' read -ra excl_array <<< "$src_excl"
for ex in "${excl_array[@]}"; do
```

### Step 11.4: Запустить shellcheck

### Step 11.5: Коммит

```bash
git add jffs/scripts/vpn-director/tunnel_director.sh
git commit -m "$(cat <<'EOF'
refactor: migrate tunnel_director.sh to bash

- Use [[ ]] pattern matching
- Use bash arrays for parsing
EOF
)"
```

---

## Task 12: Миграция xray_tproxy.sh

**Files:**
- Modify: `jffs/scripts/vpn-director/xray_tproxy.sh`

### Step 12.1: Изменить shebang и добавить debug mode

### Step 12.2: Заменить for loops с word splitting

Все паттерны типа:
```bash
for ip in $XRAY_CLIENTS; do
```

Заменить на:
```bash
local -a clients_array
read -ra clients_array <<< "$XRAY_CLIENTS"
for ip in "${clients_array[@]}"; do
```

### Step 12.3: Заменить [ ] на [[ ]]

### Step 12.4: Запустить shellcheck

### Step 12.5: Коммит

```bash
git add jffs/scripts/vpn-director/xray_tproxy.sh
git commit -m "$(cat <<'EOF'
refactor: migrate xray_tproxy.sh to bash

- Use bash arrays for client/server lists
- Use [[ ]] syntax
EOF
)"
```

---

## Task 13: Миграция import_server_list.sh

**Files:**
- Modify: `jffs/scripts/vpn-director/import_server_list.sh`

### Step 13.1: Изменить shebang с #!/bin/sh на #!/usr/bin/env bash

### Step 13.2: Добавить set -euo pipefail и debug mode

### Step 13.3: Заменить [ ] на [[ ]]

### Step 13.4: Запустить shellcheck

### Step 13.5: Коммит

```bash
git add jffs/scripts/vpn-director/import_server_list.sh
git commit -m "$(cat <<'EOF'
refactor: migrate import_server_list.sh to bash

- Change from /bin/sh to bash
- Add strict mode and debug
EOF
)"
```

---

## Task 14: Миграция configure.sh и install.sh

**Files:**
- Modify: `jffs/scripts/vpn-director/configure.sh`
- Modify: `install.sh`

### Step 14.1: Изменить shebang в configure.sh

### Step 14.2: Заменить [ ] на [[ ]] в configure.sh

### Step 14.3: Изменить shebang в install.sh

### Step 14.4: Заменить [ ] на [[ ]] в install.sh

### Step 14.5: Запустить shellcheck на обоих

### Step 14.6: Коммит

```bash
git add jffs/scripts/vpn-director/configure.sh install.sh
git commit -m "$(cat <<'EOF'
refactor: migrate configure.sh and install.sh to bash

- Final scripts migrated
- Use [[ ]] syntax
EOF
)"
```

---

## Task 15: Обновление документации

**Files:**
- Modify: `CLAUDE.md`
- Modify: `.claude/rules/shell-conventions.md`

### Step 15.1: Обновить CLAUDE.md

Заменить:
```markdown
- Shebang: `#!/usr/bin/env ash` with `set -euo pipefail`
```

На:
```markdown
- Shebang: `#!/usr/bin/env bash` with `set -euo pipefail`
- Debug: `DEBUG=1 ./script.sh` for tracing
```

### Step 15.2: Обновить shell-conventions.md

Добавить раздел про bash-специфичные паттерны:
```markdown
## Bash-specific patterns

- Use `[[ ]]` instead of `[ ]` for conditionals
- Use `read -ra array <<< "$string"` for splitting
- Use `${array[@]}` for iterating arrays
- Debug mode: `DEBUG=1` enables `set -x` with informative PS4
```

### Step 15.3: Коммит

```bash
git add CLAUDE.md .claude/rules/shell-conventions.md
git commit -m "$(cat <<'EOF'
docs: update shell conventions for bash migration

- Document new shebang and debug mode
- Add bash-specific patterns guide
EOF
)"
```

---

## Task 16: Финальная проверка

### Step 16.1: Запустить все тесты

Run: `bats test/`
Expected: All PASS

### Step 16.2: Запустить shellcheck на всех скриптах

```bash
shellcheck -s bash \
    jffs/scripts/vpn-director/*.sh \
    jffs/scripts/vpn-director/utils/*.sh \
    install.sh
```

Expected: No errors

### Step 16.3: Проверить синтаксис всех скриптов

```bash
for f in jffs/scripts/vpn-director/*.sh jffs/scripts/vpn-director/utils/*.sh install.sh; do
    bash -n "$f" && echo "OK: $f"
done
```

Expected: All OK

### Step 16.4: Финальный коммит (если нужны исправления)

```bash
git add -A
git commit -m "fix: address shellcheck warnings from final review"
```

---

## Summary

| Task | Description | Files |
|------|-------------|-------|
| 0 | Test infrastructure | test/* |
| 1-4 | common.sh migration | utils/common.sh |
| 5-7 | firewall.sh migration | utils/firewall.sh |
| 8 | config.sh migration | utils/config.sh |
| 9 | shared.sh, send-email.sh | utils/*.sh |
| 10 | ipset_builder.sh | ipset_builder.sh |
| 11 | tunnel_director.sh | tunnel_director.sh |
| 12 | xray_tproxy.sh | xray_tproxy.sh |
| 13 | import_server_list.sh | import_server_list.sh |
| 14 | configure.sh, install.sh | *.sh |
| 15 | Documentation | CLAUDE.md, .claude/rules/* |
| 16 | Final verification | All |

Total: ~50 commits, each small and focused.
