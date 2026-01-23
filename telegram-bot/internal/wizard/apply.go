package wizard

import (
	"fmt"
	"sort"
	"strings"

	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/service"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/telegram"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/vpnconfig"
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

	// Load current config
	vpnCfg, err := a.config.LoadVPNConfig()
	if err != nil {
		a.sender.SendPlain(chatID, fmt.Sprintf("Config load error: %v", err))
		return err
	}

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
			// Add /32 suffix if not present
			ip := c.IP
			if !strings.Contains(ip, "/") {
				ip = ip + "/32"
			}
			// Add client to tunnel
			if existing, ok := tunnels[c.Route]; ok {
				existing.Clients = append(existing.Clients, ip)
				tunnels[c.Route] = existing
			} else {
				tunnels[c.Route] = vpnconfig.TunnelConfig{
					Clients: []string{ip},
					Exclude: excl,
				}
			}
		}
	}

	// Server IPs (unique, non-empty, sorted)
	seen := make(map[string]bool)
	var serverIPs []string
	for _, s := range servers {
		if s.IP != "" && !seen[s.IP] {
			seen[s.IP] = true
			serverIPs = append(serverIPs, s.IP)
		}
	}
	sort.Strings(serverIPs)

	// Update config
	vpnCfg.Xray.Clients = xrayClients
	vpnCfg.Xray.ExcludeSets = excl
	vpnCfg.Xray.Servers = serverIPs
	vpnCfg.TunnelDirector.Tunnels = tunnels

	// Save config
	if err := a.config.SaveVPNConfig(vpnCfg); err != nil {
		a.sender.SendPlain(chatID, fmt.Sprintf("Save error: %v", err))
		return err
	}
	a.sender.SendPlain(chatID, "vpn-director.json updated")

	// Generate Xray config if server index is valid
	if serverIndex >= 0 && serverIndex < len(servers) {
		s := servers[serverIndex]
		if err := a.xray.GenerateConfig(s); err != nil {
			a.sender.SendPlain(chatID, fmt.Sprintf("Xray config generation error: %v", err))
			// Continue anyway - vpn-director.json is already saved
		} else {
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
