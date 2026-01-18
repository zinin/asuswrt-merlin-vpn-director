# Telegram Bot Daemon Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Run telegram-bot as proper Entware daemon with monit auto-restart.

**Architecture:** Separate S98telegram-bot init script using rc.func for standard Entware integration. Monit monitors process and restarts on crash. Remove manual bot management from S99vpn-director and setup_telegram_bot.sh.

**Tech Stack:** Shell (rc.func), monit

---

### Task 1: Create S98telegram-bot init script

**Files:**
- Create: `opt/etc/init.d/S98telegram-bot`

**Step 1: Create the init script**

```sh
#!/bin/sh

ENABLED=yes
PROCS=telegram-bot
ARGS=""
PREARGS=""
DESC="Telegram Bot for VPN Director"
PATH=/opt/sbin:/opt/bin:/usr/sbin:/usr/bin:/sbin:/bin

. /opt/etc/init.d/rc.func
```

**Step 2: Commit**

```bash
git add opt/etc/init.d/S98telegram-bot
git commit -m "feat(telegram-bot): add Entware init script S98telegram-bot"
```

---

### Task 2: Remove telegram-bot management from S99vpn-director

**Files:**
- Modify: `opt/etc/init.d/S99vpn-director`

**Step 1: Remove bot start from start()**

Remove these lines from `start()`:

```sh
    # Start telegram bot (if configured)
    if [ -x "$SCRIPT_DIR/telegram-bot" ] && [ -f "$SCRIPT_DIR/telegram-bot.json" ]; then
        "$SCRIPT_DIR/telegram-bot" >> /tmp/telegram-bot.log 2>&1 &
    fi
```

**Step 2: Remove bot stop from stop()**

Remove this line from `stop()`:

```sh
    killall telegram-bot 2>/dev/null || true
```

**Step 3: Commit**

```bash
git add opt/etc/init.d/S99vpn-director
git commit -m "refactor(init): remove telegram-bot from S99vpn-director

Bot now managed by dedicated S98telegram-bot init script."
```

---

### Task 3: Update setup_telegram_bot.sh to use init script

**Files:**
- Modify: `jffs/scripts/vpn-director/setup_telegram_bot.sh`

**Step 1: Replace manual restart with init script**

Replace lines 60-69:

```sh
# Restart bot if running
if pgrep -x telegram-bot > /dev/null; then
    killall telegram-bot 2>/dev/null || true
    sleep 1
fi

if [[ -x "$JFFS_DIR/telegram-bot" ]]; then
    "$JFFS_DIR/telegram-bot" >> /tmp/telegram-bot.log 2>&1 &
    echo "Bot restarted"
fi
```

With:

```sh
# Restart bot via init script
if [[ -x /opt/etc/init.d/S98telegram-bot ]]; then
    /opt/etc/init.d/S98telegram-bot restart
fi
```

**Step 2: Commit**

```bash
git add jffs/scripts/vpn-director/setup_telegram_bot.sh
git commit -m "refactor(setup): use init script for telegram-bot restart"
```

---

### Task 4: Add S98telegram-bot creation to install.sh

**Files:**
- Modify: `install.sh`

**Step 1: Find insertion point**

After the block that creates S99vpn-director (around line 129, after `print_success "Created /opt/etc/init.d/S99vpn-director"`).

**Step 2: Add S98telegram-bot creation**

Insert after S99vpn-director creation:

```sh
# Create Telegram bot init.d script
log "Creating Entware init.d script for Telegram bot..."
cat > /opt/etc/init.d/S98telegram-bot << 'INITEOF'
#!/bin/sh

ENABLED=yes
PROCS=telegram-bot
ARGS=""
PREARGS=""
DESC="Telegram Bot for VPN Director"
PATH=/opt/sbin:/opt/bin:/usr/sbin:/usr/bin:/sbin:/bin

. /opt/etc/init.d/rc.func
INITEOF

chmod +x /opt/etc/init.d/S98telegram-bot
print_success "Created /opt/etc/init.d/S98telegram-bot"
```

**Step 3: Commit**

```bash
git add install.sh
git commit -m "feat(install): add S98telegram-bot init script creation"
```

---

### Task 5: Update README.md Process Monitoring section

**Files:**
- Modify: `README.md`

**Step 1: Update section header**

Change line 140:

```diff
-Xray may occasionally crash. Use monit for automatic restart.
+Xray and Telegram bot may occasionally crash. Use monit for automatic restart.
```

**Step 2: Update config creation instruction**

Change line 149:

```diff
-2. Create config `/opt/etc/monit.d/xray`:
+2. Create configs in `/opt/etc/monit.d/`:
+
+   **xray:**
```

**Step 3: Add telegram-bot config after xray config block**

After line 155 (after xray config closing ```), add:

```markdown

   **telegram-bot:**
   ```
   check process telegram-bot matching "telegram-bot"
       start program = "/opt/etc/init.d/S98telegram-bot start"
       stop program = "/opt/etc/init.d/S98telegram-bot stop"
       if does not exist then restart
   ```
```

**Step 4: Commit**

```bash
git add README.md
git commit -m "docs: add telegram-bot to Process Monitoring section"
```

---

### Task 6: Final verification

**Step 1: Check all files modified correctly**

```bash
git diff HEAD~5 --stat
```

Expected: 5 files changed (S98telegram-bot, S99vpn-director, setup_telegram_bot.sh, install.sh, README.md)

**Step 2: Verify S98telegram-bot syntax**

```bash
sh -n opt/etc/init.d/S98telegram-bot && echo "Syntax OK"
```

Expected: `Syntax OK`

**Step 3: Verify setup_telegram_bot.sh syntax**

```bash
bash -n jffs/scripts/vpn-director/setup_telegram_bot.sh && echo "Syntax OK"
```

Expected: `Syntax OK`
