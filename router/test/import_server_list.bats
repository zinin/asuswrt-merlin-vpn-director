#!/usr/bin/env bats

load 'test_helper'

# Test URI for basic parsing (ASCII name, no special chars)
TEST_URI_BASIC='vless://11111111-2222-3333-4444-555555555555@server1.test.example:8443?type=tcp&security=tls#Prague, Czechia'

# URI with emoji flag + cyrillic name
TEST_URI_EMOJI_CYRILLIC='vless://11111111-2222-3333-4444-555555555555@server2.test.example:8443?type=tcp&security=tls#🇷🇺 Россия, Москва'

# URI with only emoji (should fallback to hostname)
TEST_URI_EMOJI_ONLY='vless://11111111-2222-3333-4444-555555555555@server3.test.example:8443?type=tcp&security=tls#🇺🇸🌟✨'

# URI with URL-encoded spaces
TEST_URI_URLENCODED='vless://11111111-2222-3333-4444-555555555555@server4.test.example:8443?type=tcp&security=tls#New%20York%20City'

# URI with cyrillic only (no emoji)
TEST_URI_CYRILLIC='vless://11111111-2222-3333-4444-555555555555@server5.test.example:8443?type=tcp&security=tls#Казахстан, Алматы'

# ============================================================================
# parse_vless_uri: Field extraction
# ============================================================================

@test "parse_vless_uri: extracts server hostname" {
    load_import_server_list
    result=$(parse_vless_uri "$TEST_URI_BASIC")
    server=$(printf '%s' "$result" | cut -d'|' -f1)
    [ "$server" = "server1.test.example" ]
}

@test "parse_vless_uri: extracts port number" {
    load_import_server_list
    result=$(parse_vless_uri "$TEST_URI_BASIC")
    port=$(printf '%s' "$result" | cut -d'|' -f2)
    [ "$port" = "8443" ]
}

@test "parse_vless_uri: extracts UUID" {
    load_import_server_list
    result=$(parse_vless_uri "$TEST_URI_BASIC")
    uuid=$(printf '%s' "$result" | cut -d'|' -f3)
    [ "$uuid" = "11111111-2222-3333-4444-555555555555" ]
}

@test "parse_vless_uri: extracts ASCII name" {
    load_import_server_list
    result=$(parse_vless_uri "$TEST_URI_BASIC")
    name=$(printf '%s' "$result" | cut -d'|' -f4)
    [ "$name" = "Prague, Czechia" ]
}

@test "parse_vless_uri extracts reality stream params" {
    load_import_server_list
    # Real subscription format includes headerType=none before type=tcp — guards the
    # `_vless_query_get type` lookup against matching `headerType`.
    uri='vless://uuid@1.2.3.4:443?security=reality&encryption=none&fp=firefox&headerType=none&type=tcp&flow=xtls-rprx-vision&sni=cdn3-87.yahoo.com&pbk=PBKEY&sid=55e6#NL'
    run parse_vless_uri "$uri"
    [ "$status" -eq 0 ]
    # fields: server|port|uuid|name|security|network|flow|sni|fp|pbk|sid|alpn
    [ "$(printf '%s' "$output" | cut -d'|' -f5)" = "reality" ]
    [ "$(printf '%s' "$output" | cut -d'|' -f6)" = "tcp" ]
    [ "$(printf '%s' "$output" | cut -d'|' -f7)" = "xtls-rprx-vision" ]
    [ "$(printf '%s' "$output" | cut -d'|' -f8)" = "cdn3-87.yahoo.com" ]
    [ "$(printf '%s' "$output" | cut -d'|' -f9)" = "firefox" ]
    [ "$(printf '%s' "$output" | cut -d'|' -f10)" = "PBKEY" ]
    [ "$(printf '%s' "$output" | cut -d'|' -f11)" = "55e6" ]
}

@test "parse_vless_uri keeps core fields without params" {
    load_import_server_list
    run parse_vless_uri 'vless://uuid@1.2.3.4:443#Name'
    [ "$status" -eq 0 ]
    [ "$(printf '%s' "$output" | cut -d'|' -f1)" = "1.2.3.4" ]
    [ "$(printf '%s' "$output" | cut -d'|' -f3)" = "uuid" ]
    [ "$(printf '%s' "$output" | cut -d'|' -f5)" = "" ]
}

# ============================================================================
# parse_vless_uri: Name handling
# ============================================================================

@test "parse_vless_uri: filters emoji from name, keeps cyrillic" {
    load_import_server_list
    result=$(parse_vless_uri "$TEST_URI_EMOJI_CYRILLIC")
    name=$(printf '%s' "$result" | cut -d'|' -f4)
    # Emoji flag should be removed, cyrillic preserved
    [ "$name" = "Россия, Москва" ]
}

@test "parse_vless_uri: falls back to hostname when name is only emoji" {
    load_import_server_list
    result=$(parse_vless_uri "$TEST_URI_EMOJI_ONLY")
    name=$(printf '%s' "$result" | cut -d'|' -f4)
    # All emoji filtered out, should fallback to server hostname
    [ "$name" = "server3.test.example" ]
}

@test "parse_vless_uri: decodes URL-encoded spaces" {
    load_import_server_list
    result=$(parse_vless_uri "$TEST_URI_URLENCODED")
    name=$(printf '%s' "$result" | cut -d'|' -f4)
    [ "$name" = "New York City" ]
}

@test "parse_vless_uri: handles cyrillic-only name" {
    load_import_server_list
    result=$(parse_vless_uri "$TEST_URI_CYRILLIC")
    name=$(printf '%s' "$result" | cut -d'|' -f4)
    [ "$name" = "Казахстан, Алматы" ]
}

# ============================================================================
# decode_vless_content: Format detection
# ============================================================================

@test "decode_vless_content: detects plaintext format (single URI)" {
    load_import_server_list
    content="vless://uuid@server:443?type=tcp#Name"
    result=$(decode_vless_content "$content")
    [ "$result" = "$content" ]
}

@test "decode_vless_content: detects plaintext format (multiple URIs)" {
    load_import_server_list
    content="vless://uuid1@server1:443?type=tcp#Name1
vless://uuid2@server2:443?type=tcp#Name2"
    result=$(decode_vless_content "$content")
    [ "$result" = "$content" ]
}

@test "decode_vless_content: handles plaintext with leading empty lines" {
    load_import_server_list
    content="

vless://uuid@server:443?type=tcp#Name"
    result=$(decode_vless_content "$content")
    [ "$result" = "$content" ]
}

@test "decode_vless_content: decodes base64 format" {
    load_import_server_list
    plaintext="vless://uuid@server:443?type=tcp#Name"
    encoded=$(printf '%s' "$plaintext" | base64)
    result=$(decode_vless_content "$encoded")
    [ "$result" = "$plaintext" ]
}

@test "decode_vless_content: decodes base64 with multiple URIs" {
    load_import_server_list
    plaintext="vless://uuid1@server1:443#Name1
vless://uuid2@server2:443#Name2"
    encoded=$(printf '%s' "$plaintext" | base64)
    result=$(decode_vless_content "$encoded")
    [ "$result" = "$plaintext" ]
}

@test "decode_vless_content: fails on invalid content" {
    load_import_server_list
    run decode_vless_content "not-base64-and-not-vless!!!"
    assert_failure
}

@test "decode_vless_content: fails on whitespace-only content" {
    load_import_server_list
    run decode_vless_content "

    "
    assert_failure
}

@test "decode_vless_content: decodes valid base64 even if not VLESS (validation is downstream)" {
    load_import_server_list
    plaintext="just some random text"
    encoded=$(printf '%s' "$plaintext" | base64)
    result=$(decode_vless_content "$encoded")
    # Function succeeds - content validation is handled downstream
    [ "$result" = "$plaintext" ]
}

@test "decode_vless_content: decodes URL-safe base64 alphabet" {
    load_import_server_list
    # "????" standard base64 is "Pz8/Pw=="; the URL-safe form replaces / with _.
    # The standard decoder rejects _, so this exercises the url-safe fallback.
    # Use command substitution (not run) so the log line on stderr is excluded.
    result=$(decode_vless_content "Pz8_Pw==")
    [ "$result" = "????" ]
}

# ============================================================================
# parse_vless_uri: IPv6 literal host
# ============================================================================

@test "parse_vless_uri: parses bracketed IPv6 host and port" {
    load_import_server_list
    run parse_vless_uri 'vless://uuid@[2001:db8::1]:443?type=tcp#v6'
    [ "$status" -eq 0 ]
    [ "$(printf '%s' "$output" | cut -d'|' -f1)" = "2001:db8::1" ]
    [ "$(printf '%s' "$output" | cut -d'|' -f2)" = "443" ]
}

# ============================================================================
# _redact_uri: mask UUID for DEBUG logging
# ============================================================================

@test "_redact_uri: masks UUID and strips fragment, keeps host" {
    load_import_server_list
    run _redact_uri 'vless://11111111-2222-3333-4444-555555555555@server.example:443?type=tcp#MyName'
    [ "$status" -eq 0 ]
    # UUID is a secret and must not leak into logs
    [[ "$output" != *11111111-2222-3333-4444-555555555555* ]]
    # host:port and params are kept; the #fragment is stripped
    [[ "$output" == *server.example:443* ]]
    [[ "$output" != *MyName* ]]
}

# ============================================================================
# _url_decode / _vless_query_get: percent-decoding parity with url.ParseQuery
# ============================================================================

@test "_url_decode: decodes %XX escapes" {
    load_import_server_list
    result=$(_url_decode 'h2%2Chttp/1.1')
    [ "$result" = "h2,http/1.1" ]
}

@test "_url_decode: maps + to space, leaves lone/incomplete % intact" {
    load_import_server_list
    [ "$(_url_decode 'a+b')" = "a b" ]
    [ "$(_url_decode '50%')" = "50%" ]
    [ "$(_url_decode 'x%2y')" = "x%2y" ]
}

@test "_vless_query_get: URL-decodes the value (parity with url.ParseQuery)" {
    load_import_server_list
    result=$(_vless_query_get 'type=tcp&alpn=h2%2Chttp/1.1' alpn)
    [ "$result" = "h2,http/1.1" ]
}

@test "parse_vless_uri: URL-decodes percent-encoded alpn into comma list" {
    load_import_server_list
    result=$(parse_vless_uri 'vless://uuid@1.2.3.4:443?type=tcp&alpn=h2%2Chttp/1.1#N')
    [ "$(printf '%s' "$result" | cut -d'|' -f12)" = "h2,http/1.1" ]
}

# ============================================================================
# step_parse_and_save_servers: JSON output
# ============================================================================

@test "step_parse_and_save_servers: saves ips array instead of ip" {
    load_import_server_list

    DATA_DIR="/tmp/bats_test_import_data"
    SERVERS_FILE="$DATA_DIR/servers.json"
    mkdir -p "$DATA_DIR"

    # Override VPD_CONFIG to a temp config with our data_dir
    VPD_CONFIG="/tmp/bats_test_import_data/vpn-director.json"
    printf '{"data_dir": "%s"}\n' "$DATA_DIR" > "$VPD_CONFIG"

    VLESS_SERVERS="vless://test-uuid@example.com:443?type=tcp#TestServer"

    step_parse_and_save_servers

    # Check that servers.json has "ips" array, not "ip" string
    result=$(jq -r '.[0].ips | type' "$SERVERS_FILE")
    [ "$result" = "array" ]

    # Check that "ip" field does not exist
    result=$(jq -r '.[0] | has("ip")' "$SERVERS_FILE")
    [ "$result" = "false" ]

    # Check the resolved IP is in the ips array
    result=$(jq -r '.[0].ips[0]' "$SERVERS_FILE")
    [ "$result" = "93.184.216.34" ]

    rm -rf "$DATA_DIR"
}

@test "step_parse_and_save_servers writes reality params to servers.json" {
    load_import_server_list

    tmp_data="$BATS_TEST_TMPDIR/data"
    mkdir -p "$tmp_data"
    # get_data_dir reads VPD_CONFIG/VPD_TEMPLATE; override to a temp config
    cfg="$BATS_TEST_TMPDIR/vpn-director.json"
    printf '{"data_dir":"%s"}' "$tmp_data" > "$cfg"
    VPD_CONFIG="$cfg"
    VLESS_SERVERS='vless://uuid@1.2.3.4:443?security=reality&flow=xtls-rprx-vision&sni=cdn.example.com&pbk=PBK&sid=sid1&type=tcp#NL'
    run step_parse_and_save_servers
    [ "$status" -eq 0 ]
    out="$tmp_data/servers.json"
    [ "$(jq -r '.[0].security' "$out")" = "reality" ]
    [ "$(jq -r '.[0].flow' "$out")" = "xtls-rprx-vision" ]
    [ "$(jq -r '.[0].public_key' "$out")" = "PBK" ]
    [ "$(jq -r '.[0].short_id' "$out")" = "sid1" ]
    [ "$(jq -r '.[0].sni' "$out")" = "cdn.example.com" ]
    [ "$(jq -r '.[0] | has("alpn")' "$out")" = "false" ]
}

@test "step_parse_and_save_servers skips out-of-range port, keeps valid server" {
    load_import_server_list

    tmp_data="$BATS_TEST_TMPDIR/data"
    mkdir -p "$tmp_data"
    cfg="$BATS_TEST_TMPDIR/vpn-director.json"
    printf '{"data_dir":"%s"}' "$tmp_data" > "$cfg"
    VPD_CONFIG="$cfg"
    # First URI has an out-of-range port (must be skipped); the second is valid
    # and must still be saved (one bad entry does not drop the rest).
    VLESS_SERVERS=$'vless://uuid@1.2.3.4:99999?type=tcp#Bad\nvless://uuid@5.6.7.8:443?type=tcp#Good'
    run step_parse_and_save_servers
    [ "$status" -eq 0 ]
    out="$tmp_data/servers.json"
    [ "$(jq length "$out")" -eq 1 ]
    [ "$(jq -r '.[0].address' "$out")" = "5.6.7.8" ]
    [ "$(jq -r '.[0].port' "$out")" -eq 443 ]
}
