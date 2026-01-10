---
paths: "**/*.sh, jffs/**/*"
---

# Shell Conventions

## Script Structure

- Shebang: `#!/usr/bin/env ash` with `set -euo pipefail`
- shellcheck annotations for intentional expansions/externals (SC2086, SC2155, SC2034)

## Logging & Locking

- Logging: `log -l err|warn|info|notice "message"`
- Locking: `acquire_lock [name]` prevents concurrent script execution
- Temp files: `tmp_file` / `tmp_dir` with auto-cleanup on exit

## Key Utilities (common.sh)

| Function | Description |
|----------|-------------|
| `uuid4` | Generate random UUIDv4 |
| `compute_hash` | SHA-256 digest of file or stdin |
| `resolve_ip [-6] [-q] [-g] [-a] <host>` | DNS/hosts resolution |
| `resolve_lan_ip` | Resolve only private/LAN addresses |
| `is_lan_ip [-6] <ip>` | Check if IP is in RFC1918/ULA range |
| `strip_comments` | Remove blank lines and # comments |
| `get_active_wan_if` | Get active WAN interface name |

## Firewall Utilities (firewall.sh)

Chain and rule management helpers for iptables operations.

## State Tracking

Hash files in `/tmp/` detect config changes; scripts only reapply if changed.
