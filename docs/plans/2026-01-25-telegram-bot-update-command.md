# Telegram Bot /update Command Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement `/update` command that self-updates VPN Director (scripts, templates, bot binary) from GitHub releases.

**Architecture:** Bot downloads files to `/tmp/`, generates shell script with embedded data, launches it detached. Script stops bot, replaces files, restarts bot. Bot reads notification file on startup and reports success.

**Tech Stack:** Go 1.21+, text/template, net/http, GitHub Releases API, shell script

**Design Document:** `docs/plans/2026-01-25-telegram-bot-update-command-design.md`

**Review Changes Applied:**
- Options pattern for `bot.New()` instead of extending parameter list
- `HandleUpdate(msg)` without Context (use `context.Background()` internally)
- Four separate ldflags: Version, VersionFull, Commit, BuildDate
- Check `version == "dev"` with dedicated message
- Clean `files/` before downloading and on error
- Injectable `baseURL` for GitHub API testing
- Add `/update` to `/start` help text
- Strict `set -e` in shell script (no `|| true` except monit)
- Bot removes update directory after successful startup notification (script leaves it)
- Version string validation before shell script embedding
- Per-request timeout (2 min) instead of global client timeout
- Download size limit (50MB)
- Full path in pgrep (`/opt/vpn-director/telegram-bot`)
- Use `telegram.MessageSender` for startup notification (call in `Bot.Run()`)
- Require `procps-ng-pgrep` + `procps-ng-pkill` for pgrep/pkill usage
- Update `S98telegram-bot` to launch `/opt/vpn-director/telegram-bot` so full-path pgrep is reliable

---

## Task 1: Modify Makefile for Clean Version Tag

**Files:**
- Modify: `telegram-bot/Makefile`

**Step 1: Update VERSION and add VERSION_FULL**

Change from:
```makefile
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +%Y-%m-%d)

LDFLAGS = -ldflags "-s -w -X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildDate=$(BUILD_DATE)"
```

To:
```makefile
VERSION ?= $(shell git describe --tags --abbrev=0 2>/dev/null || echo "dev")
VERSION_FULL ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +%Y-%m-%d)

LDFLAGS = -ldflags "-s -w -X main.Version=$(VERSION) -X main.VersionFull=$(VERSION_FULL) -X main.Commit=$(COMMIT) -X main.BuildDate=$(BUILD_DATE)"
```

**Step 2: Verify change**

Run: `cd telegram-bot && make build 2>&1 | head -5`

Expected: Build succeeds (no error output)

**Step 3: Commit**

```bash
git add telegram-bot/Makefile
git commit -m "build(telegram-bot): add VERSION_FULL for complete version info

VERSION now contains clean tag (v1.2.0) for semver parsing.
VERSION_FULL contains full git describe output (v1.2.0-5-gabc1234-dirty)
for display in /version command."
```

---

## Task 2: Create Version Parser with Tests

**Files:**
- Create: `telegram-bot/internal/updater/version.go`
- Create: `telegram-bot/internal/updater/version_test.go`

**Step 1: Write failing tests**

Create `telegram-bot/internal/updater/version_test.go`:

```go
package updater

import (
	"testing"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Version
		wantErr bool
	}{
		{
			name:  "with v prefix",
			input: "v1.2.3",
			want:  Version{Major: 1, Minor: 2, Patch: 3, Raw: "v1.2.3"},
		},
		{
			name:  "without v prefix",
			input: "1.2.3",
			want:  Version{Major: 1, Minor: 2, Patch: 3, Raw: "1.2.3"},
		},
		{
			name:  "zero version",
			input: "v0.0.0",
			want:  Version{Major: 0, Minor: 0, Patch: 0, Raw: "v0.0.0"},
		},
		{
			name:    "dev version",
			input:   "dev",
			wantErr: true,
		},
		{
			name:    "pre-release",
			input:   "v1.2.3-rc1",
			wantErr: true,
		},
		{
			name:    "incomplete version",
			input:   "v1.2",
			wantErr: true,
		},
		{
			name:    "invalid format",
			input:   "invalid",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "non-numeric major",
			input:   "vX.2.3",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseVersion(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseVersion(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseVersion(%q) = %+v, want %+v", tt.input, got, tt.want)
			}
		})
	}
}

func TestVersionCompare(t *testing.T) {
	tests := []struct {
		name string
		v1   Version
		v2   Version
		want int
	}{
		{
			name: "equal versions",
			v1:   Version{Major: 1, Minor: 2, Patch: 3},
			v2:   Version{Major: 1, Minor: 2, Patch: 3},
			want: 0,
		},
		{
			name: "major greater",
			v1:   Version{Major: 2, Minor: 0, Patch: 0},
			v2:   Version{Major: 1, Minor: 9, Patch: 9},
			want: 1,
		},
		{
			name: "major less",
			v1:   Version{Major: 1, Minor: 9, Patch: 9},
			v2:   Version{Major: 2, Minor: 0, Patch: 0},
			want: -1,
		},
		{
			name: "minor greater",
			v1:   Version{Major: 1, Minor: 3, Patch: 0},
			v2:   Version{Major: 1, Minor: 2, Patch: 9},
			want: 1,
		},
		{
			name: "minor less",
			v1:   Version{Major: 1, Minor: 2, Patch: 9},
			v2:   Version{Major: 1, Minor: 3, Patch: 0},
			want: -1,
		},
		{
			name: "patch greater",
			v1:   Version{Major: 1, Minor: 2, Patch: 4},
			v2:   Version{Major: 1, Minor: 2, Patch: 3},
			want: 1,
		},
		{
			name: "patch less",
			v1:   Version{Major: 1, Minor: 2, Patch: 3},
			v2:   Version{Major: 1, Minor: 2, Patch: 4},
			want: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.v1.Compare(tt.v2)
			if got != tt.want {
				t.Errorf("%+v.Compare(%+v) = %d, want %d", tt.v1, tt.v2, got, tt.want)
			}
		})
	}
}

func TestVersionIsOlderThan(t *testing.T) {
	tests := []struct {
		name string
		v1   Version
		v2   Version
		want bool
	}{
		{
			name: "older",
			v1:   Version{Major: 1, Minor: 0, Patch: 0},
			v2:   Version{Major: 1, Minor: 0, Patch: 1},
			want: true,
		},
		{
			name: "equal",
			v1:   Version{Major: 1, Minor: 0, Patch: 0},
			v2:   Version{Major: 1, Minor: 0, Patch: 0},
			want: false,
		},
		{
			name: "newer",
			v1:   Version{Major: 1, Minor: 0, Patch: 1},
			v2:   Version{Major: 1, Minor: 0, Patch: 0},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.v1.IsOlderThan(tt.v2)
			if got != tt.want {
				t.Errorf("%+v.IsOlderThan(%+v) = %v, want %v", tt.v1, tt.v2, got, tt.want)
			}
		})
	}
}

func TestIsValidVersion(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"v1.2.3", true},
		{"1.2.3", true},
		{"v0.0.0", true},
		{"v1.2.3-rc1", true}, // dashes allowed
		{"", false},
		{"v1.2.3;rm -rf /", false},     // semicolon not allowed
		{"v1.2.3$(whoami)", false},     // $ not allowed
		{"v1.2.3`id`", false},          // backtick not allowed
		{"v1.2.3\"test", false},        // quote not allowed
		{string(make([]byte, 100)), false}, // too long
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := IsValidVersion(tt.input)
			if got != tt.want {
				t.Errorf("IsValidVersion(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `cd telegram-bot && go test ./internal/updater/... -v`

Expected: FAIL (package doesn't exist yet)

**Step 3: Write minimal implementation**

Create `telegram-bot/internal/updater/version.go`:

```go
package updater

import (
	"fmt"
	"strconv"
	"strings"
)

// Version represents a semantic version (major.minor.patch).
type Version struct {
	Major int
	Minor int
	Patch int
	Raw   string // Original string for display ("v1.2.3")
}

// ParseVersion parses a version string like "v1.2.3" or "1.2.3".
// Returns error for dev builds, pre-release versions, or invalid formats.
func ParseVersion(s string) (Version, error) {
	if s == "" {
		return Version{}, fmt.Errorf("empty version string")
	}

	raw := s
	s = strings.TrimPrefix(s, "v")

	// Check for pre-release suffix
	if strings.Contains(s, "-") {
		return Version{}, fmt.Errorf("pre-release versions not supported: %s", raw)
	}

	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("invalid version format (expected X.Y.Z): %s", raw)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return Version{}, fmt.Errorf("invalid major version: %s", raw)
	}

	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return Version{}, fmt.Errorf("invalid minor version: %s", raw)
	}

	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return Version{}, fmt.Errorf("invalid patch version: %s", raw)
	}

	return Version{
		Major: major,
		Minor: minor,
		Patch: patch,
		Raw:   raw,
	}, nil
}

// Compare returns:
//
//	-1 if v < other
//	 0 if v == other
//	 1 if v > other
func (v Version) Compare(other Version) int {
	if v.Major != other.Major {
		return compareInt(v.Major, other.Major)
	}
	if v.Minor != other.Minor {
		return compareInt(v.Minor, other.Minor)
	}
	return compareInt(v.Patch, other.Patch)
}

// IsOlderThan returns true if v < other.
func (v Version) IsOlderThan(other Version) bool {
	return v.Compare(other) < 0
}

func compareInt(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// IsValidVersion checks if version string contains only safe characters
// for shell script embedding: [a-zA-Z0-9.v-]
func IsValidVersion(s string) bool {
	if len(s) == 0 || len(s) > 50 {
		return false
	}
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '.' || r == '-' || r == 'v') {
			return false
		}
	}
	return true
}
```

**Step 4: Run tests to verify they pass**

Run: `cd telegram-bot && go test ./internal/updater/... -v`

Expected: PASS (all tests)

**Step 5: Commit**

```bash
git add telegram-bot/internal/updater/version.go telegram-bot/internal/updater/version_test.go
git commit -m "feat(telegram-bot): add semver parser for /update command

Strict vX.Y.Z format only. Rejects dev builds, pre-release tags,
and incomplete versions. Includes Compare(), IsOlderThan() methods.
IsValidVersion() validates strings for safe shell embedding."
```

---

## Task 3: Create Updater Service Interface and Struct

**Files:**
- Create: `telegram-bot/internal/updater/updater.go`

**Step 1: Write the interface and constructor**

Create `telegram-bot/internal/updater/updater.go`:

```go
package updater

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const (
	// UpdateDir is the base directory for update files.
	UpdateDir = "/tmp/vpn-director-update"
	// FilesDir is where downloaded files are stored.
	FilesDir = UpdateDir + "/files"
	// LockFile prevents concurrent updates.
	LockFile = UpdateDir + "/lock"
	// NotifyFile is read by bot on startup to send completion message.
	NotifyFile = UpdateDir + "/notify.json"
	// ScriptFile is the generated update shell script.
	ScriptFile = UpdateDir + "/update.sh"
)

// Updater defines the interface for the update service.
type Updater interface {
	// GetLatestRelease fetches the latest release info from GitHub.
	GetLatestRelease(ctx context.Context) (*Release, error)

	// ShouldUpdate checks if currentVersion is older than latestTag.
	// Returns false for "dev" versions.
	ShouldUpdate(currentVersion, latestTag string) (bool, error)

	// IsUpdateInProgress checks if lock file exists and process is alive.
	IsUpdateInProgress() bool

	// CreateLock creates lock file with current PID.
	CreateLock() error

	// RemoveLock removes the lock file.
	RemoveLock()

	// CleanFiles removes the files/ directory.
	CleanFiles()

	// DownloadRelease downloads all files for the given release.
	DownloadRelease(ctx context.Context, release *Release) error

	// RunUpdateScript generates and runs the update shell script.
	RunUpdateScript(chatID int64, oldVersion, newVersion string) error
}

// Release contains information about a GitHub release.
type Release struct {
	TagName string
	Assets  []Asset
}

// Asset represents a downloadable file in a release.
type Asset struct {
	Name        string
	DownloadURL string
}

// Service implements the Updater interface.
type Service struct {
	httpClient *http.Client
	baseURL    string // Injectable for testing, empty = default GitHub API
	lockFile   string // Configurable for testing
	updateDir  string // Configurable for testing
	scriptFile string // Configurable for testing
}

// New creates a new updater service.
// Note: No global timeout on http.Client - per-request timeouts used instead.
func New() *Service {
	return &Service{
		httpClient: &http.Client{},
	}
}

// NewWithBaseURL creates an updater service with custom base URL (for testing).
func NewWithBaseURL(baseURL string) *Service {
	s := New()
	s.baseURL = baseURL
	return s
}

// Ensure Service implements Updater.
var _ Updater = (*Service)(nil)

// IsUpdateInProgress checks if lock file exists and the process is still alive.
// Removes stale locks (process no longer exists).
func (s *Service) IsUpdateInProgress() bool {
	lockPath := s.getLockFile()

	data, err := os.ReadFile(lockPath)
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		// Can't read lock file, assume no update in progress
		return false
	}

	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		// Invalid PID in lock file, remove it
		os.Remove(lockPath)
		return false
	}

	// Check if process is alive using kill -0
	if err := syscall.Kill(pid, 0); err != nil {
		// Process doesn't exist, stale lock
		os.Remove(lockPath)
		return false
	}

	return true
}

// CreateLock creates the lock file with current process PID.
func (s *Service) CreateLock() error {
	lockPath := s.getLockFile()

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(lockPath), 0755); err != nil {
		return fmt.Errorf("create lock directory: %w", err)
	}

	pid := os.Getpid()
	if err := os.WriteFile(lockPath, []byte(strconv.Itoa(pid)), 0644); err != nil {
		return fmt.Errorf("write lock file: %w", err)
	}

	return nil
}

// RemoveLock removes the lock file.
func (s *Service) RemoveLock() {
	os.Remove(s.getLockFile())
}

// CleanFiles removes the files/ directory.
func (s *Service) CleanFiles() {
	os.RemoveAll(s.getFilesDir())
}

func (s *Service) getLockFile() string {
	if s.lockFile != "" {
		return s.lockFile
	}
	return LockFile
}

func (s *Service) getUpdateDir() string {
	if s.updateDir != "" {
		return s.updateDir
	}
	return UpdateDir
}

func (s *Service) getFilesDir() string {
	return filepath.Join(s.getUpdateDir(), "files")
}

func (s *Service) getScriptFile() string {
	if s.scriptFile != "" {
		return s.scriptFile
	}
	return ScriptFile
}
```

**Step 2: Verify it compiles**

Run: `cd telegram-bot && go build ./internal/updater/...`

Expected: Build succeeds

**Step 3: Commit**

```bash
git add telegram-bot/internal/updater/updater.go
git commit -m "feat(telegram-bot): add Updater interface and Service struct

Defines constants for update directory paths and the Updater interface.
Includes lock file management with PID-based stale detection.
Injectable baseURL for testing GitHub API calls.
No global http.Client timeout - per-request timeouts used instead."
```

---

## Task 4: Implement GitHub API Client with Tests

**Files:**
- Create: `telegram-bot/internal/updater/github.go`
- Create: `telegram-bot/internal/updater/github_test.go`

**Step 1: Write the implementation**

Create `telegram-bot/internal/updater/github.go`:

```go
package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	repoOwner        = "zinin"
	repoName         = "asuswrt-merlin-vpn-director"
	defaultAPIURL    = "https://api.github.com"
	releasesEndpoint = "/repos/%s/%s/releases/latest"
	apiTimeout       = 30 * time.Second
)

// githubRelease represents the GitHub API response for releases/latest.
type githubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

// githubAsset represents a release asset in the GitHub API response.
type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// GetLatestRelease fetches the latest release info from GitHub API.
func (s *Service) GetLatestRelease(ctx context.Context) (*Release, error) {
	ctx, cancel := context.WithTimeout(ctx, apiTimeout)
	defer cancel()

	baseURL := s.baseURL
	if baseURL == "" {
		baseURL = defaultAPIURL
	}
	url := baseURL + fmt.Sprintf(releasesEndpoint, repoOwner, repoName)

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

// ShouldUpdate checks if currentVersion is older than latestTag.
// Returns false for "dev" versions (cannot parse).
func (s *Service) ShouldUpdate(currentVersion, latestTag string) (bool, error) {
	current, err := ParseVersion(currentVersion)
	if err != nil {
		// "dev" or unparseable version - don't update
		return false, nil
	}

	latest, err := ParseVersion(latestTag)
	if err != nil {
		return false, fmt.Errorf("parse latest version %q: %w", latestTag, err)
	}

	return current.IsOlderThan(latest), nil
}
```

**Step 2: Write tests with httptest**

Create `telegram-bot/internal/updater/github_test.go`:

```go
package updater

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetLatestRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/vnd.github.v3+json" {
			t.Errorf("Expected Accept header, got %s", r.Header.Get("Accept"))
		}
		if r.Header.Get("User-Agent") != "vpn-director-telegram-bot" {
			t.Errorf("Expected User-Agent header, got %s", r.Header.Get("User-Agent"))
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"tag_name": "v1.2.3",
			"assets": [
				{"name": "telegram-bot-arm64", "browser_download_url": "https://example.com/arm64"},
				{"name": "telegram-bot-arm", "browser_download_url": "https://example.com/arm"}
			]
		}`))
	}))
	defer server.Close()

	s := NewWithBaseURL(server.URL)
	release, err := s.GetLatestRelease(context.Background())
	if err != nil {
		t.Fatalf("GetLatestRelease() error = %v", err)
	}

	if release.TagName != "v1.2.3" {
		t.Errorf("TagName = %q, want %q", release.TagName, "v1.2.3")
	}
	if len(release.Assets) != 2 {
		t.Fatalf("len(Assets) = %d, want 2", len(release.Assets))
	}
	if release.Assets[0].Name != "telegram-bot-arm64" {
		t.Errorf("Assets[0].Name = %q, want %q", release.Assets[0].Name, "telegram-bot-arm64")
	}
}

func TestGetLatestRelease_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	s := NewWithBaseURL(server.URL)
	_, err := s.GetLatestRelease(context.Background())
	if err == nil {
		t.Error("Expected error for 404 response")
	}
}

func TestShouldUpdate(t *testing.T) {
	s := New()

	tests := []struct {
		name           string
		currentVersion string
		latestTag      string
		want           bool
		wantErr        bool
	}{
		{
			name:           "older version should update",
			currentVersion: "v1.0.0",
			latestTag:      "v1.1.0",
			want:           true,
		},
		{
			name:           "same version should not update",
			currentVersion: "v1.1.0",
			latestTag:      "v1.1.0",
			want:           false,
		},
		{
			name:           "newer version should not update",
			currentVersion: "v1.2.0",
			latestTag:      "v1.1.0",
			want:           false,
		},
		{
			name:           "dev version should not update",
			currentVersion: "dev",
			latestTag:      "v1.1.0",
			want:           false,
		},
		{
			name:           "invalid latest tag",
			currentVersion: "v1.0.0",
			latestTag:      "invalid",
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.ShouldUpdate(tt.currentVersion, tt.latestTag)
			if (err != nil) != tt.wantErr {
				t.Errorf("ShouldUpdate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ShouldUpdate(%q, %q) = %v, want %v",
					tt.currentVersion, tt.latestTag, got, tt.want)
			}
		})
	}
}
```

**Step 3: Run tests**

Run: `cd telegram-bot && go test ./internal/updater/... -v`

Expected: PASS

**Step 4: Commit**

```bash
git add telegram-bot/internal/updater/github.go telegram-bot/internal/updater/github_test.go
git commit -m "feat(telegram-bot): add GitHub API client for releases

Fetches latest release info including tag name and asset download URLs.
ShouldUpdate() compares versions, returns false for dev builds.
Injectable baseURL enables httptest testing.
Uses per-request timeout (30s) for API calls."
```

---

## Task 5: Implement File Downloader

**Files:**
- Create: `telegram-bot/internal/updater/downloader.go`

**Step 1: Write the implementation**

Create `telegram-bot/internal/updater/downloader.go`:

```go
package updater

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	repoRawURL      = "https://raw.githubusercontent.com/%s/%s/refs/tags/%s/%s"
	downloadTimeout = 2 * time.Minute
	maxFileSize     = 50 * 1024 * 1024 // 50MB
)

// scriptFiles lists all files to download from the repository.
// NOTE: Keep in sync with install.sh file list.
var scriptFiles = []string{
	"router/opt/vpn-director/vpn-director.sh",
	"router/opt/vpn-director/configure.sh",
	"router/opt/vpn-director/import_server_list.sh",
	"router/opt/vpn-director/setup_telegram_bot.sh",
	"router/opt/vpn-director/vpn-director.json.template",
	"router/opt/vpn-director/lib/common.sh",
	"router/opt/vpn-director/lib/firewall.sh",
	"router/opt/vpn-director/lib/config.sh",
	"router/opt/vpn-director/lib/ipset.sh",
	"router/opt/vpn-director/lib/tunnel.sh",
	"router/opt/vpn-director/lib/tproxy.sh",
	"router/opt/vpn-director/lib/send-email.sh",
	"router/opt/etc/xray/config.json.template",
	"router/opt/etc/init.d/S99vpn-director",
	"router/opt/etc/init.d/S98telegram-bot",
	"router/jffs/scripts/firewall-start",
	"router/jffs/scripts/wan-event",
}

// DownloadRelease downloads all files for the given release.
// Cleans files/ directory before starting.
func (s *Service) DownloadRelease(ctx context.Context, release *Release) error {
	filesDir := s.getFilesDir()

	// Clean before download to ensure fresh state
	os.RemoveAll(filesDir)

	// Create directory structure
	if err := os.MkdirAll(filesDir, 0755); err != nil {
		return fmt.Errorf("create files directory: %w", err)
	}

	// Download scripts
	for _, file := range scriptFiles {
		if err := s.downloadScriptFile(ctx, release.TagName, file); err != nil {
			return fmt.Errorf("download %s: %w", file, err)
		}
	}

	// Download bot binary
	if err := s.downloadBotBinary(ctx, release); err != nil {
		return fmt.Errorf("download bot binary: %w", err)
	}

	return nil
}

func (s *Service) downloadScriptFile(ctx context.Context, tag, file string) error {
	url := fmt.Sprintf(repoRawURL, repoOwner, repoName, tag, file)

	// Target: "router/opt/vpn-director/lib/common.sh" → "files/opt/vpn-director/lib/common.sh"
	target := filepath.Join(s.getFilesDir(), strings.TrimPrefix(file, "router"))

	return s.downloadFile(ctx, url, target)
}

func (s *Service) downloadBotBinary(ctx context.Context, release *Release) error {
	arch := runtime.GOARCH
	var assetName string

	switch arch {
	case "arm64":
		assetName = "telegram-bot-arm64"
	case "arm":
		assetName = "telegram-bot-arm"
	default:
		return fmt.Errorf("unsupported architecture: %s", arch)
	}

	// Find asset URL
	var downloadURL string
	for _, asset := range release.Assets {
		if asset.Name == assetName {
			downloadURL = asset.DownloadURL
			break
		}
	}
	if downloadURL == "" {
		return fmt.Errorf("binary for architecture %s not found in release", arch)
	}

	target := filepath.Join(s.getFilesDir(), "telegram-bot")
	return s.downloadFile(ctx, downloadURL, target)
}

func (s *Service) downloadFile(ctx context.Context, url, target string) error {
	// Per-request timeout
	ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()

	// Create parent directory
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "vpn-director-telegram-bot")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(target)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	// Limit download size to prevent memory exhaustion
	limitedReader := io.LimitReader(resp.Body, maxFileSize)
	written, err := io.Copy(f, limitedReader)
	if err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	// Check if we hit the limit (file was truncated)
	if written == maxFileSize {
		// Try to read one more byte - if successful, file was too large
		buf := make([]byte, 1)
		if n, _ := resp.Body.Read(buf); n > 0 {
			os.Remove(target)
			return fmt.Errorf("file exceeds maximum size (%d MB)", maxFileSize/1024/1024)
		}
	}

	return nil
}
```

**Step 2: Verify it compiles**

Run: `cd telegram-bot && go build ./internal/updater/...`

Expected: Build succeeds

**Step 3: Commit**

```bash
git add telegram-bot/internal/updater/downloader.go
git commit -m "feat(telegram-bot): add file downloader for /update

Downloads scripts from raw.githubusercontent.com by tag and bot binary
from release assets. Cleans files/ directory before downloading.
Per-request timeout (2 min) and size limit (50MB) for safety."
```

---

## Task 6: Create Update Script Template

**Files:**
- Create: `telegram-bot/internal/updater/update_script.sh.tmpl`

**Step 1: Write the template**

Create `telegram-bot/internal/updater/update_script.sh.tmpl`:

```bash
#!/bin/sh
# Auto-generated by telegram-bot /update command
# Do not edit manually

set -e

CHAT_ID={{.ChatID}}
OLD_VERSION="{{.OldVersion}}"
NEW_VERSION="{{.NewVersion}}"
UPDATE_DIR="{{.UpdateDir}}"
FILES_DIR="{{.FilesDir}}"
NOTIFY_FILE="{{.NotifyFile}}"
LOCK_FILE="{{.LockFile}}"
LOG_FILE="$UPDATE_DIR/update.log"
BOT_PATH="/opt/vpn-director/telegram-bot"

log() {
    echo "$(date '+%Y-%m-%d %H:%M:%S') $1" >> "$LOG_FILE"
}

log "Starting update from $OLD_VERSION to $NEW_VERSION"

# 1. Unmonitor in monit (if available) - optional, continue on failure
if command -v monit >/dev/null 2>&1; then
    log "Unmonitoring telegram-bot in monit"
    monit unmonitor telegram-bot 2>/dev/null || true
fi

# 2. Stop bot
log "Stopping telegram-bot"
/opt/etc/init.d/S98telegram-bot stop
sleep 1

# 3. Wait for process to exit (max 30 seconds)
# Using full path to avoid matching unrelated processes
count=0
while pgrep -f "$BOT_PATH" >/dev/null 2>&1 && [ $count -lt 30 ]; do
    sleep 1
    count=$((count + 1))
done

if pgrep -f "$BOT_PATH" >/dev/null 2>&1; then
    log "WARNING: telegram-bot still running after 30s, killing"
    pkill -9 -f "$BOT_PATH"
    sleep 1
fi

# 4. Copy files - NO || true, fail on error
log "Copying files"
cp -f "$FILES_DIR/opt/vpn-director/"*.sh /opt/vpn-director/
cp -f "$FILES_DIR/opt/vpn-director/lib/"*.sh /opt/vpn-director/lib/
cp -f "$FILES_DIR/opt/vpn-director/"*.template /opt/vpn-director/
cp -f "$FILES_DIR/opt/etc/xray/"*.template /opt/etc/xray/
cp -f "$FILES_DIR/opt/etc/init.d/"* /opt/etc/init.d/
cp -f "$FILES_DIR/jffs/scripts/"* /jffs/scripts/
cp -f "$FILES_DIR/telegram-bot" /opt/vpn-director/telegram-bot

# 5. Set permissions
chmod +x /opt/vpn-director/*.sh
chmod +x /opt/vpn-director/lib/*.sh
chmod +x /opt/etc/init.d/S98telegram-bot
chmod +x /opt/etc/init.d/S99vpn-director
chmod +x /jffs/scripts/firewall-start
chmod +x /jffs/scripts/wan-event
chmod +x /opt/vpn-director/telegram-bot

# 6. Create notify file
log "Creating notify file"
cat > "$NOTIFY_FILE" << EOF
{"chat_id":$CHAT_ID,"old_version":"$OLD_VERSION","new_version":"$NEW_VERSION"}
EOF

# 7. Remove lock
rm -f "$LOCK_FILE"

# 8. Re-monitor in monit (if available) - optional, continue on failure
if command -v monit >/dev/null 2>&1; then
    log "Re-monitoring telegram-bot in monit"
    monit monitor telegram-bot 2>/dev/null || true
fi

# 9. Start bot
log "Starting telegram-bot"
/opt/etc/init.d/S98telegram-bot start

# 10. Cleanup deferred to bot after successful startup notification
# (leave $UPDATE_DIR for notify.json and update.log)
```

**Step 2: Verify file is created**

Run: `ls -la telegram-bot/internal/updater/update_script.sh.tmpl`

Expected: File exists

**Step 3: Commit**

```bash
git add telegram-bot/internal/updater/update_script.sh.tmpl
git commit -m "feat(telegram-bot): add shell script template for /update

Go template that generates update script. Uses strict set -e.
Only monit commands use || true (monit is optional).
Uses full path in pgrep to avoid matching unrelated processes.
Leaves update directory for bot cleanup after successful notify."
```

---

## Task 7: Implement Script Generator

**Files:**
- Create: `telegram-bot/internal/updater/script.go`
- Create: `telegram-bot/internal/updater/script_test.go`

**Step 1: Write the implementation**

Create `telegram-bot/internal/updater/script.go`:

```go
package updater

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
)

//go:embed update_script.sh.tmpl
var updateScriptTemplate string

type scriptData struct {
	ChatID     int64
	OldVersion string
	NewVersion string
	UpdateDir  string
	FilesDir   string
	NotifyFile string
	LockFile   string
}

// RunUpdateScript generates the update script and runs it detached.
func (s *Service) RunUpdateScript(chatID int64, oldVersion, newVersion string) error {
	// Validate versions before embedding in shell script
	if !IsValidVersion(oldVersion) {
		return fmt.Errorf("invalid old version: %q", oldVersion)
	}
	if !IsValidVersion(newVersion) {
		return fmt.Errorf("invalid new version: %q", newVersion)
	}

	script, err := s.generateScript(chatID, oldVersion, newVersion)
	if err != nil {
		return fmt.Errorf("generate script: %w", err)
	}

	scriptPath := s.getScriptFile()

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0755); err != nil {
		return fmt.Errorf("create script directory: %w", err)
	}

	// Write script
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		return fmt.Errorf("write script: %w", err)
	}

	// Run detached: nohup /bin/sh script.sh >> log 2>&1 &
	updateDir := s.getUpdateDir()
	cmd := exec.Command("/bin/sh", "-c",
		fmt.Sprintf("nohup %s >> %s/update.log 2>&1 &", scriptPath, updateDir))

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start script: %w", err)
	}

	// Don't wait - script will kill this process
	return nil
}

func (s *Service) generateScript(chatID int64, oldVersion, newVersion string) (string, error) {
	tmpl, err := template.New("update").Parse(updateScriptTemplate)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	updateDir := s.getUpdateDir()

	data := scriptData{
		ChatID:     chatID,
		OldVersion: oldVersion,
		NewVersion: newVersion,
		UpdateDir:  updateDir,
		FilesDir:   filepath.Join(updateDir, "files"),
		NotifyFile: filepath.Join(updateDir, "notify.json"),
		LockFile:   filepath.Join(updateDir, "lock"),
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return buf.String(), nil
}
```

**Step 2: Write tests**

Create `telegram-bot/internal/updater/script_test.go`:

```go
package updater

import (
	"strings"
	"testing"
)

func TestGenerateScript(t *testing.T) {
	tmpDir := t.TempDir()

	s := &Service{
		updateDir: tmpDir,
	}

	script, err := s.generateScript(123456789, "v1.0.0", "v1.1.0")
	if err != nil {
		t.Fatalf("generateScript() error = %v", err)
	}

	// Check that variables are embedded
	checks := []string{
		"CHAT_ID=123456789",
		`OLD_VERSION="v1.0.0"`,
		`NEW_VERSION="v1.1.0"`,
		"set -e",
		"S98telegram-bot stop",
		"S98telegram-bot start",
		`pgrep -f "$BOT_PATH"`,  // full path variable used
	}

	for _, check := range checks {
		if !strings.Contains(script, check) {
			t.Errorf("Script missing %q", check)
		}
	}

	// Check that monit commands have || true
	if !strings.Contains(script, "monit unmonitor telegram-bot 2>/dev/null || true") {
		t.Error("Script missing || true for monit unmonitor")
	}

	// Check that cp commands do NOT have || true
	for _, line := range strings.Split(script, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "cp -f") {
			if strings.HasSuffix(strings.TrimSpace(line), "|| true") {
				t.Errorf("cp command should not have || true: %s", line)
			}
		}
	}
}

func TestRunUpdateScript_InvalidVersion(t *testing.T) {
	s := &Service{
		updateDir: t.TempDir(),
	}

	tests := []struct {
		name       string
		oldVersion string
		newVersion string
	}{
		{"shell injection in old", "v1.0.0;rm -rf /", "v1.1.0"},
		{"shell injection in new", "v1.0.0", "v1.1.0$(whoami)"},
		{"backticks in old", "`id`", "v1.1.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := s.RunUpdateScript(123, tt.oldVersion, tt.newVersion)
			if err == nil {
				t.Error("Expected error for invalid version")
			}
		})
	}
}
```

**Step 3: Run tests**

Run: `cd telegram-bot && go test ./internal/updater/... -v`

Expected: PASS

**Step 4: Commit**

```bash
git add telegram-bot/internal/updater/script.go telegram-bot/internal/updater/script_test.go
git commit -m "feat(telegram-bot): add script generator for /update

Uses go:embed to load template, generates script with embedded
variables, runs it detached via nohup.
Validates version strings before embedding to prevent shell injection."
```

---

## Task 8: Create Startup Notification Handler

**Files:**
- Create: `telegram-bot/internal/startup/notify.go`
- Create: `telegram-bot/internal/startup/notify_test.go`

**Step 1: Write the implementation**

Create `telegram-bot/internal/startup/notify.go`:

```go
package startup

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/telegram"
)

// UpdateNotification is the JSON structure in notify.json.
type UpdateNotification struct {
	ChatID     int64  `json:"chat_id"`
	OldVersion string `json:"old_version"`
	NewVersion string `json:"new_version"`
}

// DefaultNotifyFile is the default path to the notification file.
const DefaultNotifyFile = "/tmp/vpn-director-update/notify.json"

// DefaultUpdateDir is the default update directory to clean up after success.
const DefaultUpdateDir = "/tmp/vpn-director-update"

// CheckAndSendNotify checks for pending update notification and sends it.
// Uses telegram.MessageSender interface, calling SendPlain for plain text.
// Cleans up the update directory only after a successful send.
func CheckAndSendNotify(sender telegram.MessageSender, notifyFile, updateDir string) error {
	data, err := os.ReadFile(notifyFile)
	if os.IsNotExist(err) {
		return nil // No notification pending
	}
	if err != nil {
		return fmt.Errorf("read notify file: %w", err)
	}

	var n UpdateNotification
	if err := json.Unmarshal(data, &n); err != nil {
		return fmt.Errorf("parse notify file: %w", err)
	}

	// Send plain text message (no Markdown)
	text := fmt.Sprintf("Update complete: %s → %s", n.OldVersion, n.NewVersion)
	if err := sender.SendPlain(n.ChatID, text); err != nil {
		return fmt.Errorf("send notification: %w", err)
	}

	// Cleanup entire update directory after successful send
	os.RemoveAll(updateDir)

	return nil
}
```

**Step 2: Write tests**

Create `telegram-bot/internal/startup/notify_test.go`:

```go
package startup

import (
	"os"
	"path/filepath"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// mockSender implements telegram.MessageSender for testing
type mockSender struct {
	sentMessages []sentMessage
}

type sentMessage struct {
	chatID int64
	text   string
	plain  bool
}

func (m *mockSender) Send(chatID int64, text string) error {
	m.sentMessages = append(m.sentMessages, sentMessage{chatID, text, false})
	return nil
}

func (m *mockSender) SendPlain(chatID int64, text string) error {
	m.sentMessages = append(m.sentMessages, sentMessage{chatID, text, true})
	return nil
}

func (m *mockSender) SendLongPlain(chatID int64, text string) error {
	return m.SendPlain(chatID, text)
}

func (m *mockSender) SendWithKeyboard(chatID int64, text string, keyboard tgbotapi.InlineKeyboardMarkup) error {
	return nil
}

func (m *mockSender) SendCodeBlock(chatID int64, header, content string) error {
	return nil
}

func (m *mockSender) EditMessage(chatID int64, msgID int, text string, keyboard tgbotapi.InlineKeyboardMarkup) error {
	return nil
}

func (m *mockSender) AckCallback(callbackID string) error {
	return nil
}

func TestCheckAndSendNotify(t *testing.T) {
	tmpDir := t.TempDir()
	notifyFile := filepath.Join(tmpDir, "notify.json")

	// Create notify file
	jsonData := `{"chat_id":123456789,"old_version":"v1.0.0","new_version":"v1.1.0"}`
	if err := os.WriteFile(notifyFile, []byte(jsonData), 0644); err != nil {
		t.Fatal(err)
	}

	mockSnd := &mockSender{}

	err := CheckAndSendNotify(mockSnd, notifyFile, tmpDir)
	if err != nil {
		t.Fatalf("CheckAndSendNotify() error = %v", err)
	}

	// Check message was sent as plain text
	if len(mockSnd.sentMessages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(mockSnd.sentMessages))
	}

	msg := mockSnd.sentMessages[0]
	if msg.chatID != 123456789 {
		t.Errorf("chatID = %d, want 123456789", msg.chatID)
	}
	if msg.text != "Update complete: v1.0.0 → v1.1.0" {
		t.Errorf("text = %q, want 'Update complete: v1.0.0 → v1.1.0'", msg.text)
	}
	if !msg.plain {
		t.Error("Message should be sent as plain text")
	}

	// Check update dir was deleted
	if _, err := os.Stat(tmpDir); !os.IsNotExist(err) {
		t.Error("update dir should be deleted")
	}
}

func TestCheckAndSendNotify_NoFile(t *testing.T) {
	tmpDir := t.TempDir()
	notifyFile := filepath.Join(tmpDir, "notify.json")

	mockSnd := &mockSender{}

	// Should not error when file doesn't exist
	err := CheckAndSendNotify(mockSnd, notifyFile, tmpDir)
	if err != nil {
		t.Fatalf("CheckAndSendNotify() error = %v", err)
	}

	// No messages sent
	if len(mockSnd.sentMessages) != 0 {
		t.Errorf("Expected 0 messages, got %d", len(mockSnd.sentMessages))
	}
}
```

**Step 3: Run tests**

Run: `cd telegram-bot && go test ./internal/startup/... -v`

Expected: PASS

**Step 4: Commit**

```bash
git add telegram-bot/internal/startup/notify.go telegram-bot/internal/startup/notify_test.go
git commit -m "feat(telegram-bot): add startup notification for /update

Checks for notify.json on startup, sends completion message to chat
using telegram.MessageSender.SendPlain(), cleans up update directory
only after successful send."
```

---

## Task 9: Create Update Command Handler

**Files:**
- Create: `telegram-bot/internal/handler/update.go`
- Create: `telegram-bot/internal/handler/update_test.go`

**Step 1: Write the handler**

Create `telegram-bot/internal/handler/update.go`:

```go
package handler

import (
	"context"
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/telegram"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/updater"
)

// UpdateHandler handles the /update command.
type UpdateHandler struct {
	sender  telegram.MessageSender
	updater updater.Updater
	devMode bool
	version string
}

// NewUpdateHandler creates a new update handler.
func NewUpdateHandler(sender telegram.MessageSender, upd updater.Updater, devMode bool, version string) *UpdateHandler {
	return &UpdateHandler{
		sender:  sender,
		updater: upd,
		devMode: devMode,
		version: version,
	}
}

// HandleUpdate processes the /update command.
func (h *UpdateHandler) HandleUpdate(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID

	// 1. Dev mode check
	if h.devMode {
		h.send(chatID, "Command /update is not available in dev mode")
		return
	}

	// 2. Dev version check
	if h.version == "dev" {
		h.send(chatID, "Cannot check updates for dev build")
		return
	}

	// 3. Lock check
	if h.updater.IsUpdateInProgress() {
		h.send(chatID, "Update is already in progress, please wait...")
		return
	}

	// 4. Get latest release
	ctx := context.Background()
	release, err := h.updater.GetLatestRelease(ctx)
	if err != nil {
		h.send(chatID, fmt.Sprintf("Failed to check for updates: %v", err))
		return
	}

	// 5. Validate versions
	if !updater.IsValidVersion(h.version) {
		h.send(chatID, fmt.Sprintf("Invalid current version: %s", h.version))
		return
	}
	if !updater.IsValidVersion(release.TagName) {
		h.send(chatID, fmt.Sprintf("Invalid release version: %s", release.TagName))
		return
	}

	// 6. Compare versions
	shouldUpdate, err := h.updater.ShouldUpdate(h.version, release.TagName)
	if err != nil {
		h.send(chatID, fmt.Sprintf("Failed to parse version: %v", err))
		return
	}
	if !shouldUpdate {
		h.send(chatID, fmt.Sprintf("Already running the latest version: %s", h.version))
		return
	}

	// 7. Create lock
	if err := h.updater.CreateLock(); err != nil {
		h.send(chatID, fmt.Sprintf("Failed to start update: %v", err))
		return
	}

	// 8. Notify start
	h.send(chatID, fmt.Sprintf("Starting update %s → %s...", h.version, release.TagName))

	// 9. Download in goroutine
	go h.downloadAndUpdate(chatID, release)
}

func (h *UpdateHandler) downloadAndUpdate(chatID int64, release *updater.Release) {
	ctx := context.Background()

	// Download files
	err := h.updater.DownloadRelease(ctx, release)
	if err != nil {
		h.updater.CleanFiles()
		h.updater.RemoveLock()
		h.send(chatID, fmt.Sprintf("Download failed: %v", err))
		return
	}

	// 10. Notify downloaded
	h.send(chatID, "Files downloaded, starting update...")

	// 11. Run update script
	if err := h.updater.RunUpdateScript(chatID, h.version, release.TagName); err != nil {
		h.updater.CleanFiles()
		h.updater.RemoveLock()
		h.send(chatID, fmt.Sprintf("Failed to run update script: %v", err))
		return
	}

	// 12. Notify script started
	h.send(chatID, "Update script started, bot will restart in a few seconds...")
}

func (h *UpdateHandler) send(chatID int64, text string) {
	_ = h.sender.SendPlain(chatID, text)
}
```

**Step 2: Write tests**

Create `telegram-bot/internal/handler/update_test.go`:

```go
package handler

import (
	"context"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/updater"
)

type mockUpdater struct {
	latestRelease    *updater.Release
	shouldUpdate     bool
	updateInProgress bool
	createLockErr    error
	downloadErr      error
	runScriptErr     error
}

func (m *mockUpdater) GetLatestRelease(ctx context.Context) (*updater.Release, error) {
	return m.latestRelease, nil
}

func (m *mockUpdater) ShouldUpdate(currentVersion, latestTag string) (bool, error) {
	return m.shouldUpdate, nil
}

func (m *mockUpdater) IsUpdateInProgress() bool {
	return m.updateInProgress
}

func (m *mockUpdater) CreateLock() error {
	return m.createLockErr
}

func (m *mockUpdater) RemoveLock() {}

func (m *mockUpdater) CleanFiles() {}

func (m *mockUpdater) DownloadRelease(ctx context.Context, release *updater.Release) error {
	return m.downloadErr
}

func (m *mockUpdater) RunUpdateScript(chatID int64, oldVersion, newVersion string) error {
	return m.runScriptErr
}

type mockUpdateSender struct {
	messages []string
}

func (m *mockUpdateSender) Send(chatID int64, text string) error {
	m.messages = append(m.messages, text)
	return nil
}

func (m *mockUpdateSender) SendPlain(chatID int64, text string) error {
	m.messages = append(m.messages, text)
	return nil
}

func (m *mockUpdateSender) SendLongPlain(chatID int64, text string) error {
	return m.SendPlain(chatID, text)
}

func (m *mockUpdateSender) SendWithKeyboard(chatID int64, text string, keyboard tgbotapi.InlineKeyboardMarkup) error {
	return nil
}

func (m *mockUpdateSender) SendCodeBlock(chatID int64, header, content string) error {
	return nil
}

func (m *mockUpdateSender) EditMessage(chatID int64, msgID int, text string, keyboard tgbotapi.InlineKeyboardMarkup) error {
	return nil
}

func (m *mockUpdateSender) AckCallback(callbackID string) error {
	return nil
}

func TestUpdateHandler_DevMode(t *testing.T) {
	sender := &mockUpdateSender{}
	h := NewUpdateHandler(sender, &mockUpdater{}, true, "v1.0.0")

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}}
	h.HandleUpdate(msg)

	if len(sender.messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(sender.messages))
	}
	if sender.messages[0] != "Command /update is not available in dev mode" {
		t.Errorf("Unexpected message: %s", sender.messages[0])
	}
}

func TestUpdateHandler_DevVersion(t *testing.T) {
	sender := &mockUpdateSender{}
	h := NewUpdateHandler(sender, &mockUpdater{}, false, "dev")

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}}
	h.HandleUpdate(msg)

	if len(sender.messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(sender.messages))
	}
	if sender.messages[0] != "Cannot check updates for dev build" {
		t.Errorf("Unexpected message: %s", sender.messages[0])
	}
}

func TestUpdateHandler_AlreadyLatest(t *testing.T) {
	sender := &mockUpdateSender{}
	upd := &mockUpdater{
		latestRelease: &updater.Release{TagName: "v1.0.0"},
		shouldUpdate:  false,
	}
	h := NewUpdateHandler(sender, upd, false, "v1.0.0")

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}}
	h.HandleUpdate(msg)

	if len(sender.messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(sender.messages))
	}
	if sender.messages[0] != "Already running the latest version: v1.0.0" {
		t.Errorf("Unexpected message: %s", sender.messages[0])
	}
}

func TestUpdateHandler_UpdateInProgress(t *testing.T) {
	sender := &mockUpdateSender{}
	upd := &mockUpdater{
		updateInProgress: true,
	}
	h := NewUpdateHandler(sender, upd, false, "v1.0.0")

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}}
	h.HandleUpdate(msg)

	if len(sender.messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(sender.messages))
	}
	if sender.messages[0] != "Update is already in progress, please wait..." {
		t.Errorf("Unexpected message: %s", sender.messages[0])
	}
}
```

**Step 3: Verify it compiles and tests pass**

Run: `cd telegram-bot && go test ./internal/handler/... -v`

Expected: PASS

**Step 4: Commit**

```bash
git add telegram-bot/internal/handler/update.go telegram-bot/internal/handler/update_test.go
git commit -m "feat(telegram-bot): add /update command handler

Checks dev mode, dev version, lock, version validation, comparison.
Downloads files in goroutine for responsiveness.
Cleans files/ on error. Sends progress messages.
Includes tests for all early-exit conditions."
```

---

## Task 10: Refactor bot.New() to Options Pattern

**Files:**
- Modify: `telegram-bot/internal/bot/bot.go`

**Step 1: Read current bot.go structure**

Read the file to understand current implementation.

**Step 2: Add Options pattern, update struct and New()**

Add after imports:
```go
import (
	// ... existing imports ...
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/startup"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/updater"
)

// Option configures the Bot.
type Option func(*Bot)

// WithDevMode enables development mode with custom executor.
func WithDevMode(executor service.ShellExecutor) Option {
	return func(b *Bot) {
		b.devMode = true
		b.executor = executor
	}
}

// WithUpdater sets the updater service.
func WithUpdater(u updater.Updater) Option {
	return func(b *Bot) {
		b.updater = u
	}
}
```

Update Bot struct:
```go
type Bot struct {
	api      *tgbotapi.BotAPI
	auth     *Auth
	router   *Router
	sender   telegram.MessageSender
	devMode  bool
	executor service.ShellExecutor
	updater  updater.Updater
}
```

Update New() signature:
```go
func New(cfg *config.Config, p paths.Paths, version, versionFull, commit, buildDate string, opts ...Option) (*Bot, error)
```

Apply options after creating bot:
```go
b := &Bot{...}
for _, opt := range opts {
    opt(b)
}
```

Pass to Deps:
```go
deps := &handler.Deps{
    // ... existing ...
    DevMode:     b.devMode,
    Updater:     b.updater,
    Version:     version,
    VersionFull: versionFull,
    Commit:      commit,
    BuildDate:   buildDate,
}
```

**Step 3: Add /update to commands list**

```go
{Command: "update", Description: "Update VPN Director to latest release"},
```

**Step 4: Add CheckAndSendNotify call in Run()**

At the start of Run():
```go
func (b *Bot) Run(ctx context.Context) {
	// Check for pending update notification before starting polling
	if err := startup.CheckAndSendNotify(b.sender, startup.DefaultNotifyFile, startup.DefaultUpdateDir); err != nil {
		slog.Warn("Failed to send update notification", "error", err)
	}

	// ... existing polling code ...
}
```

**Step 5: Commit**

```bash
git add telegram-bot/internal/bot/bot.go
git commit -m "refactor(telegram-bot): use Options pattern for bot.New()

Add WithDevMode() and WithUpdater() options for cleaner configuration.
Register /update command with BotFather.
Call CheckAndSendNotify() at start of Run() for update notifications."
```

---

## Task 11: Update Handler Deps

**Files:**
- Modify: `telegram-bot/internal/handler/handler.go`

**Step 1: Add new fields to Deps**

```go
import (
	// ... existing imports ...
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/updater"
)

type Deps struct {
	Sender      telegram.MessageSender
	Config      service.ConfigStore
	VPN         service.VPNDirector
	Xray        service.XrayGenerator
	Network     service.NetworkInfo
	Logs        service.LogReader
	Paths       paths.Paths
	Version     string
	VersionFull string           // ADD
	Commit      string           // ADD
	BuildDate   string           // ADD
	DevMode     bool             // ADD
	Updater     updater.Updater  // ADD
}
```

**Step 2: Commit**

```bash
git add telegram-bot/internal/handler/handler.go
git commit -m "feat(telegram-bot): add Updater and version fields to Deps

Add DevMode, Updater, VersionFull, Commit, BuildDate to handler
dependencies for /update command and improved /version display."
```

---

## Task 12: Add /update to /start Help Text

**Files:**
- Modify: `telegram-bot/internal/handler/misc.go`

**Step 1: Find HandleStart function and add /update**

Add to the help text in HandleStart():
```go
"/update - Update VPN Director to latest release"
```

**Step 2: Update HandleVersion to use VersionFull**

Change HandleVersion to show full version info:
```go
func (h *MiscHandler) HandleVersion(msg *tgbotapi.Message) {
	text := fmt.Sprintf("%s (%s, %s)", h.deps.VersionFull, h.deps.Commit, h.deps.BuildDate)
	h.deps.Sender.Send(msg.Chat.ID, telegram.EscapeMarkdownV2(text))
}
```

**Step 3: Commit**

```bash
git add telegram-bot/internal/handler/misc.go
git commit -m "feat(telegram-bot): add /update to /start help text

Users can now discover the /update command from /start help.
HandleVersion now shows VersionFull with commit and build date."
```

---

## Task 13: Add Route for /update

**Files:**
- Modify: `telegram-bot/internal/bot/router.go`

**Step 1: Add UpdateRouterHandler interface**

```go
// UpdateRouterHandler defines methods for update command
type UpdateRouterHandler interface {
	HandleUpdate(msg *tgbotapi.Message)
}
```

**Step 2: Add to Router struct**

```go
type Router struct {
	// ... existing fields ...
	update UpdateRouterHandler
}
```

**Step 3: Update NewRouter parameters and initialization**

**Step 4: Add case in RouteMessage**

```go
case "update":
    r.update.HandleUpdate(msg)
```

**Step 5: Commit**

```bash
git add telegram-bot/internal/bot/router.go
git commit -m "feat(telegram-bot): add /update route

Register UpdateRouterHandler and route /update command."
```

---

## Task 14: Update main.go for Integration

**Files:**
- Modify: `telegram-bot/cmd/bot/main.go`

**Step 1: Update version variables**

```go
var (
	Version     = "dev"
	VersionFull = "dev"
	Commit      = "unknown"
	BuildDate   = "unknown"
)
```

**Step 2: Update versionString() to use VersionFull**

```go
func versionString() string {
	return fmt.Sprintf("%s (%s, %s)", VersionFull, Commit, BuildDate)
}
```

**Step 3: Add import for updater**

```go
import (
	// ... existing imports ...
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/updater"
)
```

**Step 4: Use Options pattern**

Replace executor handling with Options:
```go
var opts []bot.Option
if *devFlag {
    opts = append(opts, bot.WithDevMode(devmode.NewExecutor()))
}
opts = append(opts, bot.WithUpdater(updater.New()))

b, err := bot.New(cfg, p, Version, VersionFull, Commit, BuildDate, opts...)
```

**Step 5: Commit**

```bash
git add telegram-bot/cmd/bot/main.go
git commit -m "feat(telegram-bot): integrate /update command

- Add VersionFull variable for full git describe output
- Use Options pattern for bot configuration
- Initialize updater service"
```

---

## Task 15: Update init.d to Use Absolute Bot Path

**Files:**
- Modify: `router/opt/etc/init.d/S98telegram-bot`

**Step 1: Update startup to use absolute path**

Ensure the bot is started via `/opt/vpn-director/telegram-bot` (not PATH lookup), so `pgrep -f /opt/vpn-director/telegram-bot` matches the running process reliably.

**Step 2: Verify script still starts the bot**

No behavior change expected besides the command line containing the absolute path.

**Step 3: Commit**

```bash
git add router/opt/etc/init.d/S98telegram-bot
git commit -m "fix(init.d): start telegram-bot via absolute path

Ensures the command line includes /opt/vpn-director/telegram-bot so
update script process checks (pgrep -f) are reliable."
```

---

## Task 16: Final Testing

**Step 1: Run all tests**

Run: `cd telegram-bot && go test ./... -v`

Expected: All tests pass

**Step 2: Build for all architectures**

Run: `cd telegram-bot && make build-arm64 && make build-arm`

Expected: Both binaries created in `bin/`

**Step 3: Commit all changes**

Create final PR commit message:

```
feat(telegram-bot): add /update command for self-updating

Implements self-update functionality via /update command:

- Downloads scripts from raw.githubusercontent.com by release tag
- Downloads bot binary from release assets
- Generates shell script that stops bot, replaces files, restarts bot
- Bot sends completion notification on startup after update
- Lock file with PID prevents concurrent updates
- Dev mode check prevents updates in development
- Options pattern for bot.New() configuration
- Injectable GitHub API URL for testing
- Version validation prevents shell injection
- Per-request timeouts and size limits for downloads
- Full path in pgrep for accurate process matching
- init.d starts telegram-bot via absolute path for reliable process checks

Design: docs/plans/2026-01-25-telegram-bot-update-command-design.md
```

---

## Summary

| Task | Description | Files |
|------|-------------|-------|
| 1 | Makefile VERSION + VERSION_FULL | `Makefile` |
| 2 | Version parser + validation | `updater/version.go`, `version_test.go` |
| 3 | Updater interface | `updater/updater.go` |
| 4 | GitHub API client | `updater/github.go`, `github_test.go` |
| 5 | File downloader | `updater/downloader.go` |
| 6 | Script template | `updater/update_script.sh.tmpl` |
| 7 | Script generator | `updater/script.go`, `script_test.go` |
| 8 | Startup notification | `startup/notify.go`, `notify_test.go` |
| 9 | Update handler + tests | `handler/update.go`, `update_test.go` |
| 10 | Options pattern + notify in Run() | `bot/bot.go` |
| 11 | Handler Deps | `handler/handler.go` |
| 12 | /start help text + HandleVersion | `handler/misc.go` |
| 13 | Router integration | `bot/router.go` |
| 14 | Main integration | `cmd/bot/main.go` |
| 15 | init.d uses absolute bot path | `router/opt/etc/init.d/S98telegram-bot` |
| 16 | Final testing | Tests, builds |
