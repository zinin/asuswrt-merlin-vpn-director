#!/usr/bin/env bash

###################################################################################################
# tunnel.sh - tunnel director module for VPN Director
# -------------------------------------------------------------------------------------------------
# Purpose:
#   Modular library for tunnel director operations: status, apply, stop.
#   Routes LAN client traffic through VPN tunnels with exclusion-based routing.
#
# Dependencies:
#   - common.sh (log, tmp_file, compute_hash, is_lan_ip)
#   - firewall.sh (create_fw_chain, delete_fw_chain, ensure_fw_rule, sync_fw_rule,
#                  purge_fw_rules, fw_chain_exists)
#   - config.sh (TUN_DIR_TUNNELS_JSON, TUN_DIR_CHAIN, TUN_DIR_PREF_BASE,
#                TUN_DIR_MARK_MASK, TUN_DIR_MARK_SHIFT)
#   - ipset.sh (_ipset_exists, parse_exclude_sets_from_json, TUN_DIR_HASH)
#
# Public API:
#   tunnel_status()              - show TUN_DIR chain, ip rules, configured tunnels
#   tunnel_apply()               - apply rules from config (idempotent)
#   tunnel_stop()                - remove chain and ip rules
#   tunnel_get_required_ipsets() - return list of ipsets needed for rules
#
# Internal functions (for testing):
#   _tunnel_table_allowed()         - check if routing table is valid (wgcN, ovpncN, main)
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
# Displays TUN_DIR chain rules, ip rules, and configured tunnels.
# -------------------------------------------------------------------------------------------------
tunnel_status() {
    _tunnel_init

    printf '%s\n' "=== Tunnel Director Status ==="
    printf '\n'

    # Show chain rules
    printf '%s\n' "--- Chain: $TUN_DIR_CHAIN ---"
    if fw_chain_exists mangle "$TUN_DIR_CHAIN"; then
        iptables -t mangle -S "$TUN_DIR_CHAIN" 2>/dev/null | tail -n +2
    else
        printf '%s\n' "Chain not found (not applied)"
    fi
    printf '\n'

    # Show IP rules
    printf '%s\n' "--- IP Rules (fwmark-based) ---"
    local ip_rules
    ip_rules=$(ip rule show 2>/dev/null | grep -E "fwmark.*lookup" || true)

    if [[ -z $ip_rules ]]; then
        printf '%s\n' "No fwmark-based ip rules found."
    else
        printf '%s\n' "$ip_rules"
    fi
    printf '\n'

    # Show configured tunnels
    printf '%s\n' "--- Configured Tunnels ---"
    if [[ -z $TUN_DIR_TUNNELS_JSON ]] || [[ $TUN_DIR_TUNNELS_JSON == "{}" ]]; then
        printf '%s\n' "No tunnels configured."
    else
        printf '%s\n' "$TUN_DIR_TUNNELS_JSON" | jq -r '
            to_entries[] |
            "Tunnel: \(.key)\n  Clients: \(.value.clients // [] | join(", "))\n  Exclude: \(.value.exclude // [] | join(", "))"
        ' 2>/dev/null || printf '%s\n' "Error: Failed to parse tunnels JSON"
    fi
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
# tunnel_stop - remove chain and ip rules
# -------------------------------------------------------------------------------------------------
# Removes TUN_DIR chain and all associated ip rules.
# -------------------------------------------------------------------------------------------------
tunnel_stop() {
    _tunnel_init

    log "Stopping Tunnel Director..."

    # Remove PREROUTING jump
    purge_fw_rules -q "mangle PREROUTING" "-j ${TUN_DIR_CHAIN}\$"

    # Delete chain if exists
    if fw_chain_exists mangle "$TUN_DIR_CHAIN"; then
        delete_fw_chain -q mangle "$TUN_DIR_CHAIN"
        log "Removed chain: $TUN_DIR_CHAIN"
    fi

    # Remove ip rules in our pref range
    local pref_base="${TUN_DIR_PREF_BASE:-16384}"
    local max_rules="${_tunnel_mark_field_max:-255}"
    local i

    for ((i = 0; i < max_rules; i++)); do
        ip rule del pref $((pref_base + i)) 2>/dev/null || true
    done

    # Clear hash file
    rm -f "$TUN_DIR_HASH"

    log "Tunnel Director stopped"
    return 0
}

# -------------------------------------------------------------------------------------------------
# tunnel_apply - apply rules from config (idempotent)
# -------------------------------------------------------------------------------------------------
# Applies TUN_DIR_TUNNELS_JSON configuration. Single chain, exclusion-based routing.
# -------------------------------------------------------------------------------------------------
tunnel_apply() {
    _tunnel_init

    local changes=0
    local warnings=0

    # Check if tunnels config is empty
    if [[ -z $TUN_DIR_TUNNELS_JSON ]] || [[ $TUN_DIR_TUNNELS_JSON == "{}" ]]; then
        log "No tunnels configured"
        return 0
    fi

    # Validate JSON before modifying firewall state
    if ! printf '%s\n' "$TUN_DIR_TUNNELS_JSON" | jq -e 'type == "object"' >/dev/null 2>&1; then
        log -l ERROR "Invalid tunnels JSON configuration"
        return 1
    fi

    # Compute config hash for change detection
    local new_hash old_hash empty_hash
    new_hash=$(printf '%s' "$TUN_DIR_TUNNELS_JSON" | compute_hash)
    empty_hash=$(printf '' | compute_hash)
    old_hash=$(cat "$TUN_DIR_HASH" 2>/dev/null || printf '%s' "$empty_hash")

    # Check if rebuild needed
    local rebuild=0
    if [[ $new_hash != "$old_hash" ]]; then
        rebuild=1
    elif ! fw_chain_exists mangle "$TUN_DIR_CHAIN"; then
        rebuild=1
    fi

    if [[ $rebuild -eq 0 ]]; then
        log "Rules are applied and up-to-date"
        return 0
    fi

    # Stop existing rules if config changed
    if [[ $old_hash != "$empty_hash" ]]; then
        log "Configuration changed; removing existing rules..."
        tunnel_stop
        changes=1
    fi

    # Create the single chain
    create_fw_chain -q -f mangle "$TUN_DIR_CHAIN"

    # Get base position in PREROUTING
    local base_pos
    base_pos=$(_tunnel_get_prerouting_base_pos)

    # Process each tunnel
    local tunnel_idx=0
    local tunnels
    tunnels=$(printf '%s\n' "$TUN_DIR_TUNNELS_JSON" | jq -r 'keys[]')

    while IFS= read -r tunnel; do
        [[ -n $tunnel ]] || continue

        # Validate tunnel exists in rt_tables
        if ! _tunnel_table_allowed "$tunnel"; then
            log -l WARN "Tunnel '$tunnel' not in rt_tables; skipping"
            warnings=1
            continue
        fi

        # Validate tunnel config is an object (not string or other type)
        local tunnel_type
        tunnel_type=$(printf '%s\n' "$TUN_DIR_TUNNELS_JSON" | jq -r --arg t "$tunnel" '.[$t] | type')
        if [[ $tunnel_type != "object" ]]; then
            log -l WARN "Tunnel '$tunnel' has invalid config (expected object, got $tunnel_type); skipping"
            warnings=1
            continue
        fi

        # Validate clients is an array
        local clients_type
        clients_type=$(printf '%s\n' "$TUN_DIR_TUNNELS_JSON" | jq -r --arg t "$tunnel" '.[$t].clients | type')
        if [[ $clients_type != "array" ]] && [[ $clients_type != "null" ]]; then
            log -l WARN "Tunnel '$tunnel' has invalid clients (expected array, got $clients_type); skipping"
            warnings=1
            continue
        fi

        # Validate exclude is an array (if present)
        local exclude_type
        exclude_type=$(printf '%s\n' "$TUN_DIR_TUNNELS_JSON" | jq -r --arg t "$tunnel" '.[$t].exclude | type')
        if [[ $exclude_type != "array" ]] && [[ $exclude_type != "null" ]]; then
            log -l WARN "Tunnel '$tunnel' has invalid exclude (expected array, got $exclude_type); skipping exclusions"
            warnings=1
            exclude_type="null"  # Skip excludes but continue with clients
        fi

        # Get clients and excludes for this tunnel
        local clients excludes
        clients=$(printf '%s\n' "$TUN_DIR_TUNNELS_JSON" | jq -r --arg t "$tunnel" '.[$t].clients // [] | .[]')
        if [[ $exclude_type == "array" ]]; then
            excludes=$(printf '%s\n' "$TUN_DIR_TUNNELS_JSON" | jq -r --arg t "$tunnel" '.[$t].exclude // [] | .[]')
        else
            excludes=""
        fi

        if [[ -z $clients ]]; then
            log -l WARN "Tunnel '$tunnel' has no clients; skipping"
            warnings=1
            continue
        fi

        # Compute fwmark for this tunnel
        local slot=$((tunnel_idx + 1))
        if [[ $slot -gt $_tunnel_mark_field_max ]]; then
            log -l WARN "Too many tunnels (max $_tunnel_mark_field_max); skipping '$tunnel'"
            warnings=1
            continue
        fi

        local mark_val=$(( slot << _tunnel_mark_shift_val ))
        local mark_hex
        mark_hex=$(printf '0x%x' "$mark_val")

        # Add rules for each client
        while IFS= read -r client; do
            [[ -n $client ]] || continue

            # Validate client is RFC1918
            local client_ip="${client%%/*}"
            if ! is_lan_ip "$client_ip"; then
                log -l WARN "Client '$client' is not RFC1918; skipping"
                warnings=1
                continue
            fi

            # Add RETURN rules for each exclude ipset
            while IFS= read -r excl; do
                [[ -n $excl ]] || continue
                local excl_set
                excl_set=$(printf '%s' "$excl" | tr 'A-Z' 'a-z')

                if ! _ipset_exists "$excl_set"; then
                    log -l WARN "Exclude ipset '$excl_set' not found; skipping exclusion"
                    warnings=1
                    continue
                fi

                ensure_fw_rule -q mangle "$TUN_DIR_CHAIN" \
                    -s "$client" -m set --match-set "$excl_set" dst -j RETURN
            done <<< "$excludes"

            # Add MARK rule for this client (first-match: only if not already marked)
            ensure_fw_rule -q mangle "$TUN_DIR_CHAIN" \
                -s "$client" -m mark --mark "0x0/$_tunnel_mark_mask_hex" \
                -j MARK --set-xmark "$mark_hex/$_tunnel_mark_mask_hex"

            log "Added: client=$client tunnel=$tunnel mark=$mark_hex"
            changes=1
        done <<< "$clients"

        # Add ip rule for this tunnel
        local pref=$((TUN_DIR_PREF_BASE + tunnel_idx))
        ip rule del pref "$pref" 2>/dev/null || true
        ip rule add pref "$pref" fwmark "$mark_hex/$_tunnel_mark_mask_hex" lookup "$tunnel" 2>/dev/null || true

        tunnel_idx=$((tunnel_idx + 1))
    done <<< "$tunnels"

    # Add jump from PREROUTING to TUN_DIR chain (filter LAN traffic via br0)
    sync_fw_rule -q mangle PREROUTING "-j ${TUN_DIR_CHAIN}\$" \
        "-i br0 -m mark --mark 0x0/$_tunnel_mark_mask_hex -j $TUN_DIR_CHAIN" "$base_pos"

    # Save hash
    mkdir -p "$(dirname "$TUN_DIR_HASH")"
    printf '%s\n' "$new_hash" > "$TUN_DIR_HASH"

    if [[ $changes -eq 0 ]]; then
        log "No changes applied"
    elif [[ $warnings -eq 0 ]]; then
        log "All rules applied successfully"
    else
        log -l WARN "Completed with warnings"
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
