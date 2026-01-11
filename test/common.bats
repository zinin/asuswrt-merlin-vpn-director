#!/usr/bin/env bats

load 'test_helper'

# ============================================================================
# uuid4
# ============================================================================

@test "uuid4: returns valid UUID format" {
    load_common
    run uuid4
    assert_success
    # UUID format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
    assert_output --regexp '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
}

@test "uuid4: generates unique values" {
    load_common
    uuid1=$(uuid4)
    uuid2=$(uuid4)
    [ "$uuid1" != "$uuid2" ]
}

# ============================================================================
# compute_hash
# ============================================================================

@test "compute_hash: hashes string from stdin" {
    load_common
    result=$(echo -n "test" | compute_hash)
    # SHA-256 of "test" is known
    [ "$result" = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08" ]
}

@test "compute_hash: hashes file" {
    load_common
    echo -n "test" > /tmp/bats_hash_test
    run compute_hash /tmp/bats_hash_test
    assert_success
    assert_output "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
    rm /tmp/bats_hash_test
}

# ============================================================================
# is_lan_ip
# ============================================================================

@test "is_lan_ip: 192.168.x.x is private" {
    load_common
    run is_lan_ip 192.168.1.100
    assert_success
}

@test "is_lan_ip: 10.x.x.x is private" {
    load_common
    run is_lan_ip 10.0.0.1
    assert_success
}

@test "is_lan_ip: 172.16.x.x is private" {
    load_common
    run is_lan_ip 172.16.0.1
    assert_success
}

@test "is_lan_ip: 172.31.x.x is private" {
    load_common
    run is_lan_ip 172.31.255.255
    assert_success
}

@test "is_lan_ip: 172.15.x.x is NOT private" {
    load_common
    run is_lan_ip 172.15.0.1
    assert_failure
}

@test "is_lan_ip: 8.8.8.8 is NOT private" {
    load_common
    run is_lan_ip 8.8.8.8
    assert_failure
}

@test "is_lan_ip: IPv6 ULA fd00:: is private" {
    load_common
    run is_lan_ip -6 "fd00::1"
    assert_success
}

@test "is_lan_ip: IPv6 link-local fe80:: is private" {
    load_common
    run is_lan_ip -6 "fe80::1"
    assert_success
}

@test "is_lan_ip: IPv6 global 2001:: is NOT private" {
    load_common
    run is_lan_ip -6 "2001:4860::1"
    assert_failure
}

# ============================================================================
# resolve_ip
# ============================================================================

@test "resolve_ip: returns literal IPv4" {
    load_common
    run resolve_ip 192.168.1.1
    assert_success
    assert_output "192.168.1.1"
}

@test "resolve_ip: resolves from /etc/hosts" {
    load_common
    # Uses fixture hosts file via HOSTS_FILE env
    run resolve_ip mypc
    assert_success
    assert_output "192.168.1.100"
}

@test "resolve_ip: -q suppresses error on failure" {
    load_common
    run resolve_ip -q nonexistent.invalid
    assert_failure
    assert_output ""
}

# ============================================================================
# log
# ============================================================================

@test "log: writes to LOG_FILE" {
    load_common
    log "test message"
    run cat "$LOG_FILE"
    assert_success
    assert_output --partial "INFO"
    assert_output --partial "test message"
}

@test "log: supports -l ERROR level" {
    load_common
    log -l ERROR "error message"
    run cat "$LOG_FILE"
    assert_output --partial "ERROR"
    assert_output --partial "error message"
}

@test "log: supports -l WARN level" {
    load_common
    log -l WARN "warning message"
    run cat "$LOG_FILE"
    assert_output --partial "WARN"
}

@test "log_error_trace: includes stack trace" {
    load_common

    # Define nested function to test stack trace
    inner_func() { log_error_trace "inner error"; }
    outer_func() { inner_func; }

    outer_func

    run cat "$LOG_FILE"
    assert_output --partial "inner error"
    assert_output --partial "at"
}

# ============================================================================
# strip_comments
# ============================================================================

@test "strip_comments: removes # comments" {
    load_common
    input=$'line1\n# comment\nline2'
    run strip_comments "$input"
    assert_success
    assert_line -n 0 "line1"
    assert_line -n 1 "line2"
}

@test "strip_comments: removes inline comments" {
    load_common
    run strip_comments "value # comment"
    assert_success
    assert_output "value"
}

@test "strip_comments: trims whitespace" {
    load_common
    run strip_comments "  spaced  "
    assert_success
    assert_output "spaced"
}
