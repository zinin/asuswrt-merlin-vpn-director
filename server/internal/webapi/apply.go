package webapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/vpnconfig"
)

// errSavedNotApplied marks a saveAndApply failure where vpn-director.json was
// written but `vpn-director.sh apply` failed. The client must learn that the
// change is on disk and only the apply needs a retry.
type errSavedNotApplied struct{ cause error }

func (e *errSavedNotApplied) Error() string { return "saved but not applied: " + e.cause.Error() }
func (e *errSavedNotApplied) Unwrap() error { return e.cause }

// saveAndApply persists cfg and applies it, mirroring the bot which runs
// `vpn-director.sh apply` after every config mutation so the config on disk
// always matches the kernel state. A save failure is returned as-is; an
// apply failure is wrapped in *errSavedNotApplied.
func saveAndApply(deps *Deps, cfg *vpnconfig.VPNDirectorConfig) error {
	if err := deps.Config.SaveVPNConfig(cfg); err != nil {
		return err
	}
	if err := deps.VPN.Apply(); err != nil {
		return &errSavedNotApplied{cause: err}
	}
	return nil
}

// writeSaveApplyResult maps a saveAndApply result to the HTTP response:
// 200 {"ok":true}; 500 "failed to save configuration"; or 500 with
// "saved": true and the last line of the apply output when only the apply
// failed, so the UI can refresh the list and offer a retry.
func writeSaveApplyResult(w http.ResponseWriter, err error) {
	if err == nil {
		jsonOK(w, map[string]bool{"ok": true})
		return
	}
	var notApplied *errSavedNotApplied
	if errors.As(err, &notApplied) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": fmt.Sprintf("configuration saved, but apply failed: %s", lastErrorLine(notApplied.cause)),
			"saved": true,
		})
		return
	}
	jsonError(w, http.StatusInternalServerError, "failed to save configuration")
}
