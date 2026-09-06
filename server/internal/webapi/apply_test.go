package webapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/service"
	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/vpnconfig"
)

func TestUpdateAndApply_OK(t *testing.T) {
	deps := newTestDeps(t)
	mc := &mockConfig{cfg: &vpnconfig.VPNDirectorConfig{}}
	deps.Config = mc
	vpn := &mockVPN{}
	deps.VPN = vpn

	rec := httptest.NewRecorder()
	writeSaveApplyResult(rec, updateAndApply(deps, func(*vpnconfig.VPNDirectorConfig) error { return nil }))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if mc.savedCfg == nil {
		t.Fatal("expected config to be saved")
	}
	if vpn.applyCalls != 1 {
		t.Errorf("expected 1 Apply() call after save, got %d", vpn.applyCalls)
	}
	var resp map[string]bool
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp["ok"] {
		t.Error("expected ok: true")
	}
}

func TestUpdateAndApply_SaveError(t *testing.T) {
	deps := newTestDeps(t)
	deps.Config = &mockConfig{cfg: &vpnconfig.VPNDirectorConfig{}, saveVPNCfgErr: errors.New("disk full")}
	vpn := &mockVPN{}
	deps.VPN = vpn

	rec := httptest.NewRecorder()
	writeSaveApplyResult(rec, updateAndApply(deps, func(*vpnconfig.VPNDirectorConfig) error { return nil }))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
	if vpn.applyCalls != 0 {
		t.Errorf("Apply() must not run when save failed, got %d calls", vpn.applyCalls)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "failed to save configuration" {
		t.Errorf("unexpected error text: %v", resp["error"])
	}
	if _, has := resp["saved"]; has {
		t.Error("saved must be absent when nothing was saved")
	}
}

func TestUpdateAndApply_ApplyError(t *testing.T) {
	deps := newTestDeps(t)
	mc := &mockConfig{cfg: &vpnconfig.VPNDirectorConfig{}}
	deps.Config = mc
	deps.VPN = &mockVPN{err: errors.New("apply failed (exit 1): [INFO] ensuring ipsets\n[ERROR] Invalid country code 'xx'\n")}

	rec := httptest.NewRecorder()
	writeSaveApplyResult(rec, updateAndApply(deps, func(*vpnconfig.VPNDirectorConfig) error { return nil }))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
	if mc.savedCfg == nil {
		t.Fatal("expected config to be saved before apply")
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["saved"] != true {
		t.Errorf("expected saved: true, got %v", resp["saved"])
	}
	want := "configuration saved, but apply failed: [ERROR] Invalid country code 'xx'"
	if resp["error"] != want {
		t.Errorf("error = %v, want %q", resp["error"], want)
	}
}

func TestUpdateAndApply_HTTPErrorFromMutateIsAnsweredAsIs(t *testing.T) {
	deps := newTestDeps(t)
	mc := &mockConfig{cfg: &vpnconfig.VPNDirectorConfig{}}
	deps.Config = mc
	vpn := &mockVPN{}
	deps.VPN = vpn

	rec := httptest.NewRecorder()
	writeSaveApplyResult(rec, updateAndApply(deps, func(*vpnconfig.VPNDirectorConfig) error {
		return &httpError{status: http.StatusConflict, msg: "client already configured for xray"}
	}))

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	if mc.savedCfg != nil {
		t.Error("a rejected mutation must not be saved")
	}
	if vpn.applyCalls != 0 {
		t.Errorf("Apply() must not run after a rejected mutation, got %d calls", vpn.applyCalls)
	}
	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "client already configured for xray" {
		t.Errorf("unexpected error text: %q", resp["error"])
	}
}

func TestUpdateAndApply_LoadError(t *testing.T) {
	deps := newTestDeps(t)
	deps.Config = &mockConfig{err: errors.New("read failed")}
	vpn := &mockVPN{}
	deps.VPN = vpn

	rec := httptest.NewRecorder()
	writeSaveApplyResult(rec, updateAndApply(deps, func(*vpnconfig.VPNDirectorConfig) error { return nil }))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	var resp map[string]interface{}
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp["error"] != "failed to load configuration" {
		t.Errorf("unexpected error text: %v", resp["error"])
	}
	if vpn.applyCalls != 0 {
		t.Errorf("Apply() must not run when the config could not be loaded, got %d calls", vpn.applyCalls)
	}
}

func TestUpdateAndApply_LockTimeoutIsASaveFailure(t *testing.T) {
	deps := newTestDeps(t)
	deps.Config = &mockConfig{cfg: &vpnconfig.VPNDirectorConfig{}, updateErr: service.ErrConfigLockTimeout}
	vpn := &mockVPN{}
	deps.VPN = vpn

	rec := httptest.NewRecorder()
	writeSaveApplyResult(rec, updateAndApply(deps, func(*vpnconfig.VPNDirectorConfig) error { return nil }))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
	var resp map[string]interface{}
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp["error"] != "configuration is busy, try again" {
		t.Errorf("unexpected error text: %v", resp["error"])
	}
	if _, has := resp["saved"]; has {
		t.Error("saved must be absent when nothing was written")
	}
	if vpn.applyCalls != 0 {
		t.Errorf("Apply() must not run after a lock timeout, got %d calls", vpn.applyCalls)
	}
}
