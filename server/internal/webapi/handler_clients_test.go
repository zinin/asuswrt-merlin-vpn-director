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

func TestHandleListClients_OK(t *testing.T) {
	deps := newTestDeps(t)
	deps.Config = &mockConfig{
		cfg: &vpnconfig.VPNDirectorConfig{
			PausedClients: []string{"192.168.50.20"},
			Xray: vpnconfig.XrayConfig{
				Clients: []string{"192.168.50.10", "192.168.50.20"},
			},
			TunnelDirector: vpnconfig.TunnelDirectorConfig{
				Tunnels: map[string]vpnconfig.TunnelConfig{
					"wgc1": {Clients: []string{"192.168.50.30"}, Exclude: []string{"ru"}},
				},
			},
		},
	}

	handler := handleListClients(deps)

	req := httptest.NewRequest("GET", "/api/clients", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Clients []vpnconfig.ClientInfo `json:"clients"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Clients) != 3 {
		t.Fatalf("expected 3 clients, got %d", len(resp.Clients))
	}

	// Check that paused client is marked.
	for _, c := range resp.Clients {
		if c.IP == "192.168.50.20" && !c.Paused {
			t.Error("expected 192.168.50.20 to be paused")
		}
		if c.IP == "192.168.50.10" && c.Paused {
			t.Error("expected 192.168.50.10 to not be paused")
		}
	}
}

func TestHandleListClients_Error(t *testing.T) {
	deps := newTestDeps(t)
	deps.Config = &mockConfig{err: errors.New("config error")}

	handler := handleListClients(deps)

	req := httptest.NewRequest("GET", "/api/clients", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleAddClient_XrayRoute(t *testing.T) {
	mc := &mockConfig{
		cfg: &vpnconfig.VPNDirectorConfig{
			Xray: vpnconfig.XrayConfig{
				Clients: []string{"192.168.50.10"},
			},
			TunnelDirector: vpnconfig.TunnelDirectorConfig{
				Tunnels: map[string]vpnconfig.TunnelConfig{},
			},
		},
	}
	deps := newTestDeps(t)
	deps.Config = mc

	handler := handleAddClient(deps)

	body := `{"ip": "192.168.50.20", "route": "xray"}`
	req := httptest.NewRequest("POST", "/api/clients", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if mc.savedCfg == nil {
		t.Fatal("expected config to be saved")
	}
	if len(mc.savedCfg.Xray.Clients) != 2 {
		t.Fatalf("expected 2 xray clients, got %d", len(mc.savedCfg.Xray.Clients))
	}
	if mc.savedCfg.Xray.Clients[1] != "192.168.50.20" {
		t.Errorf("expected new client '192.168.50.20', got %q", mc.savedCfg.Xray.Clients[1])
	}
}

func TestHandleAddClient_XrayDuplicate(t *testing.T) {
	mc := &mockConfig{
		cfg: &vpnconfig.VPNDirectorConfig{
			Xray: vpnconfig.XrayConfig{
				Clients: []string{"192.168.50.10"},
			},
			TunnelDirector: vpnconfig.TunnelDirectorConfig{
				Tunnels: map[string]vpnconfig.TunnelConfig{},
			},
		},
	}
	deps := newTestDeps(t)
	deps.Config = mc

	handler := handleAddClient(deps)

	body := `{"ip": "192.168.50.10", "route": "xray"}`
	req := httptest.NewRequest("POST", "/api/clients", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Should not add duplicate.
	if mc.savedCfg == nil {
		t.Fatal("expected config to be saved")
	}
	if len(mc.savedCfg.Xray.Clients) != 1 {
		t.Errorf("expected 1 xray client (no duplicate), got %d", len(mc.savedCfg.Xray.Clients))
	}
}

func TestHandleAddClient_TunnelRoute(t *testing.T) {
	mc := &mockConfig{
		cfg: &vpnconfig.VPNDirectorConfig{
			Xray: vpnconfig.XrayConfig{},
			TunnelDirector: vpnconfig.TunnelDirectorConfig{
				Tunnels: map[string]vpnconfig.TunnelConfig{},
			},
		},
	}
	deps := newTestDeps(t)
	deps.Config = mc

	handler := handleAddClient(deps)

	body := `{"ip": "192.168.50.30", "route": "wgc1"}`
	req := httptest.NewRequest("POST", "/api/clients", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if mc.savedCfg == nil {
		t.Fatal("expected config to be saved")
	}
	tunnel, ok := mc.savedCfg.TunnelDirector.Tunnels["wgc1"]
	if !ok {
		t.Fatal("expected wgc1 tunnel to be created")
	}
	if len(tunnel.Clients) != 1 || tunnel.Clients[0] != "192.168.50.30" {
		t.Errorf("expected tunnel client [192.168.50.30], got %v", tunnel.Clients)
	}
}

func TestHandleAddClient_TunnelRouteNilTunnels(t *testing.T) {
	mc := &mockConfig{
		cfg: &vpnconfig.VPNDirectorConfig{
			Xray: vpnconfig.XrayConfig{},
			TunnelDirector: vpnconfig.TunnelDirectorConfig{
				Tunnels: nil,
			},
		},
	}
	deps := newTestDeps(t)
	deps.Config = mc

	handler := handleAddClient(deps)

	body := `{"ip": "192.168.50.30", "route": "ovpnc1"}`
	req := httptest.NewRequest("POST", "/api/clients", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if mc.savedCfg == nil {
		t.Fatal("expected config to be saved")
	}
	if mc.savedCfg.TunnelDirector.Tunnels == nil {
		t.Fatal("expected tunnels map to be initialized")
	}
	tunnel, ok := mc.savedCfg.TunnelDirector.Tunnels["ovpnc1"]
	if !ok {
		t.Fatal("expected ovpnc1 tunnel to be created")
	}
	if len(tunnel.Clients) != 1 {
		t.Errorf("expected 1 client, got %d", len(tunnel.Clients))
	}
}

func TestHandleAddClient_MissingIP(t *testing.T) {
	deps := newTestDeps(t)
	deps.Config = &mockConfig{
		cfg: &vpnconfig.VPNDirectorConfig{},
	}

	handler := handleAddClient(deps)

	body := `{"ip": "", "route": "xray"}`
	req := httptest.NewRequest("POST", "/api/clients", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleAddClient_MissingRoute(t *testing.T) {
	deps := newTestDeps(t)
	deps.Config = &mockConfig{
		cfg: &vpnconfig.VPNDirectorConfig{},
	}

	handler := handleAddClient(deps)

	body := `{"ip": "192.168.50.10", "route": ""}`
	req := httptest.NewRequest("POST", "/api/clients", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandlePauseClient_OK(t *testing.T) {
	mc := &mockConfig{
		cfg: &vpnconfig.VPNDirectorConfig{
			PausedClients: []string{},
		},
	}
	deps := newTestDeps(t)
	deps.Config = mc

	handler := handlePauseClient(deps)

	req := httptest.NewRequest("POST", "/api/clients/pause?ip=192.168.50.10", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if mc.savedCfg == nil {
		t.Fatal("expected config to be saved")
	}
	if len(mc.savedCfg.PausedClients) != 1 || mc.savedCfg.PausedClients[0] != "192.168.50.10" {
		t.Errorf("expected PausedClients=[192.168.50.10], got %v", mc.savedCfg.PausedClients)
	}
}

func TestHandlePauseClient_AlreadyPaused(t *testing.T) {
	mc := &mockConfig{
		cfg: &vpnconfig.VPNDirectorConfig{
			PausedClients: []string{"192.168.50.10"},
		},
	}
	deps := newTestDeps(t)
	deps.Config = mc

	handler := handlePauseClient(deps)

	req := httptest.NewRequest("POST", "/api/clients/pause?ip=192.168.50.10", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Should not add duplicate.
	if mc.savedCfg == nil {
		t.Fatal("expected config to be saved")
	}
	if len(mc.savedCfg.PausedClients) != 1 {
		t.Errorf("expected 1 paused client (no duplicate), got %d", len(mc.savedCfg.PausedClients))
	}
}

func TestHandlePauseClient_MissingIP(t *testing.T) {
	deps := newTestDeps(t)

	handler := handlePauseClient(deps)

	req := httptest.NewRequest("POST", "/api/clients/pause", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleResumeClient_OK(t *testing.T) {
	mc := &mockConfig{
		cfg: &vpnconfig.VPNDirectorConfig{
			PausedClients: []string{"192.168.50.10", "192.168.50.20"},
		},
	}
	deps := newTestDeps(t)
	deps.Config = mc

	handler := handleResumeClient(deps)

	req := httptest.NewRequest("POST", "/api/clients/resume?ip=192.168.50.10", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if mc.savedCfg == nil {
		t.Fatal("expected config to be saved")
	}
	if len(mc.savedCfg.PausedClients) != 1 {
		t.Errorf("expected 1 paused client after resume, got %d", len(mc.savedCfg.PausedClients))
	}
	if mc.savedCfg.PausedClients[0] != "192.168.50.20" {
		t.Errorf("expected remaining paused client '192.168.50.20', got %q", mc.savedCfg.PausedClients[0])
	}
}

func TestHandleResumeClient_MissingIP(t *testing.T) {
	deps := newTestDeps(t)

	handler := handleResumeClient(deps)

	req := httptest.NewRequest("POST", "/api/clients/resume", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDeleteClient_OK(t *testing.T) {
	mc := &mockConfig{
		cfg: &vpnconfig.VPNDirectorConfig{
			Xray: vpnconfig.XrayConfig{
				Clients: []string{"192.168.50.10", "192.168.50.20"},
			},
			TunnelDirector: vpnconfig.TunnelDirectorConfig{
				Tunnels: map[string]vpnconfig.TunnelConfig{
					"wgc1": {
						Clients: []string{"192.168.50.10", "192.168.50.30"},
						Exclude: []string{"ru"},
					},
				},
			},
		},
	}
	deps := newTestDeps(t)
	deps.Config = mc

	handler := handleDeleteClient(deps)

	req := httptest.NewRequest("DELETE", "/api/clients?ip=192.168.50.10", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if mc.savedCfg == nil {
		t.Fatal("expected config to be saved")
	}

	// Removed from Xray.
	if len(mc.savedCfg.Xray.Clients) != 1 || mc.savedCfg.Xray.Clients[0] != "192.168.50.20" {
		t.Errorf("expected Xray.Clients=[192.168.50.20], got %v", mc.savedCfg.Xray.Clients)
	}

	// Removed from tunnel.
	tunnel := mc.savedCfg.TunnelDirector.Tunnels["wgc1"]
	if len(tunnel.Clients) != 1 || tunnel.Clients[0] != "192.168.50.30" {
		t.Errorf("expected wgc1 clients=[192.168.50.30], got %v", tunnel.Clients)
	}
	// Tunnel config preserved (exclude still there).
	if len(tunnel.Exclude) != 1 {
		t.Errorf("expected wgc1 exclude preserved, got %v", tunnel.Exclude)
	}
}

func TestHandleDeleteClient_MissingIP(t *testing.T) {
	deps := newTestDeps(t)

	handler := handleDeleteClient(deps)

	req := httptest.NewRequest("DELETE", "/api/clients", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestContains(t *testing.T) {
	if !contains([]string{"a", "b", "c"}, "b") {
		t.Error("expected true for present item")
	}
	if contains([]string{"a", "b"}, "c") {
		t.Error("expected false for absent item")
	}
	if contains(nil, "a") {
		t.Error("expected false for nil slice")
	}
}

func TestRemoveString(t *testing.T) {
	result := removeString([]string{"a", "b", "c", "b"}, "b")
	if len(result) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(result))
	}
	if result[0] != "a" || result[1] != "c" {
		t.Errorf("expected [a c], got %v", result)
	}

	// Remove non-existent.
	result = removeString([]string{"a", "b"}, "z")
	if len(result) != 2 {
		t.Errorf("expected 2 elements for non-existent removal, got %d", len(result))
	}

	// Remove from nil.
	result = removeString(nil, "a")
	if len(result) != 0 {
		t.Errorf("expected 0 elements for nil slice, got %d", len(result))
	}
}
