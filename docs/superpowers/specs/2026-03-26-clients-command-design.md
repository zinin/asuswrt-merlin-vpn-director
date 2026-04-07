# Design: `/clients` command — manage VPN clients via Telegram bot

## Problem

Currently the only way to add, remove, or toggle VPN clients is through the full `/configure` wizard (4 steps: server, exclusions, exclude IPs, clients). There is no way to temporarily disable a client without removing it from config entirely.

## Solution

New `/clients` bot command that provides:
- List of all configured clients with their route and status (active/paused)
- Pause/Resume toggle per client (temporary, client stays in config)
- Remove client (permanent, removed from config)
- Add new client (IP + route selection)

## Config format

New top-level field `paused_clients` in `vpn-director.json`:

```json
{
  "paused_clients": ["192.168.50.10"],
  "xray": {
    "clients": ["192.168.50.10", "192.168.50.30"],
    "servers": ["1.2.3.4"],
    "exclude_ips": [],
    "exclude_sets": ["ru"]
  },
  "tunnel_director": {
    "tunnels": {
      "wgc1": {
        "clients": ["192.168.50.40/32"],
        "exclude": ["ru"]
      }
    }
  }
}
```

`paused_clients` acts as a mask: shell scripts filter it out from all `clients` arrays at config load time. A paused client's IP remains in its original `clients` array — only the `paused_clients` list changes on pause/resume.

### Rules

- `paused_clients` is a flat array of IP/CIDR strings
- IPs are stored in **normalized form**: single hosts always without `/32` suffix (e.g. `"192.168.50.10"`), subnets with their CIDR (e.g. `"192.168.50.0/24"`)
- The same normalized form is used in all `clients` arrays: xray stores `"192.168.50.10"`, tunnel_director also stores `"192.168.50.10"` (not `"192.168.50.10/32"`). The `/32` suffix is added only at the boundary when generating shell rules in tunnel.sh
- Matching is exact string comparison on normalized values
- An IP in `paused_clients` is filtered from ALL `clients` arrays (xray + all tunnels)
- If `paused_clients` is absent or empty, no filtering occurs
- Duplicates in `paused_clients` are ignored (treated as a set)

### Normalization

When adding a client (bot or wizard), strip `/32` suffix from single-host IPs before storing. This ensures consistent matching across xray and tunnel_director sections.

`normalizeIP(ip)`: if `ip` ends with `/32`, strip the suffix. Otherwise keep as-is.

## Shell-side changes

### config.sh

Single change: after loading `clients` arrays, subtract `paused_clients`. One filtering function applied to every clients array.

Pseudocode:
```bash
_config_filter_paused() {
  local paused_json
  paused_json=$(printf '%s' "$CONFIG_JSON" | jq -r '.paused_clients // []')
  # Filter: clients - paused_clients
  printf '%s' "$1" | jq --argjson paused "$paused_json" '[.[] | select(. as $ip | $paused | index($ip) | not)]'
}
```

Applied when parsing `XRAY_CLIENTS` and each tunnel's `clients` in config loading.

No changes to tunnel.sh, tproxy.sh, ipset.sh, or vpn-director.sh. Note: tunnel.sh already handles IPs without `/32` — it uses `${client%%/*}` to extract the host part for validation, and iptables `-s` accepts both formats.

## Bot UI

### `/clients` message

```
Clients:

192.168.50.10 → xray ⏸
192.168.50.30 → xray ▶
192.168.50.40 → wgc1 ▶
```

### Inline keyboard

One row per client:
```
[ ▶ Resume ] [ 🗑 Remove ]     ← paused client
[ ⏸ Pause  ] [ 🗑 Remove ]     ← active client
```

Bottom row:
```
[ ➕ Add client ]
```

### Empty state

If no clients configured:
```
No clients configured.

[ ➕ Add client ]
```

## Scenarios

### Pause

1. User taps ⏸ Pause next to a client
2. Bot adds IP to `paused_clients` in vpn-director.json
3. Bot calls `vpn-director.sh apply`
4. Message updates with new status (⏸ icon, Resume button)

### Resume

1. User taps ▶ Resume next to a client
2. Bot removes IP from `paused_clients`
3. Bot calls `vpn-director.sh apply`
4. Message updates with new status (▶ icon, Pause button)

### Remove

1. User taps 🗑 Remove
2. Bot shows confirmation: "Remove 192.168.50.10 from wgc1?"
3. User confirms
4. Bot removes IP from the corresponding `clients` array AND from `paused_clients` (if present)
5. Bot calls `vpn-director.sh apply`
6. Message updates (client gone from list)

### Add client

1. User taps ➕ Add client
2. Bot asks to enter IP address (text input)
3. User enters IP (e.g. "192.168.50.10" or "192.168.50.0/24")
4. Bot validates:
   - Valid IP or CIDR format
   - Not already present in any `clients` array
5. Bot shows inline buttons with available routes: `xray` + all tunnel names from config (e.g. `wgc1`, `ovpnc1`)
6. User selects route
7. Bot normalizes IP (strips `/32` for single hosts) and adds to corresponding `clients` array
8. Bot calls `vpn-director.sh apply`
9. Returns to client list view

## Bot-side changes

### New files

- `internal/handler/clients.go` — `ClientsHandler` struct and methods

### Handler structure

```go
type ClientsHandler struct {
    deps *Deps
}

// Command handler
func (h *ClientsHandler) HandleClients(ctx, msg)          // /clients command
// Callback handlers
func (h *ClientsHandler) HandlePause(ctx, callback)        // clients:pause:<ip>
func (h *ClientsHandler) HandleResume(ctx, callback)       // clients:resume:<ip>
func (h *ClientsHandler) HandleRemove(ctx, callback)       // clients:remove:<ip>
func (h *ClientsHandler) HandleRemoveConfirm(ctx, callback) // clients:remove_confirm:<ip>
func (h *ClientsHandler) HandleAdd(ctx, callback)          // clients:add
func (h *ClientsHandler) HandleAddRoute(ctx, callback)     // clients:add_route:<route>
// Text handler (for IP input during add flow)
func (h *ClientsHandler) HandleText(ctx, msg)
```

### State for add flow

Minimal state: track which chat is in "waiting for IP" mode. Similar pattern to ExcludeHandler's pending IP input.

### Config model changes

`internal/vpnconfig/vpnconfig.go`:
```go
type VPNDirectorConfig struct {
    DataDir        string                  `json:"data_dir,omitempty"`
    PausedClients  []string                `json:"paused_clients,omitempty"`
    TunnelDirector TunnelDirectorConfig    `json:"tunnel_director,omitempty"`
    Xray           XrayConfig              `json:"xray,omitempty"`
    Advanced       map[string]interface{}  `json:"advanced,omitempty"`
}
```

### Router registration

Register in `internal/bot/router.go`:
- Command: `/clients`
- Callback prefix: `clients:`
- Text handler: delegate to ClientsHandler when in add-IP state

### Helper: collect all clients

Function to build unified client list from config:

```go
type ClientInfo struct {
    IP     string
    Route  string
    Paused bool
}

func CollectClients(cfg *VPNDirectorConfig) []ClientInfo
```

Iterates xray.clients and all tunnel_director.tunnels[*].clients, checks each against paused_clients.

## Validation rules

- IP format: valid IPv4 address or CIDR (parsed with `net.ParseCIDR` or `net.ParseIP`)
- No duplicate IPs across all routes
- Route must exist in current config (xray section present, or tunnel name exists in tunnel_director.tunnels)
- Pause/Resume/Remove: IP must exist in config at time of action (handle race with stale keyboard)

## Error handling

- Apply failure: show error message, **return immediately** — don't update keyboard (config was already written, but apply failed — user can retry or check /status)
- Stale keyboard (client already removed by another action): verify IP exists in config via CollectClients before pause/resume/remove; if not found, refresh list silently
- Invalid IP input during add: show error, ask to try again
- Duplicate IP during add: show "This IP is already configured for route X"
