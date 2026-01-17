# Entware Init.d Migration — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Move vpn-director startup from services-start to Entware init.d to ensure Entware bash is available at boot.

**Architecture:** Create `/opt/etc/init.d/S99vpn-director` as main startup entry point. Modify `firewall-start` to skip early boot when Entware is unavailable. Remove `services-start` from vpn-director distribution.

**Tech Stack:** Shell scripts (sh for init.d, bash for vpn-director scripts), Asuswrt-Merlin hooks, Entware init.d system.

---

### Task 1: Create S99vpn-director init script

**Files:**
- Create: `opt/etc/init.d/S99vpn-director`

**Step 1: Create init.d script**

```sh
#!/bin/sh

PATH=/opt/sbin:/opt/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
SCRIPT_DIR="/jffs/scripts/vpn-director"

start() {
    # Build ipsets, then start tunnel_director + xray_tproxy
    "$SCRIPT_DIR/ipset_builder.sh" -t -x

    # Cron job: update ipsets daily at 03:00
    cru a update_ipsets "0 3 * * * $SCRIPT_DIR/ipset_builder.sh -u -t -x"

    # Startup notification
    "$SCRIPT_DIR/utils/send-email.sh" "Startup Notification" \
        "I've just started up and got connected to the internet."
}

stop() {
    "$SCRIPT_DIR/xray_tproxy.sh" stop
    cru d update_ipsets
}

case "$1" in
    start)   start ;;
    stop)    stop ;;
    restart) stop; start ;;
    *)       echo "Usage: $0 {start|stop|restart}" ;;
esac
```

**Step 2: Verify file structure**

Run: `ls -la opt/etc/init.d/`
Expected: S99vpn-director exists

**Step 3: Commit**

```bash
git add opt/etc/init.d/S99vpn-director
git commit -m "feat: add S99vpn-director init script for Entware"
```

---

### Task 2: Update firewall-start with Entware check

**Files:**
- Modify: `jffs/scripts/firewall-start`

**Step 1: Update firewall-start**

Replace entire file with:

```sh
#!/bin/sh

###################################################################################################
# firewall-start - Asuswrt-Merlin hook invoked when the firewall stack reloads
###################################################################################################

# Skip if Entware not ready (during early boot)
[ -x /opt/bin/bash ] || exit 0

# Apply Tunnel Director rules
/jffs/scripts/vpn-director/tunnel_director.sh
```

**Step 2: Verify changes**

Run: `head -10 jffs/scripts/firewall-start`
Expected: Shows Entware check on line 7

**Step 3: Commit**

```bash
git add jffs/scripts/firewall-start
git commit -m "fix: skip firewall-start during early boot before Entware"
```

---

### Task 3: Remove services-start from distribution

**Files:**
- Delete: `jffs/scripts/services-start`

**Step 1: Delete services-start**

Run: `rm jffs/scripts/services-start`

**Step 2: Verify deletion**

Run: `ls jffs/scripts/services-start 2>&1`
Expected: "No such file or directory"

**Step 3: Commit**

```bash
git add -A jffs/scripts/services-start
git commit -m "refactor: remove services-start, replaced by S99vpn-director"
```

---

### Task 4: Update install.sh

**Files:**
- Modify: `install.sh:73-121`

**Step 1: Add create_initd_script function after create_directories**

Add after line 81 (after `create_directories` function):

```bash
###############################################################################
# Create Entware init.d script
###############################################################################

create_initd_script() {
    print_info "Creating Entware init script..."

    cat > /opt/etc/init.d/S99vpn-director << 'INITEOF'
#!/bin/sh

PATH=/opt/sbin:/opt/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
SCRIPT_DIR="/jffs/scripts/vpn-director"

start() {
    # Build ipsets, then start tunnel_director + xray_tproxy
    "$SCRIPT_DIR/ipset_builder.sh" -t -x

    # Cron job: update ipsets daily at 03:00
    cru a update_ipsets "0 3 * * * $SCRIPT_DIR/ipset_builder.sh -u -t -x"

    # Startup notification
    "$SCRIPT_DIR/utils/send-email.sh" "Startup Notification" \
        "I've just started up and got connected to the internet."
}

stop() {
    "$SCRIPT_DIR/xray_tproxy.sh" stop
    cru d update_ipsets
}

case "$1" in
    start)   start ;;
    stop)    stop ;;
    restart) stop; start ;;
    *)       echo "Usage: $0 {start|stop|restart}" ;;
esac
INITEOF

    chmod +x /opt/etc/init.d/S99vpn-director
    print_success "Created /opt/etc/init.d/S99vpn-director"
}
```

**Step 2: Remove services-start from download list**

In `download_scripts` function, remove this line from the list:
```
        "jffs/scripts/services-start" \
```

**Step 3: Add create_initd_script call in main**

Update `main` function to call `create_initd_script` after `download_scripts`:

```bash
main() {
    print_header "VPN Director Installer"
    printf "This will install VPN Director scripts to your router.\n\n"

    check_environment
    create_directories
    download_scripts
    create_initd_script
    print_next_steps
}
```

**Step 4: Verify install.sh syntax**

Run: `bash -n install.sh`
Expected: No output (no syntax errors)

**Step 5: Commit**

```bash
git add install.sh
git commit -m "feat: install.sh creates S99vpn-director, removes services-start"
```

---

### Task 5: Update README.md

**Files:**
- Modify: `README.md:84-96`

**Step 1: Update Startup Scripts section**

Replace the "Startup Scripts" section (lines 84-96) with:

```markdown
## Startup Scripts

This project uses Entware init.d for automatic startup:

| Script | When Called | Purpose |
|--------|-------------|---------|
| `/opt/etc/init.d/S99vpn-director` | After Entware initialized | Builds ipsets, starts Xray TPROXY, sets up cron |
| `/jffs/scripts/firewall-start` | After firewall rules applied | Applies Tunnel Director rules (runtime reload) |

**Note:** The init.d script ensures Entware bash is available before running vpn-director scripts.

To enable user scripts: Administration -> System -> Enable JFFS custom scripts and configs -> Yes
```

**Step 2: Remove TODO notes at end of file**

Delete lines 102-104 (the TODO comments).

**Step 3: Commit**

```bash
git add README.md
git commit -m "docs: update README for Entware init.d startup"
```

---

### Task 6: Update CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

**Step 1: Check if Architecture table needs update**

Read CLAUDE.md and update the Architecture table if it references services-start.

**Step 2: Commit if changed**

```bash
git add CLAUDE.md
git commit -m "docs: update CLAUDE.md for init.d architecture"
```

---

### Task 7: Final verification

**Step 1: Run existing tests**

Run: `cd /opt/github/zinin/asuswrt-merlin-vpn-director && bats test/`
Expected: All tests pass

**Step 2: Check all files committed**

Run: `git status`
Expected: "nothing to commit, working tree clean" (except README.md if it had prior changes)

**Step 3: Review commit history**

Run: `git log --oneline -10`
Expected: Shows all new commits for this feature
