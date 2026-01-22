---
paths: "router/opt/vpn-director/**"
---

# Xray TPROXY

Transparent proxy routing for LAN clients via Xray.

Module location: `lib/tproxy.sh`

## Usage via CLI

```bash
vpn-director.sh status xray       # Show Xray TPROXY status
vpn-director.sh restart xray      # Restart Xray TPROXY
vpn-director.sh apply             # Apply all (including TPROXY)
```

## How It Works

1. Selected LAN clients → mangle PREROUTING chain
2. Exclude: servers, private IPs, specified countries
3. Remaining traffic → TPROXY to Xray port
4. Xray dokodemo-door inbound → VLESS outbound

## Configuration

In `vpn-director.json`:

| JSON Path | Default | Purpose |
|-----------|---------|---------|
| `xray.clients` | `[]` | LAN IPs/CIDRs to proxy (JSON array) |
| `xray.servers` | `[]` | Xray server IPs to exclude (avoid loops) |
| `xray.exclude_sets` | `[]` | Country codes/ipsets to skip |
| `advanced.xray.tproxy_port` | `12345` | Xray dokodemo-door port |
| `advanced.xray.route_table` | `100` | ip route table number |
| `advanced.xray.rule_pref` | `200` | ip rule priority |
| `advanced.xray.fwmark` | `0x100` | Routing fwmark (bit 8) |
| `advanced.xray.fwmark_mask` | `0x100` | Fwmark mask |
| `advanced.xray.chain` | `XRAY_TPROXY` | mangle chain name |
| `advanced.xray.clients_ipset` | `XRAY_CLIENTS` | Source clients ipset |
| `advanced.xray.servers_ipset` | `XRAY_SERVERS` | Server exclusion ipset |

## IPSets Created

| Name | Type | Purpose |
|------|------|---------|
| `XRAY_CLIENTS_IPSET` | `hash:net` | Source clients |
| `XRAY_SERVERS_IPSET` | `hash:net` | Xray servers (excluded) |

## Chain Structure

```
PREROUTING (pos 1)
    └─→ XRAY_TPROXY
          ├─ ! src clients → RETURN
          ├─ dst servers → RETURN
          ├─ dst 127.0.0.0/8 → RETURN
          ├─ dst 10.0.0.0/8 → RETURN
          ├─ dst 172.16.0.0/12 → RETURN
          ├─ dst 192.168.0.0/16 → RETURN
          ├─ dst 169.254.0.0/16 → RETURN
          ├─ dst 224.0.0.0/4 → RETURN
          ├─ dst 255.255.255.255 → RETURN
          ├─ dst {exclude_sets} → RETURN
          ├─ tcp → TPROXY :port mark
          └─ udp → TPROXY :port mark
```

## Routing Setup

```bash
# Route table (for TPROXY to work)
ip route add local default dev lo table $XRAY_ROUTE_TABLE

# ip rule (fwmark-based lookup)
ip rule add pref $XRAY_RULE_PREF fwmark $XRAY_FWMARK/$XRAY_FWMARK_MASK table $XRAY_ROUTE_TABLE
```

**Fwmark bit allocation**: `0x100` (bit 8) is free from firmware VPN marking (bit 0) and Tunnel Director (bits 16-23).

## Requirements

- `xt_TPROXY` kernel module
- Xray running with dokodemo-door inbound (tproxy mode)
- ipsets from `lib/ipset.sh` (for country exclusions)

## Fail-Safe

Script exits without changes if:
- Required exclusion ipsets not found
- xt_TPROXY module unavailable

## Extended Exclusion Sets

Prefers `{set}_ext` variant if exists:
```bash
resolve_exclude_set "<country_code>"  # Returns: <country_code>_ext if exists, else <country_code>
```

## Key Functions

**Public API**:

| Function | Purpose |
|----------|---------|
| `tproxy_status()` | Show XRAY_TPROXY chain, routing, xray process |
| `tproxy_apply()` | Apply TPROXY rules (idempotent), soft-fail if unavailable |
| `tproxy_stop()` | Remove chain and routing |
| `tproxy_restart_process()` | Restart Xray process via Entware init script |
| `tproxy_get_required_ipsets()` | Return list of exclude ipsets |

**Internal functions** (for testing):

| Function | Purpose |
|----------|---------|
| `_tproxy_init()` | Initialize module state |
| `_tproxy_check_module()` | Verify/load xt_TPROXY kernel module |
| `_tproxy_check_required_ipsets()` | Fail-safe: exit if exclusion ipsets missing |
| `_tproxy_resolve_exclude_set(key)` | Try `{set}_ext` first, fall back to `{set}` |
| `_tproxy_setup_routing()` | Create route table + ip rule |
| `_tproxy_teardown_routing()` | Remove route table + ip rule |
| `_tproxy_setup_clients_ipset()` | Create and populate client ipset |
| `_tproxy_setup_servers_ipset()` | Create and populate server ipset |
| `_tproxy_setup_iptables()` | Build XRAY_TPROXY chain with exclusions |
| `_tproxy_teardown_iptables()` | Remove chain and ipsets |

**Soft-fail behavior**: `tproxy_apply()` returns 0 even if xt_TPROXY unavailable or ipsets missing, allowing caller scripts to continue.
