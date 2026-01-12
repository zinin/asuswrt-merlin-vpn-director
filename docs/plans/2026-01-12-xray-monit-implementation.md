# Xray Monit Monitoring Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add documentation to README.md explaining how to set up monit for automatic xray restart on crash.

**Architecture:** Two additions to README.md: (1) add monit to Optional packages, (2) add new "Process Monitoring" section with setup instructions.

**Tech Stack:** Markdown documentation only.

---

## Task 1: Add monit to Optional packages

**Files:**
- Modify: `README.md:45-47`

**Step 1: Add monit package to Optional section**

In `README.md`, find the Optional section (line 45-47):

```markdown
### Optional

- `opkg install openssl-util` — for email notifications
```

Replace with:

```markdown
### Optional

- `opkg install openssl-util` — for email notifications
- `opkg install monit` — for automatic Xray restart on crash (see [Process Monitoring](#process-monitoring))
```

**Step 2: Verify markdown renders correctly**

Run: `cat README.md | grep -A3 "### Optional"`

Expected output should show both optional packages.

**Step 3: Commit**

```bash
git add README.md
git commit -m "docs: add monit to optional packages"
```

---

## Task 2: Add Process Monitoring section

**Files:**
- Modify: `README.md` (after line 96, before License section)

**Step 1: Add Process Monitoring section**

In `README.md`, add new section before `## License` (after line 96):

```markdown
## Process Monitoring

Xray may occasionally crash. Use monit for automatic restart.

### Setup

1. Install monit:
   ```bash
   opkg install monit
   ```

2. Create config `/opt/etc/monit.d/xray`:
   ```
   check process xray matching "xray"
       start program = "/opt/etc/init.d/S24xray start"
       stop program = "/opt/etc/init.d/S24xray stop"
       if does not exist then restart
   ```

3. Edit `/opt/etc/monitrc`, set check interval:
   ```
   set daemon 30    # check every 30 seconds
   ```

4. Restart monit:
   ```bash
   /opt/etc/init.d/S99monit restart
   ```

5. Verify:
   ```bash
   monit status
   ```

```

**Step 2: Verify section structure**

Run: `grep "^## " README.md`

Expected output should include `## Process Monitoring` between `## Startup Scripts` and `## License`.

**Step 3: Verify internal link works**

The link `#process-monitoring` from Task 1 should now have a target. Verify:

Run: `grep -c "## Process Monitoring" README.md`

Expected: `1`

**Step 4: Commit**

```bash
git add README.md
git commit -m "docs: add process monitoring section with monit setup"
```

---

## Verification

After both tasks complete:

1. Check README structure:
   ```bash
   grep "^## " README.md
   ```
   Expected sections in order: Features, Quick Install, Requirements, Manual Configuration, Commands, How It Works, Startup Scripts, Process Monitoring, License

2. Check anchor link target exists:
   ```bash
   grep "process-monitoring" README.md
   ```
   Should show both the link (in Optional) and the heading.
