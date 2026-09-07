package wizard

import (
	"errors"
	"fmt"
	"log/slog"
	"sort"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/service"
	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/telegram"
	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/vpnconfig"
)

// StateClearer is the interface for clearing wizard state
type StateClearer interface {
	Clear(chatID int64)
}

// Applier applies wizard configuration to the system
type Applier struct {
	manager StateClearer
	sender  telegram.MessageSender
	config  service.ConfigStore
	vpn     service.VPNDirector
	xray    service.XrayGenerator
}

// NewApplier creates a new Applier
func NewApplier(
	manager StateClearer,
	sender telegram.MessageSender,
	config service.ConfigStore,
	vpn service.VPNDirector,
	xray service.XrayGenerator,
) *Applier {
	return &Applier{
		manager: manager,
		sender:  sender,
		config:  config,
		vpn:     vpn,
		xray:    xray,
	}
}

// Apply applies the wizard configuration.
// IMPORTANT: State is ALWAYS cleared, even on error.
// This is intentional: router apply failures are usually config issues
// that require user to reconsider settings, not just retry.
func (a *Applier) Apply(chatID int64, state *State) error {
	// Always clear state at the end, regardless of success or failure
	defer a.manager.Clear(chatID)

	a.sender.SendPlain(chatID, "Applying configuration...")

	// Load servers
	servers, err := a.config.LoadServers()
	if err != nil {
		a.sender.SendPlain(chatID, fmt.Sprintf("Server load error: %v", err))
		return err
	}

	// Get state data with thread-safe getters
	clients := state.GetClients()
	exclusions := state.GetExclusions()
	serverIndex := state.GetServerIndex()

	// Build exclusion list (sorted for deterministic config)
	var excl []string
	for k, v := range exclusions {
		if v {
			excl = append(excl, k)
		}
	}
	sort.Strings(excl)
	if len(excl) == 0 {
		excl = []string{"ru"}
	}

	// Build valid routes set for validation
	validRoutes := make(map[string]bool)
	for _, r := range RouteOptions {
		validRoutes[r] = true
	}

	// Build new configuration
	var xrayClients []string
	tunnels := make(map[string]vpnconfig.TunnelConfig)

	for _, c := range clients {
		// Skip clients with invalid routes
		if !validRoutes[c.Route] {
			continue
		}
		if c.Route == "xray" {
			xrayClients = append(xrayClients, c.IP)
		} else {
			// Store the address as entered, with no /32 appended. The Web UI
			// stores what vpnconfig.NormalizeClientAddr returns, which strips
			// /32, and lib/config.sh subtracts paused_clients by exact string:
			// a second spelling here orphans entries paused from the Web UI.
			if existing, ok := tunnels[c.Route]; ok {
				existing.Clients = append(existing.Clients, c.IP)
				tunnels[c.Route] = existing
			} else {
				tunnels[c.Route] = vpnconfig.TunnelConfig{
					Clients: []string{c.IP},
					Exclude: excl,
				}
			}
		}
	}

	// Server IPs (unique, non-empty, sorted) — collect ALL IPs from ALL servers
	seen := make(map[string]bool)
	var serverIPs []string
	for _, s := range servers {
		for _, ip := range s.IPs {
			if ip != "" && !seen[ip] {
				seen[ip] = true
				serverIPs = append(serverIPs, ip)
			}
		}
	}
	sort.Strings(serverIPs)

	// Exclude IPs from wizard state
	excludeIPs := state.GetExcludeIPs()

	// Update config under the cross-process lock
	var ports service.InboundPorts
	err = a.config.UpdateVPNConfig(func(vpnCfg *vpnconfig.VPNDirectorConfig) error {
		vpnCfg.Xray.Clients = xrayClients
		vpnCfg.Xray.ExcludeSets = excl
		vpnCfg.Xray.ExcludeIPs = excludeIPs
		vpnCfg.Xray.Servers = serverIPs
		vpnCfg.TunnelDirector.Tunnels = tunnels
		// The wizard stores addresses as entered, so a client the old wizard
		// wrote as 1.2.3.4/32 comes back as 1.2.3.4. paused_clients is matched
		// literally, so its entry has to follow or the client resumes on its
		// own. Runs after the clients are in place, on the new spellings.
		vpnCfg.PausedClients = vpnconfig.RepointPausedClients(
			vpnCfg.PausedClients, vpnconfig.CollectClients(vpnCfg))
		// The generated inbound has to listen where the TPROXY rules send
		// traffic, so read the ports while the config is in hand.
		ports.TProxy, ports.Socks = vpnconfig.XrayInboundPorts(vpnCfg)
		return nil
	})
	if err != nil {
		if errors.Is(err, service.ErrConfigLoad) {
			a.sender.SendPlain(chatID, fmt.Sprintf("Config load error: %v", err))
		} else {
			a.sender.SendPlain(chatID, fmt.Sprintf("Save error: %v", err))
		}
		return err
	}
	a.sender.SendPlain(chatID, "vpn-director.json updated")

	// Generate Xray config if server index is valid
	if serverIndex >= 0 && serverIndex < len(servers) {
		s := servers[serverIndex]
		// Generation and the record of it under one lock, so a switch from the
		// Web UI or /xray cannot land between them; the record follows only a
		// generation that succeeded, since a failure leaves the previous
		// config.json running and that is the server still to be named.
		generated, err := service.GenerateAndRecordActiveServer(a.config, a.xray, s, ports)
		if !generated {
			a.sender.SendPlain(chatID, fmt.Sprintf("Xray config generation error: %v", err))
			// Continue anyway - vpn-director.json is already saved
		} else {
			if err != nil {
				// config.json is written; only the record of it is missing.
				slog.Warn("Failed to record the active server", "server", s.Name, "error", err)
			}
			a.sender.SendPlain(chatID, "xray/config.json updated")
		}
	} else {
		a.sender.SendPlain(chatID, "Warning: Invalid server selection, Xray config not updated")
	}

	// Apply configuration via vpn-director
	if err := a.vpn.Apply(); err != nil {
		a.sender.SendPlain(chatID, fmt.Sprintf("vpn-director apply error: %v", err))
		return err
	}
	a.sender.SendPlain(chatID, "VPN Director applied")

	// Restart Xray to apply new config
	if err := a.vpn.RestartXray(); err != nil {
		a.sender.SendPlain(chatID, fmt.Sprintf("Xray restart error: %v", err))
		return err
	}
	a.sender.SendPlain(chatID, "Xray restarted")

	a.sender.SendPlain(chatID, "Done!")
	return nil
}
