#!/usr/bin/env bats

load '../test_helper'

# Helper to source lib/ipset.sh for testing
load_ipset_module() {
    load_common
    load_config
    source "$LIB_DIR/ipset.sh" --source-only
}

# ============================================================================
# _next_pow2 - round up to next power of two
# ============================================================================

@test "_next_pow2: returns 1 for input 0" {
    load_ipset_module
    run _next_pow2 0
    assert_success
    assert_output "1"
}

@test "_next_pow2: returns 1 for input 1" {
    load_ipset_module
    run _next_pow2 1
    assert_success
    assert_output "1"
}

@test "_next_pow2: returns 2 for input 2" {
    load_ipset_module
    run _next_pow2 2
    assert_success
    assert_output "2"
}

@test "_next_pow2: rounds 3 up to 4" {
    load_ipset_module
    run _next_pow2 3
    assert_success
    assert_output "4"
}

@test "_next_pow2: rounds 1000 up to 1024" {
    load_ipset_module
    run _next_pow2 1000
    assert_success
    assert_output "1024"
}

@test "_next_pow2: returns exact power of two unchanged" {
    load_ipset_module
    run _next_pow2 512
    assert_success
    assert_output "512"
}

# ============================================================================
# _calc_ipset_size - calculate hashsize with minimum of 1024
# ============================================================================

@test "_calc_ipset_size: returns at least 1024 for 0 entries" {
    load_ipset_module
    run _calc_ipset_size 0
    assert_success
    assert_output "1024"
}

@test "_calc_ipset_size: returns at least 1024 for small entry counts" {
    load_ipset_module
    run _calc_ipset_size 100
    assert_success
    assert_output "1024"
}

@test "_calc_ipset_size: calculates correct size for large sets" {
    load_ipset_module
    # For 10000 entries: buckets = (4*10000+2)/3 = 13334, next pow2 = 16384
    run _calc_ipset_size 10000
    assert_success
    assert_output "16384"
}

# ============================================================================
# _derive_set_name - lowercase or hash for long names
# ============================================================================

@test "_derive_set_name: returns lowercase for short names" {
    load_ipset_module
    run _derive_set_name "RU"
    assert_success
    assert_output "ru"
}

@test "_derive_set_name: returns lowercase for names at limit (31 chars)" {
    load_ipset_module
    local name="abcdefghijklmnopqrstuvwxyz12345"  # 31 chars
    run _derive_set_name "$name"
    assert_success
    assert_output "$name"
}

@test "_derive_set_name: returns 24-char hash for names over 31 chars" {
    load_ipset_module
    local long_name="this_is_a_very_long_ipset_name_that_exceeds_limit"
    # Call directly (not via run) to avoid capturing TRACE logs
    local result
    result=$(_derive_set_name "$long_name")
    # Output should be 24 chars (SHA256 prefix)
    [ ${#result} -eq 24 ]
}

@test "_derive_set_name: hash is deterministic" {
    load_ipset_module
    local long_name="this_is_a_very_long_ipset_name_that_exceeds_limit"
    run _derive_set_name "$long_name"
    local first_result="$output"
    run _derive_set_name "$long_name"
    [ "$output" = "$first_result" ]
}

# ============================================================================
# parse_country_codes - extract country codes from rules
# ============================================================================

@test "parse_country_codes: extracts country code from field 4" {
    load_ipset_module
    result=$(echo "wgc1:192.168.50.0/24::ru" | parse_country_codes)
    [ "$result" = "ru" ]
}

@test "parse_country_codes: extracts multiple countries from comma list" {
    load_ipset_module
    result=$(echo "wgc1:192.168.50.0/24::us,ca,de" | parse_country_codes)
    echo "$result" | grep -q "ca"
    echo "$result" | grep -q "de"
    echo "$result" | grep -q "us"
}

@test "parse_country_codes: handles exclusion field (field 5)" {
    load_ipset_module
    result=$(echo "wgc1:192.168.50.0/24::us:ru" | parse_country_codes)
    echo "$result" | grep -q "ru"
    echo "$result" | grep -q "us"
}

@test "parse_country_codes: ignores invalid country codes" {
    load_ipset_module
    result=$(echo "wgc1:192.168.50.0/24::invalid,us" | parse_country_codes)
    [ "$result" = "us" ]
}

@test "parse_country_codes: deduplicates country codes" {
    load_ipset_module
    result=$(printf "wgc1:192.168.50.0/24::us\nwgc2:192.168.60.0/24::us" | parse_country_codes)
    [ "$result" = "us" ]
}

# ============================================================================
# parse_combo_from_rules - extract combo ipsets from rules
# ============================================================================

@test "parse_combo_from_rules: extracts combo sets with comma" {
    load_ipset_module
    result=$(echo "wgc1:192.168.50.0/24::us,ca" | parse_combo_from_rules)
    [ "$result" = "us,ca" ]
}

@test "parse_combo_from_rules: ignores single country (no combo)" {
    load_ipset_module
    result=$(echo "wgc1:192.168.50.0/24::us" | parse_combo_from_rules)
    [ -z "$result" ]
}

@test "parse_combo_from_rules: extracts combo from exclusion field" {
    load_ipset_module
    result=$(echo "wgc1:192.168.50.0/24::any:ru,ua" | parse_combo_from_rules)
    [ "$result" = "ru,ua" ]
}

@test "parse_combo_from_rules: deduplicates identical combos" {
    load_ipset_module
    result=$(printf "wgc1:192.168.50.0/24::us,ca\nwgc2:192.168.60.0/24::us,ca" | parse_combo_from_rules)
    [ "$result" = "us,ca" ]
}

# ============================================================================
# _ipset_exists - check if ipset exists
# ============================================================================

@test "_ipset_exists: returns success for existing ipset" {
    load_ipset_module
    run _ipset_exists "ru"
    assert_success
}

@test "_ipset_exists: returns failure for non-existing ipset" {
    load_ipset_module
    run _ipset_exists "nonexistent_ipset_xyz"
    assert_failure
}

# ============================================================================
# _ipset_count - get entry count
# ============================================================================

@test "_ipset_count: returns count for existing ipset" {
    load_ipset_module
    run _ipset_count "ru"
    assert_success
    assert_output "1000"
}

# ============================================================================
# ipset_status - show ipset information
# ============================================================================

@test "ipset_status: outputs status header" {
    load_ipset_module
    run ipset_status
    assert_success
    assert_output --partial "IPSet Status"
}

# ============================================================================
# ipset_ensure - ensure ipsets exist
# ============================================================================

@test "ipset_ensure: returns success for existing ipset" {
    load_ipset_module
    run ipset_ensure "ru"
    assert_success
}
