package bot

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strconv"
	"sync"
	"syscall"
	"time"

	"golang.org/x/net/proxy"
)

// PermanentError wraps errors that should not be retried (config errors, auth failures).
type PermanentError struct {
	Err error
}

func (e *PermanentError) Error() string { return e.Err.Error() }
func (e *PermanentError) Unwrap() error { return e.Err }

// PathSource is the current Telegram API path and the sink for dial/TLS failures.
type PathSource interface {
	Current() Path
	ReportFailure(Path)
}

// IdleCloser is implemented by a PathSource that must close keep-alives on a path change.
// The callback receives the path the new transport generation is bound to.
type IdleCloser interface {
	RegisterIdleCloser(func(Path))
}

var errNoPath = errors.New("telegram API unreachable on every path")

type pathDialer struct {
	lookupIPv4  func(ctx context.Context, host string) ([]net.IP, error)
	bindControl func(iface string, mark uint32) func(network, address string, c syscall.RawConn) error
	socksDial   func(ctx context.Context, port int, network, addr string) (net.Conn, error)
	tcpDial     func(ctx context.Context, network, addr string, control func(network, address string, c syscall.RawConn) error) (net.Conn, error)
	dnsDial     func(ctx context.Context, network, address string) (net.Conn, error)
}

// productionDialer is written only at initialisation; tests inject a dialer
// through newPathClientWith instead of writing here.
var productionDialer pathDialer

// Matches http.DefaultTransport's net.Dialer.Timeout. NewPathClient overwrites
// DialContext, so this must be set on the replacement dialer or a SYN-drop
// hangs getUpdates until the kernel retry budget (~minutes).
const productionDialTimeout = 30 * time.Second

func newProductionDialer(control func(network, address string, c syscall.RawConn) error) *net.Dialer {
	return &net.Dialer{Timeout: productionDialTimeout, Control: control}
}

// DialPath connects using p. The caller snapshots p; this function does not.
func DialPath(ctx context.Context, p Path, network, addr string) (net.Conn, error) {
	return productionDialer.dial(ctx, p, network, addr)
}

func (d *pathDialer) dial(ctx context.Context, p Path, network, addr string) (net.Conn, error) {
	switch p.kind {
	case kindSOCKS:
		return d.dialSOCKS(ctx, p, addr)
	case kindTunnel:
		return d.dialTunnel(ctx, p, addr)
	case kindDirect:
		return d.dialDirect(ctx, addr)
	default:
		return nil, errNoPath
	}
}

func (d *pathDialer) dialSOCKS(ctx context.Context, p Path, addr string) (net.Conn, error) {
	fn := d.socksDial
	if fn == nil {
		fn = defaultSOCKSDial
	}
	return fn(ctx, p.socksPort, "tcp", addr)
}

func defaultSOCKSDial(ctx context.Context, port int, network, addr string) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, productionDialTimeout)
	defer cancel()
	dialer, err := proxy.SOCKS5("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), nil, &net.Dialer{Timeout: productionDialTimeout})
	if err != nil {
		return nil, err
	}
	if cd, ok := dialer.(proxy.ContextDialer); ok {
		return cd.DialContext(ctx, network, addr)
	}
	return dialer.Dial(network, addr)
}

func (d *pathDialer) dialDirect(ctx context.Context, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ips, err := d.lookupDirect(ctx, host)
	if err != nil {
		return nil, err
	}
	return d.dialIPv4s(ctx, ips, port, nil)
}

func (d *pathDialer) dialTunnel(ctx context.Context, p Path, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ips, err := d.lookupTunnel(ctx, p.iface, p.mark, host)
	if err != nil {
		return nil, err
	}
	// Call bindControl even when tcpDial is injected so tests can record iface.
	return d.dialIPv4s(ctx, ips, port, d.control(p.iface, p.mark))
}

func (d *pathDialer) control(iface string, mark uint32) func(network, address string, c syscall.RawConn) error {
	if d.bindControl != nil {
		return d.bindControl(iface, mark)
	}
	return tunnelSocketControl(iface, mark)
}

func (d *pathDialer) lookupDirect(ctx context.Context, host string) ([]net.IP, error) {
	if ip := ipv4Literal(host); ip != nil {
		return []net.IP{ip}, nil
	}
	if d.lookupIPv4 != nil {
		return d.lookupIPv4(ctx, host)
	}
	return net.DefaultResolver.LookupIP(ctx, "ip4", host)
}

var tunnelDNSServers = []string{"8.8.8.8:53", "1.1.1.1:53"}

// Cap per nameserver so a silent 8.8.8.8 cannot consume the whole probe budget
// and skip 1.1.1.1.
const tunnelDNSTimeout = 2 * time.Second

func (d *pathDialer) lookupTunnel(ctx context.Context, iface string, mark uint32, host string) ([]net.IP, error) {
	if ip := ipv4Literal(host); ip != nil {
		return []net.IP{ip}, nil
	}
	if d.lookupIPv4 != nil {
		return d.lookupIPv4(ctx, host)
	}
	var lastErr error
	for _, dns := range tunnelDNSServers {
		dnsCtx, cancel := context.WithTimeout(ctx, tunnelDNSTimeout)
		r := &net.Resolver{
			PreferGo: true, // cgo resolver ignores Dial
			Dial:     d.tunnelDNSDial(iface, mark, dns),
		}
		ips, err := r.LookupIP(dnsCtx, "ip4", host)
		cancel()
		if err == nil {
			return ips, nil
		}
		slog.Debug("Tunnel DNS lookup failed", "dns", dns, "iface", iface, "host", host, "error", err)
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
			return nil, err
		}
		lastErr = err
	}
	return nil, lastErr
}

func (d *pathDialer) tunnelDNSDial(iface string, mark uint32, dns string) func(ctx context.Context, network, address string) (net.Conn, error) {
	if d.dnsDial != nil {
		return func(ctx context.Context, network, _ string) (net.Conn, error) {
			return d.dnsDial(ctx, network, dns)
		}
	}
	control := d.control(iface, mark)
	return func(ctx context.Context, network, _ string) (net.Conn, error) {
		// Ignore the resolver's address so resolv.conf is not used.
		switch network {
		case "udp", "udp4", "udp6":
			network = "udp4"
		default:
			network = "tcp4"
		}
		return newProductionDialer(control).DialContext(ctx, network, dns)
	}
}

func (d *pathDialer) dialIPv4s(ctx context.Context, ips []net.IP, port string, control func(network, address string, c syscall.RawConn) error) (net.Conn, error) {
	var lastErr error
	tried := false
	for _, ip := range ips {
		v4 := ip.To4()
		if v4 == nil {
			continue
		}
		tried = true
		conn, err := d.doTCP(ctx, "tcp4", net.JoinHostPort(v4.String(), port), control)
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if !tried {
		return nil, errors.New("no IPv4 address")
	}
	return nil, lastErr
}

func (d *pathDialer) doTCP(ctx context.Context, network, addr string, control func(network, address string, c syscall.RawConn) error) (net.Conn, error) {
	if d.tcpDial != nil {
		return d.tcpDial(ctx, network, addr, control)
	}
	return newProductionDialer(control).DialContext(ctx, network, addr)
}

func ipv4Literal(host string) net.IP {
	ip := net.ParseIP(host)
	if ip == nil {
		return nil
	}
	return ip.To4()
}

type pathCtxKey struct{}

type pathTransport struct {
	src   PathSource
	mu    sync.Mutex
	bound Path
	base  *http.Transport
	gen   *connGen
	// dialer nil means the package's production dialer. The field exists so a
	// test injects a dialer instead of writing to that package variable.
	dialer *pathDialer
}

// connGen is the live sockets of one Transport generation. CloseIdleConnections
// leaves in-flight getUpdates alone; closeAll is what unblocks it on a path change.
type connGen struct {
	mu    sync.Mutex
	conns map[net.Conn]struct{}
}

func newConnGen() *connGen {
	return &connGen{conns: make(map[net.Conn]struct{})}
}

func (g *connGen) add(c net.Conn) {
	g.mu.Lock()
	g.conns[c] = struct{}{}
	g.mu.Unlock()
}

func (g *connGen) remove(c net.Conn) {
	g.mu.Lock()
	delete(g.conns, c)
	g.mu.Unlock()
}

func (g *connGen) closeAll() {
	g.mu.Lock()
	conns := g.conns
	g.conns = make(map[net.Conn]struct{})
	g.mu.Unlock()
	for c := range conns {
		_ = c.Close()
	}
}

type trackedConn struct {
	net.Conn
	once sync.Once
	gen  *connGen
}

func (c *trackedConn) Close() error {
	c.once.Do(func() { c.gen.remove(c) })
	return c.Conn.Close()
}

func newPathBase(dial func(ctx context.Context, network, addr string) (net.Conn, error)) *http.Transport {
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.Proxy = func(*http.Request) (*url.URL, error) { return nil, nil }
	base.DialContext = dial
	return base
}

// NewPathClient returns an HTTP client that dials through src.Current().
// Timeout is left at zero so getUpdates can long-poll.
func NewPathClient(src PathSource) *http.Client {
	return newPathClientWith(src, nil)
}

// newPathClientWith is NewPathClient with an injected dialer, so a test can
// replace one dial without writing to productionDialer.
func newPathClientWith(src PathSource, d *pathDialer) *http.Client {
	t := &pathTransport{src: src, bound: src.Current(), dialer: d}
	t.gen = newConnGen()
	t.base = t.attach(t.gen)
	if ic, ok := src.(IdleCloser); ok {
		ic.RegisterIdleCloser(t.retireBase)
	}
	return &http.Client{Transport: t}
}

func (t *pathTransport) attach(gen *connGen) *http.Transport {
	return newPathBase(func(ctx context.Context, network, addr string) (net.Conn, error) {
		return t.dialOn(ctx, network, addr, gen)
	})
}

// retireBase installs a new Transport bound to next so keep-alives from the
// previous path cannot be reused, and closes in-flight sockets of the old
// generation so getUpdates is not pinned to a blackholed path.
func (t *pathTransport) retireBase(next Path) {
	t.mu.Lock()
	old := t.base
	oldGen := t.gen
	t.bound = next
	t.gen = newConnGen()
	t.base = t.attach(t.gen)
	t.mu.Unlock()
	old.CloseIdleConnections()
	oldGen.closeAll()
}

func (t *pathTransport) CloseIdleConnections() {
	t.mu.Lock()
	base := t.base
	t.mu.Unlock()
	base.CloseIdleConnections()
}

func (t *pathTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	p := t.bound
	base := t.base
	t.mu.Unlock()
	req = req.WithContext(context.WithValue(req.Context(), pathCtxKey{}, p))
	var handshakeMu sync.Mutex
	var handshakeErr error
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{
		TLSHandshakeDone: func(_ tls.ConnectionState, err error) {
			if err != nil {
				handshakeMu.Lock()
				handshakeErr = err
				handshakeMu.Unlock()
			}
		},
	}))
	resp, err := base.RoundTrip(req)
	handshakeMu.Lock()
	hs := handshakeErr
	handshakeMu.Unlock()
	if err != nil && (isPathFailure(err) || hs != nil) {
		t.src.ReportFailure(p)
	}
	return resp, err
}

func (t *pathTransport) dialOn(ctx context.Context, network, addr string, gen *connGen) (net.Conn, error) {
	p, _ := ctx.Value(pathCtxKey{}).(Path)
	// Set once at construction and never mutated, so it needs no lock.
	d := t.dialer
	if d == nil {
		d = &productionDialer
	}
	c, err := d.dial(ctx, p, network, addr)
	if err != nil {
		return nil, err
	}
	tc := &trackedConn{Conn: c, gen: gen}
	gen.add(tc)
	return tc, nil
}

func socksListening(port int) bool {
	c, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), time.Second)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

func isPathFailure(err error) bool {
	if err == nil {
		return false
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	var ne net.Error
	if errors.As(err, &ne) {
		return true
	}
	var op *net.OpError
	return errors.As(err, &op)
}
