# VLESS REALITY/Vision Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Parse and persist per-server VLESS stream parameters (REALITY + TLS) end-to-end so generated Xray configs connect to REALITY/XTLS-Vision servers, fixing the Telegram bot (socks5) and LAN TPROXY.

**Architecture:** Extend the `Server` schema (Go `vless` + `vpnconfig`, shell `servers.json`) with `security/network/flow/sni/fingerprint/public_key/short_id/alpn`. Both parsers read URI query params; config generation replaces the `outbounds` array wholesale built from those params — `encoding/json` in Go (`xray.go`), a sourceable jq helper (`lib/xrayconf.sh`) in shell. The template becomes valid JSON with empty `outbounds`.

**Tech Stack:** Go (`encoding/json`, `net/url`), Bash + `jq`, bats (bats-support/bats-assert), `go test`.

**Spec:** `docs/superpowers/specs/2026-06-07-vless-reality-design.md`

**Conventions:**
- Go tests: `cd server && go test ./...` (delegate to build-runner agent).
- Bats tests: `bats router/test/...` (delegate to build-runner agent).
- Legacy = empty `security`: output must be **semantically** identical to the old TLS config (`security:"tls"`, `serverName`=address, `alpn:["h2"]`, no `flow`). JSON key order may differ; Xray is order-insensitive — tests assert parsed structure, never bytes.

---

## STATUS — ALL 9 TASKS COMPLETE ✅ (plan trimmed 2026-06-07)

All implementation tasks are done, committed, and verified. The original detailed per-task
steps are preserved in git history (full plan in commit `559408e`). Branch `feat/vless-reality-vision`,
HEAD after Task 8 = `2bac838`. Branch base (before this work) = `d2aef54`.

| Task | Status | Final commit |
|---|---|---|
| Task 1: Make `config.json.template` valid JSON (+ dev template) | ✅ Done | `c7805f6` |
| Task 2: Go — parse query params in `vless.ParseURI` | ✅ Done | `d460d46` |
| Task 3: Go — `vpnconfig.Server` fields + `ToVPNConfig` converter | ✅ Done | `1df87e6` |
| Task 4: Go — generate REALITY/TLS outbound in `xray.go` | ✅ Done | `3fcdf0c` |
| Task 5: Shell — `lib/xrayconf.sh` outbound/config builder | ✅ Done | `76c67c5` |
| Task 6: Shell — extract query params in `import_server_list.sh` | ✅ Done | `84b3038` |
| Task 7: Shell — wire `configure.sh` to generator (+ `.ip`→`.ips` fix) | ✅ Done | `f1323c5` |
| Task 8: Install plumbing (`install.sh` + `downloader.go`) + docs + migration note | ✅ Done | `2bac838` |
| Task 9: Full test sweep + e2e generation sanity | ✅ Done | verification only (no commit) |

Each task ran the loop: implementer subagent → **build-runner** authoritative verification →
spec-compliance review ✅ → code-quality review ✅ → mark complete. Minor review nits were folded
in via `git commit --amend` (Task 1 `load` path → extension-less; Task 3 strengthened
`ToVPNConfig` test to assert all 13 fields; Task 5 header cross-ref to the Go twin; Task 7 removed
now-dead `SELECTED_SERVER_UUID`).

**Task 9 verification results:**
- Go `cd server && go test ./...` → **18/18 packages pass** (vless, service, vpnconfig, handler, webapi, wizard, updater, …).
- Feature bats: `xray_template` 3/3, `xrayconf` 6/6, `import_server_list` 20/20 — all green.
- e2e generation on the real REALITY subscription line → **PASS** (`security=reality`,
  `realitySettings{publicKey/serverName/shortId}`, user `flow=xtls-rprx-vision`, single outbound,
  inbounds preserved, valid JSON).

## POST-COMPLETION — #193 FIXED + external review applied (2026-06-07)

**#193 (`tunnel_apply: handles clients as string instead of array`) — FIXED in `98f72db`.**
Root cause (confirmed; differs from the earlier *unverified* hypothesis): NOT the "invalid JSON →
exit" path — the fixture is valid JSON. `config.sh` built `TUN_DIR_TUNNELS_JSON` with
`(.value.clients // []) - $p` (the `paused_clients` filter); a string-valued `clients` makes that a
jq `string - array` type error (exit 5), and under `set -euo pipefail` the failing command
substitution aborted *sourcing* `config.sh` inside the bats test body → no result line →
`Executed 240 instead of expected 241`. Fix: subtract `paused_clients` only when `clients` is an
array. Introduced by the `paused_clients` feature, not this branch (config.sh was byte-identical to
base — the bug is the interaction). `tunnel_apply` already validated string clients correctly.

**External mesh review — ROUND 1 (7 reviewers: claude/codex + 5 ext models) — applied:**
- `a0de79a` (auto-fixes): atomic `config.json` write in `configure.sh` (the `>` redirect truncated
  the live config on generator failure — codex Critical); `_cfg_arr_active` got the same array-guard
  as `98f72db`; `xrayconf_build_outbound` rejects empty/null/non-object stdin; `parse_vless_uri`
  locals; user `flow` emitted only when `security` non-empty (legacy contract); `alpn` tests + regressions.
- `4991cd0` (decision): REALITY generators now require non-empty `public_key`+`sni`+`fingerprint`
  (shortId optional) in both `xray.go` and `xrayconf.sh` + tests.
- Verified after each: Go **738** green, bats **246/246** green.

**External mesh review — ROUND 2 (this session 2026-06-07, same 7 reviewers) — applied:**
- `07e7ac1` (auto-fixes): Go `GenerateConfig` now writes `config.json` atomically
  (`os.CreateTemp`+`os.Rename`, 0600) like the shell twin — a truncated/interrupted write can no
  longer brick Xray (and the bot proxying through it); `ParseURI` now propagates `url.ParseQuery`
  errors instead of silently discarding them (would drop stream params → broken outbound)
  (+`TestParseURI_MalformedQuery`).
- `22db1e6` (decision, Variant A): reject out-of-range port (1–65535) at the **import entry points**
  — Go `parser.go` + shell `import_server_list.sh` (`10#` radix avoids octal misread). Keeps
  `servers.json` clean so both generators stay safe without duplicate checks (+Go & bats tests).
- Round 2 found **0 Critical / 0 Important** happy-path bugs; strong 7-reviewer convergence
  confirmed REALITY/TLS generation, backward-compat, and Go↔shell parity. Everything else dismissed
  as pre-existing / out-of-scope / documented / false-positive. Verified: Go all packages green,
  bats green (+3 new tests). Delegation guard: all 6 wrappers REAL.

**DISMISSED but worth SEPARATE follow-ups (all pre-existing, out of scope here — confirmed not in
this branch's diff hunks / present at base `aebb84af`):**
- SSRF: Telegram `/import` (`server/internal/handler/import.go`) fetches arbitrary URLs with **no
  SSRF guard** (no private/loopback/scheme check), unlike the guarded WebAPI `/api/servers/import`.
- `server/internal/webapi/handler_servers.go:199` swallows the `SaveVPNConfig` error (`_ =`) — API
  returns 200 while `xray.servers` stays stale (the bot twin at `import.go:126` warns on failure).
- `handleSelectServer` (`handler_servers.go:76`) sets only the selected server's IPs into
  `xray.servers` (shell `configure.sh` sets all imported IPs) — possible routing loop on switch.
- Minor pre-existing: `import_server_list.sh` `curl` has no `--max-time`; DEBUG log line prints the
  UUID; `base64 -d` is std-only (Go tries 4 variants); IPv6-in-brackets host parse is broken in shell.
- This branch only changed `ToVPNConfig()` wiring in those handler files — none of the above is part
  of this work, but all are real and worth their own issue/PR.

→ Only remaining: finish the branch — `git rm` all tracked files under `docs/superpowers/` in a
SEPARATE commit (per repo workflow), then open the PR. See
`2026-06-07-vless-reality-pr-v2-continuation-prompt.md` (supersedes the earlier `-pr-` prompt;
round-2 mesh-review now also applied).

---

_(Original per-task implementation steps — failing tests, exact code blocks, commit commands —
preserved in git history at commit `559408e`. Trimmed here per
`claude-mesh:continue-plan-fresh-session` to keep the fresh session's context lean.)_
