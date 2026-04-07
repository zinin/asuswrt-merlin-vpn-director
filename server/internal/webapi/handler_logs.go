package webapi

import (
	"net/http"
	"strconv"
)

// logPaths maps log source names to their file paths on disk.
var logPaths = map[string]string{
	"vpn":  "/tmp/vpn-director.log",
	"xray": "/tmp/xray-access.log",
	"bot":  "/tmp/telegram-bot.log",
}

// handleLogs returns a handler that reads log files.
// Query params: source (vpn|xray|bot), lines (default 50, max 500).
// If source is specified, returns that single log. Otherwise returns all.
func handleLogs(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		source := r.URL.Query().Get("source")
		linesStr := r.URL.Query().Get("lines")

		lines := 50
		if linesStr != "" {
			n, err := strconv.Atoi(linesStr)
			if err != nil || n < 1 {
				jsonError(w, http.StatusBadRequest, "lines must be a positive integer")
				return
			}
			if n > 500 {
				n = 500
			}
			lines = n
		}

		if source != "" {
			path, ok := logPaths[source]
			if !ok {
				jsonError(w, http.StatusBadRequest, "unknown source: valid values are vpn, xray, bot")
				return
			}

			output, err := deps.Logs.Read(path, lines)
			if err != nil {
				jsonError(w, http.StatusInternalServerError, "failed to read log file")
				return
			}

			jsonOK(w, map[string]string{"output": output, "source": source})
			return
		}

		// No source specified: return all logs.
		result := make(map[string]string, len(logPaths))
		for name, path := range logPaths {
			output, err := deps.Logs.Read(path, lines)
			if err != nil {
				result[name] = "error: " + err.Error()
			} else {
				result[name] = output
			}
		}

		jsonOK(w, result)
	}
}

// handleConfig returns a handler that returns the VPN Director configuration
// with sensitive fields redacted.
func handleConfig(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		cfg, err := deps.Config.LoadVPNConfig()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to load configuration")
			return
		}

		// Make a shallow copy to avoid mutating the original.
		redacted := *cfg
		redacted.WebUI.JWTSecret = ""

		jsonOK(w, &redacted)
	}
}

// handleUpdate returns a handler for the self-update endpoint.
// Currently returns a not-supported message.
func handleUpdate(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		jsonOK(w, map[string]interface{}{"ok": false, "error": "self-update via web UI not yet supported"})
	}
}
