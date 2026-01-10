---
paths: "jffs/scripts/xray/**"
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

In `config.sh`:

| Variable | Purpose | Example |
|----------|---------|---------|
| `XRAY_TPROXY_PORT` | Xray dokodemo-door port | `12345` |
| `XRAY_CLIENTS` | LAN IPs to proxy | `192.168.50.10` |
| `XRAY_SERVERS` | Xray server IPs (excluded) | `1.2.3.4` |
| `XRAY_EXCLUDE_SETS` | Country/custom ipsets to skip | `ru` |
| `XRAY_FWMARK` | Routing fwmark | `0x1` |
| `XRAY_ROUTE_TABLE` | ip route table | `100` |

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
# Route table
ip route add local default dev lo table $XRAY_ROUTE_TABLE

# ip rule
ip rule add pref $XRAY_RULE_PREF fwmark $XRAY_FWMARK/$XRAY_FWMARK_MASK table $XRAY_ROUTE_TABLE
```

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
| `check_tproxy_module()` | Verify/load xt_TPROXY |
| `check_required_ipsets()` | Fail-safe ipset check |
| `setup_routing()` | Create route + ip rule |
| `setup_clients_ipset()` | Populate client ipset |
| `setup_servers_ipset()` | Populate server ipset |
| `setup_iptables()` | Build mangle chain |
| `show_status()` | Debug output |
