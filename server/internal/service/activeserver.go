// internal/service/activeserver.go
package service

import (
	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/vpnconfig"
)

// RecordActiveServer stores which server the Xray config was built from.
//
// Call it after the generation succeeds, never before. This record is the only
// answer anyone has to "which server is running" - config.json holds just the
// outbound, and a subscription routinely puts many names behind one endpoint -
// so a record naming a server whose config failed to generate is worse than no
// record at all.
func RecordActiveServer(store ConfigStore, s vpnconfig.Server) error {
	return store.UpdateVPNConfig(func(cfg *vpnconfig.VPNDirectorConfig) error {
		cfg.Xray.ActiveServer = vpnconfig.NewActiveServer(s)
		return nil
	})
}
