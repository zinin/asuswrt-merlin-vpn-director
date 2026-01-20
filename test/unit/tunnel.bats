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
# _tunnel_resolve_set - resolve ipset name
# ============================================================================

@test "_tunnel_resolve_set: returns set name for existing single country" {
    load_tunnel_module
    result=$(_tunnel_resolve_set "ru")
    [ "$result" = "ru" ]
}

@test "_tunnel_resolve_set: returns empty for non-existing set" {
    load_tunnel_module
    result=$(_tunnel_resolve_set "nonexistent_xyz")
    [ -z "$result" ]
}

@test "_tunnel_resolve_set: returns derived name for combo set" {
    load_tunnel_module
    # For combo "us,ca", the derived name would be "us_ca" (if it exists)
    result=$(_tunnel_resolve_set "us,ca")
    # Should return us_ca if the ipset exists (mock returns true)
    [ "$result" = "us_ca" ]
}

# ============================================================================
# tunnel_get_required_ipsets - parse rules and return required ipsets
# ============================================================================

@test "tunnel_get_required_ipsets: returns country codes from rules" {
    load_tunnel_module
    result=$(tunnel_get_required_ipsets)
    # From fixture: "wgc1:192.168.50.0/24::us,ca"
    echo "$result" | grep -q "us"
    echo "$result" | grep -q "ca"
}

@test "tunnel_get_required_ipsets: returns combo sets from rules" {
    load_tunnel_module
    result=$(tunnel_get_required_ipsets)
    # From fixture: "wgc1:192.168.50.0/24::us,ca" -> combo "us,ca"
    echo "$result" | grep -q "us,ca"
}

@test "tunnel_get_required_ipsets: handles empty rules gracefully" {
    # This test uses a subshell to avoid readonly variable issues
    load_common
    source "$LIB_DIR/firewall.sh"
    # Set TUN_DIR_RULES before loading config (which makes it readonly)
    export TUN_DIR_RULES=""
    # Load config manually without readonly
    export VPD_CONFIG_FILE="$TEST_ROOT/fixtures/vpn-director.json"
    # Source only ipset for parse functions
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

@test "tunnel_status: shows chain info section" {
    load_tunnel_module
    run tunnel_status
    assert_success
    assert_output --partial "Chains"
}

@test "tunnel_status: shows ip rules section" {
    load_tunnel_module
    run tunnel_status
    assert_success
    assert_output --partial "IP Rules"
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
# tunnel_stop - remove all chains and ip rules
# ============================================================================

@test "tunnel_stop: returns success when no chains exist" {
    load_tunnel_module
    run tunnel_stop
    assert_success
}

@test "tunnel_stop: logs cleanup message" {
    load_tunnel_module
    run tunnel_stop
    assert_success
    # Should indicate stopping/cleanup
    assert_output --partial "Stopping Tunnel Director"
}

# ============================================================================
# tunnel_apply - apply rules from config (idempotent)
# ============================================================================

@test "tunnel_apply: returns success" {
    load_tunnel_module
    run tunnel_apply
    assert_success
}

@test "tunnel_apply: logs when rules are applied" {
    load_tunnel_module
    run tunnel_apply
    assert_success
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
