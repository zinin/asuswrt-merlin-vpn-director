package bot

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/zinin/vpn-director/server/internal/vpnconfig"
)

func testMgr(t *testing.T, live map[string]bool, cfg *vpnconfig.VPNDirectorConfig, plat vpnconfig.PlatformInfo, socksUp bool) *PathManager {
	t.Helper()
	var mu sync.Mutex
	return NewPathManager(PathManagerConfig{
		Token: "TOKEN",
		LoadVPN: func() (*vpnconfig.VPNDirectorConfig, error) {
			if cfg == nil {
				return nil, errors.New("no config")
			}
			return cfg, nil
		},
		LoadPlatform: func() (vpnconfig.PlatformInfo, error) { return plat, nil },
		Listening:    func(port int) bool { return socksUp },
		Probe: func(ctx context.Context, p Path) error {
			mu.Lock()
			defer mu.Unlock()
			if live[p.String()] {
				return nil
			}
			return errors.New("dead")
		},
		APIBase:  "http://127.0.0.1:1",
		Interval: time.Hour,
	})
}

func tdCfg() *vpnconfig.VPNDirectorConfig {
	return &vpnconfig.VPNDirectorConfig{
		TunnelDirector: vpnconfig.TunnelDirectorConfig{
			Tunnels: map[string]vpnconfig.TunnelConfig{
				"ovpnc2": {Clients: []string{"192.168.1.3"}},
			},
		},
	}
}

func tdPlat() vpnconfig.PlatformInfo {
	return vpnconfig.PlatformInfo{Tunnels: []vpnconfig.PlatformTunnel{
		{ID: "ovpnc2", Iface: "tun12", Connected: true},
	}}
}

func TestPathManager_DirectWinsAndSwitchesBack(t *testing.T) {
	live := map[string]bool{"direct": false, "socks": true, "tunnel:ovpnc2": true}
	m := testMgr(t, live, tdCfg(), tdPlat(), true)
	m.SelectOnce(context.Background())
	if m.Current().String() != "socks" {
		t.Fatalf("want socks, got %s", m.Current())
	}
	live["direct"] = true
	m.SelectOnce(context.Background())
	if m.Current().String() != "direct" {
		t.Fatalf("switch back: %s", m.Current())
	}
}

func TestPathManager_EqualsStick(t *testing.T) {
	live := map[string]bool{"direct": false, "socks": true, "tunnel:ovpnc2": true}
	m := testMgr(t, live, tdCfg(), tdPlat(), true)
	m.SelectOnce(context.Background())
	if m.Current().String() != "socks" {
		t.Fatalf("got %s", m.Current())
	}
	m.SelectOnce(context.Background())
	if m.Current().String() != "socks" {
		t.Fatalf("flapped to %s", m.Current())
	}
}

func TestPathManager_PlatformErrorSkipsTunnels(t *testing.T) {
	m := NewPathManager(PathManagerConfig{
		LoadVPN: func() (*vpnconfig.VPNDirectorConfig, error) { return tdCfg(), nil },
		LoadPlatform: func() (vpnconfig.PlatformInfo, error) {
			return vpnconfig.PlatformInfo{}, errors.New("rci down")
		},
		Listening: func(int) bool { return false },
		Probe: func(ctx context.Context, p Path) error {
			if p.kind == kindTunnel {
				t.Fatal("must not probe tunnels when platform fails")
			}
			return errors.New("dead")
		},
	})
	m.SelectOnce(context.Background())
	if m.Current().String() != "none" {
		t.Fatalf("got %s", m.Current())
	}
}

func TestPathManager_ReportFailureNoopIfStale(t *testing.T) {
	live := map[string]bool{"direct": true}
	m := testMgr(t, live, tdCfg(), tdPlat(), false)
	m.SelectOnce(context.Background())
	if m.Current().String() != "direct" {
		t.Fatal(m.Current())
	}
	m.ReportFailure(Path{kind: kindTunnel, id: "ovpnc2"})
	time.Sleep(50 * time.Millisecond)
	if m.Current().String() != "direct" {
		t.Fatalf("stale failure flapped to %s", m.Current())
	}
}

func TestPathManager_ReportFailureReselects(t *testing.T) {
	live := map[string]bool{"direct": false, "socks": true, "tunnel:ovpnc2": true}
	m := testMgr(t, live, tdCfg(), tdPlat(), true)
	m.SelectOnce(context.Background())
	if m.Current().String() != "socks" {
		t.Fatal(m.Current())
	}
	live["socks"] = false
	m.ReportFailure(m.Current())
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if m.Current().String() == "tunnel:ovpnc2" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("still %s", m.Current())
}

func TestPathManager_ClosesIdleOnChange(t *testing.T) {
	live := map[string]bool{"direct": true}
	m := testMgr(t, live, tdCfg(), tdPlat(), false)
	n := 0
	m.RegisterIdleCloser(func() { n++ })
	m.SelectOnce(context.Background()) // none -> direct
	if n != 1 {
		t.Fatalf("first select closes idle: %d", n)
	}
	live["direct"] = false
	live["tunnel:ovpnc2"] = true
	// socks down; replacement is tunnel
	m.SelectOnce(context.Background())
	if n != 2 {
		t.Fatalf("path change: %d", n)
	}
}
