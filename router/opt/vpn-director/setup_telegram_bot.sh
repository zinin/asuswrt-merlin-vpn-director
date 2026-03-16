#!/usr/bin/env bash
set -euo pipefail

VPD_DIR="/opt/vpn-director"
CONFIG_FILE="$VPD_DIR/telegram-bot.json"

echo "Telegram Bot Setup"
echo "=================="
echo

# Check for jq
if ! command -v jq &> /dev/null; then
    echo "Error: jq not installed. Install via opkg install jq"
    exit 1
fi

# Bot token
printf "Enter bot token: "
read -r BOT_TOKEN < /dev/tty

if [[ -z "$BOT_TOKEN" ]]; then
    echo "Error: token cannot be empty"
    exit 1
fi

# Users
USERS=()
while true; do
    printf "Enter username (without @): "
    read -r USERNAME < /dev/tty

    if [[ -n "$USERNAME" ]]; then
        USERS+=("$USERNAME")
    fi

    printf "Add another? [y/N]: "
    read -r REPLY < /dev/tty
    case "$REPLY" in
        [Yy]*) continue ;;
        *) break ;;
    esac
done

if [[ ${#USERS[@]} -eq 0 ]]; then
    echo "Error: add at least one user"
    exit 1
fi

# Proxy configuration
echo
printf "Use proxy for Telegram API? (recommended if Telegram is blocked)\n"
printf "  1) Yes - use Xray SOCKS5 proxy\n"
printf "  2) No - direct connection\n"
printf "Choice [2]: "
read -r PROXY_CHOICE < /dev/tty

PROXY_URL=""
PROXY_FALLBACK=false
if [[ "${PROXY_CHOICE:-2}" == "1" ]]; then
    # Try to read socks_port from vpn-director.json
    SOCKS_PORT=12346
    if [[ -f "$VPD_DIR/vpn-director.json" ]]; then
        CONFIGURED_PORT=$(jq -r '.advanced.xray.socks_port // empty' "$VPD_DIR/vpn-director.json" 2>/dev/null)
        if [[ -n "${CONFIGURED_PORT:-}" ]]; then
            SOCKS_PORT="$CONFIGURED_PORT"
        fi
    fi
    PROXY_URL="socks5://127.0.0.1:${SOCKS_PORT}"
    PROXY_FALLBACK=true
    echo "Proxy: $PROXY_URL (with direct fallback)"
fi

# Create JSON
USERS_JSON=$(printf '%s\n' "${USERS[@]}" | jq -R . | jq -s .)

jq -n \
    --arg token "$BOT_TOKEN" \
    --argjson users "$USERS_JSON" \
    --arg proxy "$PROXY_URL" \
    --argjson fallback "$PROXY_FALLBACK" \
    '{bot_token: $token, allowed_users: $users, log_level: "info"} +
     (if $proxy != "" then {proxy: $proxy, proxy_fallback_direct: $fallback} else {} end)' > "$CONFIG_FILE"

echo
echo "Config created: $CONFIG_FILE"

# Restart bot via init script
if [[ -x /opt/etc/init.d/S98telegram-bot ]]; then
    /opt/etc/init.d/S98telegram-bot restart
fi

echo
echo "Done! Send /start to the bot"
