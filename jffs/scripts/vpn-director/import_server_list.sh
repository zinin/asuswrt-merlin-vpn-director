#!/bin/sh
set -e

###############################################################################
# import_server_list.sh - Import VLESS servers from base64-encoded file/URL
# Run after install.sh to download and parse server list
###############################################################################

# Source common utilities
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=utils/common.sh
. "$SCRIPT_DIR/utils/common.sh"

# Paths
JFFS_DIR="/jffs/scripts/vpn-director"
VPD_CONFIG="$JFFS_DIR/vpn-director.json"
VPD_TEMPLATE="$JFFS_DIR/vpn-director.json.template"

###############################################################################
# Helper functions
###############################################################################

read_input() {
    printf "%s: " "$1" >&2
    read -r INPUT_RESULT
}

###############################################################################
# VLESS URI Parser
###############################################################################

# Parse a single VLESS URI and extract components
# Format: vless://uuid@server:port?params#name
# Output: pipe-separated fields
parse_vless_uri() {
    uri="$1"

    # Remove vless:// prefix
    rest="${uri#vless://}"

    # Extract name (after #, URL-decoded)
    raw_name="${rest##*#}"
    raw_name=$(printf '%s' "$raw_name" | sed 's/%20/ /g; s/%2F/\//g; s/+/ /g')
    # Remove non-ASCII bytes (emoji, etc), keep only printable ASCII
    # Then trim leading/trailing spaces and commas
    name=$(printf '%s' "$raw_name" | tr -cd '\11\12\15\40-\176' | sed 's/^[[:space:],]*//; s/[[:space:],]*$//')
    rest="${rest%%#*}"

    # Extract UUID (before @)
    uuid="${rest%%@*}"
    rest="${rest#*@}"

    # Extract server:port (before ?)
    server_port="${rest%%\?*}"
    server="${server_port%%:*}"
    port="${server_port##*:}"

    # Fallback: if name is empty or just punctuation, use server hostname
    clean_name=$(printf '%s' "$name" | sed 's/[^a-zA-Z0-9]//g')
    if [ -z "$clean_name" ]; then
        name="$server"
    fi

    printf '%s|%s|%s|%s\n' "$server" "$port" "$uuid" "$name"
}

###############################################################################
# Get data directory from config
###############################################################################

get_data_dir() {
    local config_file="$VPD_CONFIG"

    # Fall back to template if config doesn't exist
    if [ ! -f "$config_file" ]; then
        config_file="$VPD_TEMPLATE"
    fi

    if [ ! -f "$config_file" ]; then
        log -l err "Config not found: $VPD_CONFIG or $VPD_TEMPLATE"
        exit 1
    fi

    jq -r '.tunnel_director.data_dir // "/jffs/scripts/vpn-director/data"' "$config_file"
}

###############################################################################
# Step 1: Get VLESS file
###############################################################################

step_get_vless_file() {
    log -l notice "Step 1: VLESS Server List"

    printf "Enter path to VLESS file or URL:\n"
    printf "(File should contain base64-encoded VLESS URIs)\n\n"

    read_input "Path or URL"
    VLESS_INPUT="$INPUT_RESULT"

    if [ -z "$VLESS_INPUT" ]; then
        log -l err "No input provided"
        exit 1
    fi

    # Check if it's a URL or file path
    case "$VLESS_INPUT" in
        http://*|https://*)
            log "Downloading from URL..."
            VLESS_CONTENT=$(curl -fsSL "$VLESS_INPUT") || {
                log -l err "Failed to download from $VLESS_INPUT"
                exit 1
            }
            ;;
        *)
            if [ ! -f "$VLESS_INPUT" ]; then
                log -l err "File not found: $VLESS_INPUT"
                exit 1
            fi
            VLESS_CONTENT=$(cat "$VLESS_INPUT")
            ;;
    esac

    # Decode base64
    VLESS_DECODED=$(printf '%s' "$VLESS_CONTENT" | base64 -d 2>/dev/null) || {
        log -l err "Failed to decode base64 content"
        exit 1
    }

    # Count servers
    SERVER_COUNT=$(printf '%s\n' "$VLESS_DECODED" | grep -c '^vless://' || true)

    if [ "$SERVER_COUNT" -eq 0 ]; then
        log -l err "No VLESS servers found in file"
        exit 1
    fi

    log "Found $SERVER_COUNT VLESS servers"
    VLESS_SERVERS="$VLESS_DECODED"
}

###############################################################################
# Step 2: Parse and save servers
###############################################################################

step_parse_and_save_servers() {
    log -l notice "Step 2: Parsing Servers"

    DATA_DIR=$(get_data_dir)
    SERVERS_FILE="$DATA_DIR/servers.json"

    # Ensure data directory exists
    mkdir -p "$DATA_DIR"

    # Parse servers and build JSON
    printf '%s\n' "$VLESS_SERVERS" | grep '^vless://' | while IFS= read -r uri; do
        log -l debug "URI: ${uri%%#*}"

        parsed=$(parse_vless_uri "$uri")
        server=$(printf '%s' "$parsed" | cut -d'|' -f1)
        port=$(printf '%s' "$parsed" | cut -d'|' -f2)
        uuid=$(printf '%s' "$parsed" | cut -d'|' -f3)
        name=$(printf '%s' "$parsed" | cut -d'|' -f4)

        log -l debug "Parsed: server=$server port=$port uuid=$uuid name=$name"

        # Validate required fields
        if [ -z "$server" ] || [ -z "$port" ] || [ -z "$uuid" ]; then
            log -l debug "SKIP: missing required field"
            log -l warn "Skipping invalid URI (missing server/port/uuid)"
            continue
        fi

        # Validate port is numeric
        if ! printf '%s' "$port" | grep -qE '^[0-9]+$'; then
            printf "[DEBUG] SKIP: invalid port '%s'\n" "$port" >> "$LOG_FILE"
            log -l warn "Skipping $server: invalid port '$port'"
            continue
        fi

        # Resolve IP using common.sh resolve_ip (tries IPv4 first, then IPv6)
        ip=$(resolve_ip -q "$server" 2>/dev/null) || ip=$(resolve_ip -6 -g -q "$server" 2>/dev/null) || ip=""

        if [ -z "$ip" ]; then
            printf "[DEBUG] SKIP: cannot resolve %s\n" "$server" >> "$LOG_FILE"
            log -l warn "Cannot resolve $server, skipping"
            continue
        fi

        printf "[DEBUG] Resolved: %s -> %s\n" "$server" "$ip" >> "$LOG_FILE"
        printf "  %s (%s) -> %s\n" "$name" "$server" "$ip" >&2

        # Output JSON line (use jq to properly escape strings)
        printf '%s\n%s\n%s\n%s\n%s\n' "$server" "$port" "$uuid" "$name" "$ip"
    done | {
        # Build JSON from piped data using jq for proper escaping
        printf '[\n'
        first=1
        while IFS= read -r server && IFS= read -r port && IFS= read -r uuid && IFS= read -r name && IFS= read -r ip; do
            [ -z "$server" ] && continue
            [ "$first" -eq 0 ] && printf ',\n'
            first=0
            # Use jq to create properly escaped JSON object
            jq -n \
                --arg addr "$server" \
                --arg port "$port" \
                --arg uuid "$uuid" \
                --arg name "$name" \
                --arg ip "$ip" \
                '{address: $addr, port: ($port | tonumber), uuid: $uuid, name: $name, ip: $ip}' | tr -d '\n'
        done
        printf '\n]\n'
    } > "$SERVERS_FILE"

    # Validate JSON
    if ! jq empty "$SERVERS_FILE" 2>/dev/null; then
        log -l err "Generated invalid JSON"
        cat "$SERVERS_FILE"
        exit 1
    fi

    SERVER_COUNT=$(jq length "$SERVERS_FILE")

    if [ "$SERVER_COUNT" -eq 0 ]; then
        log -l err "No servers could be resolved"
        rm -f "$SERVERS_FILE"
        exit 1
    fi

    log "Saved $SERVER_COUNT servers to $SERVERS_FILE"
}

###############################################################################
# Step 3: Update vpn-director.json
###############################################################################

step_update_config() {
    log -l notice "Step 3: Updating Configuration"

    DATA_DIR=$(get_data_dir)
    SERVERS_FILE="$DATA_DIR/servers.json"

    # Extract all IPs from servers.json
    server_ips=$(jq -r '.[].ip' "$SERVERS_FILE" | sort -u)

    # Build JSON array of IPs
    xray_servers_json=$(printf '%s\n' $server_ips | jq -R . | jq -s .)

    # Create config from template if doesn't exist
    if [ ! -f "$VPD_CONFIG" ]; then
        if [ ! -f "$VPD_TEMPLATE" ]; then
            log -l err "Template not found: $VPD_TEMPLATE"
            exit 1
        fi
        cp "$VPD_TEMPLATE" "$VPD_CONFIG"
        log "Created $VPD_CONFIG from template"
    fi

    # Update xray.servers in config
    jq --argjson servers "$xray_servers_json" \
        '.xray.servers = $servers' \
        "$VPD_CONFIG" > "${VPD_CONFIG}.tmp" && \
        mv "${VPD_CONFIG}.tmp" "$VPD_CONFIG"

    ip_count=$(printf '%s\n' $server_ips | wc -l | tr -d ' ')
    log "Updated xray.servers with $ip_count IP addresses"
}

###############################################################################
# Main
###############################################################################

main() {
    log -l notice "Import VLESS Server List"
    printf "This will download and parse VLESS servers.\n\n"

    step_get_vless_file
    step_parse_and_save_servers
    step_update_config

    log -l notice "Import Complete"
    printf "Server list saved. Run configure.sh to continue setup.\n"
}

main "$@"
