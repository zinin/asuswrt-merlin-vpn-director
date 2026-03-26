# Exclude IPs & Multi-Resolve Design

## Problem

The XRAY_SERVERS ipset prevents TPROXY from intercepting traffic destined to Xray servers (avoiding routing loops). Currently it has three gaps:

1. **Single IP resolution** — if an Xray server hostname resolves to multiple IPs, only the first is added. Traffic to other IPs creates a loop.
2. **OpenVPN endpoints not excluded** — traffic to OpenVPN server endpoints (ovpnc1–ovpnc5) can be intercepted by TPROXY, breaking VPN tunnels.
3. **No custom exclusions** — users cannot exclude arbitrary IPs/CIDRs from proxying (e.g., servers that don't need VPN).

## Solution: Approach B — Separate Config Fields

### Configuration

New field `exclude_ips` in `xray` section of `vpn-director.json`:

```json
{
  "xray": {
    "clients": ["192.168.1.3"],
    "servers": ["85.208.85.193", "85.208.85.214"],
    "exclude_ips": ["1.2.3.4", "10.20.0.0/16"],
    "exclude_sets": ["ru", "by", "kz"]
  }
}
```

- `servers` — IPs of Xray servers (populated by bot on `/configure`)
- `exclude_ips` — arbitrary IPs/CIDRs set by user (via bot or manual edit)
- OpenVPN endpoints — **not stored** in config, resolved automatically on each `apply`

### IPSet Assembly

On `vpn-director.sh apply`, `_tproxy_setup_servers_ipset()` merges three sources into `XRAY_SERVERS` ipset:

```
XRAY_SERVERS ipset =
    xray.servers[]                          (all Xray server IPs)
  + xray.exclude_ips[]                      (user-defined static exclusions)
  + nvram vpn_client{1-5}_addr resolved IPs (OpenVPN endpoints, on-the-fly)
```

Logging: `INFO: XRAY_SERVERS ipset: 8 xray, 2 user, 3 openvpn = 13 total`

Failed OpenVPN endpoint resolution: WARN, continue (don't block apply).

### Multi-IP Resolution

#### servers.json structure change

Before:
```json
{"address": "server.example.com", "port": 8443, "uuid": "...", "name": "City", "ip": "1.2.3.4"}
```

After:
```json
{"address": "server.example.com", "port": 8443, "uuid": "...", "name": "City", "ips": ["1.2.3.4", "5.6.7.8"]}
```

Field `ip` removed. No backward compatibility — user re-imports via `/import`.

#### Shell (import_server_list.sh)

Use `resolve_ip -a` to get all IPv4 addresses. Store as `ips` array in servers.json.

#### Go bot (vless/parser.go)

`ResolveIP()` → `ResolveIPs()`:
- `net.LookupIP()` already returns all IPs
- Keep all IPv4 addresses instead of just the first
- `Server.IP string` → `Server.IPs []string`

#### /configure apply (wizard/apply.go)

Collect all IPs from `ips` field of all servers → `xray.servers`.

### OpenVPN Endpoint Auto-Detection

On each `apply` in `_tproxy_setup_servers_ipset()`:

1. Read `nvram get vpn_client{1-5}_addr` for each slot
2. Skip empty values
3. Resolve all IPv4 via `resolve_ip -a`
4. Add to XRAY_SERVERS ipset
5. WARN on resolution failure, don't block

Endpoints are not stored in config — determined fresh on every apply.

### Telegram Bot

#### New command: `/exclude`

Opens a wizard with inline buttons:
- Shows current list of IPs/CIDRs from `xray.exclude_ips`
- `[Add]` button — user enters IP or CIDR as text
- `[Remove X.X.X.X]` button next to each entry
- On wizard exit — runs `vpn-director.sh apply`

#### `/configure` wizard — new step

After country selection (exclude_sets), before final apply:
- "Add extra IPs/subnets to exclude from proxying?"
- `[Skip]` or `[Add]`
- If add — same interface: list + add/remove
- Result stored in `xray.exclude_ips`

### Shell Changes

| File | Change |
|------|--------|
| `lib/config.sh` | Add `XRAY_EXCLUDE_IPS=$(_cfg_arr '.xray.exclude_ips')` |
| `lib/tproxy.sh` `_tproxy_setup_servers_ipset()` | Merge three sources into ipset, log counts per source |
| `import_server_list.sh` | Use `resolve_ip -a`, save `ips` instead of `ip` |
| `vpn-director.json.template` | Add `"exclude_ips": []` to xray section |

### Tests

- Unit tests for ipset assembly from three sources
- Test for multi-IP resolution
- Test for OpenVPN endpoint auto-detection (mock nvram)
