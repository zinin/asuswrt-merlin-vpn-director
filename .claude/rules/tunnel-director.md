---
paths: "jffs/scripts/vpn-director/**"
---

# Tunnel Director

Policy-based outbound routing through VPN tunnels (WireGuard/OpenVPN) or WAN.

Module location: `lib/tunnel.sh`

## Overview

The tunnel module routes LAN traffic through VPN clients based on destination ipsets.

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

## Configuration

In `vpn-director.json`:

| JSON Path | Default | Purpose |
|-----------|---------|---------|
| `tunnel_director.rules` | `[]` | Routing rules (JSON array) |
| `data_dir` | `/jffs/scripts/vpn-director/data` | Persistent storage (ipset dumps, servers) |
| `advanced.tunnel_director.chain_prefix` | `TUN_DIR_` | Chain name prefix |
| `advanced.tunnel_director.pref_base` | `16384` | ip rule priority base |
| `advanced.tunnel_director.mark_mask` | `0x00ff0000` | Mask for TD bits |
| `advanced.tunnel_director.mark_shift` | `16` | Bit position |

## Fwmark Layout

Default: bits 16-23 (8 bits = 255 rules max)

## State Tracking

| File | Purpose |
|------|---------|
| `/tmp/tunnel_director/tun_dir_rules.sha256` | Hash of last applied rules |
| `/tmp/ipset_builder/tun_dir_ipsets.sha256` | Hash from ipset module (for sync) |

**Rebuild triggers**:
- Rules hash changed
- Chain count != config rows
- PREROUTING jump count != config rows
- ip rule pref count mismatch

## Dependencies

- Requires ipsets from `lib/ipset.sh`
- Waits for `TUN_DIR_IPSETS_HASH` to match rules hash
- VPN client must be active with NAT enabled

## Key Functions

| Function | Purpose |
|----------|---------|
| `table_allowed()` | Validate routing table (wgcN, ovpncN, main) |
| `resolve_set_name()` | Lookup ipset by key (uses `derive_set_name`) |
| `get_prerouting_base_pos()` | Find insert position after system iface-mark rules |

**Variables**:
- `valid_tables` - space-separated list of allowed routing tables
- `_mark_field_max` - max rules that fit in fwmark field (default: 255)
- `mark_mask_hex` - hex string of `TUN_DIR_MARK_MASK`

## Utilities Used

From `lib/common.sh`: `log`, `acquire_lock`, `tmp_file`, `strip_comments`, `compute_hash`, `is_lan_ip`

From `lib/firewall.sh`: `create_fw_chain`, `delete_fw_chain`, `ensure_fw_rule`, `sync_fw_rule`, `purge_fw_rules`
