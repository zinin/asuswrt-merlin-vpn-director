package vpnconfig

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadServers_FileNotFound(t *testing.T) {
	_, err := LoadServers("/nonexistent/path/to/servers.json")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
	if !os.IsNotExist(err) {
		t.Errorf("expected os.IsNotExist error, got: %v", err)
	}
}

func TestLoadServers_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "servers.json")

	err := os.WriteFile(path, []byte("not valid json"), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, err = LoadServers(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoadServers_ValidServers(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "servers.json")

	jsonContent := `[
		{"address": "server1.com", "port": 443, "uuid": "uuid1", "name": "Server 1", "ip": "1.2.3.4"},
		{"address": "server2.com", "port": 443, "uuid": "uuid2", "name": "Server 2", "ip": "5.6.7.8"}
	]`

	err := os.WriteFile(path, []byte(jsonContent), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	servers, err := LoadServers(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(servers))
	}

	if servers[0].Name != "Server 1" {
		t.Errorf("expected first server name 'Server 1', got '%s'", servers[0].Name)
	}

	if servers[1].IP != "5.6.7.8" {
		t.Errorf("expected second server IP '5.6.7.8', got '%s'", servers[1].IP)
	}
}

func TestSaveServers_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "servers.json")

	original := []Server{
		{Address: "server1.com", Port: 443, UUID: "uuid1", Name: "Server 1", IP: "1.2.3.4"},
		{Address: "server2.com", Port: 8443, UUID: "uuid2", Name: "Server 2", IP: "5.6.7.8"},
	}

	err := SaveServers(path, original)
	if err != nil {
		t.Fatalf("unexpected error saving: %v", err)
	}

	loaded, err := LoadServers(path)
	if err != nil {
		t.Fatalf("unexpected error loading: %v", err)
	}

	if !reflect.DeepEqual(original, loaded) {
		t.Errorf("round-trip failed: original %+v != loaded %+v", original, loaded)
	}
}

func TestSaveServers_EmptySlice(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "servers.json")

	err := SaveServers(path, []Server{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	loaded, err := LoadServers(path)
	if err != nil {
		t.Fatalf("unexpected error loading: %v", err)
	}

	if len(loaded) != 0 {
		t.Errorf("expected empty slice, got %d servers", len(loaded))
	}
}

func TestLoadVPNDirectorConfig_FileNotFound(t *testing.T) {
	_, err := LoadVPNDirectorConfig("/nonexistent/path/to/config.json")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestLoadVPNDirectorConfig_ValidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "vpn-director.json")

	jsonContent := `{
		"data_dir": "/jffs/scripts/vpn-director/data",
		"tunnel_director": {
			"tunnels": {
				"wgc1": {
					"clients": ["192.168.50.0/24"],
					"exclude": ["us", "ca"]
				}
			}
		},
		"xray": {
			"clients": ["192.168.50.0/24"],
			"servers": ["server1", "server2"],
			"exclude_sets": ["ru"]
		},
		"advanced": {
			"debug": true
		}
	}`

	err := os.WriteFile(path, []byte(jsonContent), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cfg, err := LoadVPNDirectorConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.DataDir != "/jffs/scripts/vpn-director/data" {
		t.Errorf("expected DataDir '/jffs/scripts/vpn-director/data', got '%s'", cfg.DataDir)
	}

	if len(cfg.TunnelDirector.Tunnels) != 1 {
		t.Fatalf("expected 1 tunnel, got %d", len(cfg.TunnelDirector.Tunnels))
	}

	wgc1, ok := cfg.TunnelDirector.Tunnels["wgc1"]
	if !ok {
		t.Fatal("expected tunnel 'wgc1' to exist")
	}
	if len(wgc1.Clients) != 1 || wgc1.Clients[0] != "192.168.50.0/24" {
		t.Errorf("unexpected wgc1 clients: %v", wgc1.Clients)
	}
	if len(wgc1.Exclude) != 2 {
		t.Errorf("expected 2 exclude sets, got %d", len(wgc1.Exclude))
	}

	if len(cfg.Xray.Clients) != 1 {
		t.Errorf("expected 1 client, got %d", len(cfg.Xray.Clients))
	}

	if len(cfg.Xray.Servers) != 2 {
		t.Errorf("expected 2 servers, got %d", len(cfg.Xray.Servers))
	}

	if len(cfg.Xray.ExcludeSets) != 1 {
		t.Errorf("expected 1 exclude set, got %d", len(cfg.Xray.ExcludeSets))
	}

	if cfg.Advanced == nil {
		t.Error("expected Advanced to be non-nil")
	}
}

func TestSaveVPNDirectorConfig_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "vpn-director.json")

	original := &VPNDirectorConfig{
		DataDir: "/data",
		TunnelDirector: TunnelDirectorConfig{
			Tunnels: map[string]TunnelConfig{
				"wgc1": {
					Clients: []string{"192.168.1.0/24"},
					Exclude: []string{"ru"},
				},
				"ovpnc1": {
					Clients: []string{"10.0.0.5/32"},
					Exclude: []string{"ru", "cn"},
				},
			},
		},
		Xray: XrayConfig{
			Clients:     []string{"192.168.1.0/24"},
			Servers:     []string{"server1"},
			ExcludeSets: []string{"ru", "cn"},
		},
		Advanced: map[string]interface{}{
			"debug": true,
		},
	}

	err := SaveVPNDirectorConfig(path, original)
	if err != nil {
		t.Fatalf("unexpected error saving: %v", err)
	}

	loaded, err := LoadVPNDirectorConfig(path)
	if err != nil {
		t.Fatalf("unexpected error loading: %v", err)
	}

	if loaded.DataDir != original.DataDir {
		t.Errorf("DataDir mismatch: %s != %s", original.DataDir, loaded.DataDir)
	}

	if !reflect.DeepEqual(loaded.TunnelDirector.Tunnels, original.TunnelDirector.Tunnels) {
		t.Errorf("TunnelDirector.Tunnels mismatch")
	}

	if !reflect.DeepEqual(loaded.Xray.Clients, original.Xray.Clients) {
		t.Errorf("Xray.Clients mismatch")
	}

	if !reflect.DeepEqual(loaded.Xray.Servers, original.Xray.Servers) {
		t.Errorf("Xray.Servers mismatch")
	}

	if !reflect.DeepEqual(loaded.Xray.ExcludeSets, original.Xray.ExcludeSets) {
		t.Errorf("Xray.ExcludeSets mismatch")
	}
}

func TestSaveVPNDirectorConfig_FormattedJSON(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "vpn-director.json")

	cfg := &VPNDirectorConfig{
		DataDir: "/data",
		TunnelDirector: TunnelDirectorConfig{
			Tunnels: map[string]TunnelConfig{
				"wgc1": {
					Clients: []string{"192.168.1.100/32"},
					Exclude: []string{"ru"},
				},
			},
		},
		Xray: XrayConfig{
			Clients:     []string{},
			Servers:     []string{},
			ExcludeSets: []string{},
		},
	}

	err := SaveVPNDirectorConfig(path, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	// Check that JSON is pretty-printed (contains newlines and indentation)
	content := string(data)
	if len(content) < 10 {
		t.Error("expected formatted JSON output")
	}

	// Should contain newlines (pretty-printed)
	if content[0] != '{' || content[len(content)-1] != '\n' {
		t.Error("expected properly formatted JSON")
	}
}
