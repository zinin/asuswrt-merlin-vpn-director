#!/usr/bin/env bats

load '../test_helper'

# install.sh is sourced with --source-only, so main() never runs. The paths it
# writes to are redirected into BATS_TEST_TMPDIR by overriding the globals it
# declares at the top of the file.
load_installer() {
    source "$PROJECT_ROOT/../install.sh" --source-only
    VPD_DIR="$BATS_TEST_TMPDIR/vpn-director"
    INIT_DIR="$BATS_TEST_TMPDIR/init.d"
    # create_directories also mkdirs XRAY_CONFIG_DIR; /opt/etc is not writable here.
    XRAY_CONFIG_DIR="$BATS_TEST_TMPDIR/xray"
    mkdir -p "$VPD_DIR" "$INIT_DIR"
    WEBUI_INIT="$INIT_DIR/S98vpn-director-webui"
    # What download_scripts installs before the LAN facts are needed.
    cp -r "$SCRIPTS_DIR/lib" "$VPD_DIR/lib"
    PLATFORM=merlin
    load_platform_lib
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

@test "manifest_files: an invalid path fails and names the offending line" {
    load_installer
    cat > "$BATS_TEST_TMPDIR/files.manifest" <<'EOF'
common   router/opt/vpn-director/lib/common.sh
common   router/opt/../../etc/passwd
EOF

    run manifest_files "$BATS_TEST_TMPDIR/files.manifest" merlin

    # The path is pasted into "${INSTALL_ROOT}/${file#router/}", so a traversal
    # would install outside the root. The Go parser refuses the same shapes.
    assert_failure
    assert_output --partial "line 2"
    assert_output --partial "router/opt/../../etc/passwd"

    printf 'common   /etc/passwd\n' > "$BATS_TEST_TMPDIR/files.manifest"

    run manifest_files "$BATS_TEST_TMPDIR/files.manifest" merlin

    assert_failure
    assert_output --partial "/etc/passwd"
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

# ============================================================================
# Platform detection and the Keenetic prerequisites
# ============================================================================

@test "detect_platform: keenetic, merlin, or a refusal" {
    load_installer
    local root="$BATS_TEST_TMPDIR/root"
    mkdir -p "$root/opt/etc/ndm" "$root/bin"
    printf '#!/bin/sh\n' > "$root/bin/ndmc"
    chmod +x "$root/bin/ndmc"
    VPD_PROBE_ROOT="$root" detect_platform
    assert_equal "$PLATFORM" keenetic

    root="$BATS_TEST_TMPDIR/root2"
    mkdir -p "$root/jffs" "$root/bin"
    printf '#!/bin/sh\n' > "$root/bin/nvram"
    chmod +x "$root/bin/nvram"
    VPD_PROBE_ROOT="$root" detect_platform
    assert_equal "$PLATFORM" merlin

    mkdir -p "$BATS_TEST_TMPDIR/empty"
    VPD_PROBE_ROOT="$BATS_TEST_TMPDIR/empty" run detect_platform
    assert_failure
    assert_output --partial "Unsupported platform"
}

# fake_opkg <installed...> answers "opkg status <pkg>" for the named packages
# and records every "opkg install".
fake_opkg() {
    mkdir -p "$BATS_TEST_TMPDIR/bin"
    cat > "$BATS_TEST_TMPDIR/bin/opkg" <<EOF
#!/bin/bash
installed=" $* "
case "\$1" in
    status)  [[ "\$installed" == *" \$2 "* ]] && echo "Status: install user installed"; exit 0 ;;
    update)  exit 0 ;;
    install) shift; echo "install \$*" >> "$BATS_TEST_TMPDIR/opkg.log"; exit 0 ;;
esac
EOF
    chmod +x "$BATS_TEST_TMPDIR/bin/opkg"
    export PATH="$BATS_TEST_TMPDIR/bin:$PATH"
}

@test "check_keenetic_prerequisites: refuses without the TPROXY kernel module and names the component" {
    load_installer
    export VPD_MODULES_DIR="$BATS_TEST_TMPDIR/modules"
    mkdir -p "$VPD_MODULES_DIR"
    run check_keenetic_prerequisites
    assert_failure
    assert_output --partial "xt_TPROXY.ko not found"
    assert_output --partial "Kernel modules for Netfilter"
}

@test "check_keenetic_prerequisites: passes when the module and every package are there" {
    load_installer
    export VPD_MODULES_DIR="$BATS_TEST_TMPDIR/modules"
    mkdir -p "$VPD_MODULES_DIR"
    : > "$VPD_MODULES_DIR/xt_TPROXY.ko"
    fake_opkg $KEENETIC_PACKAGES
    run check_keenetic_prerequisites
    assert_success
    assert_output --partial "Required Entware packages are installed"
}

@test "check_keenetic_prerequisites: without a terminal it prints the opkg command and exits 1" {
    load_installer
    export VPD_MODULES_DIR="$BATS_TEST_TMPDIR/modules"
    mkdir -p "$VPD_MODULES_DIR"
    : > "$VPD_MODULES_DIR/xt_TPROXY.ko"
    fake_opkg bash curl jq iptables ipset ip-full
    INSTALL_TTY="$BATS_TEST_TMPDIR/no-tty" run check_keenetic_prerequisites
    assert_failure
    assert_output --partial "opkg update && opkg install flock coreutils-nohup"
    assert_output --partial " xray"
    [ ! -e "$BATS_TEST_TMPDIR/opkg.log" ]
}

@test "check_keenetic_prerequisites: a terminal that yields no answer prints the opkg command and exits 1" {
    load_installer
    export VPD_MODULES_DIR="$BATS_TEST_TMPDIR/modules"
    mkdir -p "$VPD_MODULES_DIR"
    : > "$VPD_MODULES_DIR/xt_TPROXY.ko"
    fake_opkg bash curl jq iptables ipset ip-full
    : > "$BATS_TEST_TMPDIR/silent-tty"       # a read that yields nothing must refuse, not default to yes
    INSTALL_TTY="$BATS_TEST_TMPDIR/silent-tty" run check_keenetic_prerequisites
    assert_failure
    assert_output --partial "opkg update && opkg install"
    [ ! -e "$BATS_TEST_TMPDIR/opkg.log" ]
}

@test "check_keenetic_prerequisites: on a terminal installs the missing packages after a yes" {
    load_installer
    export VPD_MODULES_DIR="$BATS_TEST_TMPDIR/modules"
    mkdir -p "$VPD_MODULES_DIR"
    : > "$VPD_MODULES_DIR/xt_TPROXY.ko"
    fake_opkg bash curl jq iptables ipset ip-full flock coreutils-nohup coreutils-base64 coreutils-sha256sum gawk procps-ng-pgrep procps-ng-pkill procps-ng-ps openssl-util
    printf '\n' > "$BATS_TEST_TMPDIR/tty"   # Enter = the default, yes
    INSTALL_TTY="$BATS_TEST_TMPDIR/tty" run check_keenetic_prerequisites
    assert_success
    run cat "$BATS_TEST_TMPDIR/opkg.log"
    assert_output "install cron xray"
}

@test "check_keenetic_prerequisites: a no on the terminal prints the command and exits 1" {
    load_installer
    export VPD_MODULES_DIR="$BATS_TEST_TMPDIR/modules"
    mkdir -p "$VPD_MODULES_DIR"
    : > "$VPD_MODULES_DIR/xt_TPROXY.ko"
    fake_opkg bash
    printf 'n\n' > "$BATS_TEST_TMPDIR/tty"
    INSTALL_TTY="$BATS_TEST_TMPDIR/tty" run check_keenetic_prerequisites
    assert_failure
    assert_output --partial "opkg update && opkg install"
    [ ! -e "$BATS_TEST_TMPDIR/opkg.log" ]
}

@test "release_arch: aarch64, armv7l and mips map to release asset suffixes" {
    load_installer
    mkdir -p "$BATS_TEST_TMPDIR/bin"
    for m in aarch64 armv7l mips x86_64; do
        printf '#!/bin/bash\necho %s\n' "$m" > "$BATS_TEST_TMPDIR/bin/uname"
        chmod +x "$BATS_TEST_TMPDIR/bin/uname"
        PATH="$BATS_TEST_TMPDIR/bin:$PATH" run release_arch
        case "$m" in
            aarch64) assert_output arm64 ;;
            armv7l)  assert_output arm ;;
            mips)    assert_output mipsle ;;
            x86_64)  assert_failure; refute_output ;;
        esac
    done
}

@test "create_directories: hook directories per platform, under INSTALL_ROOT" {
    load_installer
    INSTALL_ROOT="$BATS_TEST_TMPDIR/root"
    PLATFORM=merlin run create_directories
    assert_success
    [ -d "$INSTALL_ROOT/jffs/scripts" ]
    [ ! -d "$INSTALL_ROOT/opt/etc/ndm" ]

    INSTALL_ROOT="$BATS_TEST_TMPDIR/root2"
    PLATFORM=keenetic run create_directories
    assert_success
    [ -d "$INSTALL_ROOT/opt/etc/ndm/netfilter.d" ]
    [ -d "$INSTALL_ROOT/opt/etc/ndm/wan.d" ]
    [ -d "$INSTALL_ROOT/opt/etc/ndm/iflayerchanged.d" ]
    [ ! -d "$INSTALL_ROOT/opt/etc/ndm/ifstatechanged.d" ]
    [ -d "$INSTALL_ROOT/opt/etc/cron.d" ]
    [ ! -d "$INSTALL_ROOT/jffs" ]
}

@test "lan_ip and lan_hostname: from the installed platform library, with defaults" {
    load_installer
    PLATFORM=merlin
    load_platform_lib
    run lan_ip
    assert_output "192.168.50.1"
    run lan_hostname
    assert_output "RT-AX88U-1234"
    NVRAM_NO_LAN_IP=1 run lan_ip
    assert_output "192.168.1.1"
}

@test "print_next_steps: names the login per platform" {
    load_installer
    RELEASE_TAG=v1.0.0
    WEBUI_URL="https://192.168.1.1:8444"
    PLATFORM=keenetic run print_next_steps
    assert_output --partial "root"
    assert_output --partial "Entware password"
    PLATFORM=merlin run print_next_steps
    assert_output --partial "router admin username and password"
}
