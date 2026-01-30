// internal/handler/handler.go
package handler

import (
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/paths"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/service"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/telegram"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/updater"
)

// Deps holds dependencies for all handlers
type Deps struct {
	Sender  telegram.MessageSender
	Config  service.ConfigStore   // interface from service/
	VPN     service.VPNDirector   // interface from service/
	Xray    service.XrayGenerator // interface from service/
	Network service.NetworkInfo   // interface from service/
	Logs    service.LogReader     // interface from service/
	Paths   paths.Paths
	Version     string          // Clean version for semver parsing (v1.2.0)
	VersionFull string          // Full git describe output (v1.2.0-5-gabc1234)
	Commit      string          // Git commit hash
	BuildDate   string          // Build date
	DevMode     bool            // Development mode flag
	Updater     updater.Updater // Update service for /update command
}
