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

func TestHandleListExcludeSets_OK(t *testing.T) {
	deps := newTestDeps(t)
	deps.Config = &mockConfig{
		cfg: &vpnconfig.VPNDirectorConfig{
			Xray: vpnconfig.XrayConfig{
				ExcludeSets: []string{"ru", "ua"},
			},
		},
	}

	handler := handleListExcludeSets(deps)

	req := httptest.NewRequest("GET", "/api/excludes/sets", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Sets []string `json:"sets"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Sets) != 2 {
		t.Errorf("expected 2 sets, got %d", len(resp.Sets))
	}
}

func TestHandleListExcludeSets_Error(t *testing.T) {
	deps := newTestDeps(t)
	deps.Config = &mockConfig{err: errors.New("load failed")}

	handler := handleListExcludeSets(deps)

	req := httptest.NewRequest("GET", "/api/excludes/sets", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleUpdateExcludeSets_OK(t *testing.T) {
	mc := &mockConfig{
		cfg: &vpnconfig.VPNDirectorConfig{
			Xray: vpnconfig.XrayConfig{
				ExcludeSets: []string{"ru"},
			},
		},
	}
	deps := newTestDeps(t)
	deps.Config = mc

	handler := handleUpdateExcludeSets(deps)

	body := `{"sets": ["ru", "ua", "by"]}`
	req := httptest.NewRequest("POST", "/api/excludes/sets", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if mc.savedCfg == nil {
		t.Fatal("expected config to be saved")
	}
	if len(mc.savedCfg.Xray.ExcludeSets) != 3 {
		t.Errorf("expected 3 exclude sets, got %d", len(mc.savedCfg.Xray.ExcludeSets))
	}
}

func TestHandleUpdateExcludeSets_EmptyList(t *testing.T) {
	mc := &mockConfig{
		cfg: &vpnconfig.VPNDirectorConfig{
			Xray: vpnconfig.XrayConfig{
				ExcludeSets: []string{"ru"},
			},
		},
	}
	deps := newTestDeps(t)
	deps.Config = mc

	handler := handleUpdateExcludeSets(deps)

	body := `{"sets": []}`
	req := httptest.NewRequest("POST", "/api/excludes/sets", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if mc.savedCfg == nil {
		t.Fatal("expected config to be saved")
	}
	if len(mc.savedCfg.Xray.ExcludeSets) != 0 {
		t.Errorf("expected 0 exclude sets, got %d", len(mc.savedCfg.Xray.ExcludeSets))
	}
}

func TestHandleUpdateExcludeSets_InvalidJSON(t *testing.T) {
	deps := newTestDeps(t)

	handler := handleUpdateExcludeSets(deps)

	req := httptest.NewRequest("POST", "/api/excludes/sets", strings.NewReader("not json"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleListExcludeIPs_OK(t *testing.T) {
	deps := newTestDeps(t)
	deps.Config = &mockConfig{
		cfg: &vpnconfig.VPNDirectorConfig{
			Xray: vpnconfig.XrayConfig{
				ExcludeIPs: []string{"1.2.3.4/24", "5.6.7.8"},
			},
		},
	}

	handler := handleListExcludeIPs(deps)

	req := httptest.NewRequest("GET", "/api/excludes/ips", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		IPs []string `json:"ips"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.IPs) != 2 {
		t.Errorf("expected 2 IPs, got %d", len(resp.IPs))
	}
}

func TestHandleListExcludeIPs_Error(t *testing.T) {
	deps := newTestDeps(t)
	deps.Config = &mockConfig{err: errors.New("load failed")}

	handler := handleListExcludeIPs(deps)

	req := httptest.NewRequest("GET", "/api/excludes/ips", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleAddExcludeIP_OK(t *testing.T) {
	mc := &mockConfig{
		cfg: &vpnconfig.VPNDirectorConfig{
			Xray: vpnconfig.XrayConfig{
				ExcludeIPs: []string{"1.2.3.4"},
			},
		},
	}
	deps := newTestDeps(t)
	deps.Config = mc

	handler := handleAddExcludeIP(deps)

	body := `{"ip": "5.6.7.8/24"}`
	req := httptest.NewRequest("POST", "/api/excludes/ips", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if mc.savedCfg == nil {
		t.Fatal("expected config to be saved")
	}
	if len(mc.savedCfg.Xray.ExcludeIPs) != 2 {
		t.Errorf("expected 2 exclude IPs, got %d", len(mc.savedCfg.Xray.ExcludeIPs))
	}
}

func TestHandleAddExcludeIP_Duplicate(t *testing.T) {
	mc := &mockConfig{
		cfg: &vpnconfig.VPNDirectorConfig{
			Xray: vpnconfig.XrayConfig{
				ExcludeIPs: []string{"1.2.3.4"},
			},
		},
	}
	deps := newTestDeps(t)
	deps.Config = mc

	handler := handleAddExcludeIP(deps)

	body := `{"ip": "1.2.3.4"}`
	req := httptest.NewRequest("POST", "/api/excludes/ips", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if mc.savedCfg == nil {
		t.Fatal("expected config to be saved")
	}
	if len(mc.savedCfg.Xray.ExcludeIPs) != 1 {
		t.Errorf("expected 1 exclude IP (no duplicate), got %d", len(mc.savedCfg.Xray.ExcludeIPs))
	}
}

func TestHandleAddExcludeIP_EmptyIP(t *testing.T) {
	deps := newTestDeps(t)

	handler := handleAddExcludeIP(deps)

	body := `{"ip": ""}`
	req := httptest.NewRequest("POST", "/api/excludes/ips", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDeleteExcludeIP_OK(t *testing.T) {
	mc := &mockConfig{
		cfg: &vpnconfig.VPNDirectorConfig{
			Xray: vpnconfig.XrayConfig{
				ExcludeIPs: []string{"1.2.3.4", "5.6.7.8"},
			},
		},
	}
	deps := newTestDeps(t)
	deps.Config = mc

	handler := handleDeleteExcludeIP(deps)

	req := httptest.NewRequest("DELETE", "/api/excludes/ips?ip=1.2.3.4", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if mc.savedCfg == nil {
		t.Fatal("expected config to be saved")
	}
	if len(mc.savedCfg.Xray.ExcludeIPs) != 1 || mc.savedCfg.Xray.ExcludeIPs[0] != "5.6.7.8" {
		t.Errorf("expected ExcludeIPs=[5.6.7.8], got %v", mc.savedCfg.Xray.ExcludeIPs)
	}
}

func TestHandleDeleteExcludeIP_MissingIP(t *testing.T) {
	deps := newTestDeps(t)

	handler := handleDeleteExcludeIP(deps)

	req := httptest.NewRequest("DELETE", "/api/excludes/ips", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleUpdateExcludeSets_LowercasesAndDedupes(t *testing.T) {
	mc := &mockConfig{cfg: &vpnconfig.VPNDirectorConfig{}}
	deps := newTestDeps(t)
	deps.Config = mc
	vpn := &mockVPN{}
	deps.VPN = vpn

	handler := handleUpdateExcludeSets(deps)

	body := `{"sets": ["RU", " ua ", "ru"]}`
	req := httptest.NewRequest("POST", "/api/excludes/sets", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := strings.Join(mc.savedCfg.Xray.ExcludeSets, ","); got != "ru,ua" {
		t.Errorf("expected [ru ua], got %q", got)
	}
	if vpn.applyCalls != 1 {
		t.Errorf("expected 1 Apply() call, got %d", vpn.applyCalls)
	}
}

func TestHandleUpdateExcludeSets_InvalidCode(t *testing.T) {
	mc := &mockConfig{cfg: &vpnconfig.VPNDirectorConfig{}}
	deps := newTestDeps(t)
	deps.Config = mc

	handler := handleUpdateExcludeSets(deps)

	body := `{"sets": ["ru", "xxx"]}`
	req := httptest.NewRequest("POST", "/api/excludes/sets", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != `invalid country code: "xxx"` {
		t.Errorf("unexpected error text: %q", resp["error"])
	}
	if mc.savedCfg != nil {
		t.Error("config must not be saved on validation error")
	}
}

func TestHandleUpdateExcludeSets_SavedButApplyFailed(t *testing.T) {
	mc := &mockConfig{cfg: &vpnconfig.VPNDirectorConfig{}}
	deps := newTestDeps(t)
	deps.Config = mc
	// Multi-line, like the real `vpn-director.sh apply` output: the response
	// must carry only the last line, the one the user needs to see.
	deps.VPN = &mockVPN{err: errors.New("apply failed (exit 1): [INFO] ensuring ipsets\n[ERROR] Invalid country code 'zz'\n")}

	handler := handleUpdateExcludeSets(deps)

	body := `{"sets": ["zz"]}`
	req := httptest.NewRequest("POST", "/api/excludes/sets", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["saved"] != true {
		t.Errorf("expected saved: true, got %v", resp["saved"])
	}
	if resp["error"] != "configuration saved, but apply failed: [ERROR] Invalid country code 'zz'" {
		t.Errorf("unexpected error text: %v", resp["error"])
	}
}

func TestHandleAddExcludeIP_NormalizesSlash32(t *testing.T) {
	mc := &mockConfig{cfg: &vpnconfig.VPNDirectorConfig{}}
	deps := newTestDeps(t)
	deps.Config = mc
	vpn := &mockVPN{}
	deps.VPN = vpn

	handler := handleAddExcludeIP(deps)

	body := `{"ip": "5.6.7.8/32"}`
	req := httptest.NewRequest("POST", "/api/excludes/ips", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := strings.Join(mc.savedCfg.Xray.ExcludeIPs, ","); got != "5.6.7.8" {
		t.Errorf("expected /32 stripped, got %q", got)
	}
	if vpn.applyCalls != 1 {
		t.Errorf("expected 1 Apply() call, got %d", vpn.applyCalls)
	}
}

func TestHandleAddExcludeIP_RejectsIPv6(t *testing.T) {
	deps := newTestDeps(t)
	deps.Config = &mockConfig{cfg: &vpnconfig.VPNDirectorConfig{}}

	handler := handleAddExcludeIP(deps)

	body := `{"ip": "2001:db8::/32"}`
	req := httptest.NewRequest("POST", "/api/excludes/ips", strings.NewReader(body))
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

func TestHandleDeleteExcludeIP_MatchesStoredForm(t *testing.T) {
	mc := &mockConfig{
		cfg: &vpnconfig.VPNDirectorConfig{
			Xray: vpnconfig.XrayConfig{ExcludeIPs: []string{"1.2.3.4/32", "5.6.7.8"}},
		},
	}
	deps := newTestDeps(t)
	deps.Config = mc
	vpn := &mockVPN{}
	deps.VPN = vpn

	handler := handleDeleteExcludeIP(deps)

	req := httptest.NewRequest("DELETE", "/api/excludes/ips?ip=1.2.3.4", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := strings.Join(mc.savedCfg.Xray.ExcludeIPs, ","); got != "5.6.7.8" {
		t.Errorf("expected [5.6.7.8], got %q", got)
	}
	if vpn.applyCalls != 1 {
		t.Errorf("expected 1 Apply() call, got %d", vpn.applyCalls)
	}
}

func TestNormalizeExcludeSets(t *testing.T) {
	got, err := normalizeExcludeSets([]string{"RU", " ua ", "ru", "By"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Join(got, ",") != "ru,ua,by" {
		t.Errorf("normalizeExcludeSets = %v, want [ru ua by]", got)
	}
	if got, err := normalizeExcludeSets(nil); err != nil || len(got) != 0 {
		t.Errorf("nil input: got %v, %v; want empty, nil", got, err)
	}
	for _, bad := range []string{"", "x", "xxx", "r1", "ru " + "\n" + "ua"} {
		if _, err := normalizeExcludeSets([]string{bad}); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}
