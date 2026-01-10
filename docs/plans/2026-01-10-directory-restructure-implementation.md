# Directory Restructure Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Consolidate all VPN Director scripts into `/jffs/scripts/vpn-director/` with clean separation of concerns.

**Architecture:** Move all scripts from scattered directories (firewall/, xray/, utils/) into a single vpn-director/ folder. Utils stay in utils/ subfolder, configs in configs/ subfolder, executables in root.

**Tech Stack:** Shell scripts (ash), git for version control

**Design doc:** `docs/plans/2026-01-10-directory-restructure-design.md`

---

### Task 1: Create new directory structure

**Files:**
- Create: `jffs/scripts/vpn-director/`
- Create: `jffs/scripts/vpn-director/configs/`
- Create: `jffs/scripts/vpn-director/utils/`

**Step 1: Create directories**

```bash
mkdir -p jffs/scripts/vpn-director/configs
mkdir -p jffs/scripts/vpn-director/utils
```

**Step 2: Verify structure**

```bash
ls -la jffs/scripts/vpn-director/
```

Expected: `configs/` and `utils/` subdirectories exist

**Step 3: Commit**

```bash
git add jffs/scripts/vpn-director/
git commit -m "chore: create vpn-director directory structure"
```

---

### Task 2: Move utility files to vpn-director/utils/

**Files:**
- Move: `jffs/scripts/utils/common.sh` → `jffs/scripts/vpn-director/utils/common.sh`
- Move: `jffs/scripts/utils/firewall.sh` → `jffs/scripts/vpn-director/utils/firewall.sh`
- Move: `jffs/scripts/utils/send-email.sh` → `jffs/scripts/vpn-director/utils/send-email.sh`
- Move: `jffs/scripts/firewall/fw_shared.sh` → `jffs/scripts/vpn-director/utils/shared.sh`

**Step 1: Move files with git**

```bash
git mv jffs/scripts/utils/common.sh jffs/scripts/vpn-director/utils/common.sh
git mv jffs/scripts/utils/firewall.sh jffs/scripts/vpn-director/utils/firewall.sh
git mv jffs/scripts/utils/send-email.sh jffs/scripts/vpn-director/utils/send-email.sh
git mv jffs/scripts/firewall/fw_shared.sh jffs/scripts/vpn-director/utils/shared.sh
```

**Step 2: Verify files moved**

```bash
ls -la jffs/scripts/vpn-director/utils/
```

Expected: `common.sh`, `firewall.sh`, `send-email.sh`, `shared.sh`

**Step 3: Commit**

```bash
git commit -m "refactor: move utility scripts to vpn-director/utils/"
```

---

### Task 3: Move config templates to vpn-director/configs/

**Files:**
- Move: `jffs/scripts/firewall/config.sh.template` → `jffs/scripts/vpn-director/configs/config-tunnel-director.sh.template`
- Move: `jffs/scripts/xray/config.sh.template` → `jffs/scripts/vpn-director/configs/config-xray.sh.template`

**Step 1: Move and rename files**

```bash
git mv jffs/scripts/firewall/config.sh.template jffs/scripts/vpn-director/configs/config-tunnel-director.sh.template
git mv jffs/scripts/xray/config.sh.template jffs/scripts/vpn-director/configs/config-xray.sh.template
```

**Step 2: Verify files moved**

```bash
ls -la jffs/scripts/vpn-director/configs/
```

Expected: `config-tunnel-director.sh.template`, `config-xray.sh.template`

**Step 3: Commit**

```bash
git commit -m "refactor: move config templates to vpn-director/configs/"
```

---

### Task 4: Move executable scripts to vpn-director/

**Files:**
- Move: `jffs/scripts/firewall/ipset_builder.sh` → `jffs/scripts/vpn-director/ipset_builder.sh`
- Move: `jffs/scripts/firewall/tunnel_director.sh` → `jffs/scripts/vpn-director/tunnel_director.sh`
- Move: `jffs/scripts/xray/xray_tproxy.sh` → `jffs/scripts/vpn-director/xray_tproxy.sh`
- Move: `jffs/scripts/utils/configure.sh` → `jffs/scripts/vpn-director/configure.sh`

**Step 1: Move files**

```bash
git mv jffs/scripts/firewall/ipset_builder.sh jffs/scripts/vpn-director/ipset_builder.sh
git mv jffs/scripts/firewall/tunnel_director.sh jffs/scripts/vpn-director/tunnel_director.sh
git mv jffs/scripts/xray/xray_tproxy.sh jffs/scripts/vpn-director/xray_tproxy.sh
git mv jffs/scripts/utils/configure.sh jffs/scripts/vpn-director/configure.sh
```

**Step 2: Verify files moved**

```bash
ls -la jffs/scripts/vpn-director/*.sh
```

Expected: `configure.sh`, `ipset_builder.sh`, `tunnel_director.sh`, `xray_tproxy.sh`

**Step 3: Commit**

```bash
git commit -m "refactor: move executable scripts to vpn-director/"
```

---

### Task 5: Update paths in common.sh

**Files:**
- Modify: `jffs/scripts/vpn-director/utils/common.sh`

**Step 1: Update LOG_FILE path**

In `common.sh`, change line 328:

```bash
# Old:
LOG_FILE="/tmp/vpn_director.log"

# New (no change needed - path is already correct)
```

**Step 2: Verify syntax**

```bash
ash -n jffs/scripts/vpn-director/utils/common.sh && echo "Syntax OK"
```

Expected: `Syntax OK`

**Step 3: Commit (if changes made)**

```bash
git add jffs/scripts/vpn-director/utils/common.sh
git commit -m "refactor: update paths in common.sh"
```

Note: common.sh has no hardcoded paths to other scripts, so no changes needed. Skip commit if no changes.

---

### Task 6: Update paths in shared.sh (formerly fw_shared.sh)

**Files:**
- Modify: `jffs/scripts/vpn-director/utils/shared.sh`

**Step 1: Update header comment**

Change the file header from:

```bash
# fw_shared.sh - INTERNAL shared library for ipset_builder.sh and tunnel_director.sh
```

To:

```bash
# shared.sh - INTERNAL shared library for ipset_builder.sh and tunnel_director.sh
```

**Step 2: Verify syntax**

```bash
ash -n jffs/scripts/vpn-director/utils/shared.sh && echo "Syntax OK"
```

Expected: `Syntax OK`

**Step 3: Commit**

```bash
git add jffs/scripts/vpn-director/utils/shared.sh
git commit -m "refactor: rename fw_shared.sh to shared.sh"
```

---

### Task 7: Update paths in send-email.sh

**Files:**
- Modify: `jffs/scripts/vpn-director/utils/send-email.sh`

**Step 1: Update source path**

Change line 32:

```bash
# Old:
. /jffs/scripts/utils/common.sh

# New:
. /jffs/scripts/vpn-director/utils/common.sh
```

**Step 2: Verify syntax**

```bash
ash -n jffs/scripts/vpn-director/utils/send-email.sh && echo "Syntax OK"
```

Expected: `Syntax OK`

**Step 3: Commit**

```bash
git add jffs/scripts/vpn-director/utils/send-email.sh
git commit -m "refactor: update source path in send-email.sh"
```

---

### Task 8: Update paths in ipset_builder.sh

**Files:**
- Modify: `jffs/scripts/vpn-director/ipset_builder.sh`

**Step 1: Update source paths**

Change lines 60-65:

```bash
# Old:
. /jffs/scripts/utils/common.sh
. /jffs/scripts/utils/firewall.sh

DIR="$(get_script_dir)"
. "$DIR/config.sh"
. "$DIR/fw_shared.sh"

# New:
. /jffs/scripts/vpn-director/utils/common.sh
. /jffs/scripts/vpn-director/utils/firewall.sh
. /jffs/scripts/vpn-director/utils/shared.sh
. /jffs/scripts/vpn-director/configs/config-tunnel-director.sh
```

**Step 2: Update Xray config path**

Change lines 68-71:

```bash
# Old:
XRAY_CONFIG="/jffs/scripts/xray/config.sh"
if [ -f "$XRAY_CONFIG" ]; then
    . "$XRAY_CONFIG"
fi

# New:
XRAY_CONFIG="/jffs/scripts/vpn-director/configs/config-xray.sh"
if [ -f "$XRAY_CONFIG" ]; then
    . "$XRAY_CONFIG"
fi
```

**Step 3: Update xray_tproxy.sh path at end of file**

Change line 573:

```bash
# Old:
[ "$start_xray_tproxy" -eq 1 ] && /jffs/scripts/xray/xray_tproxy.sh

# New:
[ "$start_xray_tproxy" -eq 1 ] && /jffs/scripts/vpn-director/xray_tproxy.sh
```

**Step 4: Verify syntax**

```bash
ash -n jffs/scripts/vpn-director/ipset_builder.sh && echo "Syntax OK"
```

Expected: `Syntax OK`

**Step 5: Commit**

```bash
git add jffs/scripts/vpn-director/ipset_builder.sh
git commit -m "refactor: update all paths in ipset_builder.sh"
```

---

### Task 9: Update paths in tunnel_director.sh

**Files:**
- Modify: `jffs/scripts/vpn-director/tunnel_director.sh`

**Step 1: Update source paths**

Change lines 55-60:

```bash
# Old:
. /jffs/scripts/utils/common.sh
. /jffs/scripts/utils/firewall.sh

DIR="$(get_script_dir)"
. "$DIR/config.sh"
. "$DIR/fw_shared.sh"

# New:
. /jffs/scripts/vpn-director/utils/common.sh
. /jffs/scripts/vpn-director/utils/firewall.sh
. /jffs/scripts/vpn-director/utils/shared.sh
. /jffs/scripts/vpn-director/configs/config-tunnel-director.sh
```

**Step 2: Verify syntax**

```bash
ash -n jffs/scripts/vpn-director/tunnel_director.sh && echo "Syntax OK"
```

Expected: `Syntax OK`

**Step 3: Commit**

```bash
git add jffs/scripts/vpn-director/tunnel_director.sh
git commit -m "refactor: update all paths in tunnel_director.sh"
```

---

### Task 10: Update paths in xray_tproxy.sh

**Files:**
- Modify: `jffs/scripts/vpn-director/xray_tproxy.sh`

**Step 1: Update source paths**

Change lines 33-37:

```bash
# Old:
. /jffs/scripts/utils/common.sh
. /jffs/scripts/utils/firewall.sh

SCRIPT_DIR="$(get_script_dir)"
. "$SCRIPT_DIR/config.sh"

# New:
. /jffs/scripts/vpn-director/utils/common.sh
. /jffs/scripts/vpn-director/utils/firewall.sh
. /jffs/scripts/vpn-director/configs/config-xray.sh
```

**Step 2: Verify syntax**

```bash
ash -n jffs/scripts/vpn-director/xray_tproxy.sh && echo "Syntax OK"
```

Expected: `Syntax OK`

**Step 3: Commit**

```bash
git add jffs/scripts/vpn-director/xray_tproxy.sh
git commit -m "refactor: update all paths in xray_tproxy.sh"
```

---

### Task 11: Update paths in configure.sh

**Files:**
- Modify: `jffs/scripts/vpn-director/configure.sh`

**Step 1: Update JFFS_DIR constant**

Change line 17:

```bash
# Old:
JFFS_DIR="/jffs/scripts"

# New:
JFFS_DIR="/jffs/scripts/vpn-director"
```

**Step 2: Update template path references**

Change line 423:

```bash
# Old:
if [ ! -f "$JFFS_DIR/xray/config.sh.template" ]; then
    print_error "Template not found: $JFFS_DIR/xray/config.sh.template"

# New:
if [ ! -f "$JFFS_DIR/configs/config-xray.sh.template" ]; then
    print_error "Template not found: $JFFS_DIR/configs/config-xray.sh.template"
```

**Step 3: Update xray config generation (lines 429-434)**

```bash
# Old:
sed "s|{{XRAY_SERVER_ADDRESS}}|$SELECTED_SERVER_ADDRESS|g" \
    /opt/etc/xray/config.json.template 2>/dev/null | \

# (keep as is - this path is correct)
```

**Step 4: Update xray/config.sh generation (lines 443-449)**

```bash
# Old:
sed "s|{{XRAY_CLIENTS}}|$xray_clients_escaped|g" \
    "$JFFS_DIR/xray/config.sh.template" | \
...
    > "$JFFS_DIR/xray/config.sh"
chmod +x "$JFFS_DIR/xray/config.sh"
print_success "Generated $JFFS_DIR/xray/config.sh"

# New:
sed "s|{{XRAY_CLIENTS}}|$xray_clients_escaped|g" \
    "$JFFS_DIR/configs/config-xray.sh.template" | \
...
    > "$JFFS_DIR/configs/config-xray.sh"
chmod +x "$JFFS_DIR/configs/config-xray.sh"
print_success "Generated $JFFS_DIR/configs/config-xray.sh"
```

**Step 5: Update firewall/config.sh generation (lines 454-460)**

```bash
# Old:
sed "s|{{TUN_DIR_RULES}}|$tun_dir_escaped|g" \
    "$JFFS_DIR/firewall/config.sh.template" \
    > "$JFFS_DIR/firewall/config.sh"
chmod +x "$JFFS_DIR/firewall/config.sh"
print_success "Generated $JFFS_DIR/firewall/config.sh"

# New:
sed "s|{{TUN_DIR_RULES}}|$tun_dir_escaped|g" \
    "$JFFS_DIR/configs/config-tunnel-director.sh.template" \
    > "$JFFS_DIR/configs/config-tunnel-director.sh"
chmod +x "$JFFS_DIR/configs/config-tunnel-director.sh"
print_success "Generated $JFFS_DIR/configs/config-tunnel-director.sh"
```

**Step 6: Update ipset_builder.sh path (lines 485-489)**

```bash
# Old:
if [ -x "$JFFS_DIR/firewall/ipset_builder.sh" ]; then
    "$JFFS_DIR/firewall/ipset_builder.sh" -c "$XRAY_EXCLUDE_SETS_LIST" || {

# New:
if [ -x "$JFFS_DIR/ipset_builder.sh" ]; then
    "$JFFS_DIR/ipset_builder.sh" -c "$XRAY_EXCLUDE_SETS_LIST" || {
```

**Step 7: Update tunnel_director.sh path (lines 494-498)**

```bash
# Old:
if [ -x "$JFFS_DIR/firewall/tunnel_director.sh" ]; then
    "$JFFS_DIR/firewall/tunnel_director.sh" || {

# New:
if [ -x "$JFFS_DIR/tunnel_director.sh" ]; then
    "$JFFS_DIR/tunnel_director.sh" || {
```

**Step 8: Update xray_tproxy.sh path (lines 504-508)**

```bash
# Old:
if [ -x "$JFFS_DIR/xray/xray_tproxy.sh" ]; then
    "$JFFS_DIR/xray/xray_tproxy.sh" || {

# New:
if [ -x "$JFFS_DIR/xray_tproxy.sh" ]; then
    "$JFFS_DIR/xray_tproxy.sh" || {
```

**Step 9: Update final message (line 532)**

```bash
# Old:
printf "Check status with: /jffs/scripts/xray/xray_tproxy.sh status\n"

# New:
printf "Check status with: /jffs/scripts/vpn-director/xray_tproxy.sh status\n"
```

**Step 10: Verify syntax**

```bash
ash -n jffs/scripts/vpn-director/configure.sh && echo "Syntax OK"
```

Expected: `Syntax OK`

**Step 11: Commit**

```bash
git add jffs/scripts/vpn-director/configure.sh
git commit -m "refactor: update all paths in configure.sh"
```

---

### Task 12: Update firewall-start hook

**Files:**
- Modify: `jffs/scripts/firewall-start`

**Step 1: Update tunnel_director.sh path**

Change line 18:

```bash
# Old:
/jffs/scripts/firewall/tunnel_director.sh

# New:
/jffs/scripts/vpn-director/tunnel_director.sh
```

**Step 2: Verify syntax**

```bash
ash -n jffs/scripts/firewall-start && echo "Syntax OK"
```

Expected: `Syntax OK`

**Step 3: Commit**

```bash
git add jffs/scripts/firewall-start
git commit -m "refactor: update path in firewall-start hook"
```

---

### Task 13: Update services-start hook

**Files:**
- Modify: `jffs/scripts/services-start`

**Step 1: Update ipset_builder.sh path (line 13)**

```bash
# Old:
(/jffs/scripts/firewall/ipset_builder.sh -t -x) &

# New:
(/jffs/scripts/vpn-director/ipset_builder.sh -t -x) &
```

**Step 2: Update cron job path (line 17)**

```bash
# Old:
cru a update_ipsets "0 3 * * * /jffs/scripts/firewall/ipset_builder.sh -u -t -x"

# New:
cru a update_ipsets "0 3 * * * /jffs/scripts/vpn-director/ipset_builder.sh -u -t -x"
```

**Step 3: Update send-email.sh path (lines 21-22)**

```bash
# Old:
(sleep 60; /jffs/scripts/utils/send-email.sh "Startup Notification" \

# New:
(sleep 60; /jffs/scripts/vpn-director/utils/send-email.sh "Startup Notification" \
```

**Step 4: Verify syntax**

```bash
ash -n jffs/scripts/services-start && echo "Syntax OK"
```

Expected: `Syntax OK`

**Step 5: Commit**

```bash
git add jffs/scripts/services-start
git commit -m "refactor: update paths in services-start hook"
```

---

### Task 14: Update profile.add

**Files:**
- Modify: `jffs/configs/profile.add`

**Step 1: Update ipt alias**

Change the entire file:

```bash
# Old:
alias ipt="/jffs/scripts/firewall/ipset_builder.sh -t"

# New:
alias ipt="/jffs/scripts/vpn-director/ipset_builder.sh -t"
```

**Step 2: Commit**

```bash
git add jffs/configs/profile.add
git commit -m "refactor: update path in profile.add alias"
```

---

### Task 15: Update install.sh

**Files:**
- Modify: `install.sh`

**Step 1: Update create_directories function (lines 70-79)**

```bash
# Old:
create_directories() {
    print_info "Creating directories..."

    mkdir -p "$JFFS_DIR/firewall"
    mkdir -p "$JFFS_DIR/xray"
    mkdir -p "$JFFS_DIR/utils"
    mkdir -p "$XRAY_CONFIG_DIR"
    mkdir -p "/jffs/configs"

    print_success "Directories created"
}

# New:
create_directories() {
    print_info "Creating directories..."

    mkdir -p "$JFFS_DIR/vpn-director/configs"
    mkdir -p "$JFFS_DIR/vpn-director/utils"
    mkdir -p "$XRAY_CONFIG_DIR"
    mkdir -p "/jffs/configs"

    print_success "Directories created"
}
```

**Step 2: Update download_scripts file list (lines 89-101)**

```bash
# Old:
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

# New:
for script in \
    "jffs/scripts/vpn-director/ipset_builder.sh" \
    "jffs/scripts/vpn-director/tunnel_director.sh" \
    "jffs/scripts/vpn-director/xray_tproxy.sh" \
    "jffs/scripts/vpn-director/configure.sh" \
    "jffs/scripts/vpn-director/configs/config-tunnel-director.sh.template" \
    "jffs/scripts/vpn-director/configs/config-xray.sh.template" \
    "jffs/scripts/vpn-director/utils/common.sh" \
    "jffs/scripts/vpn-director/utils/firewall.sh" \
    "jffs/scripts/vpn-director/utils/shared.sh" \
    "jffs/scripts/vpn-director/utils/send-email.sh" \
    "jffs/scripts/firewall-start" \
    "jffs/scripts/services-start" \
    "jffs/configs/profile.add"
```

**Step 3: Update print_next_steps messages (lines 124-133)**

```bash
# Old:
print_next_steps() {
    print_header "Installation Complete"

    printf "Next step: Run the configuration wizard:\n\n"
    printf "  ${GREEN}/jffs/scripts/utils/configure.sh${NC}\n\n"
    printf "Or edit configs manually:\n"
    printf "  /jffs/scripts/xray/config.sh\n"
    printf "  /jffs/scripts/firewall/config.sh\n"
    printf "  /opt/etc/xray/config.json\n"
}

# New:
print_next_steps() {
    print_header "Installation Complete"

    printf "Next step: Run the configuration wizard:\n\n"
    printf "  ${GREEN}/jffs/scripts/vpn-director/configure.sh${NC}\n\n"
    printf "Or edit configs manually:\n"
    printf "  /jffs/scripts/vpn-director/configs/config-xray.sh\n"
    printf "  /jffs/scripts/vpn-director/configs/config-tunnel-director.sh\n"
    printf "  /opt/etc/xray/config.json\n"
}
```

**Step 4: Verify syntax**

```bash
sh -n install.sh && echo "Syntax OK"
```

Expected: `Syntax OK`

**Step 5: Commit**

```bash
git add install.sh
git commit -m "refactor: update install.sh for new directory structure"
```

---

### Task 16: Update CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

**Step 1: Replace entire file content**

```markdown
# VPN Director for Asuswrt-Merlin

Traffic routing system for Asus routers: Xray TPROXY, Tunnel Director, IPSet Builder.

## Commands

```bash
# Install
curl -fsSL https://raw.githubusercontent.com/zinin/asuswrt-merlin-vpn-director/master/install.sh | sh

# Xray TPROXY
/jffs/scripts/vpn-director/xray_tproxy.sh status|start|stop|restart

# IPSet Builder
/jffs/scripts/vpn-director/ipset_builder.sh       # Restore from cache
/jffs/scripts/vpn-director/ipset_builder.sh -u    # Force rebuild
/jffs/scripts/vpn-director/ipset_builder.sh -t    # Rebuild + Tunnel Director
/jffs/scripts/vpn-director/ipset_builder.sh -x    # Rebuild + Xray TPROXY

# Tunnel Director
/jffs/scripts/vpn-director/tunnel_director.sh

# Shell alias
ipt  # Runs: ipset_builder.sh -t
```

## Architecture

| Path | Purpose |
|------|---------|
| `jffs/scripts/vpn-director/` | Main scripts: ipset_builder, tunnel_director, xray_tproxy, configure |
| `jffs/scripts/vpn-director/utils/` | Shared utilities: common.sh, firewall.sh, shared.sh, send-email.sh |
| `jffs/scripts/vpn-director/configs/` | Config templates |
| `jffs/configs/` | profile.add (shell alias) |
| `config/` | Xray server config template |
| `install.sh` | Interactive installer |

## Key Concepts

**Tunnel Director Rules**: `table:src[%iface][:src_excl]:set[,set...][:set_excl]`
- Example: `wgc1:192.168.50.0/24::us,ca` — route US/CA traffic via wgc1

**IPSet Types**:
- Country sets: 2-letter ISO codes (us, ca, ru) from IPdeny
- Combo sets: union of multiple sets

## Config Files (after install)

```
/jffs/scripts/vpn-director/configs/config-tunnel-director.sh  # Tunnel Director & IPSet
/jffs/scripts/vpn-director/configs/config-xray.sh             # Xray TPROXY
/opt/etc/xray/config.json                                      # Xray server
```

## Shell Conventions

- Shebang: `#!/usr/bin/env ash` with `set -euo pipefail`
- Logging: `log -l err|warn|info|notice "message"`

## Modular Docs

See `.claude/rules/` for detailed docs:
- `tunnel-director.md` — rule format, chain architecture, fwmark layout
- `ipset-builder.md` — IPdeny sources, dump/restore, combo sets
- `xray-tproxy.md` — TPROXY chain, exclusions, fail-safe
- `shell-conventions.md` — utilities from common.sh/firewall.sh
```

**Step 2: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: update CLAUDE.md for new directory structure"
```

---

### Task 17: Update .claude/rules/tunnel-director.md

**Files:**
- Modify: `.claude/rules/tunnel-director.md`

**Step 1: Update all path references**

Search and replace:
- `/jffs/scripts/firewall/tunnel_director.sh` → `/jffs/scripts/vpn-director/tunnel_director.sh`
- `firewall/config.sh` → `configs/config-tunnel-director.sh`
- `/tmp/tunnel_director/` stays as is (runtime path)
- `fw_shared.sh` → `shared.sh`
- `common.sh` and `firewall.sh` locations to `vpn-director/utils/`

**Step 2: Commit**

```bash
git add .claude/rules/tunnel-director.md
git commit -m "docs: update tunnel-director.md paths"
```

---

### Task 18: Update .claude/rules/ipset-builder.md

**Files:**
- Modify: `.claude/rules/ipset-builder.md`

**Step 1: Update all path references**

Search and replace:
- `/jffs/scripts/firewall/ipset_builder.sh` → `/jffs/scripts/vpn-director/ipset_builder.sh`
- `fw_shared.sh` → `shared.sh`
- `/tmp/ipset_builder/` stays as is (runtime path)

**Step 2: Commit**

```bash
git add .claude/rules/ipset-builder.md
git commit -m "docs: update ipset-builder.md paths"
```

---

### Task 19: Update .claude/rules/xray-tproxy.md

**Files:**
- Modify: `.claude/rules/xray-tproxy.md`

**Step 1: Update all path references**

Search and replace:
- `/jffs/scripts/xray/xray_tproxy.sh` → `/jffs/scripts/vpn-director/xray_tproxy.sh`
- `xray/config.sh` → `configs/config-xray.sh`
- `ipset_builder.sh` location

**Step 2: Commit**

```bash
git add .claude/rules/xray-tproxy.md
git commit -m "docs: update xray-tproxy.md paths"
```

---

### Task 20: Update .claude/rules/shell-conventions.md

**Files:**
- Modify: `.claude/rules/shell-conventions.md`

**Step 1: Update utility locations if mentioned**

Check for any hardcoded paths and update to `vpn-director/utils/`.

**Step 2: Commit (if changes made)**

```bash
git add .claude/rules/shell-conventions.md
git commit -m "docs: update shell-conventions.md paths"
```

---

### Task 21: Remove old empty directories

**Files:**
- Delete: `jffs/scripts/firewall/` (should be empty)
- Delete: `jffs/scripts/xray/` (should be empty)
- Delete: `jffs/scripts/utils/` (should be empty)

**Step 1: Verify directories are empty**

```bash
ls -la jffs/scripts/firewall/
ls -la jffs/scripts/xray/
ls -la jffs/scripts/utils/
```

Expected: All directories should be empty or not exist

**Step 2: Remove directories**

```bash
rmdir jffs/scripts/firewall 2>/dev/null || true
rmdir jffs/scripts/xray 2>/dev/null || true
rmdir jffs/scripts/utils 2>/dev/null || true
```

**Step 3: Verify removal**

```bash
ls jffs/scripts/
```

Expected: Only `firewall-start`, `services-start`, `vpn-director/`

**Step 4: Commit**

```bash
git add -A
git commit -m "chore: remove old empty directories"
```

---

### Task 22: Final verification

**Step 1: Verify all scripts have correct syntax**

```bash
for f in jffs/scripts/vpn-director/*.sh jffs/scripts/vpn-director/utils/*.sh; do
    echo "Checking $f..."
    ash -n "$f" || echo "FAILED: $f"
done
```

Expected: All scripts pass syntax check

**Step 2: Verify directory structure**

```bash
find jffs/scripts/vpn-director -type f -name "*.sh*" | sort
```

Expected output:
```
jffs/scripts/vpn-director/configs/config-tunnel-director.sh.template
jffs/scripts/vpn-director/configs/config-xray.sh.template
jffs/scripts/vpn-director/configure.sh
jffs/scripts/vpn-director/ipset_builder.sh
jffs/scripts/vpn-director/tunnel_director.sh
jffs/scripts/vpn-director/utils/common.sh
jffs/scripts/vpn-director/utils/firewall.sh
jffs/scripts/vpn-director/utils/send-email.sh
jffs/scripts/vpn-director/utils/shared.sh
jffs/scripts/vpn-director/xray_tproxy.sh
```

**Step 3: Verify git status is clean**

```bash
git status
```

Expected: `nothing to commit, working tree clean`

---

### Task 23: Create summary commit (optional squash)

**Step 1: View commit history**

```bash
git log --oneline -20
```

**Step 2: Decide on squash**

If desired, squash all refactor commits into one:

```bash
git rebase -i HEAD~22
```

Mark all but first as `squash`, then edit message to:

```
refactor: restructure directories to vpn-director/

- Move all scripts to /jffs/scripts/vpn-director/
- Rename config templates: config-tunnel-director.sh, config-xray.sh
- Rename fw_shared.sh to shared.sh
- Update all source paths and references
- Update install.sh for new structure
- Update documentation (CLAUDE.md, .claude/rules/)
- Add send-email.sh to install.sh
```

---

## Summary

| Task | Description | Files |
|------|-------------|-------|
| 1 | Create directory structure | vpn-director/, configs/, utils/ |
| 2 | Move utility files | common.sh, firewall.sh, send-email.sh, shared.sh |
| 3 | Move config templates | config-tunnel-director.sh.template, config-xray.sh.template |
| 4 | Move executable scripts | ipset_builder.sh, tunnel_director.sh, xray_tproxy.sh, configure.sh |
| 5-6 | Update common.sh, shared.sh | Header comments |
| 7 | Update send-email.sh | Source path |
| 8 | Update ipset_builder.sh | All paths |
| 9 | Update tunnel_director.sh | All paths |
| 10 | Update xray_tproxy.sh | All paths |
| 11 | Update configure.sh | All paths |
| 12 | Update firewall-start | Script path |
| 13 | Update services-start | All paths |
| 14 | Update profile.add | Alias path |
| 15 | Update install.sh | Directories and file list |
| 16-20 | Update documentation | CLAUDE.md, .claude/rules/*.md |
| 21 | Remove old directories | firewall/, xray/, utils/ |
| 22 | Final verification | Syntax checks |
| 23 | Summary commit | Optional squash |
