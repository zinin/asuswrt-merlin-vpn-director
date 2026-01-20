# VPN Director for Asuswrt-Merlin

Traffic routing system for Asus routers: Xray TPROXY, Tunnel Director, IPSet Builder.

## Commands

```bash
# Install
curl -fsSL https://raw.githubusercontent.com/zinin/asuswrt-merlin-vpn-director/master/install.sh | bash

# VPN Director CLI
/jffs/scripts/vpn-director/vpn-director.sh status              # Show all status
/jffs/scripts/vpn-director/vpn-director.sh apply               # Apply configuration
/jffs/scripts/vpn-director/vpn-director.sh stop                # Stop all components
/jffs/scripts/vpn-director/vpn-director.sh restart             # Restart all
/jffs/scripts/vpn-director/vpn-director.sh update              # Update ipsets + reapply

# Component-specific commands
/jffs/scripts/vpn-director/vpn-director.sh status tunnel       # Tunnel Director status only
/jffs/scripts/vpn-director/vpn-director.sh restart xray        # Restart Xray TPROXY only

# Import servers
/jffs/scripts/vpn-director/import_server_list.sh

# Shell aliases
vpd status    # Short form via alias
vpd apply
vpd update
ipt           # Legacy alias (runs: vpd update)
```

## Architecture

| Path | Purpose |
|------|---------|
| `jffs/scripts/vpn-director/vpn-director.sh` | Unified CLI entry point |
| `jffs/scripts/vpn-director/lib/` | Modules: common, firewall, config, ipset, tunnel, tproxy |
| `opt/etc/init.d/S99vpn-director` | Entware init.d script for startup |
| `jffs/scripts/firewall-start` | Asuswrt-Merlin hook for firewall reload |
| `jffs/scripts/wan-event` | Asuswrt-Merlin hook for WAN events |
| `test/` | Bats tests (unit/, integration/) |
| `jffs/scripts/vpn-director/vpn-director.json.template` | Unified config template |
| `jffs/configs/profile.add` | Shell alias for `vpd` command |
| `config/xray.json.template` | Xray server config template |
| `install.sh` | Interactive installer |
| `telegram-bot/` | Go-based Telegram bot for remote management |
| `setup_telegram_bot.sh` | Bot configuration script |

## Key Concepts

**Tunnel Director Rules**: `table:src[%iface][:src_excl]:set[,set...][:set_excl]`
- Example: `wgc1:192.168.50.0/24::us,ca` — route US/CA traffic via wgc1

**IPSet Types**:
- Country sets: 2-letter ISO codes (us, ca, ru) from IPdeny
- Combo sets: union of multiple sets

## Config Files (after install)

| Path | Purpose |
|------|---------|
| `/jffs/scripts/vpn-director/vpn-director.json` | Unified config (Xray + Tunnel Director) |
| `/opt/etc/xray/config.json` | Xray server configuration |
| `/jffs/scripts/vpn-director/telegram-bot.json` | Telegram bot config (token, allowed users) |

**Data storage**: `data_dir` in vpn-director.json (default: `/jffs/scripts/vpn-director/data`) — servers.json, ipset dumps

## Shell Conventions

- Shebang: `#!/usr/bin/env bash` with `set -euo pipefail`
- Debug: `DEBUG=1 ./script.sh` enables tracing with informative PS4
- Conditionals: Use `[[ ]]` instead of `[ ]`
- Logging: `log -l ERROR|WARN|INFO|DEBUG|TRACE "message"`

## Modular Docs

See `.claude/rules/` for detailed docs:
- `tunnel-director.md` — rule format, chain architecture, fwmark layout
- `ipset-builder.md` — IPdeny sources, dump/restore, combo sets
- `xray-tproxy.md` — TPROXY chain, exclusions, fail-safe
- `shell-conventions.md` — utilities from lib/common.sh, lib/firewall.sh, **known pitfalls**
- `testing.md` — Bats framework, mocks, fixtures
- `telegram-bot.md` — Go bot architecture, commands, wizard flow
- `entware-init.md` — Entware init system (rc.unslung, rc.func, S* scripts)
