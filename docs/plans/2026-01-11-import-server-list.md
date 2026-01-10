# Import Server List Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Разделить configure.sh — вынести импорт VLESS серверов в отдельный скрипт import_server_list.sh.

**Architecture:** Новый скрипт import_server_list.sh загружает серверы и сохраняет в data/servers.json. Configure.sh читает этот файл для выбора сервера. Параметр ipset_dump_dir переименовывается в data_dir с новым путём по умолчанию.

**Tech Stack:** Shell (ash), jq

---

## Task 1: Update vpn-director.json.template

**Files:**
- Modify: `jffs/scripts/vpn-director/vpn-director.json.template`

**Step 1: Rename parameter and update path**

Change `ipset_dump_dir` to `data_dir` and update default path:

```json
{
  "tunnel_director": {
    "rules": [],
    "data_dir": "/jffs/scripts/vpn-director/data"
  },
```

**Step 2: Commit**

```bash
git add jffs/scripts/vpn-director/vpn-director.json.template
git commit -m "refactor: rename ipset_dump_dir to data_dir in template"
```

---

## Task 2: Update utils/config.sh

**Files:**
- Modify: `jffs/scripts/vpn-director/utils/config.sh:39`

**Step 1: Update JSON path for IPS_BDR_DIR**

Line 39, change from:
```sh
IPS_BDR_DIR=$(_cfg '.tunnel_director.ipset_dump_dir')
```

To:
```sh
IPS_BDR_DIR=$(_cfg '.tunnel_director.data_dir')
```

**Step 2: Commit**

```bash
git add jffs/scripts/vpn-director/utils/config.sh
git commit -m "refactor: read data_dir instead of ipset_dump_dir"
```

---

## Task 3: Update ipset_builder.sh paths

**Files:**
- Modify: `jffs/scripts/vpn-director/ipset_builder.sh:72-73`

**Step 1: Simplify dump directory structure**

Lines 72-73, change from:
```sh
DUMP_DIR="$IPS_BDR_DIR/dumps"
COUNTRY_DUMP_DIR="$DUMP_DIR/countries"
```

To:
```sh
COUNTRY_DUMP_DIR="$IPS_BDR_DIR/ipsets"
```

Remove the intermediate `DUMP_DIR` variable — it's no longer needed.

**Step 2: Commit**

```bash
git add jffs/scripts/vpn-director/ipset_builder.sh
git commit -m "refactor: move ipset dumps to data_dir/ipsets/"
```

---

## Task 4: Create import_server_list.sh

**Files:**
- Create: `jffs/scripts/vpn-director/import_server_list.sh`

**Step 1: Create the script with header and helpers**

```sh
#!/bin/sh
set -e

###############################################################################
# import_server_list.sh - Import VLESS servers from base64-encoded file/URL
# Run after install.sh to download and parse server list
###############################################################################

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Paths
JFFS_DIR="/jffs/scripts/vpn-director"
VPD_CONFIG="$JFFS_DIR/vpn-director.json"
VPD_TEMPLATE="$JFFS_DIR/vpn-director.json.template"

###############################################################################
# Helper functions
###############################################################################

print_header() {
    printf "\n${BLUE}=== %s ===${NC}\n\n" "$1"
}

print_success() {
    printf "${GREEN}[OK]${NC} %s\n" "$1"
}

print_error() {
    printf "${RED}[ERROR]${NC} %s\n" "$1"
}

print_warning() {
    printf "${YELLOW}[WARN]${NC} %s\n" "$1"
}

print_info() {
    printf "${BLUE}[INFO]${NC} %s\n" "$1"
}

read_input() {
    printf "%s: " "$1"
    read -r INPUT_RESULT
}
```

**Step 2: Add VLESS URI parser function**

```sh
###############################################################################
# VLESS URI Parser
###############################################################################

# Parse a single VLESS URI and extract components
# Format: vless://uuid@server:port?params#name
# Output: JSON object
parse_vless_uri() {
    uri="$1"

    # Remove vless:// prefix
    rest="${uri#vless://}"

    # Extract name (after #, URL-decoded)
    name="${rest##*#}"
    name=$(printf '%s' "$name" | sed 's/%20/ /g; s/%2F/\//g; s/+/ /g')
    # Remove emoji and other 3-4 byte UTF-8 chars
    name=$(printf '%s' "$name" | awk '{
        gsub(/[\360-\364][\200-\277][\200-\277][\200-\277]/, "")
        gsub(/[\340-\357][\200-\277][\200-\277][\200-\277]/, "")
        print
    }' | sed 's/^[[:space:]]*//')
    rest="${rest%%#*}"

    # Extract UUID (before @)
    uuid="${rest%%@*}"
    rest="${rest#*@}"

    # Extract server:port (before ?)
    server_port="${rest%%\?*}"
    server="${server_port%%:*}"
    port="${server_port##*:}"

    printf '%s|%s|%s|%s\n' "$server" "$port" "$uuid" "$name"
}
```

**Step 3: Add get_data_dir function**

```sh
###############################################################################
# Get data directory from config
###############################################################################

get_data_dir() {
    local config_file="$VPD_CONFIG"

    # Fall back to template if config doesn't exist
    if [ ! -f "$config_file" ]; then
        config_file="$VPD_TEMPLATE"
    fi

    if [ ! -f "$config_file" ]; then
        print_error "Config not found: $VPD_CONFIG or $VPD_TEMPLATE"
        exit 1
    fi

    jq -r '.tunnel_director.data_dir // "/jffs/scripts/vpn-director/data"' "$config_file"
}
```

**Step 4: Add main import functions**

```sh
###############################################################################
# Step 1: Get VLESS file
###############################################################################

step_get_vless_file() {
    print_header "Step 1: VLESS Server List"

    printf "Enter path to VLESS file or URL:\n"
    printf "(File should contain base64-encoded VLESS URIs)\n\n"

    read_input "Path or URL"
    VLESS_INPUT="$INPUT_RESULT"

    if [ -z "$VLESS_INPUT" ]; then
        print_error "No input provided"
        exit 1
    fi

    # Check if it's a URL or file path
    case "$VLESS_INPUT" in
        http://*|https://*)
            print_info "Downloading from URL..."
            VLESS_CONTENT=$(curl -fsSL "$VLESS_INPUT") || {
                print_error "Failed to download from $VLESS_INPUT"
                exit 1
            }
            ;;
        *)
            if [ ! -f "$VLESS_INPUT" ]; then
                print_error "File not found: $VLESS_INPUT"
                exit 1
            fi
            VLESS_CONTENT=$(cat "$VLESS_INPUT")
            ;;
    esac

    # Decode base64
    VLESS_DECODED=$(printf '%s' "$VLESS_CONTENT" | base64 -d 2>/dev/null) || {
        print_error "Failed to decode base64 content"
        exit 1
    }

    # Count servers
    SERVER_COUNT=$(printf '%s\n' "$VLESS_DECODED" | grep -c '^vless://' || true)

    if [ "$SERVER_COUNT" -eq 0 ]; then
        print_error "No VLESS servers found in file"
        exit 1
    fi

    print_success "Found $SERVER_COUNT VLESS servers"
    VLESS_SERVERS="$VLESS_DECODED"
}

###############################################################################
# Step 2: Parse and save servers
###############################################################################

step_parse_and_save_servers() {
    print_header "Step 2: Parsing Servers"

    DATA_DIR=$(get_data_dir)
    SERVERS_FILE="$DATA_DIR/servers.json"

    # Ensure data directory exists
    mkdir -p "$DATA_DIR"

    # Parse servers and build JSON
    servers_json="["
    first=1
    resolved=0
    failed=0

    printf '%s\n' "$VLESS_SERVERS" | grep '^vless://' | while IFS= read -r uri; do
        parsed=$(parse_vless_uri "$uri")
        server=$(printf '%s' "$parsed" | cut -d'|' -f1)
        port=$(printf '%s' "$parsed" | cut -d'|' -f2)
        uuid=$(printf '%s' "$parsed" | cut -d'|' -f3)
        name=$(printf '%s' "$parsed" | cut -d'|' -f4)

        # Resolve IP using nslookup
        ip=$(nslookup "$server" 2>/dev/null | awk '/^Address/ && !/^Address:.*#/ { print $2; exit }')

        if [ -z "$ip" ]; then
            print_warning "Cannot resolve $server, skipping"
            continue
        fi

        printf "  %s (%s) -> %s\n" "$name" "$server" "$ip"

        # Output JSON line
        printf '%s|%s|%s|%s|%s\n' "$server" "$port" "$uuid" "$name" "$ip"
    done | {
        # Build JSON from piped data
        first=1
        printf '['
        while IFS='|' read -r server port uuid name ip; do
            [ -z "$server" ] && continue
            [ "$first" -eq 0 ] && printf ','
            first=0
            printf '\n  {"address":"%s","port":%s,"uuid":"%s","name":"%s","ip":"%s"}' \
                "$server" "$port" "$uuid" "$name" "$ip"
        done
        printf '\n]\n'
    } > "$SERVERS_FILE"

    SERVER_COUNT=$(jq length "$SERVERS_FILE")

    if [ "$SERVER_COUNT" -eq 0 ]; then
        print_error "No servers could be resolved"
        rm -f "$SERVERS_FILE"
        exit 1
    fi

    print_success "Saved $SERVER_COUNT servers to $SERVERS_FILE"
}

###############################################################################
# Step 3: Update vpn-director.json
###############################################################################

step_update_config() {
    print_header "Step 3: Updating Configuration"

    DATA_DIR=$(get_data_dir)
    SERVERS_FILE="$DATA_DIR/servers.json"

    # Extract all IPs from servers.json
    server_ips=$(jq -r '.[].ip' "$SERVERS_FILE" | sort -u)

    # Build JSON array of IPs
    xray_servers_json=$(printf '%s\n' $server_ips | jq -R . | jq -s .)

    # Create config from template if doesn't exist
    if [ ! -f "$VPD_CONFIG" ]; then
        if [ ! -f "$VPD_TEMPLATE" ]; then
            print_error "Template not found: $VPD_TEMPLATE"
            exit 1
        fi
        cp "$VPD_TEMPLATE" "$VPD_CONFIG"
        print_info "Created $VPD_CONFIG from template"
    fi

    # Update xray.servers in config
    jq --argjson servers "$xray_servers_json" \
        '.xray.servers = $servers' \
        "$VPD_CONFIG" > "${VPD_CONFIG}.tmp" && \
        mv "${VPD_CONFIG}.tmp" "$VPD_CONFIG"

    ip_count=$(printf '%s\n' $server_ips | wc -l | tr -d ' ')
    print_success "Updated xray.servers with $ip_count IP addresses"
}
```

**Step 5: Add main function**

```sh
###############################################################################
# Main
###############################################################################

main() {
    print_header "Import VLESS Server List"
    printf "This will download and parse VLESS servers.\n\n"

    step_get_vless_file
    step_parse_and_save_servers
    step_update_config

    print_header "Import Complete"
    printf "Server list saved. Run configure.sh to continue setup.\n"
}

main "$@"
```

**Step 6: Make executable and commit**

```bash
chmod +x jffs/scripts/vpn-director/import_server_list.sh
git add jffs/scripts/vpn-director/import_server_list.sh
git commit -m "feat: add import_server_list.sh for VLESS server import"
```

---

## Task 5: Update configure.sh — remove import logic

**Files:**
- Modify: `jffs/scripts/vpn-director/configure.sh`

**Step 1: Remove VLESS-related variables**

Remove lines 22-28 (VLESS_SERVERS, XRAY_SERVERS_IPS, etc.):
```sh
# DELETE these lines:
VLESS_SERVERS=""
XRAY_SERVERS_IPS=""
```

Keep only:
```sh
XRAY_CLIENTS_LIST=""
TUN_DIR_RULES_LIST=""
SELECTED_SERVER_ADDRESS=""
SELECTED_SERVER_PORT=""
SELECTED_SERVER_UUID=""
XRAY_EXCLUDE_SETS_LIST="ru"
```

**Step 2: Remove parse_vless_uri function**

Delete lines 69-103 (the entire `parse_vless_uri()` function).

**Step 3: Remove step_get_vless_file function**

Delete lines 105-157 (the entire `step_get_vless_file()` function).

**Step 4: Remove step_parse_vless_servers function**

Delete lines 159-201 (the entire `step_parse_vless_servers()` function).

**Step 5: Commit removal of import functions**

```bash
git add jffs/scripts/vpn-director/configure.sh
git commit -m "refactor(configure): remove VLESS import functions"
```

---

## Task 6: Update configure.sh — add servers.json reading

**Files:**
- Modify: `jffs/scripts/vpn-director/configure.sh`

**Step 1: Add get_data_dir and startup check**

After the helper functions, add:

```sh
###############################################################################
# Get data directory and validate servers
###############################################################################

get_data_dir() {
    local config_file="$JFFS_DIR/vpn-director.json"

    if [ ! -f "$config_file" ]; then
        config_file="$JFFS_DIR/vpn-director.json.template"
    fi

    jq -r '.tunnel_director.data_dir // "/jffs/scripts/vpn-director/data"' "$config_file"
}

check_servers_file() {
    DATA_DIR=$(get_data_dir)
    SERVERS_FILE="$DATA_DIR/servers.json"

    if [ ! -f "$SERVERS_FILE" ]; then
        print_error "Server list not found: $SERVERS_FILE"
        print_info "Run import_server_list.sh first"
        exit 1
    fi

    SERVER_COUNT=$(jq length "$SERVERS_FILE")
    if [ "$SERVER_COUNT" -eq 0 ]; then
        print_error "Server list is empty"
        print_info "Run import_server_list.sh again"
        exit 1
    fi

    print_success "Found $SERVER_COUNT servers in $SERVERS_FILE"
}
```

**Step 2: Rewrite step_select_xray_server to read from JSON**

Replace the entire `step_select_xray_server()` function:

```sh
###############################################################################
# Step 1: Select Xray server
###############################################################################

step_select_xray_server() {
    print_header "Step 1: Select Xray Server"

    printf "Available servers:\n\n"

    # Read servers from JSON and display
    i=1
    jq -r '.[] | "\(.name)|\(.address)|\(.ip)|\(.port)|\(.uuid)"' "$SERVERS_FILE" | \
    while IFS='|' read -r name address ip port uuid; do
        printf "  %2d) %s\n      %s -> %s\n\n" "$i" "$name" "$address" "$ip"
        i=$((i + 1))
    done

    total=$(jq length "$SERVERS_FILE")

    while true; do
        printf "Select server [1-%d]: " "$total"
        read -r choice

        if [ "$choice" -ge 1 ] 2>/dev/null && [ "$choice" -le "$total" ] 2>/dev/null; then
            break
        fi
        print_error "Invalid choice. Enter a number between 1 and $total"
    done

    # Get selected server data (jq uses 0-based index)
    idx=$((choice - 1))
    SELECTED_SERVER_ADDRESS=$(jq -r ".[$idx].address" "$SERVERS_FILE")
    SELECTED_SERVER_PORT=$(jq -r ".[$idx].port" "$SERVERS_FILE")
    SELECTED_SERVER_UUID=$(jq -r ".[$idx].uuid" "$SERVERS_FILE")
    selected_name=$(jq -r ".[$idx].name" "$SERVERS_FILE")

    print_success "Selected: $selected_name ($SELECTED_SERVER_ADDRESS)"
}
```

**Step 3: Update step numbers in remaining functions**

Update headers in existing functions:
- `step_configure_xray_exclusions`: "Step 4" → "Step 2"
- `step_configure_clients`: "Step 5" → "Step 3"
- `step_show_summary`: "Step 6" → "Step 4"
- `step_generate_configs`: "Step 7" → "Step 5"
- `step_apply_rules`: "Step 8" → "Step 6"

**Step 4: Update step_show_summary**

Remove the XRAY_SERVERS_IPS count display (lines 403-404):
```sh
# DELETE these lines:
server_ip_count=$(printf '%s\n' "$XRAY_SERVERS_IPS" | wc -w | tr -d ' ')
printf "XRAY_SERVERS ipset: %s IP addresses\n" "$server_ip_count"
```

**Step 5: Update step_generate_configs**

Remove the XRAY_SERVERS_IPS JSON generation (lines 445-448):
```sh
# DELETE these lines:
xray_servers_json="[]"
if [ -n "$XRAY_SERVERS_IPS" ]; then
    xray_servers_json=$(printf '%s\n' $XRAY_SERVERS_IPS | jq -R . | jq -s .)
fi
```

And remove `--argjson servers "$xray_servers_json"` and `.xray.servers = $servers |` from the jq command — servers are already set by import_server_list.sh.

**Step 6: Update main function**

Replace main():

```sh
main() {
    print_header "VPN Director Configuration"
    printf "This wizard will configure Xray TPROXY and Tunnel Director.\n\n"

    check_servers_file                # Validate servers.json exists
    step_select_xray_server           # Step 1
    step_configure_xray_exclusions    # Step 2
    step_configure_clients            # Step 3
    step_show_summary                 # Step 4
    step_generate_configs             # Step 5
    step_apply_rules                  # Step 6

    print_header "Configuration Complete"
    printf "Check status with: /jffs/scripts/vpn-director/xray_tproxy.sh status\n"
}
```

**Step 7: Commit**

```bash
git add jffs/scripts/vpn-director/configure.sh
git commit -m "refactor(configure): read servers from servers.json"
```

---

## Task 7: Update documentation

**Files:**
- Modify: `CLAUDE.md`
- Modify: `.claude/rules/ipset-builder.md`

**Step 1: Update CLAUDE.md**

Add import_server_list.sh to commands section:

```markdown
# Import servers
/jffs/scripts/vpn-director/import_server_list.sh
```

Update config path description:
```markdown
| `tunnel_director.data_dir` | `/jffs/scripts/vpn-director/data` | Data storage (servers, ipset dumps) |
```

**Step 2: Update .claude/rules/ipset-builder.md**

Update paths:
- `$IPS_BDR_DIR/dumps/countries/` → `$IPS_BDR_DIR/ipsets/`
- `ipset_dump_dir` → `data_dir`

**Step 3: Commit**

```bash
git add CLAUDE.md .claude/rules/ipset-builder.md
git commit -m "docs: update for data_dir and import_server_list.sh"
```

---

## Task 8: Final verification

**Step 1: Verify file structure**

```bash
ls -la jffs/scripts/vpn-director/
```

Expected new file: `import_server_list.sh`

**Step 2: Verify JSON template**

```bash
jq '.tunnel_director.data_dir' jffs/scripts/vpn-director/vpn-director.json.template
```

Expected: `"/jffs/scripts/vpn-director/data"`

**Step 3: Verify config.sh reads correct path**

```bash
grep 'data_dir' jffs/scripts/vpn-director/utils/config.sh
```

Expected: `.tunnel_director.data_dir`

**Step 4: Verify ipset_builder.sh paths**

```bash
grep 'COUNTRY_DUMP_DIR' jffs/scripts/vpn-director/ipset_builder.sh
```

Expected: `COUNTRY_DUMP_DIR="$IPS_BDR_DIR/ipsets"`
