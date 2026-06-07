# Design: VLESS REALITY/Vision support (generalized reality + tls)

- **Date:** 2026-06-07
- **Branch:** `feat/vless-reality-vision`
- **Status:** Approved (pending spec review)

## Problem & Root Cause

The VLESS subscription migrated to **REALITY + XTLS-Vision flow**. Every URI now carries
mandatory stream parameters that the codebase silently discards:

```
security=reality  flow=xtls-rprx-vision  sni=cdn3-87.yahoo.com
pbk=CMkW1axrhEXoiJ6anMz9XEjlfqlAtEZya7L0b5ZPMyw  sid=55e6d9bd269aac46  fp=firefox
```

The breakage spans three layers (all confirmed empirically):

1. **Parsers cut everything after `?`.**
   - Shell `parse_vless_uri()` (`import_server_list.sh`): `server_port="${rest%%\?*}"`.
   - Go `ParseURI()` (`vless/parser.go`): `rest = rest[:idx]` at the `?`.
   - Both emit only `address/port/uuid/name`.
2. **`Server` structs** (`vless.Server`, `vpnconfig.Server`) store only
   `Address/Port/UUID/Name/IPs` — nowhere to keep the params; `servers.json` lacks them.
3. **`config.json.template`** is hardcoded to plain TLS (`security:"tls"`,
   `serverName` = server IP, `alpn:["h2"]`, no `flow`, no `realitySettings`). Both generators
   (`configure.sh` via `sed`, `xray.go` via `strings.ReplaceAll`) substitute only address/port/uuid.

Result: a plain-TLS outbound is generated for REALITY servers → the handshake never completes →
**all traffic through `proxy-out` fails**. The Telegram bot is hit hardest because it routes its
*entire* connection to the Telegram API through `socks5://127.0.0.1:12346 → socks-in → proxy-out`
with no bypass; LAN TPROXY traffic partially masks the breakage via its exclusion sets
(RU / bypass / private ranges go direct and keep working).

## Goals

- Parse and persist per-server stream parameters end-to-end.
- Generate correct Xray outbounds for `security=reality` and `security=tls`.
- Keep the shell (install/CLI) and Go (bot/WebUI) paths in sync.
- No regression for existing `servers.json` files (legacy TLS output unchanged).

## Non-Goals (YAGNI)

- Transports other than `tcp` (`network` field is parsed & stored, but only `tcp` generation is built).
- `ws` / `grpc` / `httpupgrade` stream settings generation.
- WebUI feature work beyond carrying the new fields through existing conversion.

## Decisions Summary

| Decision | Choice |
|---|---|
| Scope | REALITY **and** TLS, generalized |
| Transports generated | `tcp` only (field stored for future) |
| Generation mechanism | **B**: template becomes valid JSON; generators replace `outbounds[0]` wholesale (`jq` in shell, `encoding/json` in Go) |
| `alpn` | parsed from URI; emitted in `tlsSettings` only when present |
| reality `spiderX`/`show` | omitted (Xray defaults); may revisit if a client needs them |
| Backward compat | empty `security` → byte-identical legacy TLS output |

## Section 1 — Data Model: extend `Server`

Add optional (`omitempty`) per-server fields to `vless.Server` and `vpnconfig.Server` (Go), and to
the shell `servers.json` builder:

| JSON key | URI source | Purpose |
|---|---|---|
| `security` | `security` | `reality` \| `tls` \| `""` |
| `network` | `type` | transport (`tcp`); stored for future |
| `flow` | `flow` | `xtls-rprx-vision` \| `""` |
| `sni` | `sni` | handshake serverName |
| `fingerprint` | `fp` | uTLS fingerprint |
| `public_key` | `pbk` | REALITY publicKey |
| `short_id` | `sid` | REALITY shortId |
| `alpn` | `alpn` | TLS ALPN list (when present) |

Old `servers.json` without these keys → all empty (handled in Section 4).

## Section 2 — Parsers: extract query params

- **Go `ParseURI`** (`vless/parser.go`): strip `#name` first (current behavior), then parse the
  query with `url.ParseQuery` before splitting `uuid@host:port`. Map params → new `Server` fields.
  Split `alpn` on `,`.
- **Shell `parse_vless_uri`** (`import_server_list.sh`): add a dependency-free query mini-parser
  (parameter expansion). Extend the function output and the `step_parse_and_save_servers` builder so
  new fields land in the per-server `jq` object; empty values are omitted to match Go `omitempty`.
- **Conversion sites** `vless.Server → vpnconfig.Server` must copy the new fields. Two identical
  literal constructions exist and both must be updated:
  - `server/internal/handler/import.go:98` (bot `/import`)
  - `server/internal/webapi/handler_servers.go:169` (WebUI import)
  Consider a single shared converter (e.g. a `vless.Server` method or helper) to avoid drift;
  watch for import cycles (`vpnconfig` must not depend on `vless`).

## Section 3 — Config generation (mechanism B)

`config.json.template` becomes **valid JSON**. Both generators build the `proxy-out` outbound from
the selected server's fields and replace `outbounds[0]` wholesale:

- **Shell** (`configure.sh`): `jq --argjson ob "$out" '.outbounds[0] = $ob' template > config.json`
- **Go** (`xray.go`): `json.Unmarshal(template)` → replace `outbounds` → `json.MarshalIndent`

`streamSettings` shape by `security`:

- `reality` → `realitySettings { serverName=sni, fingerprint=fp, publicKey=pbk, shortId=sid }`;
  user object gets `flow`.
- `tls` → `tlsSettings { serverName = sni || address, fingerprint? , alpn? }`.
- `""` (legacy) → byte-identical current output:
  `tlsSettings { alpn:["h2"], serverName=address }`, no `flow`.

`network` = field value or `tcp` default. All `{{...}}` placeholders are removed from the template.

Generated outbound skeleton:

```json
{
  "protocol": "vless",
  "settings": { "vnext": [ { "address": "<addr>", "port": <port>,
    "users": [ { "id": "<uuid>", "encryption": "none", "flow": "<flow?>" } ] } ] },
  "streamSettings": { "network": "tcp", "security": "<sec>", "<sec>Settings": { ... } },
  "tag": "proxy-out"
}
```

(`flow` key omitted when empty.)

Config.json is regenerated only at configure / server-switch time, not by `vpn-director.sh apply`.

## Section 4 — Backward compatibility & migration

- Empty `security` (old `servers.json`) → legacy TLS output identical to today → **no regression**.
- New fields use `omitempty` (Go) / are omitted when empty (shell) → legacy files stay clean.
- **Migration steps for the user (documented):**
  1. Re-run `/import` (or `import_server_list.sh`) so params land in `servers.json`.
  2. Re-select the server via `/configure` wizard or `/xray` to regenerate `config.json`.

## Section 5 — Testing (TDD)

- **Go `vless` parser** (`parser_test.go`): reality URI → assert all fields; tls URI → assert tls
  fields; no-param URI → empties. Extend existing `TestParseURI_ComplexQueryParams`.
- **Go `xray` generator** (`xray_test.go`): reality `Server` → `outbounds[0]` has
  `realitySettings` (pbk/sid/sni/fp) and `users[0].flow`; legacy `Server` → matches current TLS
  output; assert valid JSON.
- **Shell bats** (`router/test/`): `parse_vless_uri` extracts params (unit); generation from a
  `servers.json` with reality fields yields correct `outbounds[0]` (assert via `jq`, integration);
  template-is-valid-JSON test.

## Files Touched

- `router/opt/etc/xray/config.json.template` — convert to valid JSON, drop placeholders.
- `router/opt/vpn-director/import_server_list.sh` — query parser + extended servers.json.
- `router/opt/vpn-director/configure.sh` — jq-based generation replacing `outbounds[0]`.
- `server/internal/vless/parser.go` — parse query → fields.
- `server/internal/vpnconfig/vpnconfig.go` — add `Server` fields.
- `server/internal/service/xray.go` — JSON-based generation, reality/tls outbound builder.
- `server/internal/handler/import.go`, `server/internal/webapi/handler_servers.go` — carry fields.
- Tests: `vless/parser_test.go`, `service/xray_test.go`, `router/test/...` (bats).
- Docs: `.claude/rules/xray-tproxy.md` (+ migration note where relevant).

## Open Questions

None outstanding. `alpn` included (when present), `spiderX`/`show` omitted by default — revisit only
if a downstream client requires them.
