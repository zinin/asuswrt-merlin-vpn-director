---
paths: "jffs/scripts/vpn-director/**"
---

# Xray TPROXY

Transparent proxy routing for LAN clients via Xray.

## Usage

```bash
xray_tproxy.sh          # Apply rules (default: start)
xray_tproxy.sh start    # Apply rules
xray_tproxy.sh stop     # Remove all rules
xray_tproxy.sh restart  # Stop + start
xray_tproxy.sh status   # Show current status
```

## How It Works

1. Selected LAN clients → mangle PREROUTING chain
2. Exclude: servers, private IPs, specified countries
3. Remaining traffic → TPROXY to Xray port
4. Xray dokodemo-door inbound → VLESS outbound

## Configuration

In `configs/config-xray.sh`:

| Variable | Default | Purpose |
|----------|---------|---------|
| `XRAY_TPROXY_PORT` | `12345` | Xray dokodemo-door port |
| `XRAY_CLIENTS` | - | LAN IPs/CIDRs to proxy (newline-separated) |
| `XRAY_SERVERS` | - | Xray server IPs to exclude (avoid loops) |
| `XRAY_EXCLUDE_SETS` | - | Country codes/ipsets to skip (space-separated) |
| `XRAY_ROUTE_TABLE` | `100` | ip route table number |
| `XRAY_RULE_PREF` | `200` | ip rule priority |
| `XRAY_FWMARK` | `0x100` | Routing fwmark (bit 8) |
| `XRAY_FWMARK_MASK` | `0x100` | Fwmark mask |
| `XRAY_CHAIN` | `XRAY_TPROXY` | mangle chain name |
| `XRAY_CLIENTS_IPSET` | `XRAY_CLIENTS` | Source clients ipset |
| `XRAY_SERVERS_IPSET` | `XRAY_SERVERS` | Server exclusion ipset |

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
- ipsets from `ipset_builder.sh` (for country exclusions)

## Fail-Safe

Script exits without changes if:
- Required exclusion ipsets not found
- xt_TPROXY module unavailable

## Extended Exclusion Sets

Prefers `{set}_ext` variant if exists:
```bash
resolve_exclude_set "ru"  # Returns: ru_ext if exists, else ru
```

## Key Functions

| Function | Purpose |
|----------|---------|
| `check_tproxy_module()` | Verify/load xt_TPROXY kernel module |
| `check_required_ipsets()` | Fail-safe: exit if exclusion ipsets missing |
| `resolve_exclude_set()` | Try `{set}_ext` first, fall back to `{set}` |
| `setup_routing()` | Create route table + ip rule |
| `teardown_routing()` | Remove route table + ip rule |
| `setup_clients_ipset()` | Create and populate client ipset |
| `setup_servers_ipset()` | Create and populate server ipset |
| `setup_iptables()` | Build XRAY_TPROXY chain with exclusions |
| `teardown_iptables()` | Remove chain and ipsets |
| `show_status()` | Debug output for troubleshooting |
