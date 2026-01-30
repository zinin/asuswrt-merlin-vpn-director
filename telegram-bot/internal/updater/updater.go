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
	UpdateDir  = "/tmp/vpn-director-update"
	FilesDir   = UpdateDir + "/files"
	LockFile   = UpdateDir + "/lock"
	NotifyFile = UpdateDir + "/notify.json"
	ScriptFile = UpdateDir + "/update.sh"
)

// Release represents a GitHub release.
type Release struct {
	TagName string
	Assets  []Asset
}

// Asset represents a downloadable file in a release.
type Asset struct {
	Name        string
	DownloadURL string
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
	RunUpdateScript(chatID int64, oldVersion, newVersion string) error
}

// Service implements the Updater interface.
type Service struct {
	httpClient *http.Client
	baseURL    string // Injectable for testing, empty = default GitHub API
	lockFile   string // Configurable for testing
	updateDir  string // Configurable for testing
	scriptFile string // Configurable for testing
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
	return filepath.Join(s.getUpdateDir(), "files")
}

// getScriptFile returns the script file path.
func (s *Service) getScriptFile() string {
	if s.scriptFile != "" {
		return s.scriptFile
	}
	return ScriptFile
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
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create lock directory: %w", err)
	}

	// Atomic lock creation - fails if file already exists
	f, err := os.OpenFile(lockFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("lock file already exists (update in progress)")
		}
		return fmt.Errorf("failed to create lock file: %w", err)
	}
	defer f.Close()

	pid := os.Getpid()
	if _, err := f.WriteString(strconv.Itoa(pid)); err != nil {
		os.Remove(lockFile) // Clean up on write failure
		return fmt.Errorf("failed to write PID to lock file: %w", err)
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
