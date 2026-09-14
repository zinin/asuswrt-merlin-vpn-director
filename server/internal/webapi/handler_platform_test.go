package webapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zinin/vpn-director/server/internal/vpnconfig"
)

func TestHandlePlatform_OK(t *testing.T) {
	deps := newTestDeps(t)
	deps.VPN = &mockVPN{platform: vpnconfig.PlatformInfo{
		Platform: "keenetic", Arch: "aarch64", PasswordFile: "/opt/etc/passwd",
		LANIfaces: []string{"br0"}, WANIf: "eth2.4",
		Tunnels: []vpnconfig.PlatformTunnel{{ID: "OpenVPN0", Iface: "ovpn_br0", Type: "openvpn", Connected: true, Description: "office-ovpn"}},
	}}
	rec := httptest.NewRecorder()
	handlePlatform(deps).ServeHTTP(rec, httptest.NewRequest("GET", "/api/platform", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got vpnconfig.PlatformInfo
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Platform != "keenetic" || len(got.Tunnels) != 1 || got.Tunnels[0].ID != "OpenVPN0" {
		t.Errorf("body = %+v", got)
	}
}

func TestHandlePlatform_Unavailable(t *testing.T) {
	deps := newTestDeps(t)
	deps.VPN = &mockVPN{platformErr: errors.New("command timed out after 30s")}
	rec := httptest.NewRecorder()
	handlePlatform(deps).ServeHTTP(rec, httptest.NewRequest("GET", "/api/platform", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "platform info unavailable") {
		t.Errorf("body = %s", rec.Body.String())
	}
}

func TestRouter_ServesPlatformBehindAuth(t *testing.T) {
	deps := newTestDeps(t)
	router := NewRouter(deps, nil)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("GET", "/api/platform", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("without a token: status = %d, want 401", rec.Code)
	}

	req := httptest.NewRequest("GET", "/api/platform", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: newTestToken(t, deps)})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("with a token: status = %d: %s", rec.Code, rec.Body.String())
	}
}
