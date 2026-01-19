# VPN Director Refactoring Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Refactor VPN Director from three separate scripts into a modular architecture with a single CLI entry point.

**Architecture:** Single `vpn-director.sh` CLI orchestrates three independent modules (`lib/ipset.sh`, `lib/tunnel.sh`, `lib/tproxy.sh`). Each module exposes public functions (`*_status`, `*_apply`, `*_stop`) and declares required ipsets. The CLI handles argument parsing and calls modules in dependency order.

**Tech Stack:** Bash 4+, ipset, iptables, jq, bats (testing)

---

## Task 1: Rename utils/ to lib/

**Files:**
- Rename: `jffs/scripts/vpn-director/utils/` → `jffs/scripts/vpn-director/lib/`
- Modify: All scripts that source utils/*

**Step 1: Rename directory**

```bash
git mv jffs/scripts/vpn-director/utils jffs/scripts/vpn-director/lib
```

**Step 2: Update source paths in existing scripts**

Update these files to use `lib/` instead of `utils/`:

`jffs/scripts/vpn-director/ipset_builder.sh` lines 203-206:
```bash
. /jffs/scripts/vpn-director/lib/common.sh
. /jffs/scripts/vpn-director/lib/firewall.sh
. /jffs/scripts/vpn-director/lib/shared.sh
. /jffs/scripts/vpn-director/lib/config.sh
```

`jffs/scripts/vpn-director/tunnel_director.sh` lines 130-136:
```bash
. /jffs/scripts/vpn-director/lib/common.sh
. /jffs/scripts/vpn-director/lib/firewall.sh
. /jffs/scripts/vpn-director/lib/shared.sh
. /jffs/scripts/vpn-director/lib/config.sh
```

`jffs/scripts/vpn-director/xray_tproxy.sh` lines 39-41:
```bash
. /jffs/scripts/vpn-director/lib/common.sh
. /jffs/scripts/vpn-director/lib/firewall.sh
. /jffs/scripts/vpn-director/lib/config.sh
```

`jffs/scripts/vpn-director/import_server_list.sh` line 18:
```bash
. "$SCRIPT_DIR/lib/common.sh"
```

`jffs/scripts/vpn-director/lib/send-email.sh` line 32 (after rename from utils/):
```bash
. /jffs/scripts/vpn-director/lib/common.sh
```

**Step 3: Update test_helper.bash**

`test/test_helper.bash` line 11:
```bash
export LIB_DIR="$SCRIPTS_DIR/lib"
```

Update all `$UTILS_DIR` references to `$LIB_DIR`.

**Step 4: Run tests to verify**

Run: `npx bats test/`
Expected: All tests pass

**Step 5: Commit**

```bash
git add -A
git commit -m "$(cat <<'EOF'
refactor: rename utils/ to lib/

Standardize directory naming before modular refactoring.
EOF
)"
```

---

## Task 2: Create lib/ipset.sh module

**Files:**
- Create: `jffs/scripts/vpn-director/lib/ipset.sh`
- Test: `test/unit/ipset.bats`

**Step 1: Create test directory structure**

```bash
mkdir -p test/unit test/integration
```

**Step 2: Write failing tests for ipset module**

Create `test/unit/ipset.bats`:

```bash
#!/usr/bin/env bats

load '../test_helper'

load_ipset_module() {
    load_common
    load_config
    source "$LIB_DIR/ipset.sh" --source-only
}

# ============================================================================
# ipset_get_required - returns list of ipsets needed
# ============================================================================

@test "ipset_get_required: parses country codes from rules" {
    load_ipset_module
    local result
    result=$(echo "wgc1:192.168.50.0/24::ru" | _parse_country_codes)
    [ "$result" = "ru" ]
}

@test "ipset_get_required: parses combo sets from rules" {
    load_ipset_module
    local result
    result=$(echo "wgc1:192.168.50.0/24::us,ca" | _parse_combo_sets)
    [ "$result" = "us,ca" ]
}

# ============================================================================
# ipset_ensure - ensures ipsets exist (from cache or download)
# ============================================================================

@test "ipset_ensure: returns success when ipset exists" {
    load_ipset_module
    # Mock ipset returns success for 'ru'
    run ipset_ensure "ru"
    assert_success
}

# ============================================================================
# ipset_status - shows ipset information
# ============================================================================

@test "ipset_status: outputs ipset list" {
    load_ipset_module
    run ipset_status
    assert_success
    assert_output --partial "IPSet Status"
}

# ============================================================================
# Helper functions (migrated from ipset_builder.sh)
# ============================================================================

@test "_next_pow2: rounds up to next power of two" {
    load_ipset_module
    run _next_pow2 3
    assert_success
    assert_output "4"
}

@test "_calc_ipset_size: returns at least 1024" {
    load_ipset_module
    run _calc_ipset_size 100
    assert_success
    assert_output "1024"
}

@test "_derive_set_name: returns lowercase name for short names" {
    load_ipset_module
    run _derive_set_name "RU"
    assert_success
    assert_output "ru"
}

@test "_derive_set_name: returns hash for names over 31 chars" {
    load_ipset_module
    local long_name="this_is_a_very_long_ipset_name_that_exceeds_limit"
    run _derive_set_name "$long_name"
    assert_success
    # Output should be 24 chars (SHA256 prefix)
    [ ${#output} -eq 24 ]
}
```

**Step 3: Run tests to verify they fail**

Run: `npx bats test/unit/ipset.bats`
Expected: FAIL (module doesn't exist yet)

**Step 4: Create lib/ipset.sh with minimal implementation**

Create `jffs/scripts/vpn-director/lib/ipset.sh`:

```bash
#!/usr/bin/env bash

###################################################################################################
# ipset.sh - IPSet management module for VPN Director
# -------------------------------------------------------------------------------------------------
# Public API:
#   ipset_status              - Show loaded ipsets, sizes, cache age
#   ipset_ensure <sets...>    - Ensure ipsets exist (from cache or download)
#   ipset_update <sets...>    - Force download fresh data from IPdeny
#   ipset_cleanup             - Remove ipsets no longer needed
#
# Note: This module is agnostic to tunnel/tproxy. The CLI orchestrator (vpn-director.sh)
#       gets required ipsets from tunnel_get_required_ipsets() and tproxy_get_required_ipsets().
###################################################################################################

# shellcheck disable=SC2086

set -euo pipefail

if [[ ${DEBUG:-0} == 1 ]]; then
    set -x
    PS4='+${BASH_SOURCE[0]##*/}:${LINENO}:${FUNCNAME[0]:-main}: '
fi

###################################################################################################
# Constants
###################################################################################################

ALL_COUNTRY_CODES='
ad ae af ag ai al am ao aq ar as at au aw ax az ba bb bd be bf bg bh bi bj bl bm bn bo bq br bs bt
bv bw by bz ca cc cd cf cg ch ci ck cl cm cn co cr cu cv cw cx cy cz de dj dk dm do dz ec ee eg eh
er es et fi fj fk fm fo fr ga gb gd ge gf gg gh gi gl gm gn gp gq gr gs gt gu gw gy hk hm hn hr ht
hu id ie il im in io iq ir is it je jm jo jp ke kg kh ki km kn kp kr kw ky kz la lb lc li lk lr ls
lt lu lv ly ma mc md me mf mg mh mk ml mm mn mo mp mq mr ms mt mu mv mw mx my mz na nc ne nf ng ni
nl no np nr nu nz om pa pe pf pg ph pk pl pm pn pr ps pt pw py qa re ro rs ru rw sa sb sc sd se sg
sh si sj sk sl sm sn so sr ss st sv sx sy sz tc td tf tg th tj tk tl tm tn to tr tt tv tw tz ua ug
um us uy uz va vc ve vg vi vn vu wf ws ye yt za zm zw
'

IPDENY_BASE_URL='https://www.ipdeny.com/ipblocks/data/aggregated'
IPDENY_SUFFIX='-aggregated.zone'

###################################################################################################
# Internal state (set after sourcing config.sh)
###################################################################################################
_IPSET_DUMP_DIR=""
_IPSET_STATE_DIR=""

###################################################################################################
# Pure functions (for testing)
###################################################################################################

_next_pow2() {
    local n=${1:-1} p=1
    [[ $n -lt 1 ]] && n=1
    while [[ $p -lt $n ]]; do
        p=$((p<<1))
    done
    printf '%s\n' "$p"
}

_calc_ipset_size() {
    local n=${1:-0} floor=1024 buckets
    buckets=$(((4 * n + 2) / 3))
    [[ $buckets -lt $floor ]] && buckets=$floor
    _next_pow2 "$buckets"
}

_derive_set_name() {
    local set="$1" max=31 set_lc hash
    set_lc=$(printf '%s' "$set" | tr 'A-Z' 'a-z')

    if [[ "${#set_lc}" -le "$max" ]]; then
        printf '%s\n' "$set_lc"
        return 0
    fi

    hash="$(printf '%s' "$set_lc" | compute_hash | cut -c1-24)"
    log -l TRACE "Assigned alias='$hash' for set='$set_lc' (exceeds $max chars)"
    printf '%s\n' "$hash"
}

_parse_country_codes() {
    awk -v valid="$ALL_COUNTRY_CODES" '
        function add_code(tok, base) {
            gsub(/[[:space:]]+/, "", tok)
            if (tok ~ /^[a-z]{2}$/) {
                base = tok
                if (V[base]) seen[base] = 1
            }
        }
        BEGIN {
            n = split(valid, arr, /[[:space:]\r\n]+/)
            for (i = 1; i <= n; i++) if (arr[i] != "") V[arr[i]] = 1
        }
        {
            depth = 0; tok = ""; nf2 = 0
            for (i = 1; i <= length($0); i++) {
                c = substr($0, i, 1)
                if (c == "[") depth++
                else if (c == "]" && depth > 0) depth--
                if (c == ":" && depth == 0) {
                    f[++nf2] = tok; tok = ""
                }
                else tok = tok c
            }
            f[++nf2] = tok

            for (fld = 4; fld <= 5; fld++) {
                if (fld > nf2) continue
                n = split(tolower(f[fld]), parts, ",")
                for (i = 1; i <= n; i++) add_code(parts[i])
            }

            for (i = 1; i <= nf2; i++) delete f[i]
        }
        END { for (c in seen) print c }
    ' | sort
}

_parse_combo_sets() {
    awk '
        function emit_combo(field, f, n, a, i, t, out) {
            f = field
            gsub(/[[:space:]]/, "", f)
            if (f ~ /,/) {
                n = split(f, a, ",")
                out = ""
                for (i = 1; i <= n; i++) {
                    t = a[i]
                    out = out (out ? "," : "") t
                }
                print out
            }
        }
        {
            depth = 0; tok = ""; nf2 = 0
            for (i = 1; i <= length($0); i++) {
                c = substr($0, i, 1)
                if (c == "[") depth++
                else if (c == "]" && depth > 0) depth--
                if (c == ":" && depth == 0) {
                    F[++nf2] = tok; tok = ""
                }
                else tok = tok c
            }
            F[++nf2] = tok

            if (nf2 >= 4) emit_combo(F[4])
            if (nf2 >= 5) emit_combo(F[5])
        }
    ' | sort -u
}

###################################################################################################
# Initialize state directories (called by public functions, not on source)
###################################################################################################
_ipset_init() {
    _IPSET_DUMP_DIR="${IPS_BDR_DIR:-/jffs/scripts/vpn-director/data}/ipsets"
    _IPSET_STATE_DIR="/tmp/ipset_module"
    mkdir -p "$_IPSET_DUMP_DIR" "$_IPSET_STATE_DIR"
}

###################################################################################################
# Internal functions
###################################################################################################

_ipset_exists() {
    ipset list -n "$1" >/dev/null 2>&1
}

_ipset_count() {
    ipset list "$1" 2>/dev/null | awk '/Number of entries:/ { print $4; exit }' || echo "0"
}

_download_zone() {
    local cc="$1" url file http_code
    url="${IPDENY_BASE_URL}/${cc}${IPDENY_SUFFIX}"
    file=$(tmp_file)

    log "Downloading ${cc}${IPDENY_SUFFIX}..."

    if ! http_code=$(curl -sS -o "$file" -w "%{http_code}" "$url"); then
        http_code=000
    fi

    if [[ $http_code -ge 200 ]] && [[ $http_code -lt 300 ]]; then
        log "Downloaded ${cc}${IPDENY_SUFFIX} (HTTP $http_code)"
        printf '%s\n' "$file"
        return 0
    else
        rm -f "$file"
        log -l ERROR "Failed to download ${cc}${IPDENY_SUFFIX} (HTTP $http_code)"
        return 1
    fi
}

_restore_from_cache() {
    local set_name="$1" dump="$_IPSET_DUMP_DIR/${set_name}-ipdeny.dump"

    [[ -f $dump ]] || return 1

    if _ipset_exists "$set_name"; then
        local tmp_set="${set_name}_tmp" restore_script
        restore_script=$(tmp_file)
        ipset destroy "$tmp_set" 2>/dev/null || true

        {
            sed -e "s/^create $set_name /create $tmp_set /" \
                -e "s/^add $set_name /add $tmp_set /" "$dump"
            printf 'swap %s %s\ndestroy %s\n' "$tmp_set" "$set_name" "$tmp_set"
        } > "$restore_script"

        ipset restore -! < "$restore_script" || return 1
    else
        ipset restore -! < "$dump" || return 1
    fi

    log "Restored ipset '$set_name' from cache ($(_ipset_count "$set_name") entries)"
    return 0
}

_build_country_ipset() {
    local cc="$1" set_name="$cc" cidr_file target_set cnt

    cidr_file=$(_download_zone "$cc") || return 1
    cnt=$(wc -l < "$cidr_file")

    if _ipset_exists "$set_name"; then
        target_set="${set_name}_tmp"
        ipset destroy "$target_set" 2>/dev/null || true
    else
        target_set="$set_name"
    fi

    local restore_script size
    restore_script=$(tmp_file)
    size=$(_calc_ipset_size "$cnt")

    {
        printf 'create %s hash:net family inet hashsize %s maxelem %s\n' "$target_set" "$size" "$size"
        while IFS= read -r line; do
            [[ -n $line ]] || continue
            printf 'add %s %s\n' "$target_set" "$line"
        done < "$cidr_file"
        if [[ $target_set != "$set_name" ]]; then
            printf 'swap %s %s\ndestroy %s\n' "$target_set" "$set_name" "$target_set"
        fi
    } > "$restore_script"

    ipset restore -! < "$restore_script" || {
        log -l ERROR "Failed to build ipset '$set_name'"
        return 1
    }

    # Save dump
    ipset save "$set_name" > "$_IPSET_DUMP_DIR/${set_name}-ipdeny.dump"
    log "Built ipset '$set_name' ($cnt entries)"
}

_build_combo_ipset() {
    local combo="$1" set_name member added=0

    set_name=$(_derive_set_name "${combo//,/_}")

    if _ipset_exists "$set_name"; then
        log "Combo ipset '$set_name' exists; skipping"
        return 0
    fi

    ipset create "$set_name" list:set

    local -a members
    IFS=',' read -ra members <<< "$combo"
    for key in "${members[@]}"; do
        member=$(_derive_set_name "$key")
        if ! _ipset_exists "$member"; then
            log -l WARN "Combo '$set_name': member '$member' not found"
            continue
        fi
        if ipset add "$set_name" "$member" 2>/dev/null; then
            added=$((added + 1))
        fi
    done

    if [[ $added -eq 0 ]]; then
        ipset destroy "$set_name" 2>/dev/null || true
        log -l WARN "Combo '$set_name' has no valid members; not created"
        return 1
    fi

    log "Created combo ipset '$set_name' ($added members)"
}

###################################################################################################
# Public API
###################################################################################################

ipset_status() {
    _ipset_init

    echo "=== IPSet Status ==="
    echo ""
    echo "Loaded ipsets:"
    ipset list -n 2>/dev/null | while read -r name; do
        local cnt=$(_ipset_count "$name")
        printf "  %-20s %s entries\n" "$name" "$cnt"
    done
    echo ""
    echo "Cache directory: $_IPSET_DUMP_DIR"
    if [[ -d "$_IPSET_DUMP_DIR" ]]; then
        echo "Cached dumps:"
        ls -la "$_IPSET_DUMP_DIR"/*.dump 2>/dev/null | awk '{print "  " $9 " (" $5 " bytes, " $6 " " $7 ")"}' || echo "  (none)"
    fi
}

ipset_ensure() {
    local force_update=${IPSET_FORCE_UPDATE:-0}
    local sets=("$@")
    local countries=() combos=() cc

    _ipset_init

    # Separate countries from combos
    for s in "${sets[@]}"; do
        if [[ $s == *,* ]]; then
            combos+=("$s")
            # Also need individual countries
            IFS=',' read -ra parts <<< "$s"
            for p in "${parts[@]}"; do
                countries+=("$p")
            done
        else
            countries+=("$s")
        fi
    done

    # Deduplicate countries
    local -A seen
    local unique_countries=()
    for cc in "${countries[@]}"; do
        [[ -z $cc ]] && continue
        [[ ${seen[$cc]:-} ]] && continue
        seen[$cc]=1
        unique_countries+=("$cc")
    done

    # Ensure country ipsets
    local failed=0
    for cc in "${unique_countries[@]}"; do
        if [[ $force_update -eq 0 ]] && _ipset_exists "$cc"; then
            log -l DEBUG "IPSet '$cc' already loaded"
            continue
        fi

        if [[ $force_update -eq 0 ]] && _restore_from_cache "$cc"; then
            continue
        fi

        _build_country_ipset "$cc" || {
            log -l ERROR "Failed to build ipset '$cc' (no cache, download failed)"
            failed=1
        }
    done

    # Ensure combo ipsets
    for combo in "${combos[@]}"; do
        _build_combo_ipset "$combo" || {
            log -l WARN "Failed to build combo ipset '$combo'"
        }
    done

    # Return error if any critical ipset failed (per design: abort apply for dependent component)
    if [[ $failed -eq 1 ]]; then
        log -l ERROR "ipset_ensure: some ipsets failed to load"
        return 1
    fi
}

ipset_update() {
    IPSET_FORCE_UPDATE=1 ipset_ensure "$@"
}

ipset_cleanup() {
    # TODO: implement cleanup of unused ipsets
    log "ipset_cleanup: not implemented yet"
}

###################################################################################################
# Boot delay (wait for network after system startup)
###################################################################################################
_ipset_boot_wait() {
    if [[ ${BOOT_WAIT_DELAY:-0} -ne 0 ]] && \
        ! awk -v min="${MIN_BOOT_TIME:-60}" '{ exit ($1 < min) }' /proc/uptime;
    then
        log "Uptime < ${MIN_BOOT_TIME:-60}s, sleeping ${BOOT_WAIT_DELAY}s..."
        sleep "$BOOT_WAIT_DELAY"
    fi
}

###################################################################################################
# Allow sourcing for testing (guard at end so all functions are defined)
###################################################################################################
if [[ ${1:-} == "--source-only" ]]; then
    return 0 2>/dev/null || exit 0
fi

# When run directly (not sourced), execute boot wait and show status
_ipset_boot_wait
ipset_status
```

**Step 5: Run tests to verify they pass**

Run: `npx bats test/unit/ipset.bats`
Expected: All tests PASS

**Step 6: Commit**

```bash
git add -A
git commit -m "$(cat <<'EOF'
feat(lib): add ipset.sh module

Migrate ipset building logic from ipset_builder.sh to modular lib/ipset.sh:
- ipset_status(): show loaded ipsets and cache info
- ipset_ensure(): ensure ipsets exist (cache or download)
- ipset_update(): force fresh download
- ipset_get_required_*(): return ipsets needed by components
EOF
)"
```

---

## Task 3: Create lib/tunnel.sh module

**Files:**
- Create: `jffs/scripts/vpn-director/lib/tunnel.sh`
- Test: `test/unit/tunnel.bats`

**Step 1: Write failing tests**

Create `test/unit/tunnel.bats`:

```bash
#!/usr/bin/env bats

load '../test_helper'

load_tunnel_module() {
    load_common
    load_config
    source "$LIB_DIR/ipset.sh" --source-only
    source "$LIB_DIR/firewall.sh"
    source "$LIB_DIR/tunnel.sh" --source-only
}

@test "tunnel_table_allowed: accepts wgc1" {
    load_tunnel_module
    _tunnel_valid_tables="wgc1 wgc2 main"
    run _tunnel_table_allowed "wgc1"
    assert_success
}

@test "tunnel_table_allowed: rejects unknown table" {
    load_tunnel_module
    _tunnel_valid_tables="wgc1 main"
    run _tunnel_table_allowed "wgc99"
    assert_failure
}

@test "tunnel_resolve_set: returns set name when exists" {
    load_tunnel_module
    run _tunnel_resolve_set "ru"
    assert_success
    assert_output "ru"
}

@test "tunnel_get_required_ipsets: parses rules" {
    load_tunnel_module
    export TUN_DIR_RULES="wgc1:192.168.50.0/24::ru wgc2:192.168.60.0/24::us"
    run tunnel_get_required_ipsets
    assert_success
    assert_output --partial "ru"
    assert_output --partial "us"
}

@test "tunnel_status: outputs chain info" {
    load_tunnel_module
    run tunnel_status
    assert_success
    assert_output --partial "Tunnel Director Status"
}
```

**Step 2: Run tests to verify they fail**

Run: `npx bats test/unit/tunnel.bats`
Expected: FAIL

**Step 3: Create lib/tunnel.sh**

Create `jffs/scripts/vpn-director/lib/tunnel.sh`:

```bash
#!/usr/bin/env bash

###################################################################################################
# tunnel.sh - Tunnel Director module for VPN Director
# -------------------------------------------------------------------------------------------------
# Public API:
#   tunnel_status            - Show TUN_DIR_* chains, ip rules, fwmarks
#   tunnel_apply             - Apply rules from config (idempotent)
#   tunnel_stop              - Remove all chains and ip rules
#   tunnel_get_required_ipsets - Return list of ipsets needed for rules
###################################################################################################

# shellcheck disable=SC2086

set -euo pipefail

if [[ ${DEBUG:-0} == 1 ]]; then
    set -x
    PS4='+${BASH_SOURCE[0]##*/}:${LINENO}:${FUNCNAME[0]:-main}: '
fi

###################################################################################################
# Module state
###################################################################################################
_tunnel_valid_tables=""
_tunnel_mark_mask_val=0
_tunnel_mark_shift_val=0
_tunnel_mark_field_max=0
_tunnel_mark_mask_hex=""

###################################################################################################
# Pure functions (for testing)
###################################################################################################

_tunnel_table_allowed() {
    [[ " $_tunnel_valid_tables " == *" $1 "* ]]
}

_tunnel_resolve_set() {
    local keys_csv="$1" set_name
    set_name=$(_derive_set_name "${keys_csv//,/_}")

    if ipset list "$set_name" >/dev/null 2>&1; then
        printf '%s\n' "$set_name"
        return 0
    fi
    return 0
}

_tunnel_get_prerouting_base_pos() {
    iptables -t mangle -S PREROUTING 2>/dev/null |
    awk '
        $1 == "-A" {
          i++
          if ( ($0 ~ /-i wgc[0-9]+/ || $0 ~ /-i tun[0-9]+/) &&
               $0 ~ /-j MARK/ && $0 ~ /--set-/ ) {
            last = i
          }
        }
        END { print (last ? last + 1 : 1) }
    '
}

###################################################################################################
# Initialize module (called by public functions, not on source)
###################################################################################################
_tunnel_init() {
    # Build valid tables list
    _tunnel_valid_tables=$(
        {
            awk '$0!~/^#/ && $2 ~ /^wgc[0-9]+$/ { print $2 }' /etc/iproute2/rt_tables 2>/dev/null
            awk '$0!~/^#/ && $2 ~ /^ovpnc[0-9]+$/ { print $2 }' /etc/iproute2/rt_tables 2>/dev/null
            printf '%s\n' main
        } | sort -u | xargs
    )

    # Compute mark helpers
    _tunnel_mark_mask_val=$((TUN_DIR_MARK_MASK))
    _tunnel_mark_shift_val=$((TUN_DIR_MARK_SHIFT))
    _tunnel_mark_field_max=$((_tunnel_mark_mask_val >> _tunnel_mark_shift_val))
    _tunnel_mark_mask_hex=$(printf '0x%x' "$_tunnel_mark_mask_val")

    # State directory
    mkdir -p /tmp/tunnel_director
}

###################################################################################################
# Public API
###################################################################################################

tunnel_status() {
    _tunnel_init

    echo "=== Tunnel Director Status ==="
    echo ""

    echo "--- Valid routing tables ---"
    echo "$_tunnel_valid_tables"
    echo ""

    echo "--- TUN_DIR_* chains ---"
    iptables -t mangle -S 2>/dev/null | grep -E "^-N ${TUN_DIR_CHAIN_PREFIX}" || echo "(none)"
    echo ""

    echo "--- PREROUTING jumps ---"
    iptables -t mangle -S PREROUTING 2>/dev/null | grep "${TUN_DIR_CHAIN_PREFIX}" || echo "(none)"
    echo ""

    echo "--- IP rules ---"
    ip rule show 2>/dev/null | grep -E "fwmark.*lookup" | head -20 || echo "(none)"
}

tunnel_apply() {
    local idx=0 warnings=0 changes=0
    local tun_dir_rules

    _tunnel_init

    # Normalize rules to temp file
    tun_dir_rules=$(tmp_file)
    printf '%s\n' $TUN_DIR_RULES | awk 'NF' > "$tun_dir_rules"

    if [[ ! -s $tun_dir_rules ]]; then
        log "Tunnel Director: no rules defined"
        return 0
    fi

    # Check if rebuild needed (hash comparison)
    local new_hash old_hash empty_hash
    empty_hash=$(printf '' | compute_hash)
    new_hash=$(compute_hash "$tun_dir_rules")
    old_hash=$(cat "/tmp/tunnel_director/rules.sha256" 2>/dev/null || printf '%s' "$empty_hash")

    local build_needed=0

    if [[ $new_hash != "$old_hash" ]]; then
        build_needed=1
    else
        # Hash matches, but verify no drift (chains/rules may have been cleared by firewall restart)
        local cfg_rows ch_rows pr_rows ip_rows
        cfg_rows=$(awk 'NF { c++ } END { print c + 0 }' "$tun_dir_rules")
        ch_rows=$(iptables -t mangle -S 2>/dev/null | awk -v pre="$TUN_DIR_CHAIN_PREFIX" '$1=="-N" && $2~("^"pre"[0-9]+$") { c++ } END { print c + 0 }')
        pr_rows=$(iptables -t mangle -S PREROUTING 2>/dev/null | awk -v pre="$TUN_DIR_CHAIN_PREFIX" '$0 ~ ("-j " pre "[0-9]+$") { c++ } END { print c + 0 }')

        # Count ip rules with our fwmark pattern (pref range TUN_DIR_PREF_BASE to TUN_DIR_PREF_BASE+cfg_rows)
        local tun_dir_prefs_file ip_rules_prefs_file
        tun_dir_prefs_file=$(tmp_file)
        ip_rules_prefs_file=$(tmp_file)

        awk -v base="$TUN_DIR_PREF_BASE" -v cnt="$cfg_rows" \
            'BEGIN { for (i=0; i<cnt; i++) print base+i }' > "$tun_dir_prefs_file"
        ip rule 2>/dev/null | awk -F: '/^[0-9]+:/ { print $1 }' > "$ip_rules_prefs_file"

        ip_rows=$(awk 'NR==FNR { have[$1]=1; next } have[$1] { c++ } END { print c+0 }' \
            "$ip_rules_prefs_file" "$tun_dir_prefs_file")

        if [[ $cfg_rows -ne $ch_rows ]] || [[ $cfg_rows -ne $pr_rows ]] || [[ $cfg_rows -ne $ip_rows ]]; then
            log "Tunnel Director: drift detected (config=$cfg_rows, chains=$ch_rows, jumps=$pr_rows, ip_rules=$ip_rows)"
            build_needed=1
        fi
    fi

    if [[ $build_needed -eq 0 ]]; then
        log "Tunnel Director: rules unchanged and no drift"
        return 0
    fi

    # Clean up existing rules
    log "Tunnel Director: applying rules..."

    local chains
    chains=$(iptables -t mangle -S 2>/dev/null | awk -v pre="$TUN_DIR_CHAIN_PREFIX" '$1=="-N" && $2~("^"pre"[0-9]+$") {print $2}')

    while IFS= read -r ch; do
        [[ -n $ch ]] || continue
        local idx_num="${ch#"$TUN_DIR_CHAIN_PREFIX"}"
        local pref=$((TUN_DIR_PREF_BASE + idx_num))

        purge_fw_rules -q "mangle PREROUTING" "-j ${ch}\$"
        delete_fw_chain -q mangle "$ch"
        ip rule del pref "$pref" 2>/dev/null || true
    done <<< "$chains"

    # Apply new rules
    local base_pos
    base_pos=$(_tunnel_get_prerouting_base_pos)

    while IFS=: read -r table src src_excl set set_excl; do
        # Validate table
        if ! _tunnel_table_allowed "$table"; then
            log -l WARN "Tunnel Director: invalid table '$table'; skipping rule $idx"
            warnings=1; idx=$((idx + 1)); continue
        fi

        # Validate src
        if [[ -z $src ]]; then
            log -l WARN "Tunnel Director: missing src; skipping rule $idx"
            warnings=1; idx=$((idx + 1)); continue
        fi

        # Check fwmark slot limit before processing
        local slot=$((idx + 1))
        if [[ $slot -gt $_tunnel_mark_field_max ]]; then
            log -l WARN "Tunnel Director: rule $idx exceeds fwmark capacity ($_tunnel_mark_field_max max); skipping"
            warnings=1; idx=$((idx + 1)); continue
        fi

        # Parse interface from src
        local src_iif="br0"
        case "$src" in
            *%*) src_iif="${src#*%}"; src="${src%%%*}" ;;
        esac

        # Validate interface exists
        if [[ -z $src_iif ]]; then
            log -l WARN "Tunnel Director: empty interface after '%'; skipping rule $idx"
            warnings=1; idx=$((idx + 1)); continue
        fi
        if [[ ! -d /sys/class/net/$src_iif ]]; then
            log -l WARN "Tunnel Director: interface '$src_iif' not found; skipping rule $idx"
            warnings=1; idx=$((idx + 1)); continue
        fi

        # Validate src is a LAN/private CIDR
        local src_ip="${src%%/*}"
        if ! is_lan_ip "$src_ip"; then
            log -l WARN "Tunnel Director: src '$src' is not RFC1918 subnet; skipping rule $idx"
            warnings=1; idx=$((idx + 1)); continue
        fi

        # Validate set
        if [[ -z $set ]]; then
            log -l WARN "Tunnel Director: missing set; skipping rule $idx"
            warnings=1; idx=$((idx + 1)); continue
        fi

        # Build match clause
        local match="" dest_set
        if [[ $set != "any" ]]; then
            dest_set=$(_tunnel_resolve_set "$set")
            if [[ -z $dest_set ]]; then
                log -l WARN "Tunnel Director: ipset '$set' not found; skipping rule $idx"
                warnings=1; idx=$((idx + 1)); continue
            fi
            match="-m set --match-set $dest_set dst"
        fi

        # Build exclusion clause (fail-safe: skip rule if excl ipset not found)
        local excl=""
        if [[ -n $set_excl ]]; then
            local dest_set_excl
            dest_set_excl=$(_tunnel_resolve_set "$set_excl")
            if [[ -z $dest_set_excl ]]; then
                log -l WARN "Tunnel Director: exclusion ipset '$set_excl' not found; skipping rule $idx"
                warnings=1; idx=$((idx + 1)); continue
            fi
            excl="-m set ! --match-set $dest_set_excl dst"
        fi

        # Compute fwmark
        local slot=$((idx + 1))
        local mark_val=$((slot << _tunnel_mark_shift_val))
        local mark_val_hex=$(printf '0x%x' "$mark_val")

        # Compute chain name and pref
        local pref=$((TUN_DIR_PREF_BASE + idx))
        local chain="${TUN_DIR_CHAIN_PREFIX}${idx}"

        # Create chain
        create_fw_chain -q -f mangle "$chain"

        # Add source exclusions (validate each is private RFC1918)
        if [[ -n $src_excl ]]; then
            local -a excl_array
            IFS=',' read -ra excl_array <<< "$src_excl"
            for ex in "${excl_array[@]}"; do
                [[ -n $ex ]] || continue
                local ex_ip="${ex%%/*}"
                if ! is_lan_ip "$ex_ip"; then
                    log -l WARN "Tunnel Director: src_excl='$ex' is not RFC1918; skipping"
                    warnings=1; continue
                fi
                ensure_fw_rule -q mangle "$chain" -s "$ex" -j RETURN
            done
        fi

        # Add main rule
        ensure_fw_rule -q mangle "$chain" $match $excl \
            -j MARK --set-xmark "$mark_val_hex/$_tunnel_mark_mask_hex"

        # Jump from PREROUTING
        local insert_pos=$((base_pos + idx))
        sync_fw_rule -q mangle PREROUTING "-j $chain$" \
            "-s $src -i $src_iif -m mark --mark 0x0/$_tunnel_mark_mask_hex -j $chain" "$insert_pos"

        # Add ip rule
        ip rule del pref "$pref" 2>/dev/null || true
        ip rule add pref "$pref" fwmark "$mark_val_hex/$_tunnel_mark_mask_hex" lookup "$table" 2>/dev/null || true

        log "Tunnel Director: added rule $idx -> $table (chain=$chain)"
        changes=1; idx=$((idx + 1))
    done < "$tun_dir_rules"

    # Save hash
    printf '%s\n' "$new_hash" > "/tmp/tunnel_director/rules.sha256"

    if [[ $warnings -eq 0 ]]; then
        log "Tunnel Director: applied $idx rules successfully"
    else
        log -l WARN "Tunnel Director: applied with warnings"
    fi
}

tunnel_stop() {
    log "Tunnel Director: stopping..."

    _tunnel_init

    local chains
    chains=$(iptables -t mangle -S 2>/dev/null | awk -v pre="$TUN_DIR_CHAIN_PREFIX" '$1=="-N" && $2~("^"pre"[0-9]+$") {print $2}')

    while IFS= read -r ch; do
        [[ -n $ch ]] || continue
        local idx_num="${ch#"$TUN_DIR_CHAIN_PREFIX"}"
        local pref=$((TUN_DIR_PREF_BASE + idx_num))

        purge_fw_rules -q "mangle PREROUTING" "-j ${ch}\$"
        delete_fw_chain -q mangle "$ch"
        ip rule del pref "$pref" 2>/dev/null || true
    done <<< "$chains"

    rm -f "/tmp/tunnel_director/rules.sha256"
    log "Tunnel Director: stopped"
}

tunnel_get_required_ipsets() {
    local -a rules_array
    read -ra rules_array <<< "${TUN_DIR_RULES:-}"

    printf '%s\n' "${rules_array[@]}" | awk 'NF' | _parse_country_codes
    printf '%s\n' "${rules_array[@]}" | _parse_combo_sets
}

###################################################################################################
# Allow sourcing for testing (guard at end so all functions are defined)
###################################################################################################
if [[ ${1:-} == "--source-only" ]]; then
    return 0 2>/dev/null || exit 0
fi

# When run directly, show status
tunnel_status
```

**Step 4: Run tests**

Run: `npx bats test/unit/tunnel.bats`
Expected: PASS

**Step 5: Commit**

```bash
git add -A
git commit -m "$(cat <<'EOF'
feat(lib): add tunnel.sh module

Migrate Tunnel Director logic to modular lib/tunnel.sh:
- tunnel_status(): show chains, ip rules, fwmarks
- tunnel_apply(): apply rules from config (idempotent)
- tunnel_stop(): remove all chains and ip rules
- tunnel_get_required_ipsets(): return needed ipsets
EOF
)"
```

---

## Task 4: Create lib/tproxy.sh module

**Files:**
- Create: `jffs/scripts/vpn-director/lib/tproxy.sh`
- Test: `test/unit/tproxy.bats`

**Step 1: Write failing tests**

Create `test/unit/tproxy.bats`:

```bash
#!/usr/bin/env bats

load '../test_helper'

load_tproxy_module() {
    load_common
    load_config
    source "$LIB_DIR/ipset.sh" --source-only
    source "$LIB_DIR/firewall.sh"
    source "$LIB_DIR/tproxy.sh" --source-only
}

@test "tproxy_check_module: returns success when module loaded" {
    load_tproxy_module
    # Mock lsmod shows xt_TPROXY
    run _tproxy_check_module
    assert_success
}

@test "tproxy_get_required_ipsets: returns exclude sets" {
    load_tproxy_module
    export XRAY_EXCLUDE_SETS="ru ua"
    run tproxy_get_required_ipsets
    assert_success
    assert_output --partial "ru"
    assert_output --partial "ua"
}

@test "tproxy_status: outputs chain info" {
    load_tproxy_module
    run tproxy_status
    assert_success
    assert_output --partial "Xray TPROXY Status"
}
```

**Step 2: Run tests to verify they fail**

Run: `npx bats test/unit/tproxy.bats`
Expected: FAIL

**Step 3: Create lib/tproxy.sh**

Create `jffs/scripts/vpn-director/lib/tproxy.sh`:

```bash
#!/usr/bin/env bash

###################################################################################################
# tproxy.sh - Xray TPROXY module for VPN Director
# -------------------------------------------------------------------------------------------------
# Public API:
#   tproxy_status            - Show XRAY_TPROXY chain, routing, xray process
#   tproxy_apply             - Apply TPROXY rules (idempotent)
#   tproxy_stop              - Remove chain and routing
#   tproxy_get_required_ipsets - Return list of exclude ipsets needed
###################################################################################################

# shellcheck disable=SC2086

set -euo pipefail

if [[ ${DEBUG:-0} == 1 ]]; then
    set -x
    PS4='+${BASH_SOURCE[0]##*/}:${LINENO}:${FUNCNAME[0]:-main}: '
fi

###################################################################################################
# Pure functions (for testing)
###################################################################################################

_tproxy_check_module() {
    if ! lsmod | grep -q xt_TPROXY; then
        modprobe xt_TPROXY 2>/dev/null || {
            log -l ERROR "xt_TPROXY module not available"
            return 1
        }
    fi
    return 0
}

_tproxy_resolve_exclude_set() {
    local set_key="$1"
    local ext_set="${set_key}_ext"

    if ipset list "$ext_set" >/dev/null 2>&1; then
        printf '%s\n' "$ext_set"
        return 0
    fi

    if ipset list "$set_key" >/dev/null 2>&1; then
        printf '%s\n' "$set_key"
        return 0
    fi

    return 1
}

###################################################################################################
# Internal functions (called by public functions, not on source)
###################################################################################################

_tproxy_setup_routing() {
    local rt_exists rule_exists

    rt_exists=$(ip route show table "$XRAY_ROUTE_TABLE" 2>/dev/null | grep -c "local default" || true)
    rule_exists=$(ip rule show 2>/dev/null | grep -c "fwmark $XRAY_FWMARK.*lookup $XRAY_ROUTE_TABLE" || true)

    if [[ $rt_exists -eq 0 ]]; then
        ip route add local default dev lo table "$XRAY_ROUTE_TABLE"
        log "TPROXY: added route table $XRAY_ROUTE_TABLE"
    fi

    if [[ $rule_exists -eq 0 ]]; then
        ip rule add pref "$XRAY_RULE_PREF" fwmark "$XRAY_FWMARK/$XRAY_FWMARK_MASK" table "$XRAY_ROUTE_TABLE"
        log "TPROXY: added ip rule pref $XRAY_RULE_PREF"
    fi
}

_tproxy_teardown_routing() {
    ip rule del pref "$XRAY_RULE_PREF" 2>/dev/null || true
    ip route del local default dev lo table "$XRAY_ROUTE_TABLE" 2>/dev/null || true
    log "TPROXY: removed routing"
}

_tproxy_setup_clients_ipset() {
    local ip
    local -a clients_array
    read -ra clients_array <<< "$XRAY_CLIENTS"

    if ! ipset list "$XRAY_CLIENTS_IPSET" >/dev/null 2>&1; then
        ipset create "$XRAY_CLIENTS_IPSET" hash:net
        log "TPROXY: created $XRAY_CLIENTS_IPSET"
    fi

    ipset flush "$XRAY_CLIENTS_IPSET"

    for ip in "${clients_array[@]}"; do
        [[ -n $ip ]] || continue
        ipset add "$XRAY_CLIENTS_IPSET" "$ip" 2>/dev/null || true
    done

    log "TPROXY: populated $XRAY_CLIENTS_IPSET (${#clients_array[@]} entries)"
}

_tproxy_setup_servers_ipset() {
    local ip
    local -a servers_array
    read -ra servers_array <<< "$XRAY_SERVERS"

    if ! ipset list "$XRAY_SERVERS_IPSET" >/dev/null 2>&1; then
        ipset create "$XRAY_SERVERS_IPSET" hash:net
        log "TPROXY: created $XRAY_SERVERS_IPSET"
    fi

    ipset flush "$XRAY_SERVERS_IPSET"

    for ip in "${servers_array[@]}"; do
        [[ -n $ip ]] || continue
        ipset add "$XRAY_SERVERS_IPSET" "$ip" 2>/dev/null || true
    done

    log "TPROXY: populated $XRAY_SERVERS_IPSET (${#servers_array[@]} entries)"
}

_tproxy_setup_iptables() {
    local exclude_set resolved_set
    local -a exclude_sets_array
    read -ra exclude_sets_array <<< "$XRAY_EXCLUDE_SETS"

    create_fw_chain -f mangle "$XRAY_CHAIN"

    # Skip if not our client
    ensure_fw_rule -q mangle "$XRAY_CHAIN" \
        -m set ! --match-set "$XRAY_CLIENTS_IPSET" src -j RETURN

    # Skip traffic to Xray servers
    ensure_fw_rule -q mangle "$XRAY_CHAIN" \
        -m set --match-set "$XRAY_SERVERS_IPSET" dst -j RETURN

    # Skip private destinations
    ensure_fw_rule -q mangle "$XRAY_CHAIN" -d 127.0.0.0/8 -j RETURN
    ensure_fw_rule -q mangle "$XRAY_CHAIN" -d 10.0.0.0/8 -j RETURN
    ensure_fw_rule -q mangle "$XRAY_CHAIN" -d 172.16.0.0/12 -j RETURN
    ensure_fw_rule -q mangle "$XRAY_CHAIN" -d 192.168.0.0/16 -j RETURN
    ensure_fw_rule -q mangle "$XRAY_CHAIN" -d 169.254.0.0/16 -j RETURN
    ensure_fw_rule -q mangle "$XRAY_CHAIN" -d 224.0.0.0/4 -j RETURN
    ensure_fw_rule -q mangle "$XRAY_CHAIN" -d 255.255.255.255/32 -j RETURN

    # Skip excluded country ipsets
    for exclude_set in "${exclude_sets_array[@]}"; do
        [[ -n $exclude_set ]] || continue
        resolved_set=$(_tproxy_resolve_exclude_set "$exclude_set") || {
            log -l WARN "TPROXY: exclude ipset '$exclude_set' not found"
            continue
        }
        ensure_fw_rule -q mangle "$XRAY_CHAIN" \
            -m set --match-set "$resolved_set" dst -j RETURN
        log "TPROXY: added exclusion for $resolved_set"
    done

    # TPROXY for remaining traffic
    ensure_fw_rule -q mangle "$XRAY_CHAIN" \
        -p tcp -j TPROXY --on-port "$XRAY_TPROXY_PORT" \
        --tproxy-mark "$XRAY_FWMARK/$XRAY_FWMARK_MASK"
    ensure_fw_rule -q mangle "$XRAY_CHAIN" \
        -p udp -j TPROXY --on-port "$XRAY_TPROXY_PORT" \
        --tproxy-mark "$XRAY_FWMARK/$XRAY_FWMARK_MASK"

    # Jump from PREROUTING
    sync_fw_rule -q mangle PREROUTING "-j $XRAY_CHAIN\$" \
        "-i br0 -j $XRAY_CHAIN" 1

    log "TPROXY: applied iptables rules"
}

_tproxy_teardown_iptables() {
    purge_fw_rules -q "mangle PREROUTING" "-j $XRAY_CHAIN\$"
    delete_fw_chain -q mangle "$XRAY_CHAIN"
    ipset destroy "$XRAY_CLIENTS_IPSET" 2>/dev/null || true
    ipset destroy "$XRAY_SERVERS_IPSET" 2>/dev/null || true
    log "TPROXY: removed iptables rules and ipsets"
}

###################################################################################################
# Public API
###################################################################################################

tproxy_status() {
    echo "=== Xray TPROXY Status ==="
    echo ""

    echo "--- Kernel Module ---"
    lsmod | grep -E 'xt_TPROXY|nf_tproxy' || echo "TPROXY module not loaded"
    echo ""

    echo "--- Routing ---"
    echo "Table $XRAY_ROUTE_TABLE:"
    ip route show table "$XRAY_ROUTE_TABLE" 2>/dev/null || echo "  (empty)"
    echo ""
    echo "IP Rules:"
    ip rule show | grep -E "$XRAY_ROUTE_TABLE|$XRAY_FWMARK" || echo "  (none)"
    echo ""

    echo "--- Clients IPSet ---"
    ipset list "$XRAY_CLIENTS_IPSET" 2>/dev/null || echo "IPSet $XRAY_CLIENTS_IPSET not found"
    echo ""

    echo "--- Servers IPSet ---"
    ipset list "$XRAY_SERVERS_IPSET" 2>/dev/null || echo "IPSet $XRAY_SERVERS_IPSET not found"
    echo ""

    echo "--- Iptables Chain ---"
    iptables -t mangle -S "$XRAY_CHAIN" 2>/dev/null || echo "Chain $XRAY_CHAIN not found"
    echo ""

    echo "--- PREROUTING Jump ---"
    iptables -t mangle -S PREROUTING 2>/dev/null | grep "$XRAY_CHAIN" || echo "No jump to $XRAY_CHAIN"
    echo ""

    echo "--- Xray Process ---"
    pgrep -la xray || echo "Xray not running"
}

tproxy_apply() {
    log "TPROXY: applying..."

    # Soft-fail: return 0 so vpn-director.sh doesn't abort under set -e
    if ! _tproxy_check_module; then
        log -l WARN "TPROXY: xt_TPROXY unavailable; skipping (soft-fail)"
        return 0
    fi

    # Check if required ipsets exist (soft-fail if missing)
    local -a exclude_sets_array
    read -ra exclude_sets_array <<< "$XRAY_EXCLUDE_SETS"
    for set_key in "${exclude_sets_array[@]}"; do
        [[ -n $set_key ]] || continue
        if ! _tproxy_resolve_exclude_set "$set_key" >/dev/null; then
            log -l WARN "TPROXY: required ipset '$set_key' not found; skipping (soft-fail)"
            return 0
        fi
    done

    _tproxy_setup_routing
    _tproxy_setup_clients_ipset
    _tproxy_setup_servers_ipset
    _tproxy_setup_iptables

    log "TPROXY: applied successfully"
}

tproxy_stop() {
    log "TPROXY: stopping..."
    _tproxy_teardown_iptables
    _tproxy_teardown_routing
    log "TPROXY: stopped"
}

tproxy_get_required_ipsets() {
    printf '%s\n' ${XRAY_EXCLUDE_SETS:-} | xargs
}

###################################################################################################
# Allow sourcing for testing (guard at end so all functions are defined)
###################################################################################################
if [[ ${1:-} == "--source-only" ]]; then
    return 0 2>/dev/null || exit 0
fi

# When run directly, show status
tproxy_status
```

**Step 4: Run tests**

Run: `npx bats test/unit/tproxy.bats`
Expected: PASS

**Note:** Unit tests for `_tproxy_check_module()` require mocks for `lsmod` and `modprobe`.
Add to `test/mocks/lsmod`:
```bash
#!/bin/bash
echo "xt_TPROXY    16384  1"
```

**Step 5: Commit**

```bash
git add -A
git commit -m "$(cat <<'EOF'
feat(lib): add tproxy.sh module

Migrate Xray TPROXY logic to modular lib/tproxy.sh:
- tproxy_status(): show chain, routing, xray process
- tproxy_apply(): apply TPROXY rules (idempotent)
- tproxy_stop(): remove chain and routing
- tproxy_get_required_ipsets(): return exclude ipsets
EOF
)"
```

---

## Task 5: Create vpn-director.sh CLI

**Files:**
- Create: `jffs/scripts/vpn-director/vpn-director.sh`
- Test: `test/integration/vpn_director.bats`

**Step 1: Write integration tests**

Create `test/integration/vpn_director.bats`:

```bash
#!/usr/bin/env bats

load '../test_helper'

setup() {
    export PATH="$BATS_TEST_DIRNAME/../mocks:$PATH"
    export VPD_CONFIG_FILE="$BATS_TEST_DIRNAME/../fixtures/vpn-director.json"
}

@test "vpn-director: shows help with no args" {
    run "$SCRIPTS_DIR/vpn-director.sh"
    assert_success
    assert_output --partial "Usage:"
}

@test "vpn-director: status command works" {
    run "$SCRIPTS_DIR/vpn-director.sh" status
    assert_success
    assert_output --partial "IPSet Status"
    assert_output --partial "Tunnel Director Status"
    assert_output --partial "Xray TPROXY Status"
}

@test "vpn-director: status ipset shows only ipset" {
    run "$SCRIPTS_DIR/vpn-director.sh" status ipset
    assert_success
    assert_output --partial "IPSet Status"
    refute_output --partial "Tunnel Director"
}

@test "vpn-director: unknown command fails" {
    run "$SCRIPTS_DIR/vpn-director.sh" unknown
    assert_failure
    assert_output --partial "Unknown command"
}

@test "vpn-director: --help shows usage" {
    run "$SCRIPTS_DIR/vpn-director.sh" --help
    assert_success
    assert_output --partial "Usage:"
}

# ============================================================================
# apply/restart/update commands
# ============================================================================

@test "vpn-director: apply calls modules in correct order" {
    # Override modules to track call order
    _call_order=""
    ipset_ensure() { _call_order="${_call_order}ipset,"; }
    tunnel_apply() { _call_order="${_call_order}tunnel,"; }
    tproxy_apply() { _call_order="${_call_order}tproxy,"; }
    tunnel_get_required_ipsets() { echo "ru"; }
    tproxy_get_required_ipsets() { echo "ua"; }
    _ipset_boot_wait() { :; }
    acquire_lock() { :; }

    source "$SCRIPTS_DIR/vpn-director.sh" --source-only
    cmd_apply

    # Verify order: ipset -> tunnel -> tproxy
    [[ "$_call_order" == "ipset,tunnel,tproxy," ]]
}

@test "vpn-director: apply --dry-run does not call apply functions" {
    local applied=0
    tunnel_apply() { applied=1; }
    tproxy_apply() { applied=1; }
    tunnel_get_required_ipsets() { echo "ru"; }
    tproxy_get_required_ipsets() { echo ""; }
    _ipset_boot_wait() { :; }
    acquire_lock() { :; }

    run "$SCRIPTS_DIR/vpn-director.sh" apply --dry-run
    assert_success
    assert_output --partial "DRY-RUN"
    [[ $applied -eq 0 ]]
}

@test "vpn-director: restart stops before apply" {
    _call_order=""
    tunnel_stop() { _call_order="${_call_order}stop,"; }
    tproxy_stop() { _call_order="${_call_order}stop,"; }
    tunnel_apply() { _call_order="${_call_order}apply,"; }
    tproxy_apply() { _call_order="${_call_order}apply,"; }
    tunnel_get_required_ipsets() { echo ""; }
    tproxy_get_required_ipsets() { echo ""; }
    ipset_ensure() { :; }
    _ipset_boot_wait() { :; }
    acquire_lock() { :; }

    source "$SCRIPTS_DIR/vpn-director.sh" --source-only
    cmd_restart

    # Stop should come before apply
    [[ "$_call_order" == "stop,stop,apply,apply," ]]
}

# ============================================================================
# Error handling and soft-fail scenarios
# ============================================================================

@test "vpn-director: tproxy soft-fails when xt_TPROXY unavailable" {
    # Mock lsmod to not show xt_TPROXY
    lsmod() { echo ""; }
    modprobe() { return 1; }

    load_tproxy_module
    run tproxy_apply
    assert_success  # Soft-fail returns 0
    assert_output --partial "soft-fail"
}

@test "vpn-director: apply continues when tproxy soft-fails" {
    _tunnel_applied=0
    tunnel_apply() { _tunnel_applied=1; }
    tproxy_apply() { log -l WARN "soft-fail"; return 0; }
    tunnel_get_required_ipsets() { echo ""; }
    tproxy_get_required_ipsets() { echo ""; }
    ipset_ensure() { :; }
    _ipset_boot_wait() { :; }
    acquire_lock() { :; }

    source "$SCRIPTS_DIR/vpn-director.sh" --source-only
    cmd_apply

    [[ $_tunnel_applied -eq 1 ]]
}

@test "vpn-director: ipset_ensure fails when download fails and no cache" {
    load_ipset_module
    # Mock curl to fail
    curl() { return 1; }
    # Ensure no cache exists
    rm -f "$_IPSET_DUMP_DIR/xx-ipdeny.dump" 2>/dev/null || true

    run ipset_ensure "xx"
    assert_failure
}

# ============================================================================
# Option parsing (both positions)
# ============================================================================

@test "vpn-director: options work before command" {
    run "$SCRIPTS_DIR/vpn-director.sh" --force apply --dry-run
    assert_success
    assert_output --partial "DRY-RUN"
}

@test "vpn-director: options work after command" {
    run "$SCRIPTS_DIR/vpn-director.sh" apply --dry-run --force
    assert_success
    assert_output --partial "DRY-RUN"
}
```

**Step 2: Run tests to verify they fail**

Run: `npx bats test/integration/vpn_director.bats`
Expected: FAIL

**Step 3: Create vpn-director.sh**

Create `jffs/scripts/vpn-director/vpn-director.sh`:

```bash
#!/usr/bin/env bash

###################################################################################################
# vpn-director.sh - Unified CLI for VPN Director
# -------------------------------------------------------------------------------------------------
# Usage:
#   vpn-director status [tunnel|xray|ipset]  - Show status
#   vpn-director apply [tunnel|xray]         - Apply configuration
#   vpn-director stop [tunnel|xray]          - Stop components
#   vpn-director restart [tunnel|xray]       - Restart components
#   vpn-director update                      - Update ipsets and reapply all
#
# Options:
#   -f, --force    Force operation (ignore hash checks)
#   -q, --quiet    Minimal output
#   -v, --verbose  Debug output
#   --dry-run      Show what would be done
#   -h, --help     Show this help
###################################################################################################

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Parse options (supports both pre-command and post-command placement)
# vpn-director --force apply     # works
# vpn-director apply --force     # works
# vpn-director apply tunnel -v   # works
FORCE=0
QUIET=0
VERBOSE=0
DRY_RUN=0
COMMAND=""
COMPONENT=""

parse_option() {
    case $1 in
        -f|--force)   FORCE=1; return 0 ;;
        -q|--quiet)   QUIET=1; return 0 ;;
        -v|--verbose) VERBOSE=1; export DEBUG=1; return 0 ;;
        --dry-run)    DRY_RUN=1; return 0 ;;
        -h|--help)    COMMAND="help"; return 0 ;;
        -*)           echo "Unknown option: $1" >&2; exit 1 ;;
        *)            return 1 ;;
    esac
}

# Phase 1: Parse pre-command options and extract command/component
while [[ $# -gt 0 ]]; do
    if parse_option "$1"; then
        shift
    elif [[ -z $COMMAND ]]; then
        COMMAND="$1"; shift
    elif [[ -z $COMPONENT ]]; then
        COMPONENT="$1"; shift
    else
        # Extra positional argument
        echo "Unexpected argument: $1" >&2; exit 1
    fi
done

# Phase 2: Default command
COMMAND="${COMMAND:-help}"

# Debug mode
if [[ $VERBOSE -eq 1 ]]; then
    set -x
    PS4='+${BASH_SOURCE[0]##*/}:${LINENO}:${FUNCNAME[0]:-main}: '
fi

###################################################################################################
# Load modules
###################################################################################################
. "$SCRIPT_DIR/lib/common.sh"
. "$SCRIPT_DIR/lib/firewall.sh"
. "$SCRIPT_DIR/lib/config.sh"
. "$SCRIPT_DIR/lib/ipset.sh" --source-only
. "$SCRIPT_DIR/lib/tunnel.sh" --source-only
. "$SCRIPT_DIR/lib/tproxy.sh" --source-only

###################################################################################################
# Help
###################################################################################################
show_help() {
    cat <<'EOF'
VPN Director - Unified traffic routing for Asuswrt-Merlin

Usage:
  vpn-director <command> [component] [options]

Commands:
  status [tunnel|xray|ipset]  Show status (all or specific component)
  apply [tunnel|xray]         Apply configuration
  stop [tunnel|xray]          Stop components
  restart [tunnel|xray]       Restart (stop + apply)
  update                      Download fresh ipsets and reapply all

Options:
  -f, --force    Force operation (ignore hash checks)
  -q, --quiet    Minimal output
  -v, --verbose  Debug output
  --dry-run      Show what would be done
  -h, --help     Show this help

Examples:
  vpn-director status              # Show all status
  vpn-director apply               # Apply all (ipsets + tunnel + xray)
  vpn-director restart tunnel      # Restart only Tunnel Director
  vpn-director update              # Update ipsets from IPdeny

Alias:
  vpd                              # Short alias (add to profile.add)
EOF
}

###################################################################################################
# Commands
###################################################################################################

cmd_status() {
    case "$COMPONENT" in
        ""|all)
            ipset_status
            echo ""
            tunnel_status
            echo ""
            tproxy_status
            ;;
        ipset)
            ipset_status
            ;;
        tunnel)
            tunnel_status
            ;;
        xray|tproxy)
            tproxy_status
            ;;
        *)
            echo "Unknown component: $COMPONENT" >&2
            exit 1
            ;;
    esac
}

cmd_apply() {
    acquire_lock "vpn-director"

    # Wait for network if system just booted (before any downloads)
    _ipset_boot_wait

    # Handle --dry-run: show plan without applying
    if [[ $DRY_RUN -eq 1 ]]; then
        log "DRY-RUN: would apply configuration"
        log "DRY-RUN: tunnel ipsets needed: $(tunnel_get_required_ipsets)"
        log "DRY-RUN: tproxy ipsets needed: $(tproxy_get_required_ipsets)"
        return 0
    fi

    # Handle --force: pass to ipset module
    [[ $FORCE -eq 1 ]] && export IPSET_FORCE_UPDATE=1

    # Handle --quiet: reduce log level
    # Note: LOG_LEVEL filtering not yet implemented in log() - this is a placeholder
    # [[ $QUIET -eq 1 ]] && export LOG_LEVEL=WARN

    local required_ipsets=""

    case "$COMPONENT" in
        ""|all)
            required_ipsets="$(tunnel_get_required_ipsets) $(tproxy_get_required_ipsets)"
            required_ipsets=$(echo $required_ipsets | xargs -n1 | sort -u | xargs)

            if [[ -n $required_ipsets ]]; then
                log "Ensuring ipsets: $required_ipsets"
                ipset_ensure $required_ipsets
            fi

            tunnel_apply
            tproxy_apply
            ;;
        tunnel)
            required_ipsets=$(tunnel_get_required_ipsets)
            if [[ -n $required_ipsets ]]; then
                ipset_ensure $required_ipsets
            fi
            tunnel_apply
            ;;
        xray|tproxy)
            required_ipsets=$(tproxy_get_required_ipsets)
            if [[ -n $required_ipsets ]]; then
                ipset_ensure $required_ipsets
            fi
            tproxy_apply
            ;;
        *)
            echo "Unknown component: $COMPONENT" >&2
            exit 1
            ;;
    esac
}

cmd_stop() {
    acquire_lock "vpn-director"

    case "$COMPONENT" in
        ""|all)
            tproxy_stop
            tunnel_stop
            ;;
        tunnel)
            tunnel_stop
            ;;
        xray|tproxy)
            tproxy_stop
            ;;
        *)
            echo "Unknown component: $COMPONENT" >&2
            exit 1
            ;;
    esac
}

cmd_restart() {
    case "$COMPONENT" in
        ""|all)
            cmd_stop
            cmd_apply
            ;;
        tunnel)
            COMPONENT=tunnel cmd_stop
            COMPONENT=tunnel cmd_apply
            ;;
        xray|tproxy)
            COMPONENT=xray cmd_stop
            COMPONENT=xray cmd_apply
            ;;
        *)
            echo "Unknown component: $COMPONENT" >&2
            exit 1
            ;;
    esac
}

cmd_update() {
    acquire_lock "vpn-director"

    # Wait for network if system just booted (before any downloads)
    _ipset_boot_wait

    local required_ipsets
    required_ipsets="$(tunnel_get_required_ipsets) $(tproxy_get_required_ipsets)"
    required_ipsets=$(echo $required_ipsets | xargs -n1 | sort -u | xargs)

    if [[ -n $required_ipsets ]]; then
        log "Updating ipsets: $required_ipsets"
        IPSET_FORCE_UPDATE=1 ipset_ensure $required_ipsets
    fi

    tunnel_apply
    tproxy_apply

    log "Update complete"
}

###################################################################################################
# Main
###################################################################################################

case "$COMMAND" in
    help|--help|-h)
        show_help
        ;;
    status)
        cmd_status
        ;;
    apply)
        cmd_apply
        ;;
    stop)
        cmd_stop
        ;;
    restart)
        cmd_restart
        ;;
    update)
        cmd_update
        ;;
    *)
        echo "Unknown command: $COMMAND" >&2
        echo "Run 'vpn-director --help' for usage" >&2
        exit 1
        ;;
esac
```

**Step 4: Make executable**

```bash
chmod +x jffs/scripts/vpn-director/vpn-director.sh
```

**Step 5: Run tests**

Run: `npx bats test/integration/vpn_director.bats`
Expected: PASS

**Step 6: Commit**

```bash
git add -A
git commit -m "$(cat <<'EOF'
feat: add vpn-director.sh unified CLI

Single entry point for all VPN Director operations:
- status: show component status
- apply: apply configuration (idempotent)
- stop: stop components
- restart: stop + apply
- update: refresh ipsets and reapply
EOF
)"
```

---

## Task 6: Update system hooks

**Files:**
- Modify: `jffs/scripts/firewall-start`
- Modify: `jffs/scripts/wan-event`
- Modify: `opt/etc/init.d/S99vpn-director`
- Modify: `jffs/configs/profile.add`

**Step 1: Update firewall-start**

Replace content of `jffs/scripts/firewall-start`:

```bash
#!/bin/sh

###################################################################################################
# firewall-start - Asuswrt-Merlin hook invoked when the firewall stack reloads
###################################################################################################

[ -x /opt/bin/bash ] || exit 0
/jffs/scripts/vpn-director/vpn-director.sh apply
```

**Step 2: Update wan-event**

Replace content of `jffs/scripts/wan-event`:

```bash
#!/bin/sh

###################################################################################################
# wan-event - Asuswrt-Merlin hook invoked when WAN state changes
###################################################################################################

PATH=/opt/sbin:/opt/bin:/usr/sbin:/usr/bin:/sbin:/bin

[ -x /opt/bin/bash ] || exit 0

case "$2" in
    connected)
        /jffs/scripts/vpn-director/vpn-director.sh apply
        ;;
esac
```

**Step 3: Update S99vpn-director**

Replace content of `opt/etc/init.d/S99vpn-director`:

```bash
#!/bin/sh

PATH=/opt/sbin:/opt/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

ENABLED=yes
DESC="VPN Director"
VPD="/jffs/scripts/vpn-director/vpn-director.sh"

start() {
    echo "Starting $DESC..."
    $VPD apply
    cru a vpn_director_update "0 3 * * * $VPD update"
    /jffs/scripts/vpn-director/lib/send-email.sh "Startup" "VPN Director started" 2>/dev/null || true
}

stop() {
    echo "Stopping $DESC..."
    $VPD stop
    cru d vpn_director_update
}

case "$1" in
    start)   start ;;
    stop)    stop ;;
    restart) stop; start ;;
    status)  $VPD status ;;
    *)       echo "Usage: $0 {start|stop|restart|status}" ;;
esac
```

**Step 4: Update profile.add**

Update `jffs/configs/profile.add`:

```bash
# Remove old alias
# alias ipt='/jffs/scripts/vpn-director/ipset_builder.sh -t'

# Add new aliases
alias vpd='/jffs/scripts/vpn-director/vpn-director.sh'
alias ipt='vpd apply'  # Backward compatibility
```

**Step 5: Commit**

```bash
git add -A
git commit -m "$(cat <<'EOF'
refactor: update system hooks to use vpn-director.sh

- firewall-start: call vpn-director apply
- wan-event: call vpn-director apply on connected
- S99vpn-director: use new CLI for start/stop/status
- profile.add: add vpd alias
EOF
)"
```

---

## Task 7: Update all call sites before removing old scripts

**Files to update:**
- `install.sh` — lines 99, 102, 110, 152-154
- `jffs/scripts/vpn-director/configure.sh` — lines 403-424, 449
- `telegram-bot/internal/bot/wizard_handlers.go` — lines 487-502
- `telegram-bot/internal/bot/handlers.go` — lines 39, 80, 93
- `.claude/rules/*.md` — references to old scripts

**Step 1: Update install.sh**

Replace in `install.sh`:

```bash
# Old:
"$SCRIPT_DIR/ipset_builder.sh" -t -x
cru a update_ipsets "0 3 * * * $SCRIPT_DIR/ipset_builder.sh -u -t -x"
"$SCRIPT_DIR/xray_tproxy.sh" stop

# New:
"$SCRIPT_DIR/vpn-director.sh" apply
cru a vpn_director_update "0 3 * * * $SCRIPT_DIR/vpn-director.sh update"
"$SCRIPT_DIR/vpn-director.sh" stop xray
```

Also update the files list (lines 152-154):
```bash
# Old:
"jffs/scripts/vpn-director/ipset_builder.sh" \
"jffs/scripts/vpn-director/tunnel_director.sh" \
"jffs/scripts/vpn-director/xray_tproxy.sh" \
"jffs/scripts/vpn-director/utils/common.sh" \
"jffs/scripts/vpn-director/utils/firewall.sh" \
"jffs/scripts/vpn-director/utils/shared.sh" \
"jffs/scripts/vpn-director/utils/config.sh" \
"jffs/scripts/vpn-director/utils/send-email.sh" \

# New:
"jffs/scripts/vpn-director/vpn-director.sh" \
"jffs/scripts/vpn-director/lib/common.sh" \
"jffs/scripts/vpn-director/lib/firewall.sh" \
"jffs/scripts/vpn-director/lib/config.sh" \
"jffs/scripts/vpn-director/lib/ipset.sh" \
"jffs/scripts/vpn-director/lib/tunnel.sh" \
"jffs/scripts/vpn-director/lib/tproxy.sh" \
"jffs/scripts/vpn-director/lib/send-email.sh" \
```

**Step 2: Update configure.sh**

Replace in `jffs/scripts/vpn-director/configure.sh`:

```bash
# Old (lines 403-424):
if [[ -x $JFFS_DIR/ipset_builder.sh ]]; then
    "$JFFS_DIR/ipset_builder.sh" -c "$XRAY_EXCLUDE_SETS_LIST" || ...
if [[ -x $JFFS_DIR/tunnel_director.sh ]]; then
    "$JFFS_DIR/tunnel_director.sh" || ...
if [[ -x $JFFS_DIR/xray_tproxy.sh ]]; then
    "$JFFS_DIR/xray_tproxy.sh" || ...

# New:
if [[ -x $JFFS_DIR/vpn-director.sh ]]; then
    "$JFFS_DIR/vpn-director.sh" apply || {
        print_warning "vpn-director.sh apply failed"
    }
fi
```

Replace line 449:
```bash
# Old:
printf "Check status with: /jffs/scripts/vpn-director/xray_tproxy.sh status\n"

# New:
printf "Check status with: /jffs/scripts/vpn-director/vpn-director.sh status\n"
```

**Step 3: Update telegram-bot handlers**

Update `telegram-bot/internal/bot/wizard_handlers.go` (lines 487-502):

```go
// Old:
result, err := shell.Exec(scriptsDir+"/ipset_builder.sh", "-t", "-x")
result, err = shell.Exec(scriptsDir+"/xray_tproxy.sh", "restart")

// New:
result, err := shell.Exec(scriptsDir+"/vpn-director.sh", "apply")
result, err = shell.Exec(scriptsDir+"/vpn-director.sh", "restart", "xray")
```

Update `telegram-bot/internal/bot/handlers.go`:

```go
// Old (line 39):
result, err := shell.Exec(scriptsDir+"/xray_tproxy.sh", "status")
// New:
result, err := shell.Exec(scriptsDir+"/vpn-director.sh", "status")

// Old (line 80):
result, err := shell.Exec(scriptsDir+"/xray_tproxy.sh", "restart")
// New:
result, err := shell.Exec(scriptsDir+"/vpn-director.sh", "restart", "xray")

// Old (line 93):
result, err := shell.Exec(scriptsDir+"/xray_tproxy.sh", "stop")
// New:
result, err := shell.Exec(scriptsDir+"/vpn-director.sh", "stop", "xray")
```

**Step 4: Commit**

```bash
git add -A
git commit -m "$(cat <<'EOF'
refactor: update all call sites to use vpn-director.sh

- install.sh: use vpn-director.sh apply/update/stop
- configure.sh: use vpn-director.sh apply
- telegram-bot: update Go handlers to new CLI
EOF
)"
```

---

## Task 8: Remove old scripts

**Files:**
- Delete: `jffs/scripts/vpn-director/ipset_builder.sh`
- Delete: `jffs/scripts/vpn-director/tunnel_director.sh`
- Delete: `jffs/scripts/vpn-director/xray_tproxy.sh`
- Delete: `jffs/scripts/vpn-director/lib/shared.sh`

**Step 1: Verify no remaining references**

```bash
grep -r "ipset_builder.sh\|tunnel_director.sh\|xray_tproxy.sh\|utils/shared.sh" \
    --include="*.sh" --include="*.md" --include="*.go" \
    jffs/ opt/ test/ docs/ telegram-bot/ || echo "No references found"
```

**Step 2: Delete old files**

```bash
git rm jffs/scripts/vpn-director/ipset_builder.sh
git rm jffs/scripts/vpn-director/tunnel_director.sh
git rm jffs/scripts/vpn-director/xray_tproxy.sh
git rm jffs/scripts/vpn-director/lib/shared.sh
```

**Step 3: Commit**

```bash
git commit -m "$(cat <<'EOF'
refactor: remove deprecated scripts

Removed in favor of modular lib/* and vpn-director.sh:
- ipset_builder.sh -> lib/ipset.sh
- tunnel_director.sh -> lib/tunnel.sh
- xray_tproxy.sh -> lib/tproxy.sh
- lib/shared.sh -> merged into lib/ipset.sh
EOF
)"
```

---

## Task 9: Update tests

**Files:**
- Delete: `test/ipset_builder.bats`
- Delete: `test/tunnel_director.bats`
- Modify: `test/test_helper.bash`

**Step 1: Remove old test files**

```bash
git rm test/ipset_builder.bats
git rm test/tunnel_director.bats
```

**Step 2: Update test_helper.bash**

Update `test/test_helper.bash` to use `$LIB_DIR`:

```bash
# test/test_helper.bash

load '/usr/lib/bats/bats-support/load.bash'
load '/usr/lib/bats/bats-assert/load.bash'

export PROJECT_ROOT="$BATS_TEST_DIRNAME/.."
export SCRIPTS_DIR="$PROJECT_ROOT/jffs/scripts/vpn-director"
export LIB_DIR="$SCRIPTS_DIR/lib"

export TEST_MODE=1
export LOG_FILE="/tmp/bats_test_vpn_director.log"

setup() {
    export PATH="$BATS_TEST_DIRNAME/mocks:$PATH"
    export HOSTS_FILE="$BATS_TEST_DIRNAME/fixtures/hosts"
    : > "$LOG_FILE"
}

teardown() {
    rm -f /tmp/bats_test_*
}

load_common() {
    export BASH_SOURCE_OVERRIDE="$SCRIPTS_DIR/test_script.sh"
    source "$LIB_DIR/common.sh"
}

load_firewall() {
    load_common
    source "$LIB_DIR/firewall.sh"
}

load_config() {
    export VPD_CONFIG_FILE="$BATS_TEST_DIRNAME/fixtures/vpn-director.json"
    source "$LIB_DIR/config.sh"
}

load_ipset_module() {
    load_common
    load_config
    source "$LIB_DIR/ipset.sh" --source-only
}

load_tunnel_module() {
    load_common
    load_config
    source "$LIB_DIR/ipset.sh" --source-only
    source "$LIB_DIR/firewall.sh"
    source "$LIB_DIR/tunnel.sh" --source-only
}

load_tproxy_module() {
    load_common
    load_config
    source "$LIB_DIR/ipset.sh" --source-only
    source "$LIB_DIR/firewall.sh"
    source "$LIB_DIR/tproxy.sh" --source-only
}

# Helper to source import_server_list.sh without running main
load_import_server_list() {
    load_common
    export IMPORT_TEST_MODE=1
    source "$SCRIPTS_DIR/import_server_list.sh"
}
```

**Step 3: Run all tests**

Run: `npx bats test/`
Expected: All PASS

**Step 4: Commit**

```bash
git add -A
git commit -m "$(cat <<'EOF'
test: migrate tests to new module structure

- Remove old ipset_builder.bats and tunnel_director.bats
- Update test_helper.bash with new load_* helpers
- Add unit tests in test/unit/
- Add integration tests in test/integration/
EOF
)"
```

---

## Task 10: Update documentation

**Files:**
- Modify: `CLAUDE.md`
- Modify: `README.md`
- Modify: `.claude/rules/ipset-builder.md`
- Modify: `.claude/rules/tunnel-director.md`
- Modify: `.claude/rules/xray-tproxy.md`
- Modify: `.claude/rules/telegram-bot.md`
- Modify: `.claude/rules/testing.md`

**Step 1: Update CLAUDE.md**

Update commands section in `CLAUDE.md`:

```markdown
## Commands

```bash
# Install
curl -fsSL https://raw.githubusercontent.com/zinin/asuswrt-merlin-vpn-director/master/install.sh | bash

# VPN Director CLI
/jffs/scripts/vpn-director/vpn-director.sh status              # Show all status
/jffs/scripts/vpn-director/vpn-director.sh apply               # Apply configuration
/jffs/scripts/vpn-director/vpn-director.sh stop                # Stop all components
/jffs/scripts/vpn-director/vpn-director.sh restart             # Restart all
/jffs/scripts/vpn-director/vpn-director.sh update              # Update ipsets + reapply
/jffs/scripts/vpn-director/vpn-director.sh status tunnel       # Tunnel Director status only
/jffs/scripts/vpn-director/vpn-director.sh restart xray        # Restart Xray TPROXY only

# Shell alias
vpd status    # Short form
vpd apply
vpd update
```
```

**Step 2: Update architecture section**

```markdown
## Architecture

| Path | Purpose |
|------|---------|
| `jffs/scripts/vpn-director/vpn-director.sh` | Unified CLI entry point |
| `jffs/scripts/vpn-director/lib/` | Modules: common, firewall, config, ipset, tunnel, tproxy |
| `opt/etc/init.d/S99vpn-director` | Entware init.d script |
| `jffs/scripts/firewall-start` | Merlin firewall hook |
| `jffs/scripts/wan-event` | Merlin WAN hook |
| `test/` | Bats tests (unit/, integration/) |
```

**Step 3: Update .claude/rules files**

Update each rules file to reference `lib/*.sh` instead of standalone scripts.

**Step 4: Commit**

```bash
git add -A
git commit -m "$(cat <<'EOF'
docs: update documentation for new architecture

- CLAUDE.md: update commands to use vpn-director.sh
- .claude/rules/*: reference lib/ modules
EOF
)"
```

---

## Summary

| Task | Description | Files |
|------|-------------|-------|
| 1 | Rename utils/ to lib/ | Directory + source paths |
| 2 | Create lib/ipset.sh | New module + unit tests |
| 3 | Create lib/tunnel.sh | New module + unit tests |
| 4 | Create lib/tproxy.sh | New module + unit tests |
| 5 | Create vpn-director.sh | CLI + integration tests |
| 6 | Update system hooks | firewall-start, wan-event, S99, profile |
| 7 | Update call sites | install.sh, configure.sh, telegram-bot/*.go |
| 8 | Remove old scripts | ipset_builder, tunnel_director, xray_tproxy |
| 9 | Update tests | Migrate to unit/integration structure |
| 10 | Update documentation | CLAUDE.md, README.md, .claude/rules/* |

**Total commits:** 10
