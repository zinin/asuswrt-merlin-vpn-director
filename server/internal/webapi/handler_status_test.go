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
	deps.VPN = &mockVPN{err: errors.New("apply failed")}

	handler := handleApply(deps)

	req := httptest.NewRequest("POST", "/api/apply", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
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
	deps.VPN = &mockVPN{err: errors.New("restart failed")}

	handler := handleRestart(deps)

	req := httptest.NewRequest("POST", "/api/restart", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
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
	deps.VPN = &mockVPN{err: errors.New("stop failed")}

	handler := handleStop(deps)

	req := httptest.NewRequest("POST", "/api/stop", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
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

func TestHandleUpdateIPsets_OK(t *testing.T) {
	deps := newTestDeps(t)

	handler := handleUpdateIPsets(deps)

	req := httptest.NewRequest("POST", "/api/ipsets/update", nil)
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
