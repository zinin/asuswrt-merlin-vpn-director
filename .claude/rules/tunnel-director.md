---
paths: "jffs/scripts/firewall/**"
---

# Tunnel Director

Policy-based outbound routing through VPN tunnels (WireGuard/OpenVPN) or WAN.

## Overview

`tunnel_director.sh` routes LAN traffic through VPN clients based on destination ipsets.

**Anti-censorship design**:
- Inclusion: route only to specific countries/lists
- Exclusion: route everything except trusted destinations

## Rule Format

```
table:src[%iface][:src_excl]:set[,set...][:set_excl]
```

| Field | Description | Example |
|-------|-------------|---------|
| `table` | Routing table: `wgc1`, `ovpnc1`, `main` | `wgc1` |
| `src` | LAN subnet (RFC1918) | `192.168.50.0/24` |
| `%iface` | Optional interface (default: `br0`) | `%br1` |
| `src_excl` | Excluded source IPs (comma-sep) | `192.168.50.100` |
| `set` | Destination ipset(s) or `any` | `us,ca` |
| `set_excl` | Excluded destinations | `ru` |

**Examples**:
```bash
# Route US/CA traffic via WireGuard
wgc1:192.168.50.0/24::us,ca

# Route all except RU via OpenVPN
ovpnc1:192.168.1.0/24::any:ru

# Different interface
wgc1:10.0.0.0/24%br1::us
```

## Chain Architecture

Per-rule chains in mangle table:
```
PREROUTING → TUN_DIR_0 → TUN_DIR_1 → ...
```

Each chain:
1. RETURN for excluded sources
2. Match destination ipset
3. Set fwmark (`slot << shift`)
4. ip rule routes by fwmark to table

## Fwmark Layout

Default: bits 16-23 (8 bits = 255 rules max)

| Config | Default | Purpose |
|--------|---------|---------|
| `TUN_DIR_MARK_MASK` | `0x00ff0000` | Mask for TD bits |
| `TUN_DIR_MARK_SHIFT` | `16` | Bit position |
| `TUN_DIR_PREF_BASE` | `300` | ip rule priority base |

## State Tracking

Files in `/tmp/tunnel_director/`:

| File | Purpose |
|------|---------|
| `tun_dir_rules.sha256` | Hash of normalized rules |

**Rebuild triggers**:
- Rules hash changed
- Chain/rule count drift detected
- ip rule pref mismatch

## Dependencies

- Requires ipsets from `ipset_builder.sh`
- Waits for `TUN_DIR_IPSETS_HASH` to match rules hash
- VPN client must be active with NAT enabled

## Key Functions

| Function | Purpose |
|----------|---------|
| `table_allowed()` | Validate routing table name |
| `resolve_set_name()` | Lookup ipset by key |
| `get_prerouting_base_pos()` | Find insert position after system rules |

## Utilities Used

From `common.sh`: `log`, `acquire_lock`, `tmp_file`, `strip_comments`, `compute_hash`, `is_lan_ip`

From `firewall.sh`: `create_fw_chain`, `delete_fw_chain`, `ensure_fw_rule`, `sync_fw_rule`, `purge_fw_rules`
