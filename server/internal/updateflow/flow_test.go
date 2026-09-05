package updateflow

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/updater"
)

// mockUpdater implements updater.Updater for testing.
type mockUpdater struct {
	mu sync.Mutex

	release    *updater.Release
	releaseErr error
	releases   int // GetLatestRelease call count

	shouldUpdate    bool
	shouldUpdateErr error

	inProgress    bool
	createLockErr error
	downloadErr   error
	runScriptErr  error

	createLockCalled bool
	cleanFilesCalled bool
	removeLockCalled bool
	runScriptCalled  bool
	runScriptOpts    updater.RunOptions
}

func newMockUpdater() *mockUpdater {
	return &mockUpdater{
		release:      &updater.Release{TagName: "v1.3.0", Body: "changelog text"},
		shouldUpdate: true,
	}
}

func (m *mockUpdater) GetLatestRelease(_ context.Context) (*updater.Release, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.releases++
	if m.releaseErr != nil {
		return nil, m.releaseErr
	}
	return m.release, nil
}

func (m *mockUpdater) ShouldUpdate(_, _ string) (bool, error) {
	return m.shouldUpdate, m.shouldUpdateErr
}

func (m *mockUpdater) IsUpdateInProgress() bool { return m.inProgress }

func (m *mockUpdater) CreateLock() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createLockCalled = true
	return m.createLockErr
}

func (m *mockUpdater) RemoveLock() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeLockCalled = true
}

func (m *mockUpdater) CleanFiles() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanFilesCalled = true
}

func (m *mockUpdater) DownloadRelease(_ context.Context, _ *updater.Release) error {
	return m.downloadErr
}

func (m *mockUpdater) RunUpdateScript(opts updater.RunOptions) error {
	m.mu.Lock()
	m.runScriptCalled = true
	m.runScriptOpts = opts
	err := m.runScriptErr
	m.mu.Unlock()
	return err
}

func (m *mockUpdater) calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.releases
}

func TestCheck_ReturnsRelease(t *testing.T) {
	upd := newMockUpdater()
	f := New(upd, "v1.2.0", false)

	res, err := f.Check(context.Background(), false)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if res.Current != "v1.2.0" || res.Latest != "v1.3.0" {
		t.Errorf("Check() = %+v, want current v1.2.0 latest v1.3.0", res)
	}
	if !res.UpdateAvailable {
		t.Error("UpdateAvailable must follow ShouldUpdate")
	}
	if res.Changelog != "changelog text" {
		t.Errorf("Changelog = %q", res.Changelog)
	}
	if res.CheckedAt.IsZero() {
		t.Error("CheckedAt must be stamped")
	}
}

func TestCheck_UsesCacheWithinTTL(t *testing.T) {
	// GitHub allows 60 unauthenticated requests an hour; every SPA load calls
	// this endpoint for the header banner.
	upd := newMockUpdater()
	f := New(upd, "v1.2.0", false)

	for i := 0; i < 5; i++ {
		if _, err := f.Check(context.Background(), false); err != nil {
			t.Fatalf("Check() error = %v", err)
		}
	}
	if upd.calls() != 1 {
		t.Errorf("GetLatestRelease called %d times, want 1", upd.calls())
	}
}

func TestCheck_ForceBypassesCacheOncePerMinute(t *testing.T) {
	upd := newMockUpdater()
	f := New(upd, "v1.2.0", false)

	if _, err := f.Check(context.Background(), true); err != nil {
		t.Fatalf("first Check() error = %v", err)
	}
	if _, err := f.Check(context.Background(), true); err != nil {
		t.Fatalf("second Check() error = %v", err)
	}
	if upd.calls() != 1 {
		t.Errorf("a second force within a minute must be served from cache, got %d calls", upd.calls())
	}

	// Move the throttle window into the past; the cache is still fresh, so
	// only force may pierce it.
	f.mu.Lock()
	f.lastForce = time.Now().Add(-2 * time.Minute)
	f.mu.Unlock()

	if _, err := f.Check(context.Background(), true); err != nil {
		t.Fatalf("third Check() error = %v", err)
	}
	if upd.calls() != 2 {
		t.Errorf("force after the cooldown must hit GitHub, got %d calls", upd.calls())
	}
}

func TestCheck_RefreshesAfterTTL(t *testing.T) {
	upd := newMockUpdater()
	f := New(upd, "v1.2.0", false)

	if _, err := f.Check(context.Background(), false); err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	f.mu.Lock()
	f.cachedAt = time.Now().Add(-31 * time.Minute)
	f.mu.Unlock()

	if _, err := f.Check(context.Background(), false); err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if upd.calls() != 2 {
		t.Errorf("a stale cache must be refreshed, got %d calls", upd.calls())
	}
}

func TestCheck_DevModeAndDevVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
		devMode bool
		want    error
	}{
		{name: "dev mode", version: "v1.2.0", devMode: true, want: ErrDevMode},
		{name: "dev build", version: "dev", devMode: false, want: ErrDevVersion},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upd := newMockUpdater()
			f := New(upd, tt.version, tt.devMode)

			_, err := f.Check(context.Background(), false)
			if !errors.Is(err, tt.want) {
				t.Fatalf("Check() error = %v, want %v", err, tt.want)
			}
			if upd.calls() != 0 {
				t.Error("a dev build must not reach GitHub at all")
			}
		})
	}
}

func TestCheck_GitHubFailureIsTyped(t *testing.T) {
	upd := newMockUpdater()
	upd.releaseErr = errors.New("fetch release: connection refused")
	f := New(upd, "v1.2.0", false)

	_, err := f.Check(context.Background(), false)
	var ghErr *GitHubError
	if !errors.As(err, &ghErr) {
		t.Fatalf("Check() error = %v, want *GitHubError", err)
	}
	if ghErr.Err.Error() != "fetch release: connection refused" {
		t.Errorf("wrapped cause = %v", ghErr.Err)
	}
}

func TestCheck_FailureDoesNotPoisonCache(t *testing.T) {
	upd := newMockUpdater()
	upd.releaseErr = errors.New("boom")
	f := New(upd, "v1.2.0", false)

	if _, err := f.Check(context.Background(), false); err == nil {
		t.Fatal("Check() must fail")
	}
	upd.releaseErr = nil
	res, err := f.Check(context.Background(), false)
	if err != nil {
		t.Fatalf("second Check() error = %v", err)
	}
	if res.Latest != "v1.3.0" {
		t.Errorf("a failed check must not be cached, got %+v", res)
	}
}

func TestInProgress_DelegatesToUpdater(t *testing.T) {
	upd := newMockUpdater()
	f := New(upd, "v1.2.0", false)

	if f.InProgress() {
		t.Error("InProgress() = true with no lock")
	}
	upd.inProgress = true
	if !f.InProgress() {
		t.Error("InProgress() must follow the updater's lock file")
	}
}
