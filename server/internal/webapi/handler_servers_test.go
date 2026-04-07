package webapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/vpnconfig"
)

func TestHandleListServers_OK(t *testing.T) {
	deps := newTestDeps(t)
	deps.Config = &mockConfig{
		servers: []vpnconfig.Server{
			{Address: "server1.example.com", Port: 443, UUID: "uuid-1", Name: "Server 1", IPs: []string{"1.1.1.1"}},
			{Address: "server2.example.com", Port: 443, UUID: "uuid-2", Name: "Server 2", IPs: []string{"2.2.2.2"}},
		},
	}

	handler := handleListServers(deps)

	req := httptest.NewRequest("GET", "/api/servers", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Servers []vpnconfig.Server `json:"servers"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Servers) != 2 {
		t.Errorf("expected 2 servers, got %d", len(resp.Servers))
	}
	if resp.Servers[0].Name != "Server 1" {
		t.Errorf("expected 'Server 1', got %q", resp.Servers[0].Name)
	}
}

func TestHandleListServers_Error(t *testing.T) {
	deps := newTestDeps(t)
	deps.Config = &mockConfig{err: errors.New("load failed")}

	handler := handleListServers(deps)

	req := httptest.NewRequest("GET", "/api/servers", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleSelectServer_OK(t *testing.T) {
	mc := &mockConfig{
		servers: []vpnconfig.Server{
			{Address: "s1.example.com", Port: 443, UUID: "uuid-1", Name: "S1", IPs: []string{"1.1.1.1"}},
			{Address: "s2.example.com", Port: 443, UUID: "uuid-2", Name: "S2", IPs: []string{"2.2.2.2"}},
		},
		cfg: &vpnconfig.VPNDirectorConfig{
			Xray: vpnconfig.XrayConfig{
				Servers: []string{"old-ip"},
			},
		},
	}

	deps := newTestDeps(t)
	deps.Config = mc

	handler := handleSelectServer(deps)

	body := `{"index": 1}`
	req := httptest.NewRequest("POST", "/api/servers/active", strings.NewReader(body))
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

	// Verify saved config has updated IPs.
	if mc.savedCfg == nil {
		t.Fatal("expected config to be saved")
	}
	if len(mc.savedCfg.Xray.Servers) != 1 || mc.savedCfg.Xray.Servers[0] != "2.2.2.2" {
		t.Errorf("expected Xray.Servers=[2.2.2.2], got %v", mc.savedCfg.Xray.Servers)
	}
}

func TestHandleSelectServer_IndexOutOfRange(t *testing.T) {
	deps := newTestDeps(t)
	deps.Config = &mockConfig{
		servers: []vpnconfig.Server{
			{Address: "s1.example.com", Port: 443, UUID: "uuid-1", Name: "S1", IPs: []string{"1.1.1.1"}},
		},
	}

	handler := handleSelectServer(deps)

	body := `{"index": 5}`
	req := httptest.NewRequest("POST", "/api/servers/active", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleSelectServer_NegativeIndex(t *testing.T) {
	deps := newTestDeps(t)
	deps.Config = &mockConfig{
		servers: []vpnconfig.Server{
			{Address: "s1.example.com", Port: 443, UUID: "uuid-1", Name: "S1", IPs: []string{"1.1.1.1"}},
		},
	}

	handler := handleSelectServer(deps)

	body := `{"index": -1}`
	req := httptest.NewRequest("POST", "/api/servers/active", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleSelectServer_InvalidJSON(t *testing.T) {
	deps := newTestDeps(t)

	handler := handleSelectServer(deps)

	req := httptest.NewRequest("POST", "/api/servers/active", strings.NewReader("not json"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleImportServers_InvalidURL(t *testing.T) {
	deps := newTestDeps(t)

	handler := handleImportServers(deps)

	body := `{"url": "http://example.com/sub"}`
	req := httptest.NewRequest("POST", "/api/servers/import", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "only https URLs are allowed" {
		t.Errorf("unexpected error: %q", resp["error"])
	}
}

func TestHandleImportServers_EmptyURL(t *testing.T) {
	deps := newTestDeps(t)

	handler := handleImportServers(deps)

	body := `{"url": ""}`
	req := httptest.NewRequest("POST", "/api/servers/import", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleImportServers_PrivateIP(t *testing.T) {
	deps := newTestDeps(t)

	handler := handleImportServers(deps)

	body := `{"url": "https://192.168.1.1/sub"}`
	req := httptest.NewRequest("POST", "/api/servers/import", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.Contains(resp["error"], "private") {
		t.Errorf("expected private IP error, got %q", resp["error"])
	}
}

func TestHandleImportServers_LoopbackIP(t *testing.T) {
	deps := newTestDeps(t)

	handler := handleImportServers(deps)

	body := `{"url": "https://127.0.0.1/sub"}`
	req := httptest.NewRequest("POST", "/api/servers/import", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		ip      string
		private bool
	}{
		{"127.0.0.1", true},
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"192.168.1.1", true},
		{"169.254.1.1", true},
		{"8.8.8.8", false},
		{"203.0.113.1", false},
		{"1.1.1.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			got := isPrivateHost(tt.ip)
			if got != tt.private {
				t.Errorf("isPrivateHost(%q) = %v, want %v", tt.ip, got, tt.private)
			}
		})
	}
}
