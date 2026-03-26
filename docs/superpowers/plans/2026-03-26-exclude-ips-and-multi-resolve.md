# Exclude IPs & Multi-Resolve Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add user-defined IP exclusions, auto-detect OpenVPN endpoints, and resolve all IPs for multi-homed Xray servers to prevent TPROXY routing loops.

**Architecture:** Three sources (xray.servers, xray.exclude_ips, OpenVPN nvram endpoints) are merged into a single TPROXY_BYPASS ipset (renamed from XRAY_SERVERS) at apply time. Go bot gains multi-IP resolution (`ips` field) and a new `/exclude` wizard command. Shell import script uses `resolve_ip -a` for all IPs. `/import` auto-syncs xray.servers. Shell validates exclude_ips before adding to ipset.

**Tech Stack:** Bash (shell scripts, Bats tests), Go (Telegram bot)

---

## File Structure

### Shell (modified)

| File | Responsibility |
|------|---------------|
| `router/opt/vpn-director/lib/config.sh` | Add `XRAY_EXCLUDE_IPS` variable |
| `router/opt/vpn-director/lib/tproxy.sh` | Merge 3 sources into TPROXY_BYPASS ipset (renamed from XRAY_SERVERS), validate exclude_ips, update status |
| `router/opt/vpn-director/import_server_list.sh` | Multi-IP resolution with `resolve_ip -a` |
| `router/opt/vpn-director/vpn-director.json.template` | Add `exclude_ips` field |
| `router/test/fixtures/vpn-director.json` | Add `exclude_ips` to fixture |
| `router/test/mocks/nvram` | Add OpenVPN client mock responses |
| `router/test/mocks/nslookup` | Add multi-IP mock responses |
| `router/test/unit/tproxy.bats` | Tests for 3-source ipset assembly |
| `router/test/import_server_list.bats` | Tests for multi-IP resolution |

### Go bot (modified)

| File | Responsibility |
|------|---------------|
| `telegram-bot/internal/vless/parser.go` | `ResolveIP()` → `ResolveIPs()`, `IP` → `IPs` |
| `telegram-bot/internal/vpnconfig/vpnconfig.go` | `Server.IP` → `Server.IPs`, `XrayConfig.ExcludeIPs` |
| `telegram-bot/internal/handler/import.go` | Use `ResolveIPs()`, map to `vpnconfig.Server.IPs` |
| `telegram-bot/internal/wizard/apply.go` | Collect IPs from `server.IPs` (all servers) |

### Go bot (new)

| File | Responsibility |
|------|---------------|
| `telegram-bot/internal/wizard/exclude_ips.go` | ExcludeIPsStep: list/add/remove exclude_ips |
| `telegram-bot/internal/handler/exclude.go` | `/exclude` command handler |

### Go bot (modified for /configure new step)

| File | Responsibility |
|------|---------------|
| `telegram-bot/internal/wizard/state.go` | Add `StepExcludeIPs` step, `ExcludeIPs` field |
| `telegram-bot/internal/wizard/handler.go` | Wire ExcludeIPsStep into step chain |
| `telegram-bot/internal/bot/router.go` | Add `/exclude` command route + `ExcludeRouterHandler` |
| `telegram-bot/internal/bot/bot.go` | Wire ExcludeHandler into router |

---

## Task 1: Config & template — add `exclude_ips` field

**Files:**
- Modify: `router/opt/vpn-director/vpn-director.json.template`
- Modify: `router/opt/vpn-director/lib/config.sh:52-94`
- Modify: `router/test/fixtures/vpn-director.json`
- Test: `router/test/config.bats`

- [ ] **Step 1: Update config template**

Add `exclude_ips` to the xray section in `vpn-director.json.template`:

```json
  "xray": {
    "clients": [],
    "servers": [],
    "exclude_ips": [],
    "exclude_sets": ["ru"]
  },
```

- [ ] **Step 2: Update test fixture**

Add `exclude_ips` to `router/test/fixtures/vpn-director.json`:

```json
  "xray": {
    "clients": ["192.168.1.100"],
    "servers": ["1.2.3.4"],
    "exclude_ips": ["5.6.7.8", "10.20.0.0/16"],
    "exclude_sets": ["ru"]
  },
```

- [ ] **Step 3: Add XRAY_EXCLUDE_IPS to config.sh**

In `router/opt/vpn-director/lib/config.sh`, add after line 53 (`TPROXY_BYPASS`):

```bash
XRAY_EXCLUDE_IPS=$(_cfg_arr '.xray.exclude_ips')
```

And add to the `readonly` block (line 88):

```bash
readonly \
    VPD_CONFIG_FILE \
    TUN_DIR_TUNNELS_JSON IPS_BDR_DIR \
    XRAY_CLIENTS TPROXY_BYPASS XRAY_EXCLUDE_IPS XRAY_EXCLUDE_SETS \
```

- [ ] **Step 4: Write test for new variable**

Add to `router/test/config.bats`:

```bash
@test "config: XRAY_EXCLUDE_IPS is loaded from config" {
    load_config
    [[ "$XRAY_EXCLUDE_IPS" == *"5.6.7.8"* ]]
    [[ "$XRAY_EXCLUDE_IPS" == *"10.20.0.0/16"* ]]
}
```

- [ ] **Step 5: Run tests**

Run: `bats router/test/config.bats`
Expected: All tests PASS including the new one.

- [ ] **Step 6: Commit**

```bash
git add router/opt/vpn-director/vpn-director.json.template \
       router/opt/vpn-director/lib/config.sh \
       router/test/fixtures/vpn-director.json \
       router/test/config.bats
git commit -m "feat: add exclude_ips config field for user-defined IP exclusions"
```

---

## Task 2: Shell — 3-source ipset assembly in tproxy.sh

**Files:**
- Modify: `router/opt/vpn-director/lib/tproxy.sh:215-240` (`_tproxy_setup_bypass_ipset`)
- Modify: `router/opt/vpn-director/lib/tproxy.sh:364-401` (`tproxy_status`)
- Modify: `router/test/mocks/nvram`
- Modify: `router/test/mocks/nslookup`
- Test: `router/test/unit/tproxy.bats`

- [ ] **Step 1: Update nvram mock to include OpenVPN clients**

Replace `router/test/mocks/nvram` with:

```bash
#!/bin/bash
case "$*" in
    "get ipv6_service") echo "native" ;;
    "get wan0_primary") echo "1" ;;
    "get wan0_ifname")  echo "eth0" ;;
    "get wan1_primary") echo "0" ;;
    "get wan1_ifname")  echo "eth1" ;;
    "get model")        echo "RT-AX88U" ;;
    "get vpn_client1_addr") echo "openvpn1.example.com" ;;
    "get vpn_client2_addr") echo "" ;;
    "get vpn_client3_addr") echo "example.com" ;;
    "get vpn_client4_addr") echo "" ;;
    "get vpn_client5_addr") echo "" ;;
    *) echo "" ;;
esac
```

- [ ] **Step 2: Update nslookup mock for multi-IP**

Add a multi-IP host to `router/test/mocks/nslookup`:

```bash
#!/bin/bash
host="$1"
case "$host" in
    "example.com")
        echo "Server: 8.8.8.8"
        echo "Address: 8.8.8.8#53"
        echo ""
        echo "Name: example.com"
        echo "Address: 93.184.216.34"
        ;;
    "openvpn1.example.com")
        echo "Server: 8.8.8.8"
        echo "Address: 8.8.8.8#53"
        echo ""
        echo "Name: openvpn1.example.com"
        echo "Address: 10.0.0.1"
        ;;
    "ipv6.example.com")
        echo "Server: 8.8.8.8"
        echo "Address: 8.8.8.8#53"
        echo ""
        echo "Name: ipv6.example.com"
        echo "Address: 2606:2800:220:1:248:1893:25c8:1946"
        ;;
    *)
        echo "** server can't find $host: NXDOMAIN"
        exit 1
        ;;
esac
```

- [ ] **Step 3: Write failing tests for 3-source ipset**

Add to `router/test/unit/tproxy.bats`:

```bash
# ============================================================================
# _tproxy_setup_bypass_ipset - 3-source ipset assembly
# ============================================================================

@test "_tproxy_setup_bypass_ipset: adds xray servers from config" {
    load_tproxy_module
    _tproxy_setup_bypass_ipset
    # From fixture: TPROXY_BYPASS contains "1.2.3.4"
    run ipset test "$TPROXY_BYPASS_IPSET" "1.2.3.4"
    assert_success
}

@test "_tproxy_setup_bypass_ipset: adds exclude_ips from config" {
    load_tproxy_module
    _tproxy_setup_bypass_ipset
    # From fixture: XRAY_EXCLUDE_IPS contains "5.6.7.8"
    run ipset test "$TPROXY_BYPASS_IPSET" "5.6.7.8"
    assert_success
}

@test "_tproxy_setup_bypass_ipset: adds openvpn endpoints from nvram" {
    load_tproxy_module
    _tproxy_setup_bypass_ipset
    # nvram mock: vpn_client1_addr=openvpn1.example.com -> 10.0.0.1
    run ipset test "$TPROXY_BYPASS_IPSET" "10.0.0.1"
    assert_success
}

@test "_tproxy_setup_bypass_ipset: logs counts per source" {
    load_tproxy_module
    run _tproxy_setup_bypass_ipset
    assert_success
    assert_output --partial "xray"
    assert_output --partial "user"
    assert_output --partial "openvpn"
}

@test "_tproxy_setup_bypass_ipset: skips empty nvram entries" {
    load_tproxy_module
    # vpn_client2_addr is empty in mock — should not cause error
    run _tproxy_setup_bypass_ipset
    assert_success
}

@test "_tproxy_setup_bypass_ipset: warns on unresolvable openvpn endpoint" {
    load_tproxy_module
    # Override nvram to return unresolvable host
    nvram() {
        case "$*" in
            "get vpn_client1_addr") echo "nonexistent.host.invalid" ;;
            *) command nvram "$@" ;;
        esac
    }
    export -f nvram
    run _tproxy_setup_bypass_ipset
    assert_success
    assert_output --partial "WARN"
}
```

- [ ] **Step 4: Run tests to verify they fail**

Run: `bats router/test/unit/tproxy.bats`
Expected: New tests FAIL (exclude_ips and openvpn not yet implemented).

- [ ] **Step 5: Implement _tproxy_setup_bypass_ipset**

Replace `_tproxy_setup_bypass_ipset()` in `router/opt/vpn-director/lib/tproxy.sh` (lines 215-240):

```bash
_tproxy_setup_bypass_ipset() {
    local ip addr resolved
    local -a servers_array=()
    local -a exclude_ips_array=()
    local xray_count=0 user_count=0 ovpn_count=0

    # Create ipset if not exists
    if ! ipset list "$TPROXY_BYPASS_IPSET" >/dev/null 2>&1; then
        ipset create "$TPROXY_BYPASS_IPSET" hash:net
        log "Created ipset: $TPROXY_BYPASS_IPSET"
    fi

    # Flush and repopulate
    ipset flush "$TPROXY_BYPASS_IPSET"

    # Source 1: Xray server IPs from config
    if [[ -n ${TPROXY_BYPASS:-} ]]; then
        read -ra servers_array <<< "$TPROXY_BYPASS"
        for ip in "${servers_array[@]}"; do
            [[ -n $ip ]] || continue
            ipset add "$TPROXY_BYPASS_IPSET" "$ip" 2>/dev/null && xray_count=$((xray_count + 1)) || {
                log -l WARN "Failed to add xray server $ip to $TPROXY_BYPASS_IPSET"
            }
        done
    fi

    # Source 2: User-defined exclude IPs from config
    if [[ -n ${XRAY_EXCLUDE_IPS:-} ]]; then
        read -ra exclude_ips_array <<< "$XRAY_EXCLUDE_IPS"
        for ip in "${exclude_ips_array[@]}"; do
            [[ -n $ip ]] || continue
            ipset add "$TPROXY_BYPASS_IPSET" "$ip" 2>/dev/null && user_count=$((user_count + 1)) || {
                log -l WARN "Failed to add user exclude IP $ip to $TPROXY_BYPASS_IPSET"
            }
        done
    fi

    # Source 3: OpenVPN client endpoints from nvram (resolved on the fly)
    local slot
    for slot in 1 2 3 4 5; do
        addr=$(nvram get "vpn_client${slot}_addr" 2>/dev/null) || addr=""
        [[ -n $addr ]] || continue

        resolved=$(resolve_ip -a -q "$addr" 2>/dev/null) || {
            log -l WARN "Cannot resolve OpenVPN endpoint vpn_client${slot}_addr=$addr"
            continue
        }

        while IFS= read -r ip; do
            [[ -n $ip ]] || continue
            ipset add "$TPROXY_BYPASS_IPSET" "$ip" 2>/dev/null && ovpn_count=$((ovpn_count + 1)) || true
        done <<< "$resolved"
    done

    local total=$((xray_count + user_count + ovpn_count))
    log "Populated $TPROXY_BYPASS_IPSET ipset: $xray_count xray, $user_count user, $ovpn_count openvpn = $total total"
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `bats router/test/unit/tproxy.bats`
Expected: All tests PASS.

- [ ] **Step 7: Commit**

```bash
git add router/opt/vpn-director/lib/tproxy.sh \
       router/test/unit/tproxy.bats \
       router/test/mocks/nvram \
       router/test/mocks/nslookup
git commit -m "feat: merge 3 sources into TPROXY_BYPASS ipset (xray, user, openvpn)"
```

---

## Task 3: Shell — multi-IP resolution in import_server_list.sh

**Files:**
- Modify: `router/opt/vpn-director/import_server_list.sh:216-248`
- Test: `router/test/import_server_list.bats`

- [ ] **Step 1: Write failing test for multi-IP import**

Add to `router/test/import_server_list.bats`:

```bash
@test "step_parse_and_save_servers: saves ips array instead of ip" {
    load_import_server_list

    DATA_DIR="/tmp/bats_test_import_data"
    SERVERS_FILE="$DATA_DIR/servers.json"
    mkdir -p "$DATA_DIR"

    VLESS_SERVERS="vless://test-uuid@example.com:443?type=tcp#TestServer"

    run step_parse_and_save_servers
    assert_success

    # Check that servers.json has "ips" array, not "ip" string
    run jq -r '.[0].ips | type' "$SERVERS_FILE"
    assert_output "array"

    # Check that "ip" field does not exist
    run jq -r '.[0] | has("ip")' "$SERVERS_FILE"
    assert_output "false"

    rm -rf "$DATA_DIR"
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bats router/test/import_server_list.bats`
Expected: New test FAILS (still uses `ip` field).

- [ ] **Step 3: Update import_server_list.sh for multi-IP**

In `router/opt/vpn-director/import_server_list.sh`, replace the IP resolution and JSON building section (lines 216-248):

Change line 217 from:
```bash
ip=$(resolve_ip -q "$server" 2>/dev/null) || ip=$(resolve_ip -6 -g -q "$server" 2>/dev/null) || ip=""
```
to:
```bash
ips_raw=$(resolve_ip -a -q "$server" 2>/dev/null) || ips_raw=""
```

Update the skip condition (lines 219-223) from checking `$ip` to checking `$ips_raw`:
```bash
if [[ -z "$ips_raw" ]]; then
    log -l DEBUG "SKIP: cannot resolve $server"
    log -l WARN "Cannot resolve $server, skipping"
    continue
fi
```

Update the debug/info output (lines 225-226):
```bash
local ips_oneline
ips_oneline=$(printf '%s' "$ips_raw" | tr '\n' ',' | sed 's/,$//')
log -l DEBUG "Resolved: $server -> $ips_oneline"
printf "  %s (%s) -> %s\n" "$name" "$server" "$ips_oneline" >&2
```

Change the piped output (line 229) — output a JSON-safe IPs list instead of single IP:
```bash
printf '%s\n%s\n%s\n%s\n%s\n' "$server" "$port" "$uuid" "$name" "$ips_raw"
```

Update the JSON builder (lines 234-245) — the `read` loop now reads ips_raw, and builds an ips array:
```bash
while IFS= read -r server && IFS= read -r port && IFS= read -r uuid && IFS= read -r name && IFS= read -r ips_raw; do
    [[ -z "$server" ]] && continue
    [[ "$first" -eq 0 ]] && printf ',\n'
    first=0

    # Build ips JSON array from newline-separated IPs
    local ips_json
    ips_json=$(printf '%s\n' "$ips_raw" | tr ',' '\n' | jq -R . | jq -s .)

    jq -n \
        --arg addr "$server" \
        --arg port "$port" \
        --arg uuid "$uuid" \
        --arg name "$name" \
        --argjson ips "$ips_json" \
        '{address: $addr, port: ($port | tonumber), uuid: $uuid, name: $name, ips: $ips}' | tr -d '\n'
done
```

Note: The `ips_raw` variable from the pipe is comma-separated on a single line (from `printf` output), so we split by comma in the JSON builder.

Actually, let me reconsider — `resolve_ip -a` returns newline-separated IPs, and we're piping through `printf '%s\n' ...`. We need to pack multi-line IPs into one field for the pipe. Change the output to comma-separated:

```bash
# In the pipe output section:
ips_csv=$(printf '%s' "$ips_raw" | tr '\n' ',' | sed 's/,$//')
printf '%s\n%s\n%s\n%s\n%s\n' "$server" "$port" "$uuid" "$name" "$ips_csv"
```

And in the JSON builder:
```bash
while IFS= read -r server && IFS= read -r port && IFS= read -r uuid && IFS= read -r name && IFS= read -r ips_csv; do
    [[ -z "$server" ]] && continue
    [[ "$first" -eq 0 ]] && printf ',\n'
    first=0

    # Build ips JSON array from comma-separated IPs
    local ips_json
    ips_json=$(printf '%s\n' "$ips_csv" | tr ',' '\n' | jq -R 'select(length > 0)' | jq -s .)

    jq -n \
        --arg addr "$server" \
        --arg port "$port" \
        --arg uuid "$uuid" \
        --arg name "$name" \
        --argjson ips "$ips_json" \
        '{address: $addr, port: ($port | tonumber), uuid: $uuid, name: $name, ips: $ips}' | tr -d '\n'
done
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `bats router/test/import_server_list.bats`
Expected: All tests PASS.

- [ ] **Step 5: Commit**

```bash
git add router/opt/vpn-director/import_server_list.sh \
       router/test/import_server_list.bats
git commit -m "feat: resolve all IPs per server in import_server_list.sh"
```

---

## Task 4: Go — Server struct `IP` → `IPs` migration

**Files:**
- Modify: `telegram-bot/internal/vless/parser.go:31-37,115-130`
- Modify: `telegram-bot/internal/vpnconfig/vpnconfig.go:8-14`
- Modify: `telegram-bot/internal/handler/import.go:92-104`
- Modify: `telegram-bot/internal/wizard/apply.go:121-135`
- Test: existing tests in `telegram-bot/internal/vless/`, `telegram-bot/internal/wizard/`

- [ ] **Step 1: Update vless.Server struct**

In `telegram-bot/internal/vless/parser.go`, change the Server struct (lines 31-37):

```go
type Server struct {
	Address string `json:"address"`
	Port    int    `json:"port"`
	UUID    string `json:"uuid"`
	Name    string `json:"name"`
	IPs     []string `json:"ips"`
}
```

- [ ] **Step 2: Replace ResolveIP with ResolveIPs**

In `telegram-bot/internal/vless/parser.go`, replace ResolveIP (lines 115-130):

```go
func (s *Server) ResolveIPs() error {
	ips, err := net.LookupIP(s.Address)
	if err != nil {
		return err
	}
	var resolved []string
	for _, ip := range ips {
		if ipv4 := ip.To4(); ipv4 != nil {
			resolved = append(resolved, ipv4.String())
		}
	}
	if len(resolved) == 0 {
		return fmt.Errorf("no IPv4 addresses found for %s", s.Address)
	}
	s.IPs = resolved
	return nil
}
```

Add `"fmt"` to imports if not already present.

- [ ] **Step 3: Update vpnconfig.Server struct**

In `telegram-bot/internal/vpnconfig/vpnconfig.go`, change Server struct (lines 8-14):

```go
type Server struct {
	Address string   `json:"address"`
	Port    int      `json:"port"`
	UUID    string   `json:"uuid"`
	Name    string   `json:"name"`
	IPs     []string `json:"ips"`
}
```

- [ ] **Step 4: Update XrayConfig struct**

In `telegram-bot/internal/vpnconfig/vpnconfig.go`, add ExcludeIPs to XrayConfig (lines 41-45):

```go
type XrayConfig struct {
	Clients     []string `json:"clients"`
	Servers     []string `json:"servers"`
	ExcludeIPs  []string `json:"exclude_ips"`
	ExcludeSets []string `json:"exclude_sets"`
}
```

- [ ] **Step 5: Update import handler**

In `telegram-bot/internal/handler/import.go`, change the resolve loop (lines 92-104):

```go
	for _, s := range servers {
		if err := s.ResolveIPs(); err != nil {
			resolveErrors++
			continue
		}
		resolved = append(resolved, vpnconfig.Server{
			Address: s.Address,
			Port:    s.Port,
			UUID:    s.UUID,
			Name:    s.Name,
			IPs:     s.IPs,
		})
	}
```

- [ ] **Step 6: Update wizard apply — collect all IPs from all servers**

In `telegram-bot/internal/wizard/apply.go`, replace the server IPs collection (lines 121-130):

```go
	// Server IPs (unique, non-empty, sorted) — collect ALL IPs from ALL servers
	seen := make(map[string]bool)
	var serverIPs []string
	for _, s := range servers {
		for _, ip := range s.IPs {
			if ip != "" && !seen[ip] {
				seen[ip] = true
				serverIPs = append(serverIPs, ip)
			}
		}
	}
	sort.Strings(serverIPs)
```

- [ ] **Step 7: Fix any other references to Server.IP**

Search for `.IP` references on Server structs and update. Key places:
- `telegram-bot/internal/handler/servers.go` — server list display (if it shows IP)
- `telegram-bot/internal/handler/xray.go` — xray quick switch (if it references IP)

For each: change `s.IP` to `strings.Join(s.IPs, ", ")` for display, or `s.IPs` for data.

- [ ] **Step 8: Run Go tests**

Run: `cd telegram-bot && go test ./...`
Expected: All tests PASS (or identify tests that need updating due to struct change).

- [ ] **Step 9: Fix any failing tests**

Update test fixtures/expectations that reference `IP` field to use `IPs` field.

- [ ] **Step 10: Commit**

```bash
git add telegram-bot/
git commit -m "feat: multi-IP resolution — Server.IP → Server.IPs, resolve all IPv4 addresses"
```

---

## Task 5: Go — `/exclude` wizard command

**Files:**
- Create: `telegram-bot/internal/wizard/exclude_ips.go`
- Create: `telegram-bot/internal/handler/exclude.go`
- Modify: `telegram-bot/internal/wizard/state.go` (add ExcludeIPs field, StepExcludeIPs step)
- Modify: `telegram-bot/internal/bot/router.go` (add `/exclude` route)
- Modify: `telegram-bot/internal/bot/bot.go` (wire handler)
- Test: `telegram-bot/internal/wizard/exclude_ips_test.go`, `telegram-bot/internal/handler/exclude_test.go`

- [ ] **Step 1: Add ExcludeIPs state**

In `telegram-bot/internal/wizard/state.go`, add the new step constant and state field.

Add to Step constants:
```go
const (
	StepNone         Step = ""
	StepSelectServer Step = "select_server"
	StepExclusions   Step = "exclusions"
	StepExcludeIPs   Step = "exclude_ips"
	StepClients      Step = "clients"
	StepClientIP     Step = "client_ip"
	StepClientRoute  Step = "client_route"
	StepConfirm      Step = "confirm"
)
```

Add `ExcludeIPs` field to State struct:
```go
type State struct {
	mu          sync.RWMutex
	ChatID      int64
	Step        Step
	ServerIndex int
	Exclusions  map[string]bool
	ExcludeIPs  []string
	Clients     []ClientRoute
	PendingIP   string
}
```

Add thread-safe accessors:
```go
func (s *State) AddExcludeIP(ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ExcludeIPs = append(s.ExcludeIPs, ip)
}

func (s *State) RemoveExcludeIP(index int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index >= 0 && index < len(s.ExcludeIPs) {
		s.ExcludeIPs = append(s.ExcludeIPs[:index], s.ExcludeIPs[index+1:]...)
	}
}

func (s *State) SetExcludeIPs(ips []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ExcludeIPs = ips
}

func (s *State) GetExcludeIPs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := make([]string, len(s.ExcludeIPs))
	copy(cp, s.ExcludeIPs)
	return cp
}
```

Update `Manager.Start()` to initialize ExcludeIPs:
```go
state := &State{
	ChatID:     chatID,
	Step:       StepSelectServer,
	Exclusions: make(map[string]bool),
	ExcludeIPs: []string{},
	Clients:    []ClientRoute{},
}
```

- [ ] **Step 2: Create ExcludeIPsStep**

Create `telegram-bot/internal/wizard/exclude_ips.go`:

```go
package wizard

import (
	"fmt"
	"net"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/telegram"
)

// ExcludeIPsStep handles the exclude IPs wizard step
type ExcludeIPsStep struct {
	deps *StepDeps
	next func(chatID int64, state *State)
}

// NewExcludeIPsStep creates a new ExcludeIPsStep handler
func NewExcludeIPsStep(deps *StepDeps, next func(chatID int64, state *State)) *ExcludeIPsStep {
	return &ExcludeIPsStep{deps: deps, next: next}
}

// Render displays the exclude IPs UI
func (s *ExcludeIPsStep) Render(chatID int64, state *State) {
	text, keyboard := s.buildUI(state)
	s.deps.Sender.SendWithKeyboard(chatID, text, keyboard)
}

// HandleCallback processes button presses
func (s *ExcludeIPsStep) HandleCallback(cb *tgbotapi.CallbackQuery, state *State) {
	data := cb.Data

	if !strings.HasPrefix(data, "exclip:") {
		return
	}

	action := strings.TrimPrefix(data, "exclip:")

	switch {
	case action == "done":
		state.SetStep(StepClients)
		if s.next != nil {
			s.next(cb.Message.Chat.ID, state)
		}
		return

	case action == "skip":
		state.SetStep(StepClients)
		if s.next != nil {
			s.next(cb.Message.Chat.ID, state)
		}
		return

	case action == "add":
		state.SetStep(StepExcludeIPs)
		s.deps.Sender.SendPlain(cb.Message.Chat.ID,
			"Enter IP address or CIDR (e.g., 1.2.3.4 or 10.0.0.0/8):")
		return

	case strings.HasPrefix(action, "rm:"):
		idxStr := strings.TrimPrefix(action, "rm:")
		var idx int
		if _, err := fmt.Sscanf(idxStr, "%d", &idx); err == nil {
			state.RemoveExcludeIP(idx)
		}
	}

	// Refresh UI
	text, keyboard := s.buildUI(state)
	s.deps.Sender.EditMessage(cb.Message.Chat.ID, cb.Message.MessageID, text, keyboard)
}

// HandleMessage processes text input (IP/CIDR entry)
func (s *ExcludeIPsStep) HandleMessage(msg *tgbotapi.Message, state *State) bool {
	input := strings.TrimSpace(msg.Text)
	if input == "" {
		return false
	}

	// Validate as IP or CIDR
	if !isValidIPOrCIDR(input) {
		s.deps.Sender.SendPlain(msg.Chat.ID,
			"Invalid format. Enter IPv4 address (1.2.3.4) or CIDR (10.0.0.0/8):")
		return true
	}

	state.AddExcludeIP(input)

	// Re-render the list
	text, keyboard := s.buildUI(state)
	s.deps.Sender.SendWithKeyboard(msg.Chat.ID, text, keyboard)
	return true
}

func (s *ExcludeIPsStep) buildUI(state *State) (string, tgbotapi.InlineKeyboardMarkup) {
	ips := state.GetExcludeIPs()

	kb := telegram.NewKeyboard()

	// Show existing entries with remove buttons
	for i, ip := range ips {
		kb.Button(fmt.Sprintf("Remove %s", ip), fmt.Sprintf("exclip:rm:%d", i)).Row()
	}

	// Add button
	kb.Button("Add", "exclip:add")

	if len(ips) > 0 {
		kb.Button("Done", "exclip:done")
	} else {
		kb.Button("Skip", "exclip:skip")
	}
	kb.Row()

	kb.Button("Cancel", "cancel").Row()

	var sb strings.Builder
	sb.WriteString(telegram.EscapeMarkdownV2("Exclude IPs from proxy"))
	sb.WriteString("\n")
	if len(ips) > 0 {
		sb.WriteString(telegram.EscapeMarkdownV2(fmt.Sprintf("Current: %s", strings.Join(ips, ", "))))
	} else {
		sb.WriteString(telegram.EscapeMarkdownV2("No extra IPs configured"))
	}

	return sb.String(), kb.Build()
}

// isValidIPOrCIDR validates input as IPv4 or IPv4 CIDR
func isValidIPOrCIDR(s string) bool {
	// Try CIDR first
	if strings.Contains(s, "/") {
		_, _, err := net.ParseCIDR(s)
		return err == nil
	}
	// Try plain IP
	ip := net.ParseIP(s)
	return ip != nil && ip.To4() != nil
}
```

- [ ] **Step 3: Wire ExcludeIPsStep into /configure wizard**

In `telegram-bot/internal/wizard/handler.go`, update `NewHandler()` to insert ExcludeIPsStep between ExclusionsStep and ClientsStep:

```go
func NewHandler(
	sender telegram.MessageSender,
	config service.ConfigStore,
	vpn service.VPNDirector,
	xray service.XrayGenerator,
) *Handler {
	deps := &StepDeps{Sender: sender, Config: config}
	manager := NewManager()

	var serverStep, exclusionsStep, excludeIPsStep, clientsStep, confirmStep StepHandler

	// ServerStep -> ExclusionsStep
	serverStep = NewServerStep(deps, func(chatID int64, state *State) {
		exclusionsStep.Render(chatID, state)
	})

	// ExclusionsStep -> ExcludeIPsStep
	exclusionsStep = NewExclusionsStep(deps, func(chatID int64, state *State) {
		// Pre-populate ExcludeIPs from existing config
		cfg, err := config.LoadVPNConfig()
		if err == nil && len(cfg.Xray.ExcludeIPs) > 0 {
			state.SetExcludeIPs(cfg.Xray.ExcludeIPs)
		}
		excludeIPsStep.Render(chatID, state)
	})

	// ExcludeIPsStep -> ClientsStep
	excludeIPsStep = NewExcludeIPsStep(deps, func(chatID int64, state *State) {
		clientsStep.Render(chatID, state)
	})

	// ClientsStep -> ConfirmStep
	clientsStep = NewClientsStep(deps, func(chatID int64, state *State) {
		confirmStep.Render(chatID, state)
	})

	// ConfirmStep has no next callback
	confirmStep = NewConfirmStep(deps)

	return &Handler{
		manager: manager,
		steps: map[Step]StepHandler{
			StepSelectServer: serverStep,
			StepExclusions:   exclusionsStep,
			StepExcludeIPs:   excludeIPsStep,
			StepClients:      clientsStep,
			StepClientIP:     clientsStep,
			StepClientRoute:  clientsStep,
			StepConfirm:      confirmStep,
		},
		sender:  sender,
		applier: NewApplier(manager, sender, config, vpn, xray),
	}
}
```

- [ ] **Step 4: Update wizard apply to save ExcludeIPs**

In `telegram-bot/internal/wizard/apply.go`, add after setting `vpnCfg.Xray.Servers`:

```go
	// Exclude IPs from wizard state
	excludeIPs := state.GetExcludeIPs()
	vpnCfg.Xray.ExcludeIPs = excludeIPs
```

- [ ] **Step 5: Create /exclude command handler**

Create `telegram-bot/internal/handler/exclude.go`:

```go
package handler

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/telegram"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/wizard"
)

// ExcludeHandler handles /exclude command
type ExcludeHandler struct {
	deps    *Deps
	wizard  *wizard.Handler
	manager *wizard.Manager
}

// NewExcludeHandler creates a new ExcludeHandler
func NewExcludeHandler(deps *Deps, wizardHandler *wizard.Handler) *ExcludeHandler {
	return &ExcludeHandler{
		deps:    deps,
		wizard:  wizardHandler,
		manager: wizardHandler.GetManager(),
	}
}

// HandleExclude starts the exclude IPs wizard
func (h *ExcludeHandler) HandleExclude(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID

	// Load current config to pre-populate
	cfg, err := h.deps.Config.LoadVPNConfig()
	if err != nil {
		h.deps.Sender.Send(chatID, telegram.EscapeMarkdownV2("Config load error: "+err.Error()))
		return
	}

	// Start a wizard-like session at the ExcludeIPs step
	state := h.manager.Start(chatID)
	state.SetStep(wizard.StepExcludeIPs)
	if len(cfg.Xray.ExcludeIPs) > 0 {
		state.SetExcludeIPs(cfg.Xray.ExcludeIPs)
	}

	// The ExcludeIPsStep is already registered in the wizard handler's steps map.
	// We trigger its Render by calling HandleCallback with a synthetic "show" or
	// by directly accessing the step. Since the wizard Handler manages routing,
	// we can directly send the start message through the wizard.
	h.deps.Sender.SendPlain(chatID, "Manage excluded IPs/CIDRs:")
	h.wizard.Start(chatID)
	// Override: reset to ExcludeIPs step
	state = h.manager.Get(chatID)
	if state != nil {
		state.SetStep(wizard.StepExcludeIPs)
		if len(cfg.Xray.ExcludeIPs) > 0 {
			state.SetExcludeIPs(cfg.Xray.ExcludeIPs)
		}
	}
}
```

Actually, this approach is overly complex since `wizard.Start()` always begins at StepSelectServer. A cleaner approach: add a `StartAtStep` method to the wizard Handler.

In `telegram-bot/internal/wizard/handler.go`, add:

```go
// StartAtStep begins a wizard session at a specific step
func (h *Handler) StartAtStep(chatID int64, step Step, setup func(state *State)) {
	state := h.manager.Start(chatID)
	state.SetStep(step)
	if setup != nil {
		setup(state)
	}
	if stepHandler, ok := h.steps[step]; ok {
		stepHandler.Render(chatID, state)
	}
}
```

Then simplify `exclude.go`:

```go
package handler

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/telegram"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/wizard"
)

// ExcludeHandler handles /exclude command
type ExcludeHandler struct {
	deps   *Deps
	wizard *wizard.Handler
}

// NewExcludeHandler creates a new ExcludeHandler
func NewExcludeHandler(deps *Deps, wizardHandler *wizard.Handler) *ExcludeHandler {
	return &ExcludeHandler{deps: deps, wizard: wizardHandler}
}

// HandleExclude starts the exclude IPs wizard
func (h *ExcludeHandler) HandleExclude(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID

	cfg, err := h.deps.Config.LoadVPNConfig()
	if err != nil {
		h.deps.Sender.Send(chatID, telegram.EscapeMarkdownV2("Config load error: "+err.Error()))
		return
	}

	h.wizard.StartAtStep(chatID, wizard.StepExcludeIPs, func(state *wizard.State) {
		if len(cfg.Xray.ExcludeIPs) > 0 {
			state.SetExcludeIPs(cfg.Xray.ExcludeIPs)
		}
	})
}

// HandleCallback delegates to wizard
func (h *ExcludeHandler) HandleCallback(cb *tgbotapi.CallbackQuery) {
	h.wizard.HandleCallback(cb)
}
```

- [ ] **Step 6: Wire /exclude into router**

In `telegram-bot/internal/bot/router.go`, add the interface and field:

```go
// ExcludeRouterHandler defines methods for exclude command
type ExcludeRouterHandler interface {
	HandleExclude(msg *tgbotapi.Message)
	HandleCallback(cb *tgbotapi.CallbackQuery)
}
```

Add `exclude ExcludeRouterHandler` to Router struct and NewRouter params.

Add routing in `RouteMessage`:
```go
case "exclude":
    r.exclude.HandleExclude(msg)
```

Add routing in `RouteCallback`:
```go
if strings.HasPrefix(cb.Data, "exclip:") {
    r.exclude.HandleCallback(cb)
    return
}
```

- [ ] **Step 7: Wire in bot.go**

In `telegram-bot/internal/bot/bot.go`, after creating the wizard handler, create exclude handler:

```go
excludeHandler := handler.NewExcludeHandler(deps, wizardHandler)
```

Pass to NewRouter.

Add to the Telegram command list:
```go
{Command: "exclude", Description: "Manage excluded IPs"},
```

- [ ] **Step 8: Handle /exclude "done" — save and apply**

The current ExcludeIPsStep "done" action transitions to StepClients (for `/configure` flow). For `/exclude`, "done" should save the config and apply.

The cleanest approach: when ExcludeIPsStep's `next` callback is called and the wizard was started at StepExcludeIPs (standalone `/exclude`), save and apply instead of going to clients.

Update the exclude handler's `StartAtStep` setup to set a flag, or better — use a dedicated "done" action in the ExcludeIPsStep that saves config directly when used standalone.

Simplest approach: in the `/exclude` handler, override the "done" behavior. When `/exclude` starts the wizard at StepExcludeIPs, set `next` to a save-and-apply callback. But since `next` is set at handler construction time...

Better approach: Make ExcludeIPsStep check if its `next` callback is nil when "done" is pressed; if nil, it saves config directly. Then construct it with `next=nil` when used from `/exclude`, and with `next=clientsStep.Render` when used from `/configure`.

Actually the simplest approach: when the `/exclude` wizard "done" is pressed, the `Applier` runs. Looking at the handler flow — when "apply" callback data is received, the Handler delegates to `h.applier.Apply()`. We can reuse this.

Change ExcludeIPsStep's "done" action: instead of transitioning to next step, check if `next` is nil. If nil, send "apply" callback data (or call apply directly).

Let me simplify. In ExcludeIPsStep, when "done" is pressed:
- If `next != nil`: transition to next step (part of /configure flow)
- If `next == nil`: set step to StepConfirm and trigger apply

But ExcludeIPsStep always has `next` set. Let me use a different approach.

For the standalone `/exclude` command, the ExcludeIPsStep "done" should save `exclude_ips` to the config file and call `vpn-director.sh apply`, WITHOUT modifying other config fields. This is different from the full wizard apply.

Create a separate save method in exclude.go:

In the ExcludeHandler, when the wizard completes:

Actually, the cleanest solution: for `/exclude`, don't use the wizard flow at all — use a standalone handler that reads/writes `exclude_ips` directly.

Revised `exclude.go`:

```go
package handler

import (
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/telegram"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/wizard"
)

// ExcludeHandler handles /exclude command as a standalone wizard
type ExcludeHandler struct {
	deps    *Deps
	manager *wizard.Manager
	sender  telegram.MessageSender
}

func NewExcludeHandler(deps *Deps) *ExcludeHandler {
	return &ExcludeHandler{
		deps:    deps,
		manager: wizard.NewManager(),
		sender:  deps.Sender,
	}
}

func (h *ExcludeHandler) HandleExclude(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID

	cfg, err := h.deps.Config.LoadVPNConfig()
	if err != nil {
		h.sender.Send(chatID, telegram.EscapeMarkdownV2("Config load error: "+err.Error()))
		return
	}

	state := h.manager.Start(chatID)
	state.SetStep(wizard.StepExcludeIPs)
	if len(cfg.Xray.ExcludeIPs) > 0 {
		state.SetExcludeIPs(cfg.Xray.ExcludeIPs)
	}

	h.renderUI(chatID, state)
}

func (h *ExcludeHandler) HandleCallback(cb *tgbotapi.CallbackQuery) {
	if cb.Message == nil || cb.Message.Chat == nil {
		return
	}
	chatID := cb.Message.Chat.ID
	state := h.manager.Get(chatID)
	if state == nil {
		return
	}

	data := cb.Data
	if !strings.HasPrefix(data, "exclip:") {
		if data == "cancel" {
			h.manager.Clear(chatID)
			h.sender.SendPlain(chatID, "Cancelled")
		}
		return
	}

	action := strings.TrimPrefix(data, "exclip:")

	switch {
	case action == "done":
		h.saveAndApply(chatID, state)
		return

	case action == "add":
		h.sender.SendPlain(chatID, "Enter IP address or CIDR (e.g., 1.2.3.4 or 10.0.0.0/8):")
		return

	case strings.HasPrefix(action, "rm:"):
		var idx int
		if _, err := fmt.Sscanf(strings.TrimPrefix(action, "rm:"), "%d", &idx); err == nil {
			state.RemoveExcludeIP(idx)
		}
	}

	text, keyboard := h.buildUI(state)
	h.sender.EditMessage(chatID, cb.Message.MessageID, text, keyboard)
}

func (h *ExcludeHandler) HandleTextInput(msg *tgbotapi.Message) {
	state := h.manager.Get(msg.Chat.ID)
	if state == nil {
		return
	}

	input := strings.TrimSpace(msg.Text)
	if input == "" {
		return
	}

	if !wizard.IsValidIPOrCIDR(input) {
		h.sender.SendPlain(msg.Chat.ID,
			"Invalid format. Enter IPv4 (1.2.3.4) or CIDR (10.0.0.0/8):")
		return
	}

	state.AddExcludeIP(input)
	h.renderUI(msg.Chat.ID, state)
}

func (h *ExcludeHandler) renderUI(chatID int64, state *wizard.State) {
	text, keyboard := h.buildUI(state)
	h.sender.SendWithKeyboard(chatID, text, keyboard)
}

func (h *ExcludeHandler) buildUI(state *wizard.State) (string, tgbotapi.InlineKeyboardMarkup) {
	ips := state.GetExcludeIPs()
	kb := telegram.NewKeyboard()

	for i, ip := range ips {
		kb.Button(fmt.Sprintf("Remove %s", ip), fmt.Sprintf("exclip:rm:%d", i)).Row()
	}

	kb.Button("Add", "exclip:add")
	if len(ips) > 0 {
		kb.Button("Done", "exclip:done")
	} else {
		kb.Button("Done", "exclip:done")
	}
	kb.Row()
	kb.Button("Cancel", "cancel").Row()

	var sb strings.Builder
	sb.WriteString(telegram.EscapeMarkdownV2("Manage excluded IPs"))
	sb.WriteString("\n")
	if len(ips) > 0 {
		sb.WriteString(telegram.EscapeMarkdownV2(
			fmt.Sprintf("Current: %s", strings.Join(ips, ", "))))
	} else {
		sb.WriteString(telegram.EscapeMarkdownV2("No IPs configured"))
	}

	return sb.String(), kb.Build()
}

func (h *ExcludeHandler) saveAndApply(chatID int64, state *wizard.State) {
	defer h.manager.Clear(chatID)

	cfg, err := h.deps.Config.LoadVPNConfig()
	if err != nil {
		h.sender.SendPlain(chatID, fmt.Sprintf("Config load error: %v", err))
		return
	}

	cfg.Xray.ExcludeIPs = state.GetExcludeIPs()

	if err := h.deps.Config.SaveVPNConfig(cfg); err != nil {
		h.sender.SendPlain(chatID, fmt.Sprintf("Save error: %v", err))
		return
	}
	h.sender.SendPlain(chatID, "vpn-director.json updated")

	if err := h.deps.VPN.Apply(); err != nil {
		h.sender.SendPlain(chatID, fmt.Sprintf("Apply error: %v", err))
		return
	}
	h.sender.SendPlain(chatID, "Done!")
}
```

Export `IsValidIPOrCIDR` from the wizard package (move from `exclude_ips.go` to a shared util or export it) so both the wizard step and the handler can use it.

Put `IsValidIPOrCIDR` in `telegram-bot/internal/wizard/exclude_ips.go` as exported, and use it from both places.

The `/configure` wizard's ExcludeIPsStep will also need `IsValidIPOrCIDR` — keep it in `exclude_ips.go`.

- [ ] **Step 9: Wire /exclude route and text handler**

In `router.go`, add `ExcludeRouterHandler` interface:

```go
type ExcludeRouterHandler interface {
	HandleExclude(msg *tgbotapi.Message)
	HandleCallback(cb *tgbotapi.CallbackQuery)
	HandleTextInput(msg *tgbotapi.Message)
}
```

In `RouteMessage`, add:
```go
case "exclude":
    r.exclude.HandleExclude(msg)
```

Text input: non-command messages currently go to `r.wizard.HandleTextInput(msg)`. The exclude handler also needs text input. Route based on active state:
- If exclude wizard has active state for this chat, route to exclude handler
- Otherwise, route to wizard handler

Simplest: in the default (non-command) branch, check exclude first:
```go
default:
    // Check if exclude wizard is active
    r.exclude.HandleTextInput(msg)
    // Also try wizard (it will check its own state)
    r.wizard.HandleTextInput(msg)
```

But this double-dispatches. Better: make ExcludeHandler.HandleTextInput return bool indicating if it handled the message. But the interface uses void returns.

Simpler: ExcludeHandler checks its own manager for state. If no state, returns silently. WizardHandler also checks its manager. Since they use separate Managers, only one will have active state.

In `RouteCallback`, add:
```go
if strings.HasPrefix(cb.Data, "exclip:") {
    r.exclude.HandleCallback(cb)
    return
}
```

- [ ] **Step 10: Run Go tests**

Run: `cd telegram-bot && go test ./...`
Expected: All tests PASS.

- [ ] **Step 11: Commit**

```bash
git add telegram-bot/
git commit -m "feat: add /exclude command and ExcludeIPs step in /configure wizard"
```

---

## Task 6: Go — ExcludeIPsStep in /configure wizard flow

**Files:**
- Modify: `telegram-bot/internal/wizard/exclude_ips.go` (already created in Task 5)
- Modify: `telegram-bot/internal/wizard/handler.go` (already modified in Task 5)
- Modify: `telegram-bot/internal/wizard/apply.go`
- Test: `telegram-bot/internal/wizard/exclude_ips_test.go`

This task is already covered in Task 5, Steps 2-4. The key parts:

- [ ] **Step 1: Verify ExcludeIPsStep renders as step 2.5/5 in /configure**

After ExclusionsStep (step 2), the wizard should show ExcludeIPsStep before ClientsStep (step 3). The step numbering in UI text should update:

- ExcludeIPsStep: "Extra IPs to exclude from proxy"
- Update step labels if they show N/M format

- [ ] **Step 2: Write test for ExcludeIPsStep**

Create `telegram-bot/internal/wizard/exclude_ips_test.go`:

```go
package wizard

import (
	"testing"
)

func TestIsValidIPOrCIDR(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"1.2.3.4", true},
		{"10.0.0.0/8", true},
		{"192.168.1.0/24", true},
		{"not-an-ip", false},
		{"256.1.1.1", false},
		{"", false},
		{"::1", false}, // IPv6 not supported
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := IsValidIPOrCIDR(tt.input); got != tt.valid {
				t.Errorf("IsValidIPOrCIDR(%q) = %v, want %v", tt.input, got, tt.valid)
			}
		})
	}
}
```

- [ ] **Step 3: Run tests**

Run: `cd telegram-bot && go test ./internal/wizard/...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add telegram-bot/internal/wizard/
git commit -m "test: add ExcludeIPsStep validation tests"
```

---

## Task 7: Status output — breakdown by source

**Files:**
- Modify: `router/opt/vpn-director/lib/tproxy.sh:364-401` (`tproxy_status`)
- Test: `router/test/unit/tproxy.bats`

- [ ] **Step 1: Write failing test**

Add to `router/test/unit/tproxy.bats`:

```bash
@test "tproxy_status: shows servers ipset section" {
    load_tproxy_module
    run tproxy_status
    assert_success
    assert_output --partial "Servers Ipset"
}
```

This test already passes (existing). Add a test for source breakdown — but since status reads from live ipset, it's hard to test the breakdown format in unit tests. Skip detailed status testing for now.

- [ ] **Step 2: Update tproxy_status to show source breakdown**

In `router/opt/vpn-director/lib/tproxy.sh`, update the "Servers Ipset" section of `tproxy_status()`:

Replace the servers ipset section (lines 386-388):

```bash
    printf '%s\n' "--- Servers Ipset ---"
    if ipset list "$TPROXY_BYPASS_IPSET" >/dev/null 2>&1; then
        local total
        total=$(ipset list "$TPROXY_BYPASS_IPSET" | grep -c '^[0-9]' || true)
        printf 'Ipset %s: %d entries\n' "$TPROXY_BYPASS_IPSET" "$total"

        # Show config-based counts
        local -a srv_arr=() excl_arr=()
        [[ -n ${TPROXY_BYPASS:-} ]] && read -ra srv_arr <<< "$TPROXY_BYPASS"
        [[ -n ${XRAY_EXCLUDE_IPS:-} ]] && read -ra excl_arr <<< "$XRAY_EXCLUDE_IPS"
        printf '  Sources: %d xray servers, %d user exclude_ips, rest = openvpn endpoints\n' \
            "${#srv_arr[@]}" "${#excl_arr[@]}"
    else
        printf 'Ipset %s not found\n' "$TPROXY_BYPASS_IPSET"
    fi
    printf '\n'
```

- [ ] **Step 3: Run tests**

Run: `bats router/test/unit/tproxy.bats`
Expected: All tests PASS.

- [ ] **Step 4: Commit**

```bash
git add router/opt/vpn-director/lib/tproxy.sh \
       router/test/unit/tproxy.bats
git commit -m "feat: show source breakdown in tproxy status output"
```

---

## Task 8: Final integration verification

- [ ] **Step 1: Run all shell tests**

Run: `bats router/test/`
Expected: All tests PASS.

- [ ] **Step 2: Run all Go tests**

Run: `cd telegram-bot && go test ./...`
Expected: All tests PASS.

- [ ] **Step 3: Build Go bot**

Run: `cd telegram-bot && make build`
Expected: Build succeeds.

- [ ] **Step 4: Commit any remaining fixes**

If any tests needed fixing, commit them.

- [ ] **Step 5: Clean up plan documents**

```bash
git rm -r docs/superpowers/
git commit -m "chore: remove plan documents before PR"
```
