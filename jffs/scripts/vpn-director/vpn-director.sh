#!/usr/bin/env bash

###################################################################################################
# vpn-director.sh - Unified CLI for VPN Director
# -------------------------------------------------------------------------------------------------
# Usage:
#   vpn-director status [tunnel|xray|ipset]  - Show status
#   vpn-director apply [tunnel|xray]         - Apply configuration
#   vpn-director stop [tunnel|xray]          - Stop components
#   vpn-director restart [tunnel|xray]       - Restart components
#   vpn-director update                      - Update ipsets and reapply all
#
# Options:
#   -f, --force    Force operation (ignore hash checks)
#   -q, --quiet    Minimal output
#   -v, --verbose  Debug output
#   --dry-run      Show what would be done
#   -h, --help     Show this help
###################################################################################################

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Support --source-only for testing (must be before any parsing)
[[ "${1:-}" == "--source-only" ]] && { shift; _SOURCE_ONLY=1; } || _SOURCE_ONLY=0

###################################################################################################
# Parse options (supports both pre-command and post-command placement)
# vpn-director --force apply     # works
# vpn-director apply --force     # works
# vpn-director apply tunnel -v   # works
###################################################################################################
FORCE=0
QUIET=0
VERBOSE=0
DRY_RUN=0
COMMAND=""
COMPONENT=""

parse_option() {
    case $1 in
        -f|--force)   FORCE=1; return 0 ;;
        -q|--quiet)   QUIET=1; return 0 ;;
        -v|--verbose) VERBOSE=1; export DEBUG=1; return 0 ;;
        --dry-run)    DRY_RUN=1; return 0 ;;
        -h|--help)    COMMAND="help"; return 0 ;;
        -*)           echo "Unknown option: $1" >&2; exit 1 ;;
        *)            return 1 ;;
    esac
}

# Phase 1: Parse pre-command options and extract command/component
while [[ $# -gt 0 ]]; do
    if parse_option "$1"; then
        shift
    elif [[ -z $COMMAND ]]; then
        COMMAND="$1"; shift
    elif [[ -z $COMPONENT ]]; then
        COMPONENT="$1"; shift
    else
        # Extra positional argument
        echo "Unexpected argument: $1" >&2; exit 1
    fi
done

# Phase 2: Default command
COMMAND="${COMMAND:-help}"

# Debug mode
if [[ $VERBOSE -eq 1 ]]; then
    set -x
    PS4='+${BASH_SOURCE[0]##*/}:${LINENO}:${FUNCNAME[0]:-main}: '
fi

###################################################################################################
# Help (shown before loading modules - no config required)
###################################################################################################
show_help() {
    cat <<'EOF'
VPN Director - Unified traffic routing for Asuswrt-Merlin

Usage:
  vpn-director <command> [component] [options]

Commands:
  status [tunnel|xray|ipset]  Show status (all or specific component)
  apply [tunnel|xray]         Apply configuration
  stop [tunnel|xray]          Stop components
  restart [tunnel|xray]       Restart (stop + apply)
  update                      Download fresh ipsets and reapply all

Options:
  -f, --force    Force operation (ignore hash checks)
  -q, --quiet    Minimal output
  -v, --verbose  Debug output
  --dry-run      Show what would be done
  -h, --help     Show this help

Examples:
  vpn-director status              # Show all status
  vpn-director apply               # Apply all (ipsets + tunnel + xray)
  vpn-director restart tunnel      # Restart only Tunnel Director
  vpn-director update              # Update ipsets from IPdeny

Alias:
  vpd                              # Short alias (add to profile.add)
EOF
}

###################################################################################################
# Load modules (deferred until after help check)
###################################################################################################
_load_modules() {
    [[ ${_MODULES_LOADED:-0} -eq 1 ]] && return 0
    . "$SCRIPT_DIR/lib/common.sh"
    . "$SCRIPT_DIR/lib/firewall.sh"
    . "$SCRIPT_DIR/lib/config.sh"
    . "$SCRIPT_DIR/lib/ipset.sh" --source-only
    . "$SCRIPT_DIR/lib/tunnel.sh" --source-only
    . "$SCRIPT_DIR/lib/tproxy.sh" --source-only
    _MODULES_LOADED=1
}

###################################################################################################
# Helper to ensure ipsets (handles space-separated list)
###################################################################################################
_ensure_ipsets() {
    local set
    for set in "$@"; do
        [[ -z $set ]] && continue
        ipset_ensure "$set" || return 1
    done
}

###################################################################################################
# Commands
###################################################################################################

cmd_status() {
    _load_modules
    case "$COMPONENT" in
        ""|all)
            ipset_status
            echo ""
            tunnel_status
            echo ""
            tproxy_status
            ;;
        ipset)
            ipset_status
            ;;
        tunnel)
            tunnel_status
            ;;
        xray|tproxy)
            tproxy_status
            ;;
        *)
            echo "Unknown component: $COMPONENT" >&2
            exit 1
            ;;
    esac
}

cmd_apply() {
    _load_modules
    acquire_lock "vpn-director"

    # Handle --dry-run: show plan without applying (skip boot wait and lock)
    if [[ $DRY_RUN -eq 1 ]]; then
        case "$COMPONENT" in
            ""|all)
                log "DRY-RUN: would apply all components"
                log "DRY-RUN: tunnel ipsets needed: $(tunnel_get_required_ipsets)"
                log "DRY-RUN: tproxy ipsets needed: $(tproxy_get_required_ipsets)"
                ;;
            tunnel)
                log "DRY-RUN: would apply tunnel"
                log "DRY-RUN: tunnel ipsets needed: $(tunnel_get_required_ipsets)"
                ;;
            xray|tproxy)
                log "DRY-RUN: would apply tproxy"
                log "DRY-RUN: tproxy ipsets needed: $(tproxy_get_required_ipsets)"
                ;;
            *)
                echo "Unknown component: $COMPONENT" >&2
                exit 1
                ;;
        esac
        return 0
    fi

    # Wait for network if system just booted (before any downloads)
    _ipset_boot_wait

    # Handle --force: pass to ipset module
    [[ $FORCE -eq 1 ]] && export IPSET_FORCE_UPDATE=1

    local required_ipsets=""

    case "$COMPONENT" in
        ""|all)
            required_ipsets="$(tunnel_get_required_ipsets) $(tproxy_get_required_ipsets)"
            required_ipsets=$(echo $required_ipsets | xargs -n1 | sort -u | xargs)

            if [[ -n $required_ipsets ]]; then
                log "Ensuring ipsets: $required_ipsets"
                # shellcheck disable=SC2086
                _ensure_ipsets $required_ipsets
            fi

            tunnel_apply
            tproxy_apply
            ;;
        tunnel)
            required_ipsets=$(tunnel_get_required_ipsets)
            if [[ -n $required_ipsets ]]; then
                # shellcheck disable=SC2086
                _ensure_ipsets $required_ipsets
            fi
            tunnel_apply
            ;;
        xray|tproxy)
            required_ipsets=$(tproxy_get_required_ipsets)
            if [[ -n $required_ipsets ]]; then
                # shellcheck disable=SC2086
                _ensure_ipsets $required_ipsets
            fi
            tproxy_apply
            ;;
        *)
            echo "Unknown component: $COMPONENT" >&2
            exit 1
            ;;
    esac
}

cmd_stop() {
    _load_modules
    acquire_lock "vpn-director"

    case "$COMPONENT" in
        ""|all)
            tproxy_stop
            tunnel_stop
            ;;
        tunnel)
            tunnel_stop
            ;;
        xray|tproxy)
            tproxy_stop
            ;;
        *)
            echo "Unknown component: $COMPONENT" >&2
            exit 1
            ;;
    esac
}

cmd_restart() {
    case "$COMPONENT" in
        ""|all)
            cmd_stop
            cmd_apply
            ;;
        tunnel)
            COMPONENT=tunnel cmd_stop
            COMPONENT=tunnel cmd_apply
            ;;
        xray|tproxy)
            COMPONENT=xray cmd_stop
            COMPONENT=xray cmd_apply
            ;;
        *)
            echo "Unknown component: $COMPONENT" >&2
            exit 1
            ;;
    esac
}

cmd_update() {
    _load_modules
    acquire_lock "vpn-director"

    # Wait for network if system just booted (before any downloads)
    _ipset_boot_wait

    local required_ipsets
    required_ipsets="$(tunnel_get_required_ipsets) $(tproxy_get_required_ipsets)"
    required_ipsets=$(echo $required_ipsets | xargs -n1 | sort -u | xargs)

    if [[ -n $required_ipsets ]]; then
        log "Updating ipsets: $required_ipsets"
        # Force update: download fresh data even if cache exists
        export IPSET_FORCE_UPDATE=1
        # shellcheck disable=SC2086
        _ensure_ipsets $required_ipsets
    fi

    tunnel_apply
    tproxy_apply

    log "Update complete"
}

###################################################################################################
# Main
###################################################################################################

# Exit early if sourced for testing
[[ $_SOURCE_ONLY -eq 1 ]] && return 0 2>/dev/null || true

case "$COMMAND" in
    help|--help|-h)
        show_help
        ;;
    status)
        cmd_status
        ;;
    apply)
        cmd_apply
        ;;
    stop)
        cmd_stop
        ;;
    restart)
        cmd_restart
        ;;
    update)
        cmd_update
        ;;
    *)
        echo "Unknown command: $COMMAND" >&2
        echo "Run 'vpn-director --help' for usage" >&2
        exit 1
        ;;
esac
