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

After installation, run the configuration wizard:

```bash
/jffs/scripts/utils/configure.sh
```

## Requirements

- Asuswrt-Merlin firmware
- Entware installed
- Required packages:
  ```bash
  opkg install curl coreutils-base64 coreutils-sha256sum xray-core
  ```
- OpenVPN client configured in router UI (for Tunnel Director)

### Optional

- `opkg install openssl-util` — for email notifications

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

## Startup Scripts

This project uses [Asuswrt-Merlin user scripts](https://github.com/RMerl/asuswrt-merlin.ng/wiki/User-scripts)
for automatic startup:

| Script | When Called | Purpose |
|--------|-------------|---------|
| `services-start` | After all services started at boot | Builds ipsets, starts Xray TPROXY |
| `firewall-start` | After firewall rules applied | Applies Tunnel Director rules |

**Note:** Installation overwrites these files. If you have custom logic,
back up your scripts before installing.

To enable user scripts: Administration -> System -> Enable JFFS custom scripts and configs -> Yes

## License

MIT
