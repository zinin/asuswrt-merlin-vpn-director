# Self-Update Handover Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** From this release on, every release installs itself: the daemon a user asks to update downloads the new release's binary and hands the update over to it, through a small contract later releases may only extend.

**Architecture:** `updateflow.run` stops calling `DownloadRelease` and `RunUpdateScript` itself. Step 1 (`updater.Handover`, in the running daemon) downloads its own daemon's asset of the new release as `/tmp/vpn-director-update/installer` and runs `installer self-update --from … --to … --initiator … --chat-id …`. Step 2 (`updater.RunSelfUpdate`, dispatched first by both `main` functions of the new binary) runs the existing `DownloadRelease` and `RunUpdateScript` with its own code and template.

**Tech Stack:** Go 1.25 (`server/go.mod`: `go 1.25.5`), standard library only (`os/exec`, `flag`, `net/http/httptest`); POSIX `sh` scripts as fake installers in tests.

**Spec:** `docs/superpowers/specs/2026-09-11-self-update-handover-design.md`

## Global Constraints

- Subcommand `self-update`; flags `--from`, `--to`, `--initiator`, `--chat-id`, in exactly this invocation: `installer self-update --from <vX.Y.Z> --to <vX.Y.Z> --initiator <bot|webui> --chat-id <int64>`. Later releases may add to the contract but never remove or rename anything in it.
- Installer path `/tmp/vpn-director-update/installer` (`UpdateDir` + `/installer`); it runs from `/`, stdin `/dev/null`, the daemon's environment.
- Step 1 waits at most 15 minutes; forwards at most 10 progress lines of at most 300 characters (runes) each; the rest goes to the log.
- Asset `<daemon>-<arch>` of the daemon step 1 runs in, else of another daemon in its `Daemons` table; the existing 50 MB download limit (`maxFileSize`) applies.
- Exit 0 of step 2 means the update script has started and owns the lock and `files/`; any other exit means nothing outside `/tmp/vpn-director-update` changed, and the last non-empty stderr line is the reason.
- User-facing messages: `Download failed: …`, `Failed to start the new version: …`, `Update failed: …`, `Update timed out`; on success the existing `Update script started, the service will restart in a few seconds...`.
- Step 1 cleans up (`installer`, `files/`, lock) only while the lock names its own PID. Step 2 checks that the lock names its parent twice: before the first download and right before the script starts. Step 2 refuses unless `--to` equals the version compiled into it.
- Step 2 never logs: its stderr is the channel for the reason.
- `update_script.sh.tmpl` and `testdata/update_script.golden.sh` do not change.
- Tests, from `server/`: `go build ./... && go vet ./... && go test ./... -count=1`. In the main session run them through the `claude-forge:build-runner` agent (project rule); a task subagent may run them directly. `cmd/webui` embeds `web/dist`: if `server/cmd/webui/web/dist` is missing, run `make web-embed` from the repository root first.
- `gofmt -l server/` legitimately lists `internal/wizard/handler.go` and `internal/ssrf/ssrf_test.go` (pre-existing on `master`); nothing else may appear.
- Work on branch `feature/keenetic-platform`. Stage files by name. Never stage `.claude/settings.local.json`, `.mcp.json`, `9ca829ab40e44da8ba146a9e61ae6d3a`, `decoded.txt`, `review.diff`, or any `docs/superpowers/plans/*continuation-prompt.md`, `*handoff-prompt.md`, `*execution-prompt.md`.
- Every commit message ends with the line `Claude-Session: https://claude.ai/code/session_01UfSG5SeDGVee1x5doaXHzb`.

## File Structure

Created:

| File | Responsibility |
|------|----------------|
| `server/internal/updater/selfupdate.go` | The contract (doc comment), `SelfUpdateCommand`, the invocation step 1 builds and step 2 parses, `RunSelfUpdate` (step 2) |
| `server/internal/updater/handover.go` | `Handover` (step 1): fetch the installer, run it, relay its output, clean up after a failure; `HandoverError` |
| `server/internal/updater/selfupdate_test.go` | Step 2 and the argv contract |
| `server/internal/updater/handover_test.go` | Step 1 against fake installers |
| `server/internal/updater/testdata/selfupdate_argv.txt` | Every invocation a released step 1 builds; only grows |
| `server/cmd/bot/main_test.go`, `server/cmd/webui/main_test.go` | `main` dispatches `self-update` before the daemon starts |

Modified:

| File | Change |
|------|--------|
| `server/internal/updater/github.go` | `GetReleaseByTag`; `GetLatestRelease` shares `fetchRelease` |
| `server/internal/updater/updater.go` | `DaemonBot`/`DaemonWebUI`, `NewForDaemon`, `lockNamesPID`, installer path, new `Service` seams, `Updater` interface trades `DownloadRelease`/`RunUpdateScript` for `Handover` |
| `server/internal/updater/downloader.go` | Step 2 takes its own daemon's payload binary from its executable (hard link, copy as fallback) |
| `server/cmd/bot/main.go`, `server/cmd/webui/main.go` | Dispatch `self-update` first; build the updater with the daemon name |
| `server/internal/updateflow/start.go`, `flow.go` | `run()` hands over; `handoverMessage` |
| `server/internal/updateflow/flow_test.go`, `start_test.go`, `server/internal/updatechecker/checker_test.go` | Mocks and tests follow the interface |
| `.claude/rules/telegram-bot.md`, `.claude/rules/webui.md` | The handover |
| `docs/superpowers/plans/2026-09-11-plan-1-carry-forward.md`, `docs/superpowers/plans/2026-09-10-platform-layer-foundation.md` | Ruling R17 closed |

---

### Task 1: Release by tag, the own payload binary, the daemon names

✅ Done — see commit(s): `59d9945`

### Task 2: Step 2 — the self-update entry point and its contract

✅ Done — see commit(s): `17c826a`

### Task 3: Both daemons run step 2 before anything else

✅ Done — see commit(s): `0e058cc`

### Task 4: Step 1 — the handover

✅ Done — see commit(s): `08b297f`, `6a9cdfd` (the second applies ruling R5: success is decided on step 2's exit status)

### Task 5: The update flow hands over instead of installing

✅ Done — see commit(s): `5748638`

### Task 6: Documentation

✅ Done — see commit(s): `979fa52`, `d2a5f60`, `e8b0fcf` (the third applies ruling R6: documentation accuracy fixes)

### Task 7: Verification sweep

✅ Done — no commits: every check passed (Go suite and -race, gofmt, shell side untouched, 390 Bats tests, wiring greps)

## Self-review notes

- Spec coverage: section 4 step 1 → Task 4 (download, neutral name, invocation from `/`, relay, 15-minute wait, cleanup); step 2 → Task 2 (flags, lock check before and after the download, release by tag, self-link, script) with Task 1's building blocks; dispatch → Task 3; what stays → Task 5 leaves `Check`, `Start`'s checks, the script, `notify.json`, the adapters and `updatechecker` alone. Section 5 items 1–5 → `installerAsset` (1), `selfUpdateArgv`/`parseSelfUpdateArgs` and the argv record (2), `runInstaller`'s exit handling and step 2's stderr-only errors (3), `lockNamesPID` in both steps (4), the version check (5). Section 6 → Task 4's tests one for one (download, start, installer, timeout, lock taken over, progress cap, temp name) plus Task 2's refusals. Section 7 → Tasks 2–5. Section 8 (rollout) and section 9 (release context) are release work, not code; Task 6 records them. Section 11 → Task 6.
- Deviations from the spec, deliberate:
  - The end-to-end test (spec section 7) re-runs each package's own test binary instead of `go build -X main.Version=v9.9.9`: the same proof that `main` dispatches first, without calling the toolchain from a test. The binary is a dev build, which the version check refuses the same way.
  - The step 2 success test checks the values the script was rendered with; byte equality stays with `TestGenerateScript_Golden`, which renders with the default paths.
  - `startInstaller` retries an exec refused with "text file busy". The spec's guard covers the installer this process wrote; the retry covers a descriptor another fork of the daemon briefly holds.
- Names shared across tasks: `DaemonBot`, `DaemonWebUI`, `NewForDaemon`, `GetReleaseByTag`, `lockNamesPID`, `linkOrCopy`, `copyExecutable`, `Service.daemon`, `Service.selfBinary` (Task 1); `SelfUpdateCommand`, `RunSelfUpdate`, `selfUpdateArgv`, `parseSelfUpdateArgs`, `selfUpdateArgs`, `Service.parentPID`, `Service.executable` (Task 2); `InstallerName`, `InstallerFile`, `Handover`, `HandoverError`, `HandoverPhase`, `PhaseDownload`, `PhaseStart`, `PhaseInstaller`, `PhaseTimeout`, `Service.handoverTimeout`, `startInstaller`, `lineWriter` (Task 4); `handoverMessage` (Task 5).
