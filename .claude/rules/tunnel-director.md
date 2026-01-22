---
paths: "router/opt/vpn-director/**"
---

# Tunnel Director

Policy-based outbound routing through VPN tunnels (WireGuard/OpenVPN) using exclusion-based logic.

Module location: `lib/tunnel.sh`

## Overview

Routes LAN client traffic through VPN tunnels. All traffic goes through the tunnel except destinations in exclude lists (country ipsets).

## Configuration Format

```json
{
  "tunnel_director": {
    "tunnels": {
      "wgc1": {
        "clients": ["192.168.50.0/24", "192.168.50.100"],
        "exclude": ["<country_code>"]
      },
      "ovpnc1": {
        "clients": ["192.168.1.5"],
        "exclude": ["<country_code>"]
      }
    }
  }
}
```

| Field | Description |
|-------|-------------|
| Tunnel key | Routing table name: `wgc1`, `ovpnc1`, `main` |
| `clients` | Array of LAN IPs/CIDRs (RFC1918 only) |
| `exclude` | Array of country codes for direct routing |

## Behavior

- All traffic from `clients` routes through the tunnel
- Traffic to destinations in `exclude` ipsets bypasses VPN (goes direct)
- Order of tunnels in JSON determines fwmark assignment

## Chain Architecture

Single chain in mangle table:

```
PREROUTING
    └─> TUN_DIR
          ├─ src 192.168.50.0/24 + dst <country> -> RETURN
          ├─ src 192.168.50.0/24 -> MARK 0x10000 (wgc1)
          ├─ src 192.168.1.5 + dst <country> -> RETURN
          └─ src 192.168.1.5 -> MARK 0x20000 (ovpnc1)
```

## Advanced Configuration

| JSON Path | Default | Purpose |
|-----------|---------|---------|
| `advanced.tunnel_director.chain` | `TUN_DIR` | Chain name |
| `advanced.tunnel_director.pref_base` | `16384` | ip rule priority base |
| `advanced.tunnel_director.mark_mask` | `0x00ff0000` | Fwmark mask |
| `advanced.tunnel_director.mark_shift` | `16` | Bit position |

## Fwmark Layout

Default: bits 16-23 (8 bits = 255 tunnels max)

## State Tracking

| File | Purpose |
|------|---------|
| `/tmp/tunnel_director/tun_dir_rules.sha256` | Hash of last applied config |

**Rebuild triggers**:
- Config hash changed
- Chain does not exist

## Key Functions

**Public API**:

| Function | Purpose |
|----------|---------|
| `tunnel_status()` | Show chain, ip rules, configured tunnels |
| `tunnel_apply()` | Apply rules from config (idempotent) |
| `tunnel_stop()` | Remove chain and ip rules |
| `tunnel_get_required_ipsets()` | Return list of exclude ipsets needed |

**Internal functions** (for testing):

| Function | Purpose |
|----------|---------|
| `_tunnel_init()` | Initialize module state (valid tables, fwmark helpers) |
| `_tunnel_table_allowed(table)` | Validate routing table (wgcN, ovpncN, main) |
| `_tunnel_get_prerouting_base_pos()` | Find insert position after system rules |

**Module state variables**:
- `_tunnel_valid_tables` - space-separated list of allowed routing tables
- `_tunnel_mark_field_max` - max tunnels that fit in fwmark field (default: 255)
- `_tunnel_mark_mask_hex` - hex string of `TUN_DIR_MARK_MASK`

## Dependencies

From `lib/common.sh`: `log`, `tmp_file`, `compute_hash`, `is_lan_ip`

From `lib/firewall.sh`: `create_fw_chain`, `delete_fw_chain`, `ensure_fw_rule`, `sync_fw_rule`, `purge_fw_rules`, `fw_chain_exists`

From `lib/ipset.sh`: `_ipset_exists`, `parse_exclude_sets_from_json`, `TUN_DIR_HASH`

## Requirements

- Requires ipsets from `lib/ipset.sh` (country codes in exclude lists)
- VPN client must be active with NAT enabled
