// Package updateflow owns the self-update orchestration shared by the
// Telegram bot and the Web UI: one cached check against the GitHub release
// API and one guarded start of the download plus the update script.
package updateflow

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/updater"
)

// Cache windows for Check. GitHub allows 60 unauthenticated requests an hour
// and the Web UI asks on every page load, so an uncached check is a rate
// limit waiting to happen.
const (
	cacheTTL      = 30 * time.Minute
	forceCooldown = time.Minute
)

// Errors a caller is expected to branch on. Everything else (a lock that
// cannot be created, a version string the shell would choke on) comes back
// as a plain error.
var (
	ErrDevMode    = errors.New("update is not available in dev mode")
	ErrDevVersion = errors.New("cannot check updates for a dev build")
	ErrInProgress = errors.New("update is already in progress")
	ErrUpToDate   = errors.New("already running the latest version")
)

// GitHubError marks a failure to reach the release API, so a caller can tell
// "GitHub is down" (502, retry later) from "this build cannot update".
type GitHubError struct{ Err error }

func (e *GitHubError) Error() string { return "check for updates: " + e.Err.Error() }
func (e *GitHubError) Unwrap() error { return e.Err }

// CheckResult describes the latest release relative to the running build.
type CheckResult struct {
	Current         string    `json:"current"`
	Latest          string    `json:"latest"`
	UpdateAvailable bool      `json:"update_available"`
	Changelog       string    `json:"changelog"`
	CheckedAt       time.Time `json:"checked_at"`
}

// Flow orchestrates update checks and runs. One instance per daemon.
type Flow struct {
	upd            updater.Updater
	currentVersion string
	devMode        bool

	mu            sync.Mutex
	cached        CheckResult
	cachedRelease *updater.Release
	cachedAt      time.Time
	lastForce     time.Time
}

// New creates a Flow for the given updater and running version.
func New(upd updater.Updater, currentVersion string, devMode bool) *Flow {
	return &Flow{upd: upd, currentVersion: currentVersion, devMode: devMode}
}

// InProgress reports whether an update script is running, by the lock file.
func (f *Flow) InProgress() bool { return f.upd.IsUpdateInProgress() }

// Check returns the cached result when it is younger than cacheTTL. force
// bypasses the cache, but not more often than once every forceCooldown; a
// throttled force is served from the cache rather than rejected.
func (f *Flow) Check(ctx context.Context, force bool) (CheckResult, error) {
	if f.devMode {
		return CheckResult{}, ErrDevMode
	}
	if f.currentVersion == "dev" {
		return CheckResult{}, ErrDevVersion
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	now := time.Now()
	if !f.cachedAt.IsZero() {
		if force && now.Sub(f.lastForce) < forceCooldown {
			return f.cached, nil
		}
		if !force && now.Sub(f.cachedAt) < cacheTTL {
			return f.cached, nil
		}
	}
	if force {
		f.lastForce = now
	}

	release, err := f.upd.GetLatestRelease(ctx)
	if err != nil {
		return CheckResult{}, &GitHubError{Err: err}
	}

	available, err := f.upd.ShouldUpdate(f.currentVersion, release.TagName)
	if err != nil {
		return CheckResult{}, fmt.Errorf("compare versions: %w", err)
	}

	res := CheckResult{
		Current:         f.currentVersion,
		Latest:          release.TagName,
		UpdateAvailable: available,
		Changelog:       release.Body,
		CheckedAt:       now,
	}
	f.cached, f.cachedRelease, f.cachedAt = res, release, now
	return res, nil
}

// latestRelease returns the release behind the last successful Check.
func (f *Flow) latestRelease() *updater.Release {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cachedRelease
}
