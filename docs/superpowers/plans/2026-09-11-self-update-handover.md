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

**Files:**
- Modify: `server/internal/updater/github.go`
- Modify: `server/internal/updater/updater.go`
- Modify: `server/internal/updater/downloader.go`
- Test: `server/internal/updater/github_test.go`, `server/internal/updater/updater_test.go`, `server/internal/updater/downloader_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces:
  - `const DaemonBot = "telegram-bot"`, `const DaemonWebUI = "webui"`
  - `func NewForDaemon(name string) *Service`
  - `func (s *Service) GetReleaseByTag(ctx context.Context, tag string) (*Release, error)`
  - `func (s *Service) lockNamesPID(pid int) bool`
  - `Service` fields `daemon string` and `selfBinary string`
  - `func linkOrCopy(src, dst string) error`, `func copyExecutable(src, dst string) error`

- [ ] **Step 1: Write the failing tests**

Append to `server/internal/updater/github_test.go`:

```go
func TestGetReleaseByTag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if want := "/repos/zinin/vpn-director/releases/tags/v1.2.4"; r.URL.Path != want {
			t.Errorf("path = %q, want %q", r.URL.Path, want)
		}
		if got := r.Header.Get("Accept"); got != "application/vnd.github.v3+json" {
			t.Errorf("Accept = %q", got)
		}
		if got := r.Header.Get("User-Agent"); got != "vpn-director-telegram-bot" {
			t.Errorf("User-Agent = %q", got)
		}
		w.Write([]byte(`{"tag_name": "v1.2.4", "body": "notes", "assets": [
			{"name": "webui-arm64", "browser_download_url": "https://example.com/webui-arm64"}]}`))
	}))
	defer server.Close()

	release, err := NewWithBaseURL(server.URL).GetReleaseByTag(context.Background(), "v1.2.4")
	if err != nil {
		t.Fatalf("GetReleaseByTag() error = %v", err)
	}
	if release.TagName != "v1.2.4" || release.Body != "notes" {
		t.Errorf("release = %+v", release)
	}
	if len(release.Assets) != 1 || release.Assets[0].Name != "webui-arm64" ||
		release.Assets[0].DownloadURL != "https://example.com/webui-arm64" {
		t.Errorf("Assets = %+v", release.Assets)
	}
}

func TestGetReleaseByTag_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	if _, err := NewWithBaseURL(server.URL).GetReleaseByTag(context.Background(), "v9.9.9"); err == nil {
		t.Fatal("GetReleaseByTag() of a tag GitHub does not know succeeded")
	}
}
```

Append to `server/internal/updater/updater_test.go`:

```go
// Step 1 of a self-update prefers the release asset of the daemon it runs in,
// and step 2 takes that daemon's payload binary from itself. Both key on these
// names, so they must be the names the daemon table ships under.
func TestDaemonConstants_NameEntriesOfTheTable(t *testing.T) {
	for _, name := range []string{DaemonBot, DaemonWebUI} {
		found := false
		for _, d := range Daemons {
			if d.Name == name {
				found = true
			}
		}
		if !found {
			t.Errorf("no Daemons entry named %q", name)
		}
	}
}

func TestNewForDaemon_RemembersTheDaemon(t *testing.T) {
	if got := NewForDaemon(DaemonWebUI).daemon; got != DaemonWebUI {
		t.Errorf("NewForDaemon(webui).daemon = %q", got)
	}
	if got := New().daemon; got != "" {
		t.Errorf("New().daemon = %q, want none", got)
	}
}

func TestLockNamesPID(t *testing.T) {
	lockFile := filepath.Join(t.TempDir(), "lock")
	s := &Service{lockFile: lockFile}

	if s.lockNamesPID(4242) {
		t.Error("lockNamesPID() = true without a lock file")
	}
	for content, want := range map[string]bool{
		"4242":   true,
		"4242\n": true, // the update script republishes its PID with printf '%s\n'
		"4243":   false,
		"":       false,
		"junk":   false,
	} {
		if err := os.WriteFile(lockFile, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		if got := s.lockNamesPID(4242); got != want {
			t.Errorf("lock %q: lockNamesPID(4242) = %v, want %v", content, got, want)
		}
	}
}
```

Append to `server/internal/updater/downloader_test.go`:

```go
// Step 2 of a self-update is the new release's binary of its own daemon, so
// the payload takes that binary from the running executable instead of
// fetching the same bytes again: a hard link, which costs no tmpfs.
func TestDownloadBinaries_TakesItsOwnBinaryFromTheRunningExecutable(t *testing.T) {
	var mu sync.Mutex
	var requested []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requested = append(requested, r.URL.Path)
		mu.Unlock()
		w.Write([]byte("binary of " + strings.TrimPrefix(r.URL.Path, "/")))
	}))
	defer server.Close()

	tempDir := t.TempDir()
	self := filepath.Join(tempDir, "installer")
	if err := os.WriteFile(self, []byte("the running webui"), 0755); err != nil {
		t.Fatal(err)
	}
	s := &Service{httpClient: server.Client(), updateDir: tempDir, archSuffix: "arm64",
		daemon: DaemonWebUI, selfBinary: self}

	release := &Release{TagName: "v1.0.0"}
	for _, d := range Daemons {
		release.Assets = append(release.Assets, Asset{
			Name:        d.Name + "-arm64",
			DownloadURL: server.URL + "/" + d.Name + "-arm64",
		})
	}

	if err := s.downloadBinaries(context.Background(), release); err != nil {
		t.Fatalf("downloadBinaries() error = %v", err)
	}

	selfInfo, err := os.Stat(self)
	if err != nil {
		t.Fatal(err)
	}
	ownInfo, err := os.Stat(filepath.Join(tempDir, "files", DaemonWebUI))
	if err != nil {
		t.Fatalf("no payload binary for %s: %v", DaemonWebUI, err)
	}
	if !os.SameFile(selfInfo, ownInfo) {
		t.Errorf("files/%s is not a hard link to the running executable", DaemonWebUI)
	}
	data, err := os.ReadFile(filepath.Join(tempDir, "files", DaemonBot))
	if err != nil || string(data) != "binary of "+DaemonBot+"-arm64" {
		t.Errorf("files/%s = %q (%v), want the downloaded asset", DaemonBot, data, err)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, p := range requested {
		if strings.Contains(p, DaemonWebUI) {
			t.Errorf("downloaded %s although the running executable is that binary", p)
		}
	}
}

// A hard link needs one filesystem; without one the binary is copied, and it
// has to arrive executable.
func TestCopyExecutable_CopiesContentAndTheExecutableBit(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.WriteFile(src, []byte("binary"), 0644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "dst")

	if err := copyExecutable(src, dst); err != nil {
		t.Fatalf("copyExecutable() error = %v", err)
	}
	data, err := os.ReadFile(dst)
	if err != nil || string(data) != "binary" {
		t.Fatalf("dst = %q (%v), want the source content", data, err)
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0100 == 0 {
		t.Errorf("dst mode = %v, want it executable", info.Mode().Perm())
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run (from `server/`): `go test ./internal/updater -count=1 -run 'TestGetReleaseByTag|TestDaemonConstants|TestNewForDaemon|TestLockNamesPID|TestDownloadBinaries_TakesItsOwn|TestCopyExecutable'`
Expected: FAIL to compile — `undefined: DaemonBot`, `s.GetReleaseByTag undefined`, `unknown field daemon`, `undefined: copyExecutable`.

- [ ] **Step 3: Implement**

In `server/internal/updater/github.go`, add the import `neturl "net/url"`, extend the constant block and replace `GetLatestRelease` with the three functions below. The body of `fetchRelease` from `req, err := http.NewRequestWithContext(...)` to the final `return release, nil` is the existing body of `GetLatestRelease`, unchanged:

```go
// GitHub API constants.
const (
	repoOwner            = "zinin"
	repoName             = "vpn-director"
	defaultAPIURL        = "https://api.github.com"
	releasesEndpoint     = "/repos/%s/%s/releases/latest"
	releaseByTagEndpoint = "/repos/%s/%s/releases/tags/%s"

	// APITimeout bounds one GitHub API call. Handlers that make such a call
	// inside an HTTP request size their response deadline from it.
	APITimeout = 30 * time.Second
)
```

```go
// GetLatestRelease fetches the latest release info from GitHub API.
func (s *Service) GetLatestRelease(ctx context.Context) (*Release, error) {
	return s.fetchRelease(ctx, fmt.Sprintf(releasesEndpoint, repoOwner, repoName))
}

// GetReleaseByTag fetches the release published under tag. Step 2 of a
// self-update installs the release it belongs to, which need not be the
// latest one any more.
func (s *Service) GetReleaseByTag(ctx context.Context, tag string) (*Release, error) {
	return s.fetchRelease(ctx, fmt.Sprintf(releaseByTagEndpoint, repoOwner, repoName, neturl.PathEscape(tag)))
}

// fetchRelease reads one release document from the GitHub API.
func (s *Service) fetchRelease(ctx context.Context, endpoint string) (*Release, error) {
	ctx, cancel := context.WithTimeout(ctx, APITimeout)
	defer cancel()

	baseURL := s.baseURL
	if baseURL == "" {
		baseURL = defaultAPIURL
	}
	url := baseURL + endpoint

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "vpn-director-telegram-bot")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var ghRelease githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&ghRelease); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	release := &Release{
		TagName: ghRelease.TagName,
		Body:    ghRelease.Body,
		Assets:  make([]Asset, len(ghRelease.Assets)),
	}
	for i, a := range ghRelease.Assets {
		release.Assets[i] = Asset{
			Name:        a.Name,
			DownloadURL: a.BrowserDownloadURL,
		}
	}

	return release, nil
}
```

Also change the comment of `githubRelease` to `// githubRelease represents the GitHub API response for one release.`

In `server/internal/updater/updater.go`, replace the `Daemons` table and its comment with:

```go
// Daemon names, as the release assets and the files/ payload spell them.
// Step 1 of a self-update prefers the asset of the daemon it runs in; step 2
// takes that daemon's payload binary from itself (selfupdate.go).
const (
	DaemonBot   = "telegram-bot"
	DaemonWebUI = "webui"
)

// Daemons lists every daemon a release ships. DownloadRelease fetches one
// binary per entry and the update script restarts the entries that were
// running before the update. This table is the single source of truth: the
// downloader, the script template and install.sh must not drift apart.
var Daemons = []Daemon{
	{Name: DaemonBot, Binary: "/opt/vpn-director/telegram-bot", InitScript: "S98telegram-bot"},
	{Name: DaemonWebUI, Binary: "/opt/vpn-director/webui", InitScript: "S98vpn-director-webui"},
}
```

Add two fields at the end of the `Service` struct:

```go
	daemon     string // The daemon this process is: Handover prefers its asset, step 2 links itself for it
	selfBinary string // Step 2 only: this executable, taken as the payload binary of daemon
```

Add after `NewWithBaseURL`:

```go
// NewForDaemon creates a Service for the daemon named name (DaemonBot or
// DaemonWebUI). The daemons build their update flows with it, because a
// self-update hands over to the new release's binary of the daemon it runs in.
func NewForDaemon(name string) *Service {
	s := New()
	s.daemon = name
	return s
}
```

Add after `IsUpdateInProgress`:

```go
// lockNamesPID reports whether the lock file holds exactly pid. Step 1 of a
// self-update asks it about itself before it cleans up after a failure; step 2
// asks it about its parent before it acts.
func (s *Service) lockNamesPID(pid int) bool {
	data, err := os.ReadFile(s.getLockFile())
	if err != nil {
		return false
	}
	got, err := strconv.Atoi(strings.TrimSpace(string(data)))
	return err == nil && got == pid
}
```

In `server/internal/updater/downloader.go`, replace the loop of `downloadBinaries` with:

```go
	for _, d := range Daemons {
		target := filepath.Join(s.getFilesDir(), d.Name)
		// Step 2 of a self-update is this daemon's binary of the release it
		// installs; fetching that asset again would download the same bytes.
		if s.selfBinary != "" && d.Name == s.daemon {
			if err := linkOrCopy(s.selfBinary, target); err != nil {
				return fmt.Errorf("take %s from %s: %w", d.Name, s.selfBinary, err)
			}
			continue
		}
		assetName := d.Name + "-" + suffix
		url := assetURL(release, assetName)
		if url == "" {
			return fmt.Errorf("asset %s not found in release", assetName)
		}
		if err := requireHTTPS(url); err != nil {
			return fmt.Errorf("asset %s: %w", assetName, err)
		}
		if err := s.downloadFile(ctx, url, target); err != nil {
			return fmt.Errorf("download %s: %w", assetName, err)
		}
	}
	return nil
```

and add at the end of the file:

```go
// linkOrCopy puts the file src at dst: a hard link when both sit on one
// filesystem, as the installer and files/ do in /tmp, else a copy.
func linkOrCopy(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	if err := os.Link(src, dst); err == nil {
		return nil
	}
	return copyExecutable(src, dst)
}

// copyExecutable copies src to dst, creating dst with mode 0755.
func copyExecutable(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	return out.Close()
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run (from `server/`): `go test ./internal/updater -count=1`
Expected: PASS, the golden test included.

- [ ] **Step 5: Commit**

```bash
git add server/internal/updater/github.go server/internal/updater/github_test.go \
  server/internal/updater/updater.go server/internal/updater/updater_test.go \
  server/internal/updater/downloader.go server/internal/updater/downloader_test.go
git commit -F - <<'EOF'
feat(updater): fetch a release by tag and take the own binary from the executable

Step 2 of the self-update handover installs the release it belongs to,
so it fetches that release by tag, and it links itself into files/ as
its daemon's payload binary instead of downloading the same bytes.

Claude-Session: https://claude.ai/code/session_01UfSG5SeDGVee1x5doaXHzb
EOF
```

### Task 2: Step 2 — the self-update entry point and its contract

**Files:**
- Create: `server/internal/updater/selfupdate.go`
- Create: `server/internal/updater/selfupdate_test.go`
- Create: `server/internal/updater/testdata/selfupdate_argv.txt`
- Modify: `server/internal/updater/updater.go`

**Interfaces:**
- Consumes (Task 1): `GetReleaseByTag`, `lockNamesPID`, `NewForDaemon`, `DaemonBot`, `DaemonWebUI`, `Service.daemon`, `Service.selfBinary`; existing `DownloadRelease`, `RunUpdateScript`, `IsValidVersion`, `RunOptions`.
- Produces:
  - `const SelfUpdateCommand = "self-update"`
  - `func RunSelfUpdate(args []string, daemon, version string, stdout, stderr io.Writer) int`
  - `func selfUpdateArgv(opts RunOptions) []string` — used by step 1 in Task 4
  - `type selfUpdateArgs struct{ From, To, Initiator string; ChatID int64 }`, `func parseSelfUpdateArgs(args []string) (selfUpdateArgs, error)`
  - `Service` seams `parentPID func() int`, `executable func() (string, error)`

- [ ] **Step 1: Write the failing tests and the argv record**

Create `server/internal/updater/testdata/selfupdate_argv.txt`:

```text
# Every form of the self-update invocation a released step 1 builds, oldest
# first, for RunOptions{OldVersion: "v1.2.3", NewVersion: "v1.2.4", ChatID: 42,
# Initiator: "bot"}. Step 2 of every later release must parse every line.
# Append a line when step 1 changes; never edit or remove one.
self-update --from v1.2.3 --to v1.2.4 --initiator bot --chat-id 42
```

Create `server/internal/updater/selfupdate_test.go`:

```go
package updater

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestParseSelfUpdateArgs_AcceptsTheInvocationStep1Builds(t *testing.T) {
	argv := selfUpdateArgv(RunOptions{OldVersion: "v1.2.3", NewVersion: "v1.2.4", ChatID: 42, Initiator: "bot"})
	if argv[0] != SelfUpdateCommand {
		t.Fatalf("argv[0] = %q, want %q", argv[0], SelfUpdateCommand)
	}
	got, err := parseSelfUpdateArgs(argv[1:])
	if err != nil {
		t.Fatalf("parseSelfUpdateArgs() error = %v", err)
	}
	if want := (selfUpdateArgs{From: "v1.2.3", To: "v1.2.4", Initiator: "bot", ChatID: 42}); got != want {
		t.Errorf("parsed %+v, want %+v", got, want)
	}
}

func TestParseSelfUpdateArgs_RefusesWhatTheScriptCannotTake(t *testing.T) {
	valid := []string{"--from", "v1.2.3", "--to", "v1.2.4", "--initiator", "webui", "--chat-id", "0"}
	with := func(name, value string) []string {
		out := append([]string(nil), valid...)
		for i := 0; i < len(out); i += 2 {
			if out[i] == name {
				out[i+1] = value
			}
		}
		return out
	}
	for name, args := range map[string][]string{
		"shell in --from":     with("--from", "v1.2.3;id"),
		"empty --to":          with("--to", ""),
		"unknown initiator":   with("--initiator", "cron"),
		"non-numeric chat id": with("--chat-id", "forty-two"),
		"unknown flag":        append(append([]string(nil), valid...), "--force"),
		"positional argument": append(append([]string(nil), valid...), "extra"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseSelfUpdateArgs(args); err == nil {
				t.Errorf("parseSelfUpdateArgs(%q) accepted it", args)
			}
		})
	}
}

// TestSelfUpdateArgv_EveryReleasedFormStillParses holds step 2 to the
// invocations released step 1s make. A router cannot update the step 1 it
// runs, so every line of the file is an invocation some router makes for as
// long as it exists. The file only grows.
func TestSelfUpdateArgv_EveryReleasedFormStillParses(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "selfupdate_argv.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var forms [][]string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		forms = append(forms, strings.Fields(line))
	}
	if len(forms) == 0 {
		t.Fatal("testdata/selfupdate_argv.txt lists no released invocation")
	}
	for _, form := range forms {
		if form[0] != SelfUpdateCommand {
			t.Errorf("released form %q does not start with %q", strings.Join(form, " "), SelfUpdateCommand)
			continue
		}
		if _, err := parseSelfUpdateArgs(form[1:]); err != nil {
			t.Errorf("released form %q no longer parses: %v", strings.Join(form, " "), err)
		}
	}
	got := strings.Join(selfUpdateArgv(RunOptions{OldVersion: "v1.2.3", NewVersion: "v1.2.4", ChatID: 42, Initiator: "bot"}), " ")
	if newest := strings.Join(forms[len(forms)-1], " "); got != newest {
		t.Errorf("step 1 builds %q, but the newest released form is %q: append the new form as a line, never edit one", got, newest)
	}
}

// step2Fixture is a step 2 with every path in a temp dir: it runs as the webui
// binary of v1.2.4, its parent (PID 4242) holds the lock, and its update script
// runs under an interpreter that does nothing.
type step2Fixture struct {
	s         *Service
	dir       string
	self      string
	requested func() []string
}

// newStep2Fixture serves the GitHub API and the raw files over TLS. onRequest,
// when set, sees every request path and the lock file before it is answered.
func newStep2Fixture(t *testing.T, onRequest func(path, lockFile string)) *step2Fixture {
	t.Helper()
	dir := t.TempDir()
	lock := filepath.Join(dir, "lock")
	const parent = 4242
	if err := os.WriteFile(lock, []byte(strconv.Itoa(parent)), 0644); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var requested []string
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requested = append(requested, r.URL.Path)
		mu.Unlock()
		if onRequest != nil {
			onRequest(r.URL.Path, lock)
		}
		switch {
		case r.URL.Path == "/repos/zinin/vpn-director/releases/tags/v1.2.4":
			fmt.Fprintf(w, `{"tag_name": "v1.2.4", "assets": [
				{"name": "telegram-bot-arm64", "browser_download_url": "%[1]s/assets/telegram-bot-arm64"},
				{"name": "webui-arm64", "browser_download_url": "%[1]s/assets/webui-arm64"}]}`, server.URL)
		case strings.HasSuffix(r.URL.Path, "/"+manifestPath):
			w.Write([]byte(testManifest))
		case strings.HasPrefix(r.URL.Path, "/assets/"):
			w.Write([]byte("downloaded " + strings.TrimPrefix(r.URL.Path, "/assets/")))
		default:
			w.Write([]byte("#!/bin/sh\n"))
		}
	}))
	t.Cleanup(server.Close)

	self := filepath.Join(dir, "installer")
	if err := os.WriteFile(self, []byte("the new webui"), 0755); err != nil {
		t.Fatal(err)
	}
	noop := filepath.Join(dir, "noop-sh")
	if err := os.WriteFile(noop, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	s := &Service{
		httpClient: server.Client(),
		baseURL:    server.URL,
		rawBaseURL: server.URL,
		updateDir:  dir,
		lockFile:   lock,
		scriptFile: filepath.Join(dir, "update.sh"),
		shell:      noop,
		archSuffix: "arm64",
		platform:   "merlin",
		daemon:     DaemonWebUI,
		parentPID:  func() int { return parent },
		executable: func() (string, error) { return self, nil },
	}
	return &step2Fixture{s: s, dir: dir, self: self, requested: func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), requested...)
	}}
}

// step2Args is what step 1 passes for an update from v1.2.3 to to, started
// from the Web UI.
func step2Args(to string) []string {
	return selfUpdateArgv(RunOptions{OldVersion: "v1.2.3", NewVersion: to, ChatID: 0, Initiator: "webui"})[1:]
}

func TestSelfUpdate_InstallsItsOwnReleaseAndStartsTheScript(t *testing.T) {
	f := newStep2Fixture(t, nil)
	var stdout bytes.Buffer

	if err := f.s.selfUpdate(context.Background(), step2Args("v1.2.4"), "v1.2.4", &stdout); err != nil {
		t.Fatalf("selfUpdate() error = %v", err)
	}

	if got := stdout.String(); got != "Files downloaded, starting update...\n" {
		t.Errorf("stdout = %q, want the one progress line", got)
	}
	selfInfo, err := os.Stat(f.self)
	if err != nil {
		t.Fatal(err)
	}
	ownInfo, err := os.Stat(filepath.Join(f.dir, "files", DaemonWebUI))
	if err != nil || !os.SameFile(selfInfo, ownInfo) {
		t.Errorf("files/%s is not this binary (%v)", DaemonWebUI, err)
	}
	if data, err := os.ReadFile(filepath.Join(f.dir, "files", DaemonBot)); err != nil || string(data) != "downloaded telegram-bot-arm64" {
		t.Errorf("files/%s = %q (%v), want the release asset", DaemonBot, data, err)
	}
	// The template is unchanged and pinned byte for byte by
	// TestGenerateScript_Golden; this run renders it with temp paths, so it
	// checks what this run fed into it.
	script, err := os.ReadFile(f.s.getScriptFile())
	if err != nil {
		t.Fatalf("no update script: %v", err)
	}
	for _, want := range []string{`OLD_VERSION="v1.2.3"`, `NEW_VERSION="v1.2.4"`, `INITIATOR="webui"`, "CHAT_ID=0"} {
		if !strings.Contains(string(script), want) {
			t.Errorf("update script lacks %s", want)
		}
	}
	for _, p := range f.requested() {
		if strings.Contains(p, "webui-arm64") {
			t.Errorf("step 2 downloaded %s, the binary it is", p)
		}
	}
}

func TestSelfUpdate_RefusesAReleaseOtherThanItsOwn(t *testing.T) {
	f := newStep2Fixture(t, nil)

	err := f.s.selfUpdate(context.Background(), step2Args("v1.2.4"), "v1.2.5", &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "v1.2.5") || !strings.Contains(err.Error(), "v1.2.4") {
		t.Fatalf("selfUpdate() error = %v, want a refusal naming both versions", err)
	}
	if got := f.requested(); len(got) != 0 {
		t.Errorf("a refused step 2 made requests: %v", got)
	}
}

func TestSelfUpdate_RefusesUnlessItsParentHoldsTheLock(t *testing.T) {
	f := newStep2Fixture(t, nil)
	f.s.parentPID = func() int { return 4343 }

	err := f.s.selfUpdate(context.Background(), step2Args("v1.2.4"), "v1.2.4", &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "lock") {
		t.Fatalf("selfUpdate() error = %v, want the lock refusal", err)
	}
	if got := f.requested(); len(got) != 0 {
		t.Errorf("step 2 without the lock made requests: %v", got)
	}
}

// The download can take minutes. A claim lost meanwhile - the parent died and
// another update took the directory - must not get the script started.
func TestSelfUpdate_ChecksTheLockAgainBeforeTheScript(t *testing.T) {
	f := newStep2Fixture(t, func(path, lockFile string) {
		if strings.HasSuffix(path, "/"+manifestPath) {
			os.WriteFile(lockFile, []byte("1"), 0644)
		}
	})

	err := f.s.selfUpdate(context.Background(), step2Args("v1.2.4"), "v1.2.4", &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "lock") {
		t.Fatalf("selfUpdate() error = %v, want the lock refusal", err)
	}
	if _, err := os.Stat(f.s.getScriptFile()); !os.IsNotExist(err) {
		t.Error("the update script was written for a claim that was lost")
	}
}

// Step 1 shows the user the last line step 2 prints on stderr and reads
// progress from stdout, so a refusal belongs on stderr, with exit status 1.
func TestRunSelfUpdate_ReportsARefusalOnStderr(t *testing.T) {
	for name, args := range map[string][]string{
		"bad arguments":   {"--to", "v1.2.4;id"},
		"another version": step2Args("v1.0.0"),
	} {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := RunSelfUpdate(args, DaemonBot, "v1.2.4", &stdout, &stderr); code != 1 {
				t.Errorf("RunSelfUpdate() = %d, want 1", code)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want nothing: it is the progress channel", stdout.String())
			}
			if strings.TrimSpace(stderr.String()) == "" {
				t.Error("stderr is empty, so step 1 has no reason to show")
			}
		})
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run (from `server/`): `go test ./internal/updater -count=1 -run 'SelfUpdate'`
Expected: FAIL to compile — `undefined: selfUpdateArgv`, `undefined: SelfUpdateCommand`, `unknown field parentPID`.

- [ ] **Step 3: Implement**

In `server/internal/updater/updater.go`, add two fields at the end of the `Service` struct:

```go
	parentPID  func() int             // Injectable for testing, nil = os.Getppid
	executable func() (string, error) // Injectable for testing, nil = os.Executable
```

and after `getPlatform`:

```go
// getParentPID returns the PID of the process that started this one: for
// step 2 of a self-update, the daemon running step 1.
func (s *Service) getParentPID() int {
	if s.parentPID != nil {
		return s.parentPID()
	}
	return os.Getppid()
}

// getExecutable returns the path of the running binary.
func (s *Service) getExecutable() (string, error) {
	if s.executable != nil {
		return s.executable()
	}
	return os.Executable()
}
```

Create `server/internal/updater/selfupdate.go`:

```go
package updater

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
)

// The self-update handover.
//
// The daemon a user asks to update does not install the new release itself.
// Step 1 (Handover, handover.go) runs in that daemon: it downloads the new
// release's binary of its own daemon as InstallerFile and runs
//
//	installer self-update --from <vX.Y.Z> --to <vX.Y.Z> --initiator <bot|webui> --chat-id <int64>
//
// Step 2 (RunSelfUpdate, below) runs in that new binary and installs the
// release with its own code: DownloadRelease, then RunUpdateScript with its own
// template. A release therefore installs itself, and may change its file list,
// its daemons, its manifest and its update script at will.
//
// Step 1 of a release starts step 2 of every later release, and a router
// cannot update the step 1 it runs. This is the contract between the two.
// Later releases may add to it; they must never remove or rename anything in
// it. testdata/selfupdate_argv.txt holds every invocation a released step 1
// builds, and step 2 must parse all of them.
//
//  1. Asset: step 1 runs "<daemon>-<arch>" of its own daemon, else of another
//     daemon in its Daemons table. Every such binary implements self-update
//     and stays below maxFileSize.
//  2. Invocation: the argv above, working directory "/", stdin /dev/null,
//     the daemon's environment. --chat-id is 0 when the Web UI started it.
//  3. Result: every stdout line is a progress line for the user. Exit 0 means
//     the update script has started and owns the lock and files/. Any other
//     exit means no script started and nothing outside UpdateDir changed; the
//     last non-empty stderr line is the reason shown to the user.
//  4. Files: UpdateDir, and LockFile holding the PID of its owner - step 1
//     while step 2 runs, then the script. notify.json only gains fields.
//  5. Version: step 2 refuses unless --to is the version compiled into it.
//
// Step 2 never logs: its stderr is the channel for the reason in item 3.

// SelfUpdateCommand is the first argument that makes a daemon binary run
// step 2 instead of starting as a daemon.
const SelfUpdateCommand = "self-update"

// selfUpdateArgs is the parsed invocation of step 2.
type selfUpdateArgs struct {
	From      string
	To        string
	Initiator string
	ChatID    int64
}

// selfUpdateArgv is the invocation step 1 builds. Changing it means appending
// the new form to testdata/selfupdate_argv.txt.
func selfUpdateArgv(opts RunOptions) []string {
	return []string{
		SelfUpdateCommand,
		"--from", opts.OldVersion,
		"--to", opts.NewVersion,
		"--initiator", opts.Initiator,
		"--chat-id", strconv.FormatInt(opts.ChatID, 10),
	}
}

// parseSelfUpdateArgs parses the arguments after SelfUpdateCommand. The values
// end up in a shell script, so they pass the checks RunUpdateScript makes.
func parseSelfUpdateArgs(args []string) (selfUpdateArgs, error) {
	var a selfUpdateArgs
	fs := flag.NewFlagSet(SelfUpdateCommand, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&a.From, "from", "", "version being replaced")
	fs.StringVar(&a.To, "to", "", "version to install, the one compiled into this binary")
	fs.StringVar(&a.Initiator, "initiator", "", "bot or webui")
	fs.Int64Var(&a.ChatID, "chat-id", 0, "chat to report to, 0 for the Web UI")
	if err := fs.Parse(args); err != nil {
		return a, err
	}
	if fs.NArg() != 0 {
		return a, fmt.Errorf("unexpected arguments %q", fs.Args())
	}
	if !IsValidVersion(a.From) {
		return a, fmt.Errorf("invalid --from %q", a.From)
	}
	if !IsValidVersion(a.To) {
		return a, fmt.Errorf("invalid --to %q", a.To)
	}
	if a.Initiator != "bot" && a.Initiator != "webui" {
		return a, fmt.Errorf("invalid --initiator %q", a.Initiator)
	}
	return a, nil
}

// RunSelfUpdate is step 2. main calls it with os.Args[2:] when os.Args[1] is
// SelfUpdateCommand, before anything else a daemon does at startup. daemon is
// the daemon this binary is, version the version compiled into it. The result
// is the process exit code.
func RunSelfUpdate(args []string, daemon, version string, stdout, stderr io.Writer) int {
	s := NewForDaemon(daemon)
	if err := s.selfUpdate(context.Background(), args, version, stdout); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

// selfUpdate installs the release this binary belongs to.
func (s *Service) selfUpdate(ctx context.Context, args []string, version string, stdout io.Writer) error {
	a, err := parseSelfUpdateArgs(args)
	if err != nil {
		return err
	}
	if a.To != version {
		return fmt.Errorf("this binary is %s, asked to install %s", version, a.To)
	}
	if err := s.requireParentLock(); err != nil {
		return err
	}
	release, err := s.GetReleaseByTag(ctx, a.To)
	if err != nil {
		return fmt.Errorf("release %s: %w", a.To, err)
	}
	self, err := s.getExecutable()
	if err != nil {
		return fmt.Errorf("locate this binary: %w", err)
	}
	s.selfBinary = self
	if err := s.DownloadRelease(ctx, release); err != nil {
		return err
	}
	fmt.Fprintln(stdout, "Files downloaded, starting update...")
	// Checked again at the last moment: the download can take minutes, and
	// the script must not start for a claim lost meanwhile.
	if err := s.requireParentLock(); err != nil {
		return err
	}
	return s.RunUpdateScript(RunOptions{
		OldVersion: a.From,
		NewVersion: a.To,
		ChatID:     a.ChatID,
		Initiator:  a.Initiator,
	})
}

// requireParentLock refuses unless the lock names the process that started
// this step: step 1 took it before running us. A parent that died and left a
// lock another update may have replaced, or a self-update typed into a shell,
// stops here.
func (s *Service) requireParentLock() error {
	if !s.lockNamesPID(s.getParentPID()) {
		return errors.New("the update lock does not name the process that started this step")
	}
	return nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run (from `server/`): `go test ./internal/updater -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/internal/updater/selfupdate.go server/internal/updater/selfupdate_test.go \
  server/internal/updater/testdata/selfupdate_argv.txt server/internal/updater/updater.go
git commit -F - <<'EOF'
feat(updater): self-update step 2, the entry point a release installs itself with

RunSelfUpdate runs in the new release's binary: it checks the version and
the lock, fetches its release by tag, downloads the payload and starts
the update script with its own code. The doc comment of selfupdate.go is
the contract step 1 of every release relies on, and
testdata/selfupdate_argv.txt keeps every invocation a released step 1
builds.

Claude-Session: https://claude.ai/code/session_01UfSG5SeDGVee1x5doaXHzb
EOF
```

### Task 3: Both daemons run step 2 before anything else

**Files:**
- Modify: `server/cmd/bot/main.go`
- Modify: `server/cmd/webui/main.go`
- Create: `server/cmd/bot/main_test.go`
- Create: `server/cmd/webui/main_test.go`

**Interfaces:**
- Consumes (Tasks 1–2): `updater.SelfUpdateCommand`, `updater.RunSelfUpdate`, `updater.DaemonBot`, `updater.DaemonWebUI`, `updater.NewForDaemon`.
- Produces: the dispatch every later release's step 1 relies on; each daemon's update flow built with its daemon name, which Task 5 needs.

The spec's end-to-end test builds the binaries with `go build -X main.Version=v9.9.9`. These tests prove the same thing without the toolchain: the test binary re-runs itself with `main()` as its entry point (the helper-process pattern). It is a `dev` build, which step 2's version check refuses exactly as it refuses `v9.9.9`.

- [ ] **Step 1: Write the failing tests**

Create `server/cmd/bot/main_test.go`:

```go
package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// runMainEnv makes the test binary run main() instead of its tests, so a test
// can start the real entry point as a child process.
const runMainEnv = "VPD_TEST_RUN_MAIN"

func TestMain(m *testing.M) {
	if os.Getenv(runMainEnv) == "1" {
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// TestSelfUpdateIsDispatchedBeforeTheDaemonStarts pins the entry point of the
// self-update contract (internal/updater/selfupdate.go): the version being
// replaced runs this binary as "self-update ...", and nothing a daemon does at
// startup may come first. This binary is a dev build, so step 2 refuses the
// version it is asked to install - and only step 2 prints that refusal.
func TestSelfUpdateIsDispatchedBeforeTheDaemonStarts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0],
		"self-update", "--from", "v9.9.8", "--to", "v1.0.0", "--initiator", "bot", "--chat-id", "0")
	cmd.Env = append(os.Environ(), runMainEnv+"=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	err := cmd.Run()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("exit = %v, want status 1\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "asked to install v1.0.0") {
		t.Errorf("stderr = %q, want step 2's version refusal", stderr.String())
	}
}
```

Create `server/cmd/webui/main_test.go` (the two `main` packages cannot share test code):

```go
package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// runMainEnv makes the test binary run main() instead of its tests, so a test
// can start the real entry point as a child process.
const runMainEnv = "VPD_TEST_RUN_MAIN"

func TestMain(m *testing.M) {
	if os.Getenv(runMainEnv) == "1" {
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// TestSelfUpdateIsDispatchedBeforeTheDaemonStarts pins the entry point of the
// self-update contract (internal/updater/selfupdate.go): the version being
// replaced runs this binary as "self-update ...", and nothing a daemon does at
// startup may come first. This binary is a dev build, so step 2 refuses the
// version it is asked to install - and only step 2 prints that refusal.
func TestSelfUpdateIsDispatchedBeforeTheDaemonStarts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0],
		"self-update", "--from", "v9.9.8", "--to", "v1.0.0", "--initiator", "webui", "--chat-id", "0")
	cmd.Env = append(os.Environ(), runMainEnv+"=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	err := cmd.Run()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("exit = %v, want status 1\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "asked to install v1.0.0") {
		t.Errorf("stderr = %q, want step 2's version refusal", stderr.String())
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run (from `server/`): `go test ./cmd/bot ./cmd/webui -count=1 -run TestSelfUpdateIsDispatchedBeforeTheDaemonStarts`
Expected: FAIL in both packages. Without the dispatch the bot child starts as a daemon, logs to `/tmp/telegram-bot.log`, finds no config and exits 0; the Web UI child logs to `/tmp/vpn-director-webui.log`, fails to load `/opt/vpn-director/vpn-director.json` and exits 1 without the refusal on stderr. Both log lines are harmless.

- [ ] **Step 3: Implement**

In `server/cmd/bot/main.go`, make these the first statements of `main()`, before `devFlag := flag.Bool(...)`:

```go
	// Step 2 of a self-update (internal/updater/selfupdate.go): the version
	// being replaced runs this binary with these arguments before installing
	// it. Nothing a daemon does at startup may run first.
	if len(os.Args) > 1 && os.Args[1] == updater.SelfUpdateCommand {
		os.Exit(updater.RunSelfUpdate(os.Args[2:], updater.DaemonBot, Version, os.Stdout, os.Stderr))
	}
```

and replace

```go
	opts = append(opts, bot.WithUpdater(updater.New()))
```

with

```go
	opts = append(opts, bot.WithUpdater(updater.NewForDaemon(updater.DaemonBot)))
```

Leave `updatechecker.New(updater.New(), ...)` as it is: the checker only reads releases.

In `server/cmd/webui/main.go`, make these the first statements of `main()`, before `configPath := flag.String(...)`:

```go
	// Step 2 of a self-update (internal/updater/selfupdate.go): the version
	// being replaced runs this binary with these arguments before installing
	// it. Nothing a daemon does at startup may run first.
	if len(os.Args) > 1 && os.Args[1] == updater.SelfUpdateCommand {
		os.Exit(updater.RunSelfUpdate(os.Args[2:], updater.DaemonWebUI, Version, os.Stdout, os.Stderr))
	}
```

and replace

```go
	updateFlow := updateflow.New(updater.New(), Version, *devFlag)
```

with

```go
	updateFlow := updateflow.New(updater.NewForDaemon(updater.DaemonWebUI), Version, *devFlag)
```

- [ ] **Step 4: Run the tests to verify they pass**

Run (from `server/`): `go build ./... && go test ./cmd/... -count=1`
Expected: PASS in `cmd/bot` and `cmd/webui`.

- [ ] **Step 5: Commit**

```bash
git add server/cmd/bot/main.go server/cmd/bot/main_test.go \
  server/cmd/webui/main.go server/cmd/webui/main_test.go
git commit -F - <<'EOF'
feat(daemons): run self-update step 2 before anything a daemon does

Both binaries hand "self-update ..." to updater.RunSelfUpdate as their
first statement, so the version being replaced can run them as the
installer of their own release. Each daemon also builds its update flow
with its own name, which step 1 of the handover prefers.

Claude-Session: https://claude.ai/code/session_01UfSG5SeDGVee1x5doaXHzb
EOF
```

### Task 4: Step 1 — the handover

**Files:**
- Create: `server/internal/updater/handover.go`
- Create: `server/internal/updater/handover_test.go`
- Modify: `server/internal/updater/updater.go`
- Test: `server/internal/updater/updater_test.go`

**Interfaces:**
- Consumes: Task 1 (`lockNamesPID`, `Service.daemon`, `DaemonWebUI`), Task 2 (`selfUpdateArgv`); existing `downloadFile`, `assetURL`, `requireHTTPS`, `getArchSuffix`, `CleanFiles`, `RemoveLock`, `validOpts()` (test helper in `script_test.go`).
- Produces (Task 5 relies on these):
  - `func (s *Service) Handover(ctx context.Context, release *Release, opts RunOptions, progress func(string)) error`
  - `type HandoverPhase int` with `PhaseDownload`, `PhaseStart`, `PhaseInstaller`, `PhaseTimeout`
  - `type HandoverError struct{ Phase HandoverPhase; Err error }`
  - `const InstallerName = "installer"`, `const InstallerFile = UpdateDir + "/" + InstallerName`

- [ ] **Step 1: Write the failing tests**

In `server/internal/updater/updater_test.go`, add to the end of `TestGetters_Defaults`:

```go
	if got := s.getInstallerFile(); got != InstallerFile {
		t.Errorf("getInstallerFile() = %q, want %q", got, InstallerFile)
	}
	if got := s.getHandoverTimeout(); got != defaultHandoverTimeout {
		t.Errorf("getHandoverTimeout() = %v, want %v", got, defaultHandoverTimeout)
	}
```

and to the end of `TestGetters_Custom`:

```go
	if got := s.getInstallerFile(); got != "/custom/update/installer" {
		t.Errorf("getInstallerFile() = %q, want %q", got, "/custom/update/installer")
	}
```

Create `server/internal/updater/handover_test.go`:

```go
package updater

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// fakeInstaller returns a shell script to serve as a release asset. Run as the
// installer, it records its arguments and working directory under record,
// runs body and exits with code.
func fakeInstaller(record, body string, code int) string {
	return "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" > '" + record + "/argv'\n" +
		"pwd > '" + record + "/cwd'\n" +
		body + "\n" +
		"exit " + strconv.Itoa(code) + "\n"
}

// newHandoverService serves assets (name -> body) over TLS as a release and
// returns a Service for the webui daemon whose update directory, lock and
// installer live in a temp dir. The lock is taken, as Start takes it, and
// files/ holds a marker, so a test can see what a cleanup removed.
func newHandoverService(t *testing.T, assets map[string]string) (*Service, *Release) {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := assets[strings.TrimPrefix(r.URL.Path, "/")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	dir := t.TempDir()
	s := &Service{
		httpClient: server.Client(),
		updateDir:  dir,
		lockFile:   filepath.Join(dir, "lock"),
		archSuffix: "arm64",
		daemon:     DaemonWebUI,
	}
	release := &Release{TagName: "v1.1.0"}
	for name := range assets {
		release.Assets = append(release.Assets, Asset{Name: name, DownloadURL: server.URL + "/" + name})
	}
	if err := s.CreateLock(); err != nil {
		t.Fatalf("CreateLock() error = %v", err)
	}
	if err := os.MkdirAll(s.getFilesDir(), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.getFilesDir(), "marker"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	return s, release
}

// collect returns a progress func and the lines it received. Handover calls
// progress from the goroutine that copies the installer's output and returns
// only after that goroutine is done, so the lines are safe to read afterwards.
func collect() (func(string), *[]string) {
	var lines []string
	return func(l string) { lines = append(lines, l) }, &lines
}

func assertHandoverPhase(t *testing.T, err error, phase HandoverPhase) *HandoverError {
	t.Helper()
	var he *HandoverError
	if !errors.As(err, &he) {
		t.Fatalf("Handover() error = %v, want a *HandoverError", err)
	}
	if he.Phase != phase {
		t.Fatalf("Handover() failed in phase %d (%v), want phase %d", he.Phase, err, phase)
	}
	return he
}

func assertCleanedUp(t *testing.T, s *Service) {
	t.Helper()
	for what, path := range map[string]string{
		"files/":    s.getFilesDir(),
		"the lock":  s.getLockFile(),
		"installer": s.getInstallerFile(),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s survived a failed handover", what)
		}
	}
}

func TestHandover_RunsStep2OfTheNewVersionAndRelaysItsProgress(t *testing.T) {
	record := t.TempDir()
	s, release := newHandoverService(t, map[string]string{
		"webui-arm64":        fakeInstaller(record, "echo 'Files downloaded, starting update...'", 0),
		"telegram-bot-arm64": "#!/bin/sh\nexit 99\n",
	})
	progress, lines := collect()

	if err := s.Handover(context.Background(), release, validOpts(), progress); err != nil {
		t.Fatalf("Handover() error = %v", err)
	}

	argv, err := os.ReadFile(filepath.Join(record, "argv"))
	if err != nil {
		t.Fatalf("the webui's binary did not run: %v", err)
	}
	if got, want := strings.Join(strings.Fields(string(argv)), " "), strings.Join(selfUpdateArgv(validOpts()), " "); got != want {
		t.Errorf("installer argv = %q, want %q", got, want)
	}
	if cwd, _ := os.ReadFile(filepath.Join(record, "cwd")); strings.TrimSpace(string(cwd)) != "/" {
		t.Errorf("installer ran in %q, want /", strings.TrimSpace(string(cwd)))
	}
	if got := strings.Join(*lines, "\n"); got != "Files downloaded, starting update..." {
		t.Errorf("progress = %q, want the installer's line", got)
	}
	for _, leftover := range []string{s.getInstallerFile(), s.getInstallerFile() + ".part"} {
		if _, err := os.Stat(leftover); !os.IsNotExist(err) {
			t.Errorf("%s left behind", leftover)
		}
	}
	if !s.lockNamesPID(os.Getpid()) {
		t.Error("a handover that succeeded touched the lock the update script now owns")
	}
	if _, err := os.Stat(filepath.Join(s.getFilesDir(), "marker")); err != nil {
		t.Error("a handover that succeeded removed files/ from under the update script")
	}
}

func TestHandover_RunsAnotherDaemonsBinaryWhenItsOwnIsMissing(t *testing.T) {
	record := t.TempDir()
	s, release := newHandoverService(t, map[string]string{
		"telegram-bot-arm64": fakeInstaller(record, "", 0),
	})
	progress, _ := collect()

	if err := s.Handover(context.Background(), release, validOpts(), progress); err != nil {
		t.Fatalf("Handover() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(record, "argv")); err != nil {
		t.Error("the bot's binary did not run although the release lacks the webui's")
	}
}

func TestHandover_WithoutAnyDaemonBinaryIsADownloadFailure(t *testing.T) {
	s, release := newHandoverService(t, map[string]string{"xray-arm64": "x"})
	progress, lines := collect()

	err := s.Handover(context.Background(), release, validOpts(), progress)

	assertHandoverPhase(t, err, PhaseDownload)
	assertCleanedUp(t, s)
	if len(*lines) != 0 {
		t.Errorf("progress = %q from a step 2 that never ran", *lines)
	}
}

// The installer is executed as root, so the one scheme downgrade that would
// hand a network attacker that file is refused, as for the payload binaries.
func TestHandover_RefusesAPlainHTTPInstaller(t *testing.T) {
	s, release := newHandoverService(t, map[string]string{"webui-arm64": "x"})
	release.Assets[0].DownloadURL = strings.Replace(release.Assets[0].DownloadURL, "https://", "http://", 1)
	progress, _ := collect()

	err := s.Handover(context.Background(), release, validOpts(), progress)

	if he := assertHandoverPhase(t, err, PhaseDownload); !strings.Contains(he.Err.Error(), "https") {
		t.Errorf("error = %v, want it to name the scheme requirement", err)
	}
	assertCleanedUp(t, s)
}

func TestHandover_AnInstallerThatDoesNotExecuteIsAStartFailure(t *testing.T) {
	s, release := newHandoverService(t, map[string]string{"webui-arm64": "no shebang, no ELF header\n"})
	progress, _ := collect()

	err := s.Handover(context.Background(), release, validOpts(), progress)

	assertHandoverPhase(t, err, PhaseStart)
	assertCleanedUp(t, s)
}

func TestHandover_AFailingStep2IsReportedWithItsLastStderrLine(t *testing.T) {
	record := t.TempDir()
	s, release := newHandoverService(t, map[string]string{
		"webui-arm64": fakeInstaller(record, "echo 'first complaint' >&2\necho 'the real reason' >&2", 1),
	})
	progress, _ := collect()

	err := s.Handover(context.Background(), release, validOpts(), progress)

	if he := assertHandoverPhase(t, err, PhaseInstaller); he.Err.Error() != "the real reason" {
		t.Errorf("reason = %q, want the last stderr line", he.Err)
	}
	assertCleanedUp(t, s)
}

func TestHandover_KillsAStep2ThatRunsOutOfTime(t *testing.T) {
	record := t.TempDir()
	s, release := newHandoverService(t, map[string]string{
		"webui-arm64": fakeInstaller(record, "exec sleep 30", 0),
	})
	s.handoverTimeout = 200 * time.Millisecond
	progress, _ := collect()

	start := time.Now()
	err := s.Handover(context.Background(), release, validOpts(), progress)

	assertHandoverPhase(t, err, PhaseTimeout)
	if waited := time.Since(start); waited > 10*time.Second {
		t.Errorf("Handover() took %v to give up on a killed step 2", waited)
	}
	assertCleanedUp(t, s)
}

// Once the update script has republished the lock, the directory is the
// script's even if step 2 then exits non-zero: removing files/ would pull the
// payload from under the copy step.
func TestHandover_LeavesTheDirectoryToAScriptThatTookTheLock(t *testing.T) {
	record := t.TempDir()
	s, release := newHandoverService(t, map[string]string{
		"webui-arm64": fakeInstaller(record, `printf '1\n' > "$FAKE_LOCK"`, 1),
	})
	t.Setenv("FAKE_LOCK", s.getLockFile())
	progress, _ := collect()

	err := s.Handover(context.Background(), release, validOpts(), progress)

	assertHandoverPhase(t, err, PhaseInstaller)
	if _, err := os.Stat(filepath.Join(s.getFilesDir(), "marker")); err != nil {
		t.Error("files/ was removed although the lock names the update script")
	}
	if data, _ := os.ReadFile(s.getLockFile()); strings.TrimSpace(string(data)) != "1" {
		t.Errorf("lock = %q, want the script's claim left in place", data)
	}
}

func TestHandover_CapsWhatStep2CanPutInFrontOfTheUser(t *testing.T) {
	record := t.TempDir()
	body := "echo '" + strings.Repeat("я", 400) + "'\n" +
		"i=0; while [ $i -lt 14 ]; do echo \"line $i\"; i=$((i+1)); done"
	s, release := newHandoverService(t, map[string]string{"webui-arm64": fakeInstaller(record, body, 0)})
	progress, lines := collect()

	if err := s.Handover(context.Background(), release, validOpts(), progress); err != nil {
		t.Fatalf("Handover() error = %v", err)
	}

	if len(*lines) != maxProgressLines {
		t.Fatalf("progress got %d lines, want %d", len(*lines), maxProgressLines)
	}
	if first := (*lines)[0]; first != strings.Repeat("я", maxProgressRunes) {
		t.Errorf("first line has %d runes, want it cut to %d", utf8.RuneCountInString(first), maxProgressRunes)
	}
	if last := (*lines)[maxProgressLines-1]; last != "line 8" {
		t.Errorf("last forwarded line = %q, want line 8", last)
	}
}

// A process forked elsewhere in the daemon while the installer was still open
// for writing keeps it open until that process execs, and an exec of the
// installer fails with "text file busy" meanwhile (golang/go#22315). Holding a
// write descriptor here reproduces that on kernels that refuse the exec; on
// the others the first attempt already succeeds.
func TestStartInstaller_RetriesATextFileBusyExec(t *testing.T) {
	installer := filepath.Join(t.TempDir(), "installer")
	if err := os.WriteFile(installer, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	w, err := os.OpenFile(installer, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(150 * time.Millisecond)
		w.Close()
	}()

	discard := func(string) {}
	cmd, err := startInstaller(context.Background(), installer, nil, &lineWriter{line: discard}, &lineWriter{line: discard})
	if err != nil {
		t.Fatalf("startInstaller() error = %v, want it to retry past the busy text file", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Errorf("installer exit = %v", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run (from `server/`): `go test ./internal/updater -count=1 -run 'TestHandover|TestStartInstaller|TestGetters'`
Expected: FAIL to compile — `s.Handover undefined`, `undefined: HandoverPhase`, `undefined: InstallerFile`, `unknown field handoverTimeout`.

- [ ] **Step 3: Implement**

In `server/internal/updater/updater.go`, add `"time"` to the imports, and add to the path constants after `ScriptFile`:

```go
	// InstallerName is where step 1 of a self-update puts the new release's
	// binary it runs (selfupdate.go). Deliberately no daemon's name: pidof and
	// killall in the init scripts match the process name, and a process called
	// telegram-bot would pass for the running daemon.
	InstallerName = "installer"
	InstallerFile = UpdateDir + "/" + InstallerName
```

Add one field at the end of the `Service` struct:

```go
	handoverTimeout time.Duration // Injectable for testing, 0 = defaultHandoverTimeout
```

and after `getScriptFile`:

```go
// getInstallerFile returns where step 1 downloads the new release's binary.
func (s *Service) getInstallerFile() string {
	return filepath.Join(s.getUpdateDir(), InstallerName)
}

// getHandoverTimeout returns how long step 1 waits for step 2.
func (s *Service) getHandoverTimeout() time.Duration {
	if s.handoverTimeout > 0 {
		return s.handoverTimeout
	}
	return defaultHandoverTimeout
}
```

Create `server/internal/updater/handover.go`:

```go
package updater

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
)

// Limits step 1 holds step 2 to (selfupdate.go).
const (
	// defaultHandoverTimeout bounds step 2: downloading the release and
	// starting its script. The Web UI waits up to twenty minutes for an update.
	defaultHandoverTimeout = 15 * time.Minute
	// maxProgressLines and maxProgressRunes cap what step 2 can put in front
	// of the user, so a faulty step 2 cannot flood a chat. The rest is logged.
	maxProgressLines = 10
	maxProgressRunes = 300
	// maxPendingLine bounds a line still waiting for its newline.
	maxPendingLine = 64 * 1024
	// installerWaitDelay bounds the wait for step 2's output once it has
	// exited or been killed.
	installerWaitDelay = 5 * time.Second
	// installerStartAttempts retries an exec refused with "text file busy": a
	// process forked elsewhere in this daemon while the installer was still
	// open for writing holds it until that process execs (golang/go#22315).
	installerStartAttempts = 5
	installerStartBackoff  = 100 * time.Millisecond
)

// HandoverPhase tells where a handover failed, so the caller can phrase it.
type HandoverPhase int

const (
	// PhaseDownload: the new version's binary could not be downloaded.
	PhaseDownload HandoverPhase = iota + 1
	// PhaseStart: the new version's binary did not start.
	PhaseStart
	// PhaseInstaller: the new version's step 2 exited non-zero.
	PhaseInstaller
	// PhaseTimeout: step 2 ran out of time and was killed.
	PhaseTimeout
)

// HandoverError is a handover that did not get the update script started.
type HandoverError struct {
	Phase HandoverPhase
	Err   error
}

func (e *HandoverError) Error() string {
	switch e.Phase {
	case PhaseDownload:
		return "download the new version: " + e.Err.Error()
	case PhaseStart:
		return "start the new version: " + e.Err.Error()
	case PhaseTimeout:
		return "the new version timed out: " + e.Err.Error()
	default:
		return "the new version failed: " + e.Err.Error()
	}
}

func (e *HandoverError) Unwrap() error { return e.Err }

// Handover is step 1 of a self-update (selfupdate.go). It downloads this
// daemon's binary of release as the installer, runs its self-update step with
// opts and passes each line that step prints on to progress. It returns nil
// once step 2 reports the update script started; the script then owns the lock
// and files/. After a failure it removes files/ and the lock, as long as the
// lock still names this process.
func (s *Service) Handover(ctx context.Context, release *Release, opts RunOptions, progress func(string)) error {
	if progress == nil {
		progress = func(string) {}
	}
	installer := s.getInstallerFile()
	// Needed by nobody afterwards: on success step 2 has linked itself into
	// files/, and after a failure nothing runs it again.
	defer os.Remove(installer)

	if err := s.downloadInstaller(ctx, release, installer); err != nil {
		s.cleanUpOwned()
		return &HandoverError{Phase: PhaseDownload, Err: err}
	}
	if err := s.runInstaller(ctx, installer, opts, progress); err != nil {
		s.cleanUpOwned()
		return err
	}
	return nil
}

// installerAsset picks the binary step 1 runs: this daemon's, else the first
// other daemon's the release carries. Every one of them implements step 2.
func (s *Service) installerAsset(release *Release) (name, url string, err error) {
	suffix, err := s.getArchSuffix()
	if err != nil {
		return "", "", err
	}
	names := []string{s.daemon}
	for _, d := range Daemons {
		if d.Name != s.daemon {
			names = append(names, d.Name)
		}
	}
	for _, n := range names {
		if n == "" {
			continue
		}
		if u := assetURL(release, n+"-"+suffix); u != "" {
			return n + "-" + suffix, u, nil
		}
	}
	return "", "", fmt.Errorf("release %s carries no daemon binary for %s", release.TagName, suffix)
}

// downloadInstaller downloads the installer under a temporary name and renames
// it into place once it is complete, closed and executable.
func (s *Service) downloadInstaller(ctx context.Context, release *Release, installer string) error {
	name, url, err := s.installerAsset(release)
	if err != nil {
		return err
	}
	if err := requireHTTPS(url); err != nil {
		return fmt.Errorf("asset %s: %w", name, err)
	}
	part := installer + ".part"
	if err := s.downloadFile(ctx, url, part); err != nil {
		os.Remove(part)
		return fmt.Errorf("%s: %w", name, err)
	}
	if err := os.Chmod(part, 0755); err != nil {
		os.Remove(part)
		return fmt.Errorf("%s: %w", name, err)
	}
	if err := os.Rename(part, installer); err != nil {
		os.Remove(part)
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// runInstaller runs step 2 and waits for it, at most the handover timeout.
// Its stdout lines go to progress; its last stderr line is the reason a
// failure is reported with.
func (s *Service) runInstaller(ctx context.Context, installer string, opts RunOptions, progress func(string)) error {
	ctx, cancel := context.WithTimeout(ctx, s.getHandoverTimeout())
	defer cancel()

	var reason string
	stdout := &lineWriter{line: progressRelay(progress)}
	stderr := &lineWriter{line: func(l string) {
		slog.Warn("self-update step 2", "stderr", l)
		reason = l
	}}

	cmd, err := startInstaller(ctx, installer, selfUpdateArgv(opts), stdout, stderr)
	if err != nil {
		return &HandoverError{Phase: PhaseStart, Err: err}
	}
	err = cmd.Wait()
	stdout.flush()
	stderr.flush()
	// ErrWaitDelay: step 2 exited 0, and something it started still held its
	// output open. The script redirects its own, so this is not expected, but
	// the exit status is what the contract promises.
	if err == nil || errors.Is(err, exec.ErrWaitDelay) {
		return nil
	}
	if ctx.Err() != nil {
		return &HandoverError{Phase: PhaseTimeout, Err: ctx.Err()}
	}
	if reason == "" {
		reason = err.Error()
	}
	return &HandoverError{Phase: PhaseInstaller, Err: errors.New(truncateRunes(reason, maxProgressRunes))}
}

// startInstaller starts step 2 from "/", retrying while exec reports
// "text file busy". Stdin stays nil, which os/exec connects to /dev/null.
func startInstaller(ctx context.Context, installer string, argv []string, stdout, stderr *lineWriter) (*exec.Cmd, error) {
	for attempt := 1; ; attempt++ {
		cmd := exec.CommandContext(ctx, installer, argv...)
		cmd.Dir = "/"
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		cmd.WaitDelay = installerWaitDelay
		err := cmd.Start()
		if err == nil {
			return cmd, nil
		}
		if !errors.Is(err, syscall.ETXTBSY) || attempt == installerStartAttempts {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, err
		case <-time.After(installerStartBackoff):
		}
	}
}

// lineWriter hands every complete, non-empty line written to it to line,
// trimmed. A line that outgrows maxPendingLine is handed over in pieces.
type lineWriter struct {
	line    func(string)
	pending []byte
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.pending = append(w.pending, p...)
	for {
		i := bytes.IndexByte(w.pending, '\n')
		if i < 0 {
			break
		}
		w.emit(w.pending[:i])
		w.pending = w.pending[i+1:]
	}
	if len(w.pending) > maxPendingLine {
		w.emit(w.pending)
		w.pending = nil
	}
	return len(p), nil
}

// flush hands over a last line that ended without a newline.
func (w *lineWriter) flush() {
	w.emit(w.pending)
	w.pending = nil
}

func (w *lineWriter) emit(b []byte) {
	if line := strings.TrimSpace(string(b)); line != "" {
		w.line(line)
	}
}

// progressRelay passes the first maxProgressLines lines on to progress, each
// cut to maxProgressRunes, and only logs the rest.
func progressRelay(progress func(string)) func(string) {
	sent := 0
	return func(line string) {
		if sent >= maxProgressLines {
			slog.Info("self-update step 2 progress past the limit", "line", line)
			return
		}
		sent++
		progress(truncateRunes(line, maxProgressRunes))
	}
}

// truncateRunes cuts s to at most n runes.
func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// cleanUpOwned removes files/ and the lock after a failed handover, but only
// while the lock names this process. A lock naming another process means the
// update script has taken over, and the directory is its to clean.
func (s *Service) cleanUpOwned() {
	if !s.lockNamesPID(os.Getpid()) {
		slog.Warn("the update lock names another process, leaving the update directory to it",
			"lock", s.getLockFile())
		return
	}
	s.CleanFiles()
	s.RemoveLock()
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run (from `server/`): `go test ./internal/updater -count=1 -race`
Expected: PASS, with no race reported. The race detector matters here: progress is called from the goroutine os/exec copies stdout with.

- [ ] **Step 5: Commit**

```bash
git add server/internal/updater/handover.go server/internal/updater/handover_test.go \
  server/internal/updater/updater.go server/internal/updater/updater_test.go
git commit -F - <<'EOF'
feat(updater): self-update step 1, handing the update to the new binary

Handover downloads this daemon's binary of the new release as
/tmp/vpn-director-update/installer, runs its self-update step from /,
relays at most ten of its lines to the user and waits at most fifteen
minutes. After a failure it cleans up only while the lock still names
this process, so a script that has taken over keeps its payload.

Claude-Session: https://claude.ai/code/session_01UfSG5SeDGVee1x5doaXHzb
EOF
```

### Task 5: The update flow hands over instead of installing

**Files:**
- Modify: `server/internal/updater/updater.go` (the `Updater` interface)
- Modify: `server/internal/updateflow/start.go`
- Modify: `server/internal/updateflow/flow.go` (package comment)
- Test: `server/internal/updateflow/flow_test.go`, `server/internal/updateflow/start_test.go`, `server/internal/updatechecker/checker_test.go`

**Interfaces:**
- Consumes (Task 4): `Handover`, `HandoverError`, `PhaseDownload`, `PhaseStart`, `PhaseInstaller`, `PhaseTimeout`.
- Produces: `updater.Updater` without `DownloadRelease` and `RunUpdateScript` (both stay `Service` methods, called by step 2) and with `Handover`; `func handoverMessage(err error) string` in `updateflow`.

- [ ] **Step 1: Move the mocks and the tests to the handover**

In `server/internal/updatechecker/checker_test.go`, replace the six one-line methods from `IsUpdateInProgress` to `RunUpdateScript` with:

```go
func (m *mockUpdater) IsUpdateInProgress() bool { return false }
func (m *mockUpdater) CreateLock() error        { return nil }
func (m *mockUpdater) RemoveLock()              {}
func (m *mockUpdater) CleanFiles()              {}
func (m *mockUpdater) Handover(context.Context, *updater.Release, updater.RunOptions, func(string)) error {
	return nil
}
```

In `server/internal/updateflow/flow_test.go`, replace everything from the `// mockUpdater implements updater.Updater for testing.` comment through the end of the `RunUpdateScript` method with:

```go
// mockUpdater implements updater.Updater for testing.
type mockUpdater struct {
	mu sync.Mutex

	release    *updater.Release
	releaseErr error
	releases   int // GetLatestRelease call count

	// entered and block hold a GetLatestRelease call in flight: the call
	// reports itself on entered and then waits for block to be closed. A test
	// that needs several callers to contend for a cold cache uses them to keep
	// the first request from completing. Both are set before any goroutine
	// starts and are nil for every other test.
	entered chan struct{}
	block   chan struct{}

	shouldUpdate    bool
	shouldUpdateErr error

	inProgress    bool
	createLockErr error
	handoverErr   error
	// handoverLines are what the mock's step 2 prints: Handover passes each
	// to progress before it returns.
	handoverLines []string

	// handoverGate holds Handover until the test closes it, so a test can
	// make something happen - cancelling the context it passed to Start -
	// before the handover inspects what it was handed. Set before any
	// goroutine starts and nil for every other test.
	handoverGate chan struct{}

	createLockCalled bool
	handoverCalled   bool
	handoverDone     bool // Handover got past every progress line
	handoverCtxDone  bool // the handover was handed an already-cancelled context
	handoverOpts     updater.RunOptions
	cleanFilesCalled bool
	removeLockCalled bool
}

func newMockUpdater() *mockUpdater {
	return &mockUpdater{
		release:       &updater.Release{TagName: "v1.3.0", Body: "changelog text"},
		shouldUpdate:  true,
		handoverLines: []string{"Files downloaded, starting update..."},
	}
}

func (m *mockUpdater) GetLatestRelease(_ context.Context) (*updater.Release, error) {
	m.mu.Lock()
	m.releases++
	entered, block := m.entered, m.block
	release, err := m.release, m.releaseErr
	m.mu.Unlock()

	// Signal and wait outside m.mu, so a test holding a request in flight can
	// still read calls().
	if entered != nil {
		entered <- struct{}{}
	}
	if block != nil {
		<-block
	}

	if err != nil {
		return nil, err
	}
	return release, nil
}

func (m *mockUpdater) ShouldUpdate(_, _ string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.shouldUpdate, m.shouldUpdateErr
}

func (m *mockUpdater) IsUpdateInProgress() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.inProgress
}

func (m *mockUpdater) CreateLock() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createLockCalled = true
	return m.createLockErr
}

func (m *mockUpdater) RemoveLock() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeLockCalled = true
}

func (m *mockUpdater) CleanFiles() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanFilesCalled = true
}

func (m *mockUpdater) Handover(ctx context.Context, _ *updater.Release, opts updater.RunOptions, progress func(string)) error {
	m.mu.Lock()
	gate := m.handoverGate
	m.mu.Unlock()

	// Wait outside m.mu, so the test can still read the mock while the
	// handover is held.
	if gate != nil {
		<-gate
	}

	m.mu.Lock()
	m.handoverCalled = true
	m.handoverCtxDone = ctx.Err() != nil
	m.handoverOpts = opts
	lines, err := m.handoverLines, m.handoverErr
	m.mu.Unlock()

	// Outside m.mu: progress may be the test's collector or a callback that
	// panics.
	for _, line := range lines {
		progress(line)
	}

	m.mu.Lock()
	m.handoverDone = true
	m.mu.Unlock()
	return err
}
```

Replace the whole content of `server/internal/updateflow/start_test.go` with:

```go
package updateflow

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zinin/vpn-director/server/internal/updater"
)

// collectProgress returns a progress func and a getter for what it received.
func collectProgress() (func(string), func() []string) {
	var mu sync.Mutex
	var lines []string
	return func(s string) {
			mu.Lock()
			lines = append(lines, s)
			mu.Unlock()
		}, func() []string {
			mu.Lock()
			defer mu.Unlock()
			return append([]string(nil), lines...)
		}
}

// waitFor polls cond for up to a second: Start hands the update over in a
// goroutine, so the assertions have to wait for it.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestStart_HandsTheUpdateOverWithTheInitiator(t *testing.T) {
	upd := newMockUpdater()
	f := New(upd, "v1.2.0", false)
	progress, lines := collectProgress()

	res, err := f.Start(context.Background(), "webui", 0, progress)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if res.From != "v1.2.0" || res.To != "v1.3.0" {
		t.Errorf("StartResult = %+v, want v1.2.0 -> v1.3.0", res)
	}

	waitFor(t, "the handover to be reported", func() bool {
		return strings.Contains(strings.Join(lines(), "\n"), "Update script started")
	})

	upd.mu.Lock()
	opts := upd.handoverOpts
	upd.mu.Unlock()
	if opts.Initiator != "webui" || opts.ChatID != 0 {
		t.Errorf("RunOptions = %+v, want initiator webui and chat 0", opts)
	}
	if opts.OldVersion != "v1.2.0" || opts.NewVersion != "v1.3.0" {
		t.Errorf("RunOptions versions = %+v", opts)
	}
	want := "Files downloaded, starting update...\nUpdate script started, the service will restart in a few seconds..."
	if got := strings.Join(lines(), "\n"); got != want {
		t.Errorf("progress = %q, want step 2's line and then the success line", got)
	}
}

func TestStart_UpToDateKeepsTheVersionsAndTakesNoLock(t *testing.T) {
	upd := newMockUpdater()
	// Written before Start launches its goroutine, so the plain assignment is
	// safe; whatever the goroutine can touch goes through the mock's setters.
	upd.shouldUpdate = false
	f := New(upd, "v1.3.0", false)
	progress, _ := collectProgress()

	res, err := f.Start(context.Background(), "bot", 42, progress)
	if !errors.Is(err, ErrUpToDate) {
		t.Fatalf("Start() error = %v, want ErrUpToDate", err)
	}
	if res.From != "v1.3.0" || res.To != "v1.3.0" {
		t.Errorf("StartResult = %+v: the caller needs the versions for its message", res)
	}
	upd.mu.Lock()
	defer upd.mu.Unlock()
	if upd.createLockCalled {
		t.Error("an up-to-date router must not take the lock")
	}
	if upd.handoverCalled {
		t.Error("an up-to-date router must not hand over")
	}
}

func TestStart_RejectsWithoutTouchingAnything(t *testing.T) {
	tests := []struct {
		name       string
		version    string
		devMode    bool
		inProgress bool
		want       error
	}{
		{name: "dev mode", version: "v1.2.0", devMode: true, want: ErrDevMode},
		{name: "dev build", version: "dev", want: ErrDevVersion},
		{name: "already running", version: "v1.2.0", inProgress: true, want: ErrInProgress},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upd := newMockUpdater()
			upd.setInProgress(tt.inProgress)
			f := New(upd, tt.version, tt.devMode)
			progress, _ := collectProgress()

			if _, err := f.Start(context.Background(), "bot", 42, progress); !errors.Is(err, tt.want) {
				t.Fatalf("Start() error = %v, want %v", err, tt.want)
			}
			// "Touching anything" is all three: GitHub, the lock, the handover.
			if n := upd.calls(); n != 0 {
				t.Errorf("a rejected start made %d GitHub requests, want 0", n)
			}
			upd.mu.Lock()
			defer upd.mu.Unlock()
			if upd.createLockCalled {
				t.Error("rejected start must not take the lock")
			}
			if upd.handoverCalled {
				t.Error("rejected start must not hand over")
			}
		})
	}
}

func TestStart_GitHubFailureIsTyped(t *testing.T) {
	upd := newMockUpdater()
	upd.setReleaseErr(errors.New("connection refused"))
	f := New(upd, "v1.2.0", false)
	progress, _ := collectProgress()

	_, err := f.Start(context.Background(), "bot", 42, progress)
	var ghErr *GitHubError
	if !errors.As(err, &ghErr) {
		t.Fatalf("Start() error = %v, want *GitHubError", err)
	}
	upd.mu.Lock()
	defer upd.mu.Unlock()
	if upd.createLockCalled {
		t.Error("a failed check must not leave a lock behind")
	}
}

func TestStart_StaleReleaseIsRejectedBeforeTheLock(t *testing.T) {
	// Check and latestRelease read the cache under separate acquisitions of
	// f.mu, and Check holds that mutex across the network call: a forced check
	// queued behind it takes the lock the moment Check returns and can replace
	// the release while Start is between the two. Installing one release under
	// another version's name is what the guard prevents.
	upd := newMockUpdater()
	f := New(upd, "v1.2.0", false)

	if _, err := f.Check(context.Background(), false); err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	f.mu.Lock()
	f.cachedRelease = &updater.Release{TagName: "v1.4.0"}
	f.mu.Unlock()

	progress, _ := collectProgress()
	_, err := f.Start(context.Background(), "bot", 42, progress)
	if err == nil || !strings.Contains(err.Error(), "v1.4.0") {
		t.Fatalf("Start() error = %v, want the release mismatch", err)
	}
	upd.mu.Lock()
	defer upd.mu.Unlock()
	if upd.createLockCalled {
		t.Error("the mismatch must be caught before the lock is taken, so a retry is possible")
	}
	if upd.handoverCalled {
		t.Error("a mismatched release must not be installed")
	}
}

func TestStart_ProgressCallbackCannotAbortTheUpdate(t *testing.T) {
	// progress is caller-supplied: a Telegram send on a nil client, a write to
	// a closed channel, or simply no callback at all. A panic from it must be
	// absorbed where it happens. If it reached the goroutine's recover
	// instead, that recover would call CleanFiles and delete files/ from under
	// an update script that is still stopping daemons.
	//
	// The mock's step 2 prints a line through progress before Handover
	// returns, so handoverDone is set only when that panic was absorbed - a
	// positive anchor rather than a wait on the absence of something.
	tests := []struct {
		name     string
		progress func(string)
	}{
		{name: "panicking", progress: func(string) { panic("progress exploded") }},
		{name: "nil", progress: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upd := newMockUpdater()
			f := New(upd, "v1.2.0", false)

			if _, err := f.Start(context.Background(), "bot", 42, tt.progress); err != nil {
				t.Fatalf("Start() error = %v", err)
			}

			waitFor(t, "the handover to finish despite the callback", func() bool {
				upd.mu.Lock()
				defer upd.mu.Unlock()
				return upd.handoverDone
			})
			upd.mu.Lock()
			defer upd.mu.Unlock()
			if upd.cleanFilesCalled || upd.removeLockCalled {
				t.Error("a run already handed over must not be unwound by a broken callback")
			}
		})
	}
}

func TestStart_HandoverOutlivesTheCallersContext(t *testing.T) {
	// The whole point of the goroutine: a closed browser tab or a finished
	// Telegram poll cancels the request context, and the update must carry on
	// regardless.
	upd := newMockUpdater()
	upd.handoverGate = make(chan struct{})
	f := New(upd, "v1.2.0", false)
	progress, _ := collectProgress()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if _, err := f.Start(ctx, "webui", 0, progress); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	// The gate holds the handover until the caller is gone, so a handover
	// that had inherited ctx would see it already cancelled.
	cancel()
	close(upd.handoverGate)

	waitFor(t, "the update to continue past the cancelled request", func() bool {
		upd.mu.Lock()
		defer upd.mu.Unlock()
		return upd.handoverCalled
	})
	upd.mu.Lock()
	defer upd.mu.Unlock()
	if upd.handoverCtxDone {
		t.Error("run() must not hand over with the caller's context")
	}
}

func TestStart_AFailedHandoverIsReportedInTheUsersTerms(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"download", &updater.HandoverError{Phase: updater.PhaseDownload, Err: errors.New("HTTP 404")},
			"Download failed: HTTP 404"},
		{"start", &updater.HandoverError{Phase: updater.PhaseStart, Err: errors.New("exec format error")},
			"Failed to start the new version: exec format error"},
		{"step 2", &updater.HandoverError{Phase: updater.PhaseInstaller, Err: errors.New("this binary is v1.3.0, asked to install v1.4.0")},
			"Update failed: this binary is v1.3.0, asked to install v1.4.0"},
		{"timeout", &updater.HandoverError{Phase: updater.PhaseTimeout, Err: context.DeadlineExceeded},
			"Update timed out"},
		{"untyped", errors.New("boom"), "Update failed: boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upd := newMockUpdater()
			upd.handoverErr = tt.err
			upd.handoverLines = nil
			f := New(upd, "v1.2.0", false)
			progress, lines := collectProgress()

			if _, err := f.Start(context.Background(), "bot", 42, progress); err != nil {
				t.Fatalf("Start() error = %v: a failed handover is reported through progress", err)
			}

			waitFor(t, "the failure report", func() bool { return len(lines()) > 0 })
			if got := strings.Join(lines(), "\n"); got != tt.want {
				t.Errorf("progress = %q, want %q", got, tt.want)
			}
			upd.mu.Lock()
			defer upd.mu.Unlock()
			if upd.cleanFilesCalled || upd.removeLockCalled {
				t.Error("run() cleaned up by itself; that is Handover's call, which knows whether the lock is still its own")
			}
		})
	}
}

func TestStart_ExistingLockIsErrInProgress(t *testing.T) {
	upd := newMockUpdater()
	upd.createLockErr = updater.ErrLockExists
	f := New(upd, "v1.2.0", false)
	progress, _ := collectProgress()

	_, err := f.Start(context.Background(), "bot", 42, progress)
	if !errors.Is(err, ErrInProgress) {
		t.Fatalf("Start() error = %v, want ErrInProgress", err)
	}
	upd.mu.Lock()
	defer upd.mu.Unlock()
	if upd.handoverCalled {
		t.Error("no lock, no update")
	}
}

func TestStart_LockFailureIsReportedSynchronously(t *testing.T) {
	upd := newMockUpdater()
	upd.createLockErr = errors.New("lock file already exists (update in progress)")
	f := New(upd, "v1.2.0", false)
	progress, _ := collectProgress()

	_, err := f.Start(context.Background(), "bot", 42, progress)
	if err == nil || !strings.Contains(err.Error(), "lock file already exists") {
		t.Fatalf("Start() error = %v, want the lock failure", err)
	}
	upd.mu.Lock()
	defer upd.mu.Unlock()
	if upd.handoverCalled {
		t.Error("no lock, no update")
	}
}

func TestStart_RejectsUnsafeInitiator(t *testing.T) {
	upd := newMockUpdater()
	f := New(upd, "v1.2.0", false)
	progress, _ := collectProgress()

	if _, err := f.Start(context.Background(), "cron", 42, progress); err == nil {
		t.Fatal("Start() must reject an unknown initiator before taking the lock")
	}
	upd.mu.Lock()
	defer upd.mu.Unlock()
	if upd.createLockCalled {
		t.Error("a rejected initiator must not have taken the lock")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run (from `server/`): `go vet ./internal/updateflow ./internal/updatechecker`
Expected: FAIL to compile — `*mockUpdater does not implement updater.Updater (missing method DownloadRelease)`.

- [ ] **Step 3: Implement**

In `server/internal/updater/updater.go`, replace the last two methods of the `Updater` interface (`DownloadRelease` and `RunUpdateScript` with their comments) with:

```go
	// Handover runs step 1 of a self-update: the new release's binary installs
	// its own release (selfupdate.go). nil means the update script started.
	Handover(ctx context.Context, release *Release, opts RunOptions, progress func(string)) error
```

`DownloadRelease` and `RunUpdateScript` stay as `Service` methods; step 2 calls them.

In `server/internal/updateflow/flow.go`, change the package comment to:

```go
// Package updateflow owns the self-update orchestration shared by the
// Telegram bot and the Web UI: one cached check against the GitHub release
// API and one guarded handover of the update to the new release's binary.
package updateflow
```

In `server/internal/updateflow/start.go` the existing imports (`context`, `errors`, `fmt`, `log/slog`, the updater package) cover the new code. Replace the comment above `Start` with:

```go
// Start runs the pre-flight checks synchronously, takes the lock and hands the
// update over to the new release's binary in a goroutine; progress receives
// human-readable status lines. It returns as soon as the background work is
// under way, because the update script kills this very process a few seconds
// later.
```

Inside `Start`, change the initiator comment to:

```go
	// Rejected before the lock: an unknown initiator would only fail later,
	// inside the new version's self-update step, with the lock already taken.
```

Replace `run` (comment included) with:

```go
// run hands the update over to the new release's binary (updater.Handover)
// and reports how that went. It builds its own context: a closed browser tab
// or a finished Telegram poll must not abort an update that is already
// downloading. Cleaning up after a failure is Handover's: only it knows
// whether the lock is still this process's or already the update script's.
func (f *Flow) run(release *updater.Release, res StartResult, initiator string, chatID int64, progress func(string)) {
	err := f.upd.Handover(context.Background(), release, updater.RunOptions{
		OldVersion: res.From,
		NewVersion: res.To,
		ChatID:     chatID,
		Initiator:  initiator,
	}, func(line string) { report(progress, line) })
	if err != nil {
		report(progress, handoverMessage(err))
		return
	}
	report(progress, "Update script started, the service will restart in a few seconds...")
}

// handoverMessage phrases a failed handover for the user.
func handoverMessage(err error) string {
	var he *updater.HandoverError
	if !errors.As(err, &he) {
		return fmt.Sprintf("Update failed: %v", err)
	}
	switch he.Phase {
	case updater.PhaseDownload:
		return fmt.Sprintf("Download failed: %v", he.Err)
	case updater.PhaseStart:
		return fmt.Sprintf("Failed to start the new version: %v", he.Err)
	case updater.PhaseTimeout:
		return "Update timed out"
	default:
		return fmt.Sprintf("Update failed: %v", he.Err)
	}
}
```

The goroutine in `Start` and its panic recovery stay as they are.

- [ ] **Step 4: Run the tests to verify they pass**

Run (from `server/`): `go build ./... && go vet ./... && go test ./... -count=1 && go test -race ./internal/updateflow ./internal/updater -count=1`
Expected: everything PASS; `grep -rn 'DownloadRelease\|RunUpdateScript' internal/updateflow` prints nothing.

- [ ] **Step 5: Commit**

```bash
git add server/internal/updater/updater.go server/internal/updateflow/start.go \
  server/internal/updateflow/flow.go server/internal/updateflow/flow_test.go \
  server/internal/updateflow/start_test.go server/internal/updatechecker/checker_test.go
git commit -F - <<'EOF'
feat(updateflow): hand the update over to the new release instead of installing it

run() no longer downloads the release or renders the update script with
the code being replaced. It calls updater.Handover and phrases the
outcome for the user; cleaning up after a failure is Handover's, which
knows whether the lock still belongs to this process.

Claude-Session: https://claude.ai/code/session_01UfSG5SeDGVee1x5doaXHzb
EOF
```

### Task 6: Documentation

**Files:**
- Modify: `.claude/rules/telegram-bot.md`
- Modify: `.claude/rules/webui.md`
- Modify: `docs/superpowers/plans/2026-09-11-plan-1-carry-forward.md`
- Modify: `docs/superpowers/plans/2026-09-10-platform-layer-foundation.md`

**Interfaces:**
- Consumes: the names of Tasks 1–5 as the docs quote them.
- Produces: nothing code depends on.

- [ ] **Step 1: `.claude/rules/telegram-bot.md`**

In the architecture tree, insert after the line `│   │   ├── github.go         # GitHub release fetching`:

```text
│   │   ├── handover.go       # Step 1: run the new release's binary as the installer
│   │   ├── selfupdate.go     # Step 2 and the contract between the steps
```

In `## Self-Update (`/update`)`, insert a new item 3 after item 2 (`Flow.Start` creates the lock file):

```markdown
3. **Handover, step 1.** `Flow.Start`'s goroutine calls `updater.Handover` in the daemon the user pressed. It downloads that daemon's binary of the new release (`<daemon>-<arch>`, another daemon's when the release lacks it) to `/tmp/vpn-director-update/installer` and runs `installer self-update --from <current> --to <tag> --initiator <bot|webui> --chat-id <id>` from `/`. Every stdout line of the installer is a progress line: at most 10 reach the user, each cut to 300 characters. Exit 0 means the update script has started; otherwise the last stderr line is the reason. Step 1 waits at most 15 minutes. After a failure it removes `installer`, `files/` and the lock, unless the lock already names another process, which means the script has taken over. The new release therefore installs itself: items 4 and 5 run in the new binary, with its code and its template, so a fix to the update procedure takes effect in the release that ships it.
```

Renumber the current items 3–6 to 4–7. Prefix the text of the new item 4 (the one starting "Downloads go to") with:

```markdown
**Step 2** (`updater.RunSelfUpdate`, which both `main` functions run first when their first argument is `self-update`) refuses unless `--to` is its own version and the lock names its parent process, fetches the release by tag and downloads it.
```

and append to the end of that item:

```markdown
Its own daemon's binary is not downloaded: step 2 hard-links itself into `files/`.
```

After the list, before the `notify.json:` line, add the paragraph:

```markdown
The contract between the two steps is the doc comment at the top of `internal/updater/selfupdate.go`: the asset names, the invocation, what stdout, stderr and the exit status mean, the update directory and the version check. Later releases may add to it but never remove or rename anything in it, because a router cannot update the step 1 it runs. `testdata/selfupdate_argv.txt` keeps every invocation a released step 1 builds, and step 2 must parse them all. Step 2 never logs: its stderr carries the reason step 1 shows.
```

- [ ] **Step 2: `.claude/rules/webui.md`**

Under `## Update flow`, add after the first bullet (the write-deadline one):

```markdown
- The handover runs in the goroutine behind the 202. While step 2 downloads, `/api/update/status`
  reports the update running, because the lock names the live daemon that started it, and step 1
  gives step 2 at most 15 minutes, inside the page's twenty-minute wait.
```

Under `## Trust model of self-update`, append to the paragraph:

```markdown
Since the self-update handover the new release's daemon binary also runs as root before it is
installed: step 1 executes it as the installer (`internal/updater/selfupdate.go`). It is the binary
the update installs and starts anyway, so the trust does not change.
```

- [ ] **Step 3: Commit the rules**

```bash
git add .claude/rules/telegram-bot.md .claude/rules/webui.md
git commit -F - <<'EOF'
docs: describe the self-update handover

Claude-Session: https://claude.ai/code/session_01UfSG5SeDGVee1x5doaXHzb
EOF
```

- [ ] **Step 4: Close ruling R17 in the plan-1 documents**

In `docs/superpowers/plans/2026-09-11-plan-1-carry-forward.md`, insert right after the heading `## 1. Release plan — do not cut a single release from this branch`:

```markdown
> **Superseded on 2026-09-11.** The author chose one release that carries the platform layer and
> the self-update handover (`docs/superpowers/specs/2026-09-11-self-update-handover-design.md`).
> Routers on v0.11.x update with `install.sh`; their update button installs the release
> incompletely and leaves a CLI that fails at the last line of `common.sh`. The release notes say so
> in their first 500 characters, as plain text: the deployed bot cuts the changelog at 500
> characters and escapes Markdown. The rename comes first, and the tag goes on the merge commit
> right after the merge. The two-release analysis below stays as the record of why.
```

In `docs/superpowers/plans/2026-09-10-platform-layer-foundation.md`, append to the end of `## Post-plan: the whole-branch review and what remains`:

```markdown
**R17 decided on 2026-09-11:** one release, carrying the self-update handover of
`docs/superpowers/plans/2026-09-11-self-update-handover.md`; see the carry-forward, section 1.
```

- [ ] **Step 5: Commit the plan-1 documents**

```bash
git add docs/superpowers/plans/2026-09-11-plan-1-carry-forward.md \
  docs/superpowers/plans/2026-09-10-platform-layer-foundation.md
git commit -F - <<'EOF'
docs: close ruling R17 with one release and the self-update handover

Claude-Session: https://claude.ai/code/session_01UfSG5SeDGVee1x5doaXHzb
EOF
```

### Task 7: Verification sweep

**Files:** none changed unless a check fails; a fix goes into its own commit that names the check.

**Interfaces:** none.

- [ ] **Step 1: The Go suite**

Run (from `server/`, through `claude-forge:build-runner` in the main session): `go build ./... && go vet ./... && go test ./... -count=1 && go test -race ./internal/updater ./internal/updateflow ./cmd/... -count=1`
Expected: all PASS, no race.

- [ ] **Step 2: Formatting**

Run (from the repository root): `gofmt -l server/`
Expected: exactly `server/internal/wizard/handler.go` and `server/internal/ssrf/ssrf_test.go`.

- [ ] **Step 3: The shell side is untouched**

Run (from the repository root): `git diff 916bf27 --stat -- router/ install.sh server/internal/updater/update_script.sh.tmpl server/internal/updater/testdata/update_script.golden.sh`
Expected: no output. Then the Bats suite as a regression guard: `bats router/test/unit && bats router/test/integration && bats router/test/common.bats router/test/config.bats router/test/firewall.bats router/test/import_server_list.bats`
Expected: 390 tests, all PASS.

- [ ] **Step 4: The contract is wired end to end**

Run (from `server/`):
- `grep -n 'SelfUpdateCommand' cmd/bot/main.go cmd/webui/main.go` — one dispatch in each, the first statement of `main()`.
- `grep -rn 'DownloadRelease\|RunUpdateScript' internal/updateflow` — no output.
- `grep -n 'NewForDaemon' cmd/bot/main.go cmd/webui/main.go` — one each.

Expected: as listed.

## Self-review notes

- Spec coverage: section 4 step 1 → Task 4 (download, neutral name, invocation from `/`, relay, 15-minute wait, cleanup); step 2 → Task 2 (flags, lock check before and after the download, release by tag, self-link, script) with Task 1's building blocks; dispatch → Task 3; what stays → Task 5 leaves `Check`, `Start`'s checks, the script, `notify.json`, the adapters and `updatechecker` alone. Section 5 items 1–5 → `installerAsset` (1), `selfUpdateArgv`/`parseSelfUpdateArgs` and the argv record (2), `runInstaller`'s exit handling and step 2's stderr-only errors (3), `lockNamesPID` in both steps (4), the version check (5). Section 6 → Task 4's tests one for one (download, start, installer, timeout, lock taken over, progress cap, temp name) plus Task 2's refusals. Section 7 → Tasks 2–5. Section 8 (rollout) and section 9 (release context) are release work, not code; Task 6 records them. Section 11 → Task 6.
- Deviations from the spec, deliberate:
  - The end-to-end test (spec section 7) re-runs each package's own test binary instead of `go build -X main.Version=v9.9.9`: the same proof that `main` dispatches first, without calling the toolchain from a test. The binary is a dev build, which the version check refuses the same way.
  - The step 2 success test checks the values the script was rendered with; byte equality stays with `TestGenerateScript_Golden`, which renders with the default paths.
  - `startInstaller` retries an exec refused with "text file busy". The spec's guard covers the installer this process wrote; the retry covers a descriptor another fork of the daemon briefly holds.
- Names shared across tasks: `DaemonBot`, `DaemonWebUI`, `NewForDaemon`, `GetReleaseByTag`, `lockNamesPID`, `linkOrCopy`, `copyExecutable`, `Service.daemon`, `Service.selfBinary` (Task 1); `SelfUpdateCommand`, `RunSelfUpdate`, `selfUpdateArgv`, `parseSelfUpdateArgs`, `selfUpdateArgs`, `Service.parentPID`, `Service.executable` (Task 2); `InstallerName`, `InstallerFile`, `Handover`, `HandoverError`, `HandoverPhase`, `PhaseDownload`, `PhaseStart`, `PhaseInstaller`, `PhaseTimeout`, `Service.handoverTimeout`, `startInstaller`, `lineWriter` (Task 4); `handoverMessage` (Task 5).
