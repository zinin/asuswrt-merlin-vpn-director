package updateflow

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/updater"
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

// waitFor polls cond for up to a second: Start hands the download to a
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

func TestStart_RunsTheScriptWithTheInitiator(t *testing.T) {
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

	waitFor(t, "the update script to start", func() bool {
		upd.mu.Lock()
		defer upd.mu.Unlock()
		return upd.runScriptCalled
	})

	upd.mu.Lock()
	opts := upd.runScriptOpts
	upd.mu.Unlock()
	if opts.Initiator != "webui" || opts.ChatID != 0 {
		t.Errorf("RunOptions = %+v, want initiator webui and chat 0", opts)
	}
	if opts.OldVersion != "v1.2.0" || opts.NewVersion != "v1.3.0" {
		t.Errorf("RunOptions versions = %+v", opts)
	}

	// The download line is emitted before RunUpdateScript is called, so the
	// wait above already puts it in the collector.
	got := strings.Join(lines(), "\n")
	if !strings.Contains(got, "Files downloaded, starting update...") {
		t.Errorf("progress missing the download line: %q", got)
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
	if upd.runScriptCalled {
		t.Error("an up-to-date router must not run the script")
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
			// "Touching anything" is all three: GitHub, the lock, the script.
			if n := upd.calls(); n != 0 {
				t.Errorf("a rejected start made %d GitHub requests, want 0", n)
			}
			upd.mu.Lock()
			defer upd.mu.Unlock()
			if upd.createLockCalled {
				t.Error("rejected start must not take the lock")
			}
			if upd.runScriptCalled {
				t.Error("rejected start must not run the script")
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
	if upd.runScriptCalled {
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
	// The first progress line is emitted before RunUpdateScript, so the script
	// runs at all only if the panic was absorbed — that makes runScriptCalled
	// a positive anchor rather than a wait on the absence of something.
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

			waitFor(t, "the update script to run despite the callback", func() bool {
				upd.mu.Lock()
				defer upd.mu.Unlock()
				return upd.runScriptCalled
			})
			upd.mu.Lock()
			defer upd.mu.Unlock()
			if upd.cleanFilesCalled || upd.removeLockCalled {
				t.Error("a run already handed to the script must not be unwound by a broken callback")
			}
		})
	}
}

func TestStart_DownloadOutlivesTheCallersContext(t *testing.T) {
	// The whole point of the goroutine: a closed browser tab or a finished
	// Telegram poll cancels the request context, and the download must carry
	// on regardless.
	upd := newMockUpdater()
	upd.downloadGate = make(chan struct{})
	f := New(upd, "v1.2.0", false)
	progress, _ := collectProgress()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if _, err := f.Start(ctx, "webui", 0, progress); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	// The gate holds the download until the caller is gone, so a download
	// that had inherited ctx would see it already cancelled.
	cancel()
	close(upd.downloadGate)

	waitFor(t, "the update to continue past the cancelled request", func() bool {
		upd.mu.Lock()
		defer upd.mu.Unlock()
		return upd.runScriptCalled
	})
	upd.mu.Lock()
	defer upd.mu.Unlock()
	if upd.downloadCtxDone {
		t.Error("run() must not download with the caller's context")
	}
}

func TestStart_DownloadFailureCleansUp(t *testing.T) {
	// A half-downloaded release plus a stale lock would block every later
	// attempt, so the goroutine has to unwind both.
	upd := newMockUpdater()
	upd.downloadErr = errors.New("HTTP 404")
	f := New(upd, "v1.2.0", false)
	progress, lines := collectProgress()

	if _, err := f.Start(context.Background(), "bot", 42, progress); err != nil {
		t.Fatalf("Start() error = %v: the download failure is reported through progress", err)
	}

	// Both halves of the failure path are in the condition, so the wait does
	// not depend on the order run() does them in.
	waitFor(t, "cleanup and the report of a failed download", func() bool {
		upd.mu.Lock()
		cleaned := upd.cleanFilesCalled && upd.removeLockCalled
		upd.mu.Unlock()
		return cleaned && strings.Contains(strings.Join(lines(), "\n"), "Download failed: HTTP 404")
	})
	upd.mu.Lock()
	defer upd.mu.Unlock()
	if upd.runScriptCalled {
		t.Error("a failed download must not run the script")
	}
}

func TestStart_ScriptFailureCleansUp(t *testing.T) {
	upd := newMockUpdater()
	upd.runScriptErr = errors.New("start script: permission denied")
	f := New(upd, "v1.2.0", false)
	progress, lines := collectProgress()

	if _, err := f.Start(context.Background(), "bot", 42, progress); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	waitFor(t, "cleanup and the report of a failed script", func() bool {
		upd.mu.Lock()
		cleaned := upd.cleanFilesCalled && upd.removeLockCalled
		upd.mu.Unlock()
		return cleaned && strings.Contains(strings.Join(lines(), "\n"), "Failed to run update script")
	})
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
	if upd.runScriptCalled {
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
	if upd.runScriptCalled {
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
