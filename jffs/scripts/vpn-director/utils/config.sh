#!/usr/bin/env ash

###################################################################################################
# config.sh - load vpn-director.json and export variables
###################################################################################################

# shellcheck disable=SC2034

set -euo pipefail

###################################################################################################
# 1. Configuration file path
###################################################################################################
VPD_CONFIG_FILE="/jffs/scripts/vpn-director/vpn-director.json"

###################################################################################################
# 2. Validate config exists and is valid JSON
###################################################################################################
if [ ! -f "$VPD_CONFIG_FILE" ]; then
    echo "ERROR: Config not found: $VPD_CONFIG_FILE" >&2
    exit 1
fi

if ! jq empty "$VPD_CONFIG_FILE" 2>/dev/null; then
    echo "ERROR: Invalid JSON: $VPD_CONFIG_FILE" >&2
    exit 1
fi

###################################################################################################
# 3. Helper functions
###################################################################################################
_cfg() { jq -r "$1 // empty" "$VPD_CONFIG_FILE"; }
_cfg_arr() { jq -r "$1 // [] | .[]" "$VPD_CONFIG_FILE" | tr '\n' ' ' | sed 's/ $//'; }

###################################################################################################
# 4. Tunnel Director variables
###################################################################################################
TUN_DIR_RULES=$(_cfg_arr '.tunnel_director.rules')
IPS_BDR_DIR=$(_cfg '.tunnel_director.ipset_dump_dir')

###################################################################################################
# 5. Xray variables
###################################################################################################
XRAY_CLIENTS=$(_cfg_arr '.xray.clients')
XRAY_SERVERS=$(_cfg_arr '.xray.servers')
XRAY_EXCLUDE_SETS=$(_cfg_arr '.xray.exclude_sets')

###################################################################################################
# 6. Advanced: Xray
###################################################################################################
XRAY_TPROXY_PORT=$(_cfg '.advanced.xray.tproxy_port')
XRAY_ROUTE_TABLE=$(_cfg '.advanced.xray.route_table')
XRAY_RULE_PREF=$(_cfg '.advanced.xray.rule_pref')
XRAY_FWMARK=$(_cfg '.advanced.xray.fwmark')
XRAY_FWMARK_MASK=$(_cfg '.advanced.xray.fwmark_mask')
XRAY_CHAIN=$(_cfg '.advanced.xray.chain')
XRAY_CLIENTS_IPSET=$(_cfg '.advanced.xray.clients_ipset')
XRAY_SERVERS_IPSET=$(_cfg '.advanced.xray.servers_ipset')

###################################################################################################
# 7. Advanced: Tunnel Director
###################################################################################################
TUN_DIR_CHAIN_PREFIX=$(_cfg '.advanced.tunnel_director.chain_prefix')
TUN_DIR_PREF_BASE=$(_cfg '.advanced.tunnel_director.pref_base')
TUN_DIR_MARK_MASK=$(_cfg '.advanced.tunnel_director.mark_mask')
TUN_DIR_MARK_SHIFT=$(_cfg '.advanced.tunnel_director.mark_shift')

###################################################################################################
# 8. Advanced: Boot
###################################################################################################
MIN_BOOT_TIME=$(_cfg '.advanced.boot.min_time')
BOOT_WAIT_DELAY=$(_cfg '.advanced.boot.wait_delay')

###################################################################################################
# 9. Make all variables read-only
###################################################################################################
readonly \
    VPD_CONFIG_FILE \
    TUN_DIR_RULES IPS_BDR_DIR \
    XRAY_CLIENTS XRAY_SERVERS XRAY_EXCLUDE_SETS \
    XRAY_TPROXY_PORT XRAY_ROUTE_TABLE XRAY_RULE_PREF \
    XRAY_FWMARK XRAY_FWMARK_MASK XRAY_CHAIN \
    XRAY_CLIENTS_IPSET XRAY_SERVERS_IPSET \
    TUN_DIR_CHAIN_PREFIX TUN_DIR_PREF_BASE \
    TUN_DIR_MARK_MASK TUN_DIR_MARK_SHIFT \
    MIN_BOOT_TIME BOOT_WAIT_DELAY
