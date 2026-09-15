package bot

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"slices"
	"sync"
	"time"

	"github.com/zinin/vpn-director/server/internal/vpnconfig"
)

const defaultPathInterval = 30 * time.Second

const defaultAPIBase = "https://api.telegram.org"

var (
	_ PathSource = (*PathManager)(nil)
	_ IdleCloser = (*PathManager)(nil)
)

type PathManager struct {
	token        string
	loadVPN      func() (*vpnconfig.VPNDirectorConfig, error)
	loadPlatform func() (vpnconfig.PlatformInfo, error)
	listening    func(port int) bool
	probe        func(ctx context.Context, p Path) error
	apiBase      string
	interval     time.Duration

	mu      sync.Mutex
	current Path
	cycling bool
	closers []func()
}

type PathManagerConfig struct {
	Token        string
	LoadVPN      func() (*vpnconfig.VPNDirectorConfig, error)
	LoadPlatform func() (vpnconfig.PlatformInfo, error)
	Listening    func(port int) bool                     // nil => socksListening
	Probe        func(ctx context.Context, p Path) error // nil => probePath(ctx, apiBase, token, p)
	APIBase      string                                  // empty => https://api.telegram.org
	Interval     time.Duration                           // 0 => 30s
}

func NewPathManager(cfg PathManagerConfig) *PathManager {
	m := &PathManager{
		token:        cfg.Token,
		loadVPN:      cfg.LoadVPN,
		loadPlatform: cfg.LoadPlatform,
		listening:    cfg.Listening,
		probe:        cfg.Probe,
		apiBase:      cfg.APIBase,
		interval:     cfg.Interval,
	}
	if m.listening == nil {
		m.listening = socksListening
	}
	if m.apiBase == "" {
		m.apiBase = defaultAPIBase
	}
	if m.interval == 0 {
		m.interval = defaultPathInterval
	}
	if m.probe == nil {
		m.probe = func(ctx context.Context, p Path) error {
			return probePath(ctx, m.apiBase, m.token, p)
		}
	}
	return m
}

func (m *PathManager) Current() Path {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.current
}

func (m *PathManager) RegisterIdleCloser(fn func()) {
	m.mu.Lock()
	m.closers = append(m.closers, fn)
	m.mu.Unlock()
}

func (m *PathManager) ReportFailure(p Path) {
	m.mu.Lock()
	skip := !p.same(m.current) || m.cycling
	m.mu.Unlock()
	if skip {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		m.SelectOnce(ctx)
	}()
}

func (m *PathManager) Start(ctx context.Context) {
	t := time.NewTicker(m.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.SelectOnce(ctx)
		}
	}
}

func (m *PathManager) SelectOnce(ctx context.Context) {
	m.mu.Lock()
	if m.cycling {
		m.mu.Unlock()
		return
	}
	m.cycling = true
	current := m.current
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.cycling = false
		m.mu.Unlock()
	}()

	var cfg *vpnconfig.VPNDirectorConfig
	if m.loadVPN != nil {
		loaded, err := m.loadVPN()
		if err != nil {
			cfg = nil
		} else {
			cfg = loaded
		}
	}

	var plat vpnconfig.PlatformInfo
	if m.loadPlatform != nil {
		loaded, err := m.loadPlatform()
		if err != nil {
			plat = vpnconfig.PlatformInfo{}
		} else {
			plat = loaded
		}
	}

	port := socksPort(cfg)
	socksUp := m.listening(port)
	cands := candidates(cfg, plat, socksUp)

	var tried []string
	direct := Path{kind: kindDirect}
	directLive := m.probeOne(ctx, direct, &tried)

	stored := current
	currentLive := false
	currentProbed := false
	if current.kind == kindSOCKS || current.kind == kindTunnel {
		if fresh, ok := matchPath(cands, current); ok {
			current = fresh
			currentLive = m.probeOne(ctx, current, &tried)
			currentProbed = true
		}
	}

	needReplacement := !directLive && (current.kind == kindNone || !currentLive)
	var replacement Path
	if needReplacement {
		for _, p := range cands {
			if p.kind == kindDirect {
				continue
			}
			if currentProbed && p.same(current) {
				continue
			}
			if m.probeOne(ctx, p, &tried) {
				replacement = p
				break
			}
		}
	}

	next := selectPath(current, directLive, currentLive, replacement)
	changed := !next.same(stored)
	if changed {
		slog.Info("Telegram API path selected", "from", stored.String(), "to", next.String())
		if stored.kind == kindDirect && (next.kind == kindSOCKS || next.kind == kindTunnel) {
			slog.Warn("Telegram API unreachable on WAN, using backup path")
		}
	}
	if changed || !pathParamsEqual(next, stored) {
		m.mu.Lock()
		closers := slices.Clone(m.closers)
		m.mu.Unlock()
		for _, fn := range closers {
			fn()
		}
		m.mu.Lock()
		m.current = next
		m.mu.Unlock()
	}
	if next.kind == kindNone {
		slog.Warn("Telegram API unreachable on every path", "tried", tried)
	}
}

func (m *PathManager) probeOne(ctx context.Context, p Path, tried *[]string) bool {
	err := m.probe(ctx, p)
	live := err == nil
	slog.Debug("Telegram API path probe", "path", p.String(), "live", live)
	reason := "live"
	if !live {
		reason = pathFailReason(err)
	}
	*tried = append(*tried, p.String()+"="+reason)
	return live
}

func matchPath(cands []Path, p Path) (Path, bool) {
	for _, c := range cands {
		if c.same(p) {
			return c, true
		}
	}
	return Path{}, false
}

func pathFailReason(err error) string {
	if err == nil {
		return "live"
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "timeout"
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return "timeout"
	}
	return "dead"
}
