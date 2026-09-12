#!/usr/bin/env bats

load '../test_helper'

# lib/platform.sh is sourced directly here; common.sh sources it in production.
load_platform() {
    source "$LIB_DIR/platform.sh"
}

# ============================================================================
# platform_detect - file-system probes, prefixed by VPD_PROBE_ROOT
# ============================================================================

@test "platform_detect: keenetic when /opt/etc/ndm and /bin/ndmc exist" {
    load_platform
    local root="$BATS_TEST_TMPDIR/root"
    mkdir -p "$root/opt/etc/ndm" "$root/bin"
    printf '#!/bin/sh\n' > "$root/bin/ndmc"
    chmod +x "$root/bin/ndmc"
    VPD_PROBE_ROOT="$root" run platform_detect
    assert_success
    assert_output "keenetic"
}

@test "platform_detect: merlin when /jffs and /bin/nvram exist" {
    load_platform
    local root="$BATS_TEST_TMPDIR/root"
    mkdir -p "$root/jffs" "$root/bin"
    printf '#!/bin/sh\n' > "$root/bin/nvram"
    chmod +x "$root/bin/nvram"
    VPD_PROBE_ROOT="$root" run platform_detect
    assert_success
    assert_output "merlin"
}

@test "platform_detect: keenetic wins when both trees exist" {
    load_platform
    local root="$BATS_TEST_TMPDIR/root"
    mkdir -p "$root/jffs" "$root/opt/etc/ndm" "$root/bin"
    printf '#!/bin/sh\n' > "$root/bin/nvram"
    printf '#!/bin/sh\n' > "$root/bin/ndmc"
    chmod +x "$root/bin/nvram" "$root/bin/ndmc"
    VPD_PROBE_ROOT="$root" run platform_detect
    assert_output "keenetic"
}

@test "platform_detect: fails and prints nothing when neither tree exists" {
    load_platform
    mkdir -p "$BATS_TEST_TMPDIR/empty"
    VPD_PROBE_ROOT="$BATS_TEST_TMPDIR/empty" run platform_detect
    assert_failure
    refute_output
}

# ============================================================================
# Loader
# ============================================================================

@test "platform.sh: VPD_PLATFORM set by the environment is used as is" {
    load_platform
    run platform_name
    assert_success
    assert_output "merlin"
}

@test "platform.sh: an unsupported VPD_PLATFORM aborts the sourcing script" {
    run env VPD_PLATFORM=amiga bash -c "set -e; source '$LIB_DIR/platform.sh'; echo reached"
    assert_failure
    assert_output --partial "unsupported platform: amiga"
    refute_output --partial "reached"
}

@test "platform.sh: a failed detection aborts the sourcing script" {
    mkdir -p "$BATS_TEST_TMPDIR/empty"
    run env -u VPD_PLATFORM VPD_PROBE_ROOT="$BATS_TEST_TMPDIR/empty" \
        bash -c "set -e; source '$LIB_DIR/platform.sh'; echo reached"
    assert_failure
    assert_output --partial "unsupported platform"
    refute_output --partial "reached"
}

@test "platform.sh: detection fills VPD_PLATFORM when it is unset" {
    local root="$BATS_TEST_TMPDIR/root"
    mkdir -p "$root/jffs" "$root/bin"
    printf '#!/bin/sh\n' > "$root/bin/nvram"
    chmod +x "$root/bin/nvram"
    run env -u VPD_PLATFORM VPD_PROBE_ROOT="$root" \
        bash -c "set -e; source '$LIB_DIR/platform.sh'; printf '%s\n' \"\$VPD_PLATFORM\""
    assert_success
    assert_output "merlin"
}

@test "platform.sh: keenetic loads lib/platform/keenetic.sh" {
    run env VPD_PLATFORM=keenetic bash -c "set -e; source '$LIB_DIR/platform.sh'; platform_name"
    assert_success
    assert_output "keenetic"
}

@test "platform.sh: a platform without an implementation file is reported" {
    # The implementation is looked up next to the loader, so a copy of the
    # loader in a directory without platform/ has none to find.
    mkdir -p "$BATS_TEST_TMPDIR/lib"
    cp "$LIB_DIR/platform.sh" "$BATS_TEST_TMPDIR/lib/"
    run env VPD_PLATFORM=keenetic bash -c "set -e; source '$BATS_TEST_TMPDIR/lib/platform.sh'; echo reached"
    assert_failure
    assert_output --partial "platform implementation not found"
    refute_output --partial "reached"
}
