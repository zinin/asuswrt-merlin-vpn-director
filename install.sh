#!/bin/sh
set -e

###############################################################################
# VPN Director Installer for Asuswrt-Merlin
# Downloads and installs scripts. Run configure.sh after for setup.
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
    if [ ! -d /jffs ]; then
        print_error "This script must be run on Asuswrt-Merlin router"
        exit 1
    fi

    # Check required commands
    missing=""
    for cmd in curl; do
        if ! which "$cmd" >/dev/null 2>&1; then
            missing="$missing $cmd"
        fi
    done

    if [ -n "$missing" ]; then
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

    mkdir -p "$JFFS_DIR/firewall"
    mkdir -p "$JFFS_DIR/xray"
    mkdir -p "$JFFS_DIR/utils"
    mkdir -p "$XRAY_CONFIG_DIR"
    mkdir -p "/jffs/configs"

    print_success "Directories created"
}

###############################################################################
# Download scripts
###############################################################################

download_scripts() {
    print_info "Downloading scripts..."

    for script in \
        "jffs/scripts/firewall/ipset_builder.sh" \
        "jffs/scripts/firewall/tunnel_director.sh" \
        "jffs/scripts/firewall/fw_shared.sh" \
        "jffs/scripts/firewall/config.sh.template" \
        "jffs/scripts/xray/xray_tproxy.sh" \
        "jffs/scripts/xray/config.sh.template" \
        "jffs/scripts/utils/common.sh" \
        "jffs/scripts/utils/firewall.sh" \
        "jffs/scripts/utils/configure.sh" \
        "jffs/scripts/firewall-start" \
        "jffs/scripts/services-start" \
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
# Print next steps
###############################################################################

print_next_steps() {
    print_header "Installation Complete"

    printf "Next step: Run the configuration wizard:\n\n"
    printf "  ${GREEN}/jffs/scripts/utils/configure.sh${NC}\n\n"
    printf "Or edit configs manually:\n"
    printf "  /jffs/scripts/xray/config.sh\n"
    printf "  /jffs/scripts/firewall/config.sh\n"
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
    print_next_steps
}

main "$@"
