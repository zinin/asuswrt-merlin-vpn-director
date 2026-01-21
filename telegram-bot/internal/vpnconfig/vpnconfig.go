package vpnconfig

import (
	"encoding/json"
	"os"
)

type Server struct {
	Address string `json:"address"`
	Port    int    `json:"port"`
	UUID    string `json:"uuid"`
	Name    string `json:"name"`
	IP      string `json:"ip"`
}

type VPNDirectorConfig struct {
	DataDir        string                 `json:"data_dir"`
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
	ExcludeSets []string `json:"exclude_sets"`
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
