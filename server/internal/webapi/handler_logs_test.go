package webapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/vpnconfig"
)

func TestHandleLogs_SingleSource(t *testing.T) {
	deps := newTestDeps(t)
	deps.Logs = &mockLogs{output: "vpn log line 1\nvpn log line 2"}

	handler := handleLogs(deps)

	req := httptest.NewRequest("GET", "/api/logs?source=vpn&lines=10", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["source"] != "vpn" {
		t.Errorf("expected source 'vpn', got %q", resp["source"])
	}
	if resp["output"] != "vpn log line 1\nvpn log line 2" {
		t.Errorf("unexpected output: %q", resp["output"])
	}
}

func TestHandleLogs_AllSources(t *testing.T) {
	deps := newTestDeps(t)
	deps.Logs = &mockLogs{output: "some log content"}

	handler := handleLogs(deps)

	req := httptest.NewRequest("GET", "/api/logs", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	for _, key := range []string{"vpn", "xray", "bot"} {
		if _, ok := resp[key]; !ok {
			t.Errorf("expected key %q in response", key)
		}
	}
}

func TestHandleLogs_InvalidSource(t *testing.T) {
	deps := newTestDeps(t)

	handler := handleLogs(deps)

	req := httptest.NewRequest("GET", "/api/logs?source=invalid", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleLogs_InvalidLines(t *testing.T) {
	deps := newTestDeps(t)

	handler := handleLogs(deps)

	req := httptest.NewRequest("GET", "/api/logs?lines=abc", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleLogs_NegativeLines(t *testing.T) {
	deps := newTestDeps(t)

	handler := handleLogs(deps)

	req := httptest.NewRequest("GET", "/api/logs?lines=-5", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleLogs_LinesCapAt500(t *testing.T) {
	deps := newTestDeps(t)
	deps.Logs = &mockLogs{output: "capped"}

	handler := handleLogs(deps)

	req := httptest.NewRequest("GET", "/api/logs?source=vpn&lines=1000", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleLogs_ReadError(t *testing.T) {
	deps := newTestDeps(t)
	deps.Logs = &mockLogs{err: errors.New("file not found")}

	handler := handleLogs(deps)

	req := httptest.NewRequest("GET", "/api/logs?source=vpn", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleLogs_AllSourcesWithError(t *testing.T) {
	deps := newTestDeps(t)
	deps.Logs = &mockLogs{err: errors.New("file not found")}

	handler := handleLogs(deps)

	req := httptest.NewRequest("GET", "/api/logs", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// When reading all sources, errors are embedded in the response, not returned as HTTP errors.
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	for _, key := range []string{"vpn", "xray", "bot"} {
		if val, ok := resp[key]; !ok {
			t.Errorf("expected key %q in response", key)
		} else if val != "error: file not found" {
			t.Errorf("expected error message for %q, got %q", key, val)
		}
	}
}

func TestHandleConfig_OK(t *testing.T) {
	deps := newTestDeps(t)
	deps.Config = &mockConfig{
		cfg: &vpnconfig.VPNDirectorConfig{
			DataDir: "/opt/vpn-director/data",
			WebUI: vpnconfig.WebUIConfig{
				Port:      8443,
				JWTSecret: "super-secret-key",
			},
			Xray: vpnconfig.XrayConfig{
				Clients:     []string{"192.168.50.10"},
				ExcludeSets: []string{"ru"},
			},
		},
	}

	handler := handleConfig(deps)

	req := httptest.NewRequest("GET", "/api/config", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp vpnconfig.VPNDirectorConfig
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// JWT secret should be redacted.
	if resp.WebUI.JWTSecret != "" {
		t.Errorf("expected JWTSecret to be redacted, got %q", resp.WebUI.JWTSecret)
	}

	// Other fields should be present.
	if resp.WebUI.Port != 8443 {
		t.Errorf("expected port 8443, got %d", resp.WebUI.Port)
	}
	if resp.DataDir != "/opt/vpn-director/data" {
		t.Errorf("expected data_dir '/opt/vpn-director/data', got %q", resp.DataDir)
	}
}

func TestHandleConfig_Error(t *testing.T) {
	deps := newTestDeps(t)
	deps.Config = &mockConfig{err: errors.New("config error")}

	handler := handleConfig(deps)

	req := httptest.NewRequest("GET", "/api/config", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleUpdate_NotSupported(t *testing.T) {
	deps := newTestDeps(t)

	handler := handleUpdate(deps)

	req := httptest.NewRequest("POST", "/api/update", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp["ok"] != false {
		t.Error("expected ok: false")
	}
	errMsg, ok := resp["error"].(string)
	if !ok || errMsg == "" {
		t.Error("expected error message")
	}
}
