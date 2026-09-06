package webapi

import (
	"net/http"
)

// handleStatus returns a handler that reports the current VPN Director status.
func handleStatus(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		extendWriteDeadline(w, statusDeadline)

		output, err := deps.VPN.Status()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to get status")
			return
		}
		jsonOK(w, map[string]string{"output": output})
	}
}

// handleApply returns a handler that applies the VPN Director configuration.
func handleApply(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		unlock, ok := lockLongOp(w, r, deps, applyDeadline)
		if !ok {
			return
		}
		defer unlock()

		if err := deps.VPN.Apply(); err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to apply configuration: "+lastErrorLine(err))
			return
		}
		jsonOK(w, map[string]bool{"ok": true})
	}
}

// handleRestart returns a handler that restarts the VPN Director.
func handleRestart(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		unlock, ok := lockLongOp(w, r, deps, applyDeadline)
		if !ok {
			return
		}
		defer unlock()

		if err := deps.VPN.Restart(); err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to restart: "+lastErrorLine(err))
			return
		}
		jsonOK(w, map[string]bool{"ok": true})
	}
}

// handleStop returns a handler that stops the VPN Director.
func handleStop(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		unlock, ok := lockLongOp(w, r, deps, applyDeadline)
		if !ok {
			return
		}
		defer unlock()

		if err := deps.VPN.Stop(); err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to stop: "+lastErrorLine(err))
			return
		}
		jsonOK(w, map[string]bool{"ok": true})
	}
}

// handleUpdateIPsets returns a handler that downloads fresh ipsets and
// reapplies the configuration via `vpn-director.sh update`.
func handleUpdateIPsets(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		unlock, ok := lockLongOp(w, r, deps, updateDeadline)
		if !ok {
			return
		}
		defer unlock()

		if err := deps.VPN.Update(); err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to update ipsets: "+lastErrorLine(err))
			return
		}
		jsonOK(w, map[string]bool{"ok": true})
	}
}

// handleIP returns a handler that reports the router's external IP address.
func handleIP(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		extendWriteDeadline(w, ipDeadline)

		ip, err := deps.Network.GetExternalIP()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to get external IP")
			return
		}
		jsonOK(w, map[string]string{"ip": ip})
	}
}

// handleVersion returns a handler that reports the build version and commit.
func handleVersion(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		jsonOK(w, map[string]string{
			"version": deps.Version,
			"commit":  deps.Commit,
		})
	}
}
