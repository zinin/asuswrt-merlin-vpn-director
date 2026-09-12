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

# ============================================================================
# download_file
# ============================================================================

@test "download_file: backward compatible with 2 args" {
    load_common

    # Test that function accepts 2 args (backward compatibility)
    # Uses real curl with a stable test URL
    run download_file "http://www.msftconnecttest.com/connecttest.txt" "/tmp/bats_test_dl"
    assert_success

    rm -f /tmp/bats_test_dl
}

@test "download_file: accepts custom timeout parameter" {
    load_common

    # Test with invalid URL to trigger failure path
    # This verifies the function signature works with 3 args
    run download_file "http://invalid.localhost.test/file" "/tmp/bats_test_dl2" 1
    assert_failure

    rm -f /tmp/bats_test_dl2
}

# Keenetic's busybox wget has no TLS and segfaults on https URLs; curl is there.
@test "download_file: falls back to curl when wget fails" {
    load_common
    mkdir -p "$BATS_TEST_TMPDIR/bin"
    printf '#!/bin/bash\nexit 139\n' > "$BATS_TEST_TMPDIR/bin/wget"
    cat > "$BATS_TEST_TMPDIR/bin/curl" <<'EOF'
#!/bin/bash
out=""
while [[ $# -gt 0 ]]; do case "$1" in -o) out="$2"; shift 2 ;; *) shift ;; esac; done
printf 'payload\n' > "$out"
EOF
    chmod +x "$BATS_TEST_TMPDIR/bin/wget" "$BATS_TEST_TMPDIR/bin/curl"
    PATH="$BATS_TEST_TMPDIR/bin:$PATH" run download_file "https://example.invalid/file" "$BATS_TEST_TMPDIR/out"
    assert_success
    run cat "$BATS_TEST_TMPDIR/out"
    assert_output "payload"
}

@test "download_file: fails and leaves no file when wget and curl both fail" {
    load_common
    mkdir -p "$BATS_TEST_TMPDIR/bin"
    printf '#!/bin/bash\nexit 139\n' > "$BATS_TEST_TMPDIR/bin/wget"
    printf '#!/bin/bash\nexit 22\n' > "$BATS_TEST_TMPDIR/bin/curl"
    chmod +x "$BATS_TEST_TMPDIR/bin/wget" "$BATS_TEST_TMPDIR/bin/curl"
    PATH="$BATS_TEST_TMPDIR/bin:$PATH" run download_file "https://example.invalid/file" "$BATS_TEST_TMPDIR/out"
    assert_failure
    assert_output --partial "Failed to download"
    [ ! -e "$BATS_TEST_TMPDIR/out" ]
}

# ============================================================================
# Platform contract is available through common.sh
# ============================================================================

@test "common.sh: sourcing it loads the platform contract" {
    load_common
    run platform_name
    assert_success
    assert_output "merlin"
}

# A router that got this common.sh without platform.sh - the shape of an update
# delivered by an updater that predates the platform layer - used to die on the
# shell's own "No such file or directory", naming no way out. It must say what
# is missing and how to get it, and still fail: without the contract every
# platform_* call below would be a command-not-found mid-apply.
@test "common.sh: names the missing platform.sh and still fails" {
    local lib="$BATS_TEST_TMPDIR/lib"
    mkdir -p "$lib"
    cp "$LIB_DIR/common.sh" "$lib/common.sh"
    run bash -c "set -e; source '$lib/common.sh'; echo reached"
    assert_failure
    assert_output --partial "$lib/platform.sh"
    assert_output --partial "install.sh"
    refute_output --partial "reached"
}

@test "get_active_wan_if: wraps platform_wan_if" {
    load_common
    run get_active_wan_if
    assert_success
    assert_output "eth0"
}

@test "get_active_wan_if: prints nothing and still succeeds when the platform has no answer" {
    load_common
    platform_wan_if() { return 1; }
    run get_active_wan_if
    assert_success
    refute_output
}

@test "get_ipv6_enabled: wraps platform_ipv6_enabled" {
    load_common
    run get_ipv6_enabled
    assert_output "1"
    platform_ipv6_enabled() { printf '0\n'; }
    run get_ipv6_enabled
    assert_output "0"
}
