// internal/service/activeserver_test.go
package service

import (
	"errors"
	"testing"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/vpnconfig"
)

// stubStore is the slice of ConfigStore this function touches. UpdateVPNConfig
// runs fn against the config it holds, so a test reads back what was written.
type stubStore struct {
	cfg *vpnconfig.VPNDirectorConfig
	err error
}

func (s *stubStore) LoadVPNConfig() (*vpnconfig.VPNDirectorConfig, error) { return s.cfg, nil }
func (s *stubStore) LoadServers() ([]vpnconfig.Server, error)             { return nil, nil }
func (s *stubStore) SaveServers([]vpnconfig.Server) error                 { return nil }
func (s *stubStore) UpdateVPNConfig(fn func(*vpnconfig.VPNDirectorConfig) error) error {
	if s.err != nil {
		return s.err
	}
	return fn(s.cfg)
}
func (s *stubStore) DataDir() (string, error) { return "", nil }
func (s *stubStore) DataDirOrDefault() string { return "" }
func (s *stubStore) ScriptsDir() string       { return "" }

func TestRecordActiveServer_StoresWhatIdentifiesTheServer(t *testing.T) {
	store := &stubStore{cfg: &vpnconfig.VPNDirectorConfig{}}

	err := RecordActiveServer(store, vpnconfig.Server{
		Name:      "Берлин, Германия, Extra",
		Address:   "155.117.201.148",
		Port:      443,
		UUID:      "the-subscription-uuid",
		PublicKey: "PBK",
	})

	if err != nil {
		t.Fatalf("RecordActiveServer error: %v", err)
	}
	got := store.cfg.Xray.ActiveServer
	if got == nil {
		t.Fatal("nothing was recorded")
	}
	if got.Name != "Берлин, Германия, Extra" || got.Address != "155.117.201.148" || got.Port != 443 {
		t.Errorf("recorded %+v, want the name, address and port of the selected server", *got)
	}
}

// The caller decides what a bookkeeping failure means, so it has to hear about
// one: swallowing it here would leave the config naming the previous server
// with nobody the wiser.
func TestRecordActiveServer_ReportsAStoreFailure(t *testing.T) {
	store := &stubStore{cfg: &vpnconfig.VPNDirectorConfig{}, err: errors.New("config lock timeout")}

	if err := RecordActiveServer(store, vpnconfig.Server{Name: "Осло, Норвегия"}); err == nil {
		t.Error("expected the store failure to come back, got nil")
	}
}
