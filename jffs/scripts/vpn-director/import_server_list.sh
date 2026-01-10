#!/bin/sh
set -e

###############################################################################
# import_server_list.sh - Import VLESS servers from base64-encoded file/URL
# Run after install.sh to download and parse server list
###############################################################################

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Paths
JFFS_DIR="/jffs/scripts/vpn-director"
VPD_CONFIG="$JFFS_DIR/vpn-director.json"
VPD_TEMPLATE="$JFFS_DIR/vpn-director.json.template"

###############################################################################
# Helper functions
###############################################################################

print_header() {
    printf "\n${BLUE}=== %s ===${NC}\n\n" "$1"
}

print_success() {
    printf "${GREEN}[OK]${NC} %s\n" "$1"
}

print_error() {
    printf "${RED}[ERROR]${NC} %s\n" "$1"
}

print_warning() {
    printf "${YELLOW}[WARN]${NC} %s\n" "$1"
}

print_info() {
    printf "${BLUE}[INFO]${NC} %s\n" "$1"
}

read_input() {
    printf "%s: " "$1"
    read -r INPUT_RESULT
}

###############################################################################
# VLESS URI Parser
###############################################################################

# Parse a single VLESS URI and extract components
# Format: vless://uuid@server:port?params#name
# Output: JSON object
parse_vless_uri() {
    uri="$1"

    # Remove vless:// prefix
    rest="${uri#vless://}"

    # Extract name (after #, URL-decoded)
    name="${rest##*#}"
    name=$(printf '%s' "$name" | sed 's/%20/ /g; s/%2F/\//g; s/+/ /g')
    # Remove emoji and other 3-4 byte UTF-8 chars
    name=$(printf '%s' "$name" | awk '{
        gsub(/[\360-\364][\200-\277][\200-\277][\200-\277]/, "")
        gsub(/[\340-\357][\200-\277][\200-\277][\200-\277]/, "")
        print
    }' | sed 's/^[[:space:]]*//')
    rest="${rest%%#*}"

    # Extract UUID (before @)
    uuid="${rest%%@*}"
    rest="${rest#*@}"

    # Extract server:port (before ?)
    server_port="${rest%%\?*}"
    server="${server_port%%:*}"
    port="${server_port##*:}"

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
        print_error "Config not found: $VPD_CONFIG or $VPD_TEMPLATE"
        exit 1
    fi

    jq -r '.tunnel_director.data_dir // "/jffs/scripts/vpn-director/data"' "$config_file"
}

###############################################################################
# Step 1: Get VLESS file
###############################################################################

step_get_vless_file() {
    print_header "Step 1: VLESS Server List"

    printf "Enter path to VLESS file or URL:\n"
    printf "(File should contain base64-encoded VLESS URIs)\n\n"

    read_input "Path or URL"
    VLESS_INPUT="$INPUT_RESULT"

    if [ -z "$VLESS_INPUT" ]; then
        print_error "No input provided"
        exit 1
    fi

    # Check if it's a URL or file path
    case "$VLESS_INPUT" in
        http://*|https://*)
            print_info "Downloading from URL..."
            VLESS_CONTENT=$(curl -fsSL "$VLESS_INPUT") || {
                print_error "Failed to download from $VLESS_INPUT"
                exit 1
            }
            ;;
        *)
            if [ ! -f "$VLESS_INPUT" ]; then
                print_error "File not found: $VLESS_INPUT"
                exit 1
            fi
            VLESS_CONTENT=$(cat "$VLESS_INPUT")
            ;;
    esac

    # Decode base64
    VLESS_DECODED=$(printf '%s' "$VLESS_CONTENT" | base64 -d 2>/dev/null) || {
        print_error "Failed to decode base64 content"
        exit 1
    }

    # Count servers
    SERVER_COUNT=$(printf '%s\n' "$VLESS_DECODED" | grep -c '^vless://' || true)

    if [ "$SERVER_COUNT" -eq 0 ]; then
        print_error "No VLESS servers found in file"
        exit 1
    fi

    print_success "Found $SERVER_COUNT VLESS servers"
    VLESS_SERVERS="$VLESS_DECODED"
}

###############################################################################
# Step 2: Parse and save servers
###############################################################################

step_parse_and_save_servers() {
    print_header "Step 2: Parsing Servers"

    DATA_DIR=$(get_data_dir)
    SERVERS_FILE="$DATA_DIR/servers.json"

    # Ensure data directory exists
    mkdir -p "$DATA_DIR"

    # Parse servers and build JSON
    servers_json="["
    first=1
    resolved=0
    failed=0

    printf '%s\n' "$VLESS_SERVERS" | grep '^vless://' | while IFS= read -r uri; do
        parsed=$(parse_vless_uri "$uri")
        server=$(printf '%s' "$parsed" | cut -d'|' -f1)
        port=$(printf '%s' "$parsed" | cut -d'|' -f2)
        uuid=$(printf '%s' "$parsed" | cut -d'|' -f3)
        name=$(printf '%s' "$parsed" | cut -d'|' -f4)

        # Resolve IP using nslookup
        ip=$(nslookup "$server" 2>/dev/null | awk '/^Address/ && !/^Address:.*#/ { print $2; exit }')

        if [ -z "$ip" ]; then
            print_warning "Cannot resolve $server, skipping"
            continue
        fi

        printf "  %s (%s) -> %s\n" "$name" "$server" "$ip"

        # Output JSON line
        printf '%s|%s|%s|%s|%s\n' "$server" "$port" "$uuid" "$name" "$ip"
    done | {
        # Build JSON from piped data
        first=1
        printf '['
        while IFS='|' read -r server port uuid name ip; do
            [ -z "$server" ] && continue
            [ "$first" -eq 0 ] && printf ','
            first=0
            printf '\n  {"address":"%s","port":%s,"uuid":"%s","name":"%s","ip":"%s"}' \
                "$server" "$port" "$uuid" "$name" "$ip"
        done
        printf '\n]\n'
    } > "$SERVERS_FILE"

    SERVER_COUNT=$(jq length "$SERVERS_FILE")

    if [ "$SERVER_COUNT" -eq 0 ]; then
        print_error "No servers could be resolved"
        rm -f "$SERVERS_FILE"
        exit 1
    fi

    print_success "Saved $SERVER_COUNT servers to $SERVERS_FILE"
}

###############################################################################
# Step 3: Update vpn-director.json
###############################################################################

step_update_config() {
    print_header "Step 3: Updating Configuration"

    DATA_DIR=$(get_data_dir)
    SERVERS_FILE="$DATA_DIR/servers.json"

    # Extract all IPs from servers.json
    server_ips=$(jq -r '.[].ip' "$SERVERS_FILE" | sort -u)

    # Build JSON array of IPs
    xray_servers_json=$(printf '%s\n' $server_ips | jq -R . | jq -s .)

    # Create config from template if doesn't exist
    if [ ! -f "$VPD_CONFIG" ]; then
        if [ ! -f "$VPD_TEMPLATE" ]; then
            print_error "Template not found: $VPD_TEMPLATE"
            exit 1
        fi
        cp "$VPD_TEMPLATE" "$VPD_CONFIG"
        print_info "Created $VPD_CONFIG from template"
    fi

    # Update xray.servers in config
    jq --argjson servers "$xray_servers_json" \
        '.xray.servers = $servers' \
        "$VPD_CONFIG" > "${VPD_CONFIG}.tmp" && \
        mv "${VPD_CONFIG}.tmp" "$VPD_CONFIG"

    ip_count=$(printf '%s\n' $server_ips | wc -l | tr -d ' ')
    print_success "Updated xray.servers with $ip_count IP addresses"
}

###############################################################################
# Main
###############################################################################

main() {
    print_header "Import VLESS Server List"
    printf "This will download and parse VLESS servers.\n\n"

    step_get_vless_file
    step_parse_and_save_servers
    step_update_config

    print_header "Import Complete"
    printf "Server list saved. Run configure.sh to continue setup.\n"
}

main "$@"
