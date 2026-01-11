#!/usr/bin/env bats

load 'test_helper'

# Load tunnel_director functions
load_tunnel_director() {
    load_common
    load_config
    # Also load shared.sh for derive_set_name (used by resolve_set_name)
    source "$UTILS_DIR/shared.sh"
    export PATH="$BATS_TEST_DIRNAME/mocks:$PATH"
    source "$SCRIPTS_DIR/tunnel_director.sh" --source-only
}

# ============================================================================
# table_allowed
# ============================================================================

@test "table_allowed: accepts wgc1" {
    load_tunnel_director
    valid_tables="wgc1 wgc2 main"
    run table_allowed "wgc1"
    assert_success
}

@test "table_allowed: accepts main" {
    load_tunnel_director
    valid_tables="wgc1 main"
    run table_allowed "main"
    assert_success
}

@test "table_allowed: rejects unknown table" {
    load_tunnel_director
    valid_tables="wgc1 main"
    run table_allowed "wgc99"
    assert_failure
}

@test "table_allowed: rejects empty input" {
    load_tunnel_director
    valid_tables="wgc1 main"
    run table_allowed ""
    assert_failure
}

# ============================================================================
# resolve_set_name
# ============================================================================

@test "resolve_set_name: returns set name when ipset exists" {
    load_tunnel_director
    # mock ipset returns success for known sets
    run resolve_set_name "ru"
    assert_success
    assert_output "ru"
}

@test "resolve_set_name: returns empty for non-existent ipset" {
    load_tunnel_director
    run resolve_set_name "nonexistent"
    assert_success
    assert_output ""
}

# ============================================================================
# get_prerouting_base_pos
# ============================================================================

@test "get_prerouting_base_pos: returns position after iface-mark rules" {
    load_tunnel_director
    # This relies on mock iptables output
    run get_prerouting_base_pos
    assert_success
    # Should return a number
    [[ $output =~ ^[0-9]+$ ]]
}
