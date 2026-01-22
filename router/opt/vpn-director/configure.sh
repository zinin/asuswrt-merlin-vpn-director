#!/usr/bin/env bash
set -euo pipefail

# Debug mode: set DEBUG=1 to enable tracing
if [[ ${DEBUG:-0} == 1 ]]; then
    set -x
    PS4='+${BASH_SOURCE[0]##*/}:${LINENO}:${FUNCNAME[0]:-main}: '
fi

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
VPD_DIR="/opt/vpn-director"
XRAY_CONFIG_DIR="/opt/etc/xray"

# Temporary storage for parsed data
XRAY_CLIENTS_LIST=""
TUN_DIR_TUNNELS_JSON='{}'
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

# shellcheck disable=SC2034  # INPUT_RESULT used by callers
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
# Get data directory and validate servers
###############################################################################

get_data_dir() {
    local config_file="$VPD_DIR/vpn-director.json"

    if [[ ! -f $config_file ]]; then
        config_file="$VPD_DIR/vpn-director.json.template"
    fi

    jq -r '.data_dir // "/opt/vpn-director/data"' "$config_file"
}

check_servers_file() {
    DATA_DIR=$(get_data_dir)
    SERVERS_FILE="$DATA_DIR/servers.json"

    if [[ ! -f $SERVERS_FILE ]]; then
        print_error "Server list not found: $SERVERS_FILE"
        print_info "Run import_server_list.sh first"
        exit 1
    fi

    SERVER_COUNT=$(jq length "$SERVERS_FILE")
    if [[ $SERVER_COUNT -eq 0 ]]; then
        print_error "Server list is empty"
        print_info "Run import_server_list.sh again"
        exit 1
    fi

    print_success "Found $SERVER_COUNT servers in $SERVERS_FILE"
}

###############################################################################
# Step 1: Select Xray server
###############################################################################

step_select_xray_server() {
    print_header "Step 1: Select Xray Server"

    printf "Available servers:\n\n"

    # Read servers from JSON and display
    i=1
    jq -r '.[] | "\(.name)|\(.address)|\(.ip)"' "$SERVERS_FILE" | \
    while IFS='|' read -r name address ip; do
        printf "  %2d) %s\n      %s -> %s\n\n" "$i" "$name" "$address" "$ip"
        i=$((i + 1))
    done

    total=$(jq length "$SERVERS_FILE")

    while true; do
        printf "Select server [1-%d]: " "$total"
        read -r choice

        if [[ $choice -ge 1 ]] 2>/dev/null && [[ $choice -le $total ]] 2>/dev/null; then
            break
        fi
        print_error "Invalid choice. Enter a number between 1 and $total"
    done

    # Get selected server data (jq uses 0-based index)
    idx=$((choice - 1))
    SELECTED_SERVER_ADDRESS=$(jq -r ".[$idx].address" "$SERVERS_FILE")
    SELECTED_SERVER_PORT=$(jq -r ".[$idx].port" "$SERVERS_FILE")
    SELECTED_SERVER_UUID=$(jq -r ".[$idx].uuid" "$SERVERS_FILE")
    selected_name=$(jq -r ".[$idx].name" "$SERVERS_FILE")

    print_success "Selected: $selected_name ($SELECTED_SERVER_ADDRESS)"
}

###############################################################################
# Step 2: Configure Xray exclusions
###############################################################################

step_configure_xray_exclusions() {
    print_header "Step 2: Xray Exclusions"

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
    if [[ -z $choice ]]; then
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
        if [[ -z $XRAY_EXCLUDE_SETS_LIST ]]; then
            XRAY_EXCLUDE_SETS_LIST="$code"
        else
            XRAY_EXCLUDE_SETS_LIST="$XRAY_EXCLUDE_SETS_LIST,$code"
        fi
    done

    if [[ -n $XRAY_EXCLUDE_SETS_LIST ]]; then
        print_success "Excluding: $XRAY_EXCLUDE_SETS_LIST"
    else
        print_info "No countries excluded"
    fi
}

###############################################################################
# Step 3: Configure clients
###############################################################################

# Helper: select exclude countries
select_exclude_countries() {
    printf "\n"
    printf "Select countries to EXCLUDE from tunnel (direct routing):\n"
    printf "  1) ru - Russia          6) fr - France\n"
    printf "  2) ua - Ukraine         7) nl - Netherlands\n"
    printf "  3) by - Belarus         8) pl - Poland\n"
    printf "  4) kz - Kazakhstan      9) tr - Turkey\n"
    printf "  5) de - Germany        10) il - Israel\n"
    printf "\n"
    printf "Enter numbers separated by space (e.g., 1 5 7) or 0 for none [default: 1]: "
    read -r choice

    if [[ -z $choice ]]; then
        choice="1"
    fi

    local exclude_list=""
    for num in $choice; do
        local code=""
        case "$num" in
            0) exclude_list=""; break ;;
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
        if [[ -z $exclude_list ]]; then
            exclude_list="$code"
        else
            exclude_list="$exclude_list $code"
        fi
    done

    SELECTED_EXCLUDE="$exclude_list"
}

# Helper: add client to tunnel
add_client_to_tunnel() {
    local tunnel="$1"
    local client="$2"
    local exclude="$3"

    # Add /32 suffix if not present
    if [[ $client != */* ]]; then
        client="${client}/32"
    fi

    # Build exclude JSON array
    local exclude_json='[]'
    if [[ -n $exclude ]]; then
        # shellcheck disable=SC2086  # intentional word splitting
        exclude_json=$(printf '%s\n' $exclude | jq -R . | jq -s .)
    fi

    # Check if tunnel already exists
    if jq -e --arg t "$tunnel" '.[$t]' <<< "$TUN_DIR_TUNNELS_JSON" >/dev/null 2>&1; then
        # Tunnel exists - add client to existing array
        TUN_DIR_TUNNELS_JSON=$(jq --arg t "$tunnel" --arg c "$client" \
            '.[$t].clients += [$c]' <<< "$TUN_DIR_TUNNELS_JSON")
    else
        # New tunnel - create with clients and exclude
        TUN_DIR_TUNNELS_JSON=$(jq --arg t "$tunnel" --arg c "$client" --argjson e "$exclude_json" \
            '.[$t] = {clients: [$c], exclude: $e}' <<< "$TUN_DIR_TUNNELS_JSON")
    fi
}

step_configure_clients() {
    print_header "Step 3: Configure Clients"

    printf "Add LAN clients for routing.\n"
    printf "Enter 'done' when finished.\n\n"

    XRAY_CLIENTS_LIST=""
    TUN_DIR_TUNNELS_JSON='{}'

    while true; do
        printf "Client IP or CIDR (or 'done'): "
        read -r client_ip

        if [[ $client_ip == "done" ]]; then
            break
        fi

        # Validate IP format (basic check for private ranges)
        local ip_part="${client_ip%%/*}"
        case "$ip_part" in
            192.168.*|10.*|172.1[6-9].*|172.2[0-9].*|172.3[0-1].*)
                ;;
            *)
                print_error "Invalid LAN IP: $client_ip"
                continue
                ;;
        esac

        printf "\nWhere to route traffic for %s?\n" "$client_ip"
        printf "  1) Xray (VLESS proxy)\n"
        printf "  2) Tunnel Director (VPN tunnel)\n"
        printf "Choice [1-2]: "
        read -r route_choice

        case "$route_choice" in
            1)
                XRAY_CLIENTS_LIST="${XRAY_CLIENTS_LIST}${client_ip}
"
                print_success "$client_ip -> Xray"
                ;;
            2)
                printf "\nTunnel type:\n"
                printf "  1) WireGuard (wgc1-5)\n"
                printf "  2) OpenVPN (ovpnc1-5)\n"
                printf "Choice [1-2]: "
                read -r tunnel_type

                local tunnel_prefix=""
                case "$tunnel_type" in
                    1) tunnel_prefix="wgc" ;;
                    2) tunnel_prefix="ovpnc" ;;
                    *)
                        print_error "Invalid tunnel type"
                        continue
                        ;;
                esac

                printf "Tunnel number [1-5]: "
                read -r tunnel_num
                case "$tunnel_num" in
                    [1-5])
                        local tunnel="${tunnel_prefix}${tunnel_num}"

                        # Ask for exclude countries only for new tunnels
                        if ! jq -e --arg t "$tunnel" '.[$t]' <<< "$TUN_DIR_TUNNELS_JSON" >/dev/null 2>&1; then
                            select_exclude_countries
                            add_client_to_tunnel "$tunnel" "$client_ip" "$SELECTED_EXCLUDE"
                            print_success "$client_ip -> $tunnel (exclude: ${SELECTED_EXCLUDE:-none})"
                        else
                            # Tunnel exists - just add client
                            add_client_to_tunnel "$tunnel" "$client_ip" ""
                            print_success "$client_ip -> $tunnel (using existing exclude)"
                        fi
                        ;;
                    *)
                        print_error "Invalid tunnel number"
                        ;;
                esac
                ;;
            *)
                print_error "Invalid choice"
                ;;
        esac

        printf "\n"
    done

    if [[ -z $XRAY_CLIENTS_LIST ]] && [[ $TUN_DIR_TUNNELS_JSON == '{}' ]]; then
        print_warning "No clients configured"
    fi
}

###############################################################################
# Step 4: Show summary
###############################################################################

step_show_summary() {
    print_header "Step 4: Configuration Summary"

    printf "Xray Server:\n"
    printf "  Address: %s\n" "$SELECTED_SERVER_ADDRESS"
    printf "  Port: %s\n" "$SELECTED_SERVER_PORT"
    printf "\n"

    printf "Xray Exclusions: %s\n\n" "$XRAY_EXCLUDE_SETS_LIST"

    printf "Xray Clients:\n"
    if [[ -n $XRAY_CLIENTS_LIST ]]; then
        printf '%s' "$XRAY_CLIENTS_LIST" | while read -r ip; do
            [[ -n $ip ]] && printf "  - %s\n" "$ip"
        done
    else
        printf "  (none)\n"
    fi
    printf "\n"

    printf "Tunnel Director Tunnels:\n"
    if [[ $TUN_DIR_TUNNELS_JSON != '{}' ]]; then
        jq -r 'to_entries[] | "  \(.key):\n    clients: \(.value.clients | join(", "))\n    exclude: \(.value.exclude | join(", "))"' <<< "$TUN_DIR_TUNNELS_JSON"
    else
        printf "  (none)\n"
    fi
    printf "\n"

    if ! confirm "Proceed with configuration?"; then
        print_info "Configuration cancelled"
        exit 0
    fi
}

###############################################################################
# Step 5: Generate config files
###############################################################################

step_generate_configs() {
    print_header "Step 5: Generating Configs"

    if [[ ! -f $VPD_DIR/vpn-director.json.template ]]; then
        print_error "Template not found: $VPD_DIR/vpn-director.json.template"
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
    if [[ -n $XRAY_CLIENTS_LIST ]]; then
        xray_clients_json=$(printf '%s' "$XRAY_CLIENTS_LIST" | grep -v '^$' | jq -R . | jq -s .)
    fi

    xray_exclude_json='["ru"]'
    if [[ -n $XRAY_EXCLUDE_SETS_LIST ]]; then
        # shellcheck disable=SC2086  # intentional word splitting
        xray_exclude_json=$(printf '%s\n' ${XRAY_EXCLUDE_SETS_LIST//,/ } | jq -R . | jq -s .)
    fi

    # Build xray servers array from servers.json (unique IPs)
    xray_servers_json="[]"
    if [[ -f "$SERVERS_FILE" ]]; then
        xray_servers_json=$(jq '[.[].ip] | unique' "$SERVERS_FILE")
    fi

    # Read template and update with jq
    jq \
        --argjson clients "$xray_clients_json" \
        --argjson exclude "$xray_exclude_json" \
        --argjson tunnels "$TUN_DIR_TUNNELS_JSON" \
        --argjson servers "$xray_servers_json" \
        '.xray.clients = $clients |
         .xray.exclude_sets = $exclude |
         .xray.servers = $servers |
         .tunnel_director.tunnels = $tunnels' \
        "$VPD_DIR/vpn-director.json.template" \
        > "$VPD_DIR/vpn-director.json"

    print_success "Generated $VPD_DIR/vpn-director.json"
}

###############################################################################
# Step 6: Apply rules
###############################################################################

step_apply_rules() {
    print_header "Step 6: Applying Rules"

    print_info "Applying configuration (this may take a while)..."
    if [[ -x $VPD_DIR/vpn-director.sh ]]; then
        "$VPD_DIR/vpn-director.sh" restart || {
            print_warning "vpn-director.sh restart failed"
            return 1
        }
        print_success "Configuration applied"
    else
        print_error "vpn-director.sh not found"
        return 1
    fi
}

###############################################################################
# Main
###############################################################################

main() {
    print_header "VPN Director Configuration"
    printf "This wizard will configure Xray TPROXY and Tunnel Director.\n\n"

    check_servers_file                # Validate servers.json exists
    step_select_xray_server           # Step 1
    step_configure_xray_exclusions    # Step 2
    step_configure_clients            # Step 3
    step_show_summary                 # Step 4
    step_generate_configs             # Step 5
    step_apply_rules                  # Step 6

    print_header "Configuration Complete"
    printf "Check status with: /opt/vpn-director/vpn-director.sh status\n"
}

main "$@"
