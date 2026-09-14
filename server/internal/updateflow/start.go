package updateflow

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/zinin/vpn-director/server/internal/updater"
)

// StartResult names the versions of a run, filled in even when Start refuses
// with ErrUpToDate so the caller can put them in its message.
type StartResult struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// report delivers one progress line. progress is caller-supplied — a Telegram
// send on a nil client, a write to a closed channel — so a panic from it has
// to die right here: reaching the goroutine's recover would call CleanFiles
// and delete files/ from under an update script that is already running.
func report(progress func(string), line string) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("update progress callback panicked", "error", r, "line", line)
		}
	}()
	progress(line)
}

// Start runs the pre-flight checks synchronously, takes the lock and hands the
// update over to the new release's binary in a goroutine; progress receives
// human-readable status lines. It returns as soon as the background work is
// under way, because the update script kills this very process a few seconds
// later.
func (f *Flow) Start(ctx context.Context, initiator string, chatID int64, progress func(string)) (StartResult, error) {
	if f.devMode {
		return StartResult{}, ErrDevMode
	}
	if f.currentVersion == "dev" {
		return StartResult{}, ErrDevVersion
	}
	// Rejected before the lock: an unknown initiator would only fail later,
	// inside the new version's self-update step, with the lock already taken.
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
	// Check and latestRelease read the cache under separate acquisitions of
	// f.mu, and Check holds that mutex across the network call: a check that
	// was queued behind it can replace the release in between. Refuse rather
	// than install one release under another version's name; the next attempt
	// sees a pair that agrees. Still before CreateLock, so nothing to unwind.
	if release.TagName != check.Latest {
		return res, fmt.Errorf("release changed while starting: have %q, want %q, please retry", release.TagName, check.Latest)
	}

	if err := f.upd.CreateLock(); err != nil {
		if errors.Is(err, updater.ErrLockExists) {
			return res, ErrInProgress
		}
		return res, err
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("update goroutine panicked", "error", r)
				// Unwind before reporting: report absorbs callback panics, so
				// anything arriving here is a failure of our own, and only one
				// from before the handover succeeded can reach it - past that
				// point run makes a single report call and returns. The lock
				// and files/ are still this process's then; once step 2 has
				// exited 0 they belong to the update script, and unwinding
				// would pull the payload from under it.
				f.upd.CleanFiles()
				f.upd.RemoveLock()
				report(progress, "Update failed unexpectedly. Check logs.")
			}
		}()
		f.run(release, res, initiator, chatID, progress)
	}()

	return res, nil
}

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
		// progress may reach nobody - a chat message that cannot be
		// delivered, a browser tab that is gone - and a failed update that
		// left no trace at all is one nobody can look into.
		slog.Warn("self-update handover failed", "from", res.From, "to", res.To, "error", err)
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
