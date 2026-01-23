// internal/service/xray.go
package service

import (
	"fmt"
	"os"
	"strings"

	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/vpnconfig"
)

// XrayService handles Xray configuration generation
type XrayService struct {
	templatePath string
	outputPath   string
}

// Compile-time check that XrayService implements XrayGenerator
var _ XrayGenerator = (*XrayService)(nil)

// NewXrayService creates a new XrayService
func NewXrayService(templatePath, outputPath string) *XrayService {
	return &XrayService{
		templatePath: templatePath,
		outputPath:   outputPath,
	}
}

// GenerateConfig generates Xray config from template
func (s *XrayService) GenerateConfig(server vpnconfig.Server) error {
	template, err := os.ReadFile(s.templatePath)
	if err != nil {
		return fmt.Errorf("read template: %w", err)
	}

	config := string(template)
	config = strings.ReplaceAll(config, "{{XRAY_SERVER_ADDRESS}}", server.Address)
	config = strings.ReplaceAll(config, "{{XRAY_SERVER_PORT}}", fmt.Sprintf("%d", server.Port))
	config = strings.ReplaceAll(config, "{{XRAY_USER_UUID}}", server.UUID)

	if err := os.WriteFile(s.outputPath, []byte(config), 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}
