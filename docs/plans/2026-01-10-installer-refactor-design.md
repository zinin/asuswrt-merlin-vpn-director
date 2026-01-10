# Refactor Installer: Split into Install and Configure Scripts

## Overview

Split the monolithic `install.sh` (~600 lines) into two focused scripts:

- **install.sh** — Downloads and installs scripts to /jffs/ (for `curl | sh` usage)
- **configure.sh** — Interactive configuration wizard (runs locally after install)

## Goals

1. **Repeatable configuration** — Re-run configure.sh to change settings without reinstalling
2. **Clean separation** — Installation vs configuration are distinct concerns
3. **Script updates** — Update scripts without losing configuration

## New Architecture

```
install.sh                          # Remote: curl | sh
jffs/scripts/utils/configure.sh     # Local: interactive config
```

### User Flow

1. User runs `curl -fsSL .../install.sh | sh`
2. install.sh downloads all scripts, creates directories
3. install.sh prints: "Run /jffs/scripts/utils/configure.sh"
4. User runs configure.sh — interactive setup
5. For reconfiguration — run configure.sh again

## install.sh — New Structure

**Purpose:** Download and install files only.

**Steps:**

```
check_environment()         # Verify /jffs, curl, base64, sha256sum, xray
create_directories()        # mkdir -p for all paths
download_scripts()          # Download .sh files from repo
install_config_templates()  # Copy templates (only if configs don't exist)
print_next_steps()          # Show instructions
```

**Key behavior:**
- Does NOT overwrite existing config.sh files
- Does NOT run interactive prompts
- Approx 150 lines

**Output:**

```
Installation complete!

Next step: Run configuration wizard:
  /jffs/scripts/utils/configure.sh

Or edit configs manually:
  /jffs/scripts/xray/config.sh
  /jffs/scripts/firewall/config.sh
```

## configure.sh — New Script

**Location:** `/jffs/scripts/utils/configure.sh`

**Purpose:** Full interactive configuration. Each run overwrites configs completely.

**Steps (moved from current install.sh):**

```
step_get_vless_file()            # Input URL/path to VLESS file
step_parse_vless_servers()       # Parse and resolve servers
step_select_xray_server()        # Select server from list
step_configure_xray_exclusions() # Select countries to exclude
step_configure_clients()         # Add clients (Xray/Tunnel Director)
step_show_summary()              # Show summary, confirm
step_generate_configs()          # Generate config.sh and xray/config.json
step_apply_rules()               # Apply rules (ipset, tproxy, tunnel)
```

**Helpers (moved as-is):**
- print_header(), print_success(), print_error(), print_warning(), print_info()
- read_input(), confirm()
- parse_vless_uri()

**Minimal checks:**
- Verify config templates exist (install.sh must have run)

**Approx 450 lines**

## README.md Updates

### New Section: Startup Scripts

Add after "How It Works":

```markdown
## Startup Scripts

This project uses [Asuswrt-Merlin user scripts](https://github.com/RMerl/asuswrt-merlin.ng/wiki/User-scripts)
for automatic startup:

| Script | When Called | Purpose |
|--------|-------------|---------|
| `services-start` | After all services started at boot | Builds ipsets, starts Xray TPROXY |
| `firewall-start` | After firewall rules applied | Applies Tunnel Director rules |

**Note:** Installation overwrites these files. If you have custom logic,
back up your scripts before installing.

To enable user scripts: Administration → System → Enable JFFS custom scripts and configs → Yes
```

### Update Quick Install Section

```markdown
## Quick Install

```bash
curl -fsSL https://raw.githubusercontent.com/zinin/asuswrt-merlin-vpn-director/master/install.sh | sh
```

After installation, run the configuration wizard:

```bash
/jffs/scripts/utils/configure.sh
```
```

## Files Changed

| File | Action |
|------|--------|
| `install.sh` | Rewrite — installation only |
| `jffs/scripts/utils/configure.sh` | Create — interactive configuration |
| `README.md` | Update — Startup Scripts section, configure.sh instructions |

## Implementation Order

1. Create configure.sh (extract configuration logic)
2. Rewrite install.sh (keep only installation)
3. Update README.md

## Decisions Made

- **Configuration mode:** Full reconfiguration each run (no incremental updates)
- **Script location:** /jffs/scripts/utils/configure.sh
- **Startup scripts:** Overwrite services-start/firewall-start (user backs up if needed)
- **install.sh + configure.sh:** install.sh prints instructions, does not auto-run configure.sh
- **Existing configs:** install.sh does not overwrite existing config.sh files
