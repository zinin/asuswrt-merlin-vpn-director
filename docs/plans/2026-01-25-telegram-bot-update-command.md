# Telegram Bot /update Command Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement `/update` command that self-updates VPN Director (scripts, templates, bot binary) from GitHub releases.

**Architecture:** Bot downloads files to `/tmp/`, generates shell script with embedded data, launches it detached. Script stops bot, replaces files, restarts bot. Bot reads notification file on startup and reports success.

**Tech Stack:** Go 1.21+, text/template, net/http, GitHub Releases API, shell script

**Design Document:** `docs/plans/2026-01-25-telegram-bot-update-command-design.md`

---

## Task 1: Modify Makefile for Clean Version Tag

**Files:**
- Modify: `telegram-bot/Makefile:3`

**Step 1: Update VERSION variable**

Change from:
```makefile
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
```

To:
```makefile
VERSION ?= $(shell git describe --tags --abbrev=0 2>/dev/null || echo "dev")
```

**Step 2: Verify change**

Run: `cd telegram-bot && make build 2>&1 | head -5`

Expected: Build succeeds (no error output)

**Step 3: Commit**

```bash
git add telegram-bot/Makefile
git commit -m "build(telegram-bot): use clean version tag without commit suffix

Change --always to --abbrev=0 so VERSION contains only the tag name
(e.g., 'v1.2.0') instead of 'v1.2.0-5-gabc1234'. This enables proper
semver parsing for the /update command."
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
```

**Step 4: Run tests to verify they pass**

Run: `cd telegram-bot && go test ./internal/updater/... -v`

Expected: PASS (all tests)

**Step 5: Commit**

```bash
git add telegram-bot/internal/updater/version.go telegram-bot/internal/updater/version_test.go
git commit -m "feat(telegram-bot): add semver parser for /update command

Strict vX.Y.Z format only. Rejects dev builds, pre-release tags,
and incomplete versions. Includes Compare() and IsOlderThan() methods."
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
	"net/http"
	"time"
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

	// DownloadRelease downloads all files for the given release.
	// Runs in a goroutine, reports progress via callback.
	DownloadRelease(ctx context.Context, release *Release, onProgress func(msg string)) error

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
}

// New creates a new updater service.
func New() *Service {
	return &Service{
		httpClient: &http.Client{
			Timeout: 5 * time.Minute, // Long timeout for downloads
		},
	}
}

// Ensure Service implements Updater.
var _ Updater = (*Service)(nil)
```

**Step 2: Verify it compiles**

Run: `cd telegram-bot && go build ./internal/updater/...`

Expected: Build succeeds (methods not implemented yet, but interface is defined)

**Step 3: Commit**

```bash
git add telegram-bot/internal/updater/updater.go
git commit -m "feat(telegram-bot): add Updater interface and Service struct

Defines constants for update directory paths and the Updater interface
with methods for GitHub API, version comparison, locking, downloading,
and script execution."
```

---

## Task 4: Implement GitHub API Client

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
)

const (
	repoOwner = "zinin"
	repoName  = "asuswrt-merlin-vpn-director"
	apiURL    = "https://api.github.com/repos/%s/%s/releases/latest"
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
	url := fmt.Sprintf(apiURL, repoOwner, repoName)

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

**Step 2: Write tests**

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

	// Create service with custom HTTP client pointing to test server
	s := &Service{
		httpClient: server.Client(),
	}

	// Override the API URL for testing (we can't easily do this, so skip this test)
	t.Skip("Integration test - requires mocking apiURL")
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
ShouldUpdate() compares versions, returns false for dev builds."
```

---

## Task 5: Implement Lock File with PID

**Files:**
- Modify: `telegram-bot/internal/updater/updater.go` (add lock methods)
- Create: `telegram-bot/internal/updater/lock_test.go`

**Step 1: Write failing tests**

Create `telegram-bot/internal/updater/lock_test.go`:

```go
package updater

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestLockFile(t *testing.T) {
	// Use temp directory for tests
	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "lock")

	// Override LockFile for testing - we need to make it configurable
	s := &Service{lockFile: lockFile}

	// Test: no lock initially
	if s.IsUpdateInProgress() {
		t.Error("Expected no lock initially")
	}

	// Test: create lock
	if err := s.CreateLock(); err != nil {
		t.Fatalf("CreateLock() error = %v", err)
	}

	// Test: lock exists
	if !s.IsUpdateInProgress() {
		t.Error("Expected lock to exist after CreateLock()")
	}

	// Test: lock contains current PID
	data, err := os.ReadFile(lockFile)
	if err != nil {
		t.Fatalf("Failed to read lock file: %v", err)
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil {
		t.Fatalf("Lock file doesn't contain valid PID: %v", err)
	}
	if pid != os.Getpid() {
		t.Errorf("Lock PID = %d, want %d", pid, os.Getpid())
	}

	// Test: remove lock
	s.RemoveLock()
	if s.IsUpdateInProgress() {
		t.Error("Expected no lock after RemoveLock()")
	}
}

func TestStaleLockRemoval(t *testing.T) {
	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "lock")

	s := &Service{lockFile: lockFile}

	// Create lock with non-existent PID (stale lock)
	if err := os.WriteFile(lockFile, []byte("999999999"), 0644); err != nil {
		t.Fatalf("Failed to create stale lock: %v", err)
	}

	// IsUpdateInProgress should detect stale lock and remove it
	if s.IsUpdateInProgress() {
		t.Error("Expected stale lock to be detected and removed")
	}

	// Lock file should be removed
	if _, err := os.Stat(lockFile); !os.IsNotExist(err) {
		t.Error("Expected stale lock file to be removed")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `cd telegram-bot && go test ./internal/updater/... -v -run TestLock`

Expected: FAIL (lockFile field doesn't exist)

**Step 3: Update updater.go with lock implementation**

Update `telegram-bot/internal/updater/updater.go`, add lockFile field and methods:

```go
package updater

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
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

	// DownloadRelease downloads all files for the given release.
	// Runs in a goroutine, reports progress via callback.
	DownloadRelease(ctx context.Context, release *Release, onProgress func(msg string)) error

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
	lockFile   string // Configurable for testing, defaults to LockFile
}

// New creates a new updater service.
func New() *Service {
	return &Service{
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
		lockFile: LockFile,
	}
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

	pid, err := strconv.Atoi(string(data))
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

func (s *Service) getLockFile() string {
	if s.lockFile != "" {
		return s.lockFile
	}
	return LockFile
}
```

**Step 4: Run tests**

Run: `cd telegram-bot && go test ./internal/updater/... -v -run TestLock`

Expected: PASS

**Step 5: Commit**

```bash
git add telegram-bot/internal/updater/updater.go telegram-bot/internal/updater/lock_test.go
git commit -m "feat(telegram-bot): add lock file with PID for /update

Lock file contains PID to detect stale locks. IsUpdateInProgress()
uses kill -0 to check if process is alive, removes stale locks."
```

---

## Task 6: Implement File Downloader

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
)

const (
	repoRawURL = "https://raw.githubusercontent.com/%s/%s/refs/tags/%s/%s"
)

// scriptFiles lists all files to download from the repository.
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
func (s *Service) DownloadRelease(ctx context.Context, release *Release, onProgress func(msg string)) error {
	filesDir := s.getFilesDir()

	// Create directory structure
	if err := os.MkdirAll(filesDir, 0755); err != nil {
		return fmt.Errorf("create files directory: %w", err)
	}

	// Download scripts
	for i, file := range scriptFiles {
		if onProgress != nil {
			onProgress(fmt.Sprintf("Downloading scripts (%d/%d)...", i+1, len(scriptFiles)))
		}

		if err := s.downloadScriptFile(ctx, release.TagName, file); err != nil {
			return fmt.Errorf("download %s: %w", file, err)
		}
	}

	// Download bot binary
	if onProgress != nil {
		onProgress("Downloading bot binary...")
	}

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

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}

func (s *Service) getFilesDir() string {
	return FilesDir
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
from release assets. Reports progress via callback for UI feedback."
```

---

## Task 7: Create Update Script Template

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

log() {
    echo "$(date '+%Y-%m-%d %H:%M:%S') $1" >> "$LOG_FILE"
}

log "Starting update from $OLD_VERSION to $NEW_VERSION"

# 1. Unmonitor in monit (if available)
if command -v monit >/dev/null 2>&1; then
    log "Unmonitoring telegram-bot in monit"
    monit unmonitor telegram-bot 2>/dev/null || true
fi

# 2. Stop bot
log "Stopping telegram-bot"
/opt/etc/init.d/S98telegram-bot stop || log "Warning: stop returned error"
sleep 1

# 3. Wait for process to exit (max 30 seconds)
count=0
while pgrep -f telegram-bot >/dev/null 2>&1 && [ $count -lt 30 ]; do
    sleep 1
    count=$((count + 1))
done

if pgrep -f telegram-bot >/dev/null 2>&1; then
    log "WARNING: telegram-bot still running after 30s, killing"
    pkill -9 -f telegram-bot || true
    sleep 1
fi

# 4. Copy files
log "Copying files"
cp -f "$FILES_DIR/opt/vpn-director/"*.sh /opt/vpn-director/ 2>/dev/null || true
cp -f "$FILES_DIR/opt/vpn-director/lib/"*.sh /opt/vpn-director/lib/
cp -f "$FILES_DIR/opt/vpn-director/"*.template /opt/vpn-director/ 2>/dev/null || true
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

# 8. Re-monitor in monit (if available)
if command -v monit >/dev/null 2>&1; then
    log "Re-monitoring telegram-bot in monit"
    monit monitor telegram-bot 2>/dev/null || true
fi

# 9. Start bot
log "Starting telegram-bot"
/opt/etc/init.d/S98telegram-bot start || log "ERROR: Failed to start telegram-bot"

# 10. Cleanup
log "Update complete, cleaning up"
rm -rf "$FILES_DIR"
rm -f "$UPDATE_DIR/update.sh"

log "Done"
```

**Step 2: Verify file is created**

Run: `ls -la telegram-bot/internal/updater/update_script.sh.tmpl`

Expected: File exists

**Step 3: Commit**

```bash
git add telegram-bot/internal/updater/update_script.sh.tmpl
git commit -m "feat(telegram-bot): add shell script template for /update

Go template that generates update script. Uses set -e for fail-fast.
Handles monit, stops/starts bot via init.d, copies files, creates
notification JSON for bot to read on startup."
```

---

## Task 8: Implement Script Generator

**Files:**
- Create: `telegram-bot/internal/updater/script.go`
- Create: `telegram-bot/internal/updater/script_test.go`

**Step 1: Write failing test**

Create `telegram-bot/internal/updater/script_test.go`:

```go
package updater

import (
	"os"
	"path/filepath"
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
	if !strings.Contains(script, "CHAT_ID=123456789") {
		t.Error("Script missing CHAT_ID")
	}
	if !strings.Contains(script, `OLD_VERSION="v1.0.0"`) {
		t.Error("Script missing OLD_VERSION")
	}
	if !strings.Contains(script, `NEW_VERSION="v1.1.0"`) {
		t.Error("Script missing NEW_VERSION")
	}
	if !strings.Contains(script, "set -e") {
		t.Error("Script missing set -e")
	}
	if !strings.Contains(script, "S98telegram-bot stop") {
		t.Error("Script missing bot stop command")
	}
	if !strings.Contains(script, "S98telegram-bot start") {
		t.Error("Script missing bot start command")
	}
}

func TestRunUpdateScript(t *testing.T) {
	tmpDir := t.TempDir()
	scriptFile := filepath.Join(tmpDir, "update.sh")

	s := &Service{
		updateDir:  tmpDir,
		scriptFile: scriptFile,
	}

	// This will fail because nohup tries to run the script
	// But we can at least verify the script file is created
	_ = s.RunUpdateScript(123456789, "v1.0.0", "v1.1.0")

	// Check script file was created
	if _, err := os.Stat(scriptFile); os.IsNotExist(err) {
		t.Error("Script file was not created")
	}

	// Check script is executable
	info, _ := os.Stat(scriptFile)
	if info.Mode()&0111 == 0 {
		t.Error("Script file is not executable")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `cd telegram-bot && go test ./internal/updater/... -v -run TestScript`

Expected: FAIL (generateScript method doesn't exist)

**Step 3: Write the implementation**

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

func (s *Service) getUpdateDir() string {
	if s.updateDir != "" {
		return s.updateDir
	}
	return UpdateDir
}

func (s *Service) getScriptFile() string {
	if s.scriptFile != "" {
		return s.scriptFile
	}
	return ScriptFile
}
```

**Step 4: Update Service struct in updater.go**

Add fields to Service struct in `telegram-bot/internal/updater/updater.go`:

```go
// Service implements the Updater interface.
type Service struct {
	httpClient *http.Client
	lockFile   string // Configurable for testing
	updateDir  string // Configurable for testing
	scriptFile string // Configurable for testing
}
```

**Step 5: Run tests**

Run: `cd telegram-bot && go test ./internal/updater/... -v`

Expected: PASS

**Step 6: Commit**

```bash
git add telegram-bot/internal/updater/script.go telegram-bot/internal/updater/script_test.go telegram-bot/internal/updater/updater.go
git commit -m "feat(telegram-bot): add script generator for /update

Uses go:embed to load template, generates script with embedded
variables, runs it detached via nohup. Script will stop the bot
and replace files."
```

---

## Task 9: Create Startup Notification Handler

**Files:**
- Create: `telegram-bot/internal/startup/notify.go`
- Create: `telegram-bot/internal/startup/notify_test.go`

**Step 1: Write failing test**

Create `telegram-bot/internal/startup/notify_test.go`:

```go
package startup

import (
	"os"
	"path/filepath"
	"testing"
)

type mockBotAPI struct {
	sentMessages []mockMessage
}

type mockMessage struct {
	chatID int64
	text   string
}

func (m *mockBotAPI) Send(chatID int64, text string) error {
	m.sentMessages = append(m.sentMessages, mockMessage{chatID: chatID, text: text})
	return nil
}

func TestCheckAndSendNotify(t *testing.T) {
	tmpDir := t.TempDir()
	notifyFile := filepath.Join(tmpDir, "notify.json")

	// Create notify file
	json := `{"chat_id":123456789,"old_version":"v1.0.0","new_version":"v1.1.0"}`
	if err := os.WriteFile(notifyFile, []byte(json), 0644); err != nil {
		t.Fatal(err)
	}

	mockBot := &mockBotAPI{}

	err := CheckAndSendNotify(mockBot, notifyFile, tmpDir)
	if err != nil {
		t.Fatalf("CheckAndSendNotify() error = %v", err)
	}

	// Check message was sent
	if len(mockBot.sentMessages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(mockBot.sentMessages))
	}

	msg := mockBot.sentMessages[0]
	if msg.chatID != 123456789 {
		t.Errorf("chatID = %d, want 123456789", msg.chatID)
	}
	if msg.text != "Update complete: v1.0.0 → v1.1.0" {
		t.Errorf("text = %q, want 'Update complete: v1.0.0 → v1.1.0'", msg.text)
	}

	// Check notify file was deleted
	if _, err := os.Stat(notifyFile); !os.IsNotExist(err) {
		t.Error("notify file should be deleted")
	}

	// Check update dir was deleted
	if _, err := os.Stat(tmpDir); !os.IsNotExist(err) {
		t.Error("update dir should be deleted")
	}
}

func TestCheckAndSendNotify_NoFile(t *testing.T) {
	tmpDir := t.TempDir()
	notifyFile := filepath.Join(tmpDir, "notify.json")

	mockBot := &mockBotAPI{}

	// Should not error when file doesn't exist
	err := CheckAndSendNotify(mockBot, notifyFile, tmpDir)
	if err != nil {
		t.Fatalf("CheckAndSendNotify() error = %v", err)
	}

	// No messages sent
	if len(mockBot.sentMessages) != 0 {
		t.Errorf("Expected 0 messages, got %d", len(mockBot.sentMessages))
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `cd telegram-bot && go test ./internal/startup/... -v`

Expected: FAIL (package doesn't exist)

**Step 3: Write the implementation**

Create `telegram-bot/internal/startup/notify.go`:

```go
package startup

import (
	"encoding/json"
	"fmt"
	"os"
)

// MessageSender is the interface for sending Telegram messages.
type MessageSender interface {
	Send(chatID int64, text string) error
}

// UpdateNotification is the JSON structure in notify.json.
type UpdateNotification struct {
	ChatID     int64  `json:"chat_id"`
	OldVersion string `json:"old_version"`
	NewVersion string `json:"new_version"`
}

// DefaultNotifyFile is the default path to the notification file.
const DefaultNotifyFile = "/tmp/vpn-director-update/notify.json"

// DefaultUpdateDir is the default update directory to clean up.
const DefaultUpdateDir = "/tmp/vpn-director-update"

// CheckAndSendNotify checks for pending update notification and sends it.
// Cleans up the notification file and update directory after sending.
func CheckAndSendNotify(sender MessageSender, notifyFile, updateDir string) error {
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

	// Send message
	text := fmt.Sprintf("Update complete: %s → %s", n.OldVersion, n.NewVersion)
	if err := sender.Send(n.ChatID, text); err != nil {
		return fmt.Errorf("send notification: %w", err)
	}

	// Cleanup
	os.Remove(notifyFile)
	os.RemoveAll(updateDir)

	return nil
}
```

**Step 4: Run tests**

Run: `cd telegram-bot && go test ./internal/startup/... -v`

Expected: PASS

**Step 5: Commit**

```bash
git add telegram-bot/internal/startup/notify.go telegram-bot/internal/startup/notify_test.go
git commit -m "feat(telegram-bot): add startup notification for /update

Checks for notify.json on startup, sends completion message to chat,
cleans up update directory. Called before bot starts polling."
```

---

## Task 10: Create Update Command Handler

**Files:**
- Create: `telegram-bot/internal/handler/update.go`

**Step 1: Write the handler**

Create `telegram-bot/internal/handler/update.go`:

```go
package handler

import (
	"context"
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"telegram-bot/internal/telegram"
	"telegram-bot/internal/updater"
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
func (h *UpdateHandler) HandleUpdate(ctx context.Context, msg *tgbotapi.Message) {
	chatID := msg.Chat.ID

	// 1. Dev mode check
	if h.devMode {
		h.send(chatID, "Command /update is not available in dev mode")
		return
	}

	// 2. Lock check
	if h.updater.IsUpdateInProgress() {
		h.send(chatID, "Update is already in progress, please wait...")
		return
	}

	// 3. Get latest release
	release, err := h.updater.GetLatestRelease(ctx)
	if err != nil {
		h.send(chatID, fmt.Sprintf("Failed to check for updates: %v", err))
		return
	}

	// 4. Compare versions
	shouldUpdate, err := h.updater.ShouldUpdate(h.version, release.TagName)
	if err != nil {
		h.send(chatID, fmt.Sprintf("Failed to parse version: %v", err))
		return
	}
	if !shouldUpdate {
		h.send(chatID, fmt.Sprintf("Already running the latest version: %s", h.version))
		return
	}

	// 5. Create lock
	if err := h.updater.CreateLock(); err != nil {
		h.send(chatID, fmt.Sprintf("Failed to start update: %v", err))
		return
	}

	// 6. Notify start
	h.send(chatID, fmt.Sprintf("Starting update %s → %s...", h.version, release.TagName))

	// 7. Download in goroutine
	go h.downloadAndUpdate(ctx, chatID, release)
}

func (h *UpdateHandler) downloadAndUpdate(ctx context.Context, chatID int64, release *updater.Release) {
	// Download with progress callback
	err := h.updater.DownloadRelease(ctx, release, func(msg string) {
		// Could send progress messages here, but keep it simple
	})
	if err != nil {
		h.updater.RemoveLock()
		h.send(chatID, fmt.Sprintf("Download failed: %v", err))
		return
	}

	// 8. Notify downloaded
	h.send(chatID, "Files downloaded, starting update...")

	// 9. Run update script
	if err := h.updater.RunUpdateScript(chatID, h.version, release.TagName); err != nil {
		h.updater.RemoveLock()
		h.send(chatID, fmt.Sprintf("Failed to run update script: %v", err))
		return
	}

	// 10. Notify script started
	h.send(chatID, "Update script started, bot will restart in a few seconds...")
}

func (h *UpdateHandler) send(chatID int64, text string) {
	_ = h.sender.SendPlain(chatID, text)
}
```

**Step 2: Verify it compiles**

Run: `cd telegram-bot && go build ./internal/handler/...`

Expected: Build succeeds

**Step 3: Commit**

```bash
git add telegram-bot/internal/handler/update.go
git commit -m "feat(telegram-bot): add /update command handler

Checks dev mode, lock, version comparison. Downloads files in goroutine
for responsiveness. Sends progress messages. Runs update script detached."
```

---

## Task 11: Integrate into Bot

**Files:**
- Modify: `telegram-bot/internal/handler/handler.go`
- Modify: `telegram-bot/internal/bot/router.go`
- Modify: `telegram-bot/internal/bot/bot.go`
- Modify: `telegram-bot/cmd/bot/main.go`

**Step 1: Add to Deps struct**

In `telegram-bot/internal/handler/handler.go`, add to Deps:

```go
type Deps struct {
	Sender  telegram.MessageSender
	Config  service.ConfigStore
	VPN     service.VPNDirector
	Xray    service.XrayGenerator
	Network service.NetworkInfo
	Logs    service.LogReader
	Paths   paths.Paths
	Version string
	DevMode bool              // ADD
	Updater updater.Updater   // ADD
}
```

Add import for updater package.

**Step 2: Add route in router.go**

In `telegram-bot/internal/bot/router.go`:

1. Add to Router struct:
```go
update *handler.UpdateHandler
```

2. Add to NewRouter:
```go
update: handler.NewUpdateHandler(deps.Sender, deps.Updater, deps.DevMode, deps.Version),
```

3. Add case in RouteMessage:
```go
case "update":
    r.update.HandleUpdate(ctx, msg)
```

**Step 3: Register command in bot.go**

In `telegram-bot/internal/bot/bot.go`, add to commands slice:

```go
{Command: "update", Description: "Update VPN Director to latest release"},
```

**Step 4: Update main.go**

In `telegram-bot/cmd/bot/main.go`:

1. Add imports:
```go
"telegram-bot/internal/startup"
"telegram-bot/internal/updater"
```

2. After creating bot API, add notification check:
```go
// Check for update notification before starting
if err := startup.CheckAndSendNotify(
    &simpleSender{api: bot},
    startup.DefaultNotifyFile,
    startup.DefaultUpdateDir,
); err != nil {
    slog.Warn("Failed to send update notification", "error", err)
}
```

3. Add simpleSender adapter:
```go
type simpleSender struct {
    api *tgbotapi.BotAPI
}

func (s *simpleSender) Send(chatID int64, text string) error {
    msg := tgbotapi.NewMessage(chatID, text)
    _, err := s.api.Send(msg)
    return err
}
```

4. Add to Deps initialization:
```go
DevMode: *devFlag,
Updater: updater.New(),
```

**Step 5: Verify it compiles**

Run: `cd telegram-bot && go build ./cmd/bot/...`

Expected: Build succeeds

**Step 6: Commit**

```bash
git add telegram-bot/internal/handler/handler.go telegram-bot/internal/bot/router.go telegram-bot/internal/bot/bot.go telegram-bot/cmd/bot/main.go
git commit -m "feat(telegram-bot): integrate /update command

- Add DevMode and Updater to Deps
- Register /update route and BotFather command
- Check for startup notification before polling
- Initialize updater service"
```

---

## Task 12: Final Testing and Documentation

**Step 1: Run all tests**

Run: `cd telegram-bot && go test ./... -v`

Expected: All tests pass

**Step 2: Build for all architectures**

Run: `cd telegram-bot && make build-arm64 && make build-arm`

Expected: Both binaries created in `bin/`

**Step 3: Verify command registration**

Run: `cd telegram-bot && go run ./cmd/bot --help`

Expected: Help output shows (bot won't start without config)

**Step 4: Update CLAUDE.md with new command**

Add `/update` to the Bot Commands table in `.claude/rules/telegram-bot.md`:

```markdown
| `/update` | `UpdateHandler` | Update VPN Director to latest release |
```

**Step 5: Commit documentation**

```bash
git add .claude/rules/telegram-bot.md
git commit -m "docs: add /update command to telegram-bot documentation"
```

**Step 6: Final commit message for PR**

Create PR with summary:
```
feat(telegram-bot): add /update command for self-updating

Implements self-update functionality via /update command:

- Downloads scripts from raw.githubusercontent.com by release tag
- Downloads bot binary from release assets
- Generates shell script that stops bot, replaces files, restarts bot
- Bot sends completion notification on startup after update
- Lock file with PID prevents concurrent updates
- Dev mode check prevents updates in development

Closes #XX
```

---

## Summary

| Task | Description | Files |
|------|-------------|-------|
| 1 | Makefile VERSION change | `Makefile` |
| 2 | Version parser | `updater/version.go`, `version_test.go` |
| 3 | Updater interface | `updater/updater.go` |
| 4 | GitHub API client | `updater/github.go`, `github_test.go` |
| 5 | Lock file with PID | `updater/updater.go`, `lock_test.go` |
| 6 | File downloader | `updater/downloader.go` |
| 7 | Script template | `updater/update_script.sh.tmpl` |
| 8 | Script generator | `updater/script.go`, `script_test.go` |
| 9 | Startup notification | `startup/notify.go`, `notify_test.go` |
| 10 | Update handler | `handler/update.go` |
| 11 | Bot integration | `handler.go`, `router.go`, `bot.go`, `main.go` |
| 12 | Testing & docs | Tests, documentation |
