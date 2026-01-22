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
