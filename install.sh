#!/bin/sh
# shellcheck shell=bash
# KeeneticOS has no /usr/bin/env and mounts / read-only, so the usual
# "#!/usr/bin/env bash" cannot start this script there. Begin as a POSIX shell
# and hand over to bash by absolute path - Entware's first, a workstation's
# after it. PATH is no help here: Asuswrt-Merlin's /bin/sh has no "command"
# builtin (see .claude/rules/shell-conventions.md) and its /bin/bash is a
# symlink to busybox rather than bash.
if [ -z "${BASH_VERSION:-}" ]; then
    for _vpd_bash in /opt/bin/bash /usr/bin/bash /bin/bash; do
        [ -x "$_vpd_bash" ] && exec "$_vpd_bash" "$0" "$@"
    done
    echo "$0: bash not found; install it (Entware package \"bash\")" >&2
    exit 1
fi
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
GITHUB_REPO="zinin/vpn-director"
INIT_DIR="/opt/etc/init.d"
WEBUI_URL=""   # set by start_webui once the daemon answers; read by print_next_steps

# Platform tag that selects files from router/files.manifest. Detection of
# Keenetic lands together with its platform module; until then this installer
# serves Asuswrt-Merlin only.
PLATFORM="merlin"

# Root the manifest files are installed under: "" on a router, a temporary
# directory in tests.
INSTALL_ROOT="${INSTALL_ROOT:-}"

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
    mkdir -p "$INIT_DIR"
    mkdir -p "$JFFS_HOOKS_DIR"

    print_success "Directories created"
}

###############################################################################
# File manifest
###############################################################################

# manifest_files <manifest> <platform>
#   Prints the repository paths tagged "common" or <platform>, one per line,
#   in manifest order. Blank lines and "#" comments are skipped. An
#   unrecognised tag is an error, not something to skip: a typo like "comon"
#   would drop that file from every install, and no "the path exists" check
#   can see it. The Go updater tolerates an unknown tag because an old binary
#   can meet a newer release; this parser ships with the manifest it reads.
manifest_files() {
    local manifest="$1" platform="$2" line="" lineno=0 tag path
    # `|| [[ -n $line ]]` keeps a last line with no trailing newline. The Go
    # parser's bufio.Scanner reads it, and the two must not disagree about
    # what a release ships.
    while read -r line || [[ -n $line ]]; do
        lineno=$((lineno + 1))
        read -r tag path _ <<< "$line"
        case "$tag" in ''|'#'*) continue ;; esac
        case "$tag" in
            common|merlin|keenetic) ;;
            *)
                # stderr: the caller reads this function's stdout as the list.
                print_error "files.manifest line $lineno: unknown tag \"$tag\" in \"$line\"" >&2
                return 1
                ;;
        esac
        # The rule is the Go parser's (server/internal/updater/manifest.go).
        if [[ ! $path =~ ^router/[A-Za-z0-9._/-]+$ || $path == *..* ]]; then
            print_error "files.manifest line $lineno: invalid path \"$path\" in \"$line\"" >&2
            return 1
        fi
        if [[ $tag == common || $tag == "$platform" ]]; then
            printf '%s\n' "$path"
        fi
    done < "$manifest"
}

# manifest_is_executable <repo path>
#   Returns 0 when the installed file gets the executable bit: everything
#   except templates, JSON and the manifest itself.
manifest_is_executable() {
    case "$1" in
        *.template|*.json|*.manifest) return 1 ;;
    esac
    return 0
}

###############################################################################
# Download scripts
###############################################################################

download_scripts() {
    print_info "Downloading file manifest..."

    local manifest
    manifest=$(mktemp)
    if ! curl -fsSL "$REPO_URL/router/files.manifest" -o "$manifest"; then
        rm -f "$manifest"
        print_error "Failed to download router/files.manifest"
        return 1
    fi

    # Parse the whole manifest before installing anything. In a process
    # substitution manifest_files' exit status is invisible, so a bad tag
    # halfway down would leave a half-installed router and still report
    # success.
    local files
    if ! files=$(manifest_files "$manifest" "$PLATFORM"); then
        rm -f "$manifest"
        return 1
    fi
    rm -f "$manifest"

    # No match means a corrupt manifest or a platform this release does not
    # ship. Reporting success here would send main() on to setup_webui_config
    # and start_webui on top of absent scripts.
    if [[ -z $files ]]; then
        print_error "files.manifest lists no file for platform $PLATFORM"
        return 1
    fi

    print_info "Downloading scripts..."
    local file target
    while IFS= read -r file; do
        target="${INSTALL_ROOT}/${file#router/}"
        mkdir -p "$(dirname "$target")"
        if ! curl -fsSL "$REPO_URL/$file" -o "$target"; then
            print_error "Failed to download $file"
            return 1
        fi
        if manifest_is_executable "$file"; then
            chmod +x "$target"
        fi
        print_success "Installed $target"
    done <<< "$files"
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

    # `nvram get` exits 0 and prints nothing for an unset variable, so `|| echo`
    # never fires: an empty value would leave subjectAltName with an empty IP,
    # openssl would reject -addext and fall through to a certificate with no SAN.
    local lan_ip
    lan_ip=$(nvram get lan_ipaddr 2>/dev/null || true)
    [[ -n "$lan_ip" ]] || lan_ip="192.168.1.1"
    local hostname
    hostname=$(nvram get lan_hostname 2>/dev/null || true)
    [[ -n "$hostname" ]] || hostname="router"

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
    local template_path="$VPD_DIR/vpn-director.json.template"

    if [[ ! -f "$config_path" ]]; then
        if [[ ! -f "$template_path" ]]; then
            return 0
        fi
        cp "$template_path" "$config_path"
        chmod 600 "$config_path"
        print_success "Created $config_path from template"
    fi

    # Check if webui section already exists
    if command -v jq >/dev/null 2>&1; then
        if jq -e '.webui' "$config_path" >/dev/null 2>&1; then
            print_success "WebUI config section already exists"
            return 0
        fi

        local tmp="${config_path}.tmp.$$"
        jq '. + {"webui": {"port": 8444, "cert_file": "/opt/vpn-director/certs/server.crt", "key_file": "/opt/vpn-director/certs/server.key", "jwt_secret": "", "log_level": "info"}}' "$config_path" > "$tmp" && chmod 600 "$tmp" && mv "$tmp" "$config_path"
        print_success "Added webui section to config"
    else
        print_info "Install jq for automatic webui config setup: opkg install jq"
        print_info "Or the webui server will auto-configure on first start"
    fi
}

###############################################################################
# Start Web UI
###############################################################################

start_webui() {
    local webui_path="$VPD_DIR/webui"
    local init_script="$INIT_DIR/S98vpn-director-webui"
    local lan_ip

    # download_webui skips unsupported architectures and tolerates a failed
    # download, so there is not always something to start.
    if [[ ! -x "$webui_path" ]]; then
        return 0
    fi
    if [[ ! -x "$init_script" ]]; then
        print_info "Web UI init script not found, skipping start"
        return 0
    fi

    # An upgrade may already have restarted it (see download_webui); the init
    # script's start is a no-op when the daemon is up.
    if ! "$init_script" start >/dev/null 2>&1; then
        print_error "Failed to start Web UI - see /tmp/vpn-director-webui.log"
        return 0
    fi

    lan_ip=$(nvram get lan_ipaddr 2>/dev/null || true)
    [[ -n "$lan_ip" ]] || lan_ip="192.168.1.1"
    local port=8444
    if [[ -f "$VPD_DIR/vpn-director.json" ]] && command -v jq >/dev/null 2>&1; then
        local p
        p=$(jq -r '.webui.port // empty' "$VPD_DIR/vpn-director.json" 2>/dev/null || true)
        if [[ "$p" =~ ^[1-9][0-9]*$ ]] && (( p <= 65535 )); then
            port=$p
        fi
    fi
    WEBUI_URL="https://${lan_ip}:${port}"
    print_success "Web UI started: $WEBUI_URL"
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
    if [[ -n "$WEBUI_URL" ]]; then
        printf "  4. Open the Web UI (already running):\n"
        printf "     ${GREEN}%s${NC}\n" "$WEBUI_URL"
        printf "     Log in with the router admin username and password\n\n"
    else
        printf "  4. Web UI is not running. Start it with:\n"
        printf "     ${GREEN}%s/S98vpn-director-webui start${NC}\n" "$INIT_DIR"
        printf "     Then open https://<router-ip>:8444\n\n"
    fi
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
    start_webui
    print_next_steps
}

###############################################################################
# Allow sourcing for testing
###############################################################################

if [[ ${1:-} == "--source-only" ]]; then
    # shellcheck disable=SC2317
    return 0 2>/dev/null || exit 0
fi

main "$@"
