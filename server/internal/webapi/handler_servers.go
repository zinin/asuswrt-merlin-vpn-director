package webapi

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"time"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/vless"
	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/vpnconfig"
)

// handleListServers returns a handler that lists all imported servers.
func handleListServers(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		servers, err := deps.Config.LoadServers()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonOK(w, map[string]interface{}{"servers": servers})
	}
}

// selectServerRequest is the expected JSON body for POST /api/servers/active.
type selectServerRequest struct {
	Index int `json:"index"`
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

		servers, err := deps.Config.LoadServers()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, fmt.Sprintf("load servers: %s", err))
			return
		}

		if req.Index < 0 || req.Index >= len(servers) {
			jsonError(w, http.StatusBadRequest, fmt.Sprintf("index out of range: %d (have %d servers)", req.Index, len(servers)))
			return
		}

		deps.OpMutex.Lock()
		defer deps.OpMutex.Unlock()

		server := servers[req.Index]

		if err := deps.Xray.GenerateConfig(server); err != nil {
			jsonError(w, http.StatusInternalServerError, fmt.Sprintf("generate xray config: %s", err))
			return
		}

		cfg, err := deps.Config.LoadVPNConfig()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, fmt.Sprintf("load vpn config: %s", err))
			return
		}

		cfg.Xray.Servers = server.IPs
		if err := deps.Config.SaveVPNConfig(cfg); err != nil {
			jsonError(w, http.StatusInternalServerError, fmt.Sprintf("save vpn config: %s", err))
			return
		}

		if err := deps.VPN.RestartXray(); err != nil {
			jsonError(w, http.StatusInternalServerError, fmt.Sprintf("restart xray: %s", err))
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

		// SSRF protection: resolve host and check for private IPs.
		host := parsed.Hostname()
		if isPrivateHost(host) {
			jsonError(w, http.StatusBadRequest, "URL must not point to private or loopback addresses")
			return
		}

		// Fetch the subscription.
		client := &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
			},
		}

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
			resolved = append(resolved, vpnconfig.Server{
				Address: s.Address,
				Port:    s.Port,
				UUID:    s.UUID,
				Name:    s.Name,
				IPs:     s.IPs,
			})
		}

		if len(resolved) == 0 {
			jsonError(w, http.StatusBadRequest, "could not resolve IP for any server")
			return
		}

		if err := deps.Config.SaveServers(resolved); err != nil {
			jsonError(w, http.StatusInternalServerError, fmt.Sprintf("save servers: %s", err))
			return
		}

		// Sync xray.servers with all imported server IPs.
		if vpnCfg, err := deps.Config.LoadVPNConfig(); err == nil && vpnCfg != nil {
			seen := make(map[string]bool)
			var serverIPs []string
			for _, s := range resolved {
				for _, ip := range s.IPs {
					if ip != "" && !seen[ip] {
						seen[ip] = true
						serverIPs = append(serverIPs, ip)
					}
				}
			}
			sort.Strings(serverIPs)
			vpnCfg.Xray.Servers = serverIPs
			_ = deps.Config.SaveVPNConfig(vpnCfg)
		}

		jsonOK(w, map[string]interface{}{"ok": true, "count": len(resolved)})
	}
}

// isPrivateHost checks if a hostname resolves to a private or loopback IP.
func isPrivateHost(host string) bool {
	// First check if it's a raw IP.
	if ip := net.ParseIP(host); ip != nil {
		return isPrivateIP(ip)
	}

	// Resolve hostname.
	ips, err := net.LookupIP(host)
	if err != nil {
		// If we can't resolve, block it to be safe.
		return true
	}

	for _, ip := range ips {
		if isPrivateIP(ip) {
			return true
		}
	}
	return false
}

// isPrivateIP returns true if the IP is private, loopback, or link-local.
func isPrivateIP(ip net.IP) bool {
	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"::1/128",
		"fc00::/7",
		"fe80::/10",
	}

	for _, cidr := range privateRanges {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
