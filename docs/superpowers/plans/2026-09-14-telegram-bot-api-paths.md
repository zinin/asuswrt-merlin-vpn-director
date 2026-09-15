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

Also created in implementation: `server/internal/bot/probe.go`, `probe_test.go` (Task 3).

---

### Task 1: Path identity, candidates, selection

✅ Done — see commit(s): `c260faa`

### Task 2: DialPath and NewPathClient

✅ Done — see commit(s): `5ffbe05`

### Task 3: Probe

✅ Done — see commit(s): `612b56f`

### Task 4: PathManager cycle

✅ Done — see commit(s): `1384ed6`, `3e42af1`

### Task 5: Wire the bot, drop proxy config, docs

✅ Done — see commit(s): `491be9d`

### Post-review: production Dialer Timeout

✅ Done — see commit(s): `9f49388`

### Post-review: connection pool, tunnel DNS fallback, sticky params, darwin bind

✅ Done — see commit(s): `0414e5d`

### Post-review: bind path to transport generation and report TLS EOF

✅ Done — see commit(s): `b81c916`

### Post-review: tunnel SO_MARK, failover budget, handshake race, ClientTrace

✅ Done — see commit(s): `9070c48`

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
| §2 userspace bind, no fwmark | Task 2; later `9070c48` reuses the existing TD mark (`SO_MARK`) — owner-approved, not a new iptables chain |
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
