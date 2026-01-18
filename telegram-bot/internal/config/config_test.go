package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/to/config.json")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
	if !os.IsNotExist(err) {
		t.Errorf("expected os.IsNotExist error, got: %v", err)
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	err := os.WriteFile(configPath, []byte("not valid json"), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, err = Load(configPath)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoad_ValidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	jsonContent := `{
		"bot_token": "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		"allowed_users": ["user1", "user2", "user3"]
	}`

	err := os.WriteFile(configPath, []byte(jsonContent), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.BotToken != "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11" {
		t.Errorf("expected bot_token '123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11', got '%s'", cfg.BotToken)
	}

	if len(cfg.AllowedUsers) != 3 {
		t.Fatalf("expected 3 allowed_users, got %d", len(cfg.AllowedUsers))
	}

	expectedUsers := []string{"user1", "user2", "user3"}
	for i, user := range expectedUsers {
		if cfg.AllowedUsers[i] != user {
			t.Errorf("expected allowed_users[%d] = '%s', got '%s'", i, user, cfg.AllowedUsers[i])
		}
	}
}

func TestLoad_EmptyAllowedUsers(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	jsonContent := `{
		"bot_token": "test-token",
		"allowed_users": []
	}`

	err := os.WriteFile(configPath, []byte(jsonContent), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.BotToken != "test-token" {
		t.Errorf("expected bot_token 'test-token', got '%s'", cfg.BotToken)
	}

	if len(cfg.AllowedUsers) != 0 {
		t.Errorf("expected empty allowed_users, got %d users", len(cfg.AllowedUsers))
	}
}
