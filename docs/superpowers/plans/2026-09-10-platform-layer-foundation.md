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

**Files:**
- Modify: `server/go.mod`, every `server/**/*.go` that imports the module, `server/internal/updater/github.go:14`, `install.sh:25`, `README.md`, `README.ru.md`, `CLAUDE.md`
- Test: `server/internal/updater/github_test.go`

**Interfaces:**
- Produces: `repoName == "vpn-director"`; module path `github.com/zinin/vpn-director/server`; `GITHUB_REPO="zinin/vpn-director"` in `install.sh`.

- [ ] **Step 1: Rename the GitHub repository**

The user approved the rename in the design review. `gh repo rename` renames the remote repository and rewrites the `origin` URL of the current clone.

Run:
```bash
gh repo rename vpn-director --yes
git remote -v
```
Expected: both `origin` lines end with `zinin/vpn-director.git` (or `zinin/vpn-director`). If `gh` reports missing authentication, stop and ask the user to run the same command.

- [ ] **Step 2: Write the failing test**

Append to `server/internal/updater/github_test.go`:

```go
// TestRepoName_IsRenamed pins the release location: the repository was
// renamed from asuswrt-merlin-vpn-director, and an updater still pointing at
// the old name would only keep working while GitHub's redirect lasts.
func TestRepoName_IsRenamed(t *testing.T) {
	if repoName != "vpn-director" {
		t.Fatalf("repoName = %q, want vpn-director", repoName)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run (from `server/`): `go test ./internal/updater/ -run TestRepoName_IsRenamed -count=1`
Expected: FAIL with `repoName = "asuswrt-merlin-vpn-director", want vpn-director`.

- [ ] **Step 4: Change the module path and the repository name**

Run from the repository root:
```bash
cd server
sed -i 's#github.com/zinin/asuswrt-merlin-vpn-director/server#github.com/zinin/vpn-director/server#g' go.mod $(grep -rl 'asuswrt-merlin-vpn-director' --include='*.go' .)
sed -i 's/repoName         = "asuswrt-merlin-vpn-director"/repoName         = "vpn-director"/' internal/updater/github.go
grep -rn 'asuswrt-merlin-vpn-director' --include='*.go' . go.mod
cd ..
sed -i 's#GITHUB_REPO="zinin/asuswrt-merlin-vpn-director"#GITHUB_REPO="zinin/vpn-director"#' install.sh
sed -i 's#zinin/asuswrt-merlin-vpn-director#zinin/vpn-director#g' README.md README.ru.md CLAUDE.md
grep -rn 'asuswrt-merlin-vpn-director' install.sh README.md README.ru.md CLAUDE.md
```
Expected: both `grep` commands print nothing. `server/testdata/dev/bot.log` may still contain the old name; it is a log fixture and stays as is.

Then change the first line of `server/go.mod` if `sed` did not touch it: it must read `module github.com/zinin/vpn-director/server`.

- [ ] **Step 5: Run the Go and Bats suites**

Run (from `server/`): `go build ./... && go vet ./... && go test ./... -count=1`
Expected: PASS, including `TestRepoName_IsRenamed`.

Run (from the root): `bats router/test/unit && bats router/test/integration`
Expected: all tests pass (the installer test file reads `install.sh`, which changed only in a string).

- [ ] **Step 6: Commit**

```bash
git add server/go.mod server/cmd server/internal install.sh README.md README.ru.md CLAUDE.md
git commit -m "chore: rename the repository to vpn-director

The project now targets Keenetic as well as Asuswrt-Merlin. GitHub redirects
the old name, so installed routers keep updating.

Claude-Session: https://claude.ai/code/session_01SygLygzpzxRtxoBqZsZuvL"
```

---

### Task 2: File manifest and manifest-driven install.sh

**Files:**
- Create: `router/files.manifest`
- Modify: `install.sh:20-26` (globals), `install.sh:134-177` (`download_scripts`), `install.sh:118-131` (`create_directories`)
- Test: `router/test/unit/install.bats`

**Interfaces:**
- Produces: `router/files.manifest` (format in Global Constraints); shell functions `manifest_files <manifest> <platform>` (prints repo paths, one per line, in manifest order) and `manifest_is_executable <path>` (returns 0 for executables, 1 for `.template`, `.json`, `.manifest`); globals `PLATFORM` (`merlin` for now) and `INSTALL_ROOT` (empty in production, a temp dir in tests) in `install.sh`.

- [ ] **Step 1: Write the manifest**

Create `router/files.manifest`:

```
# VPN Director file manifest: one file per line, "<tag> <repo path>".
# Tags: common (every platform), merlin (Asuswrt-Merlin), keenetic (KeeneticOS).
# Install destination is "/" plus the path without the "router/" prefix.
# Files are installed executable unless they end in .template, .json or .manifest.
# install.sh and the Go updater both read this file; there is no other list.
common   router/opt/vpn-director/vpn-director.sh
common   router/opt/vpn-director/configure.sh
common   router/opt/vpn-director/import_server_list.sh
common   router/opt/vpn-director/setup_telegram_bot.sh
common   router/opt/vpn-director/vpn-director.json.template
common   router/opt/vpn-director/lib/common.sh
common   router/opt/vpn-director/lib/firewall.sh
common   router/opt/vpn-director/lib/config.sh
common   router/opt/vpn-director/lib/ipset.sh
common   router/opt/vpn-director/lib/tunnel.sh
common   router/opt/vpn-director/lib/tproxy.sh
common   router/opt/vpn-director/lib/xrayconf.sh
common   router/opt/vpn-director/lib/send-email.sh
common   router/opt/etc/xray/config.json.template
common   router/opt/etc/init.d/S99vpn-director
common   router/opt/etc/init.d/S98telegram-bot
common   router/opt/etc/init.d/S98vpn-director-webui
merlin   router/jffs/scripts/firewall-start
merlin   router/jffs/scripts/wan-event
```

- [ ] **Step 2: Write the failing tests**

Append to `router/test/unit/install.bats`:

```bash
# ============================================================================
# files.manifest parsing
# ============================================================================

write_manifest() {
    cat > "$BATS_TEST_TMPDIR/files.manifest" <<'EOF'
# comment line
common   router/opt/vpn-director/vpn-director.sh

merlin   router/jffs/scripts/firewall-start
keenetic router/opt/etc/ndm/netfilter.d/50-vpn-director.sh
common   router/opt/etc/xray/config.json.template
EOF
}

@test "manifest_files: prints common and platform paths in manifest order" {
    load_installer
    write_manifest
    run manifest_files "$BATS_TEST_TMPDIR/files.manifest" merlin
    assert_success
    assert_line --index 0 "router/opt/vpn-director/vpn-director.sh"
    assert_line --index 1 "router/jffs/scripts/firewall-start"
    assert_line --index 2 "router/opt/etc/xray/config.json.template"
    refute_output --partial "netfilter.d"
}

@test "manifest_files: selects the other platform's files with its tag" {
    load_installer
    write_manifest
    run manifest_files "$BATS_TEST_TMPDIR/files.manifest" keenetic
    assert_success
    assert_line --index 1 "router/opt/etc/ndm/netfilter.d/50-vpn-director.sh"
    refute_output --partial "firewall-start"
}

@test "manifest_is_executable: scripts and init files are executable" {
    load_installer
    run manifest_is_executable "router/opt/vpn-director/lib/common.sh"
    assert_success
    run manifest_is_executable "router/opt/etc/init.d/S99vpn-director"
    assert_success
    run manifest_is_executable "router/jffs/scripts/wan-event"
    assert_success
}

@test "manifest_is_executable: templates, json and the manifest are data" {
    load_installer
    run manifest_is_executable "router/opt/vpn-director/vpn-director.json.template"
    assert_failure
    run manifest_is_executable "router/opt/etc/xray/config.json.template"
    assert_failure
    run manifest_is_executable "router/opt/vpn-director/data/servers.json"
    assert_failure
    run manifest_is_executable "router/files.manifest"
    assert_failure
}

# fake_curl serves "$REPO_URL/<path>" from $BATS_TEST_TMPDIR/repo/<path>.
# install.sh calls: curl -fsSL "$url" -o "$target"
fake_curl() {
    mkdir -p "$BATS_TEST_TMPDIR/bin"
    cat > "$BATS_TEST_TMPDIR/bin/curl" <<EOF
#!/bin/bash
out=""; url=""
while [[ \$# -gt 0 ]]; do
    case "\$1" in
        -o) out="\$2"; shift 2 ;;
        -*) shift ;;
        *)  url="\$1"; shift ;;
    esac
done
src="$BATS_TEST_TMPDIR/repo/\${url#*/refs/tags/v1.0.0/}"
[[ -f "\$src" ]] || exit 22
cp "\$src" "\$out"
EOF
    chmod +x "$BATS_TEST_TMPDIR/bin/curl"
    export PATH="$BATS_TEST_TMPDIR/bin:$PATH"
}

@test "download_scripts: installs the manifest's common and platform files under INSTALL_ROOT" {
    load_installer
    fake_curl
    local repo="$BATS_TEST_TMPDIR/repo/router"
    mkdir -p "$repo/opt/vpn-director/lib" "$repo/jffs/scripts" "$repo/opt/etc/ndm/netfilter.d"
    cat > "$BATS_TEST_TMPDIR/repo/router/files.manifest" <<'EOF'
common   router/opt/vpn-director/lib/common.sh
common   router/opt/vpn-director/vpn-director.json.template
merlin   router/jffs/scripts/firewall-start
keenetic router/opt/etc/ndm/netfilter.d/50-vpn-director.sh
EOF
    echo "lib" > "$repo/opt/vpn-director/lib/common.sh"
    echo "{}" > "$repo/opt/vpn-director/vpn-director.json.template"
    echo "hook" > "$repo/jffs/scripts/firewall-start"
    echo "ndm" > "$repo/opt/etc/ndm/netfilter.d/50-vpn-director.sh"

    REPO_URL="https://raw.example/zinin/vpn-director/refs/tags/v1.0.0"
    PLATFORM="merlin"
    INSTALL_ROOT="$BATS_TEST_TMPDIR/root"

    run download_scripts
    assert_success
    [ -x "$INSTALL_ROOT/opt/vpn-director/lib/common.sh" ]
    [ -f "$INSTALL_ROOT/opt/vpn-director/vpn-director.json.template" ]
    [ ! -x "$INSTALL_ROOT/opt/vpn-director/vpn-director.json.template" ]
    [ -x "$INSTALL_ROOT/jffs/scripts/firewall-start" ]
    [ ! -e "$INSTALL_ROOT/opt/etc/ndm/netfilter.d/50-vpn-director.sh" ]
}

@test "download_scripts: fails when the manifest cannot be downloaded" {
    load_installer
    fake_curl
    mkdir -p "$BATS_TEST_TMPDIR/repo/router"
    REPO_URL="https://raw.example/zinin/vpn-director/refs/tags/v1.0.0"
    PLATFORM="merlin"
    INSTALL_ROOT="$BATS_TEST_TMPDIR/root"
    run download_scripts
    assert_failure
    assert_output --partial "files.manifest"
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `bats router/test/unit/install.bats`
Expected: the six new tests fail (`manifest_files: command not found` and similar); the existing ones pass.

- [ ] **Step 4: Implement the manifest-driven installer**

In `install.sh`, replace the globals block (lines 20-26) with:

```bash
# Installation paths
VPD_DIR="/opt/vpn-director"
JFFS_HOOKS_DIR="/jffs/scripts"
XRAY_CONFIG_DIR="/opt/etc/xray"
GITHUB_REPO="zinin/vpn-director"
INIT_DIR="/opt/etc/init.d"
WEBUI_URL=""   # set by start_webui once the daemon answers; read by print_next_steps

# Platform tag that selects files from router/files.manifest. Detection of
# Keenetic lands together with its platform module; until then this installer
# serves Asuswrt-Merlin only.
PLATFORM="merlin"

# Root the manifest files are installed under: "" on a router, a temporary
# directory in tests.
INSTALL_ROOT="${INSTALL_ROOT:-}"
```

Replace `download_scripts()` (lines 134-177) with:

```bash
###############################################################################
# File manifest
###############################################################################

# manifest_files <manifest> <platform>
#   Prints the repository paths tagged "common" or <platform>, one per line,
#   in manifest order. Blank lines and "#" comments are skipped.
manifest_files() {
    local manifest="$1" platform="$2" tag path
    while read -r tag path _; do
        case "$tag" in ''|'#'*) continue ;; esac
        if [[ $tag == common || $tag == "$platform" ]]; then
            printf '%s\n' "$path"
        fi
    done < "$manifest"
}

# manifest_is_executable <repo path>
#   Returns 0 when the installed file gets the executable bit: everything
#   except templates, JSON and the manifest itself.
manifest_is_executable() {
    case "$1" in
        *.template|*.json|*.manifest) return 1 ;;
    esac
    return 0
}

###############################################################################
# Download scripts
###############################################################################

download_scripts() {
    print_info "Downloading file manifest..."

    local manifest
    manifest=$(mktemp)
    if ! curl -fsSL "$REPO_URL/router/files.manifest" -o "$manifest"; then
        rm -f "$manifest"
        print_error "Failed to download router/files.manifest"
        return 1
    fi

    print_info "Downloading scripts..."
    local file target
    while IFS= read -r file; do
        target="${INSTALL_ROOT}/${file#router/}"
        mkdir -p "$(dirname "$target")"
        if ! curl -fsSL "$REPO_URL/$file" -o "$target"; then
            rm -f "$manifest"
            print_error "Failed to download $file"
            return 1
        fi
        if manifest_is_executable "$file"; then
            chmod +x "$target"
        fi
        print_success "Installed $target"
    done < <(manifest_files "$manifest" "$PLATFORM")

    rm -f "$manifest"
}
```

`main()` calls `download_scripts` under `set -e`, so a `return 1` still aborts the installer as the old `exit 1` did. The xray template is now a manifest entry, so the separate `curl` for it is gone with the old function body.

`create_directories()` keeps its `mkdir -p` lines; `download_scripts` creates any other parent directory itself.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `bats router/test/unit/install.bats`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add router/files.manifest install.sh router/test/unit/install.bats
git commit -m "feat(install): drive file downloads from router/files.manifest

One list of shipped files with platform tags replaces the loop hard-coded in
install.sh. The Go updater switches to the same manifest next.

Claude-Session: https://claude.ai/code/session_01SygLygzpzxRtxoBqZsZuvL"
```

---

### Task 3: Updater downloads the release by its manifest

**Files:**
- Create: `server/internal/updater/manifest.go`, `server/internal/updater/manifest_test.go`
- Modify: `server/internal/updater/downloader.go:24-72` (`scriptFiles`, `DownloadRelease`), `server/internal/updater/updater.go:101-110` (`Service` fields)
- Test: `server/internal/updater/downloader_test.go` (rewrite `TestDownloadRelease_UsesTheInjectedRawHost`; delete `TestScriptFiles_ExistInRepo`, `TestScriptFiles_IncludesWebUIInitScript`, `TestScriptFiles_MatchInstallSh`)

**Interfaces:**
- Produces:
  - `type ManifestEntry struct { Tag, Path string }`
  - `func parseManifest(r io.Reader) ([]ManifestEntry, error)`
  - `func manifestFilesFor(entries []ManifestEntry, platform string) []string`
  - `func isExecutable(path string) bool`
  - `const manifestPath = "router/files.manifest"`
  - `func (s *Service) fetchManifest(ctx context.Context, tag string) ([]ManifestEntry, error)` (downloads to `files/files.manifest` and parses it)
  - `func (s *Service) getPlatform() string` (field `platform string` on `Service`; empty means `"merlin"`)
- Consumes: `repoName` from Task 1; `router/files.manifest` from Task 2.

- [ ] **Step 1: Write the failing tests**

Create `server/internal/updater/manifest_test.go`:

```go
package updater

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseManifest(t *testing.T) {
	in := "# comment\ncommon   router/opt/vpn-director/vpn-director.sh\n\n  merlin router/jffs/scripts/firewall-start\n"
	got, err := parseManifest(strings.NewReader(in))
	if err != nil {
		t.Fatalf("parseManifest() error = %v", err)
	}
	want := []ManifestEntry{
		{Tag: "common", Path: "router/opt/vpn-director/vpn-director.sh"},
		{Tag: "merlin", Path: "router/jffs/scripts/firewall-start"},
	}
	if len(got) != len(want) {
		t.Fatalf("parseManifest() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestParseManifest_RejectsMalformedLines(t *testing.T) {
	// The manifest comes from the network and its paths end up in a script
	// that runs as root, so anything but "<tag> router/<clean path>" is refused.
	for _, line := range []string{
		"common",
		"common router/a extra",
		"common ../etc/passwd",
		"common router/opt/../../etc/passwd",
		"common /opt/vpn-director/x.sh",
		"common router/opt/with space.sh",
	} {
		if _, err := parseManifest(strings.NewReader(line + "\n")); err == nil {
			t.Errorf("parseManifest(%q) accepted a malformed line", line)
		}
	}
}

func TestManifestFilesFor(t *testing.T) {
	entries := []ManifestEntry{
		{Tag: "common", Path: "router/a"},
		{Tag: "keenetic", Path: "router/k"},
		{Tag: "merlin", Path: "router/m"},
		{Tag: "common", Path: "router/b"},
	}
	got := manifestFilesFor(entries, "merlin")
	want := []string{"router/a", "router/m", "router/b"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("manifestFilesFor(merlin) = %v, want %v", got, want)
	}
	got = manifestFilesFor(entries, "keenetic")
	want = []string{"router/a", "router/k", "router/b"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("manifestFilesFor(keenetic) = %v, want %v", got, want)
	}
}

func TestIsExecutable(t *testing.T) {
	cases := map[string]bool{
		"router/opt/vpn-director/lib/common.sh":            true,
		"router/opt/etc/init.d/S99vpn-director":            true,
		"router/jffs/scripts/wan-event":                     true,
		"router/opt/vpn-director/vpn-director.json.template": false,
		"router/opt/etc/xray/config.json.template":         false,
		"router/opt/vpn-director/data/servers.json":        false,
		"router/files.manifest":                            false,
	}
	for path, want := range cases {
		if got := isExecutable(path); got != want {
			t.Errorf("isExecutable(%q) = %v, want %v", path, got, want)
		}
	}
}

// TestRepoManifest_EntriesExist pins the manifest to the working tree: a
// renamed or removed file would otherwise break every future update.
func TestRepoManifest_EntriesExist(t *testing.T) {
	f, err := os.Open(filepath.Join("..", "..", "..", manifestPath))
	if err != nil {
		t.Fatalf("open repo manifest: %v", err)
	}
	defer f.Close()
	entries, err := parseManifest(f)
	if err != nil {
		t.Fatalf("parse repo manifest: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("repo manifest is empty")
	}
	for _, e := range entries {
		if _, err := os.Stat(filepath.Join("..", "..", "..", e.Path)); err != nil {
			t.Errorf("manifest entry %q not found in the repo: %v", e.Path, err)
		}
	}
}

// TestRepoManifest_ShipsUpdateCriticalFiles: an update that leaves an old init
// script or the CLI behind is worse than no update.
func TestRepoManifest_ShipsUpdateCriticalFiles(t *testing.T) {
	f, err := os.Open(filepath.Join("..", "..", "..", manifestPath))
	if err != nil {
		t.Fatalf("open repo manifest: %v", err)
	}
	defer f.Close()
	entries, err := parseManifest(f)
	if err != nil {
		t.Fatalf("parse repo manifest: %v", err)
	}
	have := map[string]string{}
	for _, e := range entries {
		have[e.Path] = e.Tag
	}
	for _, want := range []string{
		"router/opt/vpn-director/vpn-director.sh",
		"router/opt/etc/init.d/S99vpn-director",
		"router/opt/etc/init.d/S98telegram-bot",
		"router/opt/etc/init.d/S98vpn-director-webui",
		"router/opt/etc/xray/config.json.template",
	} {
		if have[want] != "common" {
			t.Errorf("manifest must ship %s with tag common, got %q", want, have[want])
		}
	}
	if have["router/jffs/scripts/firewall-start"] != "merlin" {
		t.Errorf("firewall-start must be tagged merlin, got %q", have["router/jffs/scripts/firewall-start"])
	}
}
```

Replace `TestDownloadRelease_UsesTheInjectedRawHost` in `downloader_test.go` with:

```go
const testManifestBody = "common router/opt/vpn-director/vpn-director.sh\n" +
	"common router/opt/vpn-director/lib/common.sh\n" +
	"merlin router/jffs/scripts/firewall-start\n" +
	"keenetic router/opt/etc/ndm/netfilter.d/50-vpn-director.sh\n"

// TestDownloadRelease_FetchesManifestThenPlatformFiles drives the whole
// download through a server the test controls: the manifest comes first, then
// every common and merlin file in manifest order, and nothing tagged keenetic.
func TestDownloadRelease_FetchesManifestThenPlatformFiles(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		if strings.HasSuffix(r.URL.Path, "/"+manifestPath) {
			w.Write([]byte(testManifestBody))
			return
		}
		w.Write([]byte("content"))
	}))
	defer server.Close()

	tempDir := t.TempDir()
	s := &Service{
		httpClient: &http.Client{},
		baseURL:    server.URL,
		rawBaseURL: server.URL,
		updateDir:  tempDir,
		archSuffix: "arm64",
		platform:   "merlin",
	}

	// No assets in the release, so downloadBinaries fails after the files.
	_ = s.DownloadRelease(context.Background(), &Release{TagName: "v1.0.0"})

	mu.Lock()
	defer mu.Unlock()
	prefix := "/" + repoOwner + "/" + repoName + "/refs/tags/v1.0.0/"
	want := []string{
		prefix + manifestPath,
		prefix + "router/opt/vpn-director/vpn-director.sh",
		prefix + "router/opt/vpn-director/lib/common.sh",
		prefix + "router/jffs/scripts/firewall-start",
	}
	if strings.Join(paths, "\n") != strings.Join(want, "\n") {
		t.Fatalf("request paths:\n%s\nwant:\n%s", strings.Join(paths, "\n"), strings.Join(want, "\n"))
	}
	for _, f := range []string{"files/files.manifest", "files/opt/vpn-director/lib/common.sh", "files/jffs/scripts/firewall-start"} {
		if _, err := os.Stat(filepath.Join(tempDir, f)); err != nil {
			t.Errorf("%s not written: %v", f, err)
		}
	}
}

func TestDownloadRelease_FailsWithoutManifest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	s := &Service{
		httpClient: &http.Client{},
		baseURL:    server.URL,
		rawBaseURL: server.URL,
		updateDir:  t.TempDir(),
		archSuffix: "arm64",
	}
	err := s.DownloadRelease(context.Background(), &Release{TagName: "v1.0.0"})
	if err == nil || !strings.Contains(err.Error(), "files.manifest") {
		t.Fatalf("DownloadRelease() error = %v, want one naming files.manifest", err)
	}
}

func TestGetPlatform_DefaultsToMerlin(t *testing.T) {
	if got := (&Service{}).getPlatform(); got != "merlin" {
		t.Errorf("getPlatform() = %q, want merlin", got)
	}
	if got := (&Service{platform: "keenetic"}).getPlatform(); got != "keenetic" {
		t.Errorf("getPlatform() = %q, want the injected platform", got)
	}
}
```

Delete `TestScriptFiles_ExistInRepo`, `TestScriptFiles_IncludesWebUIInitScript` and `TestScriptFiles_MatchInstallSh` from `downloader_test.go` (the two `TestRepoManifest_*` tests replace them). Remove the now unused `regexp` import if nothing else in the file uses it.

- [ ] **Step 2: Run the tests to verify they fail**

Run (from `server/`): `go test ./internal/updater/ -count=1`
Expected: compilation errors for `ManifestEntry`, `parseManifest`, `manifestFilesFor`, `isExecutable`, `manifestPath`, `platform`.

- [ ] **Step 3: Implement the manifest package file**

Create `server/internal/updater/manifest.go`:

```go
package updater

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// manifestPath is where a release lists the files it ships, relative to the
// repository root. install.sh reads the same file.
const manifestPath = "router/files.manifest"

// ManifestEntry is one line of router/files.manifest: a platform tag
// ("common", "merlin", "keenetic") and a repository path under router/.
type ManifestEntry struct {
	Tag  string
	Path string
}

// manifestPathRe admits the characters repository paths use. The manifest is
// downloaded from the network and its paths are pasted into a shell script
// that runs as root, so nothing else gets through.
var manifestPathRe = regexp.MustCompile(`^router/[A-Za-z0-9._/-]+$`)

// parseManifest reads "<tag> <path>" lines. Blank lines and lines starting
// with # are skipped; any other shape is an error naming the line.
func parseManifest(r io.Reader) ([]ManifestEntry, error) {
	var entries []ManifestEntry
	sc := bufio.NewScanner(r)
	line := 0
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		fields := strings.Fields(text)
		if len(fields) != 2 {
			return nil, fmt.Errorf("files.manifest line %d: want \"<tag> <path>\", got %q", line, text)
		}
		if !manifestPathRe.MatchString(fields[1]) || strings.Contains(fields[1], "..") {
			return nil, fmt.Errorf("files.manifest line %d: invalid path %q", line, fields[1])
		}
		entries = append(entries, ManifestEntry{Tag: fields[0], Path: fields[1]})
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read files.manifest: %w", err)
	}
	return entries, nil
}

// manifestFilesFor returns the paths tagged "common" or platform, in
// manifest order. Unknown tags are ignored: a release may list files for a
// platform this binary does not know yet.
func manifestFilesFor(entries []ManifestEntry, platform string) []string {
	var files []string
	for _, e := range entries {
		if e.Tag == "common" || e.Tag == platform {
			files = append(files, e.Path)
		}
	}
	return files
}

// isExecutable reports whether an installed file gets the executable bit:
// every shipped file except templates, JSON and the manifest itself.
func isExecutable(path string) bool {
	for _, suffix := range []string{".template", ".json", ".manifest"} {
		if strings.HasSuffix(path, suffix) {
			return false
		}
	}
	return true
}
```

- [ ] **Step 4: Switch DownloadRelease to the manifest**

In `server/internal/updater/updater.go`, add a field to `Service` (after `shell`):

```go
	platform   string // Injectable for testing, empty = "merlin" until platform detection lands
```

and, next to `getShell`, the accessor:

```go
// getPlatform returns the manifest tag of the platform this daemon runs on.
// Detection is wired in with the Keenetic daemon work; until then every
// installed router is an Asuswrt-Merlin one.
func (s *Service) getPlatform() string {
	if s.platform != "" {
		return s.platform
	}
	return "merlin"
}
```

In `server/internal/updater/downloader.go`, delete the `scriptFiles` variable and its comment (lines 24-48) and replace `DownloadRelease` with:

```go
// DownloadRelease downloads the release manifest, then every file it lists
// for this platform, then the daemon binaries. Cleans files/ before starting.
func (s *Service) DownloadRelease(ctx context.Context, release *Release) error {
	filesDir := s.getFilesDir()

	// Clean before download to ensure fresh state
	os.RemoveAll(filesDir)

	// Create directory structure
	if err := os.MkdirAll(filesDir, 0755); err != nil {
		return fmt.Errorf("create files directory: %w", err)
	}

	entries, err := s.fetchManifest(ctx, release.TagName)
	if err != nil {
		return err
	}

	for _, file := range manifestFilesFor(entries, s.getPlatform()) {
		if err := s.downloadScriptFile(ctx, release.TagName, file); err != nil {
			return fmt.Errorf("download %s: %w", file, err)
		}
	}

	// Download daemon binaries
	if err := s.downloadBinaries(ctx, release); err != nil {
		return fmt.Errorf("download binaries: %w", err)
	}

	return nil
}

// fetchManifest downloads router/files.manifest of the release into files/
// (the update script reads it from there) and parses it. A release without a
// manifest cannot be installed.
func (s *Service) fetchManifest(ctx context.Context, tag string) ([]ManifestEntry, error) {
	if err := s.downloadScriptFile(ctx, tag, manifestPath); err != nil {
		return nil, fmt.Errorf("release %s has no files.manifest: %w", tag, err)
	}
	f, err := os.Open(filepath.Join(s.getFilesDir(), "files.manifest"))
	if err != nil {
		return nil, fmt.Errorf("open downloaded files.manifest: %w", err)
	}
	defer f.Close()
	return parseManifest(f)
}
```

`downloadScriptFile` already maps `router/files.manifest` to `files/files.manifest` by trimming the `router` prefix.

- [ ] **Step 5: Run the tests to verify they pass**

Run (from `server/`): `go vet ./... && go test ./internal/updater/ -count=1`
Expected: PASS. Then `go test ./... -count=1` for the whole module: PASS.

- [ ] **Step 6: Commit**

```bash
git add server/internal/updater/manifest.go server/internal/updater/manifest_test.go server/internal/updater/downloader.go server/internal/updater/downloader_test.go server/internal/updater/updater.go
git commit -m "feat(updater): download release files by the release's manifest

The updater no longer embeds a file list: it fetches router/files.manifest at
the release tag and takes the common files plus those of its platform. A file
added in a release therefore needs no prior binary update.

Claude-Session: https://claude.ai/code/session_01SygLygzpzxRtxoBqZsZuvL"
```

---

### Task 4: Update script installs the manifest files

**Files:**
- Modify: `server/internal/updater/manifest.go` (add `FileEntry`, `fileEntries`), `server/internal/updater/script.go:28-40,116-140` (`scriptData`, `generateScript`), `server/internal/updater/update_script.sh.tmpl:16-20,235-258`
- Test: `server/internal/updater/script_test.go`, `server/internal/updater/manifest_test.go`

**Interfaces:**
- Produces:
  - `type FileEntry struct { Src, Dst string; Exec bool }` (`Src` relative to `FILES_DIR`, `Dst` absolute)
  - `func fileEntries(entries []ManifestEntry, platform string) []FileEntry`
  - `func (s *Service) loadPayloadManifest() ([]ManifestEntry, error)`
  - `scriptData.Files []FileEntry`; template variable `FILES` with `src|dst|x` or `src|dst|-` words
- Consumes: `parseManifest`, `manifestFilesFor`, `isExecutable`, `getPlatform` from Task 3.

- [ ] **Step 1: Write the failing tests**

Append to `server/internal/updater/manifest_test.go`:

```go
func TestFileEntries(t *testing.T) {
	entries := []ManifestEntry{
		{Tag: "common", Path: "router/opt/vpn-director/lib/common.sh"},
		{Tag: "common", Path: "router/opt/vpn-director/vpn-director.json.template"},
		{Tag: "merlin", Path: "router/jffs/scripts/firewall-start"},
		{Tag: "keenetic", Path: "router/opt/etc/ndm/netfilter.d/50-vpn-director.sh"},
	}
	got := fileEntries(entries, "merlin")
	want := []FileEntry{
		{Src: "opt/vpn-director/lib/common.sh", Dst: "/opt/vpn-director/lib/common.sh", Exec: true},
		{Src: "opt/vpn-director/vpn-director.json.template", Dst: "/opt/vpn-director/vpn-director.json.template", Exec: false},
		{Src: "jffs/scripts/firewall-start", Dst: "/jffs/scripts/firewall-start", Exec: true},
	}
	if len(got) != len(want) {
		t.Fatalf("fileEntries() = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}
```

Add to `server/internal/updater/script_test.go`, after `assertExecRefused`:

```go
// testManifest is the payload manifest the script tests install from.
const testManifest = "common router/opt/vpn-director/vpn-director.sh\n" +
	"common router/opt/vpn-director/lib/common.sh\n" +
	"common router/opt/vpn-director/vpn-director.json.template\n" +
	"common router/opt/etc/init.d/S99vpn-director\n" +
	"merlin router/jffs/scripts/firewall-start\n" +
	"keenetic router/opt/etc/ndm/netfilter.d/50-vpn-director.sh\n"

// writeTestManifest gives the update payload the manifest generateScript
// reads; DownloadRelease writes it there in production.
func writeTestManifest(t *testing.T, s *Service) {
	t.Helper()
	dir := s.getFilesDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir files dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "files.manifest"), []byte(testManifest), 0644); err != nil {
		t.Fatalf("write files.manifest: %v", err)
	}
}

func TestGenerateScript_CopiesManifestFiles(t *testing.T) {
	s := &Service{updateDir: t.TempDir()}
	writeTestManifest(t, s)

	script, err := s.generateScript(validOpts())
	if err != nil {
		t.Fatalf("generateScript() error = %v", err)
	}
	for _, want := range []string{
		"opt/vpn-director/lib/common.sh|/opt/vpn-director/lib/common.sh|x",
		"opt/vpn-director/vpn-director.json.template|/opt/vpn-director/vpn-director.json.template|-",
		"opt/etc/init.d/S99vpn-director|/opt/etc/init.d/S99vpn-director|x",
		"jffs/scripts/firewall-start|/jffs/scripts/firewall-start|x",
		`mkdir -p "$(dirname "$dst")"`,
		`cp -f "$FILES_DIR/$src" "$dst"`,
	} {
		if !strings.Contains(script, want) {
			t.Errorf("script missing %q", want)
		}
	}
	for _, stale := range []string{
		"netfilter.d",
		`cp -f "$FILES_DIR/jffs/scripts/"*`,
		`cp -f "$FILES_DIR/opt/vpn-director/lib/"*.sh`,
		"chmod +x /jffs/scripts/firewall-start",
	} {
		if strings.Contains(script, stale) {
			t.Errorf("script still contains %q", stale)
		}
	}
}

func TestGenerateScript_FailsWithoutPayloadManifest(t *testing.T) {
	s := &Service{updateDir: t.TempDir()}
	if _, err := s.generateScript(validOpts()); err == nil || !strings.Contains(err.Error(), "files.manifest") {
		t.Fatalf("generateScript() error = %v, want one naming files.manifest", err)
	}
}
```

Then, in every existing test of `script_test.go` that calls `generateScript` or `RunUpdateScript`, add `writeTestManifest(t, s)` right after the `Service` is constructed (`s := &Service{...}` or `s := noExecService(t)`). Find them with `grep -n 'generateScript(\|RunUpdateScript(' server/internal/updater/script_test.go`; the list at the time of writing is `TestGenerateScript`, `TestGenerateScript_PathsCorrect`, `TestGenerateScript_EmbedsInitiator`, `TestGenerateScript_CoversEveryDaemon`, `TestGenerateScript_RecoveryOnFailure`, `TestGenerateScript_ReleasesApplyLockBeforeStartingDaemons`, `TestGenerateScript_UsesPOSIXFileDescriptors`, `TestGenerateScript_CommitsTheFailedStatusBeforeRestarting`, `TestGenerateScript_RestoresMonitLast` and the `RunUpdateScript` tests built on `noExecService`. Tests that assert a validation error from `RunUpdateScript` before any script is generated need no manifest but are harmless with one.

- [ ] **Step 2: Run the tests to verify they fail**

Run (from `server/`): `go test ./internal/updater/ -count=1`
Expected: compilation errors for `FileEntry` and `fileEntries`; after adding stubs, `TestGenerateScript_CopiesManifestFiles` and `TestGenerateScript_FailsWithoutPayloadManifest` fail.

- [ ] **Step 3: Implement file entries and the script table**

Append to `server/internal/updater/manifest.go`:

```go
// FileEntry is one file the update script installs: Src relative to the
// payload's files/ directory, Dst absolute on the router.
type FileEntry struct {
	Src  string
	Dst  string
	Exec bool
}

// fileEntries turns the manifest lines for platform into copy instructions.
func fileEntries(entries []ManifestEntry, platform string) []FileEntry {
	var files []FileEntry
	for _, p := range manifestFilesFor(entries, platform) {
		rel := strings.TrimPrefix(p, "router/")
		files = append(files, FileEntry{Src: rel, Dst: "/" + rel, Exec: isExecutable(p)})
	}
	return files
}
```

In `server/internal/updater/script.go`:

Add `Files []FileEntry` to `scriptData` after `Daemons`:

```go
	Daemons    []Daemon
	Files      []FileEntry
```

Add after `updateCommand`:

```go
// loadPayloadManifest reads files/files.manifest of the downloaded release.
// DownloadRelease always writes it before RunUpdateScript can run; its absence
// means the payload is not a release this binary can install.
func (s *Service) loadPayloadManifest() ([]ManifestEntry, error) {
	f, err := os.Open(filepath.Join(s.getFilesDir(), "files.manifest"))
	if err != nil {
		return nil, fmt.Errorf("update payload has no files.manifest: %w", err)
	}
	defer f.Close()
	return parseManifest(f)
}
```

In `generateScript`, before `data := scriptData{...}`:

```go
	entries, err := s.loadPayloadManifest()
	if err != nil {
		return "", err
	}
```

and add `Files: fileEntries(entries, s.getPlatform()),` to the `scriptData` literal.

In `server/internal/updater/update_script.sh.tmpl`, after the `DAEMONS=` line (line 20) add:

```sh
# File table: "src|dst|mode" entries separated by spaces, src relative to
# FILES_DIR, mode "x" for executable or "-" for data. Word splitting again, so
# no field may contain a space; the manifest parser guarantees that.
FILES="{{range $i, $f := .Files}}{{if $i}} {{end}}{{$f.Src}}|{{$f.Dst}}|{{if $f.Exec}}x{{else}}-{{end}}{{end}}"
```

Replace section 4 and section 5 (the block from `# 4. Copy files` through the end of the `for entry in $DAEMONS; do ... chmod ... done` loop of section 5) with:

```sh
# 4. Copy files - NO || true, fail on error
log "Copying files"
for entry in $FILES; do
    src="${entry%%|*}"
    rest="${entry#*|}"
    dst="${rest%%|*}"
    mode="${rest#*|}"
    mkdir -p "$(dirname "$dst")"
    cp -f "$FILES_DIR/$src" "$dst"
    if [ "$mode" = "x" ]; then
        chmod +x "$dst"
    fi
done
for entry in $DAEMONS; do
    rest="${entry#*|}"
    cp -f "$FILES_DIR/${entry%%|*}" "${rest%%|*}"
done

# 5. Set permissions of the daemon binaries (script files got theirs above)
for entry in $DAEMONS; do
    rest="${entry#*|}"
    chmod +x "$INIT_DIR/${entry##*|}"
    chmod +x "${rest%%|*}"
done
```

- [ ] **Step 4: Run the tests to verify they pass**

Run (from `server/`): `go vet ./... && go test ./... -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/internal/updater/manifest.go server/internal/updater/manifest_test.go server/internal/updater/script.go server/internal/updater/script_test.go server/internal/updater/update_script.sh.tmpl
git commit -m "feat(updater): install files from the payload manifest

The update script copies every manifest file to its destination and sets the
executable bit per file, replacing the hard-coded cp and chmod lines that knew
only /jffs/scripts and a flat lib/ directory.

Claude-Session: https://claude.ai/code/session_01SygLygzpzxRtxoBqZsZuvL"
```

---

### Task 5: Shell platform layer with the Merlin implementation

**Files:**
- Create: `router/opt/vpn-director/lib/platform.sh`, `router/opt/vpn-director/lib/platform/merlin.sh`, `router/test/unit/platform.bats`, `router/test/unit/platform_merlin.bats`, `router/test/mocks/cru`
- Modify: `router/test/test_helper.bash` (export `VPD_PLATFORM`), `router/test/mocks/nvram` (three keys), `router/files.manifest` (two entries)

**Interfaces:**
- Produces (all in the sourcing shell after `. lib/platform.sh`):
  - `platform_detect` → prints `merlin` or `keenetic`, returns 1 when neither; honours `VPD_PROBE_ROOT`
  - `VPD_PLATFORM` exported
  - Contract functions of spec 5.2 with the exact names: `platform_name`, `platform_wan_if`, `platform_ipv6_enabled`, `platform_lan_ifaces`, `platform_tunnels`, `platform_tunnel_iface <id>`, `platform_tunnel_info <id>` (three lines: type, connected `1|0`, description), `platform_tunnel_table <id> <idx>`, `platform_tunnel_route <id>`, `platform_tunnel_route_ensure <id> <idx>`, `platform_tunnel_table_release <id> <idx>`, `platform_vpn_endpoints`, `platform_load_module <name>`, `platform_cron_add <name> <schedule> <cmd>`, `platform_cron_del <name>`, `platform_tproxy_extra_rules apply|stop`, `platform_prerouting_base_pos`, `platform_password_file`, `platform_lan_ip`, `platform_hostname`, `platform_model`, `platform_email_supported`
- Consumes: nothing from earlier tasks. `platform.sh` is sourced by `common.sh` in Task 6; this task tests it standalone.

- [ ] **Step 1: Extend the mocks and the test helper**

In `router/test/mocks/nvram`, add three cases before the `*) echo "" ;;` line:

```bash
    "get lan_hostname") echo "RT-AX88U-1234" ;;
    "get wgc1_desc")    echo "Office WG" ;;
    "get vpn_client1_desc") echo "Office OVPN" ;;
```

Create `router/test/mocks/cru` (executable):

```bash
#!/bin/bash
# Mock cru (Asuswrt-Merlin cron helper) - records the call and succeeds
echo "cru $*" >> /tmp/bats_cru_calls.log
exit 0
```

Run `chmod +x router/test/mocks/cru`.

In `router/test/test_helper.bash`, after `export LOG_FILE=...` add:

```bash
# Platform under test. lib/platform.sh skips detection when this is set, so
# the suite never depends on /jffs or /opt/etc/ndm of the machine running it.
export VPD_PLATFORM="${VPD_PLATFORM:-merlin}"
```

- [ ] **Step 2: Write the failing tests**

Create `router/test/unit/platform.bats`:

```bash
#!/usr/bin/env bats

load '../test_helper'

# lib/platform.sh is sourced directly here; common.sh sources it in production.
load_platform() {
    source "$LIB_DIR/platform.sh"
}

# ============================================================================
# platform_detect - file-system probes, prefixed by VPD_PROBE_ROOT
# ============================================================================

@test "platform_detect: keenetic when /opt/etc/ndm and /bin/ndmc exist" {
    load_platform
    local root="$BATS_TEST_TMPDIR/root"
    mkdir -p "$root/opt/etc/ndm" "$root/bin"
    printf '#!/bin/sh\n' > "$root/bin/ndmc"
    chmod +x "$root/bin/ndmc"
    VPD_PROBE_ROOT="$root" run platform_detect
    assert_success
    assert_output "keenetic"
}

@test "platform_detect: merlin when /jffs and /bin/nvram exist" {
    load_platform
    local root="$BATS_TEST_TMPDIR/root"
    mkdir -p "$root/jffs" "$root/bin"
    printf '#!/bin/sh\n' > "$root/bin/nvram"
    chmod +x "$root/bin/nvram"
    VPD_PROBE_ROOT="$root" run platform_detect
    assert_success
    assert_output "merlin"
}

@test "platform_detect: keenetic wins when both trees exist" {
    load_platform
    local root="$BATS_TEST_TMPDIR/root"
    mkdir -p "$root/jffs" "$root/opt/etc/ndm" "$root/bin"
    printf '#!/bin/sh\n' > "$root/bin/nvram"
    printf '#!/bin/sh\n' > "$root/bin/ndmc"
    chmod +x "$root/bin/nvram" "$root/bin/ndmc"
    VPD_PROBE_ROOT="$root" run platform_detect
    assert_output "keenetic"
}

@test "platform_detect: fails and prints nothing when neither tree exists" {
    load_platform
    mkdir -p "$BATS_TEST_TMPDIR/empty"
    VPD_PROBE_ROOT="$BATS_TEST_TMPDIR/empty" run platform_detect
    assert_failure
    refute_output
}

# ============================================================================
# Loader
# ============================================================================

@test "platform.sh: VPD_PLATFORM set by the environment is used as is" {
    load_platform
    run platform_name
    assert_success
    assert_output "merlin"
}

@test "platform.sh: an unsupported VPD_PLATFORM aborts the sourcing script" {
    run env VPD_PLATFORM=amiga bash -c "set -e; source '$LIB_DIR/platform.sh'; echo reached"
    assert_failure
    assert_output --partial "unsupported platform: amiga"
    refute_output --partial "reached"
}

@test "platform.sh: a failed detection aborts the sourcing script" {
    mkdir -p "$BATS_TEST_TMPDIR/empty"
    run env -u VPD_PLATFORM VPD_PROBE_ROOT="$BATS_TEST_TMPDIR/empty" \
        bash -c "set -e; source '$LIB_DIR/platform.sh'; echo reached"
    assert_failure
    assert_output --partial "unsupported platform"
    refute_output --partial "reached"
}

@test "platform.sh: detection fills VPD_PLATFORM when it is unset" {
    local root="$BATS_TEST_TMPDIR/root"
    mkdir -p "$root/jffs" "$root/bin"
    printf '#!/bin/sh\n' > "$root/bin/nvram"
    chmod +x "$root/bin/nvram"
    run env -u VPD_PLATFORM VPD_PROBE_ROOT="$root" \
        bash -c "set -e; source '$LIB_DIR/platform.sh'; printf '%s\n' \"\$VPD_PLATFORM\""
    assert_success
    assert_output "merlin"
}

@test "platform.sh: a platform without an implementation file is reported" {
    run env VPD_PLATFORM=keenetic bash -c "set -e; source '$LIB_DIR/platform.sh'; echo reached"
    # lib/platform/keenetic.sh does not exist yet in this plan
    if [[ -f "$LIB_DIR/platform/keenetic.sh" ]]; then
        skip "keenetic.sh exists now; this test belongs to the era before it"
    fi
    assert_failure
    assert_output --partial "platform implementation not found"
    refute_output --partial "reached"
}
```

Create `router/test/unit/platform_merlin.bats`:

```bash
#!/usr/bin/env bats

load '../test_helper'

load_platform() {
    export VPD_PLATFORM=merlin
    source "$LIB_DIR/platform.sh"
}

# with_mock <name> <script body> puts a one-off mock first in PATH.
with_mock() {
    mkdir -p "$BATS_TEST_TMPDIR/mock"
    printf '#!/bin/bash\n%s\n' "$2" > "$BATS_TEST_TMPDIR/mock/$1"
    chmod +x "$BATS_TEST_TMPDIR/mock/$1"
    export PATH="$BATS_TEST_TMPDIR/mock:$PATH"
}

@test "platform_name: merlin" {
    load_platform
    run platform_name
    assert_output "merlin"
}

# ---------------------------------------------------------------- WAN / IPv6

@test "platform_wan_if: interface of the primary WAN" {
    load_platform
    run platform_wan_if
    assert_success
    assert_output "eth0"
}

@test "platform_wan_if: falls back to wan0_ifname when no WAN is primary" {
    load_platform
    with_mock nvram 'case "$*" in "get wan0_ifname") echo eth9 ;; *) echo "" ;; esac'
    run platform_wan_if
    assert_success
    assert_output "eth9"
}

@test "platform_wan_if: fails and prints nothing when nvram knows no WAN" {
    load_platform
    with_mock nvram 'echo ""'
    run platform_wan_if
    assert_failure
    refute_output
}

@test "platform_ipv6_enabled: 1 when ipv6_service is set" {
    load_platform
    run platform_ipv6_enabled
    assert_output "1"
}

@test "platform_ipv6_enabled: 0 when ipv6_service is disabled or empty" {
    load_platform
    with_mock nvram 'case "$*" in "get ipv6_service") echo disabled ;; *) echo "" ;; esac'
    run platform_ipv6_enabled
    assert_output "0"
    with_mock nvram 'echo ""'
    run platform_ipv6_enabled
    assert_output "0"
}

@test "platform_lan_ifaces: br0" {
    load_platform
    run platform_lan_ifaces
    assert_output "br0"
}

# ------------------------------------------------------------------ tunnels

@test "platform_tunnels: wgc, then ovpnc from rt_tables, then main" {
    load_platform
    run platform_tunnels
    assert_success
    assert_line --index 0 "wgc1"
    assert_line --index 1 "wgc2"
    assert_line --index 2 "ovpnc1"
    assert_line --index 3 "ovpnc2"
    assert_line --index 4 "main"
}

@test "platform_tunnels: only main when rt_tables is missing" {
    load_platform
    RT_TABLES_FILE="$BATS_TEST_TMPDIR/absent" run platform_tunnels
    assert_success
    assert_output "main"
}

@test "platform_tunnel_iface: wgcN is its own interface, ovpncN is tun1N" {
    load_platform
    run platform_tunnel_iface wgc1
    assert_output "wgc1"
    run platform_tunnel_iface ovpnc3
    assert_output "tun13"
}

@test "platform_tunnel_iface: unknown ids fail" {
    load_platform
    run platform_tunnel_iface main
    assert_failure
    refute_output
    run platform_tunnel_iface eth0
    assert_failure
}

@test "platform_tunnel_info: type, connected and description on three lines" {
    load_platform
    run platform_tunnel_info wgc1
    assert_success
    assert_line --index 0 "wireguard"
    assert_line --index 1 "0"
    assert_line --index 2 "Office WG"
    run platform_tunnel_info ovpnc1
    assert_line --index 0 "openvpn"
    assert_line --index 2 "Office OVPN"
}

@test "platform_tunnel_info: connected is 1 when the interface carries UP" {
    load_platform
    with_mock ip 'echo "12: wgc1: <POINTOPOINT,NOARP,UP,LOWER_UP> mtu 1420 qdisc noqueue state UNKNOWN"'
    run platform_tunnel_info wgc1
    assert_line --index 1 "1"
}

@test "platform_tunnel_info: unknown id fails" {
    load_platform
    run platform_tunnel_info eth0
    assert_failure
    refute_output
}

@test "platform_tunnel_table: the id itself; main stays main" {
    load_platform
    run platform_tunnel_table wgc1 0
    assert_output "wgc1"
    run platform_tunnel_table main 3
    assert_output "main"
}

@test "platform_tunnel_route_ensure and _table_release: no-ops that succeed" {
    load_platform
    : > /tmp/bats_ip_calls.log
    run platform_tunnel_route_ensure wgc1 0
    assert_success
    run platform_tunnel_table_release wgc1 0
    assert_success
    [ ! -s /tmp/bats_ip_calls.log ]
}

@test "platform_vpn_endpoints: non-empty vpn_clientN_addr values, one per line" {
    load_platform
    run platform_vpn_endpoints
    assert_success
    assert_line --index 0 "openvpn1.example.com"
    assert_line --index 1 "example.com"
    [ "${#lines[@]}" -eq 2 ]
}

# ------------------------------------------------------------ modules / cron

@test "platform_load_module: already loaded module needs no modprobe" {
    load_platform
    : > /tmp/bats_modprobe_calls.log
    run platform_load_module xt_TPROXY
    assert_success
    [ ! -s /tmp/bats_modprobe_calls.log ]
}

@test "platform_load_module: modprobes a module lsmod does not list" {
    load_platform
    : > /tmp/bats_modprobe_calls.log
    run platform_load_module xt_socket
    assert_success
    grep -q "modprobe xt_socket" /tmp/bats_modprobe_calls.log
}

@test "platform_load_module: fails when modprobe fails" {
    load_platform
    with_mock modprobe 'exit 1'
    run platform_load_module xt_socket
    assert_failure
}

@test "platform_cron_add / platform_cron_del: cru a and cru d" {
    load_platform
    : > /tmp/bats_cru_calls.log
    run platform_cron_add vpn_director_update "0 3 * * *" "/opt/vpn-director/vpn-director.sh update"
    assert_success
    run platform_cron_del vpn_director_update
    assert_success
    grep -qF 'cru a vpn_director_update 0 3 * * * /opt/vpn-director/vpn-director.sh update' /tmp/bats_cru_calls.log
    grep -qF 'cru d vpn_director_update' /tmp/bats_cru_calls.log
}

# ------------------------------------------------------------- firewall bits

@test "platform_tproxy_extra_rules: nothing to do on Merlin" {
    load_platform
    : > /tmp/bats_iptables_calls.log
    run platform_tproxy_extra_rules apply
    assert_success
    run platform_tproxy_extra_rules stop
    assert_success
    [ ! -s /tmp/bats_iptables_calls.log ]
}

@test "platform_prerouting_base_pos: 1 when PREROUTING has no firmware mark rules" {
    load_platform
    run platform_prerouting_base_pos
    assert_output "1"
}

@test "platform_prerouting_base_pos: right after the last firmware iface-mark rule" {
    load_platform
    with_mock iptables 'cat <<EOF
-P PREROUTING ACCEPT
-A PREROUTING -i wgc1 -j MARK --set-xmark 0x1/0x7
-A PREROUTING -i tun11 -j MARK --set-xmark 0x2/0x7
-A PREROUTING -i br0 -j SOMETHING_ELSE
EOF'
    run platform_prerouting_base_pos
    assert_output "3"
}

# ------------------------------------------------------------- system facts

@test "platform_password_file: /etc/shadow" {
    load_platform
    run platform_password_file
    assert_output "/etc/shadow"
}

@test "platform_lan_ip, platform_hostname, platform_model come from nvram" {
    load_platform
    run platform_lan_ip
    assert_output "192.168.50.1"
    run platform_hostname
    assert_output "RT-AX88U-1234"
    run platform_model
    assert_output "RT-AX88U"
}

@test "platform_lan_ip: fails when nvram has no lan_ipaddr" {
    load_platform
    NVRAM_NO_LAN_IP=1 run platform_lan_ip
    assert_failure
    refute_output
}

@test "platform_email_supported: 1" {
    load_platform
    run platform_email_supported
    assert_output "1"
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `bats router/test/unit/platform.bats router/test/unit/platform_merlin.bats`
Expected: every test fails with `No such file or directory` for `platform.sh`.

- [ ] **Step 4: Write platform.sh**

Create `router/opt/vpn-director/lib/platform.sh`:

```bash
#!/usr/bin/env bash

###################################################################################################
# platform.sh - platform detection and loader for the platform contract
# -------------------------------------------------------------------------------------------------
# VPN Director runs on Asuswrt-Merlin and on KeeneticOS. Everything the core
# modules need from the firmware goes through the platform_* functions defined
# in lib/platform/<name>.sh; the core never tests the platform name itself.
#
# Sourcing this file:
#   1. Uses VPD_PLATFORM when the environment already set it (tests, overrides).
#   2. Otherwise detects the platform from the file system (platform_detect).
#   3. Sources lib/platform/<name>.sh, which defines the contract functions.
# A platform that is neither known nor implemented aborts the sourcing script
# with "unsupported platform".
#
# Contract (every function prints its answer on stdout and returns 0; when it
# cannot answer it prints nothing and returns 1; none of them logs an ERROR):
#   platform_name                          merlin | keenetic
#   platform_wan_if                        active WAN interface name
#   platform_ipv6_enabled                  1 | 0
#   platform_lan_ifaces                    LAN interface names, one per line
#   platform_tunnels                       tunnel ids, one per line, "main" last
#   platform_tunnel_iface <id>             Linux interface of a tunnel
#   platform_tunnel_info <id>              three lines: type, connected (1|0), description
#   platform_tunnel_table <id> <idx>       routing table for "ip rule ... lookup"
#   platform_tunnel_route <id>             route spec for the tunnel table (Keenetic)
#   platform_tunnel_route_ensure <id> <idx> install the tunnel table route
#   platform_tunnel_table_release <id> <idx> drop the tunnel table route
#   platform_vpn_endpoints                 firmware VPN server hosts, one per line
#   platform_load_module <name>            load a kernel module
#   platform_cron_add <name> <schedule> <cmd> / platform_cron_del <name>
#   platform_tproxy_extra_rules apply|stop platform-only firewall rules for TPROXY
#   platform_prerouting_base_pos           insert position for TUN_DIR in mangle PREROUTING
#   platform_password_file                 file the Web UI verifies passwords against
#   platform_lan_ip, platform_hostname, platform_model
#   platform_email_supported               1 | 0
#
# Testing: VPD_PROBE_ROOT prefixes every path platform_detect looks at.
###################################################################################################

# -------------------------------------------------------------------------------------------------
# platform_detect - print the platform this system is, or fail
# -------------------------------------------------------------------------------------------------
platform_detect() {
    local root="${VPD_PROBE_ROOT:-}"
    if [[ -d "$root/opt/etc/ndm" ]] && [[ -x "$root/bin/ndmc" ]]; then
        printf 'keenetic\n'
        return 0
    fi
    if [[ -d "$root/jffs" ]] && [[ -x "$root/bin/nvram" ]]; then
        printf 'merlin\n'
        return 0
    fi
    return 1
}

# -------------------------------------------------------------------------------------------------
# Resolve the platform and load its implementation
# -------------------------------------------------------------------------------------------------
if [[ -z ${VPD_PLATFORM:-} ]]; then
    if ! VPD_PLATFORM="$(platform_detect)"; then
        printf 'unsupported platform: neither Asuswrt-Merlin nor Keenetic detected\n' >&2
        return 1 2>/dev/null || exit 1
    fi
fi
export VPD_PLATFORM

case "$VPD_PLATFORM" in
    merlin|keenetic) ;;
    *)
        printf 'unsupported platform: %s\n' "$VPD_PLATFORM" >&2
        return 1 2>/dev/null || exit 1
        ;;
esac

_platform_impl="${BASH_SOURCE[0]%/*}/platform/${VPD_PLATFORM}.sh"
if [[ ! -f $_platform_impl ]]; then
    printf 'platform implementation not found: %s\n' "$_platform_impl" >&2
    return 1 2>/dev/null || exit 1
fi
# shellcheck disable=SC1090
. "$_platform_impl"
unset _platform_impl
```

- [ ] **Step 5: Write merlin.sh**

Create `router/opt/vpn-director/lib/platform/merlin.sh`:

```bash
#!/usr/bin/env bash

###################################################################################################
# platform/merlin.sh - Asuswrt-Merlin implementation of the platform contract
# -------------------------------------------------------------------------------------------------
# See lib/platform.sh for the contract. Facts come from nvram, the firmware's
# named routing tables (wgcN, ovpncN in /etc/iproute2/rt_tables) and cru.
###################################################################################################

platform_name() {
    printf 'merlin\n'
}

# The firmware flags one WAN slot as primary; its interface is the active one.
platform_wan_if() {
    local idx name
    for idx in 0 1 2; do
        if [[ "$(nvram get "wan${idx}_primary" 2>/dev/null || true)" == "1" ]]; then
            name="$(nvram get "wan${idx}_ifname" 2>/dev/null || true)"
            [[ -n $name ]] || return 1
            printf '%s\n' "$name"
            return 0
        fi
    done
    name="$(nvram get wan0_ifname 2>/dev/null || true)"
    [[ -n $name ]] || return 1
    printf '%s\n' "$name"
}

platform_ipv6_enabled() {
    local s
    s="$(nvram get ipv6_service 2>/dev/null || true)"
    if [[ -n $s ]] && [[ $s != "disabled" ]]; then
        printf '1\n'
    else
        printf '0\n'
    fi
}

platform_lan_ifaces() {
    printf 'br0\n'
}

# wgcN first, ovpncN next, main always last. RT_TABLES_FILE overrides the path
# for tests.
platform_tunnels() {
    local rt_tables="${RT_TABLES_FILE:-/etc/iproute2/rt_tables}"
    { awk '$0!~/^#/ && $2 ~ /^wgc[0-9]+$/ { print $2 }' "$rt_tables" 2>/dev/null | sort; } || true
    { awk '$0!~/^#/ && $2 ~ /^ovpnc[0-9]+$/ { print $2 }' "$rt_tables" 2>/dev/null | sort; } || true
    printf '%s\n' main
}

# OpenVPN client N runs on tun1N; WireGuard clients are named after their table.
platform_tunnel_iface() {
    case "${1:-}" in
        wgc[0-9]*)   printf '%s\n' "$1" ;;
        ovpnc[0-9]*) printf 'tun1%s\n' "${1#ovpnc}" ;;
        *)           return 1 ;;
    esac
}

platform_tunnel_info() {
    local id="${1:-}" type desc="" iface connected=0
    case "$id" in
        wgc[0-9]*)
            type=wireguard
            desc="$(nvram get "${id}_desc" 2>/dev/null || true)"
            ;;
        ovpnc[0-9]*)
            type=openvpn
            desc="$(nvram get "vpn_client${id#ovpnc}_desc" 2>/dev/null || true)"
            ;;
        *) return 1 ;;
    esac
    iface="$(platform_tunnel_iface "$id")"
    # "<POINTOPOINT,NOARP,UP,LOWER_UP>": UP as a whole flag, not the tail of LOWER_UP.
    # No \b: busybox grep on the routers has no word boundaries.
    if ip -o link show "$iface" 2>/dev/null | grep -qE '<([^>]*,)?UP[,>]'; then
        connected=1
    fi
    printf '%s\n%s\n%s\n' "$type" "$connected" "$desc"
}

# The firmware keeps a routing table per tunnel under the tunnel's own name.
platform_tunnel_table() {
    printf '%s\n' "${1:-}"
}

platform_tunnel_route() {
    return 1
}

platform_tunnel_route_ensure() {
    return 0
}

platform_tunnel_table_release() {
    return 0
}

platform_vpn_endpoints() {
    local slot addr
    for slot in 1 2 3 4 5; do
        addr="$(nvram get "vpn_client${slot}_addr" 2>/dev/null || true)"
        [[ -n $addr ]] || continue
        printf '%s\n' "$addr"
    done
}

platform_load_module() {
    local name="${1:-}"
    [[ -n $name ]] || return 1
    if lsmod 2>/dev/null | grep -q "$name"; then
        return 0
    fi
    modprobe "$name" 2>/dev/null
}

platform_cron_add() {
    local name="$1" schedule="$2" cmd="$3"
    cru a "$name" "$schedule $cmd"
}

platform_cron_del() {
    cru d "$1"
}

platform_tproxy_extra_rules() {
    return 0
}

# Position right after the firmware's own iface-mark rules (-i wgcN / -i tunN
# -j MARK --set-...), so Tunnel Director never precedes them.
platform_prerouting_base_pos() {
    iptables -t mangle -S PREROUTING 2>/dev/null |
    awk '
        $1 == "-A" {
          i++
          if ( ($0 ~ /-i wgc[0-9]+/ || $0 ~ /-i tun[0-9]+/) &&
               $0 ~ /-j MARK/ && $0 ~ /--set-/ ) {
            last = i
          }
        }
        END { print (last ? last + 1 : 1) }
    '
}

platform_password_file() {
    printf '/etc/shadow\n'
}

platform_lan_ip() {
    local ip
    ip="$(nvram get lan_ipaddr 2>/dev/null || true)"
    [[ -n $ip ]] || return 1
    printf '%s\n' "$ip"
}

platform_hostname() {
    local name
    name="$(nvram get lan_hostname 2>/dev/null || true)"
    [[ -n $name ]] || return 1
    printf '%s\n' "$name"
}

platform_model() {
    local model
    model="$(nvram get model 2>/dev/null || true)"
    [[ -n $model ]] || return 1
    printf '%s\n' "$model"
}

# amtm email is a Merlin feature; whether it is configured stays send-email.sh's check.
platform_email_supported() {
    printf '1\n'
}
```

Add the two new files to `router/files.manifest` after the `lib/common.sh` line:

```
common   router/opt/vpn-director/lib/platform.sh
common   router/opt/vpn-director/lib/platform/merlin.sh
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `bats router/test/unit/platform.bats router/test/unit/platform_merlin.bats`
Expected: PASS (the `platform implementation not found` test passes because `keenetic.sh` does not exist yet).

Run (from `server/`): `go test ./internal/updater/ -run TestRepoManifest -count=1`
Expected: PASS (the manifest entries exist).

Run the whole Bats suite: `bats router/test/unit && bats router/test/integration`
Expected: PASS; nothing sources `platform.sh` yet besides the new tests.

- [ ] **Step 7: Commit**

```bash
git add router/opt/vpn-director/lib/platform.sh router/opt/vpn-director/lib/platform/merlin.sh router/files.manifest router/test/unit/platform.bats router/test/unit/platform_merlin.bats router/test/mocks/cru router/test/mocks/nvram router/test/test_helper.bash
git commit -m "feat(platform): platform detection and the Merlin contract implementation

lib/platform.sh resolves VPD_PLATFORM (environment or file-system probes) and
sources lib/platform/<name>.sh. merlin.sh collects every nvram, rt_tables and
cru call the core modules make today, behind the contract the Keenetic
implementation will share.

Claude-Session: https://claude.ai/code/session_01SygLygzpzxRtxoBqZsZuvL"
```

---

### Task 6: common.sh and firewall.sh call the contract

**Files:**
- Modify: `router/opt/vpn-director/lib/common.sh:4,55-66,724-780,880-887`, `router/opt/vpn-director/lib/firewall.sh:861-870,936-945` (and the doc comments above them)
- Test: `router/test/common.bats`, `router/test/firewall.bats`

**Interfaces:**
- Produces: `common.sh` sources `lib/platform.sh` (every script that sources `common.sh` gets the contract); `get_ipv6_enabled` and `get_active_wan_if` keep their names and output as wrappers; `block_wan_for_host <host>` and `allow_wan_for_host <host>` (the `wan_id` parameter is gone).
- Consumes: `platform_wan_if`, `platform_ipv6_enabled` from Task 5.

- [ ] **Step 1: Write the failing tests**

Append to `router/test/common.bats`:

```bash
# ============================================================================
# Platform contract is available through common.sh
# ============================================================================

@test "common.sh: sourcing it loads the platform contract" {
    load_common
    run platform_name
    assert_success
    assert_output "merlin"
}

@test "get_active_wan_if: wraps platform_wan_if" {
    load_common
    run get_active_wan_if
    assert_success
    assert_output "eth0"
}

@test "get_active_wan_if: prints nothing and still succeeds when the platform has no answer" {
    load_common
    platform_wan_if() { return 1; }
    run get_active_wan_if
    assert_success
    refute_output
}

@test "get_ipv6_enabled: wraps platform_ipv6_enabled" {
    load_common
    run get_ipv6_enabled
    assert_output "1"
    platform_ipv6_enabled() { printf '0\n'; }
    run get_ipv6_enabled
    assert_output "0"
}
```

Append to `router/test/firewall.bats`:

```bash
# ============================================================================
# block_wan_for_host / allow_wan_for_host take the WAN interface from the platform
# ============================================================================

@test "block_wan_for_host: blocks both directions on the platform WAN interface" {
    load_firewall
    platform_ipv6_enabled() { printf '0\n'; }
    : > /tmp/bats_iptables_calls.log
    run block_wan_for_host 192.168.50.10
    assert_success
    grep -q -- '-i eth0 -d 192.168.50.10 -j DROP' /tmp/bats_iptables_calls.log
    grep -q -- '-s 192.168.50.10 -o eth0 -j REJECT' /tmp/bats_iptables_calls.log
}

@test "block_wan_for_host: fails when the platform reports no WAN interface" {
    load_firewall
    platform_wan_if() { return 1; }
    run block_wan_for_host 192.168.50.10
    assert_failure
    assert_output --partial "WAN interface name is empty"
}

@test "allow_wan_for_host: looks the rules up on the platform WAN interface" {
    load_firewall
    platform_ipv6_enabled() { printf '0\n'; }
    : > /tmp/bats_iptables_calls.log
    run allow_wan_for_host 192.168.50.10
    assert_success
    # The iptables mock answers -C with "absent", so ensure_fw_rule -D stops at
    # the check; the check itself must already name the platform's WAN interface.
    grep -q -- '-C FORWARD -i eth0 -d 192.168.50.10 -j DROP' /tmp/bats_iptables_calls.log
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `bats router/test/common.bats router/test/firewall.bats`
Expected: the new tests fail (`platform_name: command not found`, wrong WAN message text).

- [ ] **Step 3: Wire common.sh**

In `router/opt/vpn-director/lib/common.sh`:

Line 4: change `common.sh  -  shared functions library for Asuswrt-Merlin shell scripts` to `common.sh  -  shared functions library for VPN Director shell scripts`.

In the header's public API list, replace the `get_ipv6_enabled` and `get_active_wan_if` entries with:

```
#   get_ipv6_enabled
#       Prints 1 if the platform reports IPv6 enabled, otherwise 0. Wrapper over platform_ipv6_enabled.
#
#   get_active_wan_if
#       Prints the active WAN interface name, or nothing. Wrapper over platform_wan_if.
#
#   platform_* (see lib/platform.sh)
#       The platform contract; common.sh sources lib/platform.sh at the end, so every script that
#       sources common.sh can call it.
```

Replace the bodies of the two functions (their doc blocks shrink to one paragraph each):

```bash
###################################################################################################
# get_ipv6_enabled - print 1 when IPv6 is enabled on this router, else 0
# -------------------------------------------------------------------------------------------------
# Wrapper over platform_ipv6_enabled, kept under its old name for callers outside this repository.
# Exit status is always 0.
###################################################################################################
get_ipv6_enabled() {
    platform_ipv6_enabled || printf '0\n'
}

###################################################################################################
# get_active_wan_if - print the active WAN interface name
# -------------------------------------------------------------------------------------------------
# Wrapper over platform_wan_if, kept under its old name for callers outside this repository.
# Prints nothing when the platform has no answer; exit status is always 0.
###################################################################################################
get_active_wan_if() {
    platform_wan_if || true
}
```

Append at the very end of the file:

```bash
###################################################################################################
# Platform contract - detection and platform_* functions (lib/platform.sh)
###################################################################################################
# shellcheck source=platform.sh
. "${BASH_SOURCE[0]%/*}/platform.sh"
```

- [ ] **Step 4: Wire firewall.sh**

In `router/opt/vpn-director/lib/firewall.sh`, in `block_wan_for_host` (line 861) change the signature and the WAN lookup:

```bash
block_wan_for_host() {
    local host="$1"
    local host_ip4="" wan_if v6_list="" ip6
    local any_blocked=0

    # WAN interface
    wan_if="$(platform_wan_if)" || wan_if=""
    if [[ -z $wan_if ]]; then
        log -l ERROR "WAN interface name is empty; cannot block WAN for host"
        return 1
    fi
```

In `allow_wan_for_host` (line 936) make the same change:

```bash
allow_wan_for_host() {
    local host="$1"
    local host_ip4="" wan_if v6_list="" ip6
    local any_processed=0

    # WAN interface
    wan_if="$(platform_wan_if)" || wan_if=""
    if [[ -z $wan_if ]]; then
        log -l ERROR "WAN interface name is empty; cannot unblock WAN for host"
        return 1
    fi
```

In both doc blocks above the functions, drop the `[<wan_id>]` usage note and the sentence about `wan${wan_id}_ifname`, and say the interface comes from `platform_wan_if`. Replace every remaining `$(get_ipv6_enabled)` in these two functions with `$(platform_ipv6_enabled)`.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `bats router/test/unit && bats router/test/integration && bats router/test/common.bats router/test/firewall.bats router/test/config.bats router/test/import_server_list.bats`
Expected: PASS. Every module test sources `common.sh`, so all of them now go through `platform.sh` with `VPD_PLATFORM=merlin` from the helper.

- [ ] **Step 6: Commit**

```bash
git add router/opt/vpn-director/lib/common.sh router/opt/vpn-director/lib/firewall.sh router/test/common.bats router/test/firewall.bats
git commit -m "refactor(lib): take WAN and IPv6 facts from the platform contract

common.sh sources lib/platform.sh; get_active_wan_if and get_ipv6_enabled stay
as wrappers. firewall.sh reads the WAN interface through platform_wan_if and
drops the unused wan_id parameter.

Claude-Session: https://claude.ai/code/session_01SygLygzpzxRtxoBqZsZuvL"
```

---

### Task 7: tunnel.sh uses platform tables, routes and positions

**Files:**
- Modify: `router/opt/vpn-director/lib/ipset.sh:60-66` (state path), `router/opt/vpn-director/lib/tunnel.sh` (header, `_tunnel_init`, `_tunnel_get_prerouting_base_pos`, `tunnel_stop`, `tunnel_apply`)
- Test: `router/test/unit/tunnel.bats`

**Interfaces:**
- Produces: `TUN_DIR_TABLES` (default `/tmp/tunnel_director/tun_dir_tables`, exported by `ipset.sh`), `_tunnel_ensure_routes`; `tunnel_apply` writes `<idx> <id>` lines to `TUN_DIR_TABLES`; `tunnel_stop` releases and removes it. `_tunnel_get_prerouting_base_pos` is removed.
- Consumes: `platform_tunnels`, `platform_tunnel_table`, `platform_tunnel_route_ensure`, `platform_tunnel_table_release`, `platform_prerouting_base_pos`, `platform_lan_ifaces` from Task 5.

- [ ] **Step 1: Update and write the tests**

In `router/test/unit/tunnel.bats`, replace the `_tunnel_get_prerouting_base_pos` test (lines 102-112) with:

```bash
# ============================================================================
# PREROUTING insert position comes from the platform
# ============================================================================

@test "tunnel_apply: inserts the PREROUTING jump at platform_prerouting_base_pos" {
    load_tunnel_module
    platform_prerouting_base_pos() { printf '4\n'; }
    : > /tmp/bats_iptables_calls.log
    run tunnel_apply
    assert_success
    grep -q -- '-t mangle -I PREROUTING 4 -i br0 -m mark --mark 0x0/0xff0000 -j TUN_DIR' /tmp/bats_iptables_calls.log
}
```

Append to the file:

```bash
# ============================================================================
# Tunnel tables and routes through the platform contract
# ============================================================================

@test "_tunnel_init: valid tables come from platform_tunnels" {
    load_tunnel_module
    platform_tunnels() { printf '%s\n' OpenVPN0 Wireguard0 main; }
    _tunnel_init
    [ "$_tunnel_valid_tables" = "OpenVPN0 Wireguard0 main" ]
}

@test "tunnel_apply: looks the ip rule up in the platform's table" {
    load_tunnel_module
    platform_tunnel_table() { printf '2000\n'; }
    : > /tmp/bats_ip_calls.log
    run tunnel_apply
    assert_success
    grep -q 'ip rule add pref 16384 fwmark 0x10000/0xff0000 lookup 2000' /tmp/bats_ip_calls.log
}

@test "tunnel_apply: records applied tunnels as '<idx> <id>' in TUN_DIR_TABLES" {
    load_tunnel_module
    run tunnel_apply
    assert_success
    [ -f "$TUN_DIR_TABLES" ]
    run cat "$TUN_DIR_TABLES"
    assert_output "0 wgc1"
}

@test "tunnel_apply: ensures the route and still installs the ip rule when the route fails" {
    load_tunnel_module
    platform_tunnel_route_ensure() { echo "ensure $1 $2" >> "$BATS_TEST_TMPDIR/ensure.log"; return 1; }
    : > /tmp/bats_ip_calls.log
    run tunnel_apply
    assert_success
    assert_output --partial "route not installed"
    grep -q "ensure wgc1 0" "$BATS_TEST_TMPDIR/ensure.log"
    grep -q 'ip rule add pref 16384 fwmark 0x10000/0xff0000 lookup wgc1' /tmp/bats_ip_calls.log
}

@test "tunnel_apply: up-to-date path re-ensures every recorded route" {
    load_tunnel_module
    run tunnel_apply
    assert_success
    platform_tunnel_route_ensure() { echo "ensure $1 $2" >> "$BATS_TEST_TMPDIR/ensure.log"; return 0; }
    # The chain exists as far as the mock is concerned once the hash matches
    fw_chain_exists() { return 0; }
    run tunnel_apply
    assert_success
    assert_output --partial "up-to-date"
    grep -q "ensure wgc1 0" "$BATS_TEST_TMPDIR/ensure.log"
}

@test "tunnel_apply: a missing TUN_DIR_TABLES forces a rebuild even when the hash matches" {
    load_tunnel_module
    run tunnel_apply
    assert_success
    rm -f "$TUN_DIR_TABLES"
    fw_chain_exists() { return 0; }
    run tunnel_apply
    assert_success
    refute_output --partial "up-to-date"
    [ -f "$TUN_DIR_TABLES" ]
}

@test "tunnel_stop: releases every recorded table and removes the state file" {
    load_tunnel_module
    run tunnel_apply
    assert_success
    platform_tunnel_table_release() { echo "release $1 $2" >> "$BATS_TEST_TMPDIR/release.log"; }
    run tunnel_stop
    assert_success
    grep -q "release wgc1 0" "$BATS_TEST_TMPDIR/release.log"
    [ ! -e "$TUN_DIR_TABLES" ]
}

@test "tunnel_apply: one PREROUTING jump per platform LAN interface" {
    load_tunnel_module
    platform_lan_ifaces() { printf 'br0\nbr1\n'; }
    : > /tmp/bats_iptables_calls.log
    run tunnel_apply
    assert_success
    grep -q -- '-i br0 -m mark --mark 0x0/0xff0000 -j TUN_DIR' /tmp/bats_iptables_calls.log
    grep -q -- '-i br1 -m mark --mark 0x0/0xff0000 -j TUN_DIR' /tmp/bats_iptables_calls.log
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `bats router/test/unit/tunnel.bats`
Expected: the new tests fail (`TUN_DIR_TABLES` unset, `lookup 2000` absent, no `route not installed` message); the removed test is gone.

- [ ] **Step 3: Add the state file path**

In `router/opt/vpn-director/lib/ipset.sh`, after line 60 (`TUN_DIR_HASH=...`) add:

```bash
# Applied tunnels as "<idx> <id>" lines; tunnel.sh re-ensures their routes on
# every apply and releases their tables on stop.
TUN_DIR_TABLES="$TUN_DIRECTOR_DIR/tun_dir_tables"
```

and add `TUN_DIR_TABLES` to the `export` on line 66:

```bash
export IPS_BUILDER_DIR TUN_DIRECTOR_DIR TUN_DIR_HASH TUN_DIR_TABLES
```

- [ ] **Step 4: Rewrite the platform-touching parts of tunnel.sh**

Header (lines 9-27): replace the `Dependencies` and `Internal functions` blocks with:

```
# Dependencies:
#   - common.sh (log, tmp_file, compute_hash, is_lan_ip) and, through it, the
#     platform contract (platform_tunnels, platform_tunnel_table,
#     platform_tunnel_route_ensure, platform_tunnel_table_release,
#     platform_prerouting_base_pos, platform_lan_ifaces)
#   - firewall.sh (create_fw_chain, delete_fw_chain, ensure_fw_rule, sync_fw_rule,
#                  purge_fw_rules, fw_chain_exists)
#   - config.sh (TUN_DIR_TUNNELS_JSON, TUN_DIR_CHAIN, TUN_DIR_PREF_BASE,
#                TUN_DIR_MARK_MASK, TUN_DIR_MARK_SHIFT)
#   - ipset.sh (_ipset_exists, parse_exclude_sets_from_json, TUN_DIR_HASH, TUN_DIR_TABLES)
#
# Public API:
#   tunnel_status()              - show TUN_DIR chain, ip rules, configured tunnels
#   tunnel_apply()               - apply rules from config (idempotent)
#   tunnel_stop()                - remove chain, ip rules and tunnel tables
#   tunnel_get_required_ipsets() - return list of ipsets needed for rules
#
# Internal functions (for testing):
#   _tunnel_table_allowed()      - check if a tunnel id is one the platform lists
#   _tunnel_ensure_routes()      - re-install the routes of every applied tunnel
#   _tunnel_init()               - initialize module state
```

`_tunnel_init` (lines 79-108): replace the `rt_tables` block with:

```bash
    # Valid tunnel ids come from the platform (Merlin: wgcN and ovpncN tables
    # from rt_tables; Keenetic: OpenVPN*/Wireguard* interfaces), "main" last.
    local tables_list
    tables_list="$(platform_tunnels || true)"

    # Single-line, space-separated (for matching)
    _tunnel_valid_tables="$(printf '%s\n' "$tables_list" | xargs)"
```

and drop the `local rt_tables=...` line and the comment about `rt_tables`.

Delete `_tunnel_get_prerouting_base_pos` (lines 128-146) and add in its place:

```bash
# -------------------------------------------------------------------------------------------------
# _tunnel_ensure_routes - re-install the routes of every applied tunnel
# -------------------------------------------------------------------------------------------------
# Reads TUN_DIR_TABLES ("<idx> <id>" per line, written by tunnel_apply) and calls
# platform_tunnel_route_ensure for each. A no-op on Merlin, where the firmware
# keeps the tunnel tables; on Keenetic the route follows the interface state.
# -------------------------------------------------------------------------------------------------
_tunnel_ensure_routes() {
    local idx tunnel
    [[ -f $TUN_DIR_TABLES ]] || return 0
    while read -r idx tunnel; do
        [[ -n $tunnel ]] || continue
        if ! platform_tunnel_route_ensure "$tunnel" "$idx"; then
            log -l WARN "Tunnel '$tunnel': route not installed (interface down?); traffic falls through to main"
        fi
    done < "$TUN_DIR_TABLES"
}
```

In `tunnel_stop` (lines 223-249), before `# Clear hash file` add:

```bash
    # Release the tunnel tables this module owns (no-op on Merlin)
    local idx tunnel
    if [[ -f $TUN_DIR_TABLES ]]; then
        while read -r idx tunnel; do
            [[ -n $tunnel ]] || continue
            platform_tunnel_table_release "$tunnel" "$idx" || true
        done < "$TUN_DIR_TABLES"
        rm -f "$TUN_DIR_TABLES"
    fi
```

In `tunnel_apply`:

The rebuild decision (lines 274-285) becomes:

```bash
    # Check if rebuild needed
    local rebuild=0
    if [[ $new_hash != "$old_hash" ]]; then
        rebuild=1
    elif ! fw_chain_exists mangle "$TUN_DIR_CHAIN"; then
        rebuild=1
    elif [[ ! -f $TUN_DIR_TABLES ]]; then
        rebuild=1
    fi

    if [[ $rebuild -eq 0 ]]; then
        _tunnel_ensure_routes
        log "Rules are applied and up-to-date"
        return 0
    fi
```

Line 300 `base_pos=$(_tunnel_get_prerouting_base_pos)` becomes `base_pos=$(platform_prerouting_base_pos)`.

After `local tunnel_idx=0` add a temp file for the state:

```bash
    local tables_tmp
    tables_tmp="$(tmp_file)"
```

Replace the `_tunnel_table_allowed` skip message `"Tunnel '$tunnel' not in rt_tables; skipping"` with `"Tunnel '$tunnel' is not a tunnel this platform knows; skipping"`.

Replace the ip rule block (lines 409-417, from `# Add ip rule for this tunnel` to `tunnel_idx=$((tunnel_idx + 1))`) with:

```bash
        # Routing table for this tunnel: a firmware table on Merlin, one this
        # module owns on Keenetic. The ip rule is installed even when the route
        # is not: a lookup in an empty table falls through to main.
        local table
        table="$(platform_tunnel_table "$tunnel" "$tunnel_idx")"
        if ! platform_tunnel_route_ensure "$tunnel" "$tunnel_idx"; then
            log -l WARN "Tunnel '$tunnel': route not installed (interface down?); traffic falls through to main"
            warnings=1
        fi

        local pref=$((TUN_DIR_PREF_BASE + tunnel_idx))
        ip rule del pref "$pref" 2>/dev/null || true
        if ! ip rule add pref "$pref" fwmark "$mark_hex/$_tunnel_mark_mask_hex" lookup "$table" 2>/dev/null; then
            log -l ERROR "Failed to add ip rule: pref=$pref fwmark=$mark_hex lookup=$table"
            warnings=1
        fi

        printf '%s %s\n' "$tunnel_idx" "$tunnel" >> "$tables_tmp"
        tunnel_idx=$((tunnel_idx + 1))
```

Replace the PREROUTING jump (lines 420-422) with one jump per LAN interface:

```bash
    # Jump from PREROUTING to TUN_DIR for traffic from every LAN interface
    local lan_if
    while IFS= read -r lan_if; do
        [[ -n $lan_if ]] || continue
        sync_fw_rule -q mangle PREROUTING "-i $lan_if .*-j ${TUN_DIR_CHAIN}\$" \
            "-i $lan_if -m mark --mark 0x0/$_tunnel_mark_mask_hex -j $TUN_DIR_CHAIN" "$base_pos"
    done < <(platform_lan_ifaces)
```

Replace `# Save hash` (lines 424-426) with:

```bash
    # Save hash and the applied tunnel table
    mkdir -p "$(dirname "$TUN_DIR_HASH")"
    printf '%s\n' "$new_hash" > "$TUN_DIR_HASH"
    cp -f "$tables_tmp" "$TUN_DIR_TABLES"
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `bats router/test/unit/tunnel.bats && bats router/test/unit && bats router/test/integration`
Expected: PASS. If `tunnel_apply: inserts the PREROUTING jump at platform_prerouting_base_pos` fails on the exact `iptables` argument order, read the `-I` call `sync_fw_rule` logs into `/tmp/bats_iptables_calls.log` and align the grep pattern with it; the position argument must be `4`.

- [ ] **Step 6: Commit**

```bash
git add router/opt/vpn-director/lib/ipset.sh router/opt/vpn-director/lib/tunnel.sh router/test/unit/tunnel.bats
git commit -m "refactor(tunnel): tables, routes and positions through the platform contract

tunnel_apply asks the platform for the tunnel list, the routing table and the
PREROUTING position, ensures each tunnel's route on every apply, and records
the applied tunnels so tunnel_stop can release the tables the platform owns.

Claude-Session: https://claude.ai/code/session_01SygLygzpzxRtxoBqZsZuvL"
```

---

### Task 8: tproxy.sh and send-email.sh call the contract

**Files:**
- Modify: `router/opt/vpn-director/lib/tproxy.sh:84-97,374-389,455-470,473-482`, `router/opt/vpn-director/lib/send-email.sh:27-37,92`
- Test: `router/test/unit/tproxy.bats`

**Interfaces:**
- Produces: nothing new; `_tproxy_check_module` keeps its name.
- Consumes: `platform_load_module`, `platform_vpn_endpoints`, `platform_tproxy_extra_rules`, `platform_lan_ifaces`, `platform_email_supported`, `platform_model` from Task 5.

- [ ] **Step 1: Write the failing tests**

Append to `router/test/unit/tproxy.bats`:

```bash
# ============================================================================
# Platform contract in tproxy.sh
# ============================================================================

@test "_tproxy_check_module: loads xt_TPROXY through platform_load_module" {
    load_tproxy_module
    platform_load_module() { echo "load $1" >> "$BATS_TEST_TMPDIR/load.log"; return 0; }
    run _tproxy_check_module
    assert_success
    grep -q "load xt_TPROXY" "$BATS_TEST_TMPDIR/load.log"
}

@test "_tproxy_check_module: fails with an ERROR when the platform cannot load the module" {
    load_tproxy_module
    platform_load_module() { return 1; }
    run _tproxy_check_module
    assert_failure
    assert_output --partial "xt_TPROXY module not available"
}

@test "_tproxy_setup_bypass_ipset: adds every platform VPN endpoint" {
    load_tproxy_module
    platform_vpn_endpoints() { printf '%s\n' 203.0.113.7 198.51.100.9; }
    : > /tmp/bats_ipset_calls.log
    run _tproxy_setup_bypass_ipset
    assert_success
    grep -q "ipset add TPROXY_BYPASS 203.0.113.7" /tmp/bats_ipset_calls.log
    grep -q "ipset add TPROXY_BYPASS 198.51.100.9" /tmp/bats_ipset_calls.log
}

@test "_tproxy_setup_iptables: applies platform extra rules and one jump per LAN interface" {
    load_tproxy_module
    platform_tproxy_extra_rules() { echo "extra $1" >> "$BATS_TEST_TMPDIR/extra.log"; }
    platform_lan_ifaces() { printf 'br0\nbr1\n'; }
    : > /tmp/bats_iptables_calls.log
    run _tproxy_setup_iptables
    assert_success
    grep -q "extra apply" "$BATS_TEST_TMPDIR/extra.log"
    grep -q -- '-I PREROUTING 1 -i br0 -j XRAY_TPROXY' /tmp/bats_iptables_calls.log
    grep -q -- '-I PREROUTING 1 -i br1 -j XRAY_TPROXY' /tmp/bats_iptables_calls.log
}

@test "_tproxy_teardown_iptables: removes platform extra rules" {
    load_tproxy_module
    platform_tproxy_extra_rules() { echo "extra $1" >> "$BATS_TEST_TMPDIR/extra.log"; }
    run _tproxy_teardown_iptables
    assert_success
    grep -q "extra stop" "$BATS_TEST_TMPDIR/extra.log"
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `bats router/test/unit/tproxy.bats`
Expected: the five new tests fail; the existing ones pass. Check the chain name in the fixture with `jq .advanced.xray.chain router/test/fixtures/vpn-director.json` and the bypass ipset name with `.advanced.xray.bypass_ipset` (or the key `config.sh` reads into `XRAY_BYPASS_IPSET`); adjust the grep strings in the tests if they differ from `XRAY_TPROXY` and `TPROXY_BYPASS`.

- [ ] **Step 3: Wire tproxy.sh**

`_tproxy_check_module` (lines 84-97):

```bash
# -------------------------------------------------------------------------------------------------
# _tproxy_check_module - make sure the xt_TPROXY kernel module is loaded
# -------------------------------------------------------------------------------------------------
# Returns 0 if the platform loaded (or had loaded) the module, 1 otherwise.
# -------------------------------------------------------------------------------------------------
_tproxy_check_module() {
    if ! platform_load_module xt_TPROXY; then
        log -l ERROR "xt_TPROXY module not available"
        return 1
    fi
    return 0
}
```

In `_tproxy_setup_bypass_ipset`, replace "Source 3" (lines 374-389, the `for slot in 1 2 3 4 5` loop) with:

```bash
    # Source 3: endpoints of the firmware's own VPN clients, so their traffic
    # never enters the proxy (resolved on the fly)
    local addr
    while IFS= read -r addr; do
        [[ -n $addr ]] || continue

        resolved=$(resolve_ip -a -q "$addr" 2>/dev/null) || {
            log -l WARN "Cannot resolve VPN endpoint $addr"
            continue
        }

        while IFS= read -r ip; do
            [[ -n $ip ]] || continue
            ipset add "$XRAY_BYPASS_IPSET" "$ip" 2>/dev/null && ovpn_count=$((ovpn_count + 1)) || true
        done <<< "$resolved"
    done < <(platform_vpn_endpoints || true)
```

Keep the `local slot` declaration only if something else uses it; otherwise delete it. Keep the `ovpn_count` name and the summary log line.

In `_tproxy_setup_iptables`, replace the PREROUTING jump (lines 464-466) with:

```bash
    # Rules the platform needs outside our chain (Keenetic: mangle INPUT accept)
    platform_tproxy_extra_rules apply

    # Jump from PREROUTING to our chain for every LAN interface
    # (position 1 = before Tunnel Director)
    local lan_if
    while IFS= read -r lan_if; do
        [[ -n $lan_if ]] || continue
        sync_fw_rule -q mangle PREROUTING "-i $lan_if -j $XRAY_CHAIN\$" \
            "-i $lan_if -j $XRAY_CHAIN" 1
    done < <(platform_lan_ifaces)
```

In `_tproxy_teardown_iptables` (line 473), add as the first line of the body:

```bash
    platform_tproxy_extra_rules stop
```

- [ ] **Step 4: Wire send-email.sh**

In `router/opt/vpn-director/lib/send-email.sh`, after `. /opt/vpn-director/lib/common.sh` (line 30) add:

```bash
###############################################################################
# 0a'. Platforms without amtm email have nothing to send
###############################################################################
if [[ "$(platform_email_supported)" != "1" ]]; then
    log -l DEBUG "Email notifications are not supported on this platform; skipping"
    exit 0
fi
```

Change line 92 from `FROM_NAME="ASUS $(nvram get model)"` to:

```bash
FROM_NAME="ASUS $(platform_model || true)"   # router name shown in "From:"
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `bats router/test/unit/tproxy.bats && bats router/test/unit && bats router/test/integration`
Expected: PASS, including the pre-existing `tproxy_apply: soft-fails when module unavailable` test (its failing `lsmod`/`modprobe` mocks now act through `platform_load_module`).

Also run `bash -n router/opt/vpn-director/lib/send-email.sh` (syntax only; the script is not covered by Bats).
Expected: no output.

- [ ] **Step 6: Commit**

```bash
git add router/opt/vpn-director/lib/tproxy.sh router/opt/vpn-director/lib/send-email.sh router/test/unit/tproxy.bats
git commit -m "refactor(tproxy): modules, VPN endpoints and LAN jumps through the platform contract

tproxy.sh loads xt_TPROXY with platform_load_module, fills the bypass set from
platform_vpn_endpoints, adds the platform's extra rules and one PREROUTING
jump per LAN interface. send-email.sh exits quietly where email is unsupported.

Claude-Session: https://claude.ai/code/session_01SygLygzpzxRtxoBqZsZuvL"
```

---

### Task 9: `vpn-director.sh platform` and `cron` subcommands; S99 uses them

**Files:**
- Modify: `router/opt/vpn-director/vpn-director.sh:1-20` (usage), `:83-100` (help), `:306-334` (new commands), `:340-364` (dispatch); `router/opt/etc/init.d/S99vpn-director:20-35`
- Test: `router/test/integration/vpn_director.bats`

**Interfaces:**
- Produces:
  - `vpn-director.sh platform` prints the JSON of spec 5.3: `platform`, `arch`, `password_file`, `lan_ifaces`, `wan_if` (`""` when unknown), `tunnels[]` with `id`, `iface`, `type`, `connected`, `description`; `main` is excluded; a tunnel whose lookup fails is omitted with a WARN.
  - `vpn-director.sh cron install` and `vpn-director.sh cron remove` manage the daily `update` job through `platform_cron_add`/`platform_cron_del` with the job name `vpn_director_update`.
- Consumes: the contract functions from Task 5.

- [ ] **Step 1: Write the failing tests**

Append to `router/test/integration/vpn_director.bats`:

```bash
# ============================================================================
# platform subcommand
# ============================================================================

@test "vpn-director: platform prints the platform facts as JSON" {
    run "$SCRIPTS_DIR/vpn-director.sh" platform
    assert_success
    echo "$output" | jq -e '.platform == "merlin"' >/dev/null
    echo "$output" | jq -e '.password_file == "/etc/shadow"' >/dev/null
    echo "$output" | jq -e '.lan_ifaces == ["br0"]' >/dev/null
    echo "$output" | jq -e '.wan_if == "eth0"' >/dev/null
    echo "$output" | jq -e '.arch | length > 0' >/dev/null
}

@test "vpn-director: platform lists every tunnel except main" {
    run "$SCRIPTS_DIR/vpn-director.sh" platform
    assert_success
    echo "$output" | jq -e '[.tunnels[].id] == ["wgc1","wgc2","ovpnc1","ovpnc2"]' >/dev/null
    echo "$output" | jq -e '.tunnels[0] == {id:"wgc1", iface:"wgc1", type:"wireguard", connected:false, description:"Office WG"}' >/dev/null
    echo "$output" | jq -e '.tunnels[2] == {id:"ovpnc1", iface:"tun11", type:"openvpn", connected:false, description:"Office OVPN"}' >/dev/null
}

@test "vpn-director: platform reports wan_if as empty when the platform has no answer" {
    mkdir -p "$BATS_TEST_TMPDIR/mock"
    printf '#!/bin/bash\necho ""\n' > "$BATS_TEST_TMPDIR/mock/nvram"
    chmod +x "$BATS_TEST_TMPDIR/mock/nvram"
    PATH="$BATS_TEST_TMPDIR/mock:$PATH" run "$SCRIPTS_DIR/vpn-director.sh" platform
    assert_success
    echo "$output" | jq -e '.wan_if == ""' >/dev/null
}

# ============================================================================
# cron subcommand
# ============================================================================

@test "vpn-director: cron install schedules the daily update through the platform" {
    : > /tmp/bats_cru_calls.log
    run "$SCRIPTS_DIR/vpn-director.sh" cron install
    assert_success
    # The CLI resolves its own directory with pwd; SCRIPTS_DIR still contains "test/.."
    local vpd_dir
    vpd_dir="$(cd "$SCRIPTS_DIR" && pwd)"
    grep -qF "cru a vpn_director_update 0 3 * * * $vpd_dir/vpn-director.sh update" /tmp/bats_cru_calls.log
}

@test "vpn-director: cron remove drops the job" {
    : > /tmp/bats_cru_calls.log
    run "$SCRIPTS_DIR/vpn-director.sh" cron remove
    assert_success
    grep -qF "cru d vpn_director_update" /tmp/bats_cru_calls.log
}

@test "vpn-director: cron without install|remove fails with usage" {
    run "$SCRIPTS_DIR/vpn-director.sh" cron
    assert_failure
    assert_output --partial "cron install|remove"
}

@test "vpn-director: help lists platform and cron" {
    run "$SCRIPTS_DIR/vpn-director.sh" --help
    assert_output --partial "platform"
    assert_output --partial "cron install|remove"
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `bats router/test/integration/vpn_director.bats`
Expected: the seven new tests fail with `Unknown command: platform` / `Unknown command: cron`.

- [ ] **Step 3: Add the subcommands**

In `router/opt/vpn-director/vpn-director.sh`:

Usage block (lines 6-11), add after the `update` line:

```
#   vpn-director platform                    - Print platform facts as JSON (for the daemons)
#   vpn-director cron install|remove         - Schedule or drop the daily ipset update
```

Help text (`show_help`), replace the first line `VPN Director - Unified traffic routing for Asuswrt-Merlin` with `VPN Director - Unified traffic routing for Asuswrt-Merlin and Keenetic`, and add after the `update` line of `Commands:`:

```
  platform                    Print platform facts as JSON (used by the Web UI and the bot)
  cron install|remove         Schedule or drop the daily "update" job
```

After `cmd_update()` add:

```bash
# -------------------------------------------------------------------------------------------------
# cmd_platform - print the platform facts the daemons need, as one JSON document
# -------------------------------------------------------------------------------------------------
# Tunnels are listed without "main"; a tunnel whose interface or info lookup
# fails is omitted with a WARN so the rest of the document stays usable.
# -------------------------------------------------------------------------------------------------
cmd_platform() {
    _load_common

    local arch wan pw_file lan_json tunnels_json
    arch="$(uname -m)"
    wan="$(platform_wan_if)" || wan=""
    pw_file="$(platform_password_file)"
    lan_json="$(platform_lan_ifaces | jq -R . | jq -s -c .)"

    tunnels_json="[]"
    local id iface info type connected desc
    while IFS= read -r id; do
        [[ -n $id ]] || continue
        [[ $id == main ]] && continue
        if ! iface="$(platform_tunnel_iface "$id")"; then
            log -l WARN "platform: cannot map tunnel '$id' to an interface; omitted"
            continue
        fi
        if ! info="$(platform_tunnel_info "$id")"; then
            log -l WARN "platform: no info for tunnel '$id'; omitted"
            continue
        fi
        type="$(printf '%s\n' "$info" | sed -n 1p)"
        connected="$(printf '%s\n' "$info" | sed -n 2p)"
        desc="$(printf '%s\n' "$info" | sed -n 3p)"
        tunnels_json="$(jq -c --argjson list "$tunnels_json" \
            --arg id "$id" --arg iface "$iface" --arg type "$type" \
            --argjson connected "$([[ $connected == 1 ]] && echo true || echo false)" \
            --arg desc "$desc" \
            -n '$list + [{id:$id, iface:$iface, type:$type, connected:$connected, description:$desc}]')"
    done < <(platform_tunnels || true)

    jq -n \
        --arg platform "$(platform_name)" \
        --arg arch "$arch" \
        --arg password_file "$pw_file" \
        --argjson lan_ifaces "$lan_json" \
        --arg wan_if "$wan" \
        --argjson tunnels "$tunnels_json" \
        '{platform:$platform, arch:$arch, password_file:$password_file,
          lan_ifaces:$lan_ifaces, wan_if:$wan_if, tunnels:$tunnels}'
}

# -------------------------------------------------------------------------------------------------
# cmd_cron - schedule or drop the daily "update" job through the platform's cron
# -------------------------------------------------------------------------------------------------
cmd_cron() {
    _load_common
    case "$COMPONENT" in
        install)
            platform_cron_add vpn_director_update "0 3 * * *" "$SCRIPT_DIR/vpn-director.sh update"
            log "Scheduled daily ipset update"
            ;;
        remove)
            platform_cron_del vpn_director_update
            log "Removed daily ipset update"
            ;;
        *)
            echo "Usage: vpn-director cron install|remove" >&2
            exit 1
            ;;
    esac
}
```

In the dispatch `case "$COMMAND"` add before `*)`:

```bash
    platform)
        cmd_platform
        ;;
    cron)
        cmd_cron
        ;;
```

- [ ] **Step 4: Use the CLI from S99vpn-director**

In `router/opt/etc/init.d/S99vpn-director`, replace the `start()` and `stop()` bodies:

```sh
start() {
    echo "Starting $DESC..."
    if "$VPD" apply; then
        # Remove legacy cron job from previous versions (Merlin only; harmless elsewhere)
        cru d update_ipsets 2>/dev/null
        "$VPD" cron install
        /opt/vpn-director/lib/send-email.sh "Startup" "VPN Director started" 2>/dev/null || true
    else
        echo "Failed to start $DESC"
        return 1
    fi
}

stop() {
    echo "Stopping $DESC..."
    "$VPD" stop
    "$VPD" cron remove
    # Also remove legacy cron job if present (Merlin only; harmless elsewhere)
    cru d update_ipsets 2>/dev/null
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `bats router/test/integration/vpn_director.bats && bats router/test/unit`
Expected: PASS. `sh -n router/opt/etc/init.d/S99vpn-director` prints nothing.

- [ ] **Step 6: Commit**

```bash
git add router/opt/vpn-director/vpn-director.sh router/opt/etc/init.d/S99vpn-director router/test/integration/vpn_director.bats
git commit -m "feat(cli): platform and cron subcommands

'vpn-director.sh platform' prints the platform, tunnels, LAN/WAN and password
file as JSON for the daemons; 'cron install|remove' schedules the daily update
through the platform, which lets S99vpn-director drop its direct cru calls.

Claude-Session: https://claude.ai/code/session_01SygLygzpzxRtxoBqZsZuvL"
```

---

### Task 10: Documentation for the platform layer

**Files:**
- Modify: `CLAUDE.md` (Architecture table, Shell Conventions), `.claude/rules/shell-conventions.md:30-36`, `.claude/rules/packet-flow.md:150-153,270-280`, `.claude/rules/tunnel-director.md:95-101`

**Interfaces:** none.

- [ ] **Step 1: Update CLAUDE.md**

In the Architecture table add after the `lib/common.sh` row:

```
| `router/opt/vpn-director/lib/platform.sh` | Platform detection (`VPD_PLATFORM`) and loader of the platform contract |
| `router/opt/vpn-director/lib/platform/merlin.sh` | Asuswrt-Merlin implementation: nvram, rt_tables, cru |
| `router/files.manifest` | Shipped files with platform tags; read by `install.sh` and the updater |
```

In the Commands block add after the `update` line:

```bash
/opt/vpn-director/vpn-director.sh platform            # Platform facts as JSON (tunnels, WAN, password file)
/opt/vpn-director/vpn-director.sh cron install        # Schedule the daily update (S99 does this)
```

In Shell Conventions add:

```
- Platform facts (WAN, tunnels, cron, kernel modules) come only from `platform_*` functions; core modules never test the platform name. Tests set `VPD_PLATFORM=merlin` through `test_helper.bash`.
```

- [ ] **Step 2: Update the rules files**

`.claude/rules/shell-conventions.md`: in the utilities table replace the `get_active_wan_if` and `get_ipv6_enabled` rows with:

```
| `platform_wan_if` | Active WAN interface (platform contract; `get_active_wan_if` is a wrapper) |
| `platform_ipv6_enabled` | 1 when IPv6 is enabled (platform contract; `get_ipv6_enabled` is a wrapper) |
| `platform_tunnels`, `platform_tunnel_table`, `platform_load_module`, `platform_cron_add` | Other platform facts; see `lib/platform.sh` header for the whole contract |
```

`.claude/rules/packet-flow.md`: line 152 `_tunnel_get_prerouting_base_pos()` becomes `platform_prerouting_base_pos()  # Merlin: after firmware iface-mark rules; Keenetic: 2`; in the "Key Code Locations" table replace the `_tunnel_get_prerouting_base_pos()` row with `| `lib/platform/merlin.sh` | `platform_prerouting_base_pos()` | TD position calc |` and add `| `lib/tunnel.sh` | `_tunnel_ensure_routes()` | Re-install tunnel routes on every apply |`. Update the line-number references in that table to the current lines (`grep -n` the function names).

`.claude/rules/tunnel-director.md`: replace the `_tunnel_get_prerouting_base_pos()` row with `platform_prerouting_base_pos()` and add a row for `_tunnel_ensure_routes()`; mention `TUN_DIR_TABLES` next to `TUN_DIR_HASH` where the state files are described.

- [ ] **Step 3: Verify nothing references removed names**

Run: `grep -rn '_tunnel_get_prerouting_base_pos\|scriptFiles' router server .claude CLAUDE.md README.md`
Expected: no output.

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md .claude/rules/shell-conventions.md .claude/rules/packet-flow.md .claude/rules/tunnel-director.md
git commit -m "docs: describe the platform layer and the file manifest

Claude-Session: https://claude.ai/code/session_01SygLygzpzxRtxoBqZsZuvL"
```

---

### Task 11: Full verification and Merlin regression

**Files:** none modified unless a check fails.

- [ ] **Step 1: Run every suite**

Run from the root:
```bash
bats router/test/unit && bats router/test/integration && bats router/test/common.bats router/test/config.bats router/test/firewall.bats router/test/import_server_list.bats
(cd server && go build ./... && go vet ./... && go test ./... -count=1)
shellcheck -x router/opt/vpn-director/lib/platform.sh router/opt/vpn-director/lib/platform/merlin.sh router/opt/vpn-director/vpn-director.sh install.sh 2>&1 | head -40
```
Expected: all suites PASS; shellcheck reports nothing new compared with `git stash`-free baseline (run it on `master` for the shared files if unsure). Fix anything that fails before continuing.

- [ ] **Step 2: Stage a Merlin regression on the Asus router (manual, with the user)**

The user's Asus router runs the released version. Ask the user for SSH access, then copy the working tree's router files over the installed ones without touching the config:

```bash
ASUS=admin@<router-ip>
scp -r router/opt/vpn-director/lib router/opt/vpn-director/vpn-director.sh "$ASUS:/opt/vpn-director/"
scp router/opt/etc/init.d/S99vpn-director "$ASUS:/opt/etc/init.d/"
ssh "$ASUS" 'chmod +x /opt/vpn-director/*.sh /opt/vpn-director/lib/*.sh /opt/vpn-director/lib/platform/*.sh /opt/etc/init.d/S99vpn-director'
```

Then on the router, compare before and after:

```bash
ssh "$ASUS" '
/opt/vpn-director/vpn-director.sh platform
/opt/vpn-director/vpn-director.sh status
iptables -t mangle -S PREROUTING | head; ip rule show | grep fwmark
/opt/vpn-director/vpn-director.sh restart
iptables -t mangle -S PREROUTING | head; ip rule show | grep fwmark
cat /tmp/tunnel_director/tun_dir_tables
/opt/vpn-director/vpn-director.sh cron install; cru l | grep vpn_director
'
```

Expected: `platform` prints `"platform":"merlin"`, the router's WAN interface and its `wgc`/`ovpnc` tunnels; the PREROUTING and `ip rule` output after `restart` equals the output before it; `tun_dir_tables` lists the configured tunnels; `cru l` shows `vpn_director_update`. Then open the Web UI and the bot's `/status`: both unchanged (they do not call the new subcommand yet).

- [ ] **Step 3: Record the outcome**

If any step deviates, open a fix commit on this branch and rerun Step 1. When everything matches, tell the user the foundation is complete and that plan 2 (Keenetic) can be written.

---

## Self-review notes

- Spec coverage for this plan's scope: 4.1 (Task 1), 4.2-4.3 (Tasks 2-5), 5.1-5.2 Merlin column (Task 5), 5.3 (Task 9), 7 (Tasks 5-9), 8 except `configure.sh` (Tasks 6-9; `configure.sh` moves to plan 2 with the Keenetic tunnel list), 9.6 (Tasks 3-4 for the manifest; `repoName` in Task 1; `mipsle` and the platform-aware `getPlatform` wiring belong to plan 3), 14 Merlin rows (Tasks 5-9), 18 items 2-4 (all tasks).
- Names used across tasks: `platform_*` functions as listed in Task 5; `TUN_DIR_TABLES` (Task 7); `manifest_files`/`manifest_is_executable`/`PLATFORM`/`INSTALL_ROOT` (Task 2); `parseManifest`/`manifestFilesFor`/`isExecutable`/`fetchManifest`/`getPlatform`/`FileEntry`/`fileEntries`/`loadPayloadManifest` (Tasks 3-4); `cmd_platform`/`cmd_cron` (Task 9).
