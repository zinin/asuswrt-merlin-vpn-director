# Design: Unit Tests for import_server_list.sh

## Overview

Unit-тесты для функции `parse_vless_uri` из `import_server_list.sh` с фокусом на обработку emoji и кириллицы.

## Test Data (Fixture)

Файл `test/fixtures/vless_servers.b64` — base64-encoded VLESS URI:

| # | Case | Name in URI | Expected after parsing |
|---|------|-------------|------------------------|
| 1 | Basic ASCII | `Prague, Czechia` | `Prague, Czechia` |
| 2 | Emoji + cyrillic | `🇷🇺 Россия, Москва` | `Россия, Москва` |
| 3 | Emoji only | `🇺🇸🌟✨` | fallback to hostname |
| 4 | URL-encoded spaces | `New%20York%20City` | `New York City` |
| 5 | Cyrillic only | `Казахстан, Алматы` | `Казахстан, Алматы` |

**Common URI parameters:**
- UUID: `11111111-2222-3333-4444-555555555555`
- Domains: `server1.test.example`, `server2.test.example`, ...
- Port: `8443`

## Test Structure

File: `test/import_server_list.bats`

```
parse_vless_uri: extracts server hostname
parse_vless_uri: extracts port number
parse_vless_uri: extracts UUID
parse_vless_uri: extracts ASCII name
parse_vless_uri: filters emoji from name, keeps cyrillic
parse_vless_uri: falls back to hostname when name is only emoji
parse_vless_uri: decodes URL-encoded spaces
parse_vless_uri: handles cyrillic-only name
```

## Implementation

### Helper Function

Add to `test/test_helper.bash`:

```bash
load_import_server_list() {
    load_common
    export IMPORT_TEST_MODE=1
    source "$SCRIPTS_DIR/import_server_list.sh"
}
```

### Script Modification

Change end of `jffs/scripts/vpn-director/import_server_list.sh`:

```bash
# Before:
main "$@"

# After:
if [[ "${IMPORT_TEST_MODE:-0}" != "1" ]]; then
    main "$@"
fi
```

## Files to Create/Modify

| File | Action |
|------|--------|
| `test/fixtures/vless_servers.b64` | Create — base64-encoded test URIs |
| `test/import_server_list.bats` | Create — 8 tests for `parse_vless_uri` |
| `test/test_helper.bash` | Add `load_import_server_list()` |
| `jffs/scripts/vpn-director/import_server_list.sh` | Add `IMPORT_TEST_MODE` check |
