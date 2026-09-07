package webapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/service"
	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/vpnconfig"
)

// errSavedNotApplied marks an updateAndApply failure where vpn-director.json
// was written but `vpn-director.sh apply` failed. The client must learn that
// the change is on disk and only the apply needs a retry.
type errSavedNotApplied struct{ cause error }

func (e *errSavedNotApplied) Error() string { return "saved but not applied: " + e.cause.Error() }
func (e *errSavedNotApplied) Unwrap() error { return e.cause }

// httpError carries a client-facing status and message out of an
// UpdateVPNConfig callback, so a handler can reject a change (409, 404)
// from inside the locked section and writeSaveApplyResult answers with it.
type httpError struct {
	status int
	msg    string
}

func (e *httpError) Error() string { return e.msg }

// updateAndApply mutates vpn-director.json under the config lock and then
// runs `vpn-director.sh apply`, mirroring the bot, so the config on disk
// always matches the kernel state. Errors from UpdateVPNConfig (load, lock,
// mutate, save) are returned as is; an apply failure is wrapped in
// *errSavedNotApplied.
func updateAndApply(deps *Deps, mutate func(cfg *vpnconfig.VPNDirectorConfig) error) error {
	if err := deps.Config.UpdateVPNConfig(mutate); err != nil {
		return err
	}
	if err := deps.VPN.Apply(); err != nil {
		return &errSavedNotApplied{cause: err}
	}
	return nil
}

// writeSaveApplyResult maps an updateAndApply result to the HTTP response:
// 200 {"ok":true}; the status and text of an *httpError returned by mutate;
// 500 "failed to load configuration" when vpn-director.json could not be
// read; 500 with "saved": true and the last line of the apply output when
// only the apply failed; otherwise 500 "failed to save configuration" with
// the cause (a lock timeout, a full disk) in the log. Only pass it the
// result of updateAndApply: answer validation errors with jsonError directly.
func writeSaveApplyResult(w http.ResponseWriter, err error) {
	if err == nil {
		jsonOK(w, map[string]bool{"ok": true})
		return
	}
	var he *httpError
	if errors.As(err, &he) {
		jsonError(w, he.status, he.msg)
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
	if errors.Is(err, service.ErrConfigLoad) {
		jsonError(w, http.StatusInternalServerError, "failed to load configuration")
		return
	}
	if errors.Is(err, service.ErrConfigLockTimeout) {
		jsonError(w, http.StatusServiceUnavailable, "configuration is busy, try again")
		return
	}
	slog.Warn("configuration update failed", "error", err)
	jsonError(w, http.StatusInternalServerError, "failed to save configuration")
}
