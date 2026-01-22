# VPN Director for Asuswrt-Merlin

Selective traffic routing through Xray TPROXY and OpenVPN tunnels.

## Features

- **Xray TPROXY**: Transparent proxy for selected LAN clients via VLESS
- **Tunnel Director**: Route traffic through OpenVPN/WireGuard by destination
- **Country-based routing**: Exclude Russian IPs from proxy/tunnel
- **Telegram Bot**: Remote management via Telegram (status, config, restart)
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
   /opt/vpn-director/import_server_list.sh
   ```

2. Run the configuration wizard:
   ```bash
   /opt/vpn-director/configure.sh
   ```

3. Setup Telegram bot (optional):
   ```bash
   /opt/vpn-director/setup_telegram_bot.sh
   ```

## Requirements

- Asuswrt-Merlin firmware
- Entware installed
- Required packages:
  ```bash
  opkg install curl coreutils-base64 coreutils-sha256sum gawk jq xray-core procps-ng-pgrep
  ```
- OpenVPN client configured in router UI (for Tunnel Director)

### Optional

- `opkg install wget-ssl` — faster and more reliable downloads for country zone files (recommended)
- `opkg install openssl-util` — for email notifications
- `opkg install monit` — for automatic Xray restart on crash (see [Process Monitoring](#process-monitoring))
- `opkg install coreutils-tr` — fixes buggy `tr` command (stock busybox `tr` corrupts characters with certain locales)

## Manual Configuration

After installation, configs are located at:

- `/opt/vpn-director/vpn-director.json` - Unified config (Xray + Tunnel Director)
- `/opt/etc/xray/config.json` - Xray server configuration

## Commands

```bash
# VPN Director CLI
/opt/vpn-director/vpn-director.sh status              # Show all status
/opt/vpn-director/vpn-director.sh apply               # Apply configuration
/opt/vpn-director/vpn-director.sh stop                # Stop all components
/opt/vpn-director/vpn-director.sh restart             # Restart all
/opt/vpn-director/vpn-director.sh update              # Update ipsets + reapply

# Component-specific
/opt/vpn-director/vpn-director.sh status tunnel       # Tunnel Director status only
/opt/vpn-director/vpn-director.sh restart xray        # Restart Xray TPROXY only

# Import servers
/opt/vpn-director/import_server_list.sh
```

## Telegram Bot

Remote management via Telegram with username-based authorization.

### Setup

1. Create a bot via [@BotFather](https://t.me/BotFather) and get the token
2. Run setup script:
   ```bash
   /opt/vpn-director/setup_telegram_bot.sh
   ```
3. Enter bot token and allowed usernames (without @)

### Bot Commands

| Command | Description |
|---------|-------------|
| `/status` | VPN Director status |
| `/servers` | Server list |
| `/import <url>` | Import VLESS subscription |
| `/configure` | Configuration wizard |
| `/restart` | Restart VPN Director |
| `/stop` | Stop VPN Director |
| `/logs [bot\|vpn\|all] [N]` | Recent logs (default: all, 20 lines) |
| `/ip` | External IP |
| `/version` | Bot version |

### Configuration Wizard

The `/configure` command starts a 4-step wizard:
1. Select Xray server
2. Choose country exclusions (ru, ua, etc.)
3. Add LAN clients with routing (Xray/OpenVPN/WireGuard)
4. Review and apply

## How It Works

### Xray TPROXY

Traffic from specified LAN clients is transparently redirected through Xray using TPROXY. The proxy uses VLESS protocol over TLS to connect to your VPN server.

### Tunnel Director

Routes traffic from specified LAN clients through OpenVPN/WireGuard tunnels based on destination. By default, excludes Russian IPs to allow direct access to local services.

### Country IPSets

Country IP lists are downloaded automatically from multiple sources with fallback:
1. GeoLite2 via GitHub (firehol/blocklist-ipsets) — most accurate
2. IPDeny via GitHub mirror — not blocked in most regions
3. IPDeny direct — may be blocked in some regions
4. Manual fallback — interactive prompt if all sources fail

## Startup Scripts

This project uses Entware init.d for automatic startup:

| Script | When Called | Purpose |
|--------|-------------|---------|
| `/opt/etc/init.d/S99vpn-director` | After Entware initialized | Runs `vpn-director.sh apply` to initialize all components |
| `/jffs/scripts/firewall-start` | After firewall rules applied | Reapplies configuration after firewall reload |
| `/jffs/scripts/wan-event` | On WAN events | Handles WAN connection changes |

**Note:** The init.d script ensures Entware bash is available before running vpn-director scripts.

To enable user scripts: Administration -> System -> Enable JFFS custom scripts and configs -> Yes

## Process Monitoring

Xray and Telegram bot may occasionally crash. Use monit for automatic restart.

### Setup

1. Install monit:
   ```bash
   opkg install monit
   ```

2. Create configs in `/opt/etc/monit.d/`:

   **xray:**
   ```
   check process xray matching "xray"
       start program = "/opt/etc/init.d/S24xray start"
       stop program = "/opt/etc/init.d/S24xray stop"
       if does not exist then restart
   ```

   **telegram-bot:**
   ```
   check process telegram-bot matching "telegram-bot"
       start program = "/opt/etc/init.d/S98telegram-bot start"
       stop program = "/opt/etc/init.d/S98telegram-bot stop"
       if does not exist then restart
   ```

3. Enable config directory in `/opt/etc/monitrc`:
   ```
   include /opt/etc/monit.d/*
   ```

4. Edit `/opt/etc/monitrc`, set check interval:
   ```
   set daemon 30    # check every 30 seconds
   ```

5. Restart monit:
   ```bash
   /opt/etc/init.d/S99monit restart
   ```

6. Verify:
   ```bash
   monit status
   ```

## License

MIT
