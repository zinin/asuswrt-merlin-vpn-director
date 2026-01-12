# Design: Xray Process Monitoring with Monit

## Problem

Xray process occasionally crashes and does not restart automatically. The Entware init system (rc.func) lacks automatic restart capability. Running `xray_tproxy.sh restart` does not help because it only manages iptables/routing rules, not the xray process itself.

Current workaround: manually run `/opt/etc/init.d/S24xray start` on the router.

## Solution

Add documentation to README.md explaining how to set up monit for automatic xray restart.

## Design Decisions

- **Approach**: Documentation only (no code changes to install.sh or scripts)
- **Tool**: monit (available via `opkg install monit`)
- **Check interval**: 30 seconds
- **Scope**: Only xray monitoring (can be extended later)

## Changes to README.md

### 1. Add to Optional packages section

```markdown
### Optional

- `opkg install openssl-util` — for email notifications
- `opkg install monit` — for automatic Xray restart on crash (see [Process Monitoring](#process-monitoring))
```

### 2. Add new section "Process Monitoring" after "Startup Scripts"

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

## Implementation

Single file change: `README.md`
