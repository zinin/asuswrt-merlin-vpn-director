package webapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/updateflow"
)

// handleUpdateCheck reports the latest release. force=1 bypasses the flow's
// 30-minute cache, at most once a minute. A dev build cannot compare versions,
// so it answers with the dev marker instead of an error the UI would have to
// explain.
func handleUpdateCheck(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		extendWriteDeadline(w, githubDeadline)

		res, err := deps.Update.Check(r.Context(), r.URL.Query().Get("force") == "1")
		if err == nil {
			jsonOK(w, res)
			return
		}
		if errors.Is(err, updateflow.ErrDevMode) || errors.Is(err, updateflow.ErrDevVersion) {
			jsonOK(w, map[string]interface{}{"update_available": false, "dev": true})
			return
		}
		var ghErr *updateflow.GitHubError
		if errors.As(err, &ghErr) {
			jsonError(w, http.StatusBadGateway, "failed to check for updates: "+ghErr.Err.Error())
			return
		}
		slog.Warn("update check failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "failed to check for updates")
	}
}

// handleUpdateStart launches the unified self-update. It answers 202 as soon
// as the download starts: the update script stops this process a few seconds
// later, so the client learns the outcome by polling /api/version.
func handleUpdateStart(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		extendWriteDeadline(w, githubDeadline)

		res, err := deps.Update.Start(r.Context(), "webui", 0, func(line string) {
			slog.Info("self-update", "status", line)
		})
		if err == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":   true,
				"from": res.From,
				"to":   res.To,
			})
			return
		}

		switch {
		case errors.Is(err, updateflow.ErrUpToDate):
			jsonOK(w, map[string]interface{}{"ok": true, "update_available": false})
		case errors.Is(err, updateflow.ErrDevMode), errors.Is(err, updateflow.ErrDevVersion):
			jsonError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, updateflow.ErrInProgress):
			jsonError(w, http.StatusConflict, err.Error())
		default:
			var ghErr *updateflow.GitHubError
			if errors.As(err, &ghErr) {
				jsonError(w, http.StatusBadGateway, "failed to check for updates: "+ghErr.Err.Error())
				return
			}
			slog.Warn("update start failed", "error", err)
			jsonError(w, http.StatusInternalServerError, "failed to start update: "+err.Error())
		}
	}
}

// handleUpdateStatus reports whether an update script is running, so a client
// that reconnected after a restart can tell "still updating" from "done".
func handleUpdateStatus(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		jsonOK(w, map[string]bool{"in_progress": deps.Update.InProgress()})
	}
}
