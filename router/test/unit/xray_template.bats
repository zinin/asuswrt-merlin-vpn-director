#!/usr/bin/env bats
load '../test_helper'

@test "config.json.template is valid JSON" {
    run jq empty "$PROJECT_ROOT/opt/etc/xray/config.json.template"
    [ "$status" -eq 0 ]
}

@test "config.json.template has empty outbounds (filled by generator)" {
    run jq -e '.outbounds == []' "$PROJECT_ROOT/opt/etc/xray/config.json.template"
    [ "$status" -eq 0 ]
}

@test "config.json.template keeps both inbounds (tproxy-in, socks-in)" {
    run jq -e '.inbounds | length == 2' "$PROJECT_ROOT/opt/etc/xray/config.json.template"
    [ "$status" -eq 0 ]
}

@test "config.json.template writes Xray errors to /tmp/xray-error.log and disables the access log" {
    run jq -r '.log.error' "$PROJECT_ROOT/opt/etc/xray/config.json.template"
    [ "$status" -eq 0 ]
    [ "$output" = "/tmp/xray-error.log" ]
    run jq -r '.log.access' "$PROJECT_ROOT/opt/etc/xray/config.json.template"
    [ "$output" = "none" ]
    run jq -r '.log.loglevel' "$PROJECT_ROOT/opt/etc/xray/config.json.template"
    [ "$output" = "warning" ]
}
