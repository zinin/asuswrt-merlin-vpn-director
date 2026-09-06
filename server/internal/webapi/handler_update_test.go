package webapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/updateflow"
)

func TestHandleUpdateCheck_ReturnsTheResult(t *testing.T) {
	deps := newTestDeps(t)
	flow := deps.Update.(*mockUpdateFlow)
	flow.checkResult = updateflow.CheckResult{
		Current:         "v1.2.0",
		Latest:          "v1.3.0",
		UpdateAvailable: true,
		Changelog:       "what is new",
		CheckedAt:       time.Now(),
	}

	rec := httptest.NewRecorder()
	handleUpdateCheck(deps)(rec, httptest.NewRequest("GET", "/api/update/check", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["latest"] != "v1.3.0" || resp["update_available"] != true {
		t.Errorf("unexpected body: %v", resp)
	}
}

func TestHandleUpdateCheck_ForcePassedThrough(t *testing.T) {
	deps := newTestDeps(t)
	flow := deps.Update.(*mockUpdateFlow)

	rec := httptest.NewRecorder()
	handleUpdateCheck(deps)(rec, httptest.NewRequest("GET", "/api/update/check?force=1", nil))

	if !flow.checkForce {
		t.Error("force=1 must reach the flow, otherwise the button does nothing")
	}
}

func TestHandleUpdateCheck_DevBuild(t *testing.T) {
	for _, devErr := range []error{updateflow.ErrDevMode, updateflow.ErrDevVersion} {
		deps := newTestDeps(t)
		deps.Update.(*mockUpdateFlow).checkErr = devErr

		rec := httptest.NewRecorder()
		handleUpdateCheck(deps)(rec, httptest.NewRequest("GET", "/api/update/check", nil))

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for %v, got %d", devErr, rec.Code)
		}
		var resp map[string]interface{}
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp["dev"] != true || resp["update_available"] != false {
			t.Errorf("unexpected dev body: %v", resp)
		}
	}
}

func TestHandleUpdateCheck_GitHubDown(t *testing.T) {
	deps := newTestDeps(t)
	deps.Update.(*mockUpdateFlow).checkErr = &updateflow.GitHubError{Err: errors.New("connection refused")}

	rec := httptest.NewRecorder()
	handleUpdateCheck(deps)(rec, httptest.NewRequest("GET", "/api/update/check", nil))

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", rec.Code, rec.Body.String())
	}
	assertGitHubErrorBody(t, rec)
}

func TestHandleUpdateStart_Accepted(t *testing.T) {
	deps := newTestDeps(t)
	flow := deps.Update.(*mockUpdateFlow)
	flow.startResult = updateflow.StartResult{From: "v1.2.0", To: "v1.3.0"}

	rec := httptest.NewRecorder()
	handleUpdateStart(deps)(rec, httptest.NewRequest("POST", "/api/update", nil))

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["ok"] != true || resp["from"] != "v1.2.0" || resp["to"] != "v1.3.0" {
		t.Errorf("unexpected body: %v", resp)
	}
	if flow.startInitiator != "webui" || flow.startChatID != 0 {
		t.Errorf("Start(initiator=%q, chat=%d), want webui/0", flow.startInitiator, flow.startChatID)
	}
}

func TestHandleUpdateStart_StatusPerOutcome(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "up to date", err: updateflow.ErrUpToDate, want: http.StatusOK},
		{name: "dev mode", err: updateflow.ErrDevMode, want: http.StatusBadRequest},
		{name: "dev build", err: updateflow.ErrDevVersion, want: http.StatusBadRequest},
		{name: "in progress", err: updateflow.ErrInProgress, want: http.StatusConflict},
		{name: "github down", err: &updateflow.GitHubError{Err: errors.New("timeout")}, want: http.StatusBadGateway},
		{name: "anything else", err: errors.New("invalid release version"), want: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newTestDeps(t)
			deps.Update.(*mockUpdateFlow).startErr = tt.err

			rec := httptest.NewRecorder()
			handleUpdateStart(deps)(rec, httptest.NewRequest("POST", "/api/update", nil))

			if rec.Code != tt.want {
				t.Fatalf("expected %d, got %d: %s", tt.want, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestHandleUpdateStart_UpToDateBody(t *testing.T) {
	deps := newTestDeps(t)
	deps.Update.(*mockUpdateFlow).startErr = updateflow.ErrUpToDate

	rec := httptest.NewRecorder()
	handleUpdateStart(deps)(rec, httptest.NewRequest("POST", "/api/update", nil))

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["ok"] != true || resp["update_available"] != false {
		t.Errorf("unexpected body: %v", resp)
	}
}

func TestHandleUpdateStart_GitHubDownBody(t *testing.T) {
	deps := newTestDeps(t)
	deps.Update.(*mockUpdateFlow).startErr = &updateflow.GitHubError{Err: errors.New("connection refused")}

	rec := httptest.NewRecorder()
	handleUpdateStart(deps)(rec, httptest.NewRequest("POST", "/api/update", nil))

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", rec.Code, rec.Body.String())
	}
	assertGitHubErrorBody(t, rec)
}

// assertGitHubErrorBody pins the wording of a 502: the handler must print the
// cause the GitHubError wraps, never the wrapper itself. GitHubError.Error()
// already prefixes "check for updates: ", so printing it doubles the sentence
// into "failed to check for updates: check for updates: connection refused".
func assertGitHubErrorBody(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "failed to check for updates: connection refused" {
		t.Errorf("502 body %q: the GitHubError wrapper must not be printed, it doubles the sentence", resp["error"])
	}
}

func TestHandleUpdateStatus(t *testing.T) {
	tests := []struct {
		name       string
		inProgress bool
	}{
		{name: "script running", inProgress: true},
		{name: "idle", inProgress: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newTestDeps(t)
			deps.Update.(*mockUpdateFlow).inProgress = tt.inProgress

			rec := httptest.NewRecorder()
			handleUpdateStatus(deps)(rec, httptest.NewRequest("GET", "/api/update/status", nil))

			var resp map[string]bool
			if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if resp["in_progress"] != tt.inProgress {
				t.Errorf("in_progress = %t, want %t: the body must follow the flow, not a constant",
					resp["in_progress"], tt.inProgress)
			}
		})
	}
}
