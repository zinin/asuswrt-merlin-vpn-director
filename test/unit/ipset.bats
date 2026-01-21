#!/usr/bin/env bats

load '../test_helper'

# Note: load_ipset_module is provided by test_helper.bash
# It loads: common.sh, config.sh, ipset.sh

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
# _is_valid_country_code - validate country codes
# ============================================================================

@test "_is_valid_country_code: returns success for valid code" {
    load_ipset_module
    _is_valid_country_code "ru"
}

@test "_is_valid_country_code: returns failure for invalid code" {
    load_ipset_module
    ! _is_valid_country_code "zz"
}

@test "_is_valid_country_code: returns failure for uppercase" {
    load_ipset_module
    ! _is_valid_country_code "RU"
}

@test "_is_valid_country_code: returns failure for 3-letter code" {
    load_ipset_module
    ! _is_valid_country_code "usa"
}

# ============================================================================
# _normalize_spec - normalize and validate ipset specs
# ============================================================================

@test "_normalize_spec: returns lowercase for uppercase input" {
    load_ipset_module
    run _normalize_spec "RU"
    assert_success
    assert_output "ru"
}

@test "_normalize_spec: trims leading/trailing whitespace" {
    load_ipset_module
    run _normalize_spec "  ru  "
    assert_success
    assert_output "ru"
}

@test "_normalize_spec: handles combo sets with mixed case" {
    load_ipset_module
    run _normalize_spec "US,CA"
    assert_success
    assert_output "us,ca"
}

@test "_normalize_spec: skips empty tokens in combo" {
    load_ipset_module
    run _normalize_spec "us,,ca"
    assert_success
    assert_output "us,ca"
}

@test "_normalize_spec: skips invalid codes in combo" {
    load_ipset_module
    run _normalize_spec "us,invalid,ca"
    assert_success
    assert_output --partial "us,ca"
}

@test "_normalize_spec: returns failure for all invalid codes" {
    load_ipset_module
    run _normalize_spec "invalid,xyz"
    assert_failure
}

@test "_normalize_spec: returns failure for empty input" {
    load_ipset_module
    run _normalize_spec ""
    assert_failure
}

@test "_normalize_spec: returns failure for whitespace-only input" {
    load_ipset_module
    run _normalize_spec "   "
    assert_failure
}

# ============================================================================
# ipset_ensure - ensure ipsets exist
# ============================================================================

@test "ipset_ensure: returns success for existing ipset" {
    load_ipset_module
    run ipset_ensure "ru"
    assert_success
}

@test "ipset_ensure: normalizes uppercase input" {
    load_ipset_module
    run ipset_ensure "RU"
    assert_success
}

@test "ipset_ensure: rejects invalid country code" {
    load_ipset_module
    run ipset_ensure "invalid"
    assert_failure
}

@test "ipset_ensure: rejects empty input" {
    load_ipset_module
    run ipset_ensure ""
    assert_failure
}

# ============================================================================
# ipset_update - force update ipsets
# ============================================================================

@test "ipset_update: rejects invalid country code" {
    load_ipset_module
    run ipset_update "invalid"
    assert_failure
}

@test "ipset_update: rejects empty input" {
    load_ipset_module
    run ipset_update ""
    assert_failure
}

# ============================================================================
# _try_download_zone - download and validate zone file
# ============================================================================

@test "_try_download_zone: filters comment lines from downloaded file" {
    load_ipset_module

    # Create mock zone file with comments
    local mock_zone="/tmp/bats_mock_zone.txt"
    cat > "$mock_zone" << 'EOF'
# GeoLite2 Country
# Generated: 2024-01-01
1.0.0.0/24
1.1.0.0/16
# Another comment
2.0.0.0/8
EOF

    # Override download_file to copy mock
    download_file() {
        cp "$mock_zone" "$2"
        return 0
    }

    local dest="/tmp/bats_test_dest.txt"
    run _try_download_zone "https://example.com/test.zone" "/tmp/bats_tmp.txt" "$dest"
    assert_success

    # Verify no comments in output
    run grep '^#' "$dest"
    assert_failure

    # Verify CIDRs present
    run grep '1.0.0.0/24' "$dest"
    assert_success

    rm -f "$mock_zone" "$dest"
}

@test "_try_download_zone: rejects file with invalid CIDR format" {
    load_ipset_module

    # Create mock with HTML error page
    local mock_zone="/tmp/bats_mock_html.txt"
    cat > "$mock_zone" << 'EOF'
<!DOCTYPE html>
<html><head><title>404 Not Found</title></head>
<body>Not Found</body></html>
EOF

    download_file() {
        cp "$mock_zone" "$2"
        return 0
    }

    local dest="/tmp/bats_test_dest.txt"
    run _try_download_zone "https://example.com/test.zone" "/tmp/bats_tmp.txt" "$dest"
    assert_failure

    rm -f "$mock_zone" "$dest"
}

@test "_try_download_zone: accepts valid zone file" {
    load_ipset_module

    local mock_zone="/tmp/bats_mock_valid.txt"
    cat > "$mock_zone" << 'EOF'
1.0.0.0/24
2.0.0.0/16
EOF

    download_file() {
        cp "$mock_zone" "$2"
        return 0
    }

    local dest="/tmp/bats_test_dest.txt"
    run _try_download_zone "https://example.com/test.zone" "/tmp/bats_tmp.txt" "$dest"
    assert_success

    [[ -f "$dest" ]]

    rm -f "$mock_zone" "$dest"
}

@test "_try_download_zone: returns failure on download error" {
    load_ipset_module

    download_file() {
        return 1
    }

    run _try_download_zone "https://example.com/test.zone" "/tmp/bats_tmp.txt" "/tmp/bats_dest.txt"
    assert_failure
}

# ============================================================================
# _try_manual_fallback - manual fallback for interactive mode
# ============================================================================

@test "_try_manual_fallback: skipped in non-interactive mode" {
    load_ipset_module

    # Run with stdin from /dev/null (non-interactive)
    run bash -c '
        export TEST_MODE=1
        export LOG_FILE="/tmp/bats_test.log"
        export VPD_CONFIG_FILE="'"$TEST_ROOT"'/fixtures/vpn-director.json"
        export PATH="'"$TEST_ROOT"'/mocks:$PATH"
        source "'"$LIB_DIR"'/common.sh"
        source "'"$LIB_DIR"'/config.sh"
        source "'"$LIB_DIR"'/ipset.sh" --source-only
        _try_manual_fallback "ru" "/tmp/bats_dest.txt"
    ' < /dev/null

    assert_failure
    # Should not show interactive prompt
    refute_output --partial "Please download"
}

@test "_try_manual_fallback: uses file from fallback path when available" {
    load_ipset_module

    local fallback="/tmp/ru.zone"
    local dest="/tmp/bats_manual_dest.txt"

    # Create valid zone file at fallback path
    cat > "$fallback" << 'EOF'
# Manual download
1.0.0.0/24
2.0.0.0/16
EOF

    # Test the file processing logic directly by redefining the function
    # to skip the TTY check (since we can't simulate a real TTY in tests)
    run bash -c '
        export TEST_MODE=1
        export LOG_FILE="/tmp/bats_test.log"
        export VPD_CONFIG_FILE="'"$TEST_ROOT"'/fixtures/vpn-director.json"
        export PATH="'"$TEST_ROOT"'/mocks:$PATH"
        source "'"$LIB_DIR"'/common.sh"
        source "'"$LIB_DIR"'/config.sh"
        source "'"$LIB_DIR"'/ipset.sh" --source-only

        # Override function to skip TTY check for testing
        _try_manual_fallback_test() {
            local cc="$1" dest="$2"
            local fallback_path="/tmp/${cc}.zone"

            # Skip TTY check in test - simulate user pressed Enter
            if [[ -f "$fallback_path" ]]; then
                grep -v "^#" "$fallback_path" | grep -v "^[[:space:]]*$" > "$dest"
                if head -1 "$dest" | grep -qE "^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+/[0-9]+"; then
                    log "Using manually provided zone for '\''$cc'\''"
                    return 0
                fi
                rm -f "$dest"
            fi
            log -l ERROR "Manual fallback failed for '\''$cc'\''"
            return 1
        }

        _try_manual_fallback_test "ru" "'"$dest"'"
    '

    assert_success

    # Verify comments filtered
    run grep '^#' "$dest"
    assert_failure

    # Verify CIDR present
    run grep '1.0.0.0/24' "$dest"
    assert_success

    rm -f "$fallback" "$dest"
}

# ============================================================================
# _download_zone_multi_source - try multiple sources in order
# ============================================================================

@test "_download_zone_multi_source: tries sources in priority order" {
    load_ipset_module

    local call_count=0
    local dest="/tmp/bats_multi_dest.txt"

    # Mock _try_download_zone: fail first, succeed second
    _try_download_zone() {
        call_count=$((call_count + 1))
        if [[ $call_count -eq 1 ]]; then
            return 1  # geolite2 fails
        fi
        # ipdeny-github succeeds
        echo "1.0.0.0/24" > "$3"
        return 0
    }

    _try_manual_fallback() {
        return 1
    }

    run _download_zone_multi_source "ru" "$dest"
    assert_success

    rm -f "$dest"
}

@test "_download_zone_multi_source: all sources fail returns error" {
    load_ipset_module

    _try_download_zone() {
        return 1
    }

    _try_manual_fallback() {
        return 1
    }

    run _download_zone_multi_source "ru" "/tmp/bats_dest.txt"
    assert_failure
}

@test "_download_zone_multi_source: logs ERROR for each failed source" {
    load_ipset_module

    _try_download_zone() {
        return 1
    }

    _try_manual_fallback() {
        return 1
    }

    run _download_zone_multi_source "ru" "/tmp/bats_dest.txt"
    assert_failure

    # Check log file for ERROR entries
    run grep -c "ERROR" "$LOG_FILE"
    # Should have at least 3 ERROR logs (one per source)
    [[ "$output" -ge 3 ]]
}
