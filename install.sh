#!/bin/sh
set -e

###############################################################################
# VPN Director Installer for Asuswrt-Merlin
###############################################################################

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Installation paths
JFFS_DIR="/jffs/scripts"
XRAY_CONFIG_DIR="/opt/etc/xray"
REPO_URL="https://raw.githubusercontent.com/zinin/asuswrt-merlin-vpn-director/master"

# Temporary storage for parsed data
VLESS_SERVERS=""
XRAY_CLIENTS_LIST=""
TUN_DIR_RULES_LIST=""
XRAY_SERVERS_IPS=""
SELECTED_SERVER_ADDRESS=""
SELECTED_SERVER_PORT=""
SELECTED_SERVER_UUID=""
SERVERS_TMP="/tmp/vpn_director_servers.tmp"

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

# Read user input with prompt
# Uses /dev/tty to work when script is piped (curl ... | sh)
read_input() {
    printf "%s: " "$1" >/dev/tty
    read -r REPLY </dev/tty
    printf '%s' "$REPLY"
}

# Read yes/no confirmation
confirm() {
    printf "%s [y/n]: " "$1" >/dev/tty
    read -r REPLY </dev/tty
    case "$REPLY" in
        [Yy]*) return 0 ;;
        *) return 1 ;;
    esac
}

###############################################################################
# Environment check
###############################################################################

check_environment() {
    if [ ! -d /jffs ]; then
        print_error "This script must be run on Asuswrt-Merlin router"
        exit 1
    fi

    # Use 'which' instead of 'command -v' - busybox ash doesn't support 'command'
    if ! which curl >/dev/null 2>&1; then
        print_error "curl is required but not installed"
        print_info "Install with: opkg install curl"
        exit 1
    fi

    if ! which nslookup >/dev/null 2>&1; then
        print_error "nslookup is required but not installed"
        exit 1
    fi
}

###############################################################################
# Main installation flow
###############################################################################

main() {
    print_header "VPN Director Installer"
    printf "This installer will configure Xray TPROXY and Tunnel Director\n"
    printf "for selective traffic routing on your Asuswrt-Merlin router.\n\n"

    check_environment

    step_get_vless_file
    step_parse_vless_servers
    step_select_xray_server
    step_configure_clients
    step_show_summary
    step_install_files
    step_apply_rules

    print_header "Installation Complete"
    printf "Check status with: /jffs/scripts/xray/xray_tproxy.sh status\n"
}

###############################################################################
# Step 1: Get VLESS file
###############################################################################

step_get_vless_file() {
    print_header "Step 1: VLESS Server List"

    printf "Enter path to VLESS file or URL:\n"
    printf "(File should contain base64-encoded VLESS URIs)\n\n"

    VLESS_INPUT=$(read_input "Path or URL")

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
# Step 2: Parse VLESS servers
###############################################################################

# Parse a single VLESS URI and extract components
# Format: vless://uuid@server:port?params#name
# Output: server|port|uuid|name
parse_vless_uri() {
    uri="$1"

    # Remove vless:// prefix
    rest="${uri#vless://}"

    # Extract name (after #, URL-decoded)
    name="${rest##*#}"
    name=$(printf '%s' "$name" | sed 's/%20/ /g; s/%2F/\//g; s/+/ /g')
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

step_parse_vless_servers() {
    print_header "Step 2: Parsing Servers"

    # Clear temp file for parsed servers
    : > "$SERVERS_TMP"

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

        # Append to temp file: server|port|uuid|name|ip
        printf '%s|%s|%s|%s|%s\n' "$server" "$port" "$uuid" "$name" "$ip" >> "$SERVERS_TMP"
    done

    SERVER_COUNT=$(wc -l < "$SERVERS_TMP" | tr -d ' ')

    if [ "$SERVER_COUNT" -eq 0 ]; then
        print_error "No servers could be resolved"
        exit 1
    fi

    # Collect all IPs for XRAY_SERVERS ipset
    XRAY_SERVERS_IPS=$(cut -d'|' -f5 "$SERVERS_TMP" | sort -u | tr '\n' ' ')

    print_success "Parsed $SERVER_COUNT servers"
}

###############################################################################
# Step 3: Select Xray server
###############################################################################

step_select_xray_server() {
    print_header "Step 3: Select Xray Server"

    printf "Available servers:\n\n"

    i=1
    while IFS='|' read -r server port uuid name ip; do
        printf "  %2d) %s\n      %s -> %s\n\n" "$i" "$name" "$server" "$ip"
        i=$((i + 1))
    done < "$SERVERS_TMP"

    total=$((i - 1))

    while true; do
        printf "Select server [1-%d]: " "$total"
        read -r choice </dev/tty

        if [ "$choice" -ge 1 ] 2>/dev/null && [ "$choice" -le "$total" ] 2>/dev/null; then
            break
        fi
        print_error "Invalid choice. Enter a number between 1 and $total"
    done

    # Get selected server data
    selected_line=$(sed -n "${choice}p" "$SERVERS_TMP")
    SELECTED_SERVER_ADDRESS=$(printf '%s' "$selected_line" | cut -d'|' -f1)
    SELECTED_SERVER_PORT=$(printf '%s' "$selected_line" | cut -d'|' -f2)
    SELECTED_SERVER_UUID=$(printf '%s' "$selected_line" | cut -d'|' -f3)
    selected_name=$(printf '%s' "$selected_line" | cut -d'|' -f4)

    print_success "Selected: $selected_name ($SELECTED_SERVER_ADDRESS)"
}

###############################################################################
# Step 4: Configure clients
###############################################################################

step_configure_clients() {
    print_header "Step 4: Configure Clients"

    printf "Add LAN clients for routing.\n"
    printf "Enter 'done' when finished.\n\n"

    XRAY_CLIENTS_LIST=""
    TUN_DIR_RULES_LIST=""

    while true; do
        printf "Client IP (or 'done'): "
        read -r client_ip </dev/tty

        if [ "$client_ip" = "done" ]; then
            break
        fi

        # Validate IP format (basic check for private ranges)
        case "$client_ip" in
            192.168.*|10.*|172.1[6-9].*|172.2[0-9].*|172.3[0-1].*)
                ;;
            *)
                print_error "Invalid LAN IP: $client_ip"
                continue
                ;;
        esac

        printf "\nWhere to route traffic for %s?\n" "$client_ip"
        printf "  1) Xray (VLESS proxy)\n"
        printf "  2) Tunnel Director (OpenVPN)\n"
        printf "Choice [1-2]: "
        read -r route_choice </dev/tty

        case "$route_choice" in
            1)
                XRAY_CLIENTS_LIST="${XRAY_CLIENTS_LIST}${client_ip}
"
                print_success "$client_ip -> Xray"
                ;;
            2)
                printf "OpenVPN client number [1-5]: "
                read -r ovpn_num </dev/tty
                case "$ovpn_num" in
                    [1-5])
                        TUN_DIR_RULES_LIST="${TUN_DIR_RULES_LIST}ovpnc${ovpn_num}:${client_ip}/32::any:ru
"
                        print_success "$client_ip -> ovpnc$ovpn_num"
                        ;;
                    *)
                        print_error "Invalid OpenVPN client number"
                        ;;
                esac
                ;;
            *)
                print_error "Invalid choice"
                ;;
        esac

        printf "\n"
    done

    if [ -z "$XRAY_CLIENTS_LIST" ] && [ -z "$TUN_DIR_RULES_LIST" ]; then
        print_warning "No clients configured"
    fi
}

###############################################################################
# Step 5: Show summary
###############################################################################

step_show_summary() {
    print_header "Step 5: Installation Summary"

    printf "Xray Server:\n"
    printf "  Address: %s\n" "$SELECTED_SERVER_ADDRESS"
    printf "  Port: %s\n" "$SELECTED_SERVER_PORT"
    printf "\n"

    printf "Xray Clients:\n"
    if [ -n "$XRAY_CLIENTS_LIST" ]; then
        printf '%s' "$XRAY_CLIENTS_LIST" | while read -r ip; do
            [ -n "$ip" ] && printf "  - %s\n" "$ip"
        done
    else
        printf "  (none)\n"
    fi
    printf "\n"

    printf "Tunnel Director Rules:\n"
    if [ -n "$TUN_DIR_RULES_LIST" ]; then
        printf '%s' "$TUN_DIR_RULES_LIST" | while read -r rule; do
            [ -n "$rule" ] && printf "  - %s\n" "$rule"
        done
    else
        printf "  (none)\n"
    fi
    printf "\n"

    server_ip_count=$(printf '%s\n' "$XRAY_SERVERS_IPS" | wc -w | tr -d ' ')
    printf "XRAY_SERVERS ipset: %s IP addresses\n" "$server_ip_count"
    printf "\n"

    if ! confirm "Proceed with installation?"; then
        print_info "Installation cancelled"
        exit 0
    fi
}

###############################################################################
# Step 6: Install files
###############################################################################

step_install_files() {
    print_header "Step 6: Installing Files"

    # Create directories
    mkdir -p "$JFFS_DIR/firewall"
    mkdir -p "$JFFS_DIR/xray"
    mkdir -p "$JFFS_DIR/utils"
    mkdir -p "$XRAY_CONFIG_DIR"

    # Download scripts from repo
    print_info "Downloading scripts..."

    for script in \
        "jffs/scripts/firewall/ipset_builder.sh" \
        "jffs/scripts/firewall/tunnel_director.sh" \
        "jffs/scripts/firewall/fw_shared.sh" \
        "jffs/scripts/xray/xray_tproxy.sh" \
        "jffs/scripts/utils/common.sh" \
        "jffs/scripts/utils/firewall.sh" \
        "jffs/scripts/firewall-start" \
        "jffs/scripts/services-start"
    do
        target="/$script"
        curl -fsSL "$REPO_URL/$script" -o "$target" || {
            print_error "Failed to download $script"
            exit 1
        }
        chmod +x "$target"
        print_success "Installed $target"
    done

    # Generate xray config from template
    print_info "Generating Xray config..."
    curl -fsSL "$REPO_URL/config/xray.json.template" | \
        sed "s|{{XRAY_SERVER_ADDRESS}}|$SELECTED_SERVER_ADDRESS|g" | \
        sed "s|{{XRAY_SERVER_PORT}}|$SELECTED_SERVER_PORT|g" | \
        sed "s|{{XRAY_USER_UUID}}|$SELECTED_SERVER_UUID|g" \
        > "$XRAY_CONFIG_DIR/config.json"
    print_success "Generated $XRAY_CONFIG_DIR/config.json"

    # Generate xray/config.sh from template
    print_info "Generating xray/config.sh..."

    # Prepare multiline values for sed (escape newlines)
    xray_clients_escaped=$(printf '%s' "$XRAY_CLIENTS_LIST" | sed 's/$/\\n/' | tr -d '\n' | sed 's/\\n$//')
    xray_servers_escaped=$(printf '%s' "$XRAY_SERVERS_IPS" | tr ' ' '\n' | sed 's/$/\\n/' | tr -d '\n' | sed 's/\\n$//')

    curl -fsSL "$REPO_URL/jffs/scripts/xray/config.sh.template" | \
        sed "s|{{XRAY_CLIENTS}}|$xray_clients_escaped|g" | \
        sed "s|{{XRAY_SERVERS}}|$xray_servers_escaped|g" \
        > "$JFFS_DIR/xray/config.sh"
    print_success "Generated $JFFS_DIR/xray/config.sh"

    # Generate firewall/config.sh from template
    print_info "Generating firewall/config.sh..."

    tun_dir_escaped=$(printf '%s' "$TUN_DIR_RULES_LIST" | sed 's/$/\\n/' | tr -d '\n' | sed 's/\\n$//')

    curl -fsSL "$REPO_URL/jffs/scripts/firewall/config.sh.template" | \
        sed "s|{{TUN_DIR_RULES}}|$tun_dir_escaped|g" \
        > "$JFFS_DIR/firewall/config.sh"
    print_success "Generated $JFFS_DIR/firewall/config.sh"
}

###############################################################################
# Step 7: Apply rules
###############################################################################

step_apply_rules() {
    print_header "Step 7: Applying Rules"

    # Load TPROXY module
    print_info "Loading TPROXY kernel module..."
    modprobe xt_TPROXY 2>/dev/null || true

    # Restart Xray
    print_info "Restarting Xray..."
    if [ -x /opt/etc/init.d/S24xray ]; then
        /opt/etc/init.d/S24xray restart
        print_success "Xray restarted"
    else
        print_warning "Xray init script not found, skipping restart"
    fi

    # Build ipsets
    print_info "Building ipsets (this may take a while)..."
    if [ -x "$JFFS_DIR/firewall/ipset_builder.sh" ]; then
        "$JFFS_DIR/firewall/ipset_builder.sh" || {
            print_warning "ipset_builder.sh failed, some features may not work"
        }
    fi

    # Apply Tunnel Director rules
    if [ -n "$TUN_DIR_RULES_LIST" ]; then
        print_info "Applying Tunnel Director rules..."
        if [ -x "$JFFS_DIR/firewall/tunnel_director.sh" ]; then
            "$JFFS_DIR/firewall/tunnel_director.sh" || {
                print_warning "tunnel_director.sh failed"
            }
        fi
    fi

    # Apply Xray TPROXY rules
    if [ -n "$XRAY_CLIENTS_LIST" ]; then
        print_info "Applying Xray TPROXY rules..."
        if [ -x "$JFFS_DIR/xray/xray_tproxy.sh" ]; then
            "$JFFS_DIR/xray/xray_tproxy.sh" || {
                print_warning "xray_tproxy.sh failed"
            }
        fi
    fi

    print_success "Rules applied"
}

main "$@"
