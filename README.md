# VPN Director for Asuswrt-Merlin

Selective traffic routing through Xray TPROXY and OpenVPN tunnels.

## Features

- **Xray TPROXY**: Transparent proxy for selected LAN clients via VLESS
- **Tunnel Director**: Route traffic through OpenVPN/WireGuard by destination
- **Country-based routing**: Exclude Russian IPs from proxy/tunnel
- **Easy installation**: One-command setup with interactive configuration

## Quick Install

```bash
curl -fsSL https://raw.githubusercontent.com/zinin/asuswrt-merlin-vpn-director/master/install.sh | sh
```

## Requirements

- Asuswrt-Merlin firmware
- Entware installed
- Xray installed (`opkg install xray`)
- OpenVPN client configured in router UI (for Tunnel Director)

## Manual Configuration

After installation, configs are located at:

- `/jffs/scripts/xray/config.sh` - Xray clients and servers
- `/jffs/scripts/firewall/config.sh` - Tunnel Director rules
- `/opt/etc/xray/config.json` - Xray server configuration

## Commands

```bash
# Check Xray TPROXY status
/jffs/scripts/xray/xray_tproxy.sh status

# Restart Xray TPROXY
/jffs/scripts/xray/xray_tproxy.sh restart

# Rebuild ipsets
/jffs/scripts/firewall/ipset_builder.sh

# Reapply Tunnel Director rules
/jffs/scripts/firewall/tunnel_director.sh
```

## How It Works

### Xray TPROXY

Traffic from specified LAN clients is transparently redirected through Xray using TPROXY. The proxy uses VLESS protocol over TLS to connect to your VPN server.

### Tunnel Director

Routes traffic from specified LAN clients through OpenVPN tunnels based on destination. By default, excludes Russian IPs to allow direct access to local services.

## License

MIT
