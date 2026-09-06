#!/usr/bin/env bats

load '../test_helper'

# configure.sh is sourced with --source-only, so main() never runs. VPD_DIR must
# point at the repository copy while sourcing (the script loads lib/xrayconf.sh
# from it) and is redirected into BATS_TEST_TMPDIR afterwards.
load_wizard() {
    VPD_DIR="$SCRIPTS_DIR"
    source "$SCRIPTS_DIR/configure.sh" --source-only

    VPD_DIR="$BATS_TEST_TMPDIR/vpn-director"
    XRAY_CONFIG_DIR="$BATS_TEST_TMPDIR/xray"
    mkdir -p "$VPD_DIR" "$XRAY_CONFIG_DIR"
    cp "$SCRIPTS_DIR/vpn-director.json.template" "$VPD_DIR/vpn-director.json.template"
    cp "$PROJECT_ROOT/opt/etc/xray/config.json.template" "$XRAY_CONFIG_DIR/config.json.template"

    # The answers the wizard collected in steps 1-3.
    SELECTED_SERVER_JSON='{"address":"1.2.3.4","port":443,"uuid":"u1","security":"reality","network":"tcp","flow":"xtls-rprx-vision","sni":"cdn.example.com","fingerprint":"firefox","public_key":"PBK","short_id":"sid1"}'
    XRAY_CLIENTS_LIST="192.168.50.10"
    XRAY_EXCLUDE_SETS_LIST="ru"
    TUN_DIR_TUNNELS_JSON='{"wgc1":{"clients":["192.168.50.20"],"exclude":["ru"]}}'
    SERVERS_FILE="$BATS_TEST_TMPDIR/servers.json"
    printf '%s' '[{"address":"1.2.3.4","ips":["1.2.3.4"]}]' > "$SERVERS_FILE"
}

# write_daemon_config stands in for the config as the daemons left it: a
# generated jwt_secret, a paused client, an excluded IP, and stale wizard fields.
write_daemon_config() {
    jq '.webui.jwt_secret = "secret-from-the-daemon" |
        .webui.log_level = "debug" |
        .paused_clients = ["192.168.50.30"] |
        .xray.exclude_ips = ["203.0.113.7"] |
        .xray.clients = ["10.0.0.9"] |
        .xray.exclude_sets = ["de"] |
        .tunnel_director.tunnels = {"ovpnc1": {"clients": ["10.0.0.9"], "exclude": []}}' \
        "$VPD_DIR/vpn-director.json.template" > "$VPD_DIR/vpn-director.json"
}

@test "step_generate_configs: keeps the jwt_secret the Web UI generated" {
    load_wizard
    write_daemon_config

    run step_generate_configs

    assert_success
    run jq -r '.webui.jwt_secret' "$VPD_DIR/vpn-director.json"
    assert_output "secret-from-the-daemon"
}

@test "step_generate_configs: keeps the rest of the webui section" {
    load_wizard
    write_daemon_config

    run step_generate_configs

    assert_success
    run jq -r '.webui.log_level' "$VPD_DIR/vpn-director.json"
    assert_output "debug"
}

@test "step_generate_configs: keeps paused_clients and exclude_ips" {
    load_wizard
    write_daemon_config

    run step_generate_configs

    assert_success
    run jq -c '[.paused_clients, .xray.exclude_ips]' "$VPD_DIR/vpn-director.json"
    assert_output '[["192.168.50.30"],["203.0.113.7"]]'
}

@test "step_generate_configs: still overwrites the fields the wizard owns" {
    load_wizard
    write_daemon_config

    run step_generate_configs

    assert_success
    run jq -c '[.xray.clients, .xray.exclude_sets, (.tunnel_director.tunnels | keys)]' \
        "$VPD_DIR/vpn-director.json"
    assert_output '[["192.168.50.10"],["ru"],["wgc1"]]'
}

@test "step_generate_configs: falls back to template defaults on a first run" {
    load_wizard

    run step_generate_configs

    assert_success
    run jq -r '[.webui.jwt_secret, (.paused_clients // "absent" | tostring)] | join("|")' \
        "$VPD_DIR/vpn-director.json"
    assert_output "|absent"
}

@test "step_generate_configs: writes the config with mode 600" {
    load_wizard
    write_daemon_config
    chmod 644 "$VPD_DIR/vpn-director.json"

    run step_generate_configs

    assert_success
    run stat -c '%a' "$VPD_DIR/vpn-director.json"
    assert_output "600"
}

@test "step_generate_configs: keeps data_dir and the advanced section" {
    load_wizard
    write_daemon_config
    jq '.data_dir = "/tmp/moved-storage" | .advanced.xray.tproxy_port = 23456' \
        "$VPD_DIR/vpn-director.json" > "$VPD_DIR/vpn-director.json.new"
    mv "$VPD_DIR/vpn-director.json.new" "$VPD_DIR/vpn-director.json"

    run step_generate_configs

    assert_success
    run jq -r '[.data_dir, (.advanced.xray.tproxy_port | tostring)] | join("|")' \
        "$VPD_DIR/vpn-director.json"
    assert_output "/tmp/moved-storage|23456"
}

@test "step_generate_configs: picks up a key an update added to the template" {
    load_wizard
    write_daemon_config
    jq '. + {"new_setting": "from-the-template"}' \
        "$VPD_DIR/vpn-director.json.template" > "$VPD_DIR/tpl.new"
    mv "$VPD_DIR/tpl.new" "$VPD_DIR/vpn-director.json.template"

    run step_generate_configs

    assert_success
    run jq -r '.new_setting' "$VPD_DIR/vpn-director.json"
    assert_output "from-the-template"
}

@test "step_generate_configs: refuses while another writer holds the config lock" {
    load_wizard
    write_daemon_config
    flock "$VPD_DIR/.vpn-director.json.lock" sleep 30 &
    local holder=$!
    sleep 0.5

    VPD_CONFIG_LOCK_WAIT=1
    run step_generate_configs
    kill "$holder" 2>/dev/null || true

    assert_failure
    assert_output --partial "Config is locked"
    run jq -r '.xray.clients[0]' "$VPD_DIR/vpn-director.json"
    assert_output "10.0.0.9"
}

@test "step_generate_configs: releases the config lock when it is done" {
    load_wizard
    write_daemon_config

    run step_generate_configs

    assert_success
    run flock -n "$VPD_DIR/.vpn-director.json.lock" true
    assert_success
}

@test "step_generate_configs: a broken template leaves the existing config alone" {
    load_wizard
    write_daemon_config
    printf '%s' 'not json' > "$VPD_DIR/vpn-director.json.template"

    run step_generate_configs

    assert_failure
    run jq -r '.webui.jwt_secret' "$VPD_DIR/vpn-director.json"
    assert_output "secret-from-the-daemon"
    [[ -z "$(find "$VPD_DIR" -name 'vpn-director.json.??????')" ]]
}
