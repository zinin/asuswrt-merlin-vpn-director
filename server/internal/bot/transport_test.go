package bot

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"sync"
	"syscall"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

type fakeSource struct {
	p      Path
	failed []Path
	idle   []func(Path)
}

func (f *fakeSource) Current() Path                    { return f.p }
func (f *fakeSource) ReportFailure(p Path)             { f.failed = append(f.failed, p) }
func (f *fakeSource) RegisterIdleCloser(fn func(Path)) { f.idle = append(f.idle, fn) }

func TestDialPath_None(t *testing.T) {
	_, err := DialPath(context.Background(), Path{}, "tcp", "example.com:443")
	if !errors.Is(err, errNoPath) {
		t.Fatalf("got %v", err)
	}
}

func TestDialPath_DirectTCP4(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		c, err := ln.Accept()
		if err != nil {
			return
		}
		c.Close()
	}()
	conn, err := DialPath(context.Background(), Path{kind: kindDirect}, "tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("accept timed out")
	}
}

func TestPathDialer_SOCKSKeepsHostname(t *testing.T) {
	var gotAddr string
	d := pathDialer{
		socksDial: func(ctx context.Context, port int, network, addr string) (net.Conn, error) {
			gotAddr = addr
			if port != 12346 || network != "tcp" {
				t.Errorf("port=%d network=%s", port, network)
			}
			c1, c2 := net.Pipe()
			c2.Close()
			return c1, nil
		},
	}
	p := Path{kind: kindSOCKS, socksPort: 12346}
	conn, err := d.dial(context.Background(), p, "tcp", "api.telegram.org:443")
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	if gotAddr != "api.telegram.org:443" {
		t.Fatalf("SOCKS target %q", gotAddr)
	}
}

func TestPathDialer_TunnelBindsIface(t *testing.T) {
	var gotIface string
	var gotMark uint32
	d := pathDialer{
		lookupIPv4: func(ctx context.Context, host string) ([]net.IP, error) {
			return []net.IP{net.IPv4(1, 2, 3, 4)}, nil
		},
		bindControl: func(iface string, mark uint32) func(network, address string, c syscall.RawConn) error {
			gotIface = iface
			gotMark = mark
			return func(network, address string, c syscall.RawConn) error { return nil }
		},
		// The connect is failed deliberately: the test asserts only the recorded iface and mark.
		tcpDial: func(ctx context.Context, network, addr string, control func(string, string, syscall.RawConn) error) (net.Conn, error) {
			if network != "tcp4" {
				t.Errorf("network %s", network)
			}
			return nil, errors.New("dial skipped")
		},
	}
	p := Path{kind: kindTunnel, id: "ovpnc2", iface: "tun12", mark: 0x10000}
	_, _ = d.dial(context.Background(), p, "tcp", "api.telegram.org:443")
	if gotIface != "tun12" {
		t.Fatalf("iface %q", gotIface)
	}
	if gotMark != 0x10000 {
		t.Fatalf("mark 0x%x", gotMark)
	}
}

func TestNewPathClient_ReportsDialFailure(t *testing.T) {
	src := &fakeSource{p: Path{kind: kindDirect}}
	client := NewPathClient(src)
	// Nothing listens here; DialPath to this host:port fails.
	req, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:1/", nil)
	_, err := client.Do(req)
	if err == nil {
		t.Fatal("expected dial error")
	}
	if len(src.failed) != 1 || !src.failed[0].same(src.p) {
		t.Fatalf("failures %+v", src.failed)
	}
}

// A peer that accepts TCP and closes after ClientHello makes crypto/tls
// return plain io.EOF, which is not a net.Error. RoundTrip must still
// ReportFailure so PathManager reselects instead of waiting for the 30s probe.
func TestNewPathClient_ReportsTLSCloseAfterClientHello(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			buf := make([]byte, 2048)
			_, _ = c.Read(buf)
			_ = c.Close()
		}
	}()

	src := &fakeSource{p: Path{kind: kindDirect}}
	client := NewPathClient(src)
	_, err = client.Get("https://" + ln.Addr().String() + "/")
	if err == nil {
		t.Fatal("expected TLS handshake error")
	}
	if len(src.failed) != 1 || !src.failed[0].same(src.p) {
		t.Fatalf("ReportFailure skipped for %v; failures %+v", err, src.failed)
	}
}

// WithClientTrace already composes with a previous ClientTrace. Copying those
// hooks and chaining them ourselves makes TLSHandshakeStart run twice.
// The test exercises the late TLS-handshake write and detects the race only
// under -race, so it is intentionally assertion-free.
func TestNewPathClient_HandshakeErrNoRace(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		time.Sleep(80 * time.Millisecond)
		_ = c.Close()
	}()
	client := NewPathClient(&fakeSource{p: Path{kind: kindDirect}})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+ln.Addr().String()+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = client.Do(req)
	time.Sleep(120 * time.Millisecond)
}

func TestNewPathClient_ExistingTraceHooksRunOnce(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		buf := make([]byte, 2048)
		_, _ = c.Read(buf)
		_ = c.Close()
	}()

	start := make(chan struct{})
	ctx := httptrace.WithClientTrace(context.Background(), &httptrace.ClientTrace{
		TLSHandshakeStart: func() { close(start) },
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+ln.Addr().String()+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	client := NewPathClient(&fakeSource{p: Path{kind: kindDirect}})
	_, _ = client.Do(req)
	select {
	case <-start:
	case <-time.After(2 * time.Second):
		t.Fatal("TLSHandshakeStart not called")
	}
}

func TestNewPathClient_RegistersIdleCloser(t *testing.T) {
	src := &fakeSource{p: Path{kind: kindDirect}}
	_ = NewPathClient(src)
	if len(src.idle) != 1 {
		t.Fatalf("idle closers %d", len(src.idle))
	}
}

// CloseIdleConnections does not interrupt an in-use connection. getUpdates
// long-polls with Client.Timeout=0, so a blackholed path would pin the sole
// polling goroutine until the kernel TCP timeout. Retire must close that conn.
func TestNewPathClient_PathChangeAbortsInFlight(t *testing.T) {
	arrived := make(chan struct{})
	hold := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(arrived)
		<-hold
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	defer close(hold)

	src := &fakeSource{p: Path{kind: kindDirect}}
	client := NewPathClient(src)
	if len(src.idle) != 1 {
		t.Fatalf("idle closers %d", len(src.idle))
	}

	errc := make(chan error, 1)
	go func() {
		resp, err := client.Get(srv.URL)
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			errc <- resp.Body.Close()
			return
		}
		errc <- err
	}()
	select {
	case <-arrived:
	case <-time.After(2 * time.Second):
		t.Fatal("request did not reach the server")
	}

	src.idle[0](src.p)

	select {
	case err := <-errc:
		if err == nil {
			t.Fatal("in-flight getUpdates must not survive a path change")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("in-flight request still blocked after path change")
	}
}

// closeAll empties the generation's map but, without a retired flag, a dial
// that started before retire can still add() and return an old-path socket.
func TestNewPathClient_RetireRejectsLateDial(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var startOnce sync.Once
	d := pathDialer{
		tcpDial: func(ctx context.Context, network, addr string, control func(string, string, syscall.RawConn) error) (net.Conn, error) {
			startOnce.Do(func() { close(started) })
			<-release
			return net.Dial(network, addr)
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	src := &fakeSource{p: Path{kind: kindDirect}}
	client := newPathClientWith(src, &d)
	if len(src.idle) != 1 {
		t.Fatalf("idle closers %d", len(src.idle))
	}

	errc := make(chan error, 1)
	go func() {
		resp, err := client.Get(srv.URL)
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			errc <- resp.Body.Close()
			return
		}
		errc <- err
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("dial did not start")
	}

	src.idle[0](src.p)
	close(release)

	select {
	case err := <-errc:
		if err == nil {
			t.Fatal("late dial on a retired generation served a request")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("request still blocked after late dial")
	}
}

// CloseIdleConnections on a shared Transport is undone by the next RoundTrip
// (queueForIdleConn clears closeIdle). An in-flight getUpdates can then return
// its conn to the pool after a path switch. Retiring the Transport on the idle
// closer is what stops that reuse.
func TestNewPathClient_PathChangeDoesNotReuseOldConn(t *testing.T) {
	var mu sync.Mutex
	var addrs []string
	arrived := make(chan int, 8)
	release1 := make(chan struct{})
	release2 := make(chan struct{})
	var once1, once2 sync.Once
	rel1 := func() { once1.Do(func() { close(release1) }) }
	rel2 := func() { once2.Do(func() { close(release2) }) }

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		n := len(addrs)
		addrs = append(addrs, r.RemoteAddr)
		mu.Unlock()
		arrived <- n
		switch n {
		case 0:
			<-release1
		case 1:
			<-release2
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	defer rel1()
	defer rel2()

	src := &fakeSource{p: Path{kind: kindDirect}}
	client := NewPathClient(src)
	if len(src.idle) != 1 {
		t.Fatalf("idle closers %d", len(src.idle))
	}

	do := func() error {
		resp, err := client.Get(srv.URL)
		if err != nil {
			return err
		}
		io.Copy(io.Discard, resp.Body)
		return resp.Body.Close()
	}

	err1 := make(chan error, 1)
	go func() { err1 <- do() }()
	select {
	case n := <-arrived:
		if n != 0 {
			t.Fatalf("first request index %d", n)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first request did not reach the server")
	}

	src.idle[0](src.p) // path change: retire keep-alives so they cannot serve later requests

	select {
	case err := <-err1:
		if err == nil {
			t.Fatal("in-flight request survived path change")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first request did not finish")
	}

	err2 := make(chan error, 1)
	go func() { err2 <- do() }()
	select {
	case n := <-arrived:
		if n != 1 {
			t.Fatalf("second request index %d", n)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second request did not reach the server")
	}

	err3 := make(chan error, 1)
	go func() { err3 <- do() }()
	select {
	case n := <-arrived:
		if n != 2 {
			t.Fatalf("third request index %d", n)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("third request did not reach the server")
	}

	mu.Lock()
	if addrs[2] == addrs[0] {
		t.Fatalf("third request reused the pre-switch connection %s (addrs=%v)", addrs[0], addrs)
	}
	mu.Unlock()

	rel2()
	select {
	case err := <-err2:
		if err != nil {
			t.Fatalf("second request: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second request did not finish")
	}
	select {
	case err := <-err3:
		if err != nil {
			t.Fatalf("third request: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("third request did not finish")
	}
}

// retireBase installs a new pool before PathManager stores current. A request
// in that window must dial the path passed to the closer, not the stale Current(),
// or the new pool keeps an old-path connection forever.
func TestNewPathClient_SwitchBindsNewPathBeforeCurrentUpdates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	socks := Path{kind: kindSOCKS, socksPort: 12346}
	direct := Path{kind: kindDirect}
	src := &fakeSource{p: socks}

	var mu sync.Mutex
	var dialed []string
	d := pathDialer{
		socksDial: func(ctx context.Context, port int, network, addr string) (net.Conn, error) {
			mu.Lock()
			dialed = append(dialed, "socks")
			mu.Unlock()
			return net.Dial(network, addr)
		},
	}

	client := newPathClientWith(src, &d)
	if len(src.idle) != 1 {
		t.Fatalf("idle closers %d", len(src.idle))
	}
	src.idle[0](direct)
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	mu.Lock()
	defer mu.Unlock()
	if len(dialed) != 0 {
		t.Fatalf("new pool dialed stale Current()=%s: %v", src.Current(), dialed)
	}
}

func TestNewPathClient_NoClientTimeout(t *testing.T) {
	c := NewPathClient(&fakeSource{p: Path{kind: kindDirect}})
	if c.Timeout != 0 {
		t.Fatalf("Timeout=%s; getUpdates long-polls", c.Timeout)
	}
}

func TestLookupTunnel_FallsBackAfterDNSTimeout(t *testing.T) {
	silent, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer silent.Close()
	go func() {
		buf := make([]byte, 512)
		for {
			if _, _, err := silent.ReadFrom(buf); err != nil {
				return
			}
		}
	}()

	good, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer good.Close()
	go serveDNSA(good, net.IPv4(1, 2, 3, 4))

	d := pathDialer{
		dnsDial: func(ctx context.Context, network, address string) (net.Conn, error) {
			switch network {
			case "udp", "udp6":
				network = "udp4"
			case "tcp", "tcp6":
				network = "tcp4"
			}
			target := silent.LocalAddr().String()
			if address == "1.1.1.1:53" {
				target = good.LocalAddr().String()
			}
			var nd net.Dialer
			return nd.DialContext(ctx, network, target)
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ips, err := d.lookupTunnel(ctx, "tun0", 0, "vpn-director-dns-test.example.")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if len(ips) != 1 || !ips[0].Equal(net.IPv4(1, 2, 3, 4)) {
		t.Fatalf("ips %v", ips)
	}
}

func serveDNSA(pc net.PacketConn, ip net.IP) {
	v4 := ip.To4()
	if v4 == nil {
		return
	}
	buf := make([]byte, 2048)
	for {
		n, addr, err := pc.ReadFrom(buf)
		if err != nil {
			return
		}
		var p dnsmessage.Parser
		hdr, err := p.Start(buf[:n])
		if err != nil {
			continue
		}
		q, err := p.Question()
		if err != nil {
			continue
		}
		hdr.Response = true
		hdr.RecursionAvailable = true
		b := dnsmessage.NewBuilder(make([]byte, 0, 512), hdr)
		b.EnableCompression()
		if err := b.StartQuestions(); err != nil {
			continue
		}
		if err := b.Question(q); err != nil {
			continue
		}
		if err := b.StartAnswers(); err != nil {
			continue
		}
		if err := b.AResource(dnsmessage.ResourceHeader{
			Name:  q.Name,
			Type:  dnsmessage.TypeA,
			Class: dnsmessage.ClassINET,
			TTL:   60,
		}, dnsmessage.AResource{A: [4]byte{v4[0], v4[1], v4[2], v4[3]}}); err != nil {
			continue
		}
		msg, err := b.Finish()
		if err != nil {
			continue
		}
		_, _ = pc.WriteTo(msg, addr)
	}
}

func TestProductionDialer_Timeout(t *testing.T) {
	d := newProductionDialer(nil)
	if d.Timeout != 30*time.Second {
		t.Fatalf("Timeout=%s; DefaultTransport uses 30s", d.Timeout)
	}
}

func TestSocksListening(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	if !socksListening(port) {
		t.Fatal("expected listening")
	}
	if socksListening(1) {
		t.Fatal("port 1 should be down")
	}
}

func TestIsPathFailure(t *testing.T) {
	if isPathFailure(nil) {
		t.Fatal("nil")
	}
	op := &net.OpError{Op: "dial", Err: errors.New("refused")}
	if !isPathFailure(op) {
		t.Fatal("op")
	}
	// Nothing listens here; the dial error must read as a path failure.
	_, err := http.Get("http://127.0.0.1:1/")
	if err == nil {
		t.Fatal("expected dial error")
	}
	if !isPathFailure(err) {
		t.Fatalf("dial to a closed port must be a path failure: %v", err)
	}
}
