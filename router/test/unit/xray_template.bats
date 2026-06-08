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
