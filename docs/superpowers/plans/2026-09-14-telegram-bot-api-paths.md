# Telegram Bot API Paths Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The Telegram bot reaches `api.telegram.org` by itself: prefer direct WAN, otherwise Xray SOCKS or a Tunnel Director tunnel, with no `proxy` setting.

**Architecture:** An in-process PathManager rebuilds candidates from `vpn-director.json` and `vpn-director.sh platform`, probes `getMe` through each path's dialer, and publishes the current path. The bot HTTP client snapshots that path per dial: `tcp4` for direct, SOCKS5 with the hostname for Xray, `SO_BINDTODEVICE` plus tunnel-bound DNS (`8.8.8.8`, then `1.1.1.1`) for a tunnel. `--dev` stays on a plain direct client.

**Tech Stack:** Go 1.25 (`server/`), `golang.org/x/net/proxy` (already required), `golang.org/x/sys/unix` (add for `BindToDevice`), Bats for the setup-script prologue only.

**Spec:** `docs/superpowers/specs/2026-09-14-telegram-bot-api-paths-design.md`

## Global Constraints

- Spec sections 1–13 are the source of truth. If this plan and the spec disagree, stop and ask; do not invent a third behaviour.
- Bot-only. Do not change Web UI, GitHub/updater HTTP, `/import`, or `ssrf.NewClient`.
- No new iptables, fwmark, NDM hooks, or `vpn-director.json` schema.
- IPv4 only on every Telegram path (`tcp4` / `ip4`). Dual-stack A+AAAA hangs on these routers.
- `http.Client` used by `tgbotapi` must **not** set `Timeout`: `getUpdates` long-polls for 60s. The 8s timeout belongs only on the probe client.
- Do not log probe URLs. The bot token sits in the path; the scrubber is not a reason to print it.
- Tests: from `server/` run `go test ./internal/bot/ ./internal/config/ -count=1` after Go tasks, and `go build ./... && go vet ./... && go test ./... -count=1` after Task 5. From the repository root the four-place bats suite stays green: `bats router/test/*.bats router/test/unit router/test/integration`. In the main session dispatch Go build/test through `claude-forge:build-runner`; Bats that agent cannot run. `gofmt -l server/` may list only `internal/wizard/handler.go` and `internal/ssrf/ssrf_test.go`.
- Work on branch `feature/telegram-bot-api-paths`. Stage files by name. Never stage `.claude/settings.local.json`, `.superpowers/`, `docs/session-transfer-*.md`, or other files under `docs/superpowers/` besides the spec/plan already on this branch.
- Commit messages in English. Do not copy a `Claude-Session:` trailer from another session.
- Do not SSH to either router, push, tag, open a PR, or merge. Device checks in spec §12.1 wait for the owner.
- The user writes Russian and expects Russian answers. Code, comments, commit messages, documentation and this plan stay in English.

## File Structure

Created:

| File | Responsibility |
|------|----------------|
| `server/internal/bot/path.go` | `Path`, `Path.String`, `Path.same`, `socksPort`, `candidates`, `selectPath` — pure, no I/O |
| `server/internal/bot/path_test.go` | Table tests for candidates and selection (spec §4, §6) |
| `server/internal/bot/pathmanager.go` | PathManager: cycle, `SelectOnce`, `Current`, `ReportFailure`, `Start`, `RegisterIdleCloser` |
| `server/internal/bot/pathmanager_test.go` | Cycle, stickiness, ReportFailure no-op, Platform() error |

Modified:

| File | Change |
|------|--------|
| `server/internal/bot/transport.go` | Add `DialPath`, `NewPathClient`, bind, tunnel DNS. Keep `NewHTTPClient` and `PermanentError` until Task 5 so `bot.go` still compiles. |
| `server/internal/bot/transport_test.go` | Add path-dial tests beside the existing `NewHTTPClient` tests. Task 5 deletes the old ones. |
| `server/internal/bot/bot.go` | Apply options first, build services, PathManager (unless `--dev`), then `NewBotAPIWithClient`. `Run` starts PathManager. |
| `server/internal/config/config.go` | Drop `Proxy` and `ProxyFallbackDirect` from `Config` and `rawConfig` |
| `server/internal/config/config_test.go` | Leftover `proxy` keys still load; stop asserting those fields |
| `router/opt/vpn-director/setup_telegram_bot.sh` | Remove the proxy prompt and those JSON keys |
| `.claude/rules/telegram-bot.md` | Config shape and transport behaviour |

Unchanged: `router/test/unit/entrypoints.bats` (prologue only), Web UI, updater, `ssrf`.

---

### Task 1: Path identity, candidates, selection

**Files:**
- Create: `server/internal/bot/path.go`
- Create: `server/internal/bot/path_test.go`

**Interfaces:**
- Consumes: `vpnconfig.VPNDirectorConfig`, `vpnconfig.TunnelConfig`, `vpnconfig.PlatformInfo`, `vpnconfig.PlatformTunnel`, `vpnconfig.XrayInboundPorts`
- Produces:

```go
const (
    kindNone pathKind = iota
    kindDirect
    kindSOCKS
    kindTunnel
)

type Path struct {
    kind      pathKind
    id        string // tunnel id when kindTunnel
    iface     string // bind device when kindTunnel
    socksPort int    // when kindSOCKS
}

func (p Path) String() string // "none" | "direct" | "socks" | "tunnel:<id>"
func (p Path) same(q Path) bool // kind + id; ignores iface and socksPort
func socksPort(cfg *vpnconfig.VPNDirectorConfig) int // 12346 when cfg is nil or XrayInboundPorts socks <= 0
func candidates(cfg *vpnconfig.VPNDirectorConfig, plat vpnconfig.PlatformInfo, socksUp bool) []Path
func selectPath(current Path, directLive, currentLive bool, replacement Path) Path
```

`candidates` always starts with `Path{kind: kindDirect}`. If `socksUp` it appends `Path{kind: kindSOCKS, socksPort: socksPort(cfg)}`. Then, for each id of `cfg.TunnelDirector.Tunnels` sorted by name: skip `main`; skip empty `Clients`; skip unless `plat` lists the id with `Connected == true` and non-empty `Iface`; append `Path{kind: kindTunnel, id, iface}`. A nil `cfg` yields only direct (and socks if `socksUp`, port 12346). An empty `plat.Tunnels` yields no tunnel paths.

`selectPath` is spec §6: live direct wins; else keep current when current is socks or tunnel and `currentLive`; else `replacement` (zero Path if none).

- [ ] **Step 1: Write the failing tests**

`server/internal/bot/path_test.go`:

```go
package bot

import (
    "testing"

    "github.com/zinin/vpn-director/server/internal/vpnconfig"
)

func TestPathString(t *testing.T) {
    if Path{}.String() != "none" {
        t.Fatalf("zero path: got %q", Path{}.String())
    }
    if (Path{kind: kindDirect}).String() != "direct" {
        t.Fatal("direct")
    }
    if (Path{kind: kindSOCKS}).String() != "socks" {
        t.Fatal("socks")
    }
    if (Path{kind: kindTunnel, id: "ovpnc2"}).String() != "tunnel:ovpnc2" {
        t.Fatal("tunnel")
    }
}

func TestSocksPort(t *testing.T) {
    if socksPort(nil) != 12346 {
        t.Fatalf("nil cfg: got %d", socksPort(nil))
    }
    empty := &vpnconfig.VPNDirectorConfig{}
    if socksPort(empty) != 12346 {
        t.Fatalf("missing advanced: got %d", socksPort(empty))
    }
    cfg := &vpnconfig.VPNDirectorConfig{
        Advanced: map[string]interface{}{"xray": map[string]interface{}{"socks_port": float64(23456)}},
    }
    if socksPort(cfg) != 23456 {
        t.Fatalf("configured: got %d", socksPort(cfg))
    }
    zero := &vpnconfig.VPNDirectorConfig{
        Advanced: map[string]interface{}{"xray": map[string]interface{}{"socks_port": float64(0)}},
    }
    if socksPort(zero) != 12346 {
        t.Fatalf("non-positive: got %d", socksPort(zero))
    }
}

func TestCandidates(t *testing.T) {
    cfg := &vpnconfig.VPNDirectorConfig{
        TunnelDirector: vpnconfig.TunnelDirectorConfig{
            Tunnels: map[string]vpnconfig.TunnelConfig{
                "main":    {Clients: []string{"192.168.1.3"}},
                "ovpnc2":  {Clients: []string{"192.168.1.3"}},
                "ovpnc3":  {Clients: []string{}},
                "wgc1":    {Clients: []string{"192.168.1.4"}},
                "OpenVPN0": {Clients: []string{"192.168.1.5"}},
            },
        },
    }
    plat := vpnconfig.PlatformInfo{Tunnels: []vpnconfig.PlatformTunnel{
        {ID: "ovpnc2", Iface: "tun12", Connected: true},
        {ID: "wgc1", Iface: "wgc1", Connected: false},
        {ID: "OpenVPN0", Iface: "", Connected: true},
    }}
    got := candidates(cfg, plat, true)
    want := []string{"direct", "socks", "tunnel:ovpnc2"}
    if len(got) != len(want) {
        t.Fatalf("len=%d names=%v", len(got), names(got))
    }
    for i, p := range got {
        if p.String() != want[i] {
            t.Fatalf("idx %d: got %s want %s (all %v)", i, p.String(), want[i], names(got))
        }
    }
    if got[2].iface != "tun12" || got[1].socksPort != 12346 {
        t.Fatalf("iface=%q socksPort=%d", got[2].iface, got[1].socksPort)
    }

    noSocks := candidates(cfg, plat, false)
    if len(noSocks) != 2 || noSocks[0].String() != "direct" || noSocks[1].String() != "tunnel:ovpnc2" {
        t.Fatalf("socks down: %v", names(noSocks))
    }

    if n := names(candidates(nil, plat, true)); len(n) != 2 || n[0] != "direct" || n[1] != "socks" {
        t.Fatalf("nil cfg: %v", n)
    }
    if n := names(candidates(cfg, vpnconfig.PlatformInfo{}, true)); len(n) != 2 {
        t.Fatalf("platform empty: %v", n)
    }
}

func TestSelectPath(t *testing.T) {
    direct := Path{kind: kindDirect}
    socks := Path{kind: kindSOCKS}
    tun := Path{kind: kindTunnel, id: "ovpnc2"}
    tunB := Path{kind: kindTunnel, id: "OpenVPN0"}

    if p := selectPath(tun, true, true, socks); !p.same(direct) {
        t.Fatalf("direct wins over live tunnel: %s", p)
    }
    if p := selectPath(socks, false, true, tun); !p.same(socks) {
        t.Fatalf("keep live socks while tunnel also live: %s", p)
    }
    if p := selectPath(tun, false, true, socks); !p.same(tun) {
        t.Fatalf("keep live tunnel: %s", p)
    }
    if p := selectPath(Path{}, false, false, socks); !p.same(socks) {
        t.Fatalf("no current, socks first: %s", p)
    }
    if p := selectPath(socks, false, false, tun); !p.same(tun) {
        t.Fatalf("dead socks, take replacement: %s", p)
    }
    if p := selectPath(direct, false, false, Path{}); p.String() != "none" {
        t.Fatalf("all dead: %s", p)
    }
    if p := selectPath(tunB, false, false, socks); !p.same(socks) {
        t.Fatalf("dead current, socks replacement: %s", p)
    }
}

func names(ps []Path) []string {
    out := make([]string, len(ps))
    for i, p := range ps {
        out[i] = p.String()
    }
    return out
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd server && go test ./internal/bot/ -count=1 -run 'TestPathString|TestSocksPort|TestCandidates|TestSelectPath'`

Expected: FAIL, `Path` undefined / `socksPort` undefined.

- [ ] **Step 3: Write the minimal implementation**

`server/internal/bot/path.go`:

```go
package bot

import (
    "sort"

    "github.com/zinin/vpn-director/server/internal/vpnconfig"
)

type pathKind int

const (
    kindNone pathKind = iota
    kindDirect
    kindSOCKS
    kindTunnel
)

const defaultSOCKSPort = 12346

type Path struct {
    kind      pathKind
    id        string
    iface     string
    socksPort int
}

func (p Path) String() string {
    switch p.kind {
    case kindDirect:
        return "direct"
    case kindSOCKS:
        return "socks"
    case kindTunnel:
        return "tunnel:" + p.id
    default:
        return "none"
    }
}

func (p Path) same(q Path) bool {
    return p.kind == q.kind && p.id == q.id
}

func socksPort(cfg *vpnconfig.VPNDirectorConfig) int {
    _, socks := vpnconfig.XrayInboundPorts(cfg)
    if socks <= 0 {
        return defaultSOCKSPort
    }
    return socks
}

func candidates(cfg *vpnconfig.VPNDirectorConfig, plat vpnconfig.PlatformInfo, socksUp bool) []Path {
    out := []Path{{kind: kindDirect}}
    if socksUp {
        out = append(out, Path{kind: kindSOCKS, socksPort: socksPort(cfg)})
    }
    if cfg == nil {
        return out
    }
    ids := make([]string, 0, len(cfg.TunnelDirector.Tunnels))
    for id := range cfg.TunnelDirector.Tunnels {
        ids = append(ids, id)
    }
    sort.Strings(ids)
    byID := make(map[string]vpnconfig.PlatformTunnel, len(plat.Tunnels))
    for _, t := range plat.Tunnels {
        byID[t.ID] = t
    }
    for _, id := range ids {
        if id == "main" {
            continue
        }
        tun := cfg.TunnelDirector.Tunnels[id]
        if len(tun.Clients) == 0 {
            continue
        }
        pt, ok := byID[id]
        if !ok || !pt.Connected || pt.Iface == "" {
            continue
        }
        out = append(out, Path{kind: kindTunnel, id: id, iface: pt.Iface})
    }
    return out
}

func selectPath(current Path, directLive, currentLive bool, replacement Path) Path {
    if directLive {
        return Path{kind: kindDirect}
    }
    if (current.kind == kindSOCKS || current.kind == kindTunnel) && currentLive {
        return current
    }
    return replacement
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd server && go test ./internal/bot/ -count=1 -run 'TestPathString|TestSocksPort|TestCandidates|TestSelectPath'`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/internal/bot/path.go server/internal/bot/path_test.go
git commit -m "feat(bot): select Telegram API path from candidates

Pure Path type, candidate filter (TD tunnels with clients, connected
iface, no main) and spec §6 selection: direct wins, equals stick."
```

---

### Task 2: DialPath and NewPathClient

**Files:**
- Modify: `server/internal/bot/transport.go` (add new APIs; keep `NewHTTPClient` until Task 5)
- Modify: `server/internal/bot/transport_test.go` (append; keep `TestNewHTTPClient_*` until Task 5)
- Modify: `server/go.mod` / `server/go.sum` (`golang.org/x/sys`)

**Interfaces:**
- Consumes: `Path` from Task 1
- Produces:

```go
type PathSource interface {
    Current() Path
    ReportFailure(Path)
}

type IdleCloser interface {
    RegisterIdleCloser(func())
}

func DialPath(ctx context.Context, p Path, network, addr string) (net.Conn, error)
func NewPathClient(src PathSource) *http.Client
func socksListening(port int) bool
func isPathFailure(err error) bool
```

`DialPath` snapshots nothing; the caller passes `p`. Behaviour:

- `kindDirect` / `kindNone`: `lookupIPv4` the host (skip lookup when host is already an IPv4), dial `tcp4`. `kindNone` still returns `errNoPath` (`errors.New("telegram API unreachable on every path")`) without dialing.
- `kindSOCKS`: SOCKS5 to `127.0.0.1:<p.socksPort>` via `golang.org/x/net/proxy`. The target passed to SOCKS is the original `addr` (hostname kept). Network forced to `tcp`.
- `kindTunnel`: `lookupIPv4` through a `net.Resolver` whose `Dial` binds `p.iface` and connects to `8.8.8.8:53` (`udp4`/`tcp4`), then `1.1.1.1:53` on failure — ignore the resolver's `address` argument so `resolv.conf` is not used. Then `tcp4` dial with `Control` = `SO_BINDTODEVICE` on `p.iface`. Bind error is returned to the caller (dead path), not fatal to the process.

Production hooks live on an unexported `pathDialer` (`lookupIPv4`, `bindControl`, `socksDial`) so tests inject fakes. `bindControl` uses `golang.org/x/sys/unix.BindToDevice`.

`NewPathClient` clones `http.DefaultTransport`, sets `Proxy` to a function that returns `(nil, nil)` (ignore env), sets `DialContext` to snapshot `src.Current()` and call `DialPath`. The wrapper `RoundTrip` calls `src.ReportFailure(p)` when `isPathFailure(err)` after a dial/TLS error, using the same `p` that was snapshotted for that request (store it on the request context). If `src` also implements `IdleCloser`, register `transport.CloseIdleConnections`. Do **not** set `http.Client.Timeout`.

`socksListening`: `net.DialTimeout("tcp", "127.0.0.1:<port>", time.Second)`; close on success.

`isPathFailure`: true for `net.Error` (including timeouts) and `*net.OpError`; unwrap `*url.Error`. False for nil.

Keep `PermanentError`, `NewHTTPClient`, `fallbackTransport`, and `isDialError` so `bot.go` still compiles. Task 5 deletes them.

- [ ] **Step 1: Add the module and write the failing tests**

```bash
cd server && go get golang.org/x/sys/unix
```

Append to `server/internal/bot/transport_test.go` (keep the existing `TestNewHTTPClient_*` tests). Merge the new imports (`context`, `errors`, `io`, `net`, `syscall`, `time`) into the existing import block:

```go
package bot

import (
    "context"
    "errors"
    "io"
    "net"
    "net/http"
    "net/http/httptest"
    "syscall"
    "testing"
    "time"
)

type fakeSource struct {
    p      Path
    failed []Path
    idle   []func()
}

func (f *fakeSource) Current() Path            { return f.p }
func (f *fakeSource) ReportFailure(p Path)     { f.failed = append(f.failed, p) }
func (f *fakeSource) RegisterIdleCloser(fn func()) { f.idle = append(f.idle, fn) }

func TestDialPath_None(t *testing.T) {
    _, err := DialPath(context.Background(), Path{}, "tcp", "example.com:443")
    if !errors.Is(err, errNoPath) {
        t.Fatalf("got %v", err)
    }
}

func TestDialPath_DirectTCP4(t *testing.T) {
    ln, err := net.Listen("tcp4", "127.0.0.1:0")
    if err != nil {
        t.Fatal(err)
    }
    defer ln.Close()
    done := make(chan struct{})
    go func() {
        defer close(done)
        c, err := ln.Accept()
        if err != nil {
            return
        }
        c.Close()
    }()
    conn, err := DialPath(context.Background(), Path{kind: kindDirect}, "tcp", ln.Addr().String())
    if err != nil {
        t.Fatal(err)
    }
    conn.Close()
    select {
    case <-done:
    case <-time.After(2 * time.Second):
        t.Fatal("accept timed out")
    }
}

func TestPathDialer_SOCKSKeepsHostname(t *testing.T) {
    var gotAddr string
    d := pathDialer{
        socksDial: func(ctx context.Context, port int, network, addr string) (net.Conn, error) {
            gotAddr = addr
            if port != 12346 || network != "tcp" {
                t.Errorf("port=%d network=%s", port, network)
            }
            c1, c2 := net.Pipe()
            c2.Close()
            return c1, nil
        },
    }
    p := Path{kind: kindSOCKS, socksPort: 12346}
    conn, err := d.dial(context.Background(), p, "tcp", "api.telegram.org:443")
    if err != nil {
        t.Fatal(err)
    }
    conn.Close()
    if gotAddr != "api.telegram.org:443" {
        t.Fatalf("SOCKS target %q", gotAddr)
    }
}

func TestPathDialer_TunnelBindsIface(t *testing.T) {
    var gotIface string
    d := pathDialer{
        lookupIPv4: func(ctx context.Context, host string) ([]net.IP, error) {
            return []net.IP{net.IPv4(1, 2, 3, 4)}, nil
        },
        bindControl: func(iface string) func(network, address string, c syscall.RawConn) error {
            gotIface = iface
            return func(network, address string, c syscall.RawConn) error { return nil }
        },
        // dialTCP is not injected; use a listening socket on 1.2.3.4? that won't work.
        // Instead record bindControl and fail the connect: we only assert iface.
        tcpDial: func(ctx context.Context, network, addr string, control func(string, string, syscall.RawConn) error) (net.Conn, error) {
            if network != "tcp4" {
                t.Errorf("network %s", network)
            }
            if control != nil {
                // invoke so bindControl's closure ran when building control
            }
            return nil, errors.New("dial skipped")
        },
    }
    p := Path{kind: kindTunnel, id: "ovpnc2", iface: "tun12"}
    _, _ = d.dial(context.Background(), p, "tcp", "api.telegram.org:443")
    if gotIface != "tun12" {
        t.Fatalf("iface %q", gotIface)
    }
}

func TestNewPathClient_ReportsDialFailure(t *testing.T) {
    src := &fakeSource{p: Path{kind: kindDirect}}
    client := NewPathClient(src)
    // Nothing listens here; DialPath to this host:port fails.
    req, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:1/", nil)
    _, err := client.Do(req)
    if err == nil {
        t.Fatal("expected dial error")
    }
    if len(src.failed) != 1 || !src.failed[0].same(src.p) {
        t.Fatalf("failures %+v", src.failed)
    }
}

func TestNewPathClient_RegistersIdleCloser(t *testing.T) {
    src := &fakeSource{p: Path{kind: kindDirect}}
    _ = NewPathClient(src)
    if len(src.idle) != 1 {
        t.Fatalf("idle closers %d", len(src.idle))
    }
}

func TestNewPathClient_NoClientTimeout(t *testing.T) {
    c := NewPathClient(&fakeSource{p: Path{kind: kindDirect}})
    if c.Timeout != 0 {
        t.Fatalf("Timeout=%s; getUpdates long-polls", c.Timeout)
    }
}

func TestSocksListening(t *testing.T) {
    ln, err := net.Listen("tcp", "127.0.0.1:0")
    if err != nil {
        t.Fatal(err)
    }
    defer ln.Close()
    port := ln.Addr().(*net.TCPAddr).Port
    if !socksListening(port) {
        t.Fatal("expected listening")
    }
    if socksListening(1) {
        t.Fatal("port 1 should be down")
    }
}

func TestIsPathFailure(t *testing.T) {
    if isPathFailure(nil) {
        t.Fatal("nil")
    }
    op := &net.OpError{Op: "dial", Err: errors.New("refused")}
    if !isPathFailure(op) {
        t.Fatal("op")
    }
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        io.WriteString(w, "ok")
    }))
    defer srv.Close()
    resp, err := http.Get(srv.URL)
    if err != nil {
        t.Fatal(err)
    }
    resp.Body.Close()
    if isPathFailure(err) {
        t.Fatal("success is not a path failure")
    }
}
```

The tunnel test needs a `tcpDial` hook on `pathDialer` so CI does not open a real `1.2.3.4:443`. Add that field in Step 3:

```go
type pathDialer struct {
    lookupIPv4  func(ctx context.Context, host string) ([]net.IP, error)
    bindControl func(iface string) func(network, address string, c syscall.RawConn) error
    socksDial   func(ctx context.Context, port int, network, addr string) (net.Conn, error)
    tcpDial     func(ctx context.Context, network, addr string, control func(network, address string, c syscall.RawConn) error) (net.Conn, error)
    dnsDial     func(ctx context.Context, network, address string) (net.Conn, error) // optional; production builds tunnel resolver
}
```

When `tcpDial` is nil, production uses `net.Dialer{Control: control}.DialContext`. When `socksDial` is nil, production uses `proxy.SOCKS5`. When `lookupIPv4` is nil, production uses `net.DefaultResolver.LookupIP(ctx, "ip4", host)`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd server && go test ./internal/bot/ -count=1 -run 'TestDialPath_|TestPathDialer_|TestNewPathClient_|TestSocksListening|TestIsPathFailure'`

Expected: FAIL, `DialPath` / `NewPathClient` undefined. `TestNewHTTPClient_*` still pass.

- [ ] **Step 3: Implement transport.go**

Keep `PermanentError` and `NewHTTPClient` as they are today. Append `errNoPath`, `pathDialer`, `DialPath` (delegates to `productionDialer.dial`), `NewPathClient`, `socksListening`, `isPathFailure`, and a tunnel resolver Dial that ignores its `address` parameter and tries `8.8.8.8:53` then `1.1.1.1:53` with `bindControl(iface)`.

Context key for the snapshotted path:

```go
type pathCtxKey struct{}

func (t *pathTransport) RoundTrip(req *http.Request) (*http.Response, error) {
    p := t.src.Current()
    req = req.WithContext(context.WithValue(req.Context(), pathCtxKey{}, p))
    resp, err := t.base.RoundTrip(req)
    if err != nil && isPathFailure(err) {
        t.src.ReportFailure(p)
    }
    return resp, err
}

func (t *pathTransport) dial(ctx context.Context, network, addr string) (net.Conn, error) {
    p, _ := ctx.Value(pathCtxKey{}).(Path)
    if p.kind == kindNone {
        p = t.src.Current()
    }
    conn, err := DialPath(ctx, p, network, addr)
    if err != nil {
        t.src.ReportFailure(p)
    }
    return conn, err
}
```

Double `ReportFailure` on a dial error is OK: Task 4 makes a second call a no-op while a cycle runs, and a same-path call is cheap.

`bindToDeviceControl`:

```go
func bindToDeviceControl(iface string) func(network, address string, c syscall.RawConn) error {
    return func(network, address string, c syscall.RawConn) error {
        var sockErr error
        if err := c.Control(func(fd uintptr) {
            sockErr = unix.BindToDevice(int(fd), iface)
        }); err != nil {
            return err
        }
        return sockErr
    }
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd server && go test ./internal/bot/ -count=1`

Expected: PASS (Task 1 tests plus Task 2). `gofmt` the new files.

- [ ] **Step 5: Commit**

```bash
git add server/internal/bot/transport.go server/internal/bot/transport_test.go server/go.mod server/go.sum
git commit -m "feat(bot): dial Telegram through direct, SOCKS, or bound iface

Replace the static SOCKS URL client. DialPath keeps the hostname for
SOCKS, uses tcp4 otherwise, and binds SO_BINDTODEVICE for tunnels.
DNS on a tunnel path queries 8.8.8.8 then 1.1.1.1 on that iface."
```

---

### Task 3: Probe

**Files:**
- Modify: `server/internal/bot/path.go` (add `probePath` and the API URL helper) **or** create `server/internal/bot/probe.go` if `path.go` would mix I/O with pure functions — prefer `probe.go`.
- Create: `server/internal/bot/probe_test.go`

**Interfaces:**
- Consumes: `Path`, `DialPath` from Tasks 1–2
- Produces:

```go
const probeTimeout = 8 * time.Second

func probePath(ctx context.Context, apiBase, token string, p Path) error
```

`apiBase` default in PathManager will be `https://api.telegram.org`. `probePath` builds `GET {apiBase}/bot{token}/getMe` with `context.WithTimeout(ctx, probeTimeout)` if `ctx` has no deadline, uses an `http.Client{Timeout: probeTimeout, Transport: &http.Transport{DialContext: func(...) { return DialPath(ctx, p, network, addr) }, Proxy: none}}`. Any HTTP response (close the body) returns nil — 401, 429, 5xx included. Dial/timeout/TLS errors return the error. Do not log the URL.

- [ ] **Step 1: Write the failing test**

```go
package bot

import (
    "context"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"
)

func TestProbePath_AnyHTTPIsLive(t *testing.T) {
    codes := []int{200, 401, 429, 500}
    for _, code := range codes {
        t.Run(http.StatusText(code), func(t *testing.T) {
            srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                if r.URL.Path != "/botTOKEN/getMe" {
                    t.Errorf("path %s", r.URL.Path)
                }
                w.WriteHeader(code)
            }))
            t.Cleanup(srv.Close)
            err := probePath(context.Background(), srv.URL, "TOKEN", Path{kind: kindDirect})
            if err != nil {
                t.Fatalf("code %d: %v", code, err)
            }
        })
    }
}

func TestProbePath_DialErrorIsDead(t *testing.T) {
    ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
    defer cancel()
    err := probePath(ctx, "http://127.0.0.1:1", "TOKEN", Path{kind: kindDirect})
    if err == nil {
        t.Fatal("expected error")
    }
}
```

httptest is HTTP, not TLS. `probePath` must use `apiBase` as given (no forced https) so tests can use `httptest.NewServer`. Production PathManager passes `https://api.telegram.org`.

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd server && go test ./internal/bot/ -count=1 -run TestProbePath`

Expected: FAIL, `probePath` undefined.

- [ ] **Step 3: Implement probe.go**

```go
package bot

import (
    "context"
    "fmt"
    "net"
    "net/http"
    "net/url"
    "time"
)

const probeTimeout = 8 * time.Second

func probePath(ctx context.Context, apiBase, token string, p Path) error {
    if _, ok := ctx.Deadline(); !ok {
        var cancel context.CancelFunc
        ctx, cancel = context.WithTimeout(ctx, probeTimeout)
        defer cancel()
    }
    client := &http.Client{
        Timeout: probeTimeout,
        Transport: &http.Transport{
            Proxy: func(*http.Request) (*url.URL, error) { return nil, nil },
            DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
                return DialPath(ctx, p, network, addr)
            },
        },
    }
    u := fmt.Sprintf("%s/bot%s/getMe", apiBase, token)
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
    if err != nil {
        return err
    }
    resp, err := client.Do(req)
    if err != nil {
        return err
    }
    resp.Body.Close()
    return nil
}
```

- [ ] **Step 4: Run tests**

Run: `cd server && go test ./internal/bot/ -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/internal/bot/probe.go server/internal/bot/probe_test.go
git commit -m "feat(bot): probe Telegram getMe through a path dialer

Any HTTP status counts as live, including 401, so a bad token is not
mistaken for an unreachable network."
```

---

### Task 4: PathManager cycle

**Files:**
- Create: `server/internal/bot/pathmanager.go`
- Create: `server/internal/bot/pathmanager_test.go`

**Interfaces:**
- Consumes: `candidates`, `selectPath`, `socksPort`, `Path`, `probePath`, `socksListening`, `NewPathClient`, `PathSource`, `IdleCloser`
- Produces:

```go
const defaultPathInterval = 30 * time.Second

type PathManager struct { /* unexported fields */ }

type PathManagerConfig struct {
    Token        string
    LoadVPN      func() (*vpnconfig.VPNDirectorConfig, error)
    LoadPlatform func() (vpnconfig.PlatformInfo, error)
    Listening    func(port int) bool                       // nil => socksListening
    Probe        func(ctx context.Context, p Path) error   // nil => probePath(ctx, apiBase, token, p)
    APIBase      string                                    // empty => https://api.telegram.org
    Interval     time.Duration                             // 0 => 30s
}

func NewPathManager(cfg PathManagerConfig) *PathManager
func (m *PathManager) Current() Path
func (m *PathManager) SelectOnce(ctx context.Context)
func (m *PathManager) ReportFailure(p Path)
func (m *PathManager) Start(ctx context.Context) // ticker until ctx done; blocking
func (m *PathManager) RegisterIdleCloser(fn func())
```

`PathManager` implements `PathSource` and `IdleCloser`.

`SelectOnce` cycle (spec §6.1), under a mutex so two cycles do not interleave:

1. `cfg, err := LoadVPN()`. On error, `cfg == nil` (default SOCKS port, no tunnel keys).
2. `plat, err := LoadPlatform()`. On error, empty `PlatformInfo` (no tunnels).
3. `port := socksPort(cfg)`; `socksUp := Listening(port)`.
4. `cands := candidates(cfg, plat, socksUp)`.
5. Probe `direct` (always).
6. If current is socks or tunnel and still in `cands`, probe current (liveness). If current disappeared from `cands`, treat `currentLive` as false.
7. Need replacement iff direct is dead and (no current or current not live). Then walk `cands` in order skipping `direct` and skipping the current path already probed; probe each until one is live; that is `replacement`. Stop at the first live.
8. `next := selectPath(current, directLive, currentLive, replacement)`.
9. If `!next.same(current)`: log INFO `Telegram API path selected` with `from` and `to`; if leaving `direct` for socks/tunnel, also WARN `Telegram API unreachable on WAN, using backup path`. Call every registered idle closer. Store `next`.
10. If `next` is zero: WARN `Telegram API unreachable on every path` with a `tried` slice of `path=reason` strings. Do not log tokens.

`ReportFailure(p)`: if `!p.same(Current())`, return. If a cycle is already running, return. Else start `SelectOnce` on a background goroutine with `context.WithTimeout(context.Background(), 30*time.Second)`.

Logging DEBUG per probe: `Telegram API path probe` `path` `live` (bool). Never the URL.

- [ ] **Step 1: Write the failing tests**

```go
package bot

import (
    "context"
    "errors"
    "sync"
    "testing"
    "time"

    "github.com/zinin/vpn-director/server/internal/vpnconfig"
)

func testMgr(t *testing.T, live map[string]bool, cfg *vpnconfig.VPNDirectorConfig, plat vpnconfig.PlatformInfo, socksUp bool) *PathManager {
    t.Helper()
    var mu sync.Mutex
    return NewPathManager(PathManagerConfig{
        Token: "TOKEN",
        LoadVPN: func() (*vpnconfig.VPNDirectorConfig, error) {
            if cfg == nil {
                return nil, errors.New("no config")
            }
            return cfg, nil
        },
        LoadPlatform: func() (vpnconfig.PlatformInfo, error) { return plat, nil },
        Listening:    func(port int) bool { return socksUp },
        Probe: func(ctx context.Context, p Path) error {
            mu.Lock()
            defer mu.Unlock()
            if live[p.String()] {
                return nil
            }
            return errors.New("dead")
        },
        APIBase:  "http://127.0.0.1:1",
        Interval: time.Hour,
    })
}

func tdCfg() *vpnconfig.VPNDirectorConfig {
    return &vpnconfig.VPNDirectorConfig{
        TunnelDirector: vpnconfig.TunnelDirectorConfig{
            Tunnels: map[string]vpnconfig.TunnelConfig{
                "ovpnc2": {Clients: []string{"192.168.1.3"}},
            },
        },
    }
}

func tdPlat() vpnconfig.PlatformInfo {
    return vpnconfig.PlatformInfo{Tunnels: []vpnconfig.PlatformTunnel{
        {ID: "ovpnc2", Iface: "tun12", Connected: true},
    }}
}

func TestPathManager_DirectWinsAndSwitchesBack(t *testing.T) {
    live := map[string]bool{"direct": false, "socks": true, "tunnel:ovpnc2": true}
    m := testMgr(t, live, tdCfg(), tdPlat(), true)
    m.SelectOnce(context.Background())
    if m.Current().String() != "socks" {
        t.Fatalf("want socks, got %s", m.Current())
    }
    live["direct"] = true
    m.SelectOnce(context.Background())
    if m.Current().String() != "direct" {
        t.Fatalf("switch back: %s", m.Current())
    }
}

func TestPathManager_EqualsStick(t *testing.T) {
    live := map[string]bool{"direct": false, "socks": true, "tunnel:ovpnc2": true}
    m := testMgr(t, live, tdCfg(), tdPlat(), true)
    m.SelectOnce(context.Background())
    if m.Current().String() != "socks" {
        t.Fatalf("got %s", m.Current())
    }
    m.SelectOnce(context.Background())
    if m.Current().String() != "socks" {
        t.Fatalf("flapped to %s", m.Current())
    }
}

func TestPathManager_PlatformErrorSkipsTunnels(t *testing.T) {
    m := NewPathManager(PathManagerConfig{
        LoadVPN: func() (*vpnconfig.VPNDirectorConfig, error) { return tdCfg(), nil },
        LoadPlatform: func() (vpnconfig.PlatformInfo, error) {
            return vpnconfig.PlatformInfo{}, errors.New("rci down")
        },
        Listening: func(int) bool { return false },
        Probe: func(ctx context.Context, p Path) error {
            if p.kind == kindTunnel {
                t.Fatal("must not probe tunnels when platform fails")
            }
            return errors.New("dead")
        },
    })
    m.SelectOnce(context.Background())
    if m.Current().String() != "none" {
        t.Fatalf("got %s", m.Current())
    }
}

func TestPathManager_ReportFailureNoopIfStale(t *testing.T) {
    live := map[string]bool{"direct": true}
    m := testMgr(t, live, tdCfg(), tdPlat(), false)
    m.SelectOnce(context.Background())
    if m.Current().String() != "direct" {
        t.Fatal(m.Current())
    }
    m.ReportFailure(Path{kind: kindTunnel, id: "ovpnc2"})
    time.Sleep(50 * time.Millisecond)
    if m.Current().String() != "direct" {
        t.Fatalf("stale failure flapped to %s", m.Current())
    }
}

func TestPathManager_ReportFailureReselects(t *testing.T) {
    live := map[string]bool{"direct": false, "socks": true, "tunnel:ovpnc2": true}
    m := testMgr(t, live, tdCfg(), tdPlat(), true)
    m.SelectOnce(context.Background())
    if m.Current().String() != "socks" {
        t.Fatal(m.Current())
    }
    live["socks"] = false
    m.ReportFailure(m.Current())
    deadline := time.Now().Add(2 * time.Second)
    for time.Now().Before(deadline) {
        if m.Current().String() == "tunnel:ovpnc2" {
            return
        }
        time.Sleep(10 * time.Millisecond)
    }
    t.Fatalf("still %s", m.Current())
}

func TestPathManager_ClosesIdleOnChange(t *testing.T) {
    live := map[string]bool{"direct": true}
    m := testMgr(t, live, tdCfg(), tdPlat(), false)
    n := 0
    m.RegisterIdleCloser(func() { n++ })
    m.SelectOnce(context.Background()) // none -> direct
    if n != 1 {
        t.Fatalf("first select closes idle: %d", n)
    }
    live["direct"] = false
    live["tunnel:ovpnc2"] = true
    // socks down; replacement is tunnel
    m.SelectOnce(context.Background())
    if n != 2 {
        t.Fatalf("path change: %d", n)
    }
}
```

For `TestPathManager_ClosesIdleOnChange`, `Listening` is false so socks is not a candidate; after direct dies the replacement is `tunnel:ovpnc2` if Probe uses `live`. Update `testMgr` to read `live` at probe time (already does). First cycle: direct live → current direct, idle closer once. Second: direct dead, current (direct) is not socks/tunnel so skip current liveness; need replacement; probe tunnel live.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd server && go test ./internal/bot/ -count=1 -run TestPathManager`

Expected: FAIL, `NewPathManager` undefined.

- [ ] **Step 3: Implement pathmanager.go**

Follow the cycle above. `Current` copies under the mutex. `Start`:

```go
func (m *PathManager) Start(ctx context.Context) {
    t := time.NewTicker(m.interval)
    defer t.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-t.C:
            m.SelectOnce(ctx)
        }
    }
}
```

Default Probe closure:

```go
func(ctx context.Context, p Path) error {
    return probePath(ctx, m.apiBase, m.token, p)
}
```

- [ ] **Step 4: Run tests**

Run: `cd server && go test ./internal/bot/ -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/internal/bot/pathmanager.go server/internal/bot/pathmanager_test.go
git commit -m "feat(bot): PathManager probes and sticks to a working path

Prefer direct, keep a live SOCKS or tunnel, reselect on ReportFailure
only when that path is still current."
```

---

### Task 5: Wire the bot, drop proxy config, docs

**Files:**
- Modify: `server/internal/bot/bot.go`
- Modify: `server/internal/config/config.go`
- Modify: `server/internal/config/config_test.go`
- Modify: `router/opt/vpn-director/setup_telegram_bot.sh`
- Modify: `.claude/rules/telegram-bot.md`

**Interfaces:**
- Consumes: `NewPathManager`, `PathManagerConfig`, `NewPathClient`, `SelectOnce`, `Start` from Task 4; `service.ConfigService.LoadVPNConfig`, `service.VPNDirectorService.Platform`
- Produces: bot that reaches Telegram without `cfg.Proxy`

`bot.New` today builds the HTTP client **before** options and services. Reorder:

1. `b := &Bot{version: version}`
2. Apply `opts` (so `devMode` and `executor` exist).
3. `configSvc := service.NewConfigService(...)`; `vpnSvc := service.NewVPNDirectorService(p.ScriptsDir, b.executor)`; the other services as today.
4. HTTP client:
   - if `b.devMode`: `&http.Client{}` (plain direct; no PathManager).
   - else: `pm := NewPathManager(PathManagerConfig{Token: cfg.BotToken, LoadVPN: configSvc.LoadVPNConfig, LoadPlatform: vpnSvc.Platform})`; `pm.SelectOnce(context.Background())`; `b.pathManager = pm`; `httpClient = NewPathClient(pm)`.
5. `tgbotapi.NewBotAPIWithClient` as today. 401 still `PermanentError`. Network errors still return unwrapped for `main.go` retry. Do **not** wrap PathManager construction in `PermanentError`.
6. Rest of `New` unchanged (handlers, router). `configSvc`/`vpnSvc` are already created — reuse those variables; do not construct them twice.

Add `pathManager *PathManager` to `Bot`. In `Run`, immediately after the function starts (before notify is fine):

```go
if b.pathManager != nil {
    go b.pathManager.Start(ctx)
}
```

`ctx` cancel on shutdown stops the ticker. Do not `Start` inside `New`: a failed `NewBotAPIWithClient` would leak a goroutine on every `main.go` retry.

Config: delete `Proxy` and `ProxyFallbackDirect` from `Config` and `rawConfig`. `Load` no longer copies them. JSON leftover keys are ignored.

`TestLoad_WithProxy` becomes: Load a file that still has those keys; assert `err == nil` and `cfg.BotToken == "test-token"`. Delete `TestLoad_ProxyDefaults` (no fields left) or fold into an existing load test.

`setup_telegram_bot.sh`: delete the block from `# Proxy configuration` through the `echo "Proxy: ..."` branch (lines 67–89). `jq` becomes:

```bash
jq -n \
    --arg token "$BOT_TOKEN" \
    --argjson users "$USERS_JSON" \
    '{bot_token: $token, allowed_users: $users, log_level: "info", update_check_interval: "24h"}' > "$CONFIG_FILE"
```

`.claude/rules/telegram-bot.md`:

- Architecture tree: add `path.go`, `pathmanager.go`, `transport.go` under `internal/bot/`.
- Config JSON example: drop `proxy` and `proxy_fallback_direct`.
- Remove the two field bullets for those keys.
- Add a short **Telegram API transport** subsection: the bot always probes direct, then Xray SOCKS on `advanced.xray.socks_port` (default 12346) if that port listens, then each `tunnel_director.tunnels` key except `main` that has clients and a connected platform iface; `SO_BINDTODEVICE`; `--dev` is direct only; leftover `proxy` keys in old JSON are ignored.

- [ ] **Step 1: Write the failing config test and compile check**

In `config_test.go` replace `TestLoad_WithProxy` / `TestLoad_ProxyDefaults` with:

```go
func TestLoad_IgnoresLegacyProxyKeys(t *testing.T) {
    tmpDir := t.TempDir()
    configPath := filepath.Join(tmpDir, "config.json")
    jsonContent := `{
        "bot_token": "test-token",
        "allowed_users": ["user1"],
        "proxy": "socks5://127.0.0.1:12346",
        "proxy_fallback_direct": true
    }`
    if err := os.WriteFile(configPath, []byte(jsonContent), 0644); err != nil {
        t.Fatal(err)
    }
    cfg, err := Load(configPath)
    if err != nil {
        t.Fatal(err)
    }
    if cfg.BotToken != "test-token" {
        t.Fatalf("token %q", cfg.BotToken)
    }
}
```

This test already passes today (Load ignores unknown fields only after we drop the struct tags — **today it still stores them**). After dropping the fields it must still pass. Run it now (passes), then drop the fields: if anything still references `cfg.Proxy` it fails to compile — that is the gate.

- [ ] **Step 2: Drop proxy fields; fix compile**

Remove the fields. Run `cd server && go test ./internal/config/ -count=1` — PASS. Run `cd server && go build ./...` — FAIL on `bot.go` `cfg.Proxy` until Step 3.

- [ ] **Step 3: Wire bot.go, delete the old client, setup script, docs**

Implement the reorder described above. `New` needs `"net/http"`.

Delete from `transport.go`: `NewHTTPClient`, `fallbackTransport`, `isDialError`. Delete `TestNewHTTPClient_*` from `transport_test.go`. Grep the repo for `NewHTTPClient` and `cfg.Proxy` — only this task's files should have matched, and after the edit none should.

Grep `setup_telegram_bot.sh` after the edit: no `proxy` string except possibly comments, which there should not be.

- [ ] **Step 4: Full verification**

```bash
cd server && gofmt -w internal/bot/*.go internal/config/config.go internal/config/config_test.go
cd server && go build ./... && go vet ./... && go test ./... -count=1
gofmt -l server/   # only wizard/handler.go and ssrf/ssrf_test.go
cd /opt/github/zinin/asuswrt-merlin-vpn-director
bats router/test/*.bats router/test/unit router/test/integration
```

Expected: Go all green. Bats 528/528 (or whatever the suite currently is). `entrypoints.bats` still passes (prologue of `setup_telegram_bot.sh` unchanged).

- [ ] **Step 5: Commit**

```bash
git add server/internal/bot/bot.go \
        server/internal/bot/transport.go \
        server/internal/bot/transport_test.go \
        server/internal/config/config.go \
        server/internal/config/config_test.go \
        router/opt/vpn-director/setup_telegram_bot.sh \
        .claude/rules/telegram-bot.md
git commit -m "feat(bot): pick a live Telegram path at startup

Wire PathManager into bot.New, drop proxy settings from telegram-bot.json
and setup_telegram_bot.sh, and leave leftover proxy keys ignored."
```

---

## After implementation (not a coding task)

Do not SSH anywhere until the owner asks. Then, with permission:

1. RT-AX86U: Telegram blocked on WAN, Xray outbound dead, `ovpnc2` in Tunnel Director. Bot leaves the startup retry loop. Log contains `to=tunnel:ovpnc2` (or `socks` if Xray outbound has recovered). No edit to `telegram-bot.json` required.
2. KN-4521: `SO_BINDTODEVICE` on `ovpn_brN` if that tunnel is in Tunnel Director.

Before a PR: `git rm` everything under `docs/superpowers/` and commit that removal so the spec and plan do not appear in the PR diff. They remain in branch history.

---

## Spec coverage

| Spec | Task |
|------|------|
| §1 goal, bot-only | Task 5 (no other HTTP clients touched) |
| §2 userspace bind, no fwmark | Task 2 |
| §3 PathManager + Transport + Discovery, `--dev` direct | Tasks 4–5 |
| §3.1 path identity | Task 1 |
| §4 candidates, default 12346, pause keeps clients | Task 1 |
| §5 getMe, any HTTP live, 8s, no URL log | Task 3 |
| §6 selection | Task 1 `selectPath`, Task 4 cycle |
| §6.1 30s, ReportFailure, no RoundTrip retry, CloseIdleConnections | Task 4 |
| §7 Dial/DNS/IPv4/ssrf out | Task 2 |
| §8 config + setup script | Task 5 |
| §9 logging | Task 4 |
| §10 out of scope | Global constraints |
| §11 files | File structure |
| §12 tests | Tasks 1–5 |
| §12.1 device | After implementation |
| §13 success | After implementation |
