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

## POST-COMPLETION FINDING — pre-existing unrelated test failure (remaining work)

The full bats sweep (`bats router/test/ router/test/unit/ router/test/integration/`) exits **1**
because ONE test aborts without emitting a result line: **`tunnel_apply: handles clients as string
instead of array`** (`router/test/unit/tunnel.bats:245`; #193 in the full run / #27 when running
`tunnel.bats` alone). There are **no `not ok` failures** (240 ok, 0 not ok, 1 missing) — bats warns
`Executed 240 instead of expected 241 tests`.

This is **pre-existing and unrelated** to the VLESS REALITY work:
- This branch (`d2aef54..HEAD`) never touched the test's dependency chain — `tunnel.bats`,
  `tunnel.sh`, `config.sh`, `common.sh`, `firewall.sh`, `ipset.sh`, `test_helper.bash`, `fixtures/`
  are all byte-identical to base `d2aef54`.
- The test + fixture `vpn-director-clients-string.json` were introduced in commit `e9516c8`
  (2026-01-22), long before this branch.
- Reproduces deterministically in isolation.
- **Hypothesis (unverified):** `source "$LIB_DIR/config.sh"` with the malformed `clients`-as-string
  fixture likely calls `exit` during sourcing (same code path as the passing test
  "config.sh: fails on invalid JSON"), aborting the bats test body before `run tunnel_apply` ever
  runs — so no `ok`/`not ok` line is printed and bats counts one short. Verify this before fixing.

→ Fixing this test (via `superpowers:systematic-debugging`) and then finishing the branch are the
two jobs for the fresh session — see `2026-06-07-vless-reality-finish-continuation-prompt.md`.

---

_(Original per-task implementation steps — failing tests, exact code blocks, commit commands —
preserved in git history at commit `559408e`. Trimmed here per
`claude-mesh:continue-plan-fresh-session` to keep the fresh session's context lean.)_
