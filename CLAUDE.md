# VPN Director for Asuswrt-Merlin

Traffic routing system for Asus routers: Xray TPROXY, Tunnel Director, IPSet Builder.

## Commands

```bash
# Install
curl -fsSL https://raw.githubusercontent.com/zinin/asuswrt-merlin-vpn-director/master/install.sh | sh

# Xray TPROXY
/jffs/scripts/vpn-director/xray_tproxy.sh status|start|stop|restart

# IPSet Builder
/jffs/scripts/vpn-director/ipset_builder.sh       # Restore from cache
/jffs/scripts/vpn-director/ipset_builder.sh -u    # Force rebuild
/jffs/scripts/vpn-director/ipset_builder.sh -t    # Rebuild + Tunnel Director
/jffs/scripts/vpn-director/ipset_builder.sh -x    # Rebuild + Xray TPROXY

# Tunnel Director
/jffs/scripts/vpn-director/tunnel_director.sh

# Shell alias
ipt  # Runs: ipset_builder.sh -t
```

## Architecture

| Path | Purpose |
|------|---------|
| `jffs/scripts/vpn-director/` | Main scripts: ipset_builder, tunnel_director, xray_tproxy, configure |
| `jffs/scripts/vpn-director/utils/` | Shared utilities: common.sh, firewall.sh, shared.sh, send-email.sh |
| `jffs/scripts/vpn-director/configs/` | Config templates (*.sh.template) |
| `jffs/configs/profile.add` | Shell alias for `ipt` command |
| `config/xray-config.json.template` | Xray server config template |
| `install.sh` | Interactive installer |

## Key Concepts

**Tunnel Director Rules**: `table:src[%iface][:src_excl]:set[,set...][:set_excl]`
- Example: `wgc1:192.168.50.0/24::us,ca` — route US/CA traffic via wgc1

**IPSet Types**:
- Country sets: 2-letter ISO codes (us, ca, ru) from IPdeny
- Combo sets: union of multiple sets

## Config Files (after install)

| Path | Purpose |
|------|---------|
| `/jffs/scripts/vpn-director/configs/config-tunnel-director.sh` | Tunnel Director rules & IPSet Builder settings |
| `/jffs/scripts/vpn-director/configs/config-xray.sh` | Xray TPROXY clients & servers |
| `/opt/etc/xray/config.json` | Xray server configuration |

**User dump storage**: `IPS_BDR_DIR` in config-tunnel-director.sh (default: `/jffs/ipset_builder`)

## Shell Conventions

- Shebang: `#!/usr/bin/env ash` with `set -euo pipefail`
- Logging: `log -l err|warn|info|notice "message"`

## Modular Docs

See `.claude/rules/` for detailed docs:
- `tunnel-director.md` — rule format, chain architecture, fwmark layout
- `ipset-builder.md` — IPdeny sources, dump/restore, combo sets
- `xray-tproxy.md` — TPROXY chain, exclusions, fail-safe
- `shell-conventions.md` — utilities from common.sh/firewall.sh
