package updater

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// Update directory paths.
const (
	UpdateDir = "/tmp/vpn-director-update"
	// FilesDirName is where a download lands inside the update directory. It
	// has a name of its own because the directory is injectable in tests and
	// because the bot's startup notifier clears this subdirectory once a
	// failed update has been accounted for.
	FilesDirName = "files"
	FilesDir     = UpdateDir + "/" + FilesDirName
	LockFile     = UpdateDir + "/lock"
	NotifyFile   = UpdateDir + "/notify.json"
	ScriptFile   = UpdateDir + "/update.sh"
)

// ErrLockExists is returned by CreateLock when the lock file is already there.
// Start maps it to updateflow.ErrInProgress so a second caller gets 409, not 500.
var ErrLockExists = errors.New("lock file already exists (update in progress)")

// Release represents a GitHub release.
type Release struct {
	TagName string
	Body    string // Release notes / changelog
	Assets  []Asset
}

// Asset represents a downloadable file in a release.
type Asset struct {
	Name        string
	DownloadURL string
}

// Daemon describes an updatable daemon. Name is the release asset prefix
// (<Name>-<arch>), the file name under files/ and the monit service name the
// update script unmonitors and re-monitors; Binary is where the update script
// installs it and what `pgrep -f` matches on; InitScript is the Entware script
// that starts and stops it.
type Daemon struct {
	Name       string
	Binary     string
	InitScript string
}

// Daemons lists every daemon a release ships. DownloadRelease fetches one
// binary per entry and the update script restarts the entries that were
// running before the update. This table is the single source of truth: the
// downloader, the script template and install.sh must not drift apart.
var Daemons = []Daemon{
	{Name: "telegram-bot", Binary: "/opt/vpn-director/telegram-bot", InitScript: "S98telegram-bot"},
	{Name: "webui", Binary: "/opt/vpn-director/webui", InitScript: "S98vpn-director-webui"},
}

// Updater defines the interface for update operations.
type Updater interface {
	// GetLatestRelease fetches the latest release info from GitHub.
	GetLatestRelease(ctx context.Context) (*Release, error)

	// ShouldUpdate checks if currentVersion is older than latestTag.
	// Returns an error if either version can't be parsed (dev handled by caller).
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
	RunUpdateScript(opts RunOptions) error
}

// Service implements the Updater interface.
type Service struct {
	httpClient *http.Client
	baseURL    string // Injectable for testing, empty = default GitHub API
	rawBaseURL string // Injectable for testing, empty = raw.githubusercontent.com
	lockFile   string // Configurable for testing
	updateDir  string // Configurable for testing
	scriptFile string // Configurable for testing
	archSuffix string // Injectable for testing, empty = derived from runtime.GOARCH
	shell      string // Injectable for testing, empty = /bin/sh
}

// Verify Service implements Updater interface.
var _ Updater = (*Service)(nil)

// New creates a new Service with default http.Client.
// No global timeout is set - per-request timeouts are used instead.
func New() *Service {
	return &Service{
		httpClient: &http.Client{},
	}
}

// NewWithBaseURL creates a new Service with a custom base URL for testing.
func NewWithBaseURL(baseURL string) *Service {
	return &Service{
		httpClient: &http.Client{},
		baseURL:    baseURL,
	}
}

// getLockFile returns the lock file path.
func (s *Service) getLockFile() string {
	if s.lockFile != "" {
		return s.lockFile
	}
	return LockFile
}

// getUpdateDir returns the update directory path.
func (s *Service) getUpdateDir() string {
	if s.updateDir != "" {
		return s.updateDir
	}
	return UpdateDir
}

// getFilesDir returns the files directory path.
func (s *Service) getFilesDir() string {
	return filepath.Join(s.getUpdateDir(), FilesDirName)
}

// getScriptFile returns the script file path.
func (s *Service) getScriptFile() string {
	if s.scriptFile != "" {
		return s.scriptFile
	}
	return ScriptFile
}

// getShell returns the interpreter that runs the update script. Tests point
// it at a path that does not exist, so the unit suite never launches the real
// script against the machine running go test.
func (s *Service) getShell() string {
	if s.shell != "" {
		return s.shell
	}
	return "/bin/sh"
}

// IsUpdateInProgress checks if a lock file exists and the process is still alive.
// If the process is dead, the stale lock is removed and false is returned.
func (s *Service) IsUpdateInProgress() bool {
	lockFile := s.getLockFile()

	data, err := os.ReadFile(lockFile)
	if err != nil {
		// Lock file doesn't exist or can't be read
		return false
	}

	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		// Invalid PID in lock file, remove stale lock
		os.Remove(lockFile)
		return false
	}

	// Check if process is alive using signal 0
	err = syscall.Kill(pid, 0)
	if err != nil {
		// EPERM means process exists but we don't have permission to signal it
		// This still means the process is alive
		if errors.Is(err, syscall.EPERM) {
			return true
		}
		// Process is dead, remove stale lock
		os.Remove(lockFile)
		return false
	}

	return true
}

// CreateLock creates a lock file with the current process PID.
// Uses O_CREATE|O_EXCL for atomic creation - fails if lock already exists.
func (s *Service) CreateLock() error {
	lockFile := s.getLockFile()

	// Ensure directory exists
	dir := filepath.Dir(lockFile)
	// 0755 root-owned: the update directory holds a script this process then
	// runs as root. On Asuswrt-Merlin there is no unprivileged local user to
	// defend against, which is why this is a comment and not a mechanism.
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create lock directory: %w", err)
	}

	// Publish the lock with its PID already in it. O_CREATE|O_EXCL alone
	// leaves the file empty between the open and the write, and a status check
	// landing there parses no PID, judges the lock garbage and deletes it -
	// CreateLock would then report success on a lock that no longer exists,
	// and the next Start would begin a second update that wipes the shared
	// files/ directory under this one. Link fails when the target exists, so
	// the exclusivity O_EXCL gave us is kept.
	tmp, err := os.CreateTemp(dir, "lock.*")
	if err != nil {
		return fmt.Errorf("failed to create lock file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the link is in place and this returns

	if _, err := tmp.WriteString(strconv.Itoa(os.Getpid())); err != nil {
		tmp.Close()
		return fmt.Errorf("failed to write PID to lock file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to write PID to lock file: %w", err)
	}
	// CreateTemp gives 0600; the lock has always been world-readable.
	if err := os.Chmod(tmpName, 0644); err != nil {
		return fmt.Errorf("failed to create lock file: %w", err)
	}

	if err := os.Link(tmpName, lockFile); err != nil {
		if errors.Is(err, os.ErrExist) {
			return ErrLockExists
		}
		return fmt.Errorf("failed to create lock file: %w", err)
	}

	return nil
}

// RemoveLock removes the lock file. Errors are ignored.
func (s *Service) RemoveLock() {
	os.Remove(s.getLockFile())
}

// CleanFiles removes the files/ directory. Errors are ignored.
func (s *Service) CleanFiles() {
	os.RemoveAll(s.getFilesDir())
}

// RunUpdateScript is implemented in script.go
