#!/usr/bin/env bats

load 'test_helper'

# Helper to source ipset_builder.sh functions
load_ipset_builder() {
    load_common
    load_config
    source "$SCRIPTS_DIR/ipset_builder.sh" --source-only
}

# ============================================================================
# parse_country_codes (reads rules via stdin)
# ============================================================================

@test "parse_country_codes: extracts country codes from rule field 4" {
    load_ipset_builder

    result=$(echo "wgc1:192.168.50.0/24::ru" | parse_country_codes)
    [ "$result" = "ru" ]
}

@test "parse_country_codes: extracts multiple countries from comma list" {
    load_ipset_builder

    result=$(echo "wgc1:192.168.50.0/24::us,ca,de" | parse_country_codes)
    echo "$result" | grep -q "ca"
    echo "$result" | grep -q "de"
    echo "$result" | grep -q "us"
}

@test "parse_country_codes: handles exclusion field (field 5)" {
    load_ipset_builder

    result=$(echo "wgc1:192.168.50.0/24::us:ru" | parse_country_codes)
    echo "$result" | grep -q "ru"
    echo "$result" | grep -q "us"
}

@test "parse_country_codes: ignores invalid country codes" {
    load_ipset_builder

    result=$(echo "wgc1:192.168.50.0/24::invalid,us" | parse_country_codes)
    [ "$result" = "us" ]
}

@test "parse_country_codes: handles multiple rules" {
    load_ipset_builder

    result=$(printf "wgc1:192.168.50.0/24::us\nwgc2:192.168.60.0/24::ca" | parse_country_codes)
    echo "$result" | grep -q "ca"
    echo "$result" | grep -q "us"
}

@test "parse_country_codes: deduplicates country codes" {
    load_ipset_builder

    result=$(printf "wgc1:192.168.50.0/24::us\nwgc2:192.168.60.0/24::us" | parse_country_codes)
    [ "$result" = "us" ]
}

# ============================================================================
# parse_combo_from_rules (reads rules via stdin)
# ============================================================================

@test "parse_combo_from_rules: extracts combo sets with comma" {
    load_ipset_builder

    result=$(echo "wgc1:192.168.50.0/24::us,ca" | parse_combo_from_rules)
    [ "$result" = "us,ca" ]
}

@test "parse_combo_from_rules: ignores single country (no combo)" {
    load_ipset_builder

    result=$(echo "wgc1:192.168.50.0/24::us" | parse_combo_from_rules)
    [ -z "$result" ]
}

@test "parse_combo_from_rules: extracts combo from exclusion field" {
    load_ipset_builder

    result=$(echo "wgc1:192.168.50.0/24::any:ru,ua" | parse_combo_from_rules)
    [ "$result" = "ru,ua" ]
}

@test "parse_combo_from_rules: handles multiple combos" {
    load_ipset_builder

    result=$(printf "wgc1:192.168.50.0/24::us,ca\nwgc2:192.168.60.0/24::de,fr" | parse_combo_from_rules)
    echo "$result" | grep -q "de,fr"
    echo "$result" | grep -q "us,ca"
}

@test "parse_combo_from_rules: deduplicates identical combos" {
    load_ipset_builder

    result=$(printf "wgc1:192.168.50.0/24::us,ca\nwgc2:192.168.60.0/24::us,ca" | parse_combo_from_rules)
    [ "$result" = "us,ca" ]
}

# ============================================================================
# Helper functions
# ============================================================================

@test "_next_pow2: rounds up to next power of two" {
    load_ipset_builder

    run _next_pow2 1
    assert_success
    assert_output "1"

    run _next_pow2 2
    assert_success
    assert_output "2"

    run _next_pow2 3
    assert_success
    assert_output "4"

    run _next_pow2 1000
    assert_success
    assert_output "1024"
}

@test "_calc_ipset_size: returns at least 1024" {
    load_ipset_builder

    run _calc_ipset_size 0
    assert_success
    assert_output "1024"

    run _calc_ipset_size 100
    assert_success
    assert_output "1024"
}

@test "_calc_ipset_size: calculates correct size for large sets" {
    load_ipset_builder

    # For 10000 entries: buckets = (4*10000+2)/3 = 13334, next pow2 = 16384
    run _calc_ipset_size 10000
    assert_success
    assert_output "16384"
}
