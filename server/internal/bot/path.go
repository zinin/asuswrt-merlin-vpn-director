package bot

import (
	"bufio"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

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

const defaultMarkShift = 16

const defaultTunnelTablesPath = "/tmp/tunnel_director/tun_dir_tables"

type Path struct {
	kind      pathKind
	id        string
	iface     string
	socksPort int
	mark      uint32
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
	return p.same(q) && p.socksPort == q.socksPort && p.iface == q.iface && p.mark == q.mark
}

func socksPort(cfg *vpnconfig.VPNDirectorConfig) int {
	_, socks := vpnconfig.XrayInboundPorts(cfg)
	if socks <= 0 {
		return defaultSOCKSPort
	}
	return socks
}

func markShift(cfg *vpnconfig.VPNDirectorConfig) uint {
	if cfg == nil {
		return defaultMarkShift
	}
	td, ok := cfg.Advanced["tunnel_director"].(map[string]interface{})
	if !ok {
		return defaultMarkShift
	}
	v, ok := td["mark_shift"].(float64)
	if !ok || v < 0 || v > 31 {
		return defaultMarkShift
	}
	return uint(v)
}

func tunnelMark(idx int, shift uint) uint32 {
	slot := idx + 1
	return uint32(slot) << shift
}

func parseTunnelTables(r io.Reader) map[string]int {
	out := map[string]int{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 {
			continue
		}
		idx, err := strconv.Atoi(fields[0])
		if err != nil || idx < 0 {
			continue
		}
		out[fields[1]] = idx
	}
	return out
}

func loadTunnelIdxFile(path string) map[string]int {
	f, err := os.Open(path)
	if err != nil {
		return map[string]int{}
	}
	defer f.Close()
	return parseTunnelTables(f)
}

func candidates(cfg *vpnconfig.VPNDirectorConfig, plat vpnconfig.PlatformInfo, socksUp bool, idxByID map[string]int) []Path {
	out := []Path{{kind: kindDirect}}
	if socksUp {
		out = append(out, Path{kind: kindSOCKS, socksPort: socksPort(cfg)})
	}
	if cfg == nil {
		return out
	}
	if idxByID == nil {
		idxByID = map[string]int{}
	}
	shift := markShift(cfg)
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
		var mark uint32
		if idx, ok := idxByID[id]; ok {
			mark = tunnelMark(idx, shift)
		}
		out = append(out, Path{kind: kindTunnel, id: id, iface: pt.Iface, mark: mark})
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
