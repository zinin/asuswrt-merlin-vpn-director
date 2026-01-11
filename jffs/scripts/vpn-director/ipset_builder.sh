#!/usr/bin/env ash

###################################################################################################
# ipset_builder.sh - ipset builder for country and combo sets
# -------------------------------------------------------------------------------------------------
# What this script does:
#   * Builds per-country ipsets from IPdeny feeds (IPv4 only).
#   * Generates "combo" sets (list:set) that union existing sets so firewall
#     rules can match a single key instead of many.
#   * Restores sets from cached dump files for fast boot; -u forces a fresh
#     download / rebuild even if a dump is present.
#   * Uses an atomic 'create -> swap -> destroy' flow for zero-downtime updates.
#   * Optionally adds a short post-boot delay (see BOOT_WAIT_DELAY in config.sh).
#   * Runs tunnel_director.sh and/or xray_tproxy.sh if requested via switches.
#
# Usage:
#   ipset_builder.sh           # normal run (restore when possible)
#   ipset_builder.sh -u        # force update of all ipsets
#   ipset_builder.sh -t        # launch tunnel_director.sh after build
#   ipset_builder.sh -x        # launch xray_tproxy.sh after build
#   ipset_builder.sh -u -t -x  # combinations of the above
#
# Requirements / Notes:
#   * All configuration lives in vpn-director.json. Review, edit, then run "ipt"
#     (helper alias) to apply changes without rebooting.
###################################################################################################

# -------------------------------------------------------------------------------------------------
# Disable unneeded shellcheck warnings
# -------------------------------------------------------------------------------------------------
# shellcheck disable=SC2086

# -------------------------------------------------------------------------------------------------
# Abort script on any error
# -------------------------------------------------------------------------------------------------
set -euo pipefail

###################################################################################################
# 0a. Parse args
###################################################################################################
update=0
start_tun_dir=0
start_xray_tproxy=0
extra_countries=""

while [ $# -gt 0 ]; do
    case "$1" in
        -u)  update=1;            shift ;;
        -t)  start_tun_dir=1;     shift ;;
        -x)  start_xray_tproxy=1; shift ;;
        -c)  extra_countries="$2"; shift 2 ;;
        *)                        break ;;
    esac
done

###################################################################################################
# 0b. Load utils and shared variables
###################################################################################################
. /jffs/scripts/vpn-director/utils/common.sh
. /jffs/scripts/vpn-director/utils/firewall.sh
. /jffs/scripts/vpn-director/utils/shared.sh
. /jffs/scripts/vpn-director/utils/config.sh

acquire_lock  # avoid concurrent runs

###################################################################################################
# 0c. Define constants & variables
###################################################################################################

# Paths for dumps
COUNTRY_DUMP_DIR="$IPS_BDR_DIR/ipsets"

# IPdeny downloads
IPDENY_COUNTRY_BASE_URL='https://www.ipdeny.com/ipblocks/data/aggregated'
IPDENY_COUNTRY_FILE_SUFFIX='-aggregated.zone'

# Mutable runtime flags
warnings=0  # non-critical issues encountered

###################################################################################################
# 0d. Define helper functions
###################################################################################################

download_file() {
    local url="$1" file="$2"
    shift 2
    local label="" http_code short_label

    # Pull out --label flag
    while [ $# -gt 0 ]; do
        case "$1" in
            --label) shift; label="$1" ;;
        esac
        shift
    done

    if [ -n "$label" ]; then
        short_label="$label"
    else
        label="$url"
        short_label="${url##*/}"
    fi

    log "Downloading '$label'..."

    if ! http_code=$(curl -sS -o "$file" -w "%{http_code}" "$url"); then
        http_code=000
    fi

    [ -n "$http_code" ] || http_code=000

    if [ "$http_code" -ge 200 ] && [ "$http_code" -lt 300 ]; then
        log "Successfully downloaded '$short_label'"
    else
        rm -f "$file"
        log -l err "Failed to download '$short_label' (HTTP $http_code)"
        exit 1
    fi
}

ipset_exists() {
    ipset list -n "$1" >/dev/null 2>&1
}

get_ipset_count() {
    ipset list "$1" 2>/dev/null |
        awk '/Number of entries:/ { print $4; exit }' || true
}

# Round up to the next power of two (>= 1)
_next_pow2() {
    local n=${1:-1} p=1

    [ "$n" -lt 1 ] && n=1

    while [ "$p" -lt "$n" ]; do
        p=$((p<<1))
    done

    printf '%s\n' "$p"
}

# Size from element count:
# - target load ~ 0.75 -> buckets ~ ceil(4 * n / 3)
# - round buckets to next pow2
# - floor at 1024 for safety
_calc_ipset_size() {
    local n=${1:-0} floor=1024 buckets

    buckets=$(((4 * n + 2) / 3))
    [ "$buckets" -lt "$floor" ] && buckets=$floor

    _next_pow2 "$buckets"
}

print_create_ipset() {
    local set_name="$1" cnt="${2:-0}" size
    size="$(_calc_ipset_size "$cnt")"

    printf 'create %s hash:net family inet hashsize %s maxelem %s\n' \
        "$set_name" "$size" "$size"
}

print_add_entry() { printf 'add %s %s\n' "$1" "$2"; }

print_swap_and_destroy() {
    printf 'swap %s %s\n' "$1" "$2"
    printf 'destroy %s\n' "$1"
}

save_dump() { ipset save "$1" > "$2"; }

restore_dump() {
    local set_name="$1" dump="$2" cnt rc=0

    # If forcing update or dump missing, signal caller to rebuild
    if [ "$update" -eq 1 ] || [ ! -f "$dump" ]; then
        return 1
    fi

    if ipset_exists "$set_name"; then
        # Existing set: restore into a temp clone, then swap
        local tmp_set="${set_name}_tmp" restore_script
        restore_script=$(tmp_file)
        ipset destroy "$tmp_set" 2>/dev/null || true

        {
            sed -e "s/^create $set_name /create $tmp_set /" \
                -e "s/^add $set_name /add $tmp_set /" "$dump"
            print_swap_and_destroy "$tmp_set" "$set_name"
        } > "$restore_script"

        ipset restore -! < "$restore_script" || rc=$?
        rm -f "$restore_script"
    else
        # Set doesn't exist yet - restore directly
        ipset restore -! < "$dump" || rc=$?
    fi

    if [ "$rc" -ne 0 ]; then
        log -l warn "Restore failed for ipset '$set_name'; will rebuild"
        return 1
    fi

    cnt=$(get_ipset_count "$set_name")
    log "Restored ipset '$set_name' from dump ($cnt entries)"

    return 0
}

save_hashes() {
    local tun_dir_rules_file
    tun_dir_rules_file=$(tmp_file)

    printf '%s\n' $TUN_DIR_RULES | awk 'NF' > "$tun_dir_rules_file"
    printf '%s\n' "$(compute_hash "$tun_dir_rules_file")" > "$TUN_DIR_IPSETS_HASH"
}

build_ipset() {
    local set_name="$1" src="$2" dump="$3"
    shift 3

    local label=""
    while [ $# -gt 0 ]; do
        case "$1" in
            --label) shift; label="$1" ;;
        esac
        shift
    done

    local target_set cidr_list restore_script
    local cnt=0 rc=0

    log "Building ipset '$set_name'..."

    # Decide whether we need a temporary set (for "hot" updates) or in-place create
    if ipset_exists "$set_name"; then
        target_set="${set_name}_tmp"
        ipset destroy "$target_set" 2>/dev/null || true
    else
        target_set="$set_name"
    fi

    # Download CIDR list
    cidr_list=$(tmp_file)
    download_file "$src" "$cidr_list" --label "$label"

    cnt=$(wc -l < "$cidr_list")
    log "Total CIDRs for ipset '$set_name': $cnt"

    # Generate an ipset-restore script
    restore_script=$(tmp_file)
    {
        print_create_ipset "$target_set" "$cnt"

        while IFS= read -r line; do
            [ -n "$line" ] || continue
            print_add_entry "$target_set" "$line"
        done < "$cidr_list"

        if [ "$target_set" != "$set_name" ]; then
            print_swap_and_destroy "$target_set" "$set_name"
        fi
    } > "$restore_script"

    # Apply
    log "Applying ipset '$set_name'..."
    ipset restore -! < "$restore_script" || rc=$?
    rm -f "$restore_script" "$cidr_list"

    if [ "$rc" -ne 0 ]; then
        log -l err "Failed to build ipset '$set_name'"
        exit 1
    fi

    # Save dump and log
    cnt=$(get_ipset_count "$set_name")
    save_dump "$set_name" "$dump"

    if [ "$target_set" = "$set_name" ]; then
        log "Created new ipset '$set_name' ($cnt entries)"
    else
        log "Updated existing ipset '$set_name' ($cnt entries)"
    fi

    if [ "$cnt" -eq 0 ]; then
        log -l warn "ipset '$set_name' is empty"
        warnings=1
    fi
}

###################################################################################################
# 0e. Run initialization checks
###################################################################################################

# Defer if system just booted
if [ "$BOOT_WAIT_DELAY" -ne 0 ] && \
    ! awk -v min="$MIN_BOOT_TIME" '{ exit ($1 < min) }' /proc/uptime;
then
    log "Uptime < ${MIN_BOOT_TIME}s, sleeping ${BOOT_WAIT_DELAY}s..."
    sleep "$BOOT_WAIT_DELAY"
fi

# Ensure dump directories exist
mkdir -p "$COUNTRY_DUMP_DIR"

# Let user know if update was requested
if [ "$update" -eq 1 ]; then
    log "Update requested: forcing rebuild of all ipsets"
fi

###################################################################################################
# 1. Build per-country ipsets (IPdeny, IPv4 only)
###################################################################################################

# All two-letter ISO country codes, lowercase
ALL_COUNTRY_CODES='
ad ae af ag ai al am ao aq ar as at au aw ax az ba bb bd be bf bg bh bi bj bl bm bn bo bq br bs bt
bv bw by bz ca cc cd cf cg ch ci ck cl cm cn co cr cu cv cw cx cy cz de dj dk dm do dz ec ee eg eh
er es et fi fj fk fm fo fr ga gb gd ge gf gg gh gi gl gm gn gp gq gr gs gt gu gw gy hk hm hn hr ht
hu id ie il im in io iq ir is it je jm jo jp ke kg kh ki km kn kp kr kw ky kz la lb lc li lk lr ls
lt lu lv ly ma mc md me mf mg mh mk ml mm mn mo mp mq mr ms mt mu mv mw mx my mz na nc ne nf ng ni
nl no np nr nu nz om pa pe pf pg ph pk pl pm pn pr ps pt pw py qa re ro rs ru rw sa sb sc sd se sg
sh si sj sk sl sm sn so sr ss st sv sx sy sz tc td tf tg th tj tk tl tm tn to tr tt tv tw tz ua ug
um us uy uz va vc ve vg vi vn vu wf ws ye yt za zm zw
'

# Parse country codes from TUN_DIR_RULES
# Accepts "cc" tokens from field 4 (set) and field 5 (set_excl)
# Reads rules from stdin
parse_country_codes() {
    awk -v valid="$ALL_COUNTRY_CODES" '
        function add_code(tok, base) {
            gsub(/[[:space:]]+/, "", tok)
            if (tok ~ /^[a-z]{2}$/) {
                base = tok
                if (V[base]) seen[base] = 1
            }
        }
        BEGIN {
            n = split(valid, arr, /[[:space:]\r\n]+/)
            for (i = 1; i <= n; i++) if (arr[i] != "") V[arr[i]] = 1
        }
        {
            # Split on ":" outside [...] (for IPv6 literals in src)
            depth = 0; tok = ""; nf2 = 0
            for (i = 1; i <= length($0); i++) {
                c = substr($0, i, 1)
                if (c == "[") depth++
                else if (c == "]" && depth > 0) depth--
                if (c == ":" && depth == 0) {
                    f[++nf2] = tok; tok = ""
                }
                else tok = tok c
            }
            f[++nf2] = tok

            # Field 4 = set, Field 5 = set_excl
            for (fld = 4; fld <= 5; fld++) {
                if (fld > nf2) continue
                n = split(tolower(f[fld]), parts, ",")
                for (i = 1; i <= n; i++) add_code(parts[i])
            }

            for (i = 1; i <= nf2; i++) delete f[i]
        }
        END { for (c in seen) print c }
    ' | sort
}

build_country_ipsets() {
    local tun_cc extra_cc xray_cc all_cc missing_cc="" cc set_name dump url

    tun_cc="$(printf '%s\n' $TUN_DIR_RULES | awk 'NF' | parse_country_codes)"

    # Add extra countries from -c parameter (comma-separated to space-separated)
    extra_cc=""
    if [ -n "$extra_countries" ]; then
        extra_cc="$(printf '%s' "$extra_countries" | tr ',' ' ')"
    fi

    # Add countries from XRAY_EXCLUDE_SETS (already space-separated from config.sh)
    xray_cc="${XRAY_EXCLUDE_SETS:-}"

    # Merge and deduplicate
    all_cc="$(printf '%s %s %s' "$tun_cc" "$extra_cc" "$xray_cc" | xargs -n1 2>/dev/null | sort -u | xargs)"

    if [ -z "$all_cc" ]; then
        log "Step 1: no country references; skipping"
        return 0
    fi

    log "Step 1: building country ipsets using IPdeny..."

    # Try restoring dumps first
    if [ "$update" -eq 1 ]; then
        missing_cc="$all_cc"
    else
        log "Attempting to restore country ipsets from dumps..."
        for cc in $all_cc; do
            set_name="$cc"
            dump="${COUNTRY_DUMP_DIR}/${set_name}-ipdeny.dump"
            restore_dump "$set_name" "$dump" || missing_cc="$missing_cc $cc"
        done
        missing_cc="$(printf '%s\n' "$missing_cc" | xargs)"
    fi

    if [ -z "$missing_cc" ]; then
        log "All country ipsets restored from dumps"
        return 0
    fi

    log "Building IPdeny ipsets for: $missing_cc"

    for cc in $missing_cc; do
        set_name="$cc"
        url="${IPDENY_COUNTRY_BASE_URL}/${cc}${IPDENY_COUNTRY_FILE_SUFFIX}"
        dump="${COUNTRY_DUMP_DIR}/${set_name}-ipdeny.dump"

        build_ipset "$set_name" "$url" "$dump" \
            --label "${cc}${IPDENY_COUNTRY_FILE_SUFFIX}"
    done
}

build_country_ipsets

###################################################################################################
# 2. Build combo ipsets
###################################################################################################

# Parse combo ipsets from TUN_DIR_RULES
# Emits only fields that contain a comma (e.g., "us,ca,uk")
# Reads rules from stdin
parse_combo_from_rules() {
    awk '
        function emit_combo(field, f, n, a, i, t, out) {
            f = field
            gsub(/[[:space:]]/, "", f)
            if (f ~ /,/) {
                n = split(f, a, ",")
                out = ""
                for (i = 1; i <= n; i++) {
                    t = a[i]
                    out = out (out ? "," : "") t
                }
                print out
            }
        }

        {
            # Split on ":" outside [...]
            depth = 0; tok = ""; nf2 = 0
            for (i = 1; i <= length($0); i++) {
                c = substr($0, i, 1)
                if (c == "[") depth++
                else if (c == "]" && depth > 0) depth--
                if (c == ":" && depth == 0) {
                    F[++nf2] = tok; tok = ""
                }
                else tok = tok c
            }
            F[++nf2] = tok

            if (nf2 >= 4) emit_combo(F[4])
            if (nf2 >= 5) emit_combo(F[5])
        }
    ' | sort -u
}

# Check if all combo ipsets already exist
_all_combo_present_for() {
    local list="$1" line set_name

    for line in $list; do
        set_name="$(derive_set_name "${line//,/_}")"
        ipset_exists "$set_name" || return 1
    done

    return 0
}

build_combo_ipsets() {
    local combo_ipsets line set_name key member added

    combo_ipsets="$(printf '%s\n' $TUN_DIR_RULES | parse_combo_from_rules | awk 'NF')"

    if [ -z "$combo_ipsets" ]; then
        log "Step 2: no combo ipsets required; skipping"
        return 0
    fi

    if _all_combo_present_for "$combo_ipsets"; then
        log "Step 2: all combo ipsets already present; skipping"
        return 0
    fi

    log "Step 2: building combo ipsets..."

    while IFS= read -r line; do
        [ -n "$line" ] || continue
        set_name="$(derive_set_name "${line//,/_}")"

        if ipset_exists "$set_name"; then
            log "Combo ipset '$set_name' already exists; skipping"
            continue
        fi

        ipset create "$set_name" list:set

        added=0
        for key in ${line//,/ }; do
            member="$(derive_set_name "$key")"

            if ! ipset_exists "$member"; then
                log -l warn "Combo '$set_name': member '$member' not found; skipping"
                warnings=1
                continue
            fi

            if ipset add "$set_name" "$member" 2>/dev/null; then
                added=$((added + 1))
            fi
        done

        if [ "$added" -eq 0 ]; then
            ipset destroy "$set_name" 2>/dev/null || true
            log -l warn "Combo '$set_name' had no valid members; not created"
            warnings=1
            continue
        fi

        log "Created combo ipset '$set_name'"
    done <<EOF
$combo_ipsets
EOF
}

build_combo_ipsets

###################################################################################################
# 3. Finalize
###################################################################################################
if [ "$warnings" -eq 0 ]; then
    log "All ipsets built successfully"
else
    log -l warn "Completed with warnings; check logs"
fi

# Save hashes for Tunnel Director
save_hashes

# Start Tunnel Director if requested
[ "$start_tun_dir" -eq 1 ] && /jffs/scripts/vpn-director/tunnel_director.sh

# Start Xray TPROXY if requested
[ "$start_xray_tproxy" -eq 1 ] && /jffs/scripts/vpn-director/xray_tproxy.sh

exit 0
