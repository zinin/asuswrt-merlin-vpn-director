package webapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/vpnconfig"
)

func TestSaveAndApply_OK(t *testing.T) {
	deps := newTestDeps(t)
	mc := &mockConfig{cfg: &vpnconfig.VPNDirectorConfig{}}
	deps.Config = mc
	vpn := &mockVPN{}
	deps.VPN = vpn

	rec := httptest.NewRecorder()
	writeSaveApplyResult(rec, saveAndApply(deps, mc.cfg))

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

func TestSaveAndApply_SaveError(t *testing.T) {
	deps := newTestDeps(t)
	deps.Config = &mockConfig{cfg: &vpnconfig.VPNDirectorConfig{}, saveVPNCfgErr: errors.New("disk full")}
	vpn := &mockVPN{}
	deps.VPN = vpn

	rec := httptest.NewRecorder()
	writeSaveApplyResult(rec, saveAndApply(deps, &vpnconfig.VPNDirectorConfig{}))

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

func TestSaveAndApply_ApplyError(t *testing.T) {
	deps := newTestDeps(t)
	mc := &mockConfig{cfg: &vpnconfig.VPNDirectorConfig{}}
	deps.Config = mc
	deps.VPN = &mockVPN{err: errors.New("apply failed (exit 1): [INFO] ensuring ipsets\n[ERROR] Invalid country code 'xx'\n")}

	rec := httptest.NewRecorder()
	writeSaveApplyResult(rec, saveAndApply(deps, mc.cfg))

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
