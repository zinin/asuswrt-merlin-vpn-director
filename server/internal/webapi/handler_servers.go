package webapi

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"time"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/service"
	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/ssrf"
	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/vless"
	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/vpnconfig"
)

// handleListServers returns a handler that lists all imported servers.
func handleListServers(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		servers, err := deps.Config.LoadServers()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to load servers")
			return
		}
		jsonOK(w, map[string]interface{}{"servers": servers})
	}
}

// selectServerRequest is the expected JSON body for POST /api/servers/active.
type selectServerRequest struct {
	Index *int `json:"index"`
}

// handleSelectServer returns a handler that selects a server by index,
// generates Xray config, updates vpn-director.json, and restarts Xray.
func handleSelectServer(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req selectServerRequest
		if err := decodeJSON(r, &req); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if req.Index == nil {
			jsonError(w, http.StatusBadRequest, "index is required")
			return
		}

		servers, err := deps.Config.LoadServers()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to load servers")
			return
		}

		if *req.Index < 0 || *req.Index >= len(servers) {
			jsonError(w, http.StatusBadRequest, fmt.Sprintf("index out of range: %d (have %d servers)", *req.Index, len(servers)))
			return
		}

		deps.OpMutex.Lock()
		defer deps.OpMutex.Unlock()

		server := servers[*req.Index]

		if err := deps.Xray.GenerateConfig(server); err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to generate xray config")
			return
		}

		cfg, err := deps.Config.LoadVPNConfig()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to load vpn config")
			return
		}

		// Set xray.servers to ALL servers' IPs, not just the selected one
		// (parity with the import path). xray.servers feeds the TPROXY bypass
		// set; dropping the other endpoints on a switch can cause a routing loop.
		cfg.Xray.Servers = collectServerIPs(servers)
		if err := deps.Config.SaveVPNConfig(cfg); err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to save vpn config")
			return
		}

		if err := deps.VPN.RestartXray(); err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to restart xray")
			return
		}

		jsonOK(w, map[string]bool{"ok": true})
	}
}

// importServersRequest is the expected JSON body for POST /api/servers/import.
type importServersRequest struct {
	URL string `json:"url"`
}

// handleImportServers returns a handler that imports servers from a VLESS
// subscription URL. It enforces HTTPS-only and SSRF protections.
func handleImportServers(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req importServersRequest
		if err := decodeJSON(r, &req); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if req.URL == "" {
			jsonError(w, http.StatusBadRequest, "url is required")
			return
		}

		parsed, err := url.Parse(req.URL)
		if err != nil {
			jsonError(w, http.StatusBadRequest, "invalid URL")
			return
		}

		if parsed.Scheme != "https" {
			jsonError(w, http.StatusBadRequest, "only https URLs are allowed")
			return
		}

		// SSRF protection (pre-flight): reject obvious private/reserved hosts
		// early with a clear error. The dial-time guard in ssrf.NewClient is the
		// authoritative protection and also defeats DNS rebinding.
		host := parsed.Hostname()
		if ssrf.IsPrivateHost(host) {
			jsonError(w, http.StatusBadRequest, "URL must not point to private or loopback addresses")
			return
		}

		// Fetch the subscription with the SSRF-hardened client.
		client := ssrf.NewClient(10 * time.Second)

		resp, err := client.Get(req.URL)
		if err != nil {
			jsonError(w, http.StatusBadGateway, fmt.Sprintf("download failed: %s", err))
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			jsonError(w, http.StatusBadGateway, fmt.Sprintf("upstream returned HTTP %d", resp.StatusCode))
			return
		}

		const maxBody = 1 << 20 // 1MB
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
		if err != nil {
			jsonError(w, http.StatusBadGateway, fmt.Sprintf("read body: %s", err))
			return
		}

		// Decode VLESS subscription.
		vlessServers, _ := vless.DecodeSubscription(string(body))
		if len(vlessServers) == 0 {
			jsonError(w, http.StatusBadRequest, "no VLESS servers found in subscription")
			return
		}

		// Resolve IPs and convert to vpnconfig.Server.
		var resolved []vpnconfig.Server
		for _, s := range vlessServers {
			if err := s.ResolveIPs(); err != nil {
				continue
			}
			resolved = append(resolved, s.ToVPNConfig())
		}

		if len(resolved) == 0 {
			jsonError(w, http.StatusBadRequest, "could not resolve IP for any server")
			return
		}

		deps.OpMutex.Lock()
		defer deps.OpMutex.Unlock()

		if err := deps.Config.SaveServers(resolved); err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to save servers")
			return
		}

		// Sync xray.servers with all imported server IPs. Surface a persistence
		// failure instead of returning 200 with a stale xray.servers on disk.
		// servers.json is already saved here, so the message says so explicitly:
		// the import partially persisted (servers stored, xray.servers stale) and
		// the client must not read the 500 as "nothing changed".
		if err := syncXrayServers(deps.Config, resolved); err != nil {
			jsonError(w, http.StatusInternalServerError,
				fmt.Sprintf("servers saved, but xray.servers sync failed: %s", err))
			return
		}

		jsonOK(w, map[string]interface{}{"ok": true, "count": len(resolved)})
	}
}

// collectServerIPs returns the sorted, de-duplicated list of all non-empty IPs
// across the given servers. xray.servers feeds the TPROXY bypass set, so every
// configured server endpoint must be present (otherwise the proxy's own egress
// could be routed back through itself).
func collectServerIPs(servers []vpnconfig.Server) []string {
	seen := make(map[string]bool)
	ips := make([]string, 0) // non-nil so an empty result marshals to [] not null
	for _, s := range servers {
		for _, ip := range s.IPs {
			if ip != "" && !seen[ip] {
				seen[ip] = true
				ips = append(ips, ip)
			}
		}
	}
	sort.Strings(ips)
	return ips
}

// syncXrayServers updates xray.servers with the IPs of all given servers and
// persists the config. A load or save failure is returned so the caller can
// surface it instead of silently leaving xray.servers stale.
func syncXrayServers(config service.ConfigStore, servers []vpnconfig.Server) error {
	vpnCfg, err := config.LoadVPNConfig()
	if err != nil {
		return fmt.Errorf("load vpn config: %w", err)
	}
	if vpnCfg == nil {
		return nil
	}
	vpnCfg.Xray.Servers = collectServerIPs(servers)
	if err := config.SaveVPNConfig(vpnCfg); err != nil {
		return fmt.Errorf("save vpn config: %w", err)
	}
	return nil
}
