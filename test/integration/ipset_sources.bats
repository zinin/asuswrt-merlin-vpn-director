#!/usr/bin/env bats

# test/integration/ipset_sources.bats
# Integration tests to verify IPSet source availability
# These tests require network connectivity

load '../test_helper'

setup() {
    # Skip all tests if no network connectivity
    if ! ping -c 1 -W 5 github.com >/dev/null 2>&1; then
        skip "No network connectivity"
    fi
}

# ============================================================================
# GeoLite2 via GitHub
# ============================================================================

@test "geolite2-github: ru zone is downloadable" {
    run curl -sS --max-time 30 -o /dev/null -w "%{http_code}" \
        "https://raw.githubusercontent.com/firehol/blocklist-ipsets/master/geolite2_country/country_ru.netset"
    assert_success
    assert_output "200"
}

@test "geolite2-github: ru zone has valid CIDR format" {
    local tmp="/tmp/bats_geolite2_ru.txt"
    run curl -sS --max-time 30 -o "$tmp" \
        "https://raw.githubusercontent.com/firehol/blocklist-ipsets/master/geolite2_country/country_ru.netset"
    assert_success

    # First non-comment line should be CIDR
    run bash -c "grep -v '^#' '$tmp' | head -1 | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+/[0-9]+'"
    assert_success

    rm -f "$tmp"
}

@test "geolite2-github: us zone is downloadable" {
    run curl -sS --max-time 30 -o /dev/null -w "%{http_code}" \
        "https://raw.githubusercontent.com/firehol/blocklist-ipsets/master/geolite2_country/country_us.netset"
    assert_success
    assert_output "200"
}

# ============================================================================
# IPDeny via GitHub
# ============================================================================

@test "ipdeny-github: ru zone is downloadable" {
    run curl -sS --max-time 30 -o /dev/null -w "%{http_code}" \
        "https://raw.githubusercontent.com/firehol/blocklist-ipsets/master/ipdeny_country/id_country_ru.netset"
    assert_success
    assert_output "200"
}

@test "ipdeny-github: ru zone has valid CIDR format" {
    local tmp="/tmp/bats_ipdeny_gh_ru.txt"
    run curl -sS --max-time 30 -o "$tmp" \
        "https://raw.githubusercontent.com/firehol/blocklist-ipsets/master/ipdeny_country/id_country_ru.netset"
    assert_success

    run bash -c "grep -v '^#' '$tmp' | head -1 | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+/[0-9]+'"
    assert_success

    rm -f "$tmp"
}

# ============================================================================
# IPDeny direct (may be blocked - use skip instead of fail)
# ============================================================================

@test "ipdeny-direct: ru zone availability check" {
    run curl -sS --max-time 30 -o /dev/null -w "%{http_code}" \
        "https://www.ipdeny.com/ipblocks/data/aggregated/ru-aggregated.zone"

    if [[ "$output" != "200" ]]; then
        skip "ipdeny.com not reachable (may be blocked) - status: $output"
    fi

    assert_output "200"
}
