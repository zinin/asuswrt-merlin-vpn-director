package webapi

import (
	"fmt"
	"net/http"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/vpnconfig"
)

// validRoutes is the set of allowed route names for client assignment.
var validRoutes = map[string]bool{
	"xray":   true,
	"wgc1":   true,
	"wgc2":   true,
	"wgc3":   true,
	"wgc4":   true,
	"wgc5":   true,
	"ovpnc1": true,
	"ovpnc2": true,
	"ovpnc3": true,
	"ovpnc4": true,
	"ovpnc5": true,
}

// handleListClients returns a handler that lists all VPN clients with their
// route assignment and pause status.
func handleListClients(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		cfg, err := deps.Config.LoadVPNConfig()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to load configuration")
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

// handleAddClient returns a handler that adds a client address to the
// specified route and applies the configuration. The address is normalized
// (IPv4 only, /32 stripped), must not already be configured in any route,
// and a newly created tunnel inherits xray.exclude_sets like the bot wizard.
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
		ip, err := vpnconfig.NormalizeClientAddr(req.IP)
		if err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		if req.Route == "" {
			jsonError(w, http.StatusBadRequest, "route is required")
			return
		}
		if !validRoutes[req.Route] {
			jsonError(w, http.StatusBadRequest, "invalid route: must be one of xray, wgc1-wgc5, ovpnc1-ovpnc5")
			return
		}

		unlock := lockLongOp(w, deps, applyDeadline)
		defer unlock()

		writeSaveApplyResult(w, updateAndApply(deps, func(cfg *vpnconfig.VPNDirectorConfig) error {
			if existing, found := findClient(cfg, ip); found {
				return &httpError{status: http.StatusConflict, msg: fmt.Sprintf("client already configured for %s", existing.route)}
			}
			if req.Route == "xray" {
				cfg.Xray.Clients = append(cfg.Xray.Clients, ip)
				return nil
			}
			if cfg.TunnelDirector.Tunnels == nil {
				cfg.TunnelDirector.Tunnels = make(map[string]vpnconfig.TunnelConfig)
			}
			tunnel, ok := cfg.TunnelDirector.Tunnels[req.Route]
			if !ok {
				// A new tunnel inherits the Xray country exclusions, like the
				// bot's configure wizard. An empty exclude would route the
				// client's local-country traffic through the tunnel as well.
				tunnel = vpnconfig.TunnelConfig{
					Clients: []string{},
					Exclude: append([]string{}, cfg.Xray.ExcludeSets...),
				}
			}
			tunnel.Clients = append(tunnel.Clients, ip)
			cfg.TunnelDirector.Tunnels[req.Route] = tunnel
			return nil
		}))
	}
}

// handlePauseClient returns a handler that pauses a configured client.
// The paused entry keeps the stored spelling of the address because the
// shell subtracts paused_clients from the clients arrays by exact string.
func handlePauseClient(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip, ok := clientAddrFromQuery(w, r)
		if !ok {
			return
		}

		unlock := lockLongOp(w, deps, applyDeadline)
		defer unlock()

		writeSaveApplyResult(w, updateAndApply(deps, func(cfg *vpnconfig.VPNDirectorConfig) error {
			existing, found := findClient(cfg, ip)
			if !found {
				return &httpError{status: http.StatusNotFound, msg: "client not found"}
			}
			// Drop any equivalent spelling and write the stored one in its place.
			// lib/config.sh subtracts paused_clients from the clients arrays by
			// exact string, and CollectClients reports Paused by exact lookup, so
			// an entry spelled differently from the client pauses nothing while
			// reporting success. Replacing rather than skipping keeps this
			// idempotent and repairs a mismatched legacy entry on the first pause.
			cfg.PausedClients = append(removeAddr(cfg.PausedClients, ip), existing.stored)
			return nil
		}))
	}
}

// handleResumeClient returns a handler that resumes a paused client.
func handleResumeClient(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip, ok := clientAddrFromQuery(w, r)
		if !ok {
			return
		}

		unlock := lockLongOp(w, deps, applyDeadline)
		defer unlock()

		writeSaveApplyResult(w, updateAndApply(deps, func(cfg *vpnconfig.VPNDirectorConfig) error {
			if _, found := findClient(cfg, ip); !found {
				return &httpError{status: http.StatusNotFound, msg: "client not found"}
			}
			cfg.PausedClients = removeAddr(cfg.PausedClients, ip)
			return nil
		}))
	}
}

// handleDeleteClient returns a handler that removes a client from all routes
// and from the paused list, matching every stored spelling of the address.
func handleDeleteClient(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip, ok := clientAddrFromQuery(w, r)
		if !ok {
			return
		}

		unlock := lockLongOp(w, deps, applyDeadline)
		defer unlock()

		writeSaveApplyResult(w, updateAndApply(deps, func(cfg *vpnconfig.VPNDirectorConfig) error {
			if _, found := findClient(cfg, ip); !found {
				return &httpError{status: http.StatusNotFound, msg: "client not found"}
			}
			cfg.Xray.Clients = removeAddr(cfg.Xray.Clients, ip)
			for name, tunnel := range cfg.TunnelDirector.Tunnels {
				tunnel.Clients = removeAddr(tunnel.Clients, ip)
				cfg.TunnelDirector.Tunnels[name] = tunnel
			}
			cfg.PausedClients = removeAddr(cfg.PausedClients, ip)
			return nil
		}))
	}
}

// clientMatch describes a configured client found by normalized address.
type clientMatch struct {
	route  string // "xray" or the tunnel name
	stored string // the address exactly as stored in vpn-director.json
}

// findClient looks addr up across xray.clients and every tunnel, comparing
// normalized forms because older configs store both 1.2.3.4 and 1.2.3.4/32.
func findClient(cfg *vpnconfig.VPNDirectorConfig, addr string) (clientMatch, bool) {
	for _, c := range vpnconfig.CollectClients(cfg) {
		if sameAddr(c.IP, addr) {
			return clientMatch{route: c.Route, stored: c.IP}, true
		}
	}
	return clientMatch{}, false
}

// clientAddrFromQuery reads and normalizes the ip query parameter. It writes
// the error response itself and returns ok=false when the handler must stop.
func clientAddrFromQuery(w http.ResponseWriter, r *http.Request) (string, bool) {
	raw := r.URL.Query().Get("ip")
	if raw == "" {
		jsonError(w, http.StatusBadRequest, "ip query parameter is required")
		return "", false
	}
	ip, err := vpnconfig.NormalizeClientAddr(raw)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return "", false
	}
	return ip, true
}

// sameAddr reports whether stored denotes the same address as the normalized
// addr. Entries that fail normalization are compared verbatim.
func sameAddr(stored, addr string) bool {
	normalized, err := vpnconfig.NormalizeClientAddr(stored)
	if err != nil {
		normalized = stored
	}
	return normalized == addr
}

// containsAddr reports whether slice holds addr in any stored spelling.
func containsAddr(slice []string, addr string) bool {
	for _, s := range slice {
		if sameAddr(s, addr) {
			return true
		}
	}
	return false
}

// removeAddr returns a new slice without every entry that denotes addr,
// whatever its stored spelling.
func removeAddr(slice []string, addr string) []string {
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if !sameAddr(s, addr) {
			result = append(result, s)
		}
	}
	return result
}
