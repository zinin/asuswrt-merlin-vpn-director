# Keenetic platform support: design

Date: 2026-09-10
Branch: `feature/keenetic-platform`
Status: approved in brainstorming, awaiting implementation plan

## 1. Goal

Run VPN Director on Keenetic routers (KeeneticOS 5.x with Entware) with the
same features it has on Asuswrt-Merlin: Xray TPROXY for selected LAN clients,
Tunnel Director for firmware VPN tunnels, country ipsets, the Web UI and the
Telegram bot. One repository serves both platforms. Behaviour on Merlin does
not change.

Target hardware: the author's Keenetic KN-4521 (aarch64, KeeneticOS 5.1.3,
kernel 4.9-ndm-5) plus MIPS little-endian Keenetic models (built, not tested).

## 2. Facts established on the device

Everything below was verified over SSH on the KN-4521 and drives the design.

| Area | Fact |
|------|------|
| TPROXY | Absent from the stock kernel. The firmware component `opkg-kmod-netfilter` ("Kernel modules for Netfilter") ships `xt_TPROXY.ko`, `xt_socket.ko`, `xt_comment.ko`, `xt_addrtype.ko`, `xt_multiport.ko` and more under `/lib/modules/4.9-ndm-5/`. Installing it rebuilds the firmware and reboots the router. Nothing autoloads the modules; `modprobe` does not exist; `insmod <path>.ko` works. After `insmod`, `TPROXY` appears in `/proc/net/ip_tables_targets` and `socket` in `ip_tables_matches`. |
| ipset | `hash:ip`, `hash:net`, `hash:mac` and the `set` match are built into the stock kernel. Entware provides the `ipset` tool (7.24, protocol 6 with a harmless version warning). |
| iptables | Absent from the firmware. Entware's `iptables` 1.4.21 supports `-j TPROXY`, `-m set`, `-m mark`, `MARK --set-xmark`, `REJECT --reject-with`, `REDIRECT`, `DNAT`. |
| ip | The firmware has no `ip`. Entware's busybox `ip` cannot address routing tables above 255; `ip-full` (iproute2 4.4) can. `/etc/iproute2/rt_tables` and `/opt/etc/iproute2/rt_tables` do not exist. |
| NDM marks | NDM marks with its own `NDMMARK`/`CONNNDMMARK` space and one full fwmark `0xffffaaa` (ip rule pref 100/101, LTE backup table 4096). Our marks `0x100` and `0x00ff0000` do not collide. NDM guards its DNS-routing chain with `-m mark --mark 0x0` (full match), so our chains must precede NDM's in `mangle PREROUTING`. |
| Port 443 | `mangle INPUT` jumps `-p tcp --dport 443` to `_NDM_HTTP_INPUT_TLS_`, which drops every TLS ClientHello whose SNI is not the router's own name. TPROXY keeps the original destination port, so proxied HTTPS dies there. `filter INPUT` accepts NEW connections from `br0` (private segment) regardless of destination, so nothing else blocks TPROXY. |
| Rebuilds | NDM rebuilds `filter`, `mangle` (and `nat`) from scratch on any configuration or hotspot change and deletes foreign chains, rules included. The `/opt/etc/ndm/netfilter.d/` hook runs once per table and per family with `$table` (`filter`, `mangle`, `nat`) and `$type` (`iptables`, `ip6tables`). `ip rule`, routing tables and ipsets survive rebuilds. |
| Routing | NDM keeps no routing table for OpenVPN0 (only 4096 for LTE backup and 16386 for the WAN source address). A private table `2007` with `default via 10.73.149.1 dev ovpn_br0` and `ip rule pref 16390 fwmark 0x70000/0xff0000 lookup 2007` carried real ICMP through the tunnel. `ip route get 1.1.1.1 mark 0x70000` resolves through the table. |
| RCI | `curl -s http://localhost:79/rci/show/interface/OpenVPN0` returns `type`, `connected`, `address`, `mask`, `remote-endpoint-address` (the VPN server IP), `via`, `description`. It returns neither the Linux interface name nor the in-tunnel gateway. `show/interface` lists all interfaces; `show/system` gives the hostname. `jq` from Entware lacks regex functions (`test`, `match`, `sub`). |
| Interfaces | LAN `br0` (Bridge0, Home, private) and `br1` (Bridge1, Guest, protected). OpenVPN0 is `ovpn_br0` (confirmed by matching addresses). WAN is the `dev` of `ip route show default` (`eth2.4`). |
| Tools | Busybox lacks `flock`, `nohup`, `pkill`, `openssl`, `modprobe`, `paste`. Entware has `flock`, `coreutils-nohup`, `procps-ng-pkill`, `openssl-util`, `gawk`, `cron` (Vixie with `cron.d`), `xray` + `xray-core` 26.2.6. `logger` reaches the NDM system log. `/tmp` is a 495 MB tmpfs. |
| Passwords | No `/etc/shadow`, no `/opt/etc/shadow`. `/opt/etc/passwd` holds the Entware root hash (`$1$`, MD5-crypt) in field 2. |

## 3. Architecture

One repository, one code base, one platform layer.

- Shell: `lib/platform.sh` detects the platform and sources
  `lib/platform/merlin.sh` or `lib/platform/keenetic.sh`. Both implement the
  same function contract (section 5). Core modules call the contract and never
  test the platform name.
- Go: `internal/platform` detects the platform from the file system for the
  few facts a daemon needs without shell (password file, hook files). Tunnel
  lists and interface names come from the new `vpn-director.sh platform`
  subcommand, so platform knowledge stays in shell.
- Web UI and bot build their route lists from `GET /api/platform`.
- Installer, updater and release build handle both platforms and three
  architectures from one file manifest.

Alternatives rejected: a separate Keenetic repository (duplicates ~16k lines
of Go, shell and Vue), a third "core" repository (cross-repo versioning for one
maintainer), platform facts as a generated JSON file (WAN, tunnel state and VPN
endpoints are dynamic).

## 4. Repository

### 4.1 Rename

Rename the GitHub repository to `vpn-director`. GitHub redirects git, API and
raw URLs from the old name, so installed Merlin routers keep updating. One
mechanical commit changes:

- Go module path to `github.com/zinin/vpn-director/server` (all imports).
- `repoName` in `server/internal/updater/github.go`.
- Install URLs in `README.md`, `README.ru.md`, `CLAUDE.md`, `install.sh`.

### 4.2 Layout

`router/` stays an image of `/` on the router. Platform hooks live under their
real paths:

```
router/opt/vpn-director/lib/platform.sh
router/opt/vpn-director/lib/platform/merlin.sh
router/opt/vpn-director/lib/platform/keenetic.sh
router/jffs/scripts/firewall-start                       (merlin)
router/jffs/scripts/wan-event                            (merlin)
router/opt/etc/ndm/netfilter.d/50-vpn-director.sh        (keenetic)
router/opt/etc/ndm/wan.d/50-vpn-director.sh              (keenetic)
router/opt/etc/ndm/ifstatechanged.d/50-vpn-director.sh   (keenetic)
router/files.manifest
```

### 4.3 File manifest

`router/files.manifest` replaces the file lists duplicated in `install.sh`
and `updater/downloader.go`. Format: one file per line, `<tag> <repo path>`,
`#` comments allowed. Tags: `common`, `merlin`, `keenetic`.

```
common   router/opt/vpn-director/vpn-director.sh
common   router/opt/vpn-director/lib/platform/keenetic.sh
merlin   router/jffs/scripts/firewall-start
keenetic router/opt/etc/ndm/netfilter.d/50-vpn-director.sh
```

Install destination is `/` plus the path without the `router/` prefix.
Every listed file is made executable except those ending in `.template`,
`.json` or `.manifest`. The installer and the updater fetch the manifest of
the release they install and then the files tagged `common` plus the current
platform. The Go binary embeds no file list, so a release may add files
without a prior binary update.

### 4.4 Documentation

- `README.md` / `README.ru.md`: both platforms, prerequisites per platform,
  the Keenetic component requirement, login name on Keenetic.
- `.claude/rules/keenetic.md`: NDM facts from section 2, hooks, RCI usage.
- `.claude/rules/packet-flow.md`: PREROUTING positions per platform.
- `CLAUDE.md`: architecture table entries for the platform layer and manifest.

## 5. Shell platform contract

### 5.1 Detection

`lib/platform.sh` exports `VPD_PLATFORM` and sources the implementation:

1. `VPD_PLATFORM` already set (tests, overrides): use it.
2. `/opt/etc/ndm` is a directory and `ndmc` is executable: `keenetic`.
3. `/jffs` is a directory and `nvram` is executable: `merlin`.
4. Otherwise fail with `unsupported platform` (exit 1).

Probe paths are prefixed with `VPD_PROBE_ROOT` when it is set, so tests can
build fake `/jffs` or `/opt/etc/ndm` trees in a temporary directory.

`lib/common.sh` sources `lib/platform.sh` after its own helpers, so every
script that sources `common.sh` gets the contract.

### 5.2 Functions

| Function | Merlin | Keenetic |
|----------|--------|----------|
| `platform_name` | `merlin` | `keenetic` |
| `platform_wan_if` | first `wanN_ifname` whose `wanN_primary` is 1, else `wan0_ifname` (moved from `get_active_wan_if`) | `dev` of `ip -4 route show default`, first line |
| `platform_ipv6_enabled` | `nvram get ipv6_service` not empty and not `disabled` | `ip -6 route show default` prints a line |
| `platform_lan_ifaces` | `br0` | `br0` |
| `platform_tunnels` | `wgcN` then `ovpncN` from `/etc/iproute2/rt_tables`, plus `main` last; one id per line | interfaces of type `OpenVPN` and `Wireguard` from RCI `show/interface`, plus `main` last; one id per line |
| `platform_tunnel_iface <id>` | `wgcN` for `wgcN`, `tun1N` for `ovpncN` | `ovpn_brN` for `OpenVPNN`, `nwgN` for `WireguardN` |
| `platform_tunnel_info <id>` | three lines: `type` (from the prefix), `connected` (`1` when the Linux interface exists and carries the `UP` flag, else `0`), `description` (`nvram get wgcN_desc` / `vpn_clientN_desc`) | three lines: `type`, `connected` (`1` when RCI `connected` is `yes`), `description`, from RCI `show/interface/<id>` |
| `platform_tunnel_table <id> <idx>` | prints `<id>` | prints `2000+idx` (`main` prints `main`). Pure mapping, always succeeds for ids from `platform_tunnels` |
| `platform_tunnel_route <id>` | unused | prints the route spec (section 6.5) as the arguments of `ip route replace <spec> table N` |
| `platform_tunnel_route_ensure <id> <idx>` | no-op, returns 0 | `ip route replace <spec> table 2000+idx` with the spec from `platform_tunnel_route`; returns 1 when the spec is unavailable (interface down) |
| `platform_tunnel_table_release <id> <idx>` | no-op | `ip route flush table 2000+idx` |
| `platform_vpn_endpoints` | `nvram get vpn_client{1..5}_addr`; one host per line | `remote-endpoint-address` of every OpenVPN interface and `peer[].endpoint` hosts of every Wireguard interface, from RCI; one host per line, resolved by the caller as today |
| `platform_load_module <name>` | `modprobe <name>` | `insmod /lib/modules/$(uname -r)/<name>.ko` unless `/proc/modules` lists it |
| `platform_cron_add <name> <schedule> <cmd>` | `cru a <name> "<schedule> <cmd>"` | write `/opt/etc/cron.d/<name>` as `<schedule> root <cmd>`; start `S10cron` if not running |
| `platform_cron_del <name>` | `cru d <name>` | remove `/opt/etc/cron.d/<name>` |
| `platform_tproxy_extra_rules apply\|stop` | no-op | section 6.4 |
| `platform_prerouting_base_pos` | current `_tunnel_get_prerouting_base_pos` logic | prints `2` |
| `platform_password_file` | `/etc/shadow` | `/opt/etc/passwd` |
| `platform_lan_ip` | `nvram get lan_ipaddr` | first IPv4 of `br0` |
| `platform_hostname` | `nvram get lan_hostname` | `hostname` from RCI `show/system` |
| `platform_model` | `nvram get model` | `model` from RCI `show/version` |
| `platform_email_supported` | `1` (amtm email is a Merlin feature; whether it is configured stays `send-email.sh`'s check) | `0` |

Every function prints its result on stdout and returns 0; a function that
cannot answer prints nothing and returns 1, and the caller decides (log and
skip, or fail). Functions never call `log -l ERROR`; the caller owns the
message.

### 5.3 `vpn-director.sh platform`

New subcommand. Prints one JSON document for the daemons:

```json
{
  "platform": "keenetic",
  "arch": "aarch64",
  "password_file": "/opt/etc/passwd",
  "lan_ifaces": ["br0"],
  "wan_if": "eth2.4",
  "tunnels": [
    {
      "id": "OpenVPN0",
      "iface": "ovpn_br0",
      "type": "openvpn",
      "connected": true,
      "description": "mitino.vpn.zinin.ru"
    }
  ]
}
```

`tunnels` excludes `main`. `type` is `openvpn` or `wireguard`. `wan_if` is
`""` when unknown. The command exits 0 even when a tunnel lookup fails; the
failing tunnel is omitted and a WARN line goes to the log.

## 6. Keenetic implementation

### 6.1 RCI access

`_rci_get <path>` runs `curl -s --max-time 5 http://localhost:79/rci/<path>`
and prints the body; a non-zero curl exit or empty body returns 1. Callers
parse with `jq -r`. No regex functions.

### 6.2 Kernel modules

`tproxy_apply` calls `platform_load_module xt_TPROXY` before it builds
rules. On Keenetic a missing `.ko` file means the firmware component is not
installed: `platform_load_module` returns 1, `tproxy_apply` logs
`ERROR: xt_TPROXY.ko not found; install the "Kernel modules for Netfilter"
component` and returns 1 without touching the firewall. Tunnel Director and
ipsets still apply.

### 6.3 Hooks

All three hook scripts are `#!/bin/sh`, set `PATH` like the Merlin hooks,
and never block NDM: they start the CLI with
`nohup /opt/vpn-director/vpn-director.sh --wait apply >/dev/null 2>&1 &`.
`--wait` queues behind a running instance, so bursts of hook calls converge
on the final state; repeated applies are no-ops.

`netfilter.d/50-vpn-director.sh`:

```sh
[ "$type" = "iptables" ] || exit 0
case "$table" in mangle|nat|filter) ;; *) exit 0 ;; esac
# start apply detached
```

`wan.d/50-vpn-director.sh`: runs apply when `$connected` is `yes`.

`ifstatechanged.d/50-vpn-director.sh`: runs apply when `$id` starts with
`OpenVPN` or `Wireguard`, for any change. Apply recreates the tunnel table
route; while the interface is down the route cannot be added, apply logs a
WARN, and marked traffic falls through to `main` (WAN), as it does on Merlin
when a `wgcN` table is empty.

The environment variable names for `wan.d` and `ifstatechanged.d` come from
the Keenetic hook documentation and are confirmed on the device by the
verification task in section 15 before the hooks are finalised.

### 6.4 TPROXY and port 443

`platform_tproxy_extra_rules apply` inserts, at position 1 of
`mangle INPUT`:

```
-m mark --mark 0x100/0x100 -j ACCEPT
```

`ACCEPT` in `mangle INPUT` ends that chain's traversal, so TPROXY-marked
packets never reach `_NDM_HTTP_INPUT_TLS_`. `platform_tproxy_extra_rules
stop` deletes the rule. Because NDM deletes the rule on every rebuild, the
`netfilter.d` hook restores it together with the rest.

Position of our chains in `mangle PREROUTING` on Keenetic: `XRAY_TPROXY` at
1 (unchanged), `TUN_DIR` at 2 (`platform_prerouting_base_pos`), ahead of
every `_NDM_*` jump.

### 6.5 Tunnel Director tables

Keenetic has no per-tunnel routing tables, so Tunnel Director owns them:

- Table number: `2000 + idx`, where `idx` is the 0-based position of the
  tunnel in `tunnel_director.tunnels`. Numbers are used directly; no
  `rt_tables` entry.
- `ip rule` prefs stay `16384 + idx`, marks stay `(idx + 1) << 16`.
- Route spec from `platform_tunnel_route <id>`:
  - `wireguard`: `default dev nwgN`.
  - `openvpn`: `default via <gateway> dev ovpn_brN`, where `gateway` is
    `tunnel_director.tunnels.<id>.gateway` when set, else the first host
    address of the tunnel subnet computed from RCI `address` and `mask`
    (`10.73.149.113/255.255.255.0` gives `10.73.149.1`).
  - When the interface is down or RCI reports no address, the function
    prints nothing and returns 1.
- `platform_tunnel_route_ensure` applies the spec with
  `ip route replace ... table N` on every apply, including the "up-to-date"
  path that skips the firewall rebuild, so the route survives interface flaps
  once the `ifstatechanged.d` hook fires. The `ip rule` for the tunnel is
  installed even when the route is not: a lookup in an empty table falls
  through to `main`.
- `tunnel_stop` calls `platform_tunnel_table_release` for each tunnel.

Interface name mapping is a convention (`OpenVPNN` to `ovpn_brN`,
`WireguardN` to `nwgN`). `platform_tunnel_iface` checks it when the tunnel is
connected: the IPv4 address of the Linux interface must equal RCI `address`.
On mismatch it prints nothing, returns 1, and the caller logs
`WARN: cannot map <id> to a Linux interface`.

### 6.6 Prerequisites

The installer checks, in order:

1. Firmware component: `/lib/modules/$(uname -r)/xt_TPROXY.ko` exists.
   Otherwise print where to enable "Kernel modules for Netfilter"
   (`opkg-kmod-netfilter`) in the Keenetic web interface, note that the router
   reboots, and exit 1.
2. Entware packages: `bash curl jq iptables ipset ip-full flock
   coreutils-nohup coreutils-base64 coreutils-sha256sum gawk procps-ng-pgrep
   procps-ng-pkill procps-ng-ps openssl-util cron xray`. The installer lists
   missing ones and offers to run `opkg install` for them (default yes on an
   interactive terminal; non-interactive runs print the command and exit 1).

Merlin keeps its current checks and README package list.

### 6.7 Cron

`S99vpn-director start` calls `platform_cron_add vpn_director_update
"0 3 * * *" "/opt/vpn-director/vpn-director.sh update"`; `stop` calls
`platform_cron_del`. On Keenetic this writes
`/opt/etc/cron.d/vpn_director_update` and starts `/opt/etc/init.d/S10cron`
when `crond` is not running.

### 6.8 Web UI login

The Web UI verifies against `/opt/etc/passwd`, so the login is `root` with
the Entware password set by `passwd`. `README` and the installer's final
message say so.

### 6.9 Architectures

`uname -m` maps to release asset suffixes: `aarch64` to `arm64`, `armv7l` to
`arm`, `mips` to `mipsle`. All Keenetic MIPS models are little-endian. Go
builds `mipsle` with `GOMIPS=softfloat`; Xray comes from the Entware
`mipsel-3.4` feed. MIPS builds ship untested and the README says so.

### 6.10 Email

`send-email.sh` returns 0 immediately when `platform_email_supported`
prints `0`, after one DEBUG log line.

## 7. Merlin implementation

`lib/platform/merlin.sh` collects the existing platform-specific code without
changing behaviour:

- `get_active_wan_if` and `get_ipv6_enabled` move from `common.sh` into
  `platform_wan_if` and `platform_ipv6_enabled`; `common.sh` keeps thin
  wrappers with the old names for one release so external callers do not
  break.
- `_tunnel_init`'s `rt_tables` parsing becomes `platform_tunnels`.
- `_tunnel_get_prerouting_base_pos` becomes `platform_prerouting_base_pos`.
- The `nvram get vpn_client${slot}_addr` loop in `tproxy.sh` becomes
  `platform_vpn_endpoints`.
- `cru` calls in `S99vpn-director` become `platform_cron_add/del`.
- `nvram get wan${wan_id}_ifname` in `firewall.sh` becomes
  `platform_wan_if`; the `wan_id` parameter of `block_wan_for_host` and
  `allow_wan_for_host` is dropped (the repository has no callers of either).

## 8. Core changes

- `common.sh`: sources `platform.sh`; wrappers as above.
- `tunnel.sh`: uses `platform_tunnels`, `platform_tunnel_table`,
  `platform_tunnel_route_ensure`, `platform_tunnel_table_release`,
  `platform_prerouting_base_pos`, `platform_lan_ifaces` (one PREROUTING jump
  per LAN interface). `tunnel_apply` records the applied tunnels as
  `<idx> <id>` lines in `/tmp/tunnel_director/tun_dir_tables`; the
  up-to-date path re-runs `platform_tunnel_route_ensure` for each line, and
  `tunnel_stop` calls `platform_tunnel_table_release` for each line before
  removing the file.
- `tproxy.sh`: uses `platform_load_module`, `platform_vpn_endpoints`,
  `platform_tproxy_extra_rules`, `platform_lan_ifaces`.
- `firewall.sh`: uses `platform_wan_if`, `platform_ipv6_enabled`.
- `configure.sh`: offers tunnels from `platform_tunnels` with
  `platform_tunnel_info` descriptions instead of `wgc`/`ovpnc` prefixes.
- `vpn-director.sh`: new `platform` subcommand; help text names both
  platforms.
- `S99vpn-director`: platform cron; unchanged otherwise. `S98*` daemon
  scripts are unchanged (they already avoid `rc.func` and `setsid`).
- `install.sh`: section 11.

## 9. Go daemons

### 9.1 `internal/platform`

```go
type Platform struct {
    Name         string // "merlin" | "keenetic"
    PasswordFile string // /etc/shadow | /opt/etc/passwd
}
func Detect() (Platform, error)
```

`Detect` applies the rules of section 5.1 against the real file system
(`VPD_PLATFORM` respected). `cmd/webui` and `cmd/bot` call it once at
startup; a `--platform` flag overrides it. Dev mode uses `merlin` unless the
flag says otherwise. The `--shadow` flag keeps overriding the password file.

### 9.2 Platform info from shell

`service.VPNDirector` gets `Platform(ctx) (vpnconfig.PlatformInfo, error)`,
which runs `vpn-director.sh platform` through the existing shell executor and
decodes section 5.3. No cache. The dev-mode mock executor returns
`testdata/dev/platform.json`.

```go
type PlatformInfo struct {
    Platform     string   `json:"platform"`
    Arch         string   `json:"arch"`
    PasswordFile string   `json:"password_file"`
    LANIfaces    []string `json:"lan_ifaces"`
    WANIf        string   `json:"wan_if"`
    Tunnels      []Tunnel `json:"tunnels"`
}
type Tunnel struct {
    ID          string `json:"id"`
    Iface       string `json:"iface"`
    Type        string `json:"type"`
    Connected   bool   `json:"connected"`
    Description string `json:"description"`
}
```

### 9.3 Route validation

`webapi/handler_clients.go`, `wizard/wizard.go` and `wizard/clients.go` drop
their `wgc1..ovpnc5` lists. A route is valid when it is `xray` or the `ID` of
a tunnel returned by `Platform()` at validation time. When `Platform()` fails
the handler answers 503 with `platform info unavailable`; the wizard shows
the same text and stays on the step.

### 9.4 API

`GET /api/platform` (authenticated) returns `PlatformInfo`. Response deadline
as for other shell-backed handlers.

### 9.5 Auth

`auth.ShadowAuth` parses fields 1 and 2 of each line, which suits both
`/etc/shadow` and `/opt/etc/passwd`. The package comment and `webui` help
text mention both files. No parser change.

### 9.6 Updater

- `repoName = "vpn-director"`.
- `DownloadRelease` fetches `router/files.manifest` at the release tag, then
  every `common` file and every file tagged with `platform.Name`. Unknown tags
  are ignored. A missing manifest fails the update with
  `release <tag> has no files.manifest`.
- `archAssetSuffix` accepts `mipsle`.
- `update_script.sh.tmpl` receives `Files []FileEntry{Src, Dst, Exec}` built
  from the manifest (`common` plus the platform tag) and copies each file to
  its destination, creating parent directories and setting the executable bit
  per section 4.3. This replaces every hard-coded `cp` and `chmod` line,
  including the `/jffs/scripts` ones.

## 10. Web UI and bot

- `ClientsTab.vue` loads `/api/platform` when mounted and builds the route
  options: `xray` first, then each tunnel as `<id>` with `<description>` and a
  `(down)` marker when not connected. The saved route of an existing client
  stays selectable even if the platform no longer lists it, so the user can
  see and fix it.
- Bot wizard: the route keyboard lists `xray` plus tunnel IDs from
  `Platform()`, one button per tunnel, label `<id> <description>`.
- `/status` and other commands are unchanged.

## 11. Installer, build, release

`install.sh`:

1. Detect the platform with the rules of section 5.1 (inline copy, since the
   library is not installed yet).
2. Check prerequisites: Merlin as today; Keenetic per section 6.6.
3. Resolve the release tag as today; download `router/files.manifest` for
   that tag; download `common` plus platform files; make executables per
   section 4.3.
4. Download daemon binaries by architecture (section 6.9); MIPS binaries are
   named `telegram-bot-mipsle` and `webui-mipsle`.
5. Generate the TLS certificate with `openssl` as today.
6. Cron and daemons via the init scripts as today.
7. Final message: Web UI URL from `platform_lan_ip`, login name per platform.

Build and CI:

- `server/Makefile`: `build-mipsle` and `build-webui-mipsle`
  (`GOOS=linux GOARCH=mipsle GOMIPS=softfloat CGO_ENABLED=0`).
- Top-level `Makefile`: `build-all` includes the `mipsle` targets.
- `.github/workflows/telegram-bot.yml`: builds and uploads the two `mipsle`
  assets alongside `arm64` and `arm`.

## 12. Configuration

`vpn-director.json` keeps its schema. Tunnel keys under
`tunnel_director.tunnels` are platform tunnel IDs: `wgc1`, `ovpnc3` on
Merlin; `OpenVPN0`, `Wireguard1` on Keenetic. One optional field is new:

```json
"tunnel_director": {
  "tunnels": {
    "OpenVPN0": {
      "clients": ["192.168.1.0/24"],
      "exclude": ["ru"],
      "gateway": "10.73.149.1"
    }
  }
}
```

`gateway` is read only on Keenetic for OpenVPN tunnels; elsewhere it is
ignored with a DEBUG log line. `config.sh` validates it as an IPv4 address.

## 13. Failure behaviour

| Situation | Behaviour |
|-----------|-----------|
| Unsupported platform | Every CLI command exits 1 with `unsupported platform`; the installer exits 1 before downloading. |
| `xt_TPROXY.ko` missing on Keenetic | Xray TPROXY apply fails with an ERROR naming the component; other components apply. `status` shows `TPROXY module not loaded`. |
| RCI unreachable | `platform_tunnels` prints only `main`; `platform` subcommand emits an empty tunnel list and a WARN; TD apply reports every configured tunnel as invalid, as it does today for an unknown table. |
| Tunnel interface down | `platform_tunnel_route_ensure` fails, WARN logged, the `ip rule` is installed anyway, traffic uses `main`. The next `ifstatechanged.d` call re-applies and installs the route. |
| NDM rebuild | Chains vanish; the `netfilter.d` hook re-applies within seconds. The gap equals the hook plus apply runtime, comparable to Merlin's `firewall-start`. |
| Package missing (`flock`, `ip-full`, …) | The installer refuses to proceed and names the packages; at runtime the existing "command not found" errors surface in the log. |
| Old Merlin binary after the rename | GitHub redirects the API and raw requests; the update completes with the new manifest. |

## 14. Testing

Bats (`router/test`):

- `VPD_PLATFORM` set by `test_helper.bash` (default `merlin`); platform
  detection tests unset it and stub the probe paths through `VPD_PROBE_ROOT`.
- New mocks: `ndmc`, `curl` (serves fixtures from
  `router/test/fixtures/keenetic/rci/<path>.json`), `insmod`, `cru`, `crond`.
- `unit/platform_merlin.bats`, `unit/platform_keenetic.bats`: every contract
  function, including the address-mismatch path of `platform_tunnel_iface`,
  the gateway computation and the `.ko` missing path.
- `unit/tunnel.bats`, `unit/tproxy.bats`: existing cases run under `merlin`;
  new cases under `keenetic` cover table `2000+idx` routes, PREROUTING
  position 2, the `mangle INPUT` accept rule, and `tunnel_stop` releasing
  tables.
- `unit/hooks.bats`: the three Keenetic hooks exit 0 without starting apply
  for foreign `$type`/`$table`/`$id`, and start it for matching ones
  (`nohup` and the CLI are mocked).
- `unit/install.bats`: platform detection, manifest parsing, prerequisite
  messages, architecture mapping.
- `integration/vpn_director.bats`: `platform` subcommand JSON shape on both
  platforms.

Go (`make -C server test`):

- `platform.Detect` on temporary directory trees.
- `PlatformInfo` decoding and the `Platform()` service call against the mock
  executor.
- `handler_clients` and wizard route validation with a fake service returning
  tunnels; 503 path when it fails.
- Updater: manifest download and filtering by tag (`httptest`), `mipsle`
  suffix, update script rendering with hooks.

Manual:

- Asus regression: `apply`, `status`, `update`, Web UI and bot on the
  author's Merlin router after the refactor.
- Keenetic: full run per section 15, then daily use.

## 15. Verification tasks on the device

These are the first tasks of the implementation plan. Each has a decision
rule so the design does not stall.

1. **TPROXY end to end.** Install `xray` on the Keenetic, apply the
   `mangle INPUT` accept rule, proxy one LAN device, open an HTTPS site.
   Pass: the site loads through Xray with the router web UI still on 443.
   Fail: the user chooses between moving the router's HTTPS port
   (`ip http ssl port 8443`) and a hybrid mode (TCP via `REDIRECT`, UDP via
   TPROXY); the design is amended accordingly.
2. **OpenVPN route form.** Compare `default dev ovpn_br0` with
   `default via 10.73.149.1 dev ovpn_br0` using marked ICMP and TCP. Pass for
   `dev` alone: drop the gateway computation and the `gateway` field. Otherwise
   keep section 6.5 as written.
3. **Hook variables.** Log the environment of one `wan.d` and one
   `ifstatechanged.d` call while toggling OpenVPN0. Confirm or correct the
   variable names in section 6.3.
4. **Table numbers.** Create a temporary Keenetic policy, read `ip rule`, and
   confirm its table number does not fall in `2000..2255`. If it does, move
   the base to a free range and update section 6.5.
5. **Wireguard mapping.** If a Wireguard interface is available, confirm
   `WireguardN` maps to `nwgN` and that `default dev nwgN` routes marked
   traffic.

## 16. Out of scope

- Guest segment `br1` and other LAN bridges.
- IKE, L2TP, SSTP, PPTP tunnels; Keenetic policies integration.
- Email notifications on Keenetic.
- IPv6 TPROXY.
- Hybrid (REDIRECT + TPROXY) mode, unless task 1 of section 15 fails.
- Testing MIPS builds on hardware.

## 17. Compatibility

- Merlin routers already installed: no config change, same file paths, same
  hooks. The rename is transparent through GitHub redirects; the first update
  after the rename installs the manifest-driven updater.
- The `wan_id` parameter removal in `firewall.sh` is internal; no script in
  the repository passes it.

## 18. Delivery order

The implementation plan follows this order so Merlin stays green at every
step and the device work happens while the code is still cheap to change:

1. Verification tasks of section 15 on the Keenetic.
2. Repository rename and module path change.
3. File manifest; installer and updater read it (Merlin only, no behaviour
   change).
4. Shell platform layer with the Merlin implementation; core modules moved to
   the contract; Bats green; Asus regression.
5. Keenetic shell implementation, hooks, `platform` subcommand; Keenetic
   end-to-end by hand.
6. Go `internal/platform`, `Platform()` service, `/api/platform`, route
   validation, Web UI and bot lists.
7. Installer for Keenetic, `mipsle` builds, CI, docs.
