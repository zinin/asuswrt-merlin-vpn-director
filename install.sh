#!/usr/bin/env bash
set -euo pipefail

# Debug mode: set DEBUG=1 to enable tracing
if [[ ${DEBUG:-0} == 1 ]]; then
    set -x
    PS4='+${BASH_SOURCE[0]##*/}:${LINENO}:${FUNCNAME[0]:-main}: '
fi

###############################################################################
# VPN Director Installer for Asuswrt-Merlin
# Downloads and installs scripts. Run configure.sh after for setup.
###############################################################################

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Installation paths
VPD_DIR="/opt/vpn-director"
JFFS_HOOKS_DIR="/jffs/scripts"
XRAY_CONFIG_DIR="/opt/etc/xray"
GITHUB_REPO="zinin/asuswrt-merlin-vpn-director"

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

print_info() {
    printf "${BLUE}[INFO]${NC} %s\n" "$1"
}

###############################################################################
# Resolve latest release tag
###############################################################################

resolve_release_tag() {
    print_info "Fetching latest release tag..."

    local release_url="https://github.com/${GITHUB_REPO}/releases/latest"
    local effective_url

    # Resolve tag via HTTP redirect (no API quota, no JSON parsing)
    effective_url=$(curl -fsSL --max-time 30 --connect-timeout 10 -o /dev/null -w '%{url_effective}' "$release_url") || {
        print_error "Failed to fetch latest release (HTTP error or timeout)"
        print_info "Check your internet connection and try again"
        exit 1
    }

    RELEASE_TAG="${effective_url##*/}"

    if [[ -z ${RELEASE_TAG:-} ]]; then
        print_error "Could not determine latest release tag"
        exit 1
    fi

    # Validate tag format (semver with optional v prefix)
    if [[ ! "$RELEASE_TAG" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        print_error "Unexpected release tag format: $RELEASE_TAG"
        exit 1
    fi

    REPO_URL="https://raw.githubusercontent.com/${GITHUB_REPO}/refs/tags/${RELEASE_TAG}"
    RELEASE_ASSET_URL="https://github.com/${GITHUB_REPO}/releases/download/${RELEASE_TAG}"

    print_success "Latest release: $RELEASE_TAG"
}

###############################################################################
# Environment check
###############################################################################

check_environment() {
    if [[ ! -d /jffs ]]; then
        print_error "This script must be run on Asuswrt-Merlin router"
        exit 1
    fi

    # Check for Entware
    if [[ ! -d /opt/bin ]]; then
        print_error "Entware not found. Install Entware first."
        exit 1
    fi

    # Check required commands
    missing=""
    if ! which curl >/dev/null 2>&1; then
        missing="$missing curl"
    fi

    if [[ -n $missing ]]; then
        print_error "Missing required commands:$missing"
        print_info "Install with: opkg install curl"
        exit 1
    fi
}

###############################################################################
# Create directories
###############################################################################

create_directories() {
    print_info "Creating directories..."

    mkdir -p "$VPD_DIR/lib"
    mkdir -p "$VPD_DIR/data"
    mkdir -p "$XRAY_CONFIG_DIR"
    mkdir -p "/opt/etc/init.d"
    mkdir -p "$JFFS_HOOKS_DIR"

    print_success "Directories created"
}

###############################################################################
# Download scripts
###############################################################################

download_scripts() {
    print_info "Downloading scripts..."

    # NOTE: Keep file list in sync with server/internal/updater/downloader.go
    for script in \
        "router/opt/vpn-director/vpn-director.sh" \
        "router/opt/vpn-director/configure.sh" \
        "router/opt/vpn-director/import_server_list.sh" \
        "router/opt/vpn-director/vpn-director.json.template" \
        "router/opt/vpn-director/lib/common.sh" \
        "router/opt/vpn-director/lib/firewall.sh" \
        "router/opt/vpn-director/lib/config.sh" \
        "router/opt/vpn-director/lib/ipset.sh" \
        "router/opt/vpn-director/lib/tunnel.sh" \
        "router/opt/vpn-director/lib/tproxy.sh" \
        "router/opt/vpn-director/lib/send-email.sh" \
        "router/opt/vpn-director/setup_telegram_bot.sh" \
        "router/jffs/scripts/firewall-start" \
        "router/jffs/scripts/wan-event" \
        "router/opt/etc/init.d/S99vpn-director" \
        "router/opt/etc/init.d/S98telegram-bot" \
        "router/opt/etc/init.d/S98vpn-director-webui"
    do
        target="/${script#router/}"
        curl -fsSL "$REPO_URL/$script" -o "$target" || {
            print_error "Failed to download $script"
            exit 1
        }
        chmod +x "$target"
        print_success "Installed $target"
    done

    # Download xray config template
    curl -fsSL "$REPO_URL/router/opt/etc/xray/config.json.template" -o "$XRAY_CONFIG_DIR/config.json.template" || {
        print_error "Failed to download config.json.template"
        exit 1
    }
    print_success "Installed $XRAY_CONFIG_DIR/config.json.template"
}

###############################################################################
# Download telegram bot binary (optional)
###############################################################################

download_telegram_bot() {
    print_info "Downloading telegram bot binary..."

    local arch
    arch=$(uname -m)
    local bot_binary=""
    local release_url="$RELEASE_ASSET_URL"
    local bot_path="$VPD_DIR/telegram-bot"
    local tmp_path="${bot_path}.tmp"
    local was_running=false

    case "$arch" in
        aarch64) bot_binary="telegram-bot-arm64" ;;
        armv7l)  bot_binary="telegram-bot-arm" ;;
        *)
            print_info "Architecture $arch not supported for telegram bot (optional component)"
            return 0
            ;;
    esac

    # Download to temp file first
    if ! curl -fsSL "$release_url/$bot_binary" -o "$tmp_path"; then
        print_info "Warning: Failed to download telegram bot (optional component)"
        rm -f "$tmp_path" 2>/dev/null || true
        return 0
    fi

    # Stop running bot before overwriting binary
    if pidof telegram-bot >/dev/null 2>&1; then
        was_running=true
        print_info "Stopping running telegram bot..."
        if [[ -x /opt/etc/init.d/S98telegram-bot ]]; then
            /opt/etc/init.d/S98telegram-bot stop >/dev/null 2>&1 || true
        else
            killall telegram-bot 2>/dev/null || true
        fi
        sleep 1
    fi

    # Atomic move from temp to final location
    mv "$tmp_path" "$bot_path"
    chmod +x "$bot_path"
    print_success "Installed telegram bot"

    # Restart bot if it was running
    if [[ "$was_running" == true ]] && [[ -x /opt/etc/init.d/S98telegram-bot ]]; then
        print_info "Starting telegram bot..."
        /opt/etc/init.d/S98telegram-bot start >/dev/null 2>&1 || true
    fi
}

###############################################################################
# Download webui binary (optional)
###############################################################################

download_webui() {
    print_info "Downloading Web UI binary..."

    local arch
    arch=$(uname -m)
    local webui_binary=""
    local webui_path="$VPD_DIR/webui"
    local tmp_path="${webui_path}.tmp"
    local was_running=false

    case "$arch" in
        aarch64) webui_binary="webui-arm64" ;;
        armv7l)  webui_binary="webui-arm" ;;
        *)
            print_info "Architecture $arch not supported for webui (optional component)"
            return 0
            ;;
    esac

    if ! curl -fsSL "$RELEASE_ASSET_URL/$webui_binary" -o "$tmp_path"; then
        print_info "Warning: Failed to download webui (optional component)"
        rm -f "$tmp_path" 2>/dev/null || true
        return 0
    fi

    # Stop running webui before overwriting binary
    if pidof webui >/dev/null 2>&1; then
        was_running=true
        print_info "Stopping running webui..."
        if [[ -x /opt/etc/init.d/S98vpn-director-webui ]]; then
            /opt/etc/init.d/S98vpn-director-webui stop >/dev/null 2>&1 || true
        else
            killall webui 2>/dev/null || true
        fi
        sleep 1
    fi

    mv "$tmp_path" "$webui_path"
    chmod +x "$webui_path"
    print_success "Installed webui"

    if [[ "$was_running" == true ]] && [[ -x /opt/etc/init.d/S98vpn-director-webui ]]; then
        print_info "Starting webui..."
        /opt/etc/init.d/S98vpn-director-webui start >/dev/null 2>&1 || true
    fi
}

###############################################################################
# Generate self-signed TLS certificate
###############################################################################

generate_tls_cert() {
    local cert_dir="$VPD_DIR/certs"

    if [[ -f "$cert_dir/server.crt" ]] && [[ -f "$cert_dir/server.key" ]]; then
        print_success "TLS certificate already exists"
        return 0
    fi

    print_info "Generating self-signed TLS certificate..."

    mkdir -p "$cert_dir"

    local lan_ip
    lan_ip=$(nvram get lan_ipaddr 2>/dev/null || echo "192.168.1.1")
    local hostname
    hostname=$(nvram get lan_hostname 2>/dev/null || echo "router")

    openssl req -x509 -newkey rsa:2048 \
        -keyout "$cert_dir/server.key" \
        -out "$cert_dir/server.crt" \
        -days 3650 -nodes \
        -subj "/CN=vpn-director" \
        -addext "subjectAltName=IP:${lan_ip},DNS:${hostname},IP:127.0.0.1" 2>/dev/null || {
        # Fallback for older openssl without -addext
        openssl req -x509 -newkey rsa:2048 \
            -keyout "$cert_dir/server.key" \
            -out "$cert_dir/server.crt" \
            -days 3650 -nodes \
            -subj "/CN=vpn-director" 2>/dev/null || {
            print_error "Failed to generate TLS certificate"
            return 1
        }
    }

    chmod 600 "$cert_dir/server.key"
    print_success "TLS certificate generated"
}

###############################################################################
# Setup webui config section
###############################################################################

setup_webui_config() {
    local config_path="$VPD_DIR/vpn-director.json"

    if [[ ! -f "$config_path" ]]; then
        return 0
    fi

    # Check if webui section already exists
    if command -v jq >/dev/null 2>&1; then
        if jq -e '.webui' "$config_path" >/dev/null 2>&1; then
            print_success "WebUI config section already exists"
            return 0
        fi

        local tmp="${config_path}.tmp.$$"
        jq '. + {"webui": {"port": 8444, "cert_file": "/opt/vpn-director/certs/server.crt", "key_file": "/opt/vpn-director/certs/server.key", "jwt_secret": ""}}' "$config_path" > "$tmp" && mv "$tmp" "$config_path"
        print_success "Added webui section to config"
    else
        print_info "Install jq for automatic webui config setup: opkg install jq"
        print_info "Or the webui server will auto-configure on first start"
    fi
}

###############################################################################
# Print next steps
###############################################################################

print_next_steps() {
    print_header "Installation Complete ($RELEASE_TAG)"

    printf "Next steps:\n\n"
    printf "  1. Import VLESS servers:\n"
    printf "     ${GREEN}/opt/vpn-director/import_server_list.sh${NC}\n\n"
    printf "  2. Run configuration wizard:\n"
    printf "     ${GREEN}/opt/vpn-director/configure.sh${NC}\n\n"
    printf "  3. (Optional) Setup Telegram bot:\n"
    printf "     ${GREEN}/opt/vpn-director/setup_telegram_bot.sh${NC}\n\n"
    printf "  4. (Optional) Start Web UI:\n"
    printf "     ${GREEN}/opt/etc/init.d/S98vpn-director-webui start${NC}\n"
    printf "     Then open https://<router-ip>:8444\n\n"
    printf "Or edit configs manually:\n"
    printf "  /opt/vpn-director/vpn-director.json\n"
    printf "  /opt/etc/xray/config.json\n"
}

###############################################################################
# Main
###############################################################################

main() {
    print_header "VPN Director Installer"
    printf "This will install VPN Director scripts to your router.\n\n"

    check_environment
    resolve_release_tag
    create_directories
    download_scripts
    download_telegram_bot
    download_webui
    generate_tls_cert
    setup_webui_config
    print_next_steps
}

main "$@"
