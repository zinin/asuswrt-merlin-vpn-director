#!/usr/bin/env bash

###################################################################################################
# tunnel.sh - tunnel director module for VPN Director
# -------------------------------------------------------------------------------------------------
# Purpose:
#   Modular library for tunnel director operations: status, apply, stop.
#   Migrated from tunnel_director.sh to provide independent, testable functions.
#
# Dependencies:
#   - common.sh (log, tmp_file, compute_hash, is_lan_ip)
#   - firewall.sh (create_fw_chain, delete_fw_chain, ensure_fw_rule, sync_fw_rule, purge_fw_rules)
#   - config.sh (TUN_DIR_RULES, TUN_DIR_CHAIN_PREFIX, TUN_DIR_PREF_BASE, TUN_DIR_MARK_MASK,
#                TUN_DIR_MARK_SHIFT)
#   - ipset.sh (_derive_set_name, parse_country_codes, parse_combo_from_rules, TUN_DIR_HASH)
#
# Public API:
#   tunnel_status()              - show TUN_DIR_* chains, ip rules, fwmarks
#   tunnel_apply()               - apply rules from config (idempotent)
#   tunnel_stop()                - remove all chains and ip rules
#   tunnel_get_required_ipsets() - return list of ipsets needed for rules
#
# Internal functions (for testing):
#   _tunnel_table_allowed()         - check if routing table is valid (wgcN, ovpncN, main)
#   _tunnel_resolve_set()           - resolve ipset name using _derive_set_name
#   _tunnel_get_prerouting_base_pos() - find insert position after system rules
#   _tunnel_init()                  - initialize module state
#
# Usage:
#   source lib/tunnel.sh              # source and run main if any
#   source lib/tunnel.sh --source-only # source only for testing
###################################################################################################

# -------------------------------------------------------------------------------------------------
# Disable unneeded shellcheck warnings
# -------------------------------------------------------------------------------------------------
# shellcheck disable=SC2086
# shellcheck disable=SC2155

# -------------------------------------------------------------------------------------------------
# Abort script on any error
# -------------------------------------------------------------------------------------------------
set -euo pipefail

# -------------------------------------------------------------------------------------------------
# Debug mode: set DEBUG=1 to enable tracing
# -------------------------------------------------------------------------------------------------
if [[ ${DEBUG:-0} == 1 ]]; then
    set -x
    PS4='+${BASH_SOURCE[0]##*/}:${LINENO}:${FUNCNAME[0]:-main}: '
fi

###################################################################################################
# Module state variables (initialized by _tunnel_init)
###################################################################################################

# Valid routing tables (space-separated)
_tunnel_valid_tables=""

# Fwmark helpers (computed from config)
_tunnel_mark_mask_val=""
_tunnel_mark_shift_val=""
_tunnel_mark_field_max=""
_tunnel_mark_mask_hex=""

# Initialization flag
_tunnel_initialized=0

###################################################################################################
# Internal helper functions (defined before --source-only for testability)
###################################################################################################

# -------------------------------------------------------------------------------------------------
# _tunnel_init - initialize module state
# -------------------------------------------------------------------------------------------------
# Builds valid_tables list from rt_tables and computes fwmark helpers.
# Safe to call multiple times (idempotent).
# -------------------------------------------------------------------------------------------------
_tunnel_init() {
    # Skip if already initialized
    [[ $_tunnel_initialized -eq 1 ]] && return 0

    # Allow override for testing
    local rt_tables="${RT_TABLES_FILE:-/etc/iproute2/rt_tables}"

    # Extract valid routing tables (from rt_tables):
    # - wgcN first
    # - ovpncN next
    # - main always included at the end
    local tables_list
    tables_list="$(
        awk '$0!~/^#/ && $2 ~ /^wgc[0-9]+$/ { print $2 }' "$rt_tables" 2>/dev/null | sort
        awk '$0!~/^#/ && $2 ~ /^ovpnc[0-9]+$/ { print $2 }' "$rt_tables" 2>/dev/null | sort
        printf '%s\n' main
    )"

    # Single-line, space-separated (for matching)
    _tunnel_valid_tables="$(printf '%s\n' "$tables_list" | xargs)"

    # Precompute numeric and hex helpers from config
    _tunnel_mark_mask_val=$((TUN_DIR_MARK_MASK))
    _tunnel_mark_shift_val=$((TUN_DIR_MARK_SHIFT))
    _tunnel_mark_field_max=$((_tunnel_mark_mask_val >> _tunnel_mark_shift_val))
    _tunnel_mark_mask_hex="$(printf '0x%x' "$_tunnel_mark_mask_val")"

    _tunnel_initialized=1
}

# -------------------------------------------------------------------------------------------------
# _tunnel_table_allowed - check if routing table is valid
# -------------------------------------------------------------------------------------------------
# Returns 0 if table is valid (wgcN, ovpncN, or main), 1 otherwise.
# -------------------------------------------------------------------------------------------------
_tunnel_table_allowed() {
    local table="${1:-}"

    # Empty table is not allowed
    [[ -z $table ]] && return 1

    # Ensure initialized
    [[ $_tunnel_initialized -eq 0 ]] && _tunnel_init

    # Check if table is in valid list
    [[ " $_tunnel_valid_tables " == *" $table "* ]]
}

# -------------------------------------------------------------------------------------------------
# _tunnel_resolve_set - resolve ipset name from keys
# -------------------------------------------------------------------------------------------------
# Input: comma-separated keys (countries/custom/combos)
# Output: resolved ipset name if exists, empty string otherwise
# -------------------------------------------------------------------------------------------------
_tunnel_resolve_set() {
    local keys_csv="$1" set_name

    # Derive the set name (handles combo names like us,ca -> us_ca)
    set_name="$(_derive_set_name "${keys_csv//,/_}")"

    # Check if ipset exists
    if ipset list "$set_name" >/dev/null 2>&1; then
        printf '%s\n' "$set_name"
        return 0
    fi

    # ipset not found - return empty
    return 0
}

# -------------------------------------------------------------------------------------------------
# _tunnel_get_prerouting_base_pos - find insert position after system rules
# -------------------------------------------------------------------------------------------------
# Returns the 1-based insert position in mangle/PREROUTING immediately
# after the last system iface-mark rule.
# -------------------------------------------------------------------------------------------------
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
# Public API (defined before --source-only for testability)
###################################################################################################

# -------------------------------------------------------------------------------------------------
# tunnel_status - show tunnel director status
# -------------------------------------------------------------------------------------------------
# Displays TUN_DIR_* chains, ip rules, and fwmark configuration.
# -------------------------------------------------------------------------------------------------
tunnel_status() {
    _tunnel_init

    printf '%s\n' "=== Tunnel Director Status ==="
    printf '\n'

    # Show chains
    printf '%s\n' "--- Chains ---"
    local chains
    chains="$(
        iptables -t mangle -S 2>/dev/null |
        awk -v pre="${TUN_DIR_CHAIN_PREFIX:-TUN_DIR_}" '
            $1 == "-N" && $2 ~ ("^" pre "[0-9]+$") { print $2 }
        ' | sort -t'_' -k3,3n
    )"

    if [[ -z $chains ]]; then
        printf '%s\n' "No TUN_DIR_* chains found."
    else
        printf '%s\n' "$chains"
    fi
    printf '\n'

    # Show IP rules
    printf '%s\n' "--- IP Rules (fwmark-based) ---"
    local ip_rules
    ip_rules="$(ip rule show 2>/dev/null | grep -E "fwmark.*lookup" || true)"

    if [[ -z $ip_rules ]]; then
        printf '%s\n' "No fwmark-based ip rules found."
    else
        printf '%s\n' "$ip_rules"
    fi
    printf '\n'

    # Show fwmark configuration
    printf '%s\n' "--- Fwmark Configuration ---"
    printf 'Mask: %s\n' "$_tunnel_mark_mask_hex"
    printf 'Shift: %s\n' "$_tunnel_mark_shift_val"
    printf 'Max rules: %s\n' "$_tunnel_mark_field_max"
    printf '\n'

    return 0
}

# -------------------------------------------------------------------------------------------------
# tunnel_get_required_ipsets - return list of ipsets needed for rules
# -------------------------------------------------------------------------------------------------
# Parses TUN_DIR_TUNNELS_JSON and returns exclude country codes.
# -------------------------------------------------------------------------------------------------
tunnel_get_required_ipsets() {
    if [[ -z $TUN_DIR_TUNNELS_JSON ]] || [[ $TUN_DIR_TUNNELS_JSON == "{}" ]]; then
        return 0
    fi

    printf '%s\n' "$TUN_DIR_TUNNELS_JSON" | parse_exclude_sets_from_json
}

# -------------------------------------------------------------------------------------------------
# tunnel_stop - remove all chains and ip rules
# -------------------------------------------------------------------------------------------------
# Removes all TUN_DIR_* chains and their corresponding ip rules.
# -------------------------------------------------------------------------------------------------
tunnel_stop() {
    _tunnel_init

    log "Stopping Tunnel Director..."

    # Find all TUN_DIR_* chains in mangle
    local chains
    chains="$(
        iptables -t mangle -S 2>/dev/null |
        awk -v pre="${TUN_DIR_CHAIN_PREFIX:-TUN_DIR_}" '
            $1 == "-N" && $2 ~ ("^" pre "[0-9]+$") { print $2 }
        '
    )"

    if [[ -z $chains ]]; then
        log "No TUN_DIR_* chains found"
        return 0
    fi

    local ch idx_num pref
    local pref_base="${TUN_DIR_PREF_BASE:-16384}"
    local chain_prefix="${TUN_DIR_CHAIN_PREFIX:-TUN_DIR_}"

    while IFS= read -r ch; do
        [[ -n $ch ]] || continue

        # Extract index from chain name
        idx_num="${ch#"$chain_prefix"}"
        pref=$((pref_base + idx_num))

        # Remove PREROUTING jump
        purge_fw_rules -q "mangle PREROUTING" "-j ${ch}\$"

        # Delete chain
        delete_fw_chain -q mangle "$ch"

        # Remove ip rule
        ip rule del pref "$pref" 2>/dev/null || true

        log "Removed rule: pref=$pref chain=$ch"
    done <<< "$chains"

    log "Tunnel Director stopped"
    return 0
}

# -------------------------------------------------------------------------------------------------
# tunnel_apply - apply rules from config (idempotent)
# -------------------------------------------------------------------------------------------------
# Applies TUN_DIR_RULES configuration. Rebuilds if config hash changed or drift detected.
# -------------------------------------------------------------------------------------------------
tunnel_apply() {
    _tunnel_init

    local build_rules=0
    local changes=0
    local warnings=0

    # Create temporary file to hold normalized rule definitions
    local tun_dir_rules
    tun_dir_rules="$(tmp_file)"

    # Write rules (one per line) -> normalized version
    printf '%s\n' $TUN_DIR_RULES | awk 'NF' > "$tun_dir_rules"

    # Hash of an empty ruleset (used for baseline checks)
    local empty_rules_hash
    empty_rules_hash="$(printf '' | compute_hash)"

    # Compute current rules hash and load previous one if it exists
    local new_hash old_hash
    new_hash="$(compute_hash "$tun_dir_rules")"
    old_hash="$(cat "$TUN_DIR_HASH" 2>/dev/null || printf '%s' "$empty_rules_hash")"

    # Compare hashes first
    if [[ $new_hash != "$old_hash" ]]; then
        build_rules=1
    else
        # If hashes match, verify counts also match to detect drift
        local cfg_rows ch_rows pr_rows
        cfg_rows="$(awk 'NF { c++ } END { print c + 0 }' "$tun_dir_rules")"

        local chain_prefix="${TUN_DIR_CHAIN_PREFIX:-TUN_DIR_}"

        # Count TUN_DIR_* chains
        ch_rows="$(
            iptables -t mangle -S 2>/dev/null |
            awk -v pre="$chain_prefix" '
                $1 == "-N" && $2 ~ ("^" pre "[0-9]+$") { c++ }
                END { print c + 0 }
            '
        )"

        # Count PREROUTING jumps that target TUN_DIR_*
        pr_rows="$(
            iptables -t mangle -S PREROUTING 2>/dev/null |
            awk -v pre="$chain_prefix" '
                $1 == "-A" {
                    for (i = 1; i <= NF - 1; i++) {
                        if ($i == "-j" && $(i + 1) ~ ("^" pre "[0-9]+$")) {
                            c++; break
                        }
                    }
                }
                END { print c + 0 }
            '
        )"

        # If any count differs, force rebuild
        if [[ $cfg_rows -ne $ch_rows ]] || [[ $cfg_rows -ne $pr_rows ]]; then
            build_rules=1
        fi
    fi

    # Build from scratch on any change
    if [[ $build_rules -eq 1 ]] && [[ $old_hash != "$empty_rules_hash" ]]; then
        log "Configuration has changed; deleting existing rules..."
        tunnel_stop
        changes=1
    fi

    if [[ ! -s $tun_dir_rules ]]; then
        log "No rules are defined"
    elif [[ $build_rules -eq 0 ]]; then
        log "Rules are applied and up-to-date"
    else
        # Apply all rules in order
        local idx=0
        local base_pos
        base_pos="$(_tunnel_get_prerouting_base_pos)"

        local chain_prefix="${TUN_DIR_CHAIN_PREFIX:-TUN_DIR_}"
        local pref_base="${TUN_DIR_PREF_BASE:-16384}"
        local valid_tables_csv
        valid_tables_csv="$(printf '%s\n' $_tunnel_valid_tables | tr ' ' ',')"

        while IFS=: read -r table src src_excl set set_excl; do
            # Table must be listed in rt_tables as wgcN/ovpncN or main
            if ! _tunnel_table_allowed "$table"; then
                log -l WARN "table=$table is not supported" \
                    "(must be one of: ${valid_tables_csv:-<none>}); skipping rule with idx=$idx"
                warnings=1; idx=$((idx + 1)); continue
            fi

            # Require non-empty 'src'
            if [[ -z $src ]]; then
                log -l WARN "Missing 'src' for table=$table; skipping rule with idx=$idx"
                warnings=1; idx=$((idx + 1)); continue
            fi

            # Support optional interface suffix in the form CIDR%iface (defaults to br0)
            local src_iif="br0"
            case "$src" in
                *%*)
                    src_iif="${src#*%}"        # part after '%'
                    src="${src%%%*}"           # part before '%'
                    ;;
            esac

            # Validate interface exists
            if [[ -z $src_iif ]]; then
                log -l WARN "Empty interface after '%' in 'src'; skipping rule with idx=$idx"
                warnings=1; idx=$((idx + 1)); continue
            fi
            if [[ ! -d /sys/class/net/$src_iif ]]; then
                log -l WARN "Interface '$src_iif' not found; skipping rule with idx=$idx"
                warnings=1; idx=$((idx + 1)); continue
            fi

            # Validate src is a LAN/private CIDR (check base IP before '/')
            local src_ip="${src%%/*}"
            if ! is_lan_ip "$src_ip"; then
                log -l WARN "src '$src' is not a private RFC1918 subnet; skipping rule with idx=$idx"
                warnings=1; idx=$((idx + 1)); continue
            fi

            # 'set' must be provided (non-empty)
            if [[ -z $set ]]; then
                log -l WARN "Missing 'set' for table=$table; skipping rule with idx=$idx"
                warnings=1; idx=$((idx + 1)); continue
            fi

            # Destination matching:
            #   * Support meta ipset "any" -> match ALL destinations (no ipset lookup)
            #   * Otherwise resolve the ipset name
            local match=""
            if [[ $set != "any" ]]; then
                local dest_set
                dest_set="$(_tunnel_resolve_set "$set")"
                if [[ -z $dest_set ]]; then
                    log -l WARN "No ipset found for set='$set';" \
                        "skipping rule with idx=$idx"
                    warnings=1; idx=$((idx + 1)); continue
                fi
                match="-m set --match-set $dest_set dst"
            fi

            # Optional destination exclusion ipset
            local excl="" excl_log=""
            if [[ -n $set_excl ]]; then
                local dest_set_excl
                dest_set_excl="$(_tunnel_resolve_set "$set_excl")"
                if [[ -z $dest_set_excl ]]; then
                    log -l WARN "No exclusion ipset found for set_excl='$set_excl';" \
                        "skipping rule with idx=$idx"
                    warnings=1; idx=$((idx + 1)); continue
                fi
                excl="-m set ! --match-set $dest_set_excl dst"
                excl_log=" set_excl=$set_excl"
            fi

            # Slot is idx + 1 so 0 means "unmarked"
            local slot=$((idx + 1))

            # Ensure it fits into our reserved field
            if [[ $slot -gt $_tunnel_mark_field_max ]]; then
                log -l WARN "Too many rules for fwmark field (mask=$_tunnel_mark_mask_hex," \
                    "shift=$_tunnel_mark_shift_val): idx=$idx > max=$_tunnel_mark_field_max; skipping rule"
                warnings=1; idx=$((idx + 1)); continue
            fi

            # Compute masked mark value
            local _mark_val=$((slot << _tunnel_mark_shift_val))
            local mark_val_hex
            mark_val_hex="$(printf '0x%x' "$_mark_val")"

            # Compute routing pref and per-rule chain name
            local pref=$((pref_base + idx))
            local chain="${chain_prefix}${idx}"

            # Create/flush the per-rule chain
            create_fw_chain -q -f mangle "$chain"

            # Source exclusions: RETURN early (preserve order); validate each is private
            local src_exc_log=""
            if [[ -n $src_excl ]]; then
                local -a excl_array
                IFS=',' read -ra excl_array <<< "$src_excl"
                local ex ex_ip
                for ex in "${excl_array[@]}"; do
                    [[ -n $ex ]] || continue
                    ex_ip="${ex%%/*}"
                    if ! is_lan_ip "$ex_ip"; then
                        log -l WARN "src_excl='$ex' is not a private RFC1918 subnet;" \
                            "skipping exclusion"
                        warnings=1; continue
                    fi
                    ensure_fw_rule -q mangle "$chain" -s "$ex" -j RETURN
                done
                src_exc_log=" src_excl=$src_excl"
            fi

            # Set ONLY our field (masked), leave other apps' bits intact
            ensure_fw_rule -q mangle "$chain" $match $excl \
                -j MARK --set-xmark "$mark_val_hex/$_tunnel_mark_mask_hex"

            # Jump from PREROUTING only if our field is still zero (first-match-wins)
            # Preserve config order
            local insert_pos=$((base_pos + idx))
            sync_fw_rule -q mangle PREROUTING "-j $chain\$" \
                "-s $src -i $src_iif -m mark --mark 0x0/$_tunnel_mark_mask_hex -j $chain" "$insert_pos"

            # Unique ip rule at this pref
            ip rule del pref "$pref" 2>/dev/null || true
            ip rule add pref "$pref" fwmark "$mark_val_hex/$_tunnel_mark_mask_hex" \
                lookup "$table" >/dev/null 2>&1 || true

            log "Added rule: table=$table pref=$pref fw_mark=${mark_val_hex}/${_tunnel_mark_mask_hex}" \
                "chain=$chain iface=$src_iif src=$src${src_exc_log} set=$set${excl_log}"

            changes=1; idx=$((idx + 1))
        done < "$tun_dir_rules"
    fi

    # Save hash for the current run
    mkdir -p "$(dirname "$TUN_DIR_HASH")"
    printf '%s\n' "$new_hash" > "$TUN_DIR_HASH"

    # Final status
    if [[ $changes -eq 0 ]]; then
        return 0
    elif [[ $warnings -eq 0 ]]; then
        log "All changes have been applied successfully"
    else
        log -l WARN "Completed with warnings; please check logs for details"
    fi

    return 0
}

###################################################################################################
# Allow sourcing for testing
###################################################################################################
if [[ ${1:-} == "--source-only" ]]; then
    # shellcheck disable=SC2317
    return 0 2>/dev/null || exit 0
fi
