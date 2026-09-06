package webapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/vpnconfig"
)

// deadlineRecorder is a ResponseRecorder that also accepts write deadlines,
// standing in for the *http.response the server hands to handlers.
type deadlineRecorder struct {
	*httptest.ResponseRecorder
	deadlines []time.Time
}

func (d *deadlineRecorder) SetWriteDeadline(t time.Time) error {
	d.deadlines = append(d.deadlines, t)
	return nil
}

func newDeadlineRecorder() *deadlineRecorder {
	return &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
}

// assertDeadlineAbout checks that the most recent deadline is now+want, within 2 s.
func assertDeadlineAbout(t *testing.T, rec *deadlineRecorder, want time.Duration) {
	t.Helper()
	if len(rec.deadlines) == 0 {
		t.Fatal("no write deadline was set")
	}
	got := time.Until(rec.deadlines[len(rec.deadlines)-1])
	if got > want || got < want-2*time.Second {
		t.Errorf("deadline in %s, want about %s", got, want)
	}
}

func TestExtendWriteDeadline_IgnoresUnsupportedWriter(t *testing.T) {
	rec := httptest.NewRecorder() // has no SetWriteDeadline: must neither panic nor fail
	extendWriteDeadline(rec, time.Minute)
}

func TestExtendWriteDeadline_ReachesWriterThroughStatusWriter(t *testing.T) {
	rec := newDeadlineRecorder()
	sw := &statusWriter{ResponseWriter: rec, status: http.StatusOK}
	extendWriteDeadline(sw, applyDeadline)
	assertDeadlineAbout(t, rec, applyDeadline)
}

func TestLockLongOp_ExtendsBeforeAndAfterTheWait(t *testing.T) {
	deps := newTestDeps(t)
	rec := newDeadlineRecorder()

	unlock := lockLongOp(rec, deps, updateDeadline)
	unlock()

	if len(rec.deadlines) != 2 {
		t.Fatalf("expected 2 deadline extensions (before and after the lock), got %d", len(rec.deadlines))
	}
	assertDeadlineAbout(t, rec, updateDeadline)

	// The mutex must be released: a second Lock must not block.
	locked := make(chan struct{})
	go func() { deps.OpMutex.Lock(); deps.OpMutex.Unlock(); close(locked) }()
	select {
	case <-locked:
	case <-time.After(time.Second):
		t.Fatal("OpMutex still held after unlock()")
	}
}

func TestUpdateHandlers_ReArmTheDeadlineAfterTheFlowCall(t *testing.T) {
	// The flow serializes its callers on one mutex, so a caller can spend a
	// whole updater.APITimeout queued before its own GitHub call starts -
	// exactly githubDeadline, leaving the response no margin at all. Both
	// routes therefore extend again once the call returns, when only the
	// write is left. On POST /api/update that write is the 202 without which
	// the page never starts polling, while the update runs anyway.
	tests := []struct {
		name    string
		method  string
		path    string
		handler func(*Deps) http.HandlerFunc
	}{
		{"update check", "GET", "/api/update/check", handleUpdateCheck},
		{"update start", "POST", "/api/update", handleUpdateStart},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newTestDeps(t)
			rec := newDeadlineRecorder()

			tt.handler(deps).ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))

			if len(rec.deadlines) != 2 {
				t.Fatalf("expected 2 deadline extensions (before and after the flow call), got %d", len(rec.deadlines))
			}
			if got := time.Until(rec.deadlines[0]); got > githubDeadline || got < githubDeadline-2*time.Second {
				t.Errorf("first deadline in %s, want about %s: it has to cover the GitHub call", got, githubDeadline)
			}
			assertDeadlineAbout(t, rec, deadlineSlack)
		})
	}
}

func TestExtendWriteDeadline_OutlivesServerWriteTimeout(t *testing.T) {
	// The real chain: net/http response -> loggingMiddleware's statusWriter ->
	// handler. WriteTimeout is 200 ms and the handler answers after 600 ms;
	// only a working extension lets the body reach the client.
	handler := loggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		extendWriteDeadline(w, 5*time.Second)
		time.Sleep(600 * time.Millisecond)
		_, _ = w.Write([]byte("late but delivered"))
	}))
	srv := httptest.NewUnstartedServer(handler)
	srv.Config.WriteTimeout = 200 * time.Millisecond
	srv.Start()
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL)
	if err != nil {
		t.Fatalf("request failed, so the write deadline was not extended: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "late but delivered" {
		t.Errorf("body = %q, want %q", body, "late but delivered")
	}
}

func TestExtendWriteDeadline_ControlWithoutExtensionFails(t *testing.T) {
	// Proves the previous test is not vacuous: the same server without the
	// extension tears the connection down.
	handler := loggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(600 * time.Millisecond)
		_, _ = w.Write([]byte("too late"))
	}))
	srv := httptest.NewUnstartedServer(handler)
	srv.Config.WriteTimeout = 200 * time.Millisecond
	srv.Start()
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL)
	if err == nil {
		resp.Body.Close()
		t.Fatal("expected the 200 ms WriteTimeout to tear the connection, but the request succeeded")
	}
}

func TestLongOpHandlers_ExtendWriteDeadline(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		body    string
		handler func(*Deps) http.HandlerFunc
		want    time.Duration
	}{
		{"status", "GET", "/api/status", "", handleStatus, statusDeadline},
		{"logs (all sources)", "GET", "/api/logs", "", handleLogs, logsDeadline},
		{"logs (single source)", "GET", "/api/logs?source=vpn", "", handleLogs, logsDeadline},
		{"apply", "POST", "/api/apply", "", handleApply, applyDeadline},
		{"restart", "POST", "/api/restart", "", handleRestart, applyDeadline},
		{"stop", "POST", "/api/stop", "", handleStop, applyDeadline},
		{"update ipsets", "POST", "/api/ipsets/update", "", handleUpdateIPsets, updateDeadline},
		{"add client", "POST", "/api/clients", `{"ip":"192.168.50.10","route":"xray"}`, handleAddClient, applyDeadline},
		{"pause client", "POST", "/api/clients/pause?ip=192.168.50.10", "", handlePauseClient, applyDeadline},
		{"resume client", "POST", "/api/clients/resume?ip=192.168.50.10", "", handleResumeClient, applyDeadline},
		{"delete client", "DELETE", "/api/clients?ip=192.168.50.10", "", handleDeleteClient, applyDeadline},
		{"update exclude sets", "POST", "/api/excludes/sets", `{"sets":["ru"]}`, handleUpdateExcludeSets, applyDeadline},
		{"add exclude ip", "POST", "/api/excludes/ips", `{"ip":"1.2.3.4"}`, handleAddExcludeIP, applyDeadline},
		{"delete exclude ip", "DELETE", "/api/excludes/ips?ip=1.2.3.4", "", handleDeleteExcludeIP, applyDeadline},
		{"select server", "POST", "/api/servers/active", `{"index":0}`, handleSelectServer, applyDeadline},
		{"import (rejected before download)", "POST", "/api/servers/import", `{"url":"http://insecure.example"}`, handleImportServers, importDeadline},
		// The two update routes extend twice: githubDeadline before the flow
		// call and deadlineSlack after it. This table sees the last one;
		// TestUpdateHandlers_ReArmTheDeadlineAfterTheFlowCall pins both.
		{"update check", "GET", "/api/update/check", "", handleUpdateCheck, deadlineSlack},
		{"update start", "POST", "/api/update", "", handleUpdateStart, deadlineSlack},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newTestDeps(t)
			deps.Config = &mockConfig{
				cfg: &vpnconfig.VPNDirectorConfig{
					TunnelDirector: vpnconfig.TunnelDirectorConfig{Tunnels: map[string]vpnconfig.TunnelConfig{}},
				},
				servers: []vpnconfig.Server{{Address: "srv", Port: 443, UUID: "u", IPs: []string{"1.1.1.1"}}},
			}
			rec := newDeadlineRecorder()
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			tt.handler(deps).ServeHTTP(rec, req)
			assertDeadlineAbout(t, rec, tt.want)
		})
	}
}
