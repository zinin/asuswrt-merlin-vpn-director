#!/usr/bin/env bats

load '../test_helper'

load_platform() {
    export VPD_PLATFORM=merlin
    source "$LIB_DIR/platform.sh"
}

# with_mock <name> <script body> puts a one-off mock first in PATH.
with_mock() {
    mkdir -p "$BATS_TEST_TMPDIR/mock"
    printf '#!/bin/bash\n%s\n' "$2" > "$BATS_TEST_TMPDIR/mock/$1"
    chmod +x "$BATS_TEST_TMPDIR/mock/$1"
    export PATH="$BATS_TEST_TMPDIR/mock:$PATH"
}

@test "platform_name: merlin" {
    load_platform
    run platform_name
    assert_output "merlin"
}

# ---------------------------------------------------------------- WAN / IPv6

@test "platform_wan_if: interface of the primary WAN" {
    load_platform
    run platform_wan_if
    assert_success
    assert_output "eth0"
}

@test "platform_wan_if: falls back to wan0_ifname when no WAN is primary" {
    load_platform
    with_mock nvram 'case "$*" in "get wan0_ifname") echo eth9 ;; *) echo "" ;; esac'
    run platform_wan_if
    assert_success
    assert_output "eth9"
}

@test "platform_wan_if: fails and prints nothing when nvram knows no WAN" {
    load_platform
    with_mock nvram 'echo ""'
    run platform_wan_if
    assert_failure
    refute_output
}

@test "platform_ipv6_enabled: 1 when ipv6_service is set" {
    load_platform
    run platform_ipv6_enabled
    assert_output "1"
}

@test "platform_ipv6_enabled: 0 when ipv6_service is disabled or empty" {
    load_platform
    with_mock nvram 'case "$*" in "get ipv6_service") echo disabled ;; *) echo "" ;; esac'
    run platform_ipv6_enabled
    assert_output "0"
    with_mock nvram 'echo ""'
    run platform_ipv6_enabled
    assert_output "0"
}

@test "platform_lan_ifaces: br0" {
    load_platform
    run platform_lan_ifaces
    assert_output "br0"
}

# ------------------------------------------------------------------ tunnels

@test "platform_tunnels: wgc, then ovpnc from rt_tables, then main" {
    load_platform
    run platform_tunnels
    assert_success
    assert_line --index 0 "wgc1"
    assert_line --index 1 "wgc2"
    assert_line --index 2 "ovpnc1"
    assert_line --index 3 "ovpnc2"
    assert_line --index 4 "main"
}

@test "platform_tunnels: only main when rt_tables is missing" {
    load_platform
    RT_TABLES_FILE="$BATS_TEST_TMPDIR/absent" run platform_tunnels
    assert_success
    assert_output "main"
}

@test "platform_tunnel_iface: wgcN is its own interface, ovpncN is tun1N" {
    load_platform
    run platform_tunnel_iface wgc1
    assert_output "wgc1"
    run platform_tunnel_iface ovpnc3
    assert_output "tun13"
}

@test "platform_tunnel_iface: unknown ids fail" {
    load_platform
    run platform_tunnel_iface main
    assert_failure
    refute_output
    run platform_tunnel_iface eth0
    assert_failure
}

@test "platform_tunnel_info: type, connected and description on three lines" {
    load_platform
    run platform_tunnel_info wgc1
    assert_success
    assert_line --index 0 "wireguard"
    assert_line --index 1 "0"
    assert_line --index 2 "Office WG"
    run platform_tunnel_info ovpnc1
    assert_line --index 0 "openvpn"
    assert_line --index 2 "Office OVPN"
}

@test "platform_tunnel_info: connected is 1 when the interface carries UP" {
    load_platform
    with_mock ip 'echo "12: wgc1: <POINTOPOINT,NOARP,UP,LOWER_UP> mtu 1420 qdisc noqueue state UNKNOWN"'
    run platform_tunnel_info wgc1
    assert_line --index 1 "1"
}

@test "platform_tunnel_info: unknown id fails" {
    load_platform
    run platform_tunnel_info eth0
    assert_failure
    refute_output
}

@test "platform_tunnel_table: the id itself; main stays main" {
    load_platform
    run platform_tunnel_table wgc1 0
    assert_output "wgc1"
    run platform_tunnel_table main 3
    assert_output "main"
}

@test "platform_tunnel_table: an id that is not a tunnel fails" {
    load_platform
    run platform_tunnel_table eth0 0
    assert_failure
    refute_output
    run platform_tunnel_table
    assert_failure
    refute_output
}

@test "platform_tunnel_route: Merlin needs no route spec, so it always fails" {
    load_platform
    run platform_tunnel_route wgc1
    assert_failure
    refute_output
}

# _ensure has to be idempotent (tunnel.sh calls it on every up-to-date apply)
# and _release has to answer for an index that was never ensured (tunnel_stop
# walks the recorded state file unconditionally). Both hold trivially here.
@test "platform_tunnel_route_ensure and _table_release: no-ops that succeed" {
    load_platform
    : > /tmp/bats_ip_calls.log
    run platform_tunnel_route_ensure wgc1 0
    assert_success
    run platform_tunnel_route_ensure wgc1 0
    assert_success
    run platform_tunnel_table_release wgc1 0
    assert_success
    run platform_tunnel_table_release wgc9 7
    assert_success
    [ ! -s /tmp/bats_ip_calls.log ]
}

@test "platform_vpn_endpoints: non-empty vpn_clientN_addr values, one per line" {
    load_platform
    run platform_vpn_endpoints
    assert_success
    assert_line --index 0 "openvpn1.example.com"
    assert_line --index 1 "example.com"
    [ "${#lines[@]}" -eq 2 ]
}

# ------------------------------------------------------------ modules / cron

@test "platform_load_module: already loaded module needs no modprobe" {
    load_platform
    : > /tmp/bats_modprobe_calls.log
    run platform_load_module xt_TPROXY
    assert_success
    [ ! -s /tmp/bats_modprobe_calls.log ]
}

@test "platform_load_module: modprobes a module lsmod does not list" {
    load_platform
    : > /tmp/bats_modprobe_calls.log
    run platform_load_module xt_socket
    assert_success
    grep -q "modprobe xt_socket" /tmp/bats_modprobe_calls.log
}

@test "platform_load_module: fails when modprobe fails" {
    load_platform
    with_mock modprobe 'exit 1'
    run platform_load_module xt_socket
    assert_failure
}

@test "platform_cron_add / platform_cron_del: cru a and cru d" {
    load_platform
    : > /tmp/bats_cru_calls.log
    run platform_cron_add vpn_director_update "0 3 * * *" "/opt/vpn-director/vpn-director.sh update"
    assert_success
    run platform_cron_del vpn_director_update
    assert_success
    grep -qF 'cru a vpn_director_update 0 3 * * * /opt/vpn-director/vpn-director.sh update' /tmp/bats_cru_calls.log
    grep -qF 'cru d vpn_director_update' /tmp/bats_cru_calls.log
}

# ------------------------------------------------------------- firewall bits

@test "platform_tproxy_extra_rules: nothing to do on Merlin" {
    load_platform
    : > /tmp/bats_iptables_calls.log
    run platform_tproxy_extra_rules apply
    assert_success
    run platform_tproxy_extra_rules stop
    assert_success
    [ ! -s /tmp/bats_iptables_calls.log ]
}

# The contract documents the verb as apply|stop. Accepting anything would let a
# typo in a future call site read as a clean no-op here and only break on the
# platform that acts on the verb.
@test "platform_tproxy_extra_rules: rejects a verb that is not apply or stop" {
    load_platform
    run platform_tproxy_extra_rules
    assert_failure
    refute_output
    run platform_tproxy_extra_rules aply
    assert_failure
    refute_output
}

@test "platform_prerouting_base_pos: 1 when PREROUTING has no firmware mark rules" {
    load_platform
    run platform_prerouting_base_pos
    assert_output "1"
}

@test "platform_prerouting_base_pos: right after the last firmware iface-mark rule" {
    load_platform
    with_mock iptables 'cat <<EOF
-P PREROUTING ACCEPT
-A PREROUTING -i wgc1 -j MARK --set-xmark 0x1/0x7
-A PREROUTING -i tun11 -j MARK --set-xmark 0x2/0x7
-A PREROUTING -i br0 -j SOMETHING_ELSE
EOF'
    run platform_prerouting_base_pos
    assert_output "3"
}

# An iptables that cannot read the chain must not be turned into a position:
# awk's END would print 1, and the caller would insert TUN_DIR at the top of
# PREROUTING, ahead of the firmware's iface-mark rules.
@test "platform_prerouting_base_pos: fails and prints nothing when iptables fails" {
    load_platform
    with_mock iptables 'exit 3'
    run platform_prerouting_base_pos
    assert_failure
    refute_output
}

# ------------------------------------------------------------- system facts

@test "platform_password_file: /etc/shadow" {
    load_platform
    run platform_password_file
    assert_output "/etc/shadow"
}

@test "platform_lan_ip, platform_hostname, platform_model come from nvram" {
    load_platform
    run platform_lan_ip
    assert_output "192.168.50.1"
    run platform_hostname
    assert_output "RT-AX88U-1234"
    run platform_model
    assert_output "RT-AX88U"
}

@test "platform_lan_ip: fails when nvram has no lan_ipaddr" {
    load_platform
    NVRAM_NO_LAN_IP=1 run platform_lan_ip
    assert_failure
    refute_output
}

@test "platform_email_supported: 1" {
    load_platform
    run platform_email_supported
    assert_output "1"
}
