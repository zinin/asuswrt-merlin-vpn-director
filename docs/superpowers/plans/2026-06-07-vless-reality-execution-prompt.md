## TASK

Execute the implementation plan for **VLESS REALITY/Vision support** (parse & persist per-server
VLESS stream params; generate REALITY/TLS Xray outbounds; fix Telegram bot via socks5 + LAN TPROXY).

Use `/superpowers:subagent-driven-development` skill for execution.

## PRECONDITIONS

- Work on branch `feat/vless-reality-vision` (already created; design + plan already committed there).
  Confirm with `git branch --show-current` before starting.
- This repo has a project convention: **never run build/test/lint directly — delegate to the
  `build-runner` agent** (or the `build` skill). Applies to `go test`, `go build`, `go vet`, `bats`.

## DOCUMENTS

- Design: `docs/superpowers/specs/2026-06-07-vless-reality-design.md`
- Plan: `docs/superpowers/plans/2026-06-07-vless-reality-vision.md`

Read both documents first.

## IMPORTANT: DO NOT START WORK YET

After reading the documents:
1. Confirm you have loaded all context.
2. Summarize your understanding briefly.
3. **WAIT for user instruction before taking any action.**

Do NOT begin implementation until the user explicitly tells you to start.

## SESSION CONTEXT

**Root cause (confirmed empirically):** The subscription migrated to REALITY + XTLS-Vision. Both
parsers cut everything after `?` (shell `parse_vless_uri`, Go `vless.ParseURI`); both `Server`
structs (`vless`, `vpnconfig`) lack stream fields; `config.json.template` is hardcoded to plain TLS
(`security:"tls"`, `serverName`=IP, `alpn:["h2"]`, no `flow`/`realitySettings`). The bot is hit
hardest because it routes its whole Telegram-API connection through `socks5://127.0.0.1:12346 →
socks-in → proxy-out`; LAN TPROXY is partially masked by its exclusion sets.

**Decisions (with rationale):**
- Scope = generalized **reality + tls** (both `security` types), not just the current subscription.
- Transport = **tcp only** for generation; `network` field is parsed/stored for future ws/grpc (YAGNI).
- Generation = **Mechanism B**: template becomes valid JSON with `outbounds: []`; both generators
  replace `outbounds[0]` wholesale (shell: `jq` in new `lib/xrayconf.sh`; Go: `encoding/json` in
  `service/xray.go`). Plain `{{...}}` placeholders are removed.
- `alpn` parsed and emitted **only when present** (subscription has none).
- reality `spiderX`/`show` **omitted** (rely on Xray defaults).

**Rejected alternatives:**
- Mechanism A (text placeholders `{{XRAY_STREAM_SETTINGS}}`): fragile `sed` injection of multi-line
  JSON containing base64 `/ + =` chars; easy to desync shell vs Go.
- Mechanism C (build whole config.json in code): duplicates inbounds/routing in two languages,
  discards the shared downloaded template.
- Hardcoding the template to reality: impossible — `sni`/`sid`/`fp` differ **per server**
  (the `pbk` is constant, but the rest vary), so per-server parsing is mandatory either way.

**Edge cases & warnings:**
- **Two** `vless.Server → vpnconfig.Server` conversion sites: `server/internal/handler/import.go`
  and `server/internal/webapi/handler_servers.go`. Both must use the new `ToVPNConfig()` (Task 3).
- `configure.sh` runs `main "$@"` unconditionally (not sourceable in bats) — that is why the
  generation logic lives in the sourceable `lib/xrayconf.sh` (testable), with `configure.sh` only
  wiring it (Task 7, smoke-tested).
- `configure.sh` lines 118 & 454 use `.ip` (singular) while `servers.json` stores `.ips` (array).
  This is a **pre-existing latent bug, OUT OF SCOPE** — do not fix it unless the user asks.
- `install.sh` download list (~lines 141-147) AND `server/internal/updater/downloader.go`
  `scriptFiles` must BOTH gain `lib/xrayconf.sh` (their comments say keep in sync) — Task 8.
- Legacy (empty `security`) output must be **semantically** identical to the old TLS config
  (`security:"tls"`, `serverName`=address, `alpn:["h2"]`, no `flow`), NOT byte-identical. Go map
  marshaling reorders keys alphabetically; Xray is order-insensitive. Tests assert parsed JSON
  structure, never bytes.
- Project shell rule: never use `[:upper:]`/`[:lower:]` in `tr` (busybox bug on router) — use
  `A-Z`/`a-z`. (No case conversion is needed in this work, but keep the rule in mind.)
- bats: match the existing `load '../test_helper.bash'` convention; `TEST_MODE=1`; the helper sets up
  mocks (`nslookup`, etc.) and `resolve_ip`. New unit tests go under `router/test/unit/`.
- servers.json keys: `security`, `network`, `flow`, `sni`, `fingerprint`, `public_key`, `short_id`,
  `alpn`. Shell `parse_vless_uri` pipe order is fields 1-12:
  `server|port|uuid|name|security|network|flow|sni|fp|pbk|sid|alpn`.

**Post-merge migration (document, don't automate):** users must re-run `/import` (or
`import_server_list.sh`) so params land in `servers.json`, then re-select the server via `/configure`
or `/xray` to regenerate `config.json`.

**Before opening a PR (per repo workflow):** `git rm` everything under `docs/superpowers/` and commit
so the design/plan/prompt docs do NOT appear in the PR diff (they remain in branch history).

## PLAN QUALITY WARNING

The plan was written for a large task and may contain:
- Errors or inaccuracies in implementation details
- Oversights about edge cases or dependencies
- Assumptions that don't match the actual codebase
- Missing steps or incomplete instructions

**If you notice any issues during implementation:**
1. STOP before proceeding with the problematic step.
2. Clearly describe the problem you found.
3. Explain why the plan doesn't work or seems incorrect.
4. Ask the user how to proceed.

Do NOT silently work around plan issues or make significant deviations without user approval.
