package webapi

import (
	"net/http"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/vpnconfig"
)

// handleListClients returns a handler that lists all VPN clients with their
// route assignment and pause status.
func handleListClients(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		cfg, err := deps.Config.LoadVPNConfig()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		clients := vpnconfig.CollectClients(cfg)
		jsonOK(w, map[string]interface{}{"clients": clients})
	}
}

// addClientRequest is the expected JSON body for POST /api/clients.
type addClientRequest struct {
	IP    string `json:"ip"`
	Route string `json:"route"`
}

// handleAddClient returns a handler that adds a client IP to the specified route.
func handleAddClient(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req addClientRequest
		if err := decodeJSON(r, &req); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if req.IP == "" {
			jsonError(w, http.StatusBadRequest, "ip is required")
			return
		}
		if req.Route == "" {
			jsonError(w, http.StatusBadRequest, "route is required")
			return
		}

		cfg, err := deps.Config.LoadVPNConfig()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}

		if req.Route == "xray" {
			if !contains(cfg.Xray.Clients, req.IP) {
				cfg.Xray.Clients = append(cfg.Xray.Clients, req.IP)
			}
		} else {
			// Tunnel route (wgc1, ovpnc1, etc.)
			if cfg.TunnelDirector.Tunnels == nil {
				cfg.TunnelDirector.Tunnels = make(map[string]vpnconfig.TunnelConfig)
			}
			tunnel, ok := cfg.TunnelDirector.Tunnels[req.Route]
			if !ok {
				tunnel = vpnconfig.TunnelConfig{
					Clients: []string{},
					Exclude: []string{},
				}
			}
			if !contains(tunnel.Clients, req.IP) {
				tunnel.Clients = append(tunnel.Clients, req.IP)
			}
			cfg.TunnelDirector.Tunnels[req.Route] = tunnel
		}

		if err := deps.Config.SaveVPNConfig(cfg); err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}

		jsonOK(w, map[string]bool{"ok": true})
	}
}

// handlePauseClient returns a handler that pauses a client by IP.
func handlePauseClient(deps *Deps) http.HandlerFunc {
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

		if !contains(cfg.PausedClients, ip) {
			cfg.PausedClients = append(cfg.PausedClients, ip)
		}

		if err := deps.Config.SaveVPNConfig(cfg); err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}

		jsonOK(w, map[string]bool{"ok": true})
	}
}

// handleResumeClient returns a handler that resumes a paused client by IP.
func handleResumeClient(deps *Deps) http.HandlerFunc {
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

		cfg.PausedClients = removeString(cfg.PausedClients, ip)

		if err := deps.Config.SaveVPNConfig(cfg); err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}

		jsonOK(w, map[string]bool{"ok": true})
	}
}

// handleDeleteClient returns a handler that removes a client from all routes.
func handleDeleteClient(deps *Deps) http.HandlerFunc {
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

		// Remove from Xray clients.
		cfg.Xray.Clients = removeString(cfg.Xray.Clients, ip)

		// Remove from all tunnel clients (keep tunnel config even if empty).
		for name, tunnel := range cfg.TunnelDirector.Tunnels {
			tunnel.Clients = removeString(tunnel.Clients, ip)
			cfg.TunnelDirector.Tunnels[name] = tunnel
		}

		if err := deps.Config.SaveVPNConfig(cfg); err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}

		jsonOK(w, map[string]bool{"ok": true})
	}
}

// contains returns true if the slice contains the item.
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// removeString returns a new slice with all exact matches of item removed.
func removeString(slice []string, item string) []string {
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != item {
			result = append(result, s)
		}
	}
	return result
}
