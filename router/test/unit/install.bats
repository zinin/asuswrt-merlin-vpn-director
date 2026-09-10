#!/usr/bin/env bats

load '../test_helper'

# install.sh is sourced with --source-only, so main() never runs. The paths it
# writes to are redirected into BATS_TEST_TMPDIR by overriding the globals it
# declares at the top of the file.
load_installer() {
    source "$PROJECT_ROOT/../install.sh" --source-only
    VPD_DIR="$BATS_TEST_TMPDIR/vpn-director"
    INIT_DIR="$BATS_TEST_TMPDIR/init.d"
    mkdir -p "$VPD_DIR" "$INIT_DIR"
    WEBUI_INIT="$INIT_DIR/S98vpn-director-webui"
}

# fake_init writes a stand-in init script that records its arguments and exits
# with the given code. $code is expanded now; \$1 is left for the inner script.
fake_init() {
    local code="${1:-0}"
    cat > "$WEBUI_INIT" <<EOF
#!/bin/sh
echo "\$1" >> "$BATS_TEST_TMPDIR/init.calls"
exit $code
EOF
    chmod +x "$WEBUI_INIT"
}

@test "start_webui: does nothing when the webui binary was not installed" {
    load_installer
    fake_init 0

    run start_webui

    assert_success
    [[ ! -f "$BATS_TEST_TMPDIR/init.calls" ]]
}

@test "start_webui: starts the daemon and reports the LAN address" {
    load_installer
    touch "$VPD_DIR/webui"
    chmod +x "$VPD_DIR/webui"
    fake_init 0

    run start_webui

    assert_success
    assert_output --partial "https://192.168.50.1:8444"
    run cat "$BATS_TEST_TMPDIR/init.calls"
    assert_output "start"
}

@test "start_webui: a failed start is reported without aborting the installer" {
    load_installer
    touch "$VPD_DIR/webui"
    chmod +x "$VPD_DIR/webui"
    fake_init 1

    run start_webui

    assert_success
    assert_output --partial "Failed to start Web UI"
    # Without the early return the installer would report the failure and then
    # go on to announce a URL for a daemon that is not running.
    refute_output --partial "Web UI started"
}

@test "start_webui: falls back to a default address when nvram has no lan_ipaddr" {
    load_installer
    touch "$VPD_DIR/webui"
    chmod +x "$VPD_DIR/webui"
    fake_init 0
    # The mock answers "" for anything it does not know; an empty address in the
    # printed URL would be worse than a wrong-but-plausible default.
    NVRAM_NO_LAN_IP=1 run start_webui

    assert_success
    assert_output --partial "https://192.168.1.1:8444"
}

@test "start_webui: uses webui.port from the config when present" {
    load_installer
    touch "$VPD_DIR/webui"
    chmod +x "$VPD_DIR/webui"
    fake_init 0
    printf '%s\n' '{"webui":{"port":9444}}' > "$VPD_DIR/vpn-director.json"

    run start_webui

    assert_success
    assert_output --partial "https://192.168.50.1:9444"
}

@test "start_webui: records the LAN URL in WEBUI_URL for print_next_steps" {
    load_installer
    touch "$VPD_DIR/webui"
    chmod +x "$VPD_DIR/webui"
    fake_init 0

    # Deliberately not `run`: it forks a subshell, so an assignment to the
    # global would be lost and print_next_steps would take the wrong branch.
    start_webui

    assert_equal "$WEBUI_URL" "https://192.168.50.1:8444"
}

@test "setup_webui_config: copies the template when vpn-director.json is missing" {
    load_installer
    printf '%s\n' '{"webui":{"port":8444}}' > "$VPD_DIR/vpn-director.json.template"

    run setup_webui_config

    assert_success
    assert_output --partial "Created"
    [ -f "$VPD_DIR/vpn-director.json" ]
    run stat -c %a "$VPD_DIR/vpn-director.json"
    assert_output "600"
}

@test "setup_webui_config: does nothing when neither json nor template exist" {
    load_installer

    run setup_webui_config

    assert_success
    [ ! -f "$VPD_DIR/vpn-director.json" ]
}

# ============================================================================
# files.manifest parsing
# ============================================================================

write_manifest() {
    cat > "$BATS_TEST_TMPDIR/files.manifest" <<'EOF'
# comment line
common   router/opt/vpn-director/vpn-director.sh

merlin   router/jffs/scripts/firewall-start
keenetic router/opt/etc/ndm/netfilter.d/50-vpn-director.sh
common   router/opt/etc/xray/config.json.template
EOF
}

@test "manifest_files: prints common and platform paths in manifest order" {
    load_installer
    write_manifest
    run manifest_files "$BATS_TEST_TMPDIR/files.manifest" merlin
    assert_success
    assert_line --index 0 "router/opt/vpn-director/vpn-director.sh"
    assert_line --index 1 "router/jffs/scripts/firewall-start"
    assert_line --index 2 "router/opt/etc/xray/config.json.template"
    refute_output --partial "netfilter.d"
}

@test "manifest_files: selects the other platform's files with its tag" {
    load_installer
    write_manifest
    run manifest_files "$BATS_TEST_TMPDIR/files.manifest" keenetic
    assert_success
    assert_line --index 1 "router/opt/etc/ndm/netfilter.d/50-vpn-director.sh"
    refute_output --partial "firewall-start"
}

@test "manifest_is_executable: scripts and init files are executable" {
    load_installer
    run manifest_is_executable "router/opt/vpn-director/lib/common.sh"
    assert_success
    run manifest_is_executable "router/opt/etc/init.d/S99vpn-director"
    assert_success
    run manifest_is_executable "router/jffs/scripts/wan-event"
    assert_success
}

@test "manifest_is_executable: templates, json and the manifest are data" {
    load_installer
    run manifest_is_executable "router/opt/vpn-director/vpn-director.json.template"
    assert_failure
    run manifest_is_executable "router/opt/etc/xray/config.json.template"
    assert_failure
    run manifest_is_executable "router/opt/vpn-director/data/servers.json"
    assert_failure
    run manifest_is_executable "router/files.manifest"
    assert_failure
}

# fake_curl serves "$REPO_URL/<path>" from $BATS_TEST_TMPDIR/repo/<path>.
# install.sh calls: curl -fsSL "$url" -o "$target"
fake_curl() {
    mkdir -p "$BATS_TEST_TMPDIR/bin"
    cat > "$BATS_TEST_TMPDIR/bin/curl" <<EOF
#!/bin/bash
out=""; url=""
while [[ \$# -gt 0 ]]; do
    case "\$1" in
        -o) out="\$2"; shift 2 ;;
        -*) shift ;;
        *)  url="\$1"; shift ;;
    esac
done
src="$BATS_TEST_TMPDIR/repo/\${url#*/refs/tags/v1.0.0/}"
[[ -f "\$src" ]] || exit 22
cp "\$src" "\$out"
EOF
    chmod +x "$BATS_TEST_TMPDIR/bin/curl"
    export PATH="$BATS_TEST_TMPDIR/bin:$PATH"
}

@test "download_scripts: installs the manifest's common and platform files under INSTALL_ROOT" {
    load_installer
    fake_curl
    local repo="$BATS_TEST_TMPDIR/repo/router"
    mkdir -p "$repo/opt/vpn-director/lib" "$repo/jffs/scripts" "$repo/opt/etc/ndm/netfilter.d"
    cat > "$BATS_TEST_TMPDIR/repo/router/files.manifest" <<'EOF'
common   router/opt/vpn-director/lib/common.sh
common   router/opt/vpn-director/vpn-director.json.template
merlin   router/jffs/scripts/firewall-start
keenetic router/opt/etc/ndm/netfilter.d/50-vpn-director.sh
EOF
    echo "lib" > "$repo/opt/vpn-director/lib/common.sh"
    echo "{}" > "$repo/opt/vpn-director/vpn-director.json.template"
    echo "hook" > "$repo/jffs/scripts/firewall-start"
    echo "ndm" > "$repo/opt/etc/ndm/netfilter.d/50-vpn-director.sh"

    REPO_URL="https://raw.example/zinin/vpn-director/refs/tags/v1.0.0"
    PLATFORM="merlin"
    INSTALL_ROOT="$BATS_TEST_TMPDIR/root"

    run download_scripts
    assert_success
    [ -x "$INSTALL_ROOT/opt/vpn-director/lib/common.sh" ]
    [ -f "$INSTALL_ROOT/opt/vpn-director/vpn-director.json.template" ]
    [ ! -x "$INSTALL_ROOT/opt/vpn-director/vpn-director.json.template" ]
    [ -x "$INSTALL_ROOT/jffs/scripts/firewall-start" ]
    [ ! -e "$INSTALL_ROOT/opt/etc/ndm/netfilter.d/50-vpn-director.sh" ]
}

@test "download_scripts: fails when the manifest cannot be downloaded" {
    load_installer
    fake_curl
    mkdir -p "$BATS_TEST_TMPDIR/repo/router"
    REPO_URL="https://raw.example/zinin/vpn-director/refs/tags/v1.0.0"
    PLATFORM="merlin"
    INSTALL_ROOT="$BATS_TEST_TMPDIR/root"
    run download_scripts
    assert_failure
    assert_output --partial "files.manifest"
}

@test "manifest_files: keeps a last line that has no trailing newline" {
    load_installer
    # The manifest arrives over the network, and the Go parser reads an
    # unterminated last line; a `read` loop that drops it would make the two
    # disagree about what a release ships.
    printf 'common   router/opt/vpn-director/lib/common.sh\nmerlin   router/jffs/scripts/wan-event' \
        > "$BATS_TEST_TMPDIR/files.manifest"

    run manifest_files "$BATS_TEST_TMPDIR/files.manifest" merlin

    assert_success
    assert_line --index 0 "router/opt/vpn-director/lib/common.sh"
    # Not --index 1: bats-assert reads the array unguarded, so a missing line
    # crashes it under `set -u` instead of reporting the diff. Ordering is
    # covered by "prints common and platform paths in manifest order".
    assert_line "router/jffs/scripts/wan-event"
}

@test "manifest_files: an unknown tag fails and names the offending line" {
    load_installer
    cat > "$BATS_TEST_TMPDIR/files.manifest" <<'EOF'
common   router/opt/vpn-director/lib/common.sh
comon    router/opt/vpn-director/lib/tunnel.sh
EOF

    run manifest_files "$BATS_TEST_TMPDIR/files.manifest" merlin

    # Silently skipping the typo would drop tunnel.sh from every install, and
    # no "entry exists in the repo" check can see it: the path is fine.
    assert_failure
    assert_output --partial "line 2"
    assert_output --partial "comon"
    assert_output --partial "router/opt/vpn-director/lib/tunnel.sh"
}

@test "download_scripts: a misspelled tag aborts before installing anything" {
    load_installer
    fake_curl
    local repo="$BATS_TEST_TMPDIR/repo/router"
    mkdir -p "$repo/opt/vpn-director/lib"
    cat > "$repo/files.manifest" <<'EOF'
common   router/opt/vpn-director/lib/common.sh
comon    router/opt/vpn-director/lib/tunnel.sh
EOF
    echo "lib" > "$repo/opt/vpn-director/lib/common.sh"
    echo "tunnel" > "$repo/opt/vpn-director/lib/tunnel.sh"

    REPO_URL="https://raw.example/zinin/vpn-director/refs/tags/v1.0.0"
    PLATFORM="merlin"
    INSTALL_ROOT="$BATS_TEST_TMPDIR/root"

    run download_scripts

    assert_failure
    assert_output --partial "comon"
    # The good entry is listed first, so a parser that gave up mid-loop would
    # have written it already and left a half-installed router.
    [ ! -e "$INSTALL_ROOT/opt/vpn-director/lib/common.sh" ]
}

@test "download_scripts: fails when the manifest selects no file for the platform" {
    load_installer
    fake_curl
    mkdir -p "$BATS_TEST_TMPDIR/repo/router"
    cat > "$BATS_TEST_TMPDIR/repo/router/files.manifest" <<'EOF'
# a manifest with nothing for this platform
keenetic router/opt/etc/ndm/netfilter.d/50-vpn-director.sh
EOF
    REPO_URL="https://raw.example/zinin/vpn-director/refs/tags/v1.0.0"
    PLATFORM="merlin"
    INSTALL_ROOT="$BATS_TEST_TMPDIR/root"

    run download_scripts

    # Reporting success after installing nothing would send the installer on to
    # setup_webui_config and start_webui on top of absent scripts.
    assert_failure
    assert_output --partial "no file for platform merlin"
    refute_output --partial "Installed"
}
