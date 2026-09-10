#!/usr/bin/env bash

###################################################################################################
# platform/merlin.sh - Asuswrt-Merlin implementation of the platform contract
# -------------------------------------------------------------------------------------------------
# See lib/platform.sh for the contract. Facts come from nvram, the firmware's
# named routing tables (wgcN, ovpncN in /etc/iproute2/rt_tables) and cru.
###################################################################################################

platform_name() {
    printf 'merlin\n'
}

# The firmware flags one WAN slot as primary; its interface is the active one.
platform_wan_if() {
    local idx name
    for idx in 0 1 2; do
        if [[ "$(nvram get "wan${idx}_primary" 2>/dev/null || true)" == "1" ]]; then
            name="$(nvram get "wan${idx}_ifname" 2>/dev/null || true)"
            [[ -n $name ]] || return 1
            printf '%s\n' "$name"
            return 0
        fi
    done
    name="$(nvram get wan0_ifname 2>/dev/null || true)"
    [[ -n $name ]] || return 1
    printf '%s\n' "$name"
}

platform_ipv6_enabled() {
    local s
    s="$(nvram get ipv6_service 2>/dev/null || true)"
    if [[ -n $s ]] && [[ $s != "disabled" ]]; then
        printf '1\n'
    else
        printf '0\n'
    fi
}

platform_lan_ifaces() {
    printf 'br0\n'
}

# wgcN first, ovpncN next, main always last. RT_TABLES_FILE overrides the path
# for tests.
platform_tunnels() {
    local rt_tables="${RT_TABLES_FILE:-/etc/iproute2/rt_tables}"
    { awk '$0!~/^#/ && $2 ~ /^wgc[0-9]+$/ { print $2 }' "$rt_tables" 2>/dev/null | sort; } || true
    { awk '$0!~/^#/ && $2 ~ /^ovpnc[0-9]+$/ { print $2 }' "$rt_tables" 2>/dev/null | sort; } || true
    printf '%s\n' main
}

# OpenVPN client N runs on tun1N; WireGuard clients are named after their table.
platform_tunnel_iface() {
    case "${1:-}" in
        wgc[0-9]*)   printf '%s\n' "$1" ;;
        ovpnc[0-9]*) printf 'tun1%s\n' "${1#ovpnc}" ;;
        *)           return 1 ;;
    esac
}

platform_tunnel_info() {
    local id="${1:-}" type desc="" iface connected=0
    case "$id" in
        wgc[0-9]*)
            type=wireguard
            desc="$(nvram get "${id}_desc" 2>/dev/null || true)"
            ;;
        ovpnc[0-9]*)
            type=openvpn
            desc="$(nvram get "vpn_client${id#ovpnc}_desc" 2>/dev/null || true)"
            ;;
        *) return 1 ;;
    esac
    iface="$(platform_tunnel_iface "$id")"
    # "<POINTOPOINT,NOARP,UP,LOWER_UP>": UP as a whole flag, not the tail of LOWER_UP.
    # No \b: busybox grep on the routers has no word boundaries.
    if ip -o link show "$iface" 2>/dev/null | grep -qE '<([^>]*,)?UP[,>]'; then
        connected=1
    fi
    printf '%s\n%s\n%s\n' "$type" "$connected" "$desc"
}

# The firmware keeps a routing table per tunnel under the tunnel's own name, so
# the mapping is the identity for every id platform_tunnels lists - "main"
# included, which is why this cannot just defer to platform_tunnel_iface (that
# one has no interface to name for "main"). Anything that is not a tunnel id at
# all gets the contract's general answer: nothing on stdout, rc 1.
platform_tunnel_table() {
    case "${1:-}" in
        wgc[0-9]*|ovpnc[0-9]*|main) printf '%s\n' "$1" ;;
        *)                          return 1 ;;
    esac
}

# The firmware owns the tunnel tables, so there is no route spec to print and
# nothing to install or release. Both _ensure and _release still have to answer
# for any call the core makes: _ensure on every apply of an unchanged config,
# _release for a recorded index whose route was never installed.
platform_tunnel_route() {
    return 1
}

platform_tunnel_route_ensure() {
    return 0
}

platform_tunnel_table_release() {
    return 0
}

platform_vpn_endpoints() {
    local slot addr
    for slot in 1 2 3 4 5; do
        addr="$(nvram get "vpn_client${slot}_addr" 2>/dev/null || true)"
        [[ -n $addr ]] || continue
        printf '%s\n' "$addr"
    done
}

platform_load_module() {
    local name="${1:-}"
    [[ -n $name ]] || return 1
    if lsmod 2>/dev/null | grep -q "$name"; then
        return 0
    fi
    modprobe "$name" 2>/dev/null
}

platform_cron_add() {
    local name="$1" schedule="$2" cmd="$3"
    cru a "$name" "$schedule $cmd"
}

platform_cron_del() {
    cru d "$1"
}

# Nothing to add outside our own chain on Merlin. The verb is still checked:
# accepting anything would let a typo in a future call site pass as a clean
# no-op on this platform and only surface on the one that acts on it.
platform_tproxy_extra_rules() {
    case "${1:-}" in
        apply|stop) return 0 ;;
        *)          return 1 ;;
    esac
}

# Position right after the firmware's own iface-mark rules (-i wgcN / -i tunN
# -j MARK --set-...), so Tunnel Director never precedes them.
#
# The chain is read into a variable first, and an iptables that cannot answer
# fails the whole function. Piping iptables straight into awk would let awk's
# END print a position for a chain nobody read: rc 0 and "1" without pipefail,
# so the caller would insert TUN_DIR ahead of the firmware's own rules.
platform_prerouting_base_pos() {
    local raw
    raw="$(iptables -t mangle -S PREROUTING 2>/dev/null)" || return 1
    printf '%s\n' "$raw" |
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

platform_password_file() {
    printf '/etc/shadow\n'
}

platform_lan_ip() {
    local ip
    ip="$(nvram get lan_ipaddr 2>/dev/null || true)"
    [[ -n $ip ]] || return 1
    printf '%s\n' "$ip"
}

platform_hostname() {
    local name
    name="$(nvram get lan_hostname 2>/dev/null || true)"
    [[ -n $name ]] || return 1
    printf '%s\n' "$name"
}

platform_model() {
    local model
    model="$(nvram get model 2>/dev/null || true)"
    [[ -n $model ]] || return 1
    printf '%s\n' "$model"
}

# amtm email is a Merlin feature; whether it is configured stays send-email.sh's check.
platform_email_supported() {
    printf '1\n'
}
