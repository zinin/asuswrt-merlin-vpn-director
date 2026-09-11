# Self-update handover: design

Date: 2026-09-11
Branch: `feature/keenetic-platform`
Status: approved in brainstorming, awaiting implementation plan

## 1. Problem

The daemon a user asks to update installs the new release with its own code, and everything that
code decides is fixed at the version being replaced:

| Decision | Where it lives today |
|----------|----------------------|
| Which files to fetch | `router/files.manifest` of the release since this branch; a hard-coded list of 19 paths before it |
| How to read the manifest | `parseManifest`, `manifestPathRe` |
| Which files get the executable bit | `isExecutable` |
| Which daemons exist: assets, binaries, init scripts | `updater.Daemons` |
| How daemons are stopped, files copied and daemons started | `update_script.sh.tmpl`, embedded in the binary |
| Which platform the router runs | `getPlatform` |

A fix to the update procedure therefore reaches a router one release late, and a release that
changes the procedure incompatibly cannot be installed by the update button at all. Plan 1 of the
Keenetic work hit exactly this: the v0.11.x updater copies its 19 files and never the two files of
the platform layer, so every CLI call on such a router dies at the last line of `common.sh`
(ruling R17, carry-forward section 1).

## 2. Goal

From this release on, a release installs itself. The running daemon downloads one binary of the
new release and hands the update over to it; the new binary runs the download and the update
script with its own code. A future release may change its file list, its daemon set, its manifest
format, its update script and its platform detection, and a router on this release or any later
one still installs it, provided the release keeps the handover contract of section 5.

Repairing routers on v0.11.x is not a goal: their updater predates the handover. Section 9 says
what happens to them.

## 3. Current flow

`updateflow.Flow.Start` (`server/internal/updateflow/start.go`) refuses dev mode, a dev version, an
unknown initiator and a running update, asks the cached `Check` for the latest release, validates
both versions and takes the lock `/tmp/vpn-director-update/lock`. A goroutine then runs `run()`,
which makes two calls: `DownloadRelease` fetches the manifest, the files and both daemon binaries
into `files/`, and `RunUpdateScript` renders `update.sh` from the embedded template and starts it
detached. The script stops the running daemons, copies the payload, starts the daemons back and
writes `notify.json`, which the bot reports on its next start.

The updater is a package linked into both daemons, not a binary of its own. The bot
(`server/internal/bot/bot.go:127`) and the Web UI (`server/cmd/webui/main.go:159`) each build their
own `Flow` over it.

## 4. Design

The handover splits `run()` into two steps.

**Step 1, the running binary.** `Start` keeps its checks, its cache and its lock. `run()` stops
downloading the release and rendering the script itself:

1. It downloads the release asset of its own daemon (`<daemon>-<arch>`, section 5) under a
   temporary name in `/tmp/vpn-director-update/` and renames it to `installer` with mode 0755.
   The neutral name matters: `pidof` and `killall` in the init scripts match the process name, and
   a process called `telegram-bot` would pass for the running daemon.
2. It runs `installer self-update --from <current> --to <tag> --initiator <bot|webui> --chat-id <id>`
   from `/`, with stdin from `/dev/null` and the daemon's environment, and waits at most 15 minutes.
3. It passes each stdout line of the installer to the `progress` callback, as `run()` passes its own
   lines today.
4. On exit 0 it reports "Update script started, the service will restart in a few seconds..." and
   leaves `files/` and the lock to the script. Any other outcome is a failure it cleans up and
   reports (section 6). Either way it deletes `installer`.

**Step 2, the new binary.** Both `main` functions check `os.Args[1] == "self-update"` before
`flag.Parse()` and before anything else a daemon does, and hand the remaining arguments to the
updater, which:

1. Parses and validates the flags (section 5).
2. Refuses unless the lock names its parent process.
3. Fetches the release by its tag (`GET /repos/zinin/vpn-director/releases/tags/<tag>`); the latest
   release may have moved since step 1 looked.
4. Runs `DownloadRelease`, taking its own executable as the payload binary of its daemon instead of
   downloading it again: a hard link to `installer`, a copy when the link fails. It prints
   "Files downloaded, starting update..." as `run()` does today.
5. Refuses unless the lock still names its parent process.
6. Runs `RunUpdateScript` with its own template and exits 0 as soon as the script has started.

**What stays.** `Flow.Check` and its cache, `Start`'s checks and lock, `update_script.sh.tmpl`,
`notify.json`, the bot and Web UI adapters, `updatechecker` and the bot's startup notifier.

| Unit | Place | Responsibility |
|------|-------|----------------|
| Handover (step 1) | `internal/updater`, called from `updateflow.run` | Fetch the installer, run it, relay its progress, clean up after a failure |
| Entry point (step 2) | `internal/updater/selfupdate.go` | Parse the contract, run the existing download and script |
| Dispatch | `cmd/bot/main.go`, `cmd/webui/main.go` | Route `self-update` before the daemon starts |
| Contract | doc comment in `selfupdate.go`, tests of section 7 | The frozen surface of section 5 |

`DownloadRelease` and `RunUpdateScript` keep their code; only their caller moves from step 1 to
step 2. The `Updater` interface that `updateflow` depends on trades those two methods for the
handover. Step 1 needs the name of its daemon, so each daemon passes its own (`telegram-bot` or
`webui`) to the updater it gives its `Flow`.

## 5. Handover contract

Step 1 of this release will start step 2 of every later release, and a router cannot update the
step 1 it runs. Everything step 1 relies on is therefore fixed here. Later releases may add to this
contract but never remove or rename anything in it. Step 1 is always older than the step 2 it
starts, so an addition never reaches a step 2 that does not know it.

1. **Asset.** Step 1 downloads `<daemon>-<arch>` of its own daemon and falls back to the other
   daemons of its `Daemons` table when the release lacks that asset. Every such binary implements
   `self-update` and stays below 50 MB, the download limit step 1 enforces.
2. **Invocation.** `installer self-update --from <vX.Y.Z> --to <vX.Y.Z> --initiator <bot|webui>
   --chat-id <int64>`, working directory `/`, stdin from `/dev/null`, the daemon's environment.
   `--chat-id` is 0 when the Web UI started the update.
3. **Result.** Every stdout line is a progress line for the user. Exit 0 means the update script
   has started and owns the lock and `files/`. Any other exit means no script started and nothing
   outside `/tmp/vpn-director-update` changed; the last non-empty stderr line is the reason shown to
   the user.
4. **Files.** The update directory is `/tmp/vpn-director-update`. `lock` holds the PID of its
   owner: step 1 while step 2 runs, then the script, which republishes its own PID as it does
   today. `notify.json` only gains fields: a script that fails before copying the binaries restarts
   the old daemons, and the old bot then reads it.
5. **Version.** Step 2 refuses unless `--to` equals the version compiled into it.

Step 2 owns everything else and may change it in any release: the manifest location and format,
the file list, platform detection, the daemon table and the update script.

## 6. Failure handling

Until the update script starts, the router is untouched, and step 1 restores the state before the
attempt: it reports the reason, deletes `installer` and `files/`, and removes the lock. A failed
download does the same today.

| Failure in step 1 | Message |
|-------------------|---------|
| The installer asset is missing, over 50 MB or not https, or its download fails | `Download failed: …` |
| The installer does not start: wrong architecture, corrupt file | `Failed to start the new version: …` |
| The installer exits non-zero | `Update failed: <last stderr line>` |
| The installer runs longer than 15 minutes | step 1 kills it: `Update timed out` |

Guards in step 1:

- It cleans up only while the lock names its own PID. A lock naming another process means the
  script has taken over, and step 1 leaves `files/` and the lock alone. This closes the race of a
  timeout that fires just as the script starts.
- It forwards at most 10 progress lines of at most 300 characters each and writes the rest to its
  log, so a faulty step 2 cannot flood a chat.
- It closes the download and renames it before running it; running a file still open for writing
  fails with "text file busy".
- It writes the installer's stderr to the daemon's own log.

Guards in step 2:

- It validates `--from` and `--to` with `IsValidVersion` and `--initiator` against `bot` and
  `webui`, as `RunUpdateScript` does today, because the values reach the script.
- It checks that the lock names its parent twice: before the first download and right before the
  script starts. A parent that died and left a stale lock someone may have replaced, or a
  `self-update` typed into a shell, stops there without touching the router.
- It exits right after the script starts, so nothing can fail between the two.

The update script and its `EXIT` trap do not change: a failure there still restarts the daemons that
were running and writes `notify.json` with `"status": "failed"`.

Resources: the hard link keeps the payload at about 16 MB of tmpfs, as today. Step 2 adds one GitHub
API request, the release by tag, to the budget of 60 an hour.

The user sees the same flow. The Web UI learns about a running update from the lock, which names a
live daemon while step 2 runs, and the 15-minute limit fits inside the page's 20-minute wait. The
bot sends the same messages, and dev mode still refuses to update.

## 7. Testing

Go, step 1, with a fake installer: a shell script in `testdata` that records its argv and working
directory, prints lines and exits with a chosen code.

- Success: the argv is exactly the contract's, the working directory is `/`, the progress lines
  arrive in order, `installer` is gone and the lock is untouched.
- A non-zero exit, a timeout, a file that does not execute: cleanup and the message of section 6.
- A lock the fake rewrites to another PID: no cleanup.
- Progress beyond 10 lines or 300 characters: capped.
- Assets: without its own daemon's asset step 1 takes another daemon's; with none it reports
  `Download failed`.

Go, step 2:

- Flag parsing and every refusal: invalid versions, an unknown initiator, `--to` other than its own
  version, a lock naming another process.
- The success path against `httptest` servers for the API and the raw files: the rendered script
  matches the existing golden, and `files/<daemon>` is a hard link to the installer.

Contract:

- A `testdata` file keeps one argv line per released form of step 1, starting with this release's.
  A test feeds each line to the step 2 parser and expects it accepted. The file only grows, and a
  failure means a change that breaks routers on an older release.
- An end-to-end test builds the real bot and Web UI binaries with `-X main.Version=v9.9.9` and runs
  `self-update --from v9.9.8 --to v1.0.0 --initiator bot --chat-id 0`. Exit 1 with the
  version-mismatch message on stderr proves that `main` dispatches before the daemon starts. CI
  builds the Web UI's embedded SPA before `go test`, so both binaries compile there.

Shell: `update_script.sh.tmpl` does not change, and neither do its golden or the Bats suites.

## 8. Rollout

Step 1 of this release first runs when the next release comes out, and a defect in it cannot be
fixed on the routers that carry it; `install.sh` stays their way out, as it is today. So:

1. The author's router takes this release through `install.sh`, as the release notes tell routers
   on v0.11.x to.
2. A small release follows soon after, for example with some of the minor findings of the
   carry-forward. The author updates to it with the update button, and that update is the first
   real handover. A read-only snapshot before and after, `update.log`, the bot's message and the
   versions of both daemons confirm it.

## 9. Release context

Decided in the same session and recorded here because they shape this design:

- One release from `feature/keenetic-platform` carries the platform layer and the handover; the
  two-release split of ruling R17 is dropped.
- Routers on v0.11.x update with `install.sh`. Their update button installs the release
  incompletely and leaves a CLI that fails at the last line of `common.sh`. A startup self-repair of
  missing files was considered and declined. The release notes say this in their first 500
  characters, as plain text: the deployed bot cuts the changelog at 500 characters and escapes
  Markdown.
- The repository rename comes first, and the tag goes on the merge commit right after the merge.
  A tag before the merge would let `master`'s old `install.sh` install the release without the
  platform files. Between the merge and the release, `master`'s new `install.sh` fails loudly on
  v0.11.5, which has no manifest; tagging right after the merge keeps that window to the CI build.
- Open: how the notes are in place when CI publishes the release — a draft release created ahead
  of the tag, or an edit right after CI.

## 10. Out of scope

- Repairing an incomplete installation when a daemon starts.
- Rolling back a failed copy; the script stays a restart net.
- Signatures and checksums; the trust model stays TLS to GitHub plus GitHub account security.
- A dedicated installer asset; the daemon binaries carry step 2.
- Updating in steps through intermediate releases.

## 11. Documentation

- `.claude/rules/telegram-bot.md`, Self-Update: the two steps and the contract.
- `.claude/rules/webui.md`, trust model: the new release's binary runs as root before it is
  installed, under the same trust as the binaries it installs.
- The carry-forward of plan 1: ruling R17 closed as section 9 records.
- Section 9.6 of the Keenetic spec still holds; `DownloadRelease` now runs in step 2.
