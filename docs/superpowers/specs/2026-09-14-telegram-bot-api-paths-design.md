# Telegram bot API paths: design

Date: 2026-09-14
Branch: `feature/telegram-bot-api-paths`
Status: approved in brainstorming, awaiting implementation plan

## 1. Goal

The Telegram bot reaches `api.telegram.org` by any path that works. Direct WAN
is preferred. When direct is blocked, Xray's local SOCKS inbound and any
Tunnel Director tunnel are equal: the bot stays on whichever of those is
already working, and picks one only when it has no current path.

Only the bot's Telegram API client is in scope. The Web UI, GitHub update
checks, subscription import, and every other outbound HTTP stay as they are.

This is the live failure on the author's RT-AX86U: Xray is up and listening on
12345/12346, the configured Xray server accepts no TCP, `api.telegram.org` is
unreachable from the WAN, and `ovpnc2` already carries LAN traffic. The bot
retries `TLS handshake timeout` forever and never logs `Telegram Bot started`.

## 2. Why Tunnel Director cannot do this

Tunnel Director marks in `mangle PREROUTING`. That chain sees forwarded LAN
traffic. The bot emits from `OUTPUT`. No TD rule, ipset, or fwmark will carry
the daemon's own sockets.

The chosen shape is userspace: a `DialContext` that either dials directly,
dials through SOCKS5 on Xray, or binds the socket to the tunnel interface with
`SO_BINDTODEVICE`. No new iptables chain, no fwmark bit, nothing for NDM to
wipe on KeeneticOS.

## 3. Architecture

Everything runs inside `telegram-bot`. No new daemon.

| Piece | Responsibility |
|-------|----------------|
| PathManager | Builds the candidate list, probes, stores the current path, reselects on a timer and on dial failure. |
| Transport | `http.RoundTripper` for `tgbotapi`. Each dial (and tunnel DNS) uses the current path. Changing path does not reconstruct `BotAPI`. |
| Discovery | Reads `vpn-director.json` and `vpn-director.sh platform`. Facts only, not a service. |

`bot.New` constructs PathManager with the bot token, `ConfigService.LoadVPNConfig`,
and `VPNDirectorService.Platform`, runs one selection pass, then hands
`NewBotAPIWithClient` an `http.Client` whose transport asks PathManager on
every dial. The transport calls `PathManager.ReportFailure(path)` on a
dial or TLS error for the path that dial used. If that path is still the
current path, a reselect starts immediately; if the manager has already
moved on, the call is a no-op so a late failure on an old keep-alive
cannot flap the new path.

The startup retry loop in `cmd/bot/main.go` stays: network failures retry
with backoff, `PermanentError` (bad token, unreadable bot config) still
exits.

`--dev` forces `direct` and does not start PathManager. The workstation has
no platform tunnels and must not call `SO_BINDTODEVICE`.

### 3.1 Path identity

A path is one of:

- `direct`
- `socks`
- `tunnel:<id>` where `<id>` is a Tunnel Director key (`ovpnc2`, `OpenVPN0`, …)

### 3.2 Alternatives rejected

- **Static `proxy` in `telegram-bot.json`.** The SOCKS listener is Xray, which
  VPN Director already configures. A second copy of that URL is a knob that
  the current outage leaves pointing at a dead process. Auto-discovery
  replaces it.
- **`mangle OUTPUT` + `ip rule`.** Needs a free fwmark bit, re-application
  after every NDM rebuild, and teardown in `tunnel_stop`, for one daemon.
- **A separate status daemon writing the current path to a file.** The bot
  still has to implement bind and SOCKS. Two processes for one consumer.
- **Racing every request.** The user asked for a monitor that prefers direct
  and does not flap between Xray and a tunnel.

## 4. Candidates

Rebuilt every cycle. No bot restart when a tunnel comes up or Xray starts.

| Path | Included when |
|------|----------------|
| `direct` | Always. |
| `socks` | `127.0.0.1:socks_port` accepts TCP. `socks_port` is `advanced.xray.socks_port` from `vpn-director.json`, or 12346 if missing, non-numeric, or non-positive. LAN `xray.clients` do not matter. |
| `tunnel:<id>` | `id` is a key of `tunnel_director.tunnels`; `id` is not `main`; `clients` has at least one entry (pause leaves the address in `clients`); `vpn-director.sh platform` lists that id with `connected: true` and a non-empty `iface`. |

WireGuard keys in Tunnel Director are included the same way as OpenVPN.
Tunnels the platform lists but Tunnel Director does not use are not
candidates: some of those interfaces are not internet exits.

If `LoadVPNConfig` fails, the cycle has no tunnel keys and falls back to the
default SOCKS port. If `Platform()` fails, the cycle has no tunnel
candidates. Direct and SOCKS still run.

The listen check for SOCKS is a 1-second TCP connect to `127.0.0.1:socks_port`.
Refused or timed out means `socks` is absent this cycle. The probe through
SOCKS is the real liveness test.

## 5. Probe

A path is live when an HTTPS request to Telegram's API **through that path's
dialer** returns any HTTP response. Status 401, 429, and 5xx count as live:
the API was reached. Dial errors, timeouts, and TLS handshake failures count
as dead.

The request is `GET https://api.telegram.org/bot<token>/getMe` with an 8-second
timeout, IPv4 only. Treating 401 as dead would make a bad token look like
"every path is down" and spin the startup loop forever. `New()` still maps
Telegram 401 to `PermanentError` and the process exits.

Do not log the probe URL. The token sits in the path; the existing log
scrubber is not a reason to print it.

## 6. Selection

Evaluated after each probe pass.

1. If `direct` is live, the current path is `direct` (including a switch back
   from `socks` or `tunnel:*`).
2. Else if the current path is `socks` or `tunnel:*` and that path is still
   live, keep it. Xray and tunnels have equal priority; do not flap.
3. Else the current path is the first live candidate in this order: `socks`,
   then `tunnel:<id>` sorted by `id`.
4. Else there is no current path. The HTTP client will fail dials. `main.go`
   keeps retrying. Log a WARN naming each candidate and why it failed.

### 6.1 When to probe

A cycle runs immediately at start, every 30 seconds, and immediately after
`ReportFailure` for the current path.

Each cycle:

- Always probe `direct` (otherwise the bot never returns from a tunnel when
  the WAN unblocks).
- If the current path is not `direct`, probe it (liveness).
- Probe `socks` and the tunnel candidates only when a replacement is needed:
  `direct` is dead and the current path is missing or dead. Probe them in
  selection order (`socks`, then `tunnel:<id>` sorted by id) and stop at the
  first live one.

Probes in a cycle are sequential. The failed request that triggered
`ReportFailure` is **not** retried inside `RoundTrip` (request bodies are
not safely replayable). The next request uses the new path.

On a path change the transport calls `CloseIdleConnections` so keep-alives
do not stay on the old interface.

## 7. Dial and DNS

`http.Transport.DialContext` snapshots the current path at the start of the
call and uses that snapshot for DNS and the TCP connect. A concurrent
reselect must not split one dial across two paths. Default
resolve-then-dial is not used: it would send DNS to the WAN on every path
and would pre-resolve the host for SOCKS.

| Path | Dial | DNS |
|------|------|-----|
| `direct` | `tcp4` | A records only, system resolver. |
| `socks` | `golang.org/x/net/proxy` as today | The SOCKS5 request carries the hostname `api.telegram.org`. Xray resolves it. The bot must not resolve first. |
| `tunnel:<id>` | `tcp4` with `SO_BINDTODEVICE` on `PlatformTunnel.Iface` | A `net.Resolver` whose `Dial` binds the same device and queries `8.8.8.8` over `udp4`/`tcp4`, then `1.1.1.1`. System `resolv.conf` is not used. |

`SO_BINDTODEVICE` requires root. Both daemons already run as root. A bind
error (interface gone) makes that path dead for the cycle; it is not a
process-fatal error.

IPv4 only on every path. Dual-stack lookups on these routers often never
answer the AAAA half; glibc then waits out `timeout: 5` (see
`.claude/rules/shell-conventions.md`).

`internal/ssrf` is not applied to this client. That guard is for
user-supplied import URLs. Destination `api.telegram.org` is public. The
tunnel's private address is the source, which the guard does not see.

In-flight requests finish on the dialer they started with. The next `RoundTrip`
sees the new path after `CloseIdleConnections`.

## 8. Config

`telegram-bot.json` gains no fields. `proxy` and `proxy_fallback_direct` are
removed from `config.Config`. `encoding/json` ignores leftover keys, so an
existing file keeps loading.

`setup_telegram_bot.sh` no longer asks about a proxy and no longer writes
those keys. Token, usernames, `log_level`, and `update_check_interval` stay.

SOCKS port and tunnel membership come only from `vpn-director.json`.

## 9. Logging

| Level | Event |
|-------|--------|
| INFO | Path selected or changed (`from`, `to`). |
| WARN | Direct is down and the bot moved to SOCKS or a tunnel. All paths dead, with per-candidate reasons. |
| DEBUG | Individual probe results. |

The token never appears in these lines.

## 10. Out of scope

- Web UI, GitHub API, asset downloads, `/import` / `ssrf.NewClient`
- New iptables rules, fwmark bits, NDM hooks
- IPv6
- Tunnels not in `tunnel_director.tunnels`
- Changing `vpn-director.json` schema
- Bot-token rotation, `telegram-bot.json` file mode, Keenetic v0.12.1

## 11. Files

| Path | Change |
|------|--------|
| `server/internal/bot/pathmanager.go` | New. Candidates, probes, selection, current path. |
| `server/internal/bot/pathmanager_test.go` | New. Table-driven selection and candidate filters. |
| `server/internal/bot/transport.go` | Replace `NewHTTPClient(proxyURL, fallbackDirect)` with a client whose `DialContext` follows PathManager. Bind + tunnel resolver live here or in a sibling file. |
| `server/internal/bot/transport_test.go` | Rewrite around paths, not SOCKS URLs. |
| `server/internal/bot/bot.go` | Construct PathManager, one select, then `NewBotAPIWithClient`. |
| `server/internal/config/config.go` | Drop `Proxy` and `ProxyFallbackDirect`. |
| `server/internal/config/config_test.go` | Leftover `proxy` keys still load; no assertions on those fields. |
| `router/opt/vpn-director/setup_telegram_bot.sh` | Remove the proxy prompt. |
| `router/test/unit/entrypoints.bats` | Unchanged (it only pins the bash prologue). |
| `.claude/rules/telegram-bot.md` | Config shape and transport behaviour. |

Discovery uses existing `service.ConfigService.LoadVPNConfig` and
`service.VPNDirectorService.Platform`. No new shell contract.

## 12. Testing

CI has no live Telegram and does not call `SO_BINDTODEVICE`.

PathManager, fake probes:

- Live `direct` wins, including a switch back from `tunnel:ovpnc2`.
- Dead `direct`, live current `socks` → keep `socks` even if a tunnel is live.
- No current path, live `socks` and live tunnel → `socks`.
- `main` is not a candidate. Empty `clients` is not a candidate.
  `connected: false` and an id the platform does not list are not candidates.
- SOCKS is absent when the listen check fails.
- HTTP 401 is live. Dial timeout is dead.
- `Platform()` error → no tunnels, `direct`/`socks` still considered.
- `--dev` stays on `direct`.

Dial:

- SOCKS `DialContext` receives the hostname, not a resolved IP.
- Direct and tunnel dial `tcp4` only.
- Path change calls `CloseIdleConnections`.
- Bind is asserted through an injected `Control` hook that records the
  interface name, not through a real `BindToDevice`.

`config.Load` of a file that still contains `proxy` succeeds. A test that
previously required those fields to be stored is rewritten to require that
they are ignored.

`setup_telegram_bot.sh` is interactive (`/dev/tty`) and has no prompt-level
bats today. The script change is reviewed by reading the generated `jq`
invocation: it must not write `proxy` or `proxy_fallback_direct`.

Existing `go test ./...` and the four-place bats suite stay green.

### 12.1 Device checks (not CI, only with the owner's permission)

1. **RT-AX86U.** Telegram blocked on WAN, Xray outbound dead, `ovpnc2` in
   Tunnel Director. The bot reaches the API. Log shows `to=tunnel:ovpnc2`.
2. **KN-4521.** `SO_BINDTODEVICE` on `ovpn_brN` (OpenVPN is a bridge there).
   Spec §15 check 5 of Keenetic (WireGuard iface) is still unverified and
   stays out of this work unless a WireGuard key is already in Tunnel
   Director.

## 13. Success

On the RT-AX86U as it is today, the bot leaves the startup retry loop and
logs `Telegram Bot started`, with the current path in the log, without any
edit to `telegram-bot.json` beyond what is already there. When the WAN can
reach Telegram again, a later cycle moves the path back to `direct`.
