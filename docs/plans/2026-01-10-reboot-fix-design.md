# Reboot Fix Design

## Problem

After router reboot, Xray TPROXY rules are not applied because:

1. `ipset_builder.sh` parses country codes only from `TUN_DIR_RULES`
2. When `TUN_DIR_RULES` is empty but `XRAY_EXCLUDE_SETS='ru'`, ipset `ru` is not created
3. `xray_tproxy.sh` exits with warning: "Required ipset 'ru' not found"

## Root Cause

`ipset_builder.sh` doesn't know about `XRAY_EXCLUDE_SETS` from xray/config.sh.

During initial setup, `configure.sh` works around this by passing `-c "$XRAY_EXCLUDE_SETS_LIST"` flag, but this flag is not used during reboot.

## Solution

### 1. ipset_builder.sh - Load Xray Config

Load `/jffs/scripts/xray/config.sh` after firewall config:

```ash
# After loading firewall/config.sh
XRAY_CONFIG="/jffs/scripts/xray/config.sh"
if [ -f "$XRAY_CONFIG" ]; then
    . "$XRAY_CONFIG"
fi
```

### 2. ipset_builder.sh - Parse XRAY_EXCLUDE_SETS

In `build_country_ipsets()`, add xray exclusions to country list:

```ash
# Add countries from XRAY_EXCLUDE_SETS
xray_cc=""
if [ -n "${XRAY_EXCLUDE_SETS:-}" ]; then
    xray_cc="$(printf '%s' "$XRAY_EXCLUDE_SETS" | tr ',' ' ')"
fi

# Merge all sources
all_cc="$(printf '%s %s %s' "$tun_cc" "$extra_cc" "$xray_cc" | xargs -n1 2>/dev/null | sort -u | xargs)"
```

### 3. common.sh - Add File Logging

Add log file support with rotation:

```ash
LOG_FILE="/tmp/vpn_director.log"
MAX_LOG_SIZE=102400  # 100KB

log_to_file() {
    local msg="$(date '+%Y-%m-%d %H:%M:%S') [$_log_tag] $*"

    # Rotate if file exceeds limit
    if [ -f "$LOG_FILE" ] && [ "$(wc -c < "$LOG_FILE")" -gt "$MAX_LOG_SIZE" ]; then
        mv "$LOG_FILE" "${LOG_FILE}.old"
    fi

    printf '%s\n' "$msg" >> "$LOG_FILE"
}
```

Modify existing `log()` function to call `log_to_file` internally.

## Files Changed

| File | Change |
|------|--------|
| `jffs/scripts/firewall/ipset_builder.sh` | Load xray config, parse XRAY_EXCLUDE_SETS |
| `jffs/scripts/utils/common.sh` | Add file logging with rotation |

## Files Unchanged

- `services-start` / `firewall-start` - startup hooks remain as-is
- `xray_tproxy.sh` - already correctly validates ipsets
- `configure.sh` - `-c` flag kept for backward compatibility

## Expected Result

After reboot:
1. `services-start` calls `ipset_builder.sh -t -x`
2. `ipset_builder.sh` reads both configs, creates ipset `ru`
3. `xray_tproxy.sh` successfully applies rules
4. All steps logged to `/tmp/vpn_director.log`

## Log Location

`/tmp` chosen over `/jffs` because:
- RAM-based, no flash wear
- Cleared on reboot (old logs not needed)
- Fast writes
