#!/bin/sh
set -e

###############################################################################
# VPN Director Configuration Wizard
# Run this after install.sh to configure Xray TPROXY and Tunnel Director
###############################################################################

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Paths
JFFS_DIR="/jffs/scripts/vpn-director"
XRAY_CONFIG_DIR="/opt/etc/xray"
SERVERS_TMP="/tmp/vpn_director_servers.tmp"

# Temporary storage for parsed data
XRAY_CLIENTS_LIST=""
TUN_DIR_RULES_LIST=""
SELECTED_SERVER_ADDRESS=""
SELECTED_SERVER_PORT=""
SELECTED_SERVER_UUID=""
XRAY_EXCLUDE_SETS_LIST="ru"

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

confirm() {
    printf "%s [y/n]: " "$1"
    read -r REPLY
    case "$REPLY" in
        [Yy]*) return 0 ;;
        *) return 1 ;;
    esac
}

###############################################################################
# Step 1: Select Xray server
###############################################################################

step_select_xray_server() {
    print_header "Step 1: Select Xray Server"

    printf "Available servers:\n\n"

    i=1
    while IFS='|' read -r server port uuid name ip; do
        printf "  %2d) %s\n      %s -> %s\n\n" "$i" "$name" "$server" "$ip"
        i=$((i + 1))
    done < "$SERVERS_TMP"

    total=$((i - 1))

    while true; do
        printf "Select server [1-%d]: " "$total"
        read -r choice

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
# Step 4: Configure Xray exclusions
###############################################################################

step_configure_xray_exclusions() {
    print_header "Step 4: Xray Exclusions"

    printf "Traffic to these countries will NOT go through Xray proxy.\n"
    printf "Common choice: your local country to avoid unnecessary proxying.\n\n"

    # List of common countries
    printf "Available countries:\n\n"
    printf "  1) ru - Russia          6) fr - France\n"
    printf "  2) ua - Ukraine         7) nl - Netherlands\n"
    printf "  3) by - Belarus         8) pl - Poland\n"
    printf "  4) kz - Kazakhstan      9) tr - Turkey\n"
    printf "  5) de - Germany        10) il - Israel\n"
    printf "\n"

    printf "Enter country numbers separated by space (e.g., 1 5 7)\n"
    printf "or 0 for none [default: 1]: "
    read -r choice

    # Default to ru (1) if empty
    if [ -z "$choice" ]; then
        choice="1"
    fi

    # Convert numbers to country codes
    XRAY_EXCLUDE_SETS_LIST=""
    for num in $choice; do
        case "$num" in
            0) XRAY_EXCLUDE_SETS_LIST=""; break ;;
            1) code="ru" ;;
            2) code="ua" ;;
            3) code="by" ;;
            4) code="kz" ;;
            5) code="de" ;;
            6) code="fr" ;;
            7) code="nl" ;;
            8) code="pl" ;;
            9) code="tr" ;;
            10) code="il" ;;
            *) print_warning "Unknown option: $num"; continue ;;
        esac
        if [ -z "$XRAY_EXCLUDE_SETS_LIST" ]; then
            XRAY_EXCLUDE_SETS_LIST="$code"
        else
            XRAY_EXCLUDE_SETS_LIST="$XRAY_EXCLUDE_SETS_LIST,$code"
        fi
    done

    if [ -n "$XRAY_EXCLUDE_SETS_LIST" ]; then
        print_success "Excluding: $XRAY_EXCLUDE_SETS_LIST"
    else
        print_info "No countries excluded"
    fi
}

###############################################################################
# Step 5: Configure clients
###############################################################################

step_configure_clients() {
    print_header "Step 5: Configure Clients"

    printf "Add LAN clients for routing.\n"
    printf "Enter 'done' when finished.\n\n"

    XRAY_CLIENTS_LIST=""
    TUN_DIR_RULES_LIST=""

    while true; do
        printf "Client IP (or 'done'): "
        read -r client_ip

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
        read -r route_choice

        case "$route_choice" in
            1)
                XRAY_CLIENTS_LIST="${XRAY_CLIENTS_LIST}${client_ip}
"
                print_success "$client_ip -> Xray"
                ;;
            2)
                printf "OpenVPN client number [1-5]: "
                read -r ovpn_num
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
# Step 6: Show summary
###############################################################################

step_show_summary() {
    print_header "Step 6: Configuration Summary"

    printf "Xray Server:\n"
    printf "  Address: %s\n" "$SELECTED_SERVER_ADDRESS"
    printf "  Port: %s\n" "$SELECTED_SERVER_PORT"
    printf "\n"

    printf "Xray Exclusions: %s\n\n" "$XRAY_EXCLUDE_SETS_LIST"

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

    if ! confirm "Proceed with configuration?"; then
        print_info "Configuration cancelled"
        exit 0
    fi
}

###############################################################################
# Step 7: Generate config files
###############################################################################

step_generate_configs() {
    print_header "Step 7: Generating Configs"

    if [ ! -f "$JFFS_DIR/vpn-director.json.template" ]; then
        print_error "Template not found: $JFFS_DIR/vpn-director.json.template"
        print_info "Run install.sh first to download required files"
        exit 1
    fi

    # Generate xray/config.json from template
    print_info "Generating Xray config..."

    sed "s|{{XRAY_SERVER_ADDRESS}}|$SELECTED_SERVER_ADDRESS|g" \
        /opt/etc/xray/config.json.template 2>/dev/null | \
        sed "s|{{XRAY_SERVER_PORT}}|$SELECTED_SERVER_PORT|g" | \
        sed "s|{{XRAY_USER_UUID}}|$SELECTED_SERVER_UUID|g" \
        > "$XRAY_CONFIG_DIR/config.json"
    print_success "Generated $XRAY_CONFIG_DIR/config.json"

    # Generate vpn-director.json
    print_info "Generating vpn-director.json..."

    # Build JSON arrays
    xray_clients_json="[]"
    if [ -n "$XRAY_CLIENTS_LIST" ]; then
        xray_clients_json=$(printf '%s' "$XRAY_CLIENTS_LIST" | grep -v '^$' | jq -R . | jq -s .)
    fi

    xray_servers_json="[]"
    if [ -n "$XRAY_SERVERS_IPS" ]; then
        xray_servers_json=$(printf '%s\n' $XRAY_SERVERS_IPS | jq -R . | jq -s .)
    fi

    xray_exclude_json='["ru"]'
    if [ -n "$XRAY_EXCLUDE_SETS_LIST" ]; then
        xray_exclude_json=$(printf '%s\n' ${XRAY_EXCLUDE_SETS_LIST//,/ } | jq -R . | jq -s .)
    fi

    tun_dir_rules_json="[]"
    if [ -n "$TUN_DIR_RULES_LIST" ]; then
        tun_dir_rules_json=$(printf '%s' "$TUN_DIR_RULES_LIST" | grep -v '^$' | jq -R . | jq -s .)
    fi

    # Read template and update with jq
    jq \
        --argjson clients "$xray_clients_json" \
        --argjson servers "$xray_servers_json" \
        --argjson exclude "$xray_exclude_json" \
        --argjson rules "$tun_dir_rules_json" \
        '.xray.clients = $clients |
         .xray.servers = $servers |
         .xray.exclude_sets = $exclude |
         .tunnel_director.rules = $rules' \
        "$JFFS_DIR/vpn-director.json.template" \
        > "$JFFS_DIR/vpn-director.json"

    print_success "Generated $JFFS_DIR/vpn-director.json"
}

###############################################################################
# Step 8: Apply rules
###############################################################################

step_apply_rules() {
    print_header "Step 8: Applying Rules"

    # Load TPROXY module
    print_info "Loading TPROXY kernel module..."
    modprobe xt_TPROXY 2>/dev/null || true

    # Restart Xray
    print_info "Restarting Xray..."
    XRAY_INIT=$(ls /opt/etc/init.d/S*xray 2>/dev/null | head -1)
    if [ -n "$XRAY_INIT" ] && [ -x "$XRAY_INIT" ]; then
        "$XRAY_INIT" restart
        print_success "Xray restarted"
    else
        print_warning "Xray init script not found, skipping restart"
    fi

    # Build ipsets (including extra countries for Xray exclusions)
    print_info "Building ipsets (this may take a while)..."
    if [ -x "$JFFS_DIR/ipset_builder.sh" ]; then
        "$JFFS_DIR/ipset_builder.sh" -c "$XRAY_EXCLUDE_SETS_LIST" || {
            print_warning "ipset_builder.sh failed, some features may not work"
        }
    fi

    # Apply Tunnel Director rules
    if [ -n "$TUN_DIR_RULES_LIST" ]; then
        print_info "Applying Tunnel Director rules..."
        if [ -x "$JFFS_DIR/tunnel_director.sh" ]; then
            "$JFFS_DIR/tunnel_director.sh" || {
                print_warning "tunnel_director.sh failed"
            }
        fi
    fi

    # Apply Xray TPROXY rules
    if [ -n "$XRAY_CLIENTS_LIST" ]; then
        print_info "Applying Xray TPROXY rules..."
        if [ -x "$JFFS_DIR/xray_tproxy.sh" ]; then
            "$JFFS_DIR/xray_tproxy.sh" || {
                print_warning "xray_tproxy.sh failed"
            }
        fi
    fi

    print_success "Rules applied"
}

###############################################################################
# Main
###############################################################################

main() {
    print_header "VPN Director Configuration"
    printf "This wizard will configure Xray TPROXY and Tunnel Director.\n\n"

    step_get_vless_file              # Step 1
    step_parse_vless_servers         # Step 2
    step_select_xray_server          # Step 3
    step_configure_xray_exclusions   # Step 4
    step_configure_clients           # Step 5
    step_show_summary                # Step 6
    step_generate_configs            # Step 7
    step_apply_rules                 # Step 8

    print_header "Configuration Complete"
    printf "Check status with: /jffs/scripts/vpn-director/xray_tproxy.sh status\n"
}

main "$@"
