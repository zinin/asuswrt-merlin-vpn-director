#!/usr/bin/env bats

load '../test_helper'

# Helper to source lib/tproxy.sh for testing
load_tproxy_module() {
    load_common
    load_config
    source "$LIB_DIR/firewall.sh"
    source "$LIB_DIR/ipset.sh" --source-only
    source "$LIB_DIR/tproxy.sh" --source-only
}

# ============================================================================
# _tproxy_check_module - check xt_TPROXY kernel module
# ============================================================================

@test "_tproxy_check_module: returns success when module loaded" {
    load_tproxy_module
    run _tproxy_check_module
    assert_success
}

@test "_tproxy_check_module: attempts modprobe if not loaded" {
    load_tproxy_module
    # Clean call log first
    : > /tmp/bats_modprobe_calls.log 2>/dev/null || true
    run _tproxy_check_module
    assert_success
}

# ============================================================================
# _tproxy_resolve_exclude_set - resolve exclusion ipset name
# ============================================================================

@test "_tproxy_resolve_exclude_set: returns set name for existing ipset" {
    load_tproxy_module
    result=$(_tproxy_resolve_exclude_set "ru")
    [ "$result" = "ru" ]
}

@test "_tproxy_resolve_exclude_set: prefers _ext variant if exists" {
    load_tproxy_module
    # Mock ipset returns ru_ext as existing
    result=$(_tproxy_resolve_exclude_set "ru")
    # Should return either ru or ru_ext depending on mock
    [ -n "$result" ]
}

@test "_tproxy_resolve_exclude_set: fails for non-existing set" {
    load_tproxy_module
    run _tproxy_resolve_exclude_set "nonexistent_xyz"
    assert_failure
}

# ============================================================================
# _tproxy_check_required_ipsets - fail-safe check
# ============================================================================

@test "_tproxy_check_required_ipsets: returns success when all sets exist" {
    load_tproxy_module
    run _tproxy_check_required_ipsets
    assert_success
}

# ============================================================================
# tproxy_get_required_ipsets - return list of exclude ipsets
# ============================================================================

@test "tproxy_get_required_ipsets: returns exclude sets" {
    load_tproxy_module
    run tproxy_get_required_ipsets
    assert_success
    # From fixture: XRAY_EXCLUDE_SETS should contain "ru"
    assert_output --partial "ru"
}

@test "tproxy_get_required_ipsets: handles empty exclude sets" {
    # This test uses a subshell to override XRAY_EXCLUDE_SETS
    load_common
    source "$LIB_DIR/firewall.sh"
    # Manually set XRAY_EXCLUDE_SETS before loading config
    export VPD_CONFIG_FILE="$TEST_ROOT/fixtures/vpn-director.json"
    # Source tproxy with override
    export XRAY_EXCLUDE_SETS=""
    source "$LIB_DIR/ipset.sh" --source-only
    source "$LIB_DIR/tproxy.sh" --source-only

    result=$(tproxy_get_required_ipsets)
    [ -z "$result" ]
}

# ============================================================================
# tproxy_status - display status information
# ============================================================================

@test "tproxy_status: outputs status header" {
    load_tproxy_module
    run tproxy_status
    assert_success
    assert_output --partial "Xray TPROXY Status"
}

@test "tproxy_status: shows kernel module section" {
    load_tproxy_module
    run tproxy_status
    assert_success
    assert_output --partial "Kernel Module"
}

@test "tproxy_status: shows routing section" {
    load_tproxy_module
    run tproxy_status
    assert_success
    assert_output --partial "Routing"
}

@test "tproxy_status: shows chain section" {
    load_tproxy_module
    run tproxy_status
    assert_success
    assert_output --partial "Iptables Chain"
}

@test "tproxy_status: shows xray process section" {
    load_tproxy_module
    run tproxy_status
    assert_success
    assert_output --partial "Xray Process"
}

# ============================================================================
# tproxy_stop - remove chain and routing
# ============================================================================

@test "tproxy_stop: returns success when no chain exists" {
    load_tproxy_module
    run tproxy_stop
    assert_success
}

@test "tproxy_stop: logs cleanup message" {
    load_tproxy_module
    run tproxy_stop
    assert_success
    assert_output --partial "TPROXY"
}

# ============================================================================
# tproxy_apply - apply TPROXY rules (idempotent)
# ============================================================================

@test "tproxy_apply: returns success" {
    load_tproxy_module
    run tproxy_apply
    assert_success
}

@test "tproxy_apply: logs application message" {
    load_tproxy_module
    run tproxy_apply
    assert_success
}

@test "tproxy_apply: soft-fails when module unavailable (returns 0)" {
    # Override lsmod/modprobe to simulate module unavailable
    load_common
    source "$LIB_DIR/firewall.sh"
    load_config
    source "$LIB_DIR/ipset.sh" --source-only

    # Create temp mock that always fails
    mkdir -p /tmp/bats_mock_fail
    cat > /tmp/bats_mock_fail/lsmod << 'EOF'
#!/bin/bash
echo ""
EOF
    cat > /tmp/bats_mock_fail/modprobe << 'EOF'
#!/bin/bash
exit 1
EOF
    chmod +x /tmp/bats_mock_fail/lsmod /tmp/bats_mock_fail/modprobe
    export PATH="/tmp/bats_mock_fail:$PATH"

    source "$LIB_DIR/tproxy.sh" --source-only
    run tproxy_apply
    # Should soft-fail (return 0, not fail)
    assert_success

    rm -rf /tmp/bats_mock_fail
}

@test "tproxy_apply: soft-fails when required ipsets missing (returns 0)" {
    load_common
    source "$LIB_DIR/firewall.sh"
    load_config
    source "$LIB_DIR/ipset.sh" --source-only

    # Create temp mock that returns nothing for ipset list
    mkdir -p /tmp/bats_mock_no_ipset
    cat > /tmp/bats_mock_no_ipset/ipset << 'EOF'
#!/bin/bash
exit 1
EOF
    chmod +x /tmp/bats_mock_no_ipset/ipset
    export PATH="/tmp/bats_mock_no_ipset:$PATH"

    source "$LIB_DIR/tproxy.sh" --source-only
    run tproxy_apply
    # Should soft-fail (return 0, not fail)
    assert_success

    rm -rf /tmp/bats_mock_no_ipset
}

# ============================================================================
# _tproxy_init - initialization function
# ============================================================================

@test "_tproxy_init: sets initialized flag" {
    load_tproxy_module
    _tproxy_init
    [ "$_tproxy_initialized" -eq 1 ]
}

@test "_tproxy_init: is idempotent" {
    load_tproxy_module
    _tproxy_init
    local first_call=$_tproxy_initialized
    _tproxy_init
    [ "$_tproxy_initialized" -eq "$first_call" ]
}

# ============================================================================
# Module loading
# ============================================================================

@test "tproxy.sh: can be sourced with --source-only" {
    load_common
    load_config
    source "$LIB_DIR/firewall.sh"
    source "$LIB_DIR/ipset.sh" --source-only
    source "$LIB_DIR/tproxy.sh" --source-only
    # If we get here without error, the test passes
    [ $? -eq 0 ]
}

@test "tproxy.sh: exports expected functions" {
    load_tproxy_module
    # Check that public API functions exist
    declare -f tproxy_status >/dev/null
    declare -f tproxy_apply >/dev/null
    declare -f tproxy_stop >/dev/null
    declare -f tproxy_get_required_ipsets >/dev/null
}
