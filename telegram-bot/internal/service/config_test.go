// internal/service/config_test.go
package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigService_DataDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Create minimal vpn-director.json
	configPath := filepath.Join(tmpDir, "vpn-director.json")
	os.WriteFile(configPath, []byte(`{"data_dir": "/custom/data"}`), 0644)

	svc := NewConfigService(tmpDir, filepath.Join(tmpDir, "data"))
	dataDir, err := svc.DataDir()
	if err != nil {
		t.Fatalf("DataDir() error: %v", err)
	}

	if dataDir != "/custom/data" {
		t.Errorf("expected /custom/data, got %s", dataDir)
	}
}

func TestConfigService_DataDir_Default(t *testing.T) {
	tmpDir := t.TempDir()

	// Create config without data_dir
	configPath := filepath.Join(tmpDir, "vpn-director.json")
	os.WriteFile(configPath, []byte(`{}`), 0644)

	svc := NewConfigService(tmpDir, filepath.Join(tmpDir, "data"))
	dataDir, err := svc.DataDir()
	if err != nil {
		t.Fatalf("DataDir() error: %v", err)
	}

	expected := filepath.Join(tmpDir, "data")
	if dataDir != expected {
		t.Errorf("expected %s, got %s", expected, dataDir)
	}
}

func TestConfigService_DataDir_Error(t *testing.T) {
	tmpDir := t.TempDir()
	// No config file - should return error

	svc := NewConfigService(tmpDir, filepath.Join(tmpDir, "data"))
	_, err := svc.DataDir()
	if err == nil {
		t.Error("expected error for missing config, got nil")
	}
}

func TestConfigService_SaveServers_CreatesDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Create config with data_dir pointing to non-existent directory
	dataDir := filepath.Join(tmpDir, "newdata")
	configPath := filepath.Join(tmpDir, "vpn-director.json")
	os.WriteFile(configPath, []byte(`{"data_dir": "`+dataDir+`"}`), 0644)

	svc := NewConfigService(tmpDir, filepath.Join(tmpDir, "data"))
	err := svc.SaveServers(nil)
	if err != nil {
		t.Fatalf("SaveServers() error: %v", err)
	}

	// Directory should exist now
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		t.Error("SaveServers should create data directory")
	}
}
