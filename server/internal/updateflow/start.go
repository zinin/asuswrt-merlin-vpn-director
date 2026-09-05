package updateflow

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/updater"
)

// StartResult names the versions of a run, filled in even when Start refuses
// with ErrUpToDate so the caller can put them in its message.
type StartResult struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// Start runs the pre-flight checks synchronously, takes the lock and launches
// the download plus the update script in a goroutine; progress receives
// human-readable status lines. It returns as soon as the background work is
// under way, because the script kills this very process a few seconds later.
func (f *Flow) Start(ctx context.Context, initiator string, chatID int64, progress func(string)) (StartResult, error) {
	if f.devMode {
		return StartResult{}, ErrDevMode
	}
	if f.currentVersion == "dev" {
		return StartResult{}, ErrDevVersion
	}
	// Rejected before the lock: an unknown initiator would only fail later,
	// inside RunUpdateScript, with the lock already taken.
	if initiator != "bot" && initiator != "webui" {
		return StartResult{}, fmt.Errorf("invalid initiator: %q", initiator)
	}
	if f.upd.IsUpdateInProgress() {
		return StartResult{}, ErrInProgress
	}

	check, err := f.Check(ctx, false)
	if err != nil {
		return StartResult{}, err
	}

	res := StartResult{From: check.Current, To: check.Latest}
	if !check.UpdateAvailable {
		return res, ErrUpToDate
	}

	// Both strings end up inside the generated shell script.
	if !updater.IsValidVersion(check.Current) {
		return res, fmt.Errorf("invalid current version: %s", check.Current)
	}
	if !updater.IsValidVersion(check.Latest) {
		return res, fmt.Errorf("invalid release version: %s", check.Latest)
	}

	release := f.latestRelease()
	if release == nil {
		return res, fmt.Errorf("no release information available")
	}

	if err := f.upd.CreateLock(); err != nil {
		return res, err
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("update goroutine panicked", "error", r)
				// Unwind before reporting: progress is caller-supplied, and a
				// panic in it must not leave the lock behind.
				f.upd.CleanFiles()
				f.upd.RemoveLock()
				progress("Update failed unexpectedly. Check logs.")
			}
		}()
		f.run(release, res, initiator, chatID, progress)
	}()

	return res, nil
}

// run downloads the release and hands over to the update script. It builds
// its own context: a closed browser tab or a finished Telegram poll must not
// abort an update that is already downloading.
func (f *Flow) run(release *updater.Release, res StartResult, initiator string, chatID int64, progress func(string)) {
	if err := f.upd.DownloadRelease(context.Background(), release); err != nil {
		f.upd.CleanFiles()
		f.upd.RemoveLock()
		progress(fmt.Sprintf("Download failed: %v", err))
		return
	}
	progress("Files downloaded, starting update...")

	err := f.upd.RunUpdateScript(updater.RunOptions{
		OldVersion: res.From,
		NewVersion: res.To,
		ChatID:     chatID,
		Initiator:  initiator,
	})
	if err != nil {
		f.upd.CleanFiles()
		f.upd.RemoveLock()
		progress(fmt.Sprintf("Failed to run update script: %v", err))
		return
	}

	progress("Update script started, the service will restart in a few seconds...")
}
