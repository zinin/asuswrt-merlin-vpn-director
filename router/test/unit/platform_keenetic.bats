#!/usr/bin/env bats

load '../test_helper'

# The Keenetic column of the contract. RCI answers come from the curl mock in
# mocks/keenetic (fixtures/keenetic/rci/<path>.json), kernel facts from the ip,
# insmod and iptables mocks, module state from a /proc/modules fixture.
load_platform() {
    export VPD_PLATFORM=keenetic
    export PATH="$TEST_ROOT/mocks/keenetic:$PATH"
    export VPD_PROC_MODULES="$TEST_ROOT/fixtures/keenetic/proc_modules"
    export VPD_MODULES_DIR="$BATS_TEST_TMPDIR/modules"
    export VPD_CRON_D="$BATS_TEST_TMPDIR/cron.d"
    export VPD_CRON_INIT="$BATS_TEST_TMPDIR/S10cron"
    mkdir -p "$VPD_MODULES_DIR"
    : > /tmp/bats_curl_calls.log
    : > /tmp/bats_ip_calls.log
    : > /tmp/bats_insmod_calls.log
    source "$LIB_DIR/platform.sh"
}

# with_mock <name> <script body> puts a one-off mock first in PATH.
with_mock() {
    mkdir -p "$BATS_TEST_TMPDIR/mock"
    printf '#!/bin/bash\n%s\n' "$2" > "$BATS_TEST_TMPDIR/mock/$1"
    chmod +x "$BATS_TEST_TMPDIR/mock/$1"
    export PATH="$BATS_TEST_TMPDIR/mock:$PATH"
}

@test "platform_name: keenetic" {
    load_platform
    run platform_name
    assert_output "keenetic"
}

# ------------------------------------------------------------------- RCI

@test "_rci_get: prints the body of an RCI path" {
    load_platform
    run _rci_get show/system
    assert_success
    assert_output --partial '"hostname":"Keenetic-4521"'
    grep -q 'http://localhost:79/rci/show/system' /tmp/bats_curl_calls.log
}

@test "_rci_get: fails and prints nothing for a path NDM does not have" {
    load_platform
    run _rci_get show/interface/Wireguard9
    assert_failure
    refute_output
}

@test "_rci_get: fails when NDM does not answer" {
    load_platform
    BATS_RCI_DOWN=1 run _rci_get show/system
    assert_failure
    refute_output
}

# ---------------------------------------------------------------- WAN / IPv6

@test "platform_wan_if: the device of the IPv4 default route" {
    load_platform
    run platform_wan_if
    assert_success
    assert_output "eth2.4"
}

@test "platform_wan_if: fails and prints nothing without a default route" {
    load_platform
    BATS_IP_DEFAULT_ROUTE4="" run platform_wan_if
    assert_failure
    refute_output
}

@test "platform_ipv6_enabled: 0 without an IPv6 default route, 1 with one" {
    load_platform
    run platform_ipv6_enabled
    assert_output "0"
    BATS_IP_DEFAULT_ROUTE6="default via fe80::1 dev eth2.4 metric 1024" run platform_ipv6_enabled
    assert_output "1"
}

@test "platform_lan_ifaces: br0" {
    load_platform
    run platform_lan_ifaces
    assert_output "br0"
}

# ------------------------------------------------------------- system facts

@test "platform_lan_ip: the first IPv4 address of br0" {
    load_platform
    run platform_lan_ip
    assert_success
    assert_output "192.168.1.1"
}

@test "platform_lan_ip: fails when br0 has no address" {
    load_platform
    : > "$BATS_TEST_TMPDIR/no-addrs"
    BATS_IP_ADDRS_FILE="$BATS_TEST_TMPDIR/no-addrs" run platform_lan_ip
    assert_failure
    refute_output
}

@test "platform_hostname and platform_model come from RCI" {
    load_platform
    run platform_hostname
    assert_success
    assert_output "Keenetic-4521"
    run platform_model
    assert_success
    assert_output "Ultra (NC-1812)"
}

@test "platform_hostname: fails when NDM does not answer" {
    load_platform
    BATS_RCI_DOWN=1 run platform_hostname
    assert_failure
    refute_output
}

@test "platform_password_file: /opt/etc/passwd" {
    load_platform
    run platform_password_file
    assert_output "/opt/etc/passwd"
}

@test "platform_email_supported: 0" {
    load_platform
    run platform_email_supported
    assert_output "0"
}
