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
   /jffs/scripts/vpn-director/import_server_list.sh
   ```

2. Run the configuration wizard:
   ```bash
   /jffs/scripts/vpn-director/configure.sh
   ```

3. Setup Telegram bot (optional):
   ```bash
   /jffs/scripts/vpn-director/setup_telegram_bot.sh
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
- `opkg install jq` — for Telegram bot setup script
- `opkg install monit` — for automatic Xray restart on crash (see [Process Monitoring](#process-monitoring))
- `opkg install coreutils-tr` — fixes buggy `tr` command (stock busybox `tr` corrupts characters with certain locales)

## Manual Configuration

After installation, configs are located at:

- `/jffs/scripts/vpn-director/vpn-director.json` - Unified config (Xray + Tunnel Director)
- `/opt/etc/xray/config.json` - Xray server configuration

## Commands

```bash
# VPN Director CLI
/jffs/scripts/vpn-director/vpn-director.sh status              # Show all status
/jffs/scripts/vpn-director/vpn-director.sh apply               # Apply configuration
/jffs/scripts/vpn-director/vpn-director.sh stop                # Stop all components
/jffs/scripts/vpn-director/vpn-director.sh restart             # Restart all
/jffs/scripts/vpn-director/vpn-director.sh update              # Update ipsets + reapply

# Component-specific
/jffs/scripts/vpn-director/vpn-director.sh status tunnel       # Tunnel Director status only
/jffs/scripts/vpn-director/vpn-director.sh restart xray        # Restart Xray TPROXY only

# Import servers
/jffs/scripts/vpn-director/import_server_list.sh

# Shell aliases (after install)
vpd status
vpd apply
ipt           # Legacy alias (runs: vpd update)
```

## Telegram Bot

Remote management via Telegram with username-based authorization.

### Setup

1. Create a bot via [@BotFather](https://t.me/BotFather) and get the token
2. Run setup script:
   ```bash
   /jffs/scripts/vpn-director/setup_telegram_bot.sh
   ```
3. Enter bot token and allowed usernames (without @)

### Bot Commands

| Command | Description |
|---------|-------------|
| `/status` | Xray status |
| `/servers` | List imported servers |
| `/import <url>` | Import VLESS subscription |
| `/configure` | Interactive configuration wizard |
| `/restart` | Restart Xray |
| `/stop` | Stop Xray |
| `/logs` | Recent log entries |
| `/ip` | Show external IP |

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

Routes traffic from specified LAN clients through OpenVPN tunnels based on destination. By default, excludes Russian IPs to allow direct access to local services.

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
