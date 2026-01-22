#!/usr/bin/env bash
set -euo pipefail

JFFS_DIR="/jffs/scripts/vpn-director"
CONFIG_FILE="$JFFS_DIR/telegram-bot.json"

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

# Create JSON
USERS_JSON=$(printf '%s\n' "${USERS[@]}" | jq -R . | jq -s .)

jq -n \
    --arg token "$BOT_TOKEN" \
    --argjson users "$USERS_JSON" \
    '{bot_token: $token, allowed_users: $users}' > "$CONFIG_FILE"

echo
echo "Config created: $CONFIG_FILE"

# Restart bot via init script
if [[ -x /opt/etc/init.d/S98telegram-bot ]]; then
    /opt/etc/init.d/S98telegram-bot restart
fi

echo
echo "Done! Send /start to the bot"
