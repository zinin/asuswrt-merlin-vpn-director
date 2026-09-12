#!/bin/sh
# KeeneticOS runs this after it rebuilt one netfilter table, once per table
# and family ($table: filter|nat|mangle, $type: iptables|ip6tables). Every
# rebuild deletes our chains, so re-apply. Detached: NDM runs its hooks one
# at a time under a 24-second timeout, and an apply can take longer. --wait
# queues the apply behind a running one, so a burst of rebuilds converges on
# the final state; an apply that finds nothing changed is cheap.
#
# Argv is "start" on 5.1.5, but nothing here reads it: a future NDM verb must
# re-apply too, and $type/$table already say whether the rebuild was ours.

# $type and $table are NDM's, out of the hook environment; nothing here assigns them.
# shellcheck disable=SC2154
PATH=/opt/sbin:/opt/bin:/usr/sbin:/usr/bin:/sbin:/bin
VPD_SCRIPT="${VPD_SCRIPT:-/opt/vpn-director/vpn-director.sh}"

[ "$type" = "iptables" ] || exit 0
case "$table" in
    mangle|nat|filter) ;;
    *) exit 0 ;;
esac

if [ ! -x "$VPD_SCRIPT" ]; then
    logger -t "vpn-director-hook" "vpn-director not found at $VPD_SCRIPT"
    exit 0
fi
if ! command -v nohup >/dev/null 2>&1; then
    logger -t "vpn-director-hook" "nohup not found: opkg install coreutils-nohup"
    exit 0
fi
nohup "$VPD_SCRIPT" --wait apply >/dev/null 2>&1 &
exit 0
