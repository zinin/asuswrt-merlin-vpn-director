# Platform Layer Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the Merlin-only code base into a platform-neutral one without changing what Asus routers do: rename the repository, drive the installer and the updater from one file manifest, and move every Asuswrt-Merlin-specific call behind the shell platform contract.

**Architecture:** `lib/platform.sh` detects the platform and sources `lib/platform/merlin.sh`, which implements the contract of spec section 5. Core modules (`common.sh`, `firewall.sh`, `tunnel.sh`, `tproxy.sh`, `send-email.sh`, the CLI, `S99vpn-director`) call only contract functions. `router/files.manifest` is the single list of shipped files for `install.sh` and the Go updater, so the Keenetic files of the next plan need no new lists.

**Tech Stack:** Bash 5 (router scripts), Bats with bats-support/bats-assert (`router/test`), Go 1.26 (`server/`), GitHub CLI.

**Spec:** `docs/superpowers/specs/2026-09-10-keenetic-platform-design.md`. This plan covers spec sections 4, 5, 7, 8 (Merlin side), 9.6, 14 (Merlin side) and delivery-order items 2-4 of section 18. Plan 2 (Keenetic shell implementation, hooks, installer) and plan 3 (Go `internal/platform`, `/api/platform`, Web UI, bot, `mipsle`) follow once this plan is merged.

## Global Constraints

- Shell scripts start with `#!/usr/bin/env bash` and `set -euo pipefail`, use `[[ ]]`, and log through `log -l ERROR|WARN|INFO|DEBUG|TRACE "message"`.
- Platform functions print their result on stdout and return 0; a function that cannot answer prints nothing and returns 1; platform functions never call `log -l ERROR` (spec 5.2).
- Behaviour on Merlin does not change (spec 1). Every existing Bats and Go test keeps passing, except the ones this plan replaces by name.
- Manifest format (spec 4.3): one file per line, `<tag> <repo path>`, `#` comments and blank lines allowed, tags `common`, `merlin`, `keenetic`; install destination is `/` plus the path without the `router/` prefix; every file is executable except paths ending in `.template`, `.json` or `.manifest`.
- Tests: from the repository root `bats router/test/unit` and `bats router/test/integration`; from `server/` `go build ./... && go vet ./... && go test ./... -count=1`. In the main session, run build and test commands through the `claude-forge:build-runner` agent (project rule); a task subagent may run them directly.
- Every commit message ends with the line `Claude-Session: https://claude.ai/code/session_01SygLygzpzxRtxoBqZsZuvL`.
- Work on branch `feature/keenetic-platform`. Do not stage the untracked files that were already in the working tree (`.mcp.json`, `9ca829ab40e44da8ba146a9e61ae6d3a`, `decoded.txt`, `review.diff`, `docs/superpowers/plans/*continuation*`); stage files by name.

## File Structure

Created:

| File | Responsibility |
|------|----------------|
| `router/files.manifest` | The list of shipped files with platform tags |
| `router/opt/vpn-director/lib/platform.sh` | Platform detection (`platform_detect`), `VPD_PLATFORM`, loading of `lib/platform/<name>.sh` |
| `router/opt/vpn-director/lib/platform/merlin.sh` | Merlin implementation of the contract, one function per row of spec 5.2 |
| `router/test/unit/platform.bats` | Detection and loader tests |
| `router/test/unit/platform_merlin.bats` | One test group per contract function |
| `router/test/mocks/cru` | Records `cru` calls |
| `server/internal/updater/manifest.go` | `ManifestEntry`, `parseManifest`, `manifestFilesFor`, `isExecutable`, `FileEntry`, `fileEntries` |
| `server/internal/updater/manifest_test.go` | Tests for the above plus the repository manifest |

Modified:

| File | Change |
|------|--------|
| `server/go.mod`, `server/**/*.go`, `server/internal/updater/github.go`, `install.sh`, `README.md`, `README.ru.md`, `CLAUDE.md` | Repository rename |
| `install.sh` | `PLATFORM`, `INSTALL_ROOT`, manifest-driven `download_scripts`, `manifest_files`, `manifest_is_executable` |
| `server/internal/updater/downloader.go`, `downloader_test.go` | `DownloadRelease` fetches the manifest first; `scriptFiles` removed |
| `server/internal/updater/script.go`, `script_test.go`, `update_script.sh.tmpl` | File table from the payload manifest replaces hard-coded `cp`/`chmod` lines |
| `router/opt/vpn-director/lib/common.sh` | Sources `platform.sh`; `get_ipv6_enabled` and `get_active_wan_if` become wrappers |
| `router/opt/vpn-director/lib/firewall.sh` | `platform_wan_if`; `wan_id` parameter dropped |
| `router/opt/vpn-director/lib/ipset.sh` | `TUN_DIR_TABLES` state file path |
| `router/opt/vpn-director/lib/tunnel.sh` | Contract calls, `tun_dir_tables` state file, `_tunnel_ensure_routes` |
| `router/opt/vpn-director/lib/tproxy.sh` | `platform_load_module`, `platform_vpn_endpoints`, `platform_tproxy_extra_rules`, `platform_lan_ifaces` |
| `router/opt/vpn-director/lib/send-email.sh` | `platform_email_supported`, `platform_model` |
| `router/opt/vpn-director/vpn-director.sh` | `platform` and `cron` subcommands |
| `router/opt/etc/init.d/S99vpn-director` | Cron through the CLI |
| `router/test/test_helper.bash`, `router/test/mocks/nvram`, `router/test/unit/{tunnel,tproxy,install}.bats`, `router/test/common.bats`, `router/test/firewall.bats`, `router/test/integration/vpn_director.bats` | Test updates |
| `.claude/rules/packet-flow.md`, `.claude/rules/shell-conventions.md`, `.claude/rules/tunnel-director.md`, `CLAUDE.md` | Docs |

---

### Task 1: Rename the repository and the Go module path

✅ Done — see commit(s): `159ffd4`

### Task 2: File manifest and manifest-driven install.sh

✅ Done — see commit(s): `82c3f66`, `143ea6f`

### Task 3: Updater downloads the release by its manifest

✅ Done — see commit(s): `7c56994`, `f542cc6`

### Task 4: Update script installs the manifest files

✅ Done — see commit(s): `686c6b5`, `52b0e76`

### Task 5: Shell platform layer with the Merlin implementation

✅ Done — see commit(s): `b820cd3`, `59f8a7b`

### Task 6: common.sh and firewall.sh call the contract

✅ Done — see commit(s): `740bacc`

### Task 7: tunnel.sh uses platform tables, routes and positions

✅ Done — see commit(s): `86434c0`

### Task 8: tproxy.sh and send-email.sh call the contract

✅ Done — see commit(s): `dbd4a3e`, `39fa264`

### Task 9: `vpn-director.sh platform` and `cron` subcommands; S99 uses them

✅ Done — see commit(s): `43e6fde`

### Task 10: Documentation for the platform layer

✅ Done — see commit(s): `f8f0cbd`

### Task 11: Full verification and Merlin regression

✅ Done — Step 1 green at `c6199c9`; Step 2 closed as a read-only regression; Step 3 is this section.

Step 1 (full verification sweep) — 390 Bats tests, the whole Go suite, gofmt, bash -n/sh -n,
shellcheck against master's baseline, manifest↔tree consistency in both directions and a dead-reference
sweep, all green at `c6199c9`. Report: `.superpowers/sdd/…/task-11-verification.md` (git-ignored).

Step 2 (Merlin regression) — closed on 2026-09-11 in a **read-only** form, by the author's ruling that
nothing on the router may change: no file was copied and nothing was applied. The author's RT-AX86U
(Merlin 388.11, aarch64) runs v0.11.5 byte for byte. Two scripts streamed over SSH, with every mutating
command shadowed by a stub that refuses, ran the v0.11.5 functions next to their contract replacements
on the device — 7 of 7 answers identical (active WAN, the firewall's WAN, IPv6, tunnel list, PREROUTING
position, VPN endpoints, model) — and HEAD's `cmd_platform`, which printed valid and correct JSON under
the router's jq 1.7.1; `platform_detect` answers `merlin` on the real file system. From the code and the
device facts: the `cron install` line is byte-identical (`/opt` is a symlink there, `SCRIPT_DIR` takes
the logical `pwd`), `cron` and `platform` take no lock, the four status functions are unchanged, and by
the diff and the contract's answers `tproxy.sh` issues the same commands for this router's
configuration. Tunnel Director is not configured on it (`{"tunnels":{}}`), so both versions return at
the first check of `tunnel_apply`.

Not covered: a real `apply`/`restart` on the new files, the hooks, S99 at boot, the daemons on the new
files, Tunnel Director on hardware, and the upgrade path. The full regression runs through the releases
themselves: this router goes v0.11.5 → N with the old updater and N → N+1 with the new one, with a
read-only snapshot before and after each step (carry-forward, section 1).

## Self-review notes

- Spec coverage for this plan's scope: 4.1 (Task 1), 4.2-4.3 (Tasks 2-5), 5.1-5.2 Merlin column (Task 5), 5.3 (Task 9), 7 (Tasks 5-9), 8 except `configure.sh` (Tasks 6-9; `configure.sh` moves to plan 2 with the Keenetic tunnel list), 9.6 (Tasks 3-4 for the manifest; `repoName` in Task 1; `mipsle` and the platform-aware `getPlatform` wiring belong to plan 3), 14 Merlin rows (Tasks 5-9), 18 items 2-4 (all tasks).
- Names used across tasks: `platform_*` functions as listed in Task 5; `TUN_DIR_TABLES` (Task 7); `manifest_files`/`manifest_is_executable`/`PLATFORM`/`INSTALL_ROOT` (Task 2); `parseManifest`/`manifestFilesFor`/`isExecutable`/`fetchManifest`/`getPlatform`/`FileEntry`/`fileEntries`/`loadPayloadManifest` (Tasks 3-4); `cmd_platform`/`cmd_cron` (Task 9).

---

## Post-plan: the whole-branch review and what remains

The final whole-branch review (after Task 11 Step 1) returned one Critical, three Important and six
Minor findings. All the blocking ones were fixed in `c6199c9` and confirmed by a scoped re-review;
its verdict is **ready to merge**. Task 11 Step 2 is closed (above). One thing is outstanding and
needs the author: **the two-release delivery plan (ruling R17)**, which the Critical finding requires —
a single release cut from this branch breaks the CLI on every already-installed router, because the
deployed updater cannot deliver the new `platform.sh` that the new `common.sh` requires. The order of
the release steps, found while preparing them, is not decided yet.

The release plan and its sequencing, the contract invariants the Keenetic implementation must satisfy,
the firmware facts still read outside the contract, every deferred finding, every ruling and the facts
taken from the author's router are written up in `docs/superpowers/plans/2026-09-11-plan-1-carry-forward.md`.
