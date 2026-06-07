#!/usr/bin/env bats
load '../test_helper'

setup() {
    source "$LIB_DIR/xrayconf.sh"
}

@test "build_outbound: reality -> realitySettings + flow" {
    server='{"address":"1.2.3.4","port":443,"uuid":"u1","security":"reality","network":"tcp","flow":"xtls-rprx-vision","sni":"cdn.example.com","fingerprint":"firefox","public_key":"PBK","short_id":"sid1"}'
    run xrayconf_build_outbound <<< "$server"
    [ "$status" -eq 0 ]
    [ "$(printf '%s' "$output" | jq -r '.streamSettings.security')" = "reality" ]
    [ "$(printf '%s' "$output" | jq -r '.streamSettings.realitySettings.publicKey')" = "PBK" ]
    [ "$(printf '%s' "$output" | jq -r '.streamSettings.realitySettings.serverName')" = "cdn.example.com" ]
    [ "$(printf '%s' "$output" | jq -r '.settings.vnext[0].users[0].flow')" = "xtls-rprx-vision" ]
}

@test "build_outbound: tls -> tlsSettings serverName from sni" {
    server='{"address":"1.2.3.4","port":443,"uuid":"u1","security":"tls","sni":"host.example.com","fingerprint":"chrome"}'
    run xrayconf_build_outbound <<< "$server"
    [ "$status" -eq 0 ]
    [ "$(printf '%s' "$output" | jq -r '.streamSettings.security')" = "tls" ]
    [ "$(printf '%s' "$output" | jq -r '.streamSettings.tlsSettings.serverName')" = "host.example.com" ]
}

@test "build_outbound: legacy (no security) -> tls alpn h2, no flow" {
    server='{"address":"example.com","port":443,"uuid":"u1"}'
    run xrayconf_build_outbound <<< "$server"
    [ "$status" -eq 0 ]
    [ "$(printf '%s' "$output" | jq -r '.streamSettings.security')" = "tls" ]
    [ "$(printf '%s' "$output" | jq -r '.streamSettings.tlsSettings.serverName')" = "example.com" ]
    [ "$(printf '%s' "$output" | jq -r '.streamSettings.tlsSettings.alpn[0]')" = "h2" ]
    [ "$(printf '%s' "$output" | jq -r '.settings.vnext[0].users[0] | has("flow")')" = "false" ]
}

@test "build_outbound: unsupported network -> error" {
    server='{"address":"1.2.3.4","port":443,"uuid":"u1","security":"reality","network":"ws","sni":"s","public_key":"PBK","short_id":"sid"}'
    run xrayconf_build_outbound <<< "$server"
    [ "$status" -ne 0 ]
}

@test "build_outbound: unsupported security -> error" {
    server='{"address":"1.2.3.4","port":443,"uuid":"u1","security":"xtls"}'
    run xrayconf_build_outbound <<< "$server"
    [ "$status" -ne 0 ]
}

@test "generate: replaces outbounds and preserves inbounds/routing" {
    tmpl="$BATS_TEST_TMPDIR/t.json"
    printf '%s' '{"inbounds":[{"tag":"tproxy-in"},{"tag":"socks-in"}],"outbounds":[],"routing":{"rules":[{"outboundTag":"proxy-out"}]}}' > "$tmpl"
    server='{"address":"1.2.3.4","port":443,"uuid":"u1","security":"reality","sni":"s","public_key":"PBK","short_id":"sid"}'
    run xrayconf_generate "$tmpl" <<< "$server"
    [ "$status" -eq 0 ]
    [ "$(printf '%s' "$output" | jq -r '.outbounds[0].streamSettings.security')" = "reality" ]
    [ "$(printf '%s' "$output" | jq -r '.outbounds | length')" = "1" ]
    # inbounds/routing from the template must be preserved untouched
    [ "$(printf '%s' "$output" | jq -r '.inbounds | length')" = "2" ]
    [ "$(printf '%s' "$output" | jq -r '.routing.rules[0].outboundTag')" = "proxy-out" ]
}
