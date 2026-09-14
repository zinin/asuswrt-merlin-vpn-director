package bot

import (
	"testing"

	"github.com/zinin/vpn-director/server/internal/vpnconfig"
)

func TestPathString(t *testing.T) {
	if (Path{}).String() != "none" {
		t.Fatalf("zero path: got %q", Path{}.String())
	}
	if (Path{kind: kindDirect}).String() != "direct" {
		t.Fatal("direct")
	}
	if (Path{kind: kindSOCKS}).String() != "socks" {
		t.Fatal("socks")
	}
	if (Path{kind: kindTunnel, id: "ovpnc2"}).String() != "tunnel:ovpnc2" {
		t.Fatal("tunnel")
	}
}

func TestSocksPort(t *testing.T) {
	if socksPort(nil) != 12346 {
		t.Fatalf("nil cfg: got %d", socksPort(nil))
	}
	empty := &vpnconfig.VPNDirectorConfig{}
	if socksPort(empty) != 12346 {
		t.Fatalf("missing advanced: got %d", socksPort(empty))
	}
	cfg := &vpnconfig.VPNDirectorConfig{
		Advanced: map[string]interface{}{"xray": map[string]interface{}{"socks_port": float64(23456)}},
	}
	if socksPort(cfg) != 23456 {
		t.Fatalf("configured: got %d", socksPort(cfg))
	}
	zero := &vpnconfig.VPNDirectorConfig{
		Advanced: map[string]interface{}{"xray": map[string]interface{}{"socks_port": float64(0)}},
	}
	if socksPort(zero) != 12346 {
		t.Fatalf("non-positive: got %d", socksPort(zero))
	}
}

func TestCandidates(t *testing.T) {
	cfg := &vpnconfig.VPNDirectorConfig{
		TunnelDirector: vpnconfig.TunnelDirectorConfig{
			Tunnels: map[string]vpnconfig.TunnelConfig{
				"main":     {Clients: []string{"192.168.1.3"}},
				"ovpnc2":   {Clients: []string{"192.168.1.3"}},
				"ovpnc3":   {Clients: []string{}},
				"wgc1":     {Clients: []string{"192.168.1.4"}},
				"OpenVPN0": {Clients: []string{"192.168.1.5"}},
			},
		},
	}
	plat := vpnconfig.PlatformInfo{Tunnels: []vpnconfig.PlatformTunnel{
		{ID: "ovpnc2", Iface: "tun12", Connected: true},
		{ID: "wgc1", Iface: "wgc1", Connected: false},
		{ID: "OpenVPN0", Iface: "", Connected: true},
	}}
	got := candidates(cfg, plat, true)
	want := []string{"direct", "socks", "tunnel:ovpnc2"}
	if len(got) != len(want) {
		t.Fatalf("len=%d names=%v", len(got), names(got))
	}
	for i, p := range got {
		if p.String() != want[i] {
			t.Fatalf("idx %d: got %s want %s (all %v)", i, p.String(), want[i], names(got))
		}
	}
	if got[2].iface != "tun12" || got[1].socksPort != 12346 {
		t.Fatalf("iface=%q socksPort=%d", got[2].iface, got[1].socksPort)
	}

	noSocks := candidates(cfg, plat, false)
	if len(noSocks) != 2 || noSocks[0].String() != "direct" || noSocks[1].String() != "tunnel:ovpnc2" {
		t.Fatalf("socks down: %v", names(noSocks))
	}

	if n := names(candidates(nil, plat, true)); len(n) != 2 || n[0] != "direct" || n[1] != "socks" {
		t.Fatalf("nil cfg: %v", n)
	}
	if n := names(candidates(cfg, vpnconfig.PlatformInfo{}, true)); len(n) != 2 {
		t.Fatalf("platform empty: %v", n)
	}
}

func TestSelectPath(t *testing.T) {
	direct := Path{kind: kindDirect}
	socks := Path{kind: kindSOCKS}
	tun := Path{kind: kindTunnel, id: "ovpnc2"}
	tunB := Path{kind: kindTunnel, id: "OpenVPN0"}

	if p := selectPath(tun, true, true, socks); !p.same(direct) {
		t.Fatalf("direct wins over live tunnel: %s", p)
	}
	if p := selectPath(socks, false, true, tun); !p.same(socks) {
		t.Fatalf("keep live socks while tunnel also live: %s", p)
	}
	if p := selectPath(tun, false, true, socks); !p.same(tun) {
		t.Fatalf("keep live tunnel: %s", p)
	}
	if p := selectPath(Path{}, false, false, socks); !p.same(socks) {
		t.Fatalf("no current, socks first: %s", p)
	}
	if p := selectPath(socks, false, false, tun); !p.same(tun) {
		t.Fatalf("dead socks, take replacement: %s", p)
	}
	if p := selectPath(direct, false, false, Path{}); p.String() != "none" {
		t.Fatalf("all dead: %s", p)
	}
	if p := selectPath(tunB, false, false, socks); !p.same(socks) {
		t.Fatalf("dead current, socks replacement: %s", p)
	}
}

func names(ps []Path) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.String()
	}
	return out
}
