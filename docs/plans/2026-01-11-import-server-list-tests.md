# import_server_list.sh Unit Tests Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add unit tests for `parse_vless_uri` function covering emoji filtering, cyrillic handling, and URL decoding.

**Architecture:** Bats tests with fixture containing 5 anonymized VLESS URIs. Script modified to skip `main()` when `IMPORT_TEST_MODE=1`. Tests verify field extraction and name sanitization.

**Tech Stack:** Bats, bats-assert, bats-support, gawk (for emoji filtering in script)

---

## Task 1: Enable Test Mode in Script

**Files:**
- Modify: `jffs/scripts/vpn-director/import_server_list.sh:301`

**Step 1: Add IMPORT_TEST_MODE check**

Replace line 301 (`main "$@"`) with:

```bash
if [[ "${IMPORT_TEST_MODE:-0}" != "1" ]]; then
    main "$@"
fi
```

**Step 2: Verify script still runs normally**

Run:
```bash
cd /opt/github/zinin/asuswrt-merlin-vpn-director
bash -n jffs/scripts/vpn-director/import_server_list.sh
```
Expected: No syntax errors

**Step 3: Commit**

```bash
git add jffs/scripts/vpn-director/import_server_list.sh
git commit -m "$(cat <<'EOF'
refactor: add IMPORT_TEST_MODE for unit testing

Allows sourcing script without executing main() when IMPORT_TEST_MODE=1.
EOF
)"
```

---

## Task 2: Add Test Helper Function

**Files:**
- Modify: `test/test_helper.bash:48`

**Step 1: Add load_import_server_list helper**

Append to end of `test/test_helper.bash`:

```bash

# Helper to source import_server_list.sh without running main
load_import_server_list() {
    load_common
    export IMPORT_TEST_MODE=1
    source "$SCRIPTS_DIR/import_server_list.sh"
}
```

**Step 2: Verify syntax**

Run:
```bash
bash -n test/test_helper.bash
```
Expected: No errors

**Step 3: Commit**

```bash
git add test/test_helper.bash
git commit -m "$(cat <<'EOF'
test: add load_import_server_list helper
EOF
)"
```

---

## Task 3: Create Test Fixture

**Files:**
- Create: `test/fixtures/vless_servers.b64`

**Step 1: Create raw VLESS URIs file**

Create temporary file with 5 test URIs (one per line):

```
vless://11111111-2222-3333-4444-555555555555@server1.test.example:8443?type=tcp&security=tls#Prague, Czechia
vless://11111111-2222-3333-4444-555555555555@server2.test.example:8443?type=tcp&security=tls#🇷🇺 Россия, Москва
vless://11111111-2222-3333-4444-555555555555@server3.test.example:8443?type=tcp&security=tls#🇺🇸🌟✨
vless://11111111-2222-3333-4444-555555555555@server4.test.example:8443?type=tcp&security=tls#New%20York%20City
vless://11111111-2222-3333-4444-555555555555@server5.test.example:8443?type=tcp&security=tls#Казахстан, Алматы
```

**Step 2: Encode to base64 and save**

Run:
```bash
cat <<'VLESS' | base64 > test/fixtures/vless_servers.b64
vless://11111111-2222-3333-4444-555555555555@server1.test.example:8443?type=tcp&security=tls#Prague, Czechia
vless://11111111-2222-3333-4444-555555555555@server2.test.example:8443?type=tcp&security=tls#🇷🇺 Россия, Москва
vless://11111111-2222-3333-4444-555555555555@server3.test.example:8443?type=tcp&security=tls#🇺🇸🌟✨
vless://11111111-2222-3333-4444-555555555555@server4.test.example:8443?type=tcp&security=tls#New%20York%20City
vless://11111111-2222-3333-4444-555555555555@server5.test.example:8443?type=tcp&security=tls#Казахстан, Алматы
VLESS
```

**Step 3: Verify fixture decodes correctly**

Run:
```bash
base64 -d test/fixtures/vless_servers.b64 | head -2
```
Expected:
```
vless://11111111-2222-3333-4444-555555555555@server1.test.example:8443?type=tcp&security=tls#Prague, Czechia
vless://11111111-2222-3333-4444-555555555555@server2.test.example:8443?type=tcp&security=tls#🇷🇺 Россия, Москва
```

**Step 4: Commit**

```bash
git add test/fixtures/vless_servers.b64
git commit -m "$(cat <<'EOF'
test: add anonymized VLESS fixture for import_server_list tests

Contains 5 URIs testing: ASCII names, emoji+cyrillic, emoji-only,
URL-encoded spaces, and cyrillic-only names.
EOF
)"
```

---

## Task 4: Create Test File with Basic Parsing Tests

**Files:**
- Create: `test/import_server_list.bats`

**Step 1: Create test file with basic extraction tests**

Create `test/import_server_list.bats`:

```bash
#!/usr/bin/env bats

load 'test_helper'

# Test URI for basic parsing (ASCII name, no special chars)
TEST_URI_BASIC='vless://11111111-2222-3333-4444-555555555555@server1.test.example:8443?type=tcp&security=tls#Prague, Czechia'

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
```

**Step 2: Run tests**

Run:
```bash
npx bats test/import_server_list.bats
```
Expected: 4 tests, all passing

**Step 3: Commit**

```bash
git add test/import_server_list.bats
git commit -m "$(cat <<'EOF'
test: add basic parsing tests for parse_vless_uri

Tests extraction of: hostname, port, UUID, ASCII name.
EOF
)"
```

---

## Task 5: Add Name Handling Tests

**Files:**
- Modify: `test/import_server_list.bats`

**Step 1: Add emoji and cyrillic test URIs as constants**

Append after `TEST_URI_BASIC` line:

```bash
# URI with emoji flag + cyrillic name
TEST_URI_EMOJI_CYRILLIC='vless://11111111-2222-3333-4444-555555555555@server2.test.example:8443?type=tcp&security=tls#🇷🇺 Россия, Москва'

# URI with only emoji (should fallback to hostname)
TEST_URI_EMOJI_ONLY='vless://11111111-2222-3333-4444-555555555555@server3.test.example:8443?type=tcp&security=tls#🇺🇸🌟✨'

# URI with URL-encoded spaces
TEST_URI_URLENCODED='vless://11111111-2222-3333-4444-555555555555@server4.test.example:8443?type=tcp&security=tls#New%20York%20City'

# URI with cyrillic only (no emoji)
TEST_URI_CYRILLIC='vless://11111111-2222-3333-4444-555555555555@server5.test.example:8443?type=tcp&security=tls#Казахстан, Алматы'
```

**Step 2: Add name handling tests**

Append to end of file:

```bash

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
```

**Step 3: Run all tests**

Run:
```bash
npx bats test/import_server_list.bats
```
Expected: 8 tests, all passing

**Step 4: Commit**

```bash
git add test/import_server_list.bats
git commit -m "$(cat <<'EOF'
test: add name handling tests for parse_vless_uri

Tests: emoji filtering with cyrillic preservation, emoji-only fallback
to hostname, URL-encoded space decoding, pure cyrillic names.
EOF
)"
```

---

## Task 6: Final Verification

**Step 1: Run full test suite**

Run:
```bash
npx bats test/*.bats
```
Expected: All tests pass, including new import_server_list.bats

**Step 2: Verify no regressions**

Check that existing tests still pass.

---

## Summary

| Task | Description | Files |
|------|-------------|-------|
| 1 | Enable IMPORT_TEST_MODE | `import_server_list.sh` |
| 2 | Add test helper | `test_helper.bash` |
| 3 | Create fixture | `vless_servers.b64` |
| 4 | Basic parsing tests | `import_server_list.bats` |
| 5 | Name handling tests | `import_server_list.bats` |
| 6 | Final verification | - |

**Total: 6 tasks, ~5 commits**
