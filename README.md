# VPN Director for Asuswrt-Merlin

Selective traffic routing through Xray TPROXY and OpenVPN tunnels.

## Features

- **Xray TPROXY**: Transparent proxy for selected LAN clients via VLESS
- **Tunnel Director**: Route traffic through OpenVPN/WireGuard by destination
- **Country-based routing**: Exclude Russian IPs from proxy/tunnel
- **Easy installation**: One-command setup with interactive configuration

## Quick Install

```bash
curl -fsSL \
  -H "Cache-Control: no-cache" \
  -H "Pragma: no-cache" \
  -H "If-Modified-Since: Thu, 01 Jan 1970 00:00:00 GMT" \
  "https://raw.githubusercontent.com/zinin/asuswrt-merlin-vpn-director/master/install.sh?v=$(date +%s)" \
| /usr/bin/env bash
```

After installation:

1. Import VLESS servers (optional):
   ```bash
   /jffs/scripts/vpn-director/import_server_list.sh
   ```

2. Run the configuration wizard:
   ```bash
   /jffs/scripts/vpn-director/configure.sh
   ```

## Requirements

- Asuswrt-Merlin firmware
- Entware installed
- Required packages:
  ```bash
  opkg install curl coreutils-base64 coreutils-sha256sum gawk xray-core procps-ng-pgrep
  ```
- OpenVPN client configured in router UI (for Tunnel Director)

### Optional

- `opkg install openssl-util` — for email notifications

## Manual Configuration

After installation, configs are located at:

- `/jffs/scripts/vpn-director/vpn-director.json` - Unified config (Xray + Tunnel Director)
- `/opt/etc/xray/config.json` - Xray server configuration

## Commands

```bash
# Xray TPROXY
/jffs/scripts/vpn-director/xray_tproxy.sh status|start|stop|restart

# IPSet Builder
/jffs/scripts/vpn-director/ipset_builder.sh       # Restore from cache
/jffs/scripts/vpn-director/ipset_builder.sh -u    # Force rebuild

# Tunnel Director
/jffs/scripts/vpn-director/tunnel_director.sh

# Import servers
/jffs/scripts/vpn-director/import_server_list.sh
```

## How It Works

### Xray TPROXY

Traffic from specified LAN clients is transparently redirected through Xray using TPROXY. The proxy uses VLESS protocol over TLS to connect to your VPN server.

### Tunnel Director

Routes traffic from specified LAN clients through OpenVPN tunnels based on destination. By default, excludes Russian IPs to allow direct access to local services.

## Startup Scripts

This project uses Entware init.d for automatic startup:

| Script | When Called | Purpose |
|--------|-------------|---------|
| `/opt/etc/init.d/S99vpn-director` | After Entware initialized | Builds ipsets, starts Xray TPROXY, sets up cron |
| `/jffs/scripts/firewall-start` | After firewall rules applied | Applies Tunnel Director rules (runtime reload) |

**Note:** The init.d script ensures Entware bash is available before running vpn-director scripts.

To enable user scripts: Administration -> System -> Enable JFFS custom scripts and configs -> Yes

## License

MIT
