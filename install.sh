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
JFFS_DIR="/jffs/scripts"
XRAY_CONFIG_DIR="/opt/etc/xray"
REPO_URL="https://raw.githubusercontent.com/zinin/asuswrt-merlin-vpn-director/master"

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
# Environment check
###############################################################################

check_environment() {
    if [[ ! -d /jffs ]]; then
        print_error "This script must be run on Asuswrt-Merlin router"
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

    mkdir -p "$JFFS_DIR/vpn-director/utils"
    mkdir -p "$XRAY_CONFIG_DIR"
    mkdir -p "/jffs/configs"
    mkdir -p "/opt/etc/init.d"

    print_success "Directories created"
}

###############################################################################
# Create Entware init.d script
###############################################################################

create_initd_script() {
    print_info "Creating Entware init script..."

    cat > /opt/etc/init.d/S99vpn-director << 'INITEOF'
#!/bin/sh

PATH=/opt/sbin:/opt/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
SCRIPT_DIR="/jffs/scripts/vpn-director"

start() {
    # Build ipsets, then start tunnel_director + xray_tproxy
    "$SCRIPT_DIR/ipset_builder.sh" -t -x

    # Cron job: update ipsets daily at 03:00
    cru a update_ipsets "0 3 * * * $SCRIPT_DIR/ipset_builder.sh -u -t -x"

    # Startup notification
    "$SCRIPT_DIR/utils/send-email.sh" "Startup Notification" \
        "I've just started up and got connected to the internet."
}

stop() {
    "$SCRIPT_DIR/xray_tproxy.sh" stop
    cru d update_ipsets
}

case "$1" in
    start)   start ;;
    stop)    stop ;;
    restart) stop; start ;;
    *)       echo "Usage: $0 {start|stop|restart}" ;;
esac
INITEOF

    chmod +x /opt/etc/init.d/S99vpn-director
    print_success "Created /opt/etc/init.d/S99vpn-director"

    # Create Telegram bot init.d script
    print_info "Creating Entware init.d script for Telegram bot..."
    cat > /opt/etc/init.d/S98telegram-bot << 'INITEOF'
#!/bin/sh

ENABLED=yes
PROCS=telegram-bot
ARGS=""
PREARGS=""
DESC="Telegram Bot for VPN Director"
PATH=/jffs/scripts/vpn-director:/opt/sbin:/opt/bin:/usr/sbin:/usr/bin:/sbin:/bin

. /opt/etc/init.d/rc.func
INITEOF

    chmod +x /opt/etc/init.d/S98telegram-bot
    print_success "Created /opt/etc/init.d/S98telegram-bot"
}

###############################################################################
# Download scripts
###############################################################################

download_scripts() {
    print_info "Downloading scripts..."

    for script in \
        "jffs/scripts/vpn-director/ipset_builder.sh" \
        "jffs/scripts/vpn-director/tunnel_director.sh" \
        "jffs/scripts/vpn-director/xray_tproxy.sh" \
        "jffs/scripts/vpn-director/configure.sh" \
        "jffs/scripts/vpn-director/import_server_list.sh" \
        "jffs/scripts/vpn-director/vpn-director.json.template" \
        "jffs/scripts/vpn-director/utils/common.sh" \
        "jffs/scripts/vpn-director/utils/firewall.sh" \
        "jffs/scripts/vpn-director/utils/shared.sh" \
        "jffs/scripts/vpn-director/utils/config.sh" \
        "jffs/scripts/vpn-director/utils/send-email.sh" \
        "jffs/scripts/vpn-director/setup_telegram_bot.sh" \
        "jffs/scripts/firewall-start" \
        "jffs/scripts/wan-event" \
        "jffs/configs/profile.add"
    do
        target="/$script"
        curl -fsSL "$REPO_URL/$script" -o "$target" || {
            print_error "Failed to download $script"
            exit 1
        }
        chmod +x "$target"
        print_success "Installed $target"
    done

    # Download xray config template
    curl -fsSL "$REPO_URL/config/xray.json.template" -o "$XRAY_CONFIG_DIR/config.json.template" || {
        print_error "Failed to download xray.json.template"
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
    local release_url="https://github.com/zinin/asuswrt-merlin-vpn-director/releases/latest/download"
    local bot_path="$JFFS_DIR/vpn-director/telegram-bot"
    local was_running=false

    case "$arch" in
        aarch64) bot_binary="telegram-bot-arm64" ;;
        armv7l)  bot_binary="telegram-bot-arm" ;;
        *)
            print_info "Architecture $arch not supported for telegram bot (optional component)"
            return 0
            ;;
    esac

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

    curl -fsSL "$release_url/$bot_binary" -o "$bot_path" || {
        print_info "Warning: Failed to download telegram bot (optional component)"
        return 0
    }
    chmod +x "$bot_path"
    print_success "Installed telegram bot"

    # Restart bot if it was running
    if [[ "$was_running" == true ]] && [[ -x /opt/etc/init.d/S98telegram-bot ]]; then
        print_info "Starting telegram bot..."
        /opt/etc/init.d/S98telegram-bot start >/dev/null 2>&1 || true
    fi
}

###############################################################################
# Print next steps
###############################################################################

print_next_steps() {
    print_header "Installation Complete"

    printf "Next steps:\n\n"
    printf "  1. Import VLESS servers:\n"
    printf "     ${GREEN}/jffs/scripts/vpn-director/import_server_list.sh${NC}\n\n"
    printf "  2. Run configuration wizard:\n"
    printf "     ${GREEN}/jffs/scripts/vpn-director/configure.sh${NC}\n\n"
    printf "  3. (Optional) Setup Telegram bot:\n"
    printf "     ${GREEN}/jffs/scripts/vpn-director/setup_telegram_bot.sh${NC}\n\n"
    printf "Or edit configs manually:\n"
    printf "  /jffs/scripts/vpn-director/vpn-director.json\n"
    printf "  /opt/etc/xray/config.json\n"
}

###############################################################################
# Main
###############################################################################

main() {
    print_header "VPN Director Installer"
    printf "This will install VPN Director scripts to your router.\n\n"

    check_environment
    create_directories
    download_scripts
    download_telegram_bot
    create_initd_script
    print_next_steps
}

main "$@"
