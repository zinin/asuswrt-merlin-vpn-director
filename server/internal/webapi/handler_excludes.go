package webapi

import (
	"net/http"
)

// handleListExcludeSets returns a handler that lists configured exclusion sets.
func handleListExcludeSets(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		cfg, err := deps.Config.LoadVPNConfig()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonOK(w, map[string]interface{}{"sets": cfg.Xray.ExcludeSets})
	}
}

// updateExcludeSetsRequest is the expected JSON body for POST /api/excludes/sets.
type updateExcludeSetsRequest struct {
	Sets []string `json:"sets"`
}

// handleUpdateExcludeSets returns a handler that replaces the exclusion sets list.
func handleUpdateExcludeSets(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req updateExcludeSetsRequest
		if err := decodeJSON(r, &req); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		cfg, err := deps.Config.LoadVPNConfig()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}

		cfg.Xray.ExcludeSets = req.Sets

		if err := deps.Config.SaveVPNConfig(cfg); err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}

		jsonOK(w, map[string]bool{"ok": true})
	}
}

// handleListExcludeIPs returns a handler that lists configured exclusion IPs.
func handleListExcludeIPs(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		cfg, err := deps.Config.LoadVPNConfig()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonOK(w, map[string]interface{}{"ips": cfg.Xray.ExcludeIPs})
	}
}

// addExcludeIPRequest is the expected JSON body for POST /api/excludes/ips.
type addExcludeIPRequest struct {
	IP string `json:"ip"`
}

// handleAddExcludeIP returns a handler that adds an IP/CIDR to the exclusion list.
func handleAddExcludeIP(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req addExcludeIPRequest
		if err := decodeJSON(r, &req); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if req.IP == "" {
			jsonError(w, http.StatusBadRequest, "ip is required")
			return
		}

		cfg, err := deps.Config.LoadVPNConfig()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}

		if !contains(cfg.Xray.ExcludeIPs, req.IP) {
			cfg.Xray.ExcludeIPs = append(cfg.Xray.ExcludeIPs, req.IP)
		}

		if err := deps.Config.SaveVPNConfig(cfg); err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}

		jsonOK(w, map[string]bool{"ok": true})
	}
}

// handleDeleteExcludeIP returns a handler that removes an IP/CIDR from the exclusion list.
func handleDeleteExcludeIP(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := r.URL.Query().Get("ip")
		if ip == "" {
			jsonError(w, http.StatusBadRequest, "ip query parameter is required")
			return
		}

		cfg, err := deps.Config.LoadVPNConfig()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}

		cfg.Xray.ExcludeIPs = removeString(cfg.Xray.ExcludeIPs, ip)

		if err := deps.Config.SaveVPNConfig(cfg); err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}

		jsonOK(w, map[string]bool{"ok": true})
	}
}
