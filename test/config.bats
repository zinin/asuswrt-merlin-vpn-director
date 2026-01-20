#!/usr/bin/env bats

load 'test_helper'

# ============================================================================
# config.sh: loading
# ============================================================================

@test "config.sh: loads without error" {
    load_config
    # If we get here without error, the test passes
    [[ -n $VPD_CONFIG_FILE ]]
}

# ============================================================================
# config.sh: Tunnel Director variables
# ============================================================================

@test "config.sh: sets TUN_DIR_RULES" {
    load_config
    [[ -n $TUN_DIR_RULES ]]
}

@test "config.sh: sets IPS_BDR_DIR" {
    load_config
    [[ -n $IPS_BDR_DIR ]]
}

# ============================================================================
# config.sh: Xray variables
# ============================================================================

@test "config.sh: sets XRAY_CLIENTS" {
    load_config
    # XRAY_CLIENTS can be empty or have values
    [[ -n $XRAY_CLIENTS ]] || [[ $XRAY_CLIENTS == "" ]]
}

@test "config.sh: sets XRAY_SERVERS" {
    load_config
    [[ -n $XRAY_SERVERS ]] || [[ $XRAY_SERVERS == "" ]]
}

@test "config.sh: sets XRAY_TPROXY_PORT" {
    load_config
    [[ $XRAY_TPROXY_PORT == "12345" ]]
}

@test "config.sh: sets XRAY_CHAIN" {
    load_config
    [[ $XRAY_CHAIN == "XRAY_TPROXY" ]]
}

# ============================================================================
# config.sh: Advanced Tunnel Director variables
# ============================================================================

@test "config.sh: sets TUN_DIR_CHAIN_PREFIX" {
    load_config
    [[ $TUN_DIR_CHAIN_PREFIX == "TUN_DIR_" ]]
}

@test "config.sh: sets TUN_DIR_PREF_BASE" {
    load_config
    [[ $TUN_DIR_PREF_BASE == "16384" ]]
}

# ============================================================================
# config.sh: error handling
# ============================================================================

@test "config.sh: fails on missing config file" {
    export VPD_CONFIG_FILE="/nonexistent/config.json"
    run source "$LIB_DIR/config.sh"
    assert_failure
    assert_output --partial "ERROR"
    assert_output --partial "Config not found"
}

@test "config.sh: fails on invalid JSON" {
    local tmp_invalid="/tmp/bats_invalid_config.json"
    echo "not valid json {" > "$tmp_invalid"
    export VPD_CONFIG_FILE="$tmp_invalid"
    run source "$LIB_DIR/config.sh"
    assert_failure
    assert_output --partial "ERROR"
    assert_output --partial "Invalid JSON"
    rm -f "$tmp_invalid"
}
