#!/bin/sh
# KeeneticOS runs this with "start" when a WAN connection came up and "stop"
# when it went down ($interface, $address, $mask, $gateway describe it). The
# bypass ipset resolves the VPN endpoints and the ipset update downloads, so
# apply on start only. Detached for the reason netfilter.d gives.

PATH=/opt/sbin:/opt/bin:/usr/sbin:/usr/bin:/sbin:/bin
VPD_SCRIPT="${VPD_SCRIPT:-/opt/vpn-director/vpn-director.sh}"

[ "$1" = "start" ] || exit 0

if [ ! -x "$VPD_SCRIPT" ]; then
    logger -t "vpn-director-hook" "vpn-director not found at $VPD_SCRIPT"
    exit 0
fi
nohup "$VPD_SCRIPT" --wait apply >/dev/null 2>&1 &
exit 0
