#!/usr/bin/env bats

load '../test_helper'

# Note: load_tunnel_module is provided by test_helper.bash
# It loads: common.sh, config.sh, ipset.sh, firewall.sh, tunnel.sh

# ============================================================================
# _tunnel_table_allowed - validate routing tables
# ============================================================================

@test "_tunnel_table_allowed: accepts wgc1 as valid table" {
    load_tunnel_module
    _tunnel_init
    run _tunnel_table_allowed "wgc1"
    assert_success
}

@test "_tunnel_table_allowed: accepts ovpnc1 as valid table" {
    load_tunnel_module
    _tunnel_init
    run _tunnel_table_allowed "ovpnc1"
    assert_success
}

@test "_tunnel_table_allowed: accepts main as valid table" {
    load_tunnel_module
    _tunnel_init
    run _tunnel_table_allowed "main"
    assert_success
}

@test "_tunnel_table_allowed: rejects unknown table" {
    load_tunnel_module
    _tunnel_init
    run _tunnel_table_allowed "invalid_table"
    assert_failure
}

@test "_tunnel_table_allowed: rejects empty table" {
    load_tunnel_module
    _tunnel_init
    run _tunnel_table_allowed ""
    assert_failure
}

# ============================================================================
# tunnel_get_required_ipsets - parse tunnels JSON and return required ipsets
# ============================================================================

@test "tunnel_get_required_ipsets: returns exclude sets from config" {
    load_tunnel_module
    result=$(tunnel_get_required_ipsets)
    # From fixture: wgc1 has exclude: ["ru"]
    echo "$result" | grep -q "ru"
}

@test "tunnel_get_required_ipsets: handles empty tunnels gracefully" {
    load_common
    source "$LIB_DIR/firewall.sh"
    export TUN_DIR_TUNNELS_JSON='{}'
    export VPD_CONFIG_FILE="$TEST_ROOT/fixtures/vpn-director.json"
    source "$LIB_DIR/ipset.sh" --source-only
    source "$LIB_DIR/tunnel.sh" --source-only

    result=$(tunnel_get_required_ipsets)
    [ -z "$result" ]
}

# ============================================================================
# tunnel_status - display status information
# ============================================================================

@test "tunnel_status: outputs status header" {
    load_tunnel_module
    run tunnel_status
    assert_success
    assert_output --partial "Tunnel Director Status"
}

@test "tunnel_status: shows chain section with TUN_DIR name" {
    load_tunnel_module
    run tunnel_status
    assert_success
    assert_output --partial "Chain: TUN_DIR"
}

@test "tunnel_status: shows ip rules section" {
    load_tunnel_module
    run tunnel_status
    assert_success
    assert_output --partial "IP Rules"
}

@test "tunnel_status: shows configured tunnels section" {
    load_tunnel_module
    run tunnel_status
    assert_success
    assert_output --partial "Configured Tunnels"
}

# ============================================================================
# _tunnel_get_prerouting_base_pos - find insert position
# ============================================================================

@test "_tunnel_get_prerouting_base_pos: returns position after system rules" {
    load_tunnel_module
    run _tunnel_get_prerouting_base_pos
    assert_success
    # Should return a positive integer
    [[ "$output" =~ ^[0-9]+$ ]]
}

# ============================================================================
# _tunnel_init - initialization function
# ============================================================================

@test "_tunnel_init: sets valid_tables variable" {
    load_tunnel_module
    _tunnel_init
    [ -n "$_tunnel_valid_tables" ]
}

@test "_tunnel_init: includes main in valid tables" {
    load_tunnel_module
    _tunnel_init
    [[ " $_tunnel_valid_tables " == *" main "* ]]
}

@test "_tunnel_init: sets mark mask value" {
    load_tunnel_module
    _tunnel_init
    [ -n "$_tunnel_mark_mask_val" ]
}

@test "_tunnel_init: sets mark shift value" {
    load_tunnel_module
    _tunnel_init
    [ -n "$_tunnel_mark_shift_val" ]
}

@test "_tunnel_init: computes mark field max" {
    load_tunnel_module
    _tunnel_init
    # With mask 0x00ff0000 and shift 16, max should be 255
    [ "$_tunnel_mark_field_max" -eq 255 ]
}

@test "_tunnel_init: sets mark mask hex" {
    load_tunnel_module
    _tunnel_init
    [ "$_tunnel_mark_mask_hex" = "0xff0000" ]
}

# ============================================================================
# tunnel_stop - remove single TUN_DIR chain and ip rules
# ============================================================================

@test "tunnel_stop: returns success when chain does not exist" {
    load_tunnel_module
    run tunnel_stop
    assert_success
}

@test "tunnel_stop: removes single TUN_DIR chain" {
    load_tunnel_module
    run tunnel_stop
    assert_success
    assert_output --partial "Stopping Tunnel Director"
    assert_output --partial "Tunnel Director stopped"
}

# ============================================================================
# tunnel_apply - apply rules from config (idempotent)
# ============================================================================

@test "tunnel_apply: returns success" {
    load_tunnel_module
    run tunnel_apply
    assert_success
}

@test "tunnel_apply: creates single TUN_DIR chain" {
    load_tunnel_module
    run tunnel_apply
    assert_success
    # Check that it references TUN_DIR chain and tunnel name in log
    assert_output --partial "wgc1"
}

@test "tunnel_apply: logs client routing info" {
    load_tunnel_module
    run tunnel_apply
    assert_success
    # Should mention the client from fixture (192.168.50.0/24)
    assert_output --partial "192.168.50.0/24"
}

@test "tunnel_apply: PREROUTING jump includes -i br0 interface" {
    load_tunnel_module
    # Clear iptables log
    : > /tmp/bats_iptables_calls.log
    run tunnel_apply
    assert_success
    # Verify PREROUTING rule includes -i br0 (from mock log)
    grep -q -- '-i br0.*-j TUN_DIR' /tmp/bats_iptables_calls.log
}

# ============================================================================
# Module loading
# ============================================================================

@test "tunnel.sh: can be sourced with --source-only" {
    load_common
    load_config
    source "$LIB_DIR/ipset.sh" --source-only
    run source "$LIB_DIR/tunnel.sh" --source-only
    # Note: 'run source' doesn't work well, use direct sourcing
    source "$LIB_DIR/tunnel.sh" --source-only
    # If we get here without error, the test passes
    [ $? -eq 0 ]
}

# ============================================================================
# Edge cases - Invalid JSON structure
# ============================================================================

@test "tunnel_apply: handles string instead of object in tunnels (invalid structure)" {
    load_common
    source "$LIB_DIR/firewall.sh"
    export VPD_CONFIG_FILE="$TEST_ROOT/fixtures/vpn-director-invalid-string.json"
    source "$LIB_DIR/config.sh"
    source "$LIB_DIR/ipset.sh" --source-only
    source "$LIB_DIR/tunnel.sh" --source-only

    run tunnel_apply
    # Should not crash
    assert_success
    # Should log warning about invalid tunnel config structure
    assert_output --partial "WARN"
    assert_output --partial "wgc1"
    assert_output --partial "invalid"
}

@test "tunnel_apply: handles clients as string instead of array" {
    load_common
    source "$LIB_DIR/firewall.sh"
    export VPD_CONFIG_FILE="$TEST_ROOT/fixtures/vpn-director-clients-string.json"
    source "$LIB_DIR/config.sh"
    source "$LIB_DIR/ipset.sh" --source-only
    source "$LIB_DIR/tunnel.sh" --source-only

    run tunnel_apply
    # Should not crash
    assert_success
    # Should log warning about clients being wrong type
    assert_output --partial "WARN"
    assert_output --partial "wgc1"
    assert_output --partial "clients"
    # Should skip this tunnel (no MARK rules created)
    refute_output --partial "Added:"
}

@test "tunnel_apply: handles exclude as string instead of array" {
    load_common
    source "$LIB_DIR/firewall.sh"
    export VPD_CONFIG_FILE="$TEST_ROOT/fixtures/vpn-director-exclude-string.json"
    source "$LIB_DIR/config.sh"
    source "$LIB_DIR/ipset.sh" --source-only
    source "$LIB_DIR/tunnel.sh" --source-only

    run tunnel_apply
    # Should not crash
    assert_success
    # Should log warning about exclude being wrong type
    assert_output --partial "WARN"
    assert_output --partial "exclude"
    # Should still create MARK rule for valid clients (exclusions skipped)
    assert_output --partial "Added:"
    assert_output --partial "192.168.50.0/24"
}

@test "tunnel_apply: handles overlapping clients in different tunnels (first-match wins)" {
    load_common
    source "$LIB_DIR/firewall.sh"
    export VPD_CONFIG_FILE="$TEST_ROOT/fixtures/vpn-director-overlapping.json"
    source "$LIB_DIR/config.sh"
    source "$LIB_DIR/ipset.sh" --source-only
    source "$LIB_DIR/tunnel.sh" --source-only

    # Clear iptables log
    : > /tmp/bats_iptables_calls.log

    run tunnel_apply
    assert_success

    # Both tunnels should be configured
    assert_output --partial "wgc1"
    assert_output --partial "ovpnc1"

    # Both clients should have MARK rules (first-match-wins via fwmark condition)
    assert_output --partial "192.168.50.0/24"
    assert_output --partial "192.168.50.100"
}

@test "tunnel_apply: handles non-existent exclude ipset (warns but creates MARK rule)" {
    load_common
    source "$LIB_DIR/firewall.sh"
    export VPD_CONFIG_FILE="$TEST_ROOT/fixtures/vpn-director-nonexistent-ipset.json"
    source "$LIB_DIR/config.sh"
    source "$LIB_DIR/ipset.sh" --source-only
    source "$LIB_DIR/tunnel.sh" --source-only

    run tunnel_apply
    assert_success

    # Should warn about non-existent ipset
    assert_output --partial "WARN"
    assert_output --partial "xx"
    assert_output --partial "not found"

    # But should still create the MARK rule for the client
    assert_output --partial "192.168.50.0/24"
    assert_output --partial "mark="
}
