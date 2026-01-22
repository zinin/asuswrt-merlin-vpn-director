# VPN Director for Asuswrt-Merlin

Traffic routing system for Asus routers: Xray TPROXY, Tunnel Director, IPSet Builder.

## Commands

```bash
# Install
curl -fsSL https://raw.githubusercontent.com/zinin/asuswrt-merlin-vpn-director/master/install.sh | bash

# VPN Director CLI
/opt/vpn-director/vpn-director.sh status              # Show all status
/opt/vpn-director/vpn-director.sh apply               # Apply configuration
/opt/vpn-director/vpn-director.sh stop                # Stop all components
/opt/vpn-director/vpn-director.sh restart             # Restart all
/opt/vpn-director/vpn-director.sh update              # Update ipsets + reapply

# Component-specific commands
/opt/vpn-director/vpn-director.sh status tunnel       # Tunnel Director status only
/opt/vpn-director/vpn-director.sh restart xray        # Restart Xray TPROXY only

# Import servers
/opt/vpn-director/import_server_list.sh
```

## Architecture

| Path | Purpose |
|------|---------|
| `router/opt/vpn-director/vpn-director.sh` | Unified CLI entry point |
| `router/opt/vpn-director/lib/common.sh` | Core utilities: log, tmp_file, download_file, resolve_ip |
| `router/opt/vpn-director/lib/firewall.sh` | Firewall helpers: chain, rule, block/allow host |
| `router/opt/vpn-director/lib/config.sh` | JSON config loader (vpn-director.json → shell vars) |
| `router/opt/vpn-director/lib/ipset.sh` | IPSet module: ensure, update, status |
| `router/opt/vpn-director/lib/tunnel.sh` | Tunnel Director module: apply, stop, status |
| `router/opt/vpn-director/lib/tproxy.sh` | Xray TPROXY module: apply, stop, status |
| `router/opt/etc/init.d/S99vpn-director` | Entware init.d script for startup |
| `router/jffs/scripts/firewall-start` | Asuswrt-Merlin hook for firewall reload |
| `router/jffs/scripts/wan-event` | Asuswrt-Merlin hook for WAN events |
| `router/test/` | Bats tests (unit/, integration/) |
| `router/opt/vpn-director/vpn-director.json.template` | Unified config template |
| `router/opt/etc/xray/config.json.template` | Xray server config template |
| `install.sh` | Interactive installer |
| `telegram-bot/` | Go-based Telegram bot for remote management |
| `router/opt/vpn-director/setup_telegram_bot.sh` | Bot configuration script |

## Key Concepts

**Tunnel Director**: Routes LAN client traffic through VPN tunnels with exclusion-based logic.
```json
{
  "tunnel_director": {
    "tunnels": {
      "wgc1": { "clients": ["192.168.50.0/24"], "exclude": ["<country_code>"] }
    }
  }
}
```
- All traffic from `clients` goes through tunnel
- Traffic to destinations in `exclude` bypasses VPN (direct)

**IPSet Types**: Country sets — 2-letter ISO codes from multi-source download

**IPSet Sources** (priority order):
1. GeoLite2 via GitHub (firehol/blocklist-ipsets)
2. IPDeny via GitHub (firehol mirror)
3. IPDeny direct (ipdeny.com)
4. Manual fallback (interactive)

## Config Files (after install)

| Path | Purpose |
|------|---------|
| `/opt/vpn-director/vpn-director.json` | Unified config (Xray + Tunnel Director) |
| `/opt/etc/xray/config.json` | Xray server configuration |
| `/opt/vpn-director/telegram-bot.json` | Telegram bot config (token, allowed users) |

**Data storage**: `data_dir` in vpn-director.json (default: `/opt/vpn-director/data`) — servers.json, ipset dumps

## Shell Conventions

- Shebang: `#!/usr/bin/env bash` with `set -euo pipefail`
- Debug: `DEBUG=1 ./script.sh` enables tracing with informative PS4
- Conditionals: Use `[[ ]]` instead of `[ ]`
- Logging: `log -l ERROR|WARN|INFO|DEBUG|TRACE "message"`

## Modular Docs

See `.claude/rules/` for detailed docs:
- `packet-flow.md` — packet processing order, Xray vs TD priority, fwmark bit layout
- `tunnel-director.md` — rule format, chain architecture, fwmark layout
- `ipset-builder.md` — IPdeny sources, dump/restore, combo sets
- `xray-tproxy.md` — TPROXY chain, exclusions, fail-safe
- `shell-conventions.md` — utilities from lib/common.sh, lib/firewall.sh, **known pitfalls**
- `testing.md` — Bats framework, mocks, fixtures
- `telegram-bot.md` — Go bot architecture, commands, wizard flow
- `entware-init.md` — Entware init system (rc.unslung, rc.func, S* scripts)
