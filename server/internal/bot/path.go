package bot

import (
	"sort"

	"github.com/zinin/vpn-director/server/internal/vpnconfig"
)

type pathKind int

const (
	kindNone pathKind = iota
	kindDirect
	kindSOCKS
	kindTunnel
)

const defaultSOCKSPort = 12346

type Path struct {
	kind      pathKind
	id        string
	iface     string
	socksPort int
}

func (p Path) String() string {
	switch p.kind {
	case kindDirect:
		return "direct"
	case kindSOCKS:
		return "socks"
	case kindTunnel:
		return "tunnel:" + p.id
	default:
		return "none"
	}
}

func (p Path) same(q Path) bool {
	return p.kind == q.kind && p.id == q.id
}

func pathParamsEqual(p, q Path) bool {
	return p.same(q) && p.socksPort == q.socksPort && p.iface == q.iface
}

func socksPort(cfg *vpnconfig.VPNDirectorConfig) int {
	_, socks := vpnconfig.XrayInboundPorts(cfg)
	if socks <= 0 {
		return defaultSOCKSPort
	}
	return socks
}

func candidates(cfg *vpnconfig.VPNDirectorConfig, plat vpnconfig.PlatformInfo, socksUp bool) []Path {
	out := []Path{{kind: kindDirect}}
	if socksUp {
		out = append(out, Path{kind: kindSOCKS, socksPort: socksPort(cfg)})
	}
	if cfg == nil {
		return out
	}
	ids := make([]string, 0, len(cfg.TunnelDirector.Tunnels))
	for id := range cfg.TunnelDirector.Tunnels {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	byID := make(map[string]vpnconfig.PlatformTunnel, len(plat.Tunnels))
	for _, t := range plat.Tunnels {
		byID[t.ID] = t
	}
	for _, id := range ids {
		if id == "main" {
			continue
		}
		tun := cfg.TunnelDirector.Tunnels[id]
		if len(tun.Clients) == 0 {
			continue
		}
		pt, ok := byID[id]
		if !ok || !pt.Connected || pt.Iface == "" {
			continue
		}
		out = append(out, Path{kind: kindTunnel, id: id, iface: pt.Iface})
	}
	return out
}

func selectPath(current Path, directLive, currentLive bool, replacement Path) Path {
	if directLive {
		return Path{kind: kindDirect}
	}
	if (current.kind == kindSOCKS || current.kind == kindTunnel) && currentLive {
		return current
	}
	return replacement
}
