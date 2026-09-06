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
