#!/usr/bin/env ash

###################################################################################################
# xray_tproxy.sh - transparent proxy routing for selected LAN clients via Xray
# -------------------------------------------------------------------------------------------------
# What this script does:
#   * Creates iptables rules to redirect traffic from selected LAN clients through Xray TPROXY
#   * Uses ipset for flexible client management and destination filtering
#   * Excludes traffic to specified countries/ipsets (e.g., Russia) from proxying
#   * Excludes traffic to Xray servers themselves (to avoid loops)
#   * Excludes local/private traffic from proxying
#   * Sets up required routing table and ip rules for TPROXY to function
#   * FAIL-SAFE: Does not apply rules if required ipsets are not ready
#
# Requirements:
#   * Xray must be running with dokodemo-door inbound in tproxy mode
#   * xt_TPROXY kernel module must be available
#   * ipset_builder.sh should have built the required country ipsets
#
# Usage:
#   xray_tproxy.sh         - apply rules (default)
#   xray_tproxy.sh stop    - remove all rules
#   xray_tproxy.sh status  - show current status
###################################################################################################

# shellcheck disable=SC2086

set -euo pipefail

###################################################################################################
# 0a. Load utils and configuration
###################################################################################################
. /jffs/scripts/vpn-director/utils/common.sh
. /jffs/scripts/vpn-director/utils/firewall.sh
. /jffs/scripts/vpn-director/utils/config.sh

acquire_lock

###################################################################################################
# 0b. Variables
###################################################################################################
ACTION="${1:-start}"
changes=0
warnings=0

###################################################################################################
# 0c. Helper functions
###################################################################################################

# Check if TPROXY module is available
check_tproxy_module() {
    if ! lsmod | grep -q xt_TPROXY; then
        modprobe xt_TPROXY 2>/dev/null || {
            log -l ERROR "xt_TPROXY module not available"
            return 1
        }
    fi
    return 0
}

# Resolve exclusion ipset name (prefer _ext variant)
resolve_exclude_set() {
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

# Check if all required ipsets exist (fail-safe)
check_required_ipsets() {
    local set_key resolved_set

    for set_key in $XRAY_EXCLUDE_SETS; do
        [ -n "$set_key" ] || continue
        if ! resolve_exclude_set "$set_key" >/dev/null; then
            log -l WARN "Required ipset '$set_key' not found; waiting for ipset_builder.sh"
            return 1
        fi
    done

    return 0
}

# Setup routing table and ip rule for TPROXY
setup_routing() {
    local rt_exists rule_exists

    # Check if route exists in our table
    rt_exists=$(ip route show table "$XRAY_ROUTE_TABLE" 2>/dev/null | grep -c "local default" || true)

    # Check if ip rule exists
    rule_exists=$(ip rule show 2>/dev/null | grep -c "fwmark $XRAY_FWMARK.*lookup $XRAY_ROUTE_TABLE" || true)

    if [ "$rt_exists" -eq 0 ]; then
        ip route add local default dev lo table "$XRAY_ROUTE_TABLE"
        log "Added route: local default dev lo table $XRAY_ROUTE_TABLE"
        changes=1
    fi

    if [ "$rule_exists" -eq 0 ]; then
        ip rule add pref "$XRAY_RULE_PREF" fwmark "$XRAY_FWMARK/$XRAY_FWMARK_MASK" table "$XRAY_ROUTE_TABLE"
        log "Added ip rule: pref $XRAY_RULE_PREF fwmark $XRAY_FWMARK/$XRAY_FWMARK_MASK table $XRAY_ROUTE_TABLE"
        changes=1
    fi
}

# Remove routing table and ip rule
teardown_routing() {
    ip rule del pref "$XRAY_RULE_PREF" 2>/dev/null || true
    ip route del local default dev lo table "$XRAY_ROUTE_TABLE" 2>/dev/null || true
    log "Removed TPROXY routing configuration"
}

# Setup clients ipset
setup_clients_ipset() {
    local ip

    # Create ipset if not exists
    if ! ipset list "$XRAY_CLIENTS_IPSET" >/dev/null 2>&1; then
        ipset create "$XRAY_CLIENTS_IPSET" hash:net
        log "Created ipset: $XRAY_CLIENTS_IPSET"
        changes=1
    fi

    # Flush and repopulate
    ipset flush "$XRAY_CLIENTS_IPSET"

    for ip in $XRAY_CLIENTS; do
        [ -n "$ip" ] || continue
        ipset add "$XRAY_CLIENTS_IPSET" "$ip" 2>/dev/null || {
            log -l WARN "Failed to add $ip to $XRAY_CLIENTS_IPSET"
        }
    done

    log "Populated $XRAY_CLIENTS_IPSET ipset"
}

# Setup servers ipset (to exclude from proxying)
setup_servers_ipset() {
    local ip

    # Create ipset if not exists
    if ! ipset list "$XRAY_SERVERS_IPSET" >/dev/null 2>&1; then
        ipset create "$XRAY_SERVERS_IPSET" hash:net
        log "Created ipset: $XRAY_SERVERS_IPSET"
        changes=1
    fi

    # Flush and repopulate
    ipset flush "$XRAY_SERVERS_IPSET"

    for ip in $XRAY_SERVERS; do
        [ -n "$ip" ] || continue
        ipset add "$XRAY_SERVERS_IPSET" "$ip" 2>/dev/null || {
            log -l WARN "Failed to add $ip to $XRAY_SERVERS_IPSET"
        }
    done

    log "Populated $XRAY_SERVERS_IPSET ipset"
}

# Build iptables rules
setup_iptables() {
    local exclude_set resolved_set

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
    for exclude_set in $XRAY_EXCLUDE_SETS; do
        [ -n "$exclude_set" ] || continue
        resolved_set="$(resolve_exclude_set "$exclude_set")" || {
            # Should not happen due to check_required_ipsets, but just in case
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

    changes=1
    log "Applied TPROXY iptables rules"
}

# Remove all iptables rules
teardown_iptables() {
    purge_fw_rules -q "mangle PREROUTING" "-j $XRAY_CHAIN\$"
    delete_fw_chain -q mangle "$XRAY_CHAIN"

    # Remove ipsets
    ipset destroy "$XRAY_CLIENTS_IPSET" 2>/dev/null || true
    ipset destroy "$XRAY_SERVERS_IPSET" 2>/dev/null || true

    log "Removed TPROXY iptables rules and ipsets"
}

# Show status
show_status() {
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

    echo "--- Clients Ipset ---"
    ipset list "$XRAY_CLIENTS_IPSET" 2>/dev/null || echo "Ipset $XRAY_CLIENTS_IPSET not found"
    echo ""

    echo "--- Servers Ipset ---"
    ipset list "$XRAY_SERVERS_IPSET" 2>/dev/null || echo "Ipset $XRAY_SERVERS_IPSET not found"
    echo ""

    echo "--- Iptables Chain ---"
    iptables -t mangle -S "$XRAY_CHAIN" 2>/dev/null || echo "Chain $XRAY_CHAIN not found"
    echo ""

    echo "--- PREROUTING Jump ---"
    iptables -t mangle -S PREROUTING 2>/dev/null | grep "$XRAY_CHAIN" || echo "No jump to $XRAY_CHAIN"
    echo ""

    echo "--- Xray Process ---"
    ps | grep -E '[x]ray' || echo "Xray not running"
}

###################################################################################################
# Main
###################################################################################################

case "$ACTION" in
    start|apply|"")
        log "Starting Xray TPROXY routing..."

        if ! check_tproxy_module; then
            exit 1
        fi

        # FAIL-SAFE: Check if required ipsets exist before proceeding
        if ! check_required_ipsets; then
            log -l WARN "Required ipsets not ready; exiting without applying rules"
            log -l WARN "Run ipset_builder.sh first, then re-run this script"
            exit 0
        fi

        setup_routing
        setup_clients_ipset
        setup_servers_ipset
        setup_iptables

        if [ "$warnings" -eq 0 ]; then
            log "Xray TPROXY routing applied successfully"
        else
            log -l WARN "Xray TPROXY routing applied with warnings"
        fi
        ;;

    stop|remove)
        log "Stopping Xray TPROXY routing..."
        teardown_iptables
        teardown_routing
        log "Xray TPROXY routing removed"
        ;;

    restart)
        "$0" stop
        sleep 1
        "$0" start
        ;;

    status)
        show_status
        ;;

    *)
        echo "Usage: $0 {start|stop|restart|status}"
        exit 1
        ;;
esac
