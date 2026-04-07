package vpnconfig

import (
	"encoding/json"
	"os"
	"sort"
)

type Server struct {
	Address string   `json:"address"`
	Port    int      `json:"port"`
	UUID    string   `json:"uuid"`
	Name    string   `json:"name"`
	IPs     []string `json:"ips"`
}

type WebUIConfig struct {
	Port      int    `json:"port,omitempty"`
	CertFile  string `json:"cert_file,omitempty"`
	KeyFile   string `json:"key_file,omitempty"`
	JWTSecret string `json:"jwt_secret,omitempty"`
}

type VPNDirectorConfig struct {
	DataDir        string                 `json:"data_dir"`
	WebUI          WebUIConfig            `json:"webui,omitempty"`
	PausedClients  []string               `json:"paused_clients,omitempty"`
	TunnelDirector TunnelDirectorConfig   `json:"tunnel_director"`
	Xray           XrayConfig             `json:"xray"`
	Advanced       map[string]interface{} `json:"advanced,omitempty"`
}

type TunnelConfig struct {
	Clients []string `json:"clients"`
	Exclude []string `json:"exclude"`
}

type TunnelDirectorConfig struct {
	// Note: Go's json.Marshal sorts map keys alphabetically, so tunnel order
	// in saved JSON is deterministic. This matches jq 'keys[]' behavior.
	// If insertion-order priority is needed, this should be changed to a slice.
	Tunnels map[string]TunnelConfig `json:"tunnels"`
}

type XrayConfig struct {
	Clients     []string `json:"clients"`
	Servers     []string `json:"servers"`
	ExcludeIPs  []string `json:"exclude_ips"`
	ExcludeSets []string `json:"exclude_sets"`
}

// ClientInfo represents a VPN client with its route and pause status.
type ClientInfo struct {
	IP     string
	Route  string
	Paused bool
}

// CollectClients builds a unified list of all clients from xray and tunnel_director sections.
func CollectClients(cfg *VPNDirectorConfig) []ClientInfo {
	paused := make(map[string]bool, len(cfg.PausedClients))
	for _, ip := range cfg.PausedClients {
		paused[ip] = true
	}

	var clients []ClientInfo

	for _, ip := range cfg.Xray.Clients {
		clients = append(clients, ClientInfo{
			IP:     ip,
			Route:  "xray",
			Paused: paused[ip],
		})
	}

	// Sort tunnel names for deterministic order
	tunnelNames := make([]string, 0, len(cfg.TunnelDirector.Tunnels))
	for name := range cfg.TunnelDirector.Tunnels {
		tunnelNames = append(tunnelNames, name)
	}
	sort.Strings(tunnelNames)

	for _, name := range tunnelNames {
		tunnel := cfg.TunnelDirector.Tunnels[name]
		for _, ip := range tunnel.Clients {
			clients = append(clients, ClientInfo{
				IP:     ip,
				Route:  name,
				Paused: paused[ip],
			})
		}
	}

	return clients
}

func LoadServers(path string) ([]Server, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var servers []Server
	return servers, json.Unmarshal(data, &servers)
}

func SaveServers(path string, servers []Server) error {
	data, err := json.MarshalIndent(servers, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}

func LoadVPNDirectorConfig(path string) (*VPNDirectorConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg VPNDirectorConfig
	return &cfg, json.Unmarshal(data, &cfg)
}

func SaveVPNDirectorConfig(path string, cfg *VPNDirectorConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}
