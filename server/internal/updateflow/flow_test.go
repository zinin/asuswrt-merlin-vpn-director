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

	// entered and block hold a GetLatestRelease call in flight: the call
	// reports itself on entered and then waits for block to be closed. A test
	// that needs several callers to contend for a cold cache uses them to keep
	// the first request from completing. Both are set before any goroutine
	// starts and are nil for every other test.
	entered chan struct{}
	block   chan struct{}

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
	m.releases++
	entered, block := m.entered, m.block
	release, err := m.release, m.releaseErr
	m.mu.Unlock()

	// Signal and wait outside m.mu, so a test holding a request in flight can
	// still read calls().
	if entered != nil {
		entered <- struct{}{}
	}
	if block != nil {
		<-block
	}

	if err != nil {
		return nil, err
	}
	return release, nil
}

func (m *mockUpdater) ShouldUpdate(_, _ string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.shouldUpdate, m.shouldUpdateErr
}

func (m *mockUpdater) IsUpdateInProgress() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.inProgress
}

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
	m.mu.Lock()
	defer m.mu.Unlock()
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

// setReleaseErr makes the next GetLatestRelease fail, or succeed again with
// nil. Mock fields are written through helpers so every access is under m.mu.
func (m *mockUpdater) setReleaseErr(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.releaseErr = err
}

// setInProgress sets what IsUpdateInProgress reports.
func (m *mockUpdater) setInProgress(v bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inProgress = v
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

func TestCheck_ConcurrentCallersShareOneRequest(t *testing.T) {
	// Check holds f.mu across the GitHub call on purpose. Callers arriving
	// while a request is in flight queue behind it and are served from the
	// cache it fills, instead of each spending one of the 60 unauthenticated
	// requests GitHub allows per hour. Without the gate a cold cache would let
	// all of them fetch at once, which the TTL alone cannot prevent.
	const callers = 8

	upd := newMockUpdater()
	upd.entered = make(chan struct{}, callers)
	upd.block = make(chan struct{})
	f := New(upd, "v1.2.0", false)

	results := make([]CheckResult, callers)
	errs := make([]error, callers)
	arrived := make(chan struct{}, callers)

	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			arrived <- struct{}{}
			results[i], errs[i] = f.Check(context.Background(), false)
		}(i)
	}

	// Keep the first request in flight until every caller has arrived, so the
	// others contend for a cache that is still empty.
	<-upd.entered
	for i := 0; i < callers; i++ {
		<-arrived
	}
	if n := upd.calls(); n != 1 {
		t.Errorf("with a request in flight and a cold cache, GetLatestRelease called %d times, want 1", n)
	}

	close(upd.block)
	wg.Wait()

	if n := upd.calls(); n != 1 {
		t.Errorf("%d concurrent callers made %d requests, want 1", callers, n)
	}
	for i := 0; i < callers; i++ {
		if errs[i] != nil {
			t.Fatalf("caller %d: Check() error = %v", i, errs[i])
		}
		if results[i].Latest != "v1.3.0" {
			t.Errorf("caller %d: Latest = %q, want every caller to see the same release", i, results[i].Latest)
		}
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
	upd.setReleaseErr(errors.New("fetch release: connection refused"))
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

func TestGitHubError_Contract(t *testing.T) {
	// All three accessors are load-bearing: the prefixed Error() is what a
	// caller prints when it has no better wording, while the bot and the Web
	// API reach past it for the bare cause so their own messages read as they
	// always have ("Failed to check for updates: connection refused").
	cause := errors.New("connection refused")
	var err error = &GitHubError{Err: cause}

	if got, want := err.Error(), "check for updates: connection refused"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}

	var ghErr *GitHubError
	if !errors.As(err, &ghErr) {
		t.Fatalf("errors.As() found no *GitHubError in %v", err)
	}
	if got, want := ghErr.Err.Error(), "connection refused"; got != want {
		t.Errorf("Err.Error() = %q, want the bare cause %q", got, want)
	}

	if got := errors.Unwrap(err); got != cause {
		t.Errorf("errors.Unwrap() = %v, want the cause itself", got)
	}
}

func TestCheck_FailureDoesNotPoisonCache(t *testing.T) {
	upd := newMockUpdater()
	upd.setReleaseErr(errors.New("boom"))
	f := New(upd, "v1.2.0", false)

	if _, err := f.Check(context.Background(), false); err == nil {
		t.Fatal("Check() must fail")
	}
	upd.setReleaseErr(nil)
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
	upd.setInProgress(true)
	if !f.InProgress() {
		t.Error("InProgress() must follow the updater's lock file")
	}
}
