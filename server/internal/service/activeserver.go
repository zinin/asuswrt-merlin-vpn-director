// internal/service/activeserver.go
package service

import (
	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/vpnconfig"
)

// GenerateAndRecordActiveServer writes config.json and the note of which
// server it was built from, both under the config lock.
//
// One lock, because the two have to agree. With the generation outside it, a
// switch from the other daemon can land between this one's write and its
// record: config.json ends up describing one server while the UI names
// another, and both switches report success. configure.sh already holds this
// same lock across the same pair.
//
// The record is written only once the generation has succeeded — an error from
// the closure skips the save — so a server whose parameters Xray rejects, the
// ordinary fate of an incomplete REALITY entry, never gets named as the
// running one.
//
// generated is what the caller must branch on, not the error. False means
// config.json is untouched and the switch did not happen — the generation was
// rejected, or the config could not even be loaded to reach it — so the caller
// has to report the failure and stop. True with a non-nil error means the
// opposite: config.json was written and only the record of it was not, and
// failing the caller over bookkeeping would send the user back to redo a
// switch that worked.
//
// The caller is expected to have persisted xray.servers already: a save
// failure here leaves a new config.json against a config that already lists
// its address in the bypass set, rather than one that does not.
func GenerateAndRecordActiveServer(store ConfigStore, xray XrayGenerator, s vpnconfig.Server, ports InboundPorts) (generated bool, err error) {
	err = store.UpdateVPNConfig(func(cfg *vpnconfig.VPNDirectorConfig) error {
		if err := xray.GenerateConfig(s, ports); err != nil {
			return err
		}
		generated = true
		cfg.Xray.ActiveServer = vpnconfig.NewActiveServer(s)
		return nil
	})
	return generated, err
}
