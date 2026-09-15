package bot

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"strings"
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

func TestDialDirect_LookupHasDeadline(t *testing.T) {
	var had bool
	d := pathDialer{
		lookupIPv4: func(ctx context.Context, host string) ([]net.IP, error) {
			_, had = ctx.Deadline()
			return nil, errors.New("stop")
		},
	}
	_, _ = d.dial(context.Background(), Path{kind: kindDirect}, "tcp", "api.telegram.org:443")
	if !had {
		t.Fatal("direct DNS lookup has no dial timeout")
	}
}

func TestDialIPv4s_ReservesTimeForLaterAddresses(t *testing.T) {
	var mu sync.Mutex
	var secondRemain time.Duration
	sawSecond := false
	d := pathDialer{
		tcpDial: func(ctx context.Context, network, addr string, control func(string, string, syscall.RawConn) error) (net.Conn, error) {
			host, _, _ := net.SplitHostPort(addr)
			if host == "5.6.7.8" {
				mu.Lock()
				sawSecond = true
				if dl, ok := ctx.Deadline(); ok {
					secondRemain = time.Until(dl)
				}
				mu.Unlock()
				c1, c2 := net.Pipe()
				c2.Close()
				return c1, nil
			}
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	conn, err := d.dialIPv4s(ctx, []net.IP{net.IPv4(1, 2, 3, 4), net.IPv4(5, 6, 7, 8)}, "443", nil)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("second address: %v", err)
	}
	conn.Close()
	if elapsed > 4*time.Second {
		t.Fatalf("first address consumed the budget: %s", elapsed)
	}
	mu.Lock()
	defer mu.Unlock()
	if !sawSecond {
		t.Fatal("second address not tried")
	}
	if secondRemain < time.Second {
		t.Fatalf("second address remaining %s", secondRemain)
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
		resp, err := client.Get(srv.URL + "/botTOKEN/getUpdates")
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

func TestNewPathClient_PathChangeDoesNotAbortSend(t *testing.T) {
	arrived := make(chan struct{})
	hold := make(chan struct{})
	var holdOnce sync.Once
	release := func() { holdOnce.Do(func() { close(hold) }) }
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(arrived)
		<-hold
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	defer release()

	src := &fakeSource{p: Path{kind: kindDirect}}
	client := NewPathClient(src)

	errc := make(chan error, 1)
	go func() {
		resp, err := client.Get(srv.URL + "/botTOKEN/sendMessage")
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
		t.Fatal("send did not reach the server")
	}

	src.idle[0](src.p)

	select {
	case err := <-errc:
		t.Fatalf("send aborted on path change: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	release()
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("send: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("send did not finish after release")
	}
}

func TestNewPathBase_AdvertisesOnlyHTTP11(t *testing.T) {
	tr := newPathBase(func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("no dial")
	})
	if tr.ForceAttemptHTTP2 {
		t.Fatal("ForceAttemptHTTP2 still set")
	}
	if tr.TLSClientConfig == nil {
		t.Fatal("TLSClientConfig is nil; cloned DefaultTransport still advertises h2")
	}
	found11 := false
	for _, p := range tr.TLSClientConfig.NextProtos {
		if p == "h2" || p == "h2c" {
			t.Fatalf("ALPN still has %q: %v", p, tr.TLSClientConfig.NextProtos)
		}
		if p == "http/1.1" {
			found11 = true
		}
	}
	if !found11 {
		t.Fatalf("ALPN %v", tr.TLSClientConfig.NextProtos)
	}
}

func TestNewPathClient_SendOnReusedPollConnSurvivesPathChange(t *testing.T) {
	sendArrived := make(chan struct{})
	holdSend := make(chan struct{})
	var holdOnce sync.Once
	release := func() { holdOnce.Do(func() { close(holdSend) }) }
	var arrivedOnce sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/botTOKEN/getUpdates" {
			w.WriteHeader(http.StatusOK)
			return
		}
		arrivedOnce.Do(func() { close(sendArrived) })
		<-holdSend
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	defer release()

	src := &fakeSource{p: Path{kind: kindDirect}}
	client := NewPathClient(src)
	resp, err := client.Get(srv.URL + "/botTOKEN/getUpdates")
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	errc := make(chan error, 1)
	go func() {
		resp, err := client.Get(srv.URL + "/botTOKEN/sendMessage")
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			errc <- resp.Body.Close()
			return
		}
		errc <- err
	}()
	select {
	case <-sendArrived:
	case <-time.After(2 * time.Second):
		t.Fatal("send did not reach the server")
	}

	src.idle[0](src.p)
	select {
	case err := <-errc:
		t.Fatalf("send on reused poll conn aborted: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	release()
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("send: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("send did not finish after release")
	}
}

func TestNewPathClient_PollSeesRetireAcrossSnapshot(t *testing.T) {
	sendArrived := make(chan struct{})
	holdSend := make(chan struct{})
	pollArrived := make(chan struct{})
	holdPoll := make(chan struct{})
	var sendArrivedOnce, sendHoldOnce, pollArrivedOnce, pollHoldOnce sync.Once
	releaseSend := func() { sendHoldOnce.Do(func() { close(holdSend) }) }
	releasePoll := func() { pollHoldOnce.Do(func() { close(holdPoll) }) }
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/getUpdates") {
			pollArrivedOnce.Do(func() { close(pollArrived) })
			<-holdPoll
			w.WriteHeader(http.StatusOK)
			return
		}
		sendArrivedOnce.Do(func() { close(sendArrived) })
		<-holdSend
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	defer releaseSend()
	defer releasePoll()

	src := &fakeSource{p: Path{kind: kindDirect}}
	client := NewPathClient(src)
	pt := client.Transport.(*pathTransport)
	pt.base.MaxConnsPerHost = 1

	sendErr := make(chan error, 1)
	go func() {
		resp, err := client.Get(srv.URL + "/botTOKEN/sendMessage")
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			sendErr <- resp.Body.Close()
			return
		}
		sendErr <- err
	}()
	select {
	case <-sendArrived:
	case <-time.After(2 * time.Second):
		t.Fatal("send did not reach the server")
	}

	pt.afterBoundSnapshot = func() { src.idle[0](src.p) }

	pollErr := make(chan error, 1)
	go func() {
		resp, err := client.Get(srv.URL + "/botTOKEN/getUpdates")
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			pollErr <- resp.Body.Close()
			return
		}
		pollErr <- err
	}()

	releaseSend()
	select {
	case err := <-sendErr:
		if err != nil {
			t.Fatalf("send: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("send did not finish")
	}

	select {
	case err := <-pollErr:
		if err == nil {
			t.Fatal("getUpdates completed on the retired path")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("getUpdates remains blocked on the retired path")
	}
}

func TestNewPathClient_PathChangeAbortsPollAfterHeaders(t *testing.T) {
	arrived := make(chan struct{})
	hold := make(chan struct{})
	var holdOnce sync.Once
	release := func() { holdOnce.Do(func() { close(hold) }) }
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/botTOKEN/getMe" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		close(arrived)
		<-hold
	}))
	defer srv.Close()
	defer release()

	src := &fakeSource{p: Path{kind: kindDirect}}
	client := NewPathClient(src)
	resp, err := client.Get(srv.URL + "/botTOKEN/getMe")
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	errc := make(chan error, 1)
	go func() {
		resp, err := client.Get(srv.URL + "/botTOKEN/getUpdates")
		if err != nil {
			errc <- err
			return
		}
		_, err = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		errc <- err
	}()
	select {
	case <-arrived:
	case <-time.After(2 * time.Second):
		t.Fatal("getUpdates headers did not arrive")
	}

	src.idle[0](src.p)
	select {
	case err := <-errc:
		if err == nil {
			t.Fatal("getUpdates body survived path change after headers")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("getUpdates still blocked after path change")
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
		resp, err := client.Get(srv.URL + "/botTOKEN/getUpdates")
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		addrs = append(addrs, r.RemoteAddr)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

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
	if err := do(); err != nil {
		t.Fatal(err)
	}
	src.idle[0](src.p)
	if err := do(); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(addrs) != 2 {
		t.Fatalf("requests %d addrs=%v", len(addrs), addrs)
	}
	if addrs[1] == addrs[0] {
		t.Fatalf("reused the pre-switch connection %s", addrs[0])
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
