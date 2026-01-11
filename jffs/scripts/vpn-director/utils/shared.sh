#!/usr/bin/env bash

###################################################################################################
# shared.sh - INTERNAL shared library for ipset_builder.sh and tunnel_director.sh
# -------------------------------------------------------------------------------------------------
# Purpose:
#   Provides helper functions and state file paths shared by the scripts.
#   User configuration belongs in config.sh - not here.
###################################################################################################

# -------------------------------------------------------------------------------------------------
# Disable unneeded shellcheck warnings
# -------------------------------------------------------------------------------------------------
# shellcheck disable=SC2018
# shellcheck disable=SC2019
# shellcheck disable=SC2034

###################################################################################################
# 1. Functions
# -------------------------------------------------------------------------------------------------
# derive_set_name <name> - return lowercased <name> if ≤31 chars;
#                          otherwise return a stable 24-char SHA-256 prefix alias,
#                          and log a notice about the rename
###################################################################################################
derive_set_name() {
    local set="$1" max=31 set_lc hash

    set_lc=$(printf '%s' "$set" | tr 'A-Z' 'a-z')

    # Fits already? Return as-is
    if [[ "${#set_lc}" -le "$max" ]]; then
        printf '%s\n' "$set_lc"
        return 0
    fi

    hash="$(printf '%s' "$set_lc" | compute_hash | cut -c1-24)"

    log -l TRACE "Assigned alias='$hash' for set='$set_lc'" \
        "because set name exceeds $max chars"

    printf '%s\n' "$hash"
}

###################################################################################################
# 2. State files (flags & hashes)
# -------------------------------------------------------------------------------------------------
# IPS_BUILDER_DIR    - base dir for IPSet Builder state
# TUN_DIR_IPSETS_HASH - SHA-256 marker after a successful build of Tunnel Director ipsets
#
# TUN_DIRECTOR_DIR   - base dir for Tunnel Director state
# TUN_DIR_HASH       - last applied SHA-256 hash of normalized Tunnel Director rules
###################################################################################################
IPS_BUILDER_DIR="/tmp/ipset_builder"
TUN_DIR_IPSETS_HASH="$IPS_BUILDER_DIR/tun_dir_ipsets.sha256"

TUN_DIRECTOR_DIR="/tmp/tunnel_director"
TUN_DIR_HASH="$TUN_DIRECTOR_DIR/tun_dir_rules.sha256"

###################################################################################################
# 3. Create dirs & make all configuration constants read-only
###################################################################################################
mkdir -p "$IPS_BUILDER_DIR" "$TUN_DIRECTOR_DIR"

readonly \
    IPS_BUILDER_DIR \
    TUN_DIR_IPSETS_HASH \
    TUN_DIRECTOR_DIR \
    TUN_DIR_HASH
