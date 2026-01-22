// internal/service/xray_test.go
package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/vpnconfig"
)

func TestXrayService_GenerateConfig(t *testing.T) {
	tmpDir := t.TempDir()

	templatePath := filepath.Join(tmpDir, "config.json.template")
	outputPath := filepath.Join(tmpDir, "config.json")

	template := `{
  "address": "{{XRAY_SERVER_ADDRESS}}",
  "port": {{XRAY_SERVER_PORT}},
  "uuid": "{{XRAY_USER_UUID}}"
}`
	os.WriteFile(templatePath, []byte(template), 0644)

	svc := NewXrayService(templatePath, outputPath)

	server := vpnconfig.Server{
		Address: "example.com",
		Port:    443,
		UUID:    "abc-123",
	}

	err := svc.GenerateConfig(server)
	if err != nil {
		t.Fatalf("GenerateConfig error: %v", err)
	}

	content, _ := os.ReadFile(outputPath)
	if !strings.Contains(string(content), "example.com") {
		t.Error("config should contain address")
	}
	if !strings.Contains(string(content), "443") {
		t.Error("config should contain port")
	}
	if !strings.Contains(string(content), "abc-123") {
		t.Error("config should contain uuid")
	}
}

func TestXrayService_GenerateConfig_MissingTemplate(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewXrayService(filepath.Join(tmpDir, "nonexistent"), filepath.Join(tmpDir, "out"))

	err := svc.GenerateConfig(vpnconfig.Server{})
	if err == nil {
		t.Error("expected error for missing template")
	}
}
