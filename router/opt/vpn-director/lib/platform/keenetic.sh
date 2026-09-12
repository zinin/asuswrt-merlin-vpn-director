#!/usr/bin/env bash

###################################################################################################
# platform/keenetic.sh - KeeneticOS implementation of the platform contract
# -------------------------------------------------------------------------------------------------
# See lib/platform.sh for the contract. Facts come from NDM's RCI over HTTP
# (http://localhost:79/rci/<path>, JSON), from ip-full and from the Entware
# cron. Tunnel routing tables are this module's own (KEENETIC_TABLE_BASE + idx):
# the firmware keeps none for its VPN client interfaces.
#
# Test seams: VPD_MODULES_DIR, VPD_PROC_MODULES, VPD_CRON_D, VPD_CRON_INIT.
###################################################################################################

platform_name() {
    printf 'keenetic\n'
}

# -------------------------------------------------------------------------------------------------
# _rci_get <path> - print the JSON body of http://localhost:79/rci/<path>
# -------------------------------------------------------------------------------------------------
# NDM answers an unknown path with 404 and an absent object with an empty
# body; both are "no answer" here (rc 1, nothing printed). Callers parse the
# body with jq -r and never use jq's regex functions (test, match, sub): the
# Entware jq on KeeneticOS lacks them.
_rci_get() {
    local body
    body="$(curl -sf --max-time 5 "http://localhost:79/rci/${1:-}" 2>/dev/null)" || return 1
    [[ -n $body ]] || return 1
    printf '%s\n' "$body"
}

# The WAN is whatever carries the IPv4 default route; the firmware has no one
# variable for it. "ip -4 route show default" lists the routes in metric
# order, so the first line is the active one.
platform_wan_if() {
    local dev
    dev="$(ip -4 route show default 2>/dev/null |
        awk '{ for (i = 1; i < NF; i++) if ($i == "dev") { print $(i + 1); exit } }')"
    [[ -n $dev ]] || return 1
    printf '%s\n' "$dev"
}

platform_ipv6_enabled() {
    if ip -6 route show default 2>/dev/null | grep -q .; then
        printf '1\n'
    else
        printf '0\n'
    fi
}

# Bridge0 (Home). Bridge1 (Guest) and other segments are out of scope (spec 16).
platform_lan_ifaces() {
    printf 'br0\n'
}

platform_lan_ip() {
    local ip
    ip="$(ip -4 -o addr show br0 2>/dev/null | awk '{ print $4; exit }')"
    ip="${ip%%/*}"
    [[ -n $ip ]] || return 1
    printf '%s\n' "$ip"
}

platform_hostname() {
    local json name
    json="$(_rci_get show/system)" || return 1
    name="$(printf '%s' "$json" | jq -r '.hostname // empty' 2>/dev/null)"
    [[ -n $name ]] || return 1
    printf '%s\n' "$name"
}

platform_model() {
    local json model
    json="$(_rci_get show/version)" || return 1
    model="$(printf '%s' "$json" | jq -r '.model // empty' 2>/dev/null)"
    [[ -n $model ]] || return 1
    printf '%s\n' "$model"
}

# No /etc/shadow on KeeneticOS. Entware's root password is field 2 of
# /opt/etc/passwd (MD5-crypt), which auth.ShadowAuth reads the same way.
platform_password_file() {
    printf '/opt/etc/passwd\n'
}

# amtm email is a Merlin feature.
platform_email_supported() {
    printf '0\n'
}
