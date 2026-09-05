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
	vpn := &mockVPN{}
	deps.VPN = vpn

	handler := handleAddClient(deps)

	body := `{"ip": "192.168.50.10", "route": "xray"}`
	req := httptest.NewRequest("POST", "/api/clients", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "client already configured for xray" {
		t.Errorf("unexpected error text: %q", resp["error"])
	}
	if mc.savedCfg != nil {
		t.Error("config must not be saved on conflict")
	}
	if vpn.applyCalls != 0 {
		t.Errorf("Apply() must not run on conflict, got %d calls", vpn.applyCalls)
	}
}

func TestHandleAddClient_TunnelRoute(t *testing.T) {
	mc := &mockConfig{
		cfg: &vpnconfig.VPNDirectorConfig{
			Xray: vpnconfig.XrayConfig{ExcludeSets: []string{"ru", "us"}},
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
	// A new tunnel inherits the Xray country exclusions, like the bot wizard.
	// t.Fatalf, not t.Errorf: the aliasing check below indexes Exclude[0].
	if strings.Join(tunnel.Exclude, ",") != "ru,us" {
		t.Fatalf("expected tunnel exclude [ru us], got %v", tunnel.Exclude)
	}
	// The copy must not alias xray.exclude_sets.
	tunnel.Exclude[0] = "changed"
	if mc.savedCfg.Xray.ExcludeSets[0] != "ru" {
		t.Error("tunnel exclude must be a copy of xray.exclude_sets, not the same slice")
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
	if tunnel.Exclude == nil {
		t.Error("expected non-nil exclude slice so it marshals to [] rather than null")
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
			Xray:          vpnconfig.XrayConfig{Clients: []string{"192.168.50.10"}},
			PausedClients: []string{},
		},
	}
	deps := newTestDeps(t)
	deps.Config = mc
	vpn := &mockVPN{}
	deps.VPN = vpn

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
	if vpn.applyCalls != 1 {
		t.Errorf("expected 1 Apply() call, got %d", vpn.applyCalls)
	}
}

func TestHandlePauseClient_AlreadyPaused(t *testing.T) {
	mc := &mockConfig{
		cfg: &vpnconfig.VPNDirectorConfig{
			Xray:          vpnconfig.XrayConfig{Clients: []string{"192.168.50.10"}},
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
			Xray:          vpnconfig.XrayConfig{Clients: []string{"192.168.50.10", "192.168.50.20"}},
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
		t.Fatalf("expected 1 paused client after resume, got %d", len(mc.savedCfg.PausedClients))
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

func TestHandleAddClient_ConflictOtherRoute(t *testing.T) {
	mc := &mockConfig{
		cfg: &vpnconfig.VPNDirectorConfig{
			TunnelDirector: vpnconfig.TunnelDirectorConfig{
				Tunnels: map[string]vpnconfig.TunnelConfig{
					"wgc1": {Clients: []string{"192.168.50.10/32"}, Exclude: []string{"ru"}},
				},
			},
		},
	}
	deps := newTestDeps(t)
	deps.Config = mc
	vpn := &mockVPN{}
	deps.VPN = vpn

	handler := handleAddClient(deps)

	body := `{"ip": "192.168.50.10", "route": "xray"}`
	req := httptest.NewRequest("POST", "/api/clients", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "client already configured for wgc1" {
		t.Errorf("unexpected error text: %q", resp["error"])
	}
	if mc.savedCfg != nil {
		t.Error("config must not be saved on conflict")
	}
	if vpn.applyCalls != 0 {
		t.Errorf("Apply() must not run on conflict, got %d calls", vpn.applyCalls)
	}
}

func TestHandleAddClient_StripsSlash32(t *testing.T) {
	mc := &mockConfig{cfg: &vpnconfig.VPNDirectorConfig{}}
	deps := newTestDeps(t)
	deps.Config = mc

	handler := handleAddClient(deps)

	body := `{"ip": "192.168.50.40/32", "route": "xray"}`
	req := httptest.NewRequest("POST", "/api/clients", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := strings.Join(mc.savedCfg.Xray.Clients, ","); got != "192.168.50.40" {
		t.Errorf("expected /32 stripped, got %q", got)
	}
}

func TestHandleAddClient_RejectsIPv6(t *testing.T) {
	deps := newTestDeps(t)
	deps.Config = &mockConfig{cfg: &vpnconfig.VPNDirectorConfig{}}

	handler := handleAddClient(deps)

	body := `{"ip": "::1", "route": "xray"}`
	req := httptest.NewRequest("POST", "/api/clients", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "invalid IPv4 address or CIDR" {
		t.Errorf("unexpected error text: %q", resp["error"])
	}
}

func TestHandleAddClient_AppliesAfterSave(t *testing.T) {
	mc := &mockConfig{cfg: &vpnconfig.VPNDirectorConfig{}}
	deps := newTestDeps(t)
	deps.Config = mc
	vpn := &mockVPN{}
	deps.VPN = vpn

	handler := handleAddClient(deps)

	body := `{"ip": "192.168.50.50", "route": "xray"}`
	req := httptest.NewRequest("POST", "/api/clients", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if vpn.applyCalls != 1 {
		t.Errorf("expected 1 Apply() call after save, got %d", vpn.applyCalls)
	}
}

func TestHandleAddClient_SavedButApplyFailed(t *testing.T) {
	mc := &mockConfig{cfg: &vpnconfig.VPNDirectorConfig{}}
	deps := newTestDeps(t)
	deps.Config = mc
	// Shell failures carry the whole script output; lastErrorLine echoes only
	// its last line, so the fixture spans lines like the real apply output.
	deps.VPN = &mockVPN{err: errors.New("apply failed (exit 1):\n[ERROR] iptables missing")}

	handler := handleAddClient(deps)

	body := `{"ip": "192.168.50.50", "route": "xray"}`
	req := httptest.NewRequest("POST", "/api/clients", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
	if mc.savedCfg == nil {
		t.Fatal("expected config to be saved even though apply failed")
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["saved"] != true {
		t.Errorf("expected saved: true, got %v", resp["saved"])
	}
	if resp["error"] != "configuration saved, but apply failed: [ERROR] iptables missing" {
		t.Errorf("unexpected error text: %v", resp["error"])
	}
}

func TestHandlePauseClient_NotFound(t *testing.T) {
	deps := newTestDeps(t)
	mc := &mockConfig{cfg: &vpnconfig.VPNDirectorConfig{}}
	deps.Config = mc

	handler := handlePauseClient(deps)

	req := httptest.NewRequest("POST", "/api/clients/pause?ip=192.168.50.99", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
	if mc.savedCfg != nil {
		t.Error("config must not be saved for an unknown client")
	}
}

func TestHandlePauseClient_InvalidIP(t *testing.T) {
	deps := newTestDeps(t)

	handler := handlePauseClient(deps)

	req := httptest.NewRequest("POST", "/api/clients/pause?ip=::1", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandlePauseClient_KeepsStoredForm(t *testing.T) {
	// The shell subtracts paused_clients from the clients arrays by exact
	// string, so the paused entry must use the stored spelling (here with /32).
	mc := &mockConfig{
		cfg: &vpnconfig.VPNDirectorConfig{
			TunnelDirector: vpnconfig.TunnelDirectorConfig{
				Tunnels: map[string]vpnconfig.TunnelConfig{
					"wgc1": {Clients: []string{"192.168.50.20/32"}, Exclude: []string{"ru"}},
				},
			},
		},
	}
	deps := newTestDeps(t)
	deps.Config = mc

	handler := handlePauseClient(deps)

	req := httptest.NewRequest("POST", "/api/clients/pause?ip=192.168.50.20", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := strings.Join(mc.savedCfg.PausedClients, ","); got != "192.168.50.20/32" {
		t.Errorf("expected paused entry in stored form 192.168.50.20/32, got %q", got)
	}
}

func TestHandleResumeClient_MatchesStoredForm(t *testing.T) {
	mc := &mockConfig{
		cfg: &vpnconfig.VPNDirectorConfig{
			TunnelDirector: vpnconfig.TunnelDirectorConfig{
				Tunnels: map[string]vpnconfig.TunnelConfig{
					"wgc1": {Clients: []string{"192.168.50.20/32"}, Exclude: []string{"ru"}},
				},
			},
			PausedClients: []string{"192.168.50.20/32"},
		},
	}
	deps := newTestDeps(t)
	deps.Config = mc

	handler := handleResumeClient(deps)

	req := httptest.NewRequest("POST", "/api/clients/resume?ip=192.168.50.20", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(mc.savedCfg.PausedClients) != 0 {
		t.Errorf("expected paused list empty, got %v", mc.savedCfg.PausedClients)
	}
}

func TestHandleResumeClient_NotFound(t *testing.T) {
	deps := newTestDeps(t)
	deps.Config = &mockConfig{cfg: &vpnconfig.VPNDirectorConfig{PausedClients: []string{"192.168.50.10"}}}

	handler := handleResumeClient(deps)

	req := httptest.NewRequest("POST", "/api/clients/resume?ip=192.168.50.10", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a paused entry that is no longer a client, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDeleteClient_NotFound(t *testing.T) {
	deps := newTestDeps(t)
	deps.Config = &mockConfig{cfg: &vpnconfig.VPNDirectorConfig{}}

	handler := handleDeleteClient(deps)

	req := httptest.NewRequest("DELETE", "/api/clients?ip=192.168.50.99", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDeleteClient_RemovesStoredForm(t *testing.T) {
	mc := &mockConfig{
		cfg: &vpnconfig.VPNDirectorConfig{
			TunnelDirector: vpnconfig.TunnelDirectorConfig{
				Tunnels: map[string]vpnconfig.TunnelConfig{
					"wgc1": {Clients: []string{"192.168.50.20/32", "192.168.50.30"}, Exclude: []string{"ru"}},
				},
			},
			PausedClients: []string{"192.168.50.20/32"},
		},
	}
	deps := newTestDeps(t)
	deps.Config = mc
	vpn := &mockVPN{}
	deps.VPN = vpn

	handler := handleDeleteClient(deps)

	req := httptest.NewRequest("DELETE", "/api/clients?ip=192.168.50.20", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := strings.Join(mc.savedCfg.TunnelDirector.Tunnels["wgc1"].Clients, ","); got != "192.168.50.30" {
		t.Errorf("expected wgc1 clients [192.168.50.30], got %q", got)
	}
	if len(mc.savedCfg.PausedClients) != 0 {
		t.Errorf("expected paused list cleared, got %v", mc.savedCfg.PausedClients)
	}
	if vpn.applyCalls != 1 {
		t.Errorf("expected 1 Apply() call, got %d", vpn.applyCalls)
	}
}

func TestRemoveAddr(t *testing.T) {
	got := removeAddr([]string{"192.168.50.20/32", "192.168.50.30", "192.168.50.20", "garbage"}, "192.168.50.20")
	if strings.Join(got, ",") != "192.168.50.30,garbage" {
		t.Errorf("removeAddr = %v, want [192.168.50.30 garbage]", got)
	}
	if !containsAddr([]string{"1.2.3.4/32"}, "1.2.3.4") {
		t.Error("containsAddr must match the normalized form")
	}
	if containsAddr(nil, "1.2.3.4") {
		t.Error("containsAddr on nil must be false")
	}
}
