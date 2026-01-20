---
paths: "**/*.sh, jffs/**/*"
---

# Shell Conventions

## Script Structure

- Shebang: `#!/usr/bin/env bash` with `set -euo pipefail`
- Debug mode: `DEBUG=1 ./script.sh` enables `set -x` with informative PS4
- shellcheck annotations for intentional expansions/externals (SC2086, SC2155, SC2034)

## Logging & Locking

- Logging: `log -l ERROR|WARN|INFO|DEBUG|TRACE "message"` (default: INFO)
- Locking: `acquire_lock [name]` prevents concurrent script execution
- Temp files: `tmp_file` / `tmp_dir` with auto-cleanup on exit

## Key Utilities (common.sh)

| Function | Description |
|----------|-------------|
| `uuid4` | Generate random UUIDv4 from kernel |
| `compute_hash [file\|-]` | SHA-256 digest of file or stdin |
| `get_script_path` | Absolute path to current script (resolves symlinks) |
| `get_script_dir` | Directory containing current script |
| `get_script_name [-n]` | Script filename; `-n` strips extension |
| `resolve_ip [-6] [-q] [-g] [-a] <host>` | DNS/hosts resolution |
| `resolve_lan_ip [-6] [-q] [-a] <host>` | Resolve only private/LAN addresses |
| `is_lan_ip [-6] <ip>` | Check if IP is in RFC1918/ULA range |
| `is_pos_int <value>` | Check if value is positive integer (>=1) |
| `strip_comments [text]` | Remove blank lines and # comments |
| `get_active_wan_if` | Get active WAN interface name |
| `get_ipv6_enabled` | Returns 1 if IPv6 enabled, 0 otherwise |

**Logging**: `LOG_FILE=/tmp/vpn_director.log` with 100KB rotation

## Firewall Utilities (firewall.sh)

| Function | Description |
|----------|-------------|
| `fw_chain_exists [-6] <table> <chain>` | Check if chain exists |
| `create_fw_chain [-6] [-q] [-f] <table> <chain>` | Create chain; `-f` flushes if exists |
| `delete_fw_chain [-6] [-q] <table> <chain>` | Flush and delete chain |
| `find_fw_rules [-6] "<table> <chain>" "<pattern>"` | Find rules matching regex |
| `purge_fw_rules [-6] [-q] [--count] "<table> <chain>" "<pattern>"` | Remove matching rules |
| `ensure_fw_rule [-6] [-q] [--count] <table> <chain> [-I [pos]\|-D] <rule>` | Idempotent rule add/delete |
| `sync_fw_rule [-6] [-q] [--count] <table> <chain> "<pattern>" "<desired>" [pos]` | Replace matching rules with one |
| `block_wan_for_host <host> [wan_id]` | Block host from WAN (IPv4/IPv6) |
| `allow_wan_for_host <host> [wan_id]` | Unblock host from WAN |
| `chg <cmd>` | Returns true if command output is non-zero integer |
| `validate_port <N>` | Validate port 1-65535 |
| `validate_ports <spec>` | Validate port spec (any, N, N-M, N,N2) |
| `normalize_protos <spec>` | Normalize to tcp, udp, or tcp,udp |

## State Tracking

Hash files in `/tmp/` detect config changes; scripts only reapply if changed.

## Bash-specific Patterns

- Use `[[ ]]` instead of `[ ]` for conditionals
- Use `read -ra array <<< "$string"` for splitting strings into arrays
- Use `${array[@]}` for iterating arrays
- Use `[[ $var =~ regex ]]` for regex matching instead of grep
- Debug mode: `DEBUG=1` enables `set -x` with PS4 showing file:line:function

## Known Pitfalls

### `tr` with POSIX character classes breaks on router

**Problem**: `tr '[:upper:]' '[:lower:]'` corrupts certain characters on Asuswrt-Merlin routers with Entware.

Example: letter `u` (0x75) becomes `l` (0x6c):
```bash
echo "ru" | tr '[:upper:]' '[:lower:]'
# Expected: ru
# Actual:   rl
```

This is a bug in glibc/busybox `tr` with certain locale settings (`LC_ALL=en_US.UTF-8`). Setting `LC_ALL=C` does not fix it.

**Solution**: Always use explicit ASCII ranges instead of POSIX character classes:
```bash
# BAD - breaks on router
tr '[:upper:]' '[:lower:]'

# GOOD - works everywhere
tr 'A-Z' 'a-z'
```

**Rule**: Never use `[:upper:]` / `[:lower:]` in shell scripts for this project. Always use `A-Z` / `a-z`.

**Alternative fix**: Install `opkg install coreutils-tr` which provides a working `/opt/bin/tr`. After installation and `hash -r`, the correct `tr` will be used. However, code should still use `A-Z` / `a-z` for compatibility with systems without coreutils.

### Uninitialized arrays with `set -u`

**Problem**: With `set -u` (nounset), accessing uninitialized array length fails:
```bash
local -a my_array
echo ${#my_array[@]}  # Error: my_array: unbound variable
```

**Solution**: Always initialize arrays:
```bash
local -a my_array=()
echo ${#my_array[@]}  # Works: outputs 0
```
