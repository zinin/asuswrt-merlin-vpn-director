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
		// Process is dead, remove stale lock
		os.Remove(lockFile)
		return false
	}

	return true
}

// CreateLock creates a lock file with the current process PID.
func (s *Service) CreateLock() error {
	lockFile := s.getLockFile()

	// Ensure directory exists
	dir := filepath.Dir(lockFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create lock directory: %w", err)
	}

	pid := os.Getpid()
	if err := os.WriteFile(lockFile, []byte(strconv.Itoa(pid)), 0644); err != nil {
		return fmt.Errorf("failed to write lock file: %w", err)
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

// RunUpdateScript generates and runs the update shell script.
// This is a stub - implementation will be in script.go.
func (s *Service) RunUpdateScript(chatID int64, oldVersion, newVersion string) error {
	return fmt.Errorf("not implemented")
}
