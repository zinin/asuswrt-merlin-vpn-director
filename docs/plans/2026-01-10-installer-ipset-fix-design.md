# Fix: Installer should create ipsets required by Xray TPROXY

## Problem

When user installs VPN Director with only Xray clients (no Tunnel Director rules):
1. `xray/config.sh` has `XRAY_EXCLUDE_SETS='ru'` by default
2. `ipset_builder.sh` only parses `TUN_DIR_RULES` to determine which ipsets to create
3. ipset `ru` is never created
4. `xray_tproxy.sh` fails with "Required ipset 'ru' not found"

## Solution: Approach B - Fix the Installer

### Changes to install.sh

1. **New variable** at the top:
   ```sh
   XRAY_EXCLUDE_SETS_LIST="ru"  # default value
   ```

2. **New step** `step_configure_xray_exclusions()` after server selection:
   - Ask user which countries to exclude from Xray proxying
   - Default to "ru", accept comma-separated list
   - Normalize input (lowercase, no spaces)

3. **Update template generation** in `step_install_files()`:
   - Replace `{{XRAY_EXCLUDE_SETS}}` placeholder with user's choice

4. **Update `step_apply_rules()`**:
   - Call `ipset_builder.sh -c "$XRAY_EXCLUDE_SETS_LIST"` to create required ipsets

5. **Update `main()`** step order:
   - Step 1: Get VLESS file
   - Step 2: Parse servers
   - Step 3: Select server
   - Step 4: Configure Xray exclusions (NEW)
   - Step 5: Configure clients
   - Step 6: Show summary
   - Step 7: Install files
   - Step 8: Apply rules

6. **Update `step_show_summary()`**:
   - Display selected exclusion countries

### Changes to ipset_builder.sh

1. **New parameter `-c <codes>`**:
   ```sh
   extra_countries=""

   while [ $# -gt 0 ]; do
       case "$1" in
           -c)  extra_countries="$2"; shift 2 ;;
           # ... existing cases
       esac
   done
   ```

2. **Update `build_country_ipsets()`**:
   - Parse extra countries from `-c` parameter
   - Merge with countries from `TUN_DIR_RULES`
   - Build all required ipsets

### Changes to config.sh.template (xray)

Line 60:
```sh
# Before:
XRAY_EXCLUDE_SETS='ru'

# After:
XRAY_EXCLUDE_SETS='{{XRAY_EXCLUDE_SETS}}'
```

## Files to Modify

| File | Changes |
|------|---------|
| `install.sh` | Add variable, new step, update generation, update apply order |
| `jffs/scripts/firewall/ipset_builder.sh` | Add `-c` parameter parsing, merge extra countries |
| `jffs/scripts/xray/config.sh.template` | Replace hardcoded 'ru' with placeholder |

## Backwards Compatibility

- `ipset_builder.sh` without `-c` works exactly as before
- Existing installations unaffected (config.sh already has concrete value)
