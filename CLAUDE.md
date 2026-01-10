# VPN Director for Asuswrt-Merlin

Traffic routing system for Asus routers: Xray TPROXY, Tunnel Director, IPSet Builder.

## Commands

```bash
# Install
curl -fsSL https://raw.githubusercontent.com/zinin/asuswrt-merlin-vpn-director/master/install.sh | sh

# Xray TPROXY
/jffs/scripts/xray/xray_tproxy.sh status|start|stop|restart

# IPSet Builder
/jffs/scripts/firewall/ipset_builder.sh       # Restore from cache
/jffs/scripts/firewall/ipset_builder.sh -u    # Force rebuild
/jffs/scripts/firewall/ipset_builder.sh -t    # Rebuild + Tunnel Director
/jffs/scripts/firewall/ipset_builder.sh -x    # Rebuild + Xray TPROXY

# Tunnel Director
/jffs/scripts/firewall/tunnel_director.sh

# Shell alias
ipt  # Runs: ipset_builder.sh -t
```

## Architecture

| Path | Purpose |
|------|---------|
| `jffs/scripts/firewall/` | IPSet Builder, Tunnel Director, config templates |
| `jffs/scripts/xray/` | Xray TPROXY management |
| `jffs/scripts/utils/` | Shared utilities: logging, locking, firewall helpers |
| `jffs/configs/` | profile.add (shell alias) |
| `config/` | Xray server config template |
| `install.sh` | Interactive installer |

## Key Concepts

**Tunnel Director Rules**: `table:src[%iface][:src_excl]:set[,set...][:set_excl]`
- Example: `wgc1:192.168.50.0/24::us,ca` — route US/CA traffic via wgc1

**IPSet Types**:
- Country sets: 2-letter ISO codes (us, ca, ru) from IPdeny
- Combo sets: union of multiple sets

## Config Files (after install)

```
/jffs/scripts/firewall/config.sh  # Tunnel Director & IPSet
/jffs/scripts/xray/config.sh      # Xray TPROXY
/opt/etc/xray/config.json         # Xray server
```

## Shell Conventions

- Shebang: `#!/usr/bin/env ash` with `set -euo pipefail`
- Logging: `log -l err|warn|info|notice "message"`

## Modular Docs

See `.claude/rules/` for detailed docs:
- `tunnel-director.md` — rule format, chain architecture, fwmark layout
- `ipset-builder.md` — IPdeny sources, dump/restore, combo sets
- `xray-tproxy.md` — TPROXY chain, exclusions, fail-safe
- `shell-conventions.md` — utilities from common.sh/firewall.sh
