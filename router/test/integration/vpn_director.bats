#!/usr/bin/env bats

# test/integration/vpn_director.bats
# Integration tests for vpn-director.sh CLI

load '../test_helper'

setup() {
    export PATH="$TEST_ROOT/mocks:$PATH"
    export VPD_CONFIG_FILE="$TEST_ROOT/fixtures/vpn-director.json"
    export LOG_FILE="/tmp/bats_test_vpn_director.log"
    export TEST_MODE=1
    : > "$LOG_FILE"
}

# ============================================================================
# Help and basic CLI tests
# ============================================================================

@test "vpn-director: shows help with no args" {
    run "$SCRIPTS_DIR/vpn-director.sh"
    assert_success
    assert_output --partial "Usage:"
}

@test "vpn-director: --help shows usage" {
    run "$SCRIPTS_DIR/vpn-director.sh" --help
    assert_success
    assert_output --partial "Usage:"
    assert_output --partial "Commands:"
    assert_output --partial "Options:"
}

@test "vpn-director: -h shows usage" {
    run "$SCRIPTS_DIR/vpn-director.sh" -h
    assert_success
    assert_output --partial "Usage:"
}

@test "vpn-director: unknown command fails" {
    run "$SCRIPTS_DIR/vpn-director.sh" unknown
    assert_failure
    assert_output --partial "Unknown command"
}

@test "vpn-director: unknown option fails" {
    run "$SCRIPTS_DIR/vpn-director.sh" --badoption
    assert_failure
    assert_output --partial "Unknown option"
}

# ============================================================================
# Status command tests
# ============================================================================

@test "vpn-director: status command works" {
    run "$SCRIPTS_DIR/vpn-director.sh" status
    assert_success
    assert_output --partial "IPSet Status"
    assert_output --partial "Tunnel Director Status"
    assert_output --partial "Xray TPROXY Status"
}

@test "vpn-director: status ipset shows only ipset" {
    run "$SCRIPTS_DIR/vpn-director.sh" status ipset
    assert_success
    assert_output --partial "IPSet Status"
    refute_output --partial "Tunnel Director"
    refute_output --partial "Xray TPROXY"
}

@test "vpn-director: status tunnel shows only tunnel" {
    run "$SCRIPTS_DIR/vpn-director.sh" status tunnel
    assert_success
    assert_output --partial "Tunnel Director Status"
    refute_output --partial "IPSet Status"
    refute_output --partial "Xray TPROXY"
}

@test "vpn-director: status xray shows only tproxy" {
    run "$SCRIPTS_DIR/vpn-director.sh" status xray
    assert_success
    assert_output --partial "Xray TPROXY Status"
    refute_output --partial "IPSet Status"
    refute_output --partial "Tunnel Director"
}

@test "vpn-director: status tproxy alias works" {
    run "$SCRIPTS_DIR/vpn-director.sh" status tproxy
    assert_success
    assert_output --partial "Xray TPROXY Status"
}

@test "vpn-director: status unknown component fails" {
    run "$SCRIPTS_DIR/vpn-director.sh" status badcomp
    assert_failure
    assert_output --partial "Unknown component"
}

# ============================================================================
# apply command tests
# ============================================================================

@test "vpn-director: apply --dry-run shows plan without applying" {
    run "$SCRIPTS_DIR/vpn-director.sh" apply --dry-run
    assert_success
    assert_output --partial "DRY-RUN"
    assert_output --partial "would apply"
}

@test "vpn-director: apply tunnel --dry-run shows tunnel ipsets" {
    run "$SCRIPTS_DIR/vpn-director.sh" apply tunnel --dry-run
    assert_success
    assert_output --partial "DRY-RUN"
    assert_output --partial "tunnel ipsets"
}

@test "vpn-director: apply xray --dry-run shows tproxy ipsets" {
    run "$SCRIPTS_DIR/vpn-director.sh" apply xray --dry-run
    assert_success
    assert_output --partial "DRY-RUN"
    assert_output --partial "tproxy ipsets"
}

# ============================================================================
# Option parsing (both positions)
# ============================================================================

@test "vpn-director: options work before command" {
    run "$SCRIPTS_DIR/vpn-director.sh" --dry-run apply
    assert_success
    assert_output --partial "DRY-RUN"
}

@test "vpn-director: options work after command" {
    run "$SCRIPTS_DIR/vpn-director.sh" apply --dry-run
    assert_success
    assert_output --partial "DRY-RUN"
}

@test "vpn-director: options work after component" {
    run "$SCRIPTS_DIR/vpn-director.sh" apply tunnel --dry-run
    assert_success
    assert_output --partial "DRY-RUN"
}

@test "vpn-director: multiple options work together" {
    run "$SCRIPTS_DIR/vpn-director.sh" --force apply --dry-run
    assert_success
    assert_output --partial "DRY-RUN"
}

@test "vpn-director: component argument parsed correctly" {
    run "$SCRIPTS_DIR/vpn-director.sh" status tunnel
    assert_success
    assert_output --partial "Tunnel Director Status"
    refute_output --partial "IPSet Status"
}

# ============================================================================
# stop command tests
# ============================================================================

@test "vpn-director: stop unknown component fails" {
    run "$SCRIPTS_DIR/vpn-director.sh" stop badcomp
    assert_failure
    assert_output --partial "Unknown component"
}

# ============================================================================
# restart command tests
# ============================================================================

@test "vpn-director: restart unknown component fails" {
    run "$SCRIPTS_DIR/vpn-director.sh" restart badcomp
    assert_failure
    assert_output --partial "Unknown component"
}

# ============================================================================
# Verbose mode tests
# ============================================================================

@test "vpn-director: -v enables verbose mode" {
    run "$SCRIPTS_DIR/vpn-director.sh" -v --help
    assert_success
    assert_output --partial "Usage:"
}

@test "vpn-director: --verbose enables verbose mode" {
    run "$SCRIPTS_DIR/vpn-director.sh" --verbose --help
    assert_success
    assert_output --partial "Usage:"
}

# ============================================================================
# --source-only mode tests
# ============================================================================

@test "vpn-director: --source-only loads functions without executing" {
    run bash -c 'source "$1" --source-only && echo "sourced ok"' -- "$SCRIPTS_DIR/vpn-director.sh"
    assert_success
    assert_output "sourced ok"
}

@test "vpn-director: --source-only exports cmd functions" {
    run bash -c 'source "$1" --source-only && type cmd_status' -- "$SCRIPTS_DIR/vpn-director.sh"
    assert_success
    assert_output --partial "function"
}

@test "vpn-director: --source-only exports cmd_apply function" {
    run bash -c 'source "$1" --source-only && type cmd_apply' -- "$SCRIPTS_DIR/vpn-director.sh"
    assert_success
    assert_output --partial "function"
}

# ============================================================================
# Quiet mode tests
# ============================================================================

@test "vpn-director: -q option is parsed" {
    run "$SCRIPTS_DIR/vpn-director.sh" -q --help
    assert_success
    assert_output --partial "Usage:"
}

@test "vpn-director: --quiet option is parsed" {
    run "$SCRIPTS_DIR/vpn-director.sh" --quiet --help
    assert_success
    assert_output --partial "Usage:"
}

# ============================================================================
# Force option tests
# ============================================================================

@test "vpn-director: -f option is parsed" {
    run "$SCRIPTS_DIR/vpn-director.sh" -f --help
    assert_success
    assert_output --partial "Usage:"
}

@test "vpn-director: --force option is parsed" {
    run "$SCRIPTS_DIR/vpn-director.sh" --force --help
    assert_success
    assert_output --partial "Usage:"
}
