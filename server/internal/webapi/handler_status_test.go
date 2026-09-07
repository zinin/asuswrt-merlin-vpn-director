package webapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleStatus_OK(t *testing.T) {
	deps := newTestDeps(t)
	deps.VPN = &mockVPN{statusOutput: "Xray: running\nTunnel Director: running"}

	handler := handleStatus(deps)

	req := httptest.NewRequest("GET", "/api/status", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["output"] != "Xray: running\nTunnel Director: running" {
		t.Errorf("unexpected output: %q", resp["output"])
	}
}

func TestHandleStatus_Error(t *testing.T) {
	deps := newTestDeps(t)
	deps.VPN = &mockVPN{err: errors.New("status failed")}

	handler := handleStatus(deps)

	req := httptest.NewRequest("GET", "/api/status", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleApply_OK(t *testing.T) {
	deps := newTestDeps(t)

	handler := handleApply(deps)

	req := httptest.NewRequest("POST", "/api/apply", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]bool
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp["ok"] {
		t.Error("expected ok: true")
	}
}

func TestHandleApply_Error(t *testing.T) {
	deps := newTestDeps(t)
	deps.VPN = &mockVPN{err: errors.New("apply failed (exit 1): [INFO] step\n[ERROR] Invalid country code 'xx'\n")}

	handler := handleApply(deps)

	req := httptest.NewRequest("POST", "/api/apply", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
	// The Status tab's Apply is the designated retry for a failed auto-apply,
	// so it must carry the shell's last line like the mutation endpoints do.
	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "failed to apply configuration: [ERROR] Invalid country code 'xx'" {
		t.Errorf("unexpected error text: %q", resp["error"])
	}
}

func TestHandleRestart_OK(t *testing.T) {
	deps := newTestDeps(t)

	handler := handleRestart(deps)

	req := httptest.NewRequest("POST", "/api/restart", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]bool
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp["ok"] {
		t.Error("expected ok: true")
	}
}

func TestHandleRestart_Error(t *testing.T) {
	deps := newTestDeps(t)
	deps.VPN = &mockVPN{err: errors.New("restart failed (exit 1): [INFO] stopping\n[ERROR] tunnel wgc1 is down\n")}

	handler := handleRestart(deps)

	req := httptest.NewRequest("POST", "/api/restart", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "failed to restart: [ERROR] tunnel wgc1 is down" {
		t.Errorf("unexpected error text: %q", resp["error"])
	}
}

func TestHandleStop_OK(t *testing.T) {
	deps := newTestDeps(t)

	handler := handleStop(deps)

	req := httptest.NewRequest("POST", "/api/stop", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]bool
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp["ok"] {
		t.Error("expected ok: true")
	}
}

func TestHandleStop_Error(t *testing.T) {
	deps := newTestDeps(t)
	deps.VPN = &mockVPN{err: errors.New("stop failed (exit 1): [INFO] flushing chains\n[ERROR] iptables: Permission denied\n")}

	handler := handleStop(deps)

	req := httptest.NewRequest("POST", "/api/stop", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "failed to stop: [ERROR] iptables: Permission denied" {
		t.Errorf("unexpected error text: %q", resp["error"])
	}
}

func TestHandleIP_OK(t *testing.T) {
	deps := newTestDeps(t)
	deps.Network = &mockNetwork{ip: "198.51.100.1"}

	handler := handleIP(deps)

	req := httptest.NewRequest("GET", "/api/ip", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["ip"] != "198.51.100.1" {
		t.Errorf("expected ip '198.51.100.1', got %q", resp["ip"])
	}
}

func TestHandleIP_Error(t *testing.T) {
	deps := newTestDeps(t)
	deps.Network = &mockNetwork{err: errors.New("network error")}

	handler := handleIP(deps)

	req := httptest.NewRequest("GET", "/api/ip", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

// The 500 used to be a flat "failed to get external IP" with the reason
// dropped on the floor: a DNS failure, an unreachable host and a bad response
// all reached the browser identically. The other control handlers already echo
// the cause; this one now does too.
func TestHandleIP_ErrorCarriesTheReason(t *testing.T) {
	deps := newTestDeps(t)
	deps.Network = &mockNetwork{err: errors.New("curl: could not resolve host (exit code 6)")}

	handler := handleIP(deps)

	req := httptest.NewRequest("GET", "/api/ip", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "failed to get external IP: curl: could not resolve host (exit code 6)" {
		t.Errorf("unexpected error text: %q", resp["error"])
	}
}

func TestHandleVersion(t *testing.T) {
	deps := newTestDeps(t)
	deps.Version = "2.1.0"
	deps.Commit = "deadbeef"

	handler := handleVersion(deps)

	req := httptest.NewRequest("GET", "/api/version", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["version"] != "2.1.0" {
		t.Errorf("expected version '2.1.0', got %q", resp["version"])
	}
	if resp["commit"] != "deadbeef" {
		t.Errorf("expected commit 'deadbeef', got %q", resp["commit"])
	}
}

func TestHandleUpdateIPsets_CallsUpdate(t *testing.T) {
	deps := newTestDeps(t)
	vpn := &mockVPN{}
	deps.VPN = vpn

	handler := handleUpdateIPsets(deps)

	req := httptest.NewRequest("POST", "/api/ipsets/update", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if vpn.updateCalls != 1 {
		t.Errorf("expected 1 Update() call, got %d", vpn.updateCalls)
	}
	if vpn.applyCalls != 0 {
		t.Errorf("expected no Apply() call, got %d", vpn.applyCalls)
	}

	var resp map[string]bool
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp["ok"] {
		t.Error("expected ok: true")
	}
}

func TestHandleUpdateIPsets_Error(t *testing.T) {
	deps := newTestDeps(t)
	deps.VPN = &mockVPN{err: errors.New("update failed (exit 1): [INFO] downloading\n[ERROR] no network\n")}

	handler := handleUpdateIPsets(deps)

	req := httptest.NewRequest("POST", "/api/ipsets/update", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "failed to update ipsets: [ERROR] no network" {
		t.Errorf("unexpected error text: %q", resp["error"])
	}
}
