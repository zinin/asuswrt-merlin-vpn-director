# Reboot Fix Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix ipsets not being created after reboot when TUN_DIR_RULES is empty but XRAY_EXCLUDE_SETS has values.

**Architecture:** ipset_builder.sh will load xray/config.sh and merge XRAY_EXCLUDE_SETS into country list. All scripts will log to /tmp/vpn_director.log for debugging.

**Tech Stack:** ash shell, ipset, iptables, Asuswrt-Merlin

---

## Task 1: Add File Logging to common.sh

**Files:**
- Modify: `jffs/scripts/utils/common.sh:327-352`

**Step 1: Add log file constants after _log_tag definition**

Insert after line 327 (`_log_tag="$(get_script_name -n)"`):

```ash
LOG_FILE="/tmp/vpn_director.log"
MAX_LOG_SIZE=102400  # 100KB
```

**Step 2: Add log_to_file function before log()**

Insert before line 329 (before `log()` function):

```ash
# -------------------------------------------------------------------------------------------------
# log_to_file - append timestamped message to log file with rotation
# -------------------------------------------------------------------------------------------------
log_to_file() {
    local msg="$(date '+%Y-%m-%d %H:%M:%S') [$_log_tag] $*"

    # Rotate if file exceeds limit
    if [ -f "$LOG_FILE" ] && [ "$(wc -c < "$LOG_FILE")" -gt "$MAX_LOG_SIZE" ]; then
        mv "$LOG_FILE" "${LOG_FILE}.old"
    fi

    printf '%s\n' "$msg" >> "$LOG_FILE"
}
```

**Step 3: Modify log() to call log_to_file**

Replace line 351:
```ash
    logger -s -t "$_log_tag" -p "user.$level" "${prefix}$*"
```

With:
```ash
    logger -s -t "$_log_tag" -p "user.$level" "${prefix}$*"
    log_to_file "${prefix}$*"
```

**Step 4: Verify syntax**

Run: `ash -n jffs/scripts/utils/common.sh`
Expected: No output (syntax OK)

**Step 5: Commit**

```bash
git add jffs/scripts/utils/common.sh
git commit -m "feat(logging): add file logging to /tmp/vpn_director.log

- Add LOG_FILE and MAX_LOG_SIZE constants
- Add log_to_file() function with rotation at 100KB
- Modify log() to write to both syslog and file"
```

---

## Task 2: Load Xray Config in ipset_builder.sh

**Files:**
- Modify: `jffs/scripts/firewall/ipset_builder.sh:56-66`

**Step 1: Add xray config loading after firewall config**

Insert after line 64 (`. "$DIR/fw_shared.sh"`):

```ash

# Load Xray config for XRAY_EXCLUDE_SETS (optional)
XRAY_CONFIG="/jffs/scripts/xray/config.sh"
if [ -f "$XRAY_CONFIG" ]; then
    . "$XRAY_CONFIG"
fi
```

**Step 2: Verify syntax**

Run: `ash -n jffs/scripts/firewall/ipset_builder.sh`
Expected: No output (syntax OK)

**Step 3: Commit**

```bash
git add jffs/scripts/firewall/ipset_builder.sh
git commit -m "feat(ipset): load xray config for XRAY_EXCLUDE_SETS

Allows ipset_builder to see country codes from xray exclusions."
```

---

## Task 3: Parse XRAY_EXCLUDE_SETS in build_country_ipsets

**Files:**
- Modify: `jffs/scripts/firewall/ipset_builder.sh:376-393`

**Step 1: Add xray_cc variable to local declaration**

Replace line 377:
```ash
    local tun_cc extra_cc all_cc missing_cc="" cc set_name dump url
```

With:
```ash
    local tun_cc extra_cc xray_cc all_cc missing_cc="" cc set_name dump url
```

**Step 2: Add XRAY_EXCLUDE_SETS parsing after extra_cc block**

Insert after line 385 (`extra_cc="$(printf '%s' "$extra_countries" | tr ',' ' ')"`), before the closing `fi`:

After the `fi` on line 385, add:

```ash

    # Add countries from XRAY_EXCLUDE_SETS (space or comma separated)
    xray_cc=""
    if [ -n "${XRAY_EXCLUDE_SETS:-}" ]; then
        xray_cc="$(printf '%s' "$XRAY_EXCLUDE_SETS" | tr ',' ' ')"
    fi
```

**Step 3: Update all_cc merge to include xray_cc**

Replace line 388:
```ash
    all_cc="$(printf '%s %s' "$tun_cc" "$extra_cc" | xargs -n1 2>/dev/null | sort -u | xargs)"
```

With:
```ash
    all_cc="$(printf '%s %s %s' "$tun_cc" "$extra_cc" "$xray_cc" | xargs -n1 2>/dev/null | sort -u | xargs)"
```

**Step 4: Verify syntax**

Run: `ash -n jffs/scripts/firewall/ipset_builder.sh`
Expected: No output (syntax OK)

**Step 5: Commit**

```bash
git add jffs/scripts/firewall/ipset_builder.sh
git commit -m "feat(ipset): parse XRAY_EXCLUDE_SETS for country ipsets

Country codes from XRAY_EXCLUDE_SETS are now included when building
ipsets. This fixes the reboot issue where ipsets weren't created
when TUN_DIR_RULES was empty but XRAY_EXCLUDE_SETS had values."
```

---

## Task 4: Update Script Header Documentation

**Files:**
- Modify: `jffs/scripts/firewall/ipset_builder.sh:1-26`

**Step 1: Update header comment to mention xray config**

Add after line 25 (`#     (helper alias) to apply changes without rebooting.`):

```ash
#   * Also reads /jffs/scripts/xray/config.sh for XRAY_EXCLUDE_SETS.
```

**Step 2: Commit**

```bash
git add jffs/scripts/firewall/ipset_builder.sh
git commit -m "docs(ipset): mention xray config in header"
```

---

## Task 5: Manual Testing on Router

**Prerequisites:**
- Copy modified files to router
- Ensure TUN_DIR_RULES is empty in firewall/config.sh
- Ensure XRAY_EXCLUDE_SETS='ru' in xray/config.sh

**Step 1: Clear existing state**

```bash
ipset destroy ru 2>/dev/null || true
rm -f /tmp/vpn_director.log
```

**Step 2: Run ipset_builder with flags**

```bash
/jffs/scripts/firewall/ipset_builder.sh -t -x
```

Expected output should include:
- `Step 1: building country ipsets using IPdeny...`
- `Building IPdeny ipsets for: ru`
- `xray_tproxy: Xray TPROXY routing applied successfully`

**Step 3: Verify ipset created**

```bash
ipset list ru | head -5
```

Expected: Shows ipset header with entries

**Step 4: Verify log file**

```bash
cat /tmp/vpn_director.log
```

Expected: Timestamped log entries from all scripts

**Step 5: Verify xray_tproxy status**

```bash
/jffs/scripts/xray/xray_tproxy.sh status
```

Expected: Shows XRAY_CLIENTS ipset, iptables chain, etc.

---

## Task 6: Final Commit and Cleanup

**Step 1: Squash or verify commits**

```bash
git log --oneline -5
```

Verify 4 commits from this plan.

**Step 2: Update design doc status (optional)**

Add "Implemented" status to design doc if desired.

---

## Verification Checklist

- [ ] `ash -n` passes for both modified files
- [ ] ipset `ru` created when TUN_DIR_RULES empty but XRAY_EXCLUDE_SETS='ru'
- [ ] Logs appear in /tmp/vpn_director.log
- [ ] xray_tproxy.sh applies rules successfully
- [ ] Log rotation works (file size check)
