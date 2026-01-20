#!/usr/bin/env bash

###################################################################################################
# tproxy.sh - TPROXY routing module for VPN Director
# -------------------------------------------------------------------------------------------------
# Purpose:
#   Modular library for Xray TPROXY operations: status, apply, stop.
#   Migrated from xray_tproxy.sh to provide independent, testable functions.
#
# Dependencies:
#   - common.sh (log, tmp_file)
#   - firewall.sh (create_fw_chain, delete_fw_chain, ensure_fw_rule, sync_fw_rule, purge_fw_rules)
#   - config.sh (XRAY_* variables)
#
# Public API:
#   tproxy_status()              - show XRAY_TPROXY chain, routing, xray process
#   tproxy_apply()               - apply TPROXY rules (idempotent), soft-fail if unavailable
#   tproxy_stop()                - remove chain and routing
#   tproxy_get_required_ipsets() - return list of exclude ipsets
#
# Internal functions (for testing):
#   _tproxy_check_module()          - check if xt_TPROXY kernel module is available
#   _tproxy_resolve_exclude_set()   - resolve exclusion ipset name (prefer _ext variant)
#   _tproxy_check_required_ipsets() - fail-safe check for required ipsets
#   _tproxy_setup_routing()         - setup routing table and ip rule
#   _tproxy_teardown_routing()      - remove routing table and ip rule
#   _tproxy_setup_clients_ipset()   - setup clients ipset
#   _tproxy_setup_servers_ipset()   - setup servers ipset
#   _tproxy_setup_iptables()        - build iptables rules
#   _tproxy_teardown_iptables()     - remove iptables rules and ipsets
#   _tproxy_init()                  - initialize module state
#
# Usage:
#   source lib/tproxy.sh              # source and run main if any
#   source lib/tproxy.sh --source-only # source only for testing
###################################################################################################

# -------------------------------------------------------------------------------------------------
# Disable unneeded shellcheck warnings
# -------------------------------------------------------------------------------------------------
# shellcheck disable=SC2086

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
# Module state variables (initialized by _tproxy_init)
###################################################################################################

# Initialization flag
_tproxy_initialized=0

###################################################################################################
# Internal helper functions (defined before --source-only for testability)
###################################################################################################

# -------------------------------------------------------------------------------------------------
# _tproxy_init - initialize module state
# -------------------------------------------------------------------------------------------------
# Safe to call multiple times (idempotent).
# -------------------------------------------------------------------------------------------------
_tproxy_init() {
    # Skip if already initialized
    [[ $_tproxy_initialized -eq 1 ]] && return 0

    _tproxy_initialized=1
}

# -------------------------------------------------------------------------------------------------
# _tproxy_check_module - check if xt_TPROXY kernel module is available
# -------------------------------------------------------------------------------------------------
# Returns 0 if module is loaded or can be loaded, 1 otherwise.
# -------------------------------------------------------------------------------------------------
_tproxy_check_module() {
    if ! lsmod | grep -q xt_TPROXY; then
        modprobe xt_TPROXY 2>/dev/null || {
            log -l ERROR "xt_TPROXY module not available"
            return 1
        }
    fi
    return 0
}

# -------------------------------------------------------------------------------------------------
# _tproxy_resolve_exclude_set - resolve exclusion ipset name (prefer _ext variant)
# -------------------------------------------------------------------------------------------------
# Input: set key (e.g., "ru")
# Output: resolved ipset name (e.g., "ru_ext" or "ru")
# Returns 0 on success, 1 if ipset not found.
# -------------------------------------------------------------------------------------------------
_tproxy_resolve_exclude_set() {
    local set_key="$1"
    local ext_set="${set_key}_ext"

    # Try extended set first
    if ipset list "$ext_set" >/dev/null 2>&1; then
        printf '%s\n' "$ext_set"
        return 0
    fi

    # Fall back to standard set
    if ipset list "$set_key" >/dev/null 2>&1; then
        printf '%s\n' "$set_key"
        return 0
    fi

    return 1
}

# -------------------------------------------------------------------------------------------------
# _tproxy_check_required_ipsets - fail-safe check for required exclusion ipsets
# -------------------------------------------------------------------------------------------------
# Returns 0 if all required ipsets exist, 1 otherwise.
# -------------------------------------------------------------------------------------------------
_tproxy_check_required_ipsets() {
    local set_key
    local -a exclude_sets_array

    # Handle empty XRAY_EXCLUDE_SETS
    [[ -z ${XRAY_EXCLUDE_SETS:-} ]] && return 0

    read -ra exclude_sets_array <<< "$XRAY_EXCLUDE_SETS"

    for set_key in "${exclude_sets_array[@]}"; do
        [[ -n $set_key ]] || continue
        if ! _tproxy_resolve_exclude_set "$set_key" >/dev/null; then
            log -l WARN "Required ipset '$set_key' not found; waiting for ipset_builder.sh"
            return 1
        fi
    done

    return 0
}

# -------------------------------------------------------------------------------------------------
# _tproxy_setup_routing - setup routing table and ip rule for TPROXY
# -------------------------------------------------------------------------------------------------
_tproxy_setup_routing() {
    local rt_exists rule_exists

    # Check if route exists in our table
    rt_exists=$(ip route show table "$XRAY_ROUTE_TABLE" 2>/dev/null | grep -c "local default" || true)

    # Check if ip rule exists
    rule_exists=$(ip rule show 2>/dev/null | grep -c "fwmark $XRAY_FWMARK.*lookup $XRAY_ROUTE_TABLE" || true)

    if [[ $rt_exists -eq 0 ]]; then
        ip route add local default dev lo table "$XRAY_ROUTE_TABLE"
        log "Added route: local default dev lo table $XRAY_ROUTE_TABLE"
    fi

    if [[ $rule_exists -eq 0 ]]; then
        ip rule add pref "$XRAY_RULE_PREF" fwmark "$XRAY_FWMARK/$XRAY_FWMARK_MASK" table "$XRAY_ROUTE_TABLE"
        log "Added ip rule: pref $XRAY_RULE_PREF fwmark $XRAY_FWMARK/$XRAY_FWMARK_MASK table $XRAY_ROUTE_TABLE"
    fi
}

# -------------------------------------------------------------------------------------------------
# _tproxy_teardown_routing - remove routing table and ip rule
# -------------------------------------------------------------------------------------------------
_tproxy_teardown_routing() {
    ip rule del pref "$XRAY_RULE_PREF" 2>/dev/null || true
    ip route del local default dev lo table "$XRAY_ROUTE_TABLE" 2>/dev/null || true
    log "Removed TPROXY routing configuration"
}

# -------------------------------------------------------------------------------------------------
# _tproxy_setup_clients_ipset - setup clients ipset
# -------------------------------------------------------------------------------------------------
# Always creates the ipset (even if empty) so iptables rules can reference it.
# -------------------------------------------------------------------------------------------------
_tproxy_setup_clients_ipset() {
    local ip
    local -a clients_array=()

    # Create ipset if not exists
    if ! ipset list "$XRAY_CLIENTS_IPSET" >/dev/null 2>&1; then
        ipset create "$XRAY_CLIENTS_IPSET" hash:net
        log "Created ipset: $XRAY_CLIENTS_IPSET"
    fi

    # Flush and repopulate
    ipset flush "$XRAY_CLIENTS_IPSET"

    # Handle empty XRAY_CLIENTS gracefully
    if [[ -n ${XRAY_CLIENTS:-} ]]; then
        read -ra clients_array <<< "$XRAY_CLIENTS"
        for ip in "${clients_array[@]}"; do
            [[ -n $ip ]] || continue
            ipset add "$XRAY_CLIENTS_IPSET" "$ip" 2>/dev/null || {
                log -l WARN "Failed to add $ip to $XRAY_CLIENTS_IPSET"
            }
        done
    fi

    log "Populated $XRAY_CLIENTS_IPSET ipset (${#clients_array[@]} entries)"
}

# -------------------------------------------------------------------------------------------------
# _tproxy_setup_servers_ipset - setup servers ipset (to exclude from proxying)
# -------------------------------------------------------------------------------------------------
# Always creates the ipset (even if empty) so iptables rules can reference it.
# -------------------------------------------------------------------------------------------------
_tproxy_setup_servers_ipset() {
    local ip
    local -a servers_array=()

    # Create ipset if not exists
    if ! ipset list "$XRAY_SERVERS_IPSET" >/dev/null 2>&1; then
        ipset create "$XRAY_SERVERS_IPSET" hash:net
        log "Created ipset: $XRAY_SERVERS_IPSET"
    fi

    # Flush and repopulate
    ipset flush "$XRAY_SERVERS_IPSET"

    # Handle empty XRAY_SERVERS gracefully
    if [[ -n ${XRAY_SERVERS:-} ]]; then
        read -ra servers_array <<< "$XRAY_SERVERS"
        for ip in "${servers_array[@]}"; do
            [[ -n $ip ]] || continue
            ipset add "$XRAY_SERVERS_IPSET" "$ip" 2>/dev/null || {
                log -l WARN "Failed to add $ip to $XRAY_SERVERS_IPSET"
            }
        done
    fi

    log "Populated $XRAY_SERVERS_IPSET ipset (${#servers_array[@]} entries)"
}

# -------------------------------------------------------------------------------------------------
# _tproxy_setup_iptables - build iptables rules
# -------------------------------------------------------------------------------------------------
_tproxy_setup_iptables() {
    local exclude_set resolved_set
    local -a exclude_sets_array

    # Handle empty XRAY_EXCLUDE_SETS
    if [[ -n ${XRAY_EXCLUDE_SETS:-} ]]; then
        read -ra exclude_sets_array <<< "$XRAY_EXCLUDE_SETS"
    else
        exclude_sets_array=()
    fi

    # Create chain
    create_fw_chain -f mangle "$XRAY_CHAIN"

    # Rule 1: Skip if source is not in our clients ipset
    ensure_fw_rule -q mangle "$XRAY_CHAIN" \
        -m set ! --match-set "$XRAY_CLIENTS_IPSET" src -j RETURN

    # Rule 2: Skip traffic to Xray servers (avoid loops)
    ensure_fw_rule -q mangle "$XRAY_CHAIN" \
        -m set --match-set "$XRAY_SERVERS_IPSET" dst -j RETURN

    # Rule 3: Skip local destinations (loopback)
    ensure_fw_rule -q mangle "$XRAY_CHAIN" \
        -d 127.0.0.0/8 -j RETURN

    # Rule 4: Skip private network destinations (RFC1918)
    ensure_fw_rule -q mangle "$XRAY_CHAIN" \
        -d 10.0.0.0/8 -j RETURN
    ensure_fw_rule -q mangle "$XRAY_CHAIN" \
        -d 172.16.0.0/12 -j RETURN
    ensure_fw_rule -q mangle "$XRAY_CHAIN" \
        -d 192.168.0.0/16 -j RETURN

    # Rule 5: Skip link-local
    ensure_fw_rule -q mangle "$XRAY_CHAIN" \
        -d 169.254.0.0/16 -j RETURN

    # Rule 6: Skip multicast
    ensure_fw_rule -q mangle "$XRAY_CHAIN" \
        -d 224.0.0.0/4 -j RETURN

    # Rule 7: Skip broadcast
    ensure_fw_rule -q mangle "$XRAY_CHAIN" \
        -d 255.255.255.255/32 -j RETURN

    # Rule 8: Skip excluded country/custom ipsets
    for exclude_set in "${exclude_sets_array[@]}"; do
        [[ -n $exclude_set ]] || continue
        resolved_set="$(_tproxy_resolve_exclude_set "$exclude_set")" || {
            # Should not happen due to _tproxy_check_required_ipsets, but just in case
            log -l ERROR "Exclusion ipset '$exclude_set' not found; aborting"
            return 1
        }
        ensure_fw_rule -q mangle "$XRAY_CHAIN" \
            -m set --match-set "$resolved_set" dst -j RETURN
        log "Added exclusion for ipset: $resolved_set"
    done

    # Rule 9: Apply TPROXY for remaining traffic
    # TCP
    ensure_fw_rule -q mangle "$XRAY_CHAIN" \
        -p tcp -j TPROXY --on-port "$XRAY_TPROXY_PORT" \
        --tproxy-mark "$XRAY_FWMARK/$XRAY_FWMARK_MASK"

    # UDP
    ensure_fw_rule -q mangle "$XRAY_CHAIN" \
        -p udp -j TPROXY --on-port "$XRAY_TPROXY_PORT" \
        --tproxy-mark "$XRAY_FWMARK/$XRAY_FWMARK_MASK"

    # Jump from PREROUTING to our chain (position 1 = before Tunnel Director)
    sync_fw_rule -q mangle PREROUTING "-j $XRAY_CHAIN\$" \
        "-i br0 -j $XRAY_CHAIN" 1

    log "Applied TPROXY iptables rules"
}

# -------------------------------------------------------------------------------------------------
# _tproxy_teardown_iptables - remove all iptables rules
# -------------------------------------------------------------------------------------------------
_tproxy_teardown_iptables() {
    purge_fw_rules -q "mangle PREROUTING" "-j $XRAY_CHAIN\$"
    delete_fw_chain -q mangle "$XRAY_CHAIN"

    # Remove ipsets
    ipset destroy "$XRAY_CLIENTS_IPSET" 2>/dev/null || true
    ipset destroy "$XRAY_SERVERS_IPSET" 2>/dev/null || true

    log "Removed TPROXY iptables rules and ipsets"
}

###################################################################################################
# Public API (defined before --source-only for testability)
###################################################################################################

# -------------------------------------------------------------------------------------------------
# tproxy_status - show TPROXY status
# -------------------------------------------------------------------------------------------------
# Displays kernel module, routing, chains, ipsets, and xray process.
# -------------------------------------------------------------------------------------------------
tproxy_status() {
    _tproxy_init

    printf '%s\n' "=== Xray TPROXY Status ==="
    printf '\n'

    printf '%s\n' "--- Kernel Module ---"
    lsmod | grep -E 'xt_TPROXY|nf_tproxy' || printf '%s\n' "TPROXY module not loaded"
    printf '\n'

    printf '%s\n' "--- Routing ---"
    printf 'Table %s:\n' "$XRAY_ROUTE_TABLE"
    ip route show table "$XRAY_ROUTE_TABLE" 2>/dev/null || printf '%s\n' "  (empty)"
    printf '\n'
    printf '%s\n' "IP Rules:"
    ip rule show | grep -E "$XRAY_ROUTE_TABLE|$XRAY_FWMARK" || printf '%s\n' "  (none)"
    printf '\n'

    printf '%s\n' "--- Clients Ipset ---"
    ipset list "$XRAY_CLIENTS_IPSET" 2>/dev/null || printf 'Ipset %s not found\n' "$XRAY_CLIENTS_IPSET"
    printf '\n'

    printf '%s\n' "--- Servers Ipset ---"
    ipset list "$XRAY_SERVERS_IPSET" 2>/dev/null || printf 'Ipset %s not found\n' "$XRAY_SERVERS_IPSET"
    printf '\n'

    printf '%s\n' "--- Iptables Chain ---"
    iptables -t mangle -S "$XRAY_CHAIN" 2>/dev/null || printf 'Chain %s not found\n' "$XRAY_CHAIN"
    printf '\n'

    printf '%s\n' "--- PREROUTING Jump ---"
    iptables -t mangle -S PREROUTING 2>/dev/null | grep "$XRAY_CHAIN" || printf 'No jump to %s\n' "$XRAY_CHAIN"
    printf '\n'

    printf '%s\n' "--- Xray Process ---"
    pgrep -la xray || printf '%s\n' "Xray not running"

    return 0
}

# -------------------------------------------------------------------------------------------------
# tproxy_get_required_ipsets - return list of exclude ipsets
# -------------------------------------------------------------------------------------------------
# Parses XRAY_EXCLUDE_SETS and returns them, one per line.
# -------------------------------------------------------------------------------------------------
tproxy_get_required_ipsets() {
    local set_key
    local -a exclude_sets_array

    # Handle empty XRAY_EXCLUDE_SETS
    [[ -z ${XRAY_EXCLUDE_SETS:-} ]] && return 0

    read -ra exclude_sets_array <<< "$XRAY_EXCLUDE_SETS"

    for set_key in "${exclude_sets_array[@]}"; do
        [[ -n $set_key ]] || continue
        printf '%s\n' "$set_key"
    done
}

# -------------------------------------------------------------------------------------------------
# tproxy_stop - remove chain and routing
# -------------------------------------------------------------------------------------------------
# Removes all TPROXY iptables rules and routing configuration.
# -------------------------------------------------------------------------------------------------
tproxy_stop() {
    _tproxy_init

    log "Stopping Xray TPROXY routing..."
    _tproxy_teardown_iptables
    _tproxy_teardown_routing
    log "Xray TPROXY routing removed"

    return 0
}

# -------------------------------------------------------------------------------------------------
# tproxy_apply - apply TPROXY rules (idempotent)
# -------------------------------------------------------------------------------------------------
# Applies TPROXY rules. Soft-fails if xt_TPROXY unavailable or ipsets missing.
# Returns 0 even on soft-fail to allow caller scripts to continue.
# -------------------------------------------------------------------------------------------------
tproxy_apply() {
    _tproxy_init

    log "Starting Xray TPROXY routing..."

    # Soft-fail: if xt_TPROXY module not available, return 0 without applying
    if ! _tproxy_check_module; then
        log -l WARN "xt_TPROXY module not available; skipping TPROXY setup"
        return 0
    fi

    # Soft-fail: if required ipsets not ready, return 0 without applying
    if ! _tproxy_check_required_ipsets; then
        log -l WARN "Required ipsets not ready; exiting without applying rules"
        log -l WARN "Run ipset_builder.sh first, then re-run this script"
        return 0
    fi

    _tproxy_setup_routing
    _tproxy_setup_clients_ipset
    _tproxy_setup_servers_ipset

    # Soft-fail if iptables setup fails
    if ! _tproxy_setup_iptables; then
        log -l WARN "Failed to setup iptables rules; TPROXY may not be active"
        return 0
    fi

    log "Xray TPROXY routing applied successfully"

    return 0
}

###################################################################################################
# Allow sourcing for testing
###################################################################################################
if [[ ${1:-} == "--source-only" ]]; then
    # shellcheck disable=SC2317
    return 0 2>/dev/null || exit 0
fi
