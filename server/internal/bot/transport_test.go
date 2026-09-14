package bot

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"syscall"
	"testing"
	"time"
)

type fakeSource struct {
	p      Path
	failed []Path
	idle   []func()
}

func (f *fakeSource) Current() Path                { return f.p }
func (f *fakeSource) ReportFailure(p Path)         { f.failed = append(f.failed, p) }
func (f *fakeSource) RegisterIdleCloser(fn func()) { f.idle = append(f.idle, fn) }

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
	d := pathDialer{
		lookupIPv4: func(ctx context.Context, host string) ([]net.IP, error) {
			return []net.IP{net.IPv4(1, 2, 3, 4)}, nil
		},
		bindControl: func(iface string) func(network, address string, c syscall.RawConn) error {
			gotIface = iface
			return func(network, address string, c syscall.RawConn) error { return nil }
		},
		// dialTCP is not injected; use a listening socket on 1.2.3.4? that won't work.
		// Instead record bindControl and fail the connect: we only assert iface.
		tcpDial: func(ctx context.Context, network, addr string, control func(string, string, syscall.RawConn) error) (net.Conn, error) {
			if network != "tcp4" {
				t.Errorf("network %s", network)
			}
			if control != nil {
				// invoke so bindControl's closure ran when building control
			}
			return nil, errors.New("dial skipped")
		},
	}
	p := Path{kind: kindTunnel, id: "ovpnc2", iface: "tun12"}
	_, _ = d.dial(context.Background(), p, "tcp", "api.telegram.org:443")
	if gotIface != "tun12" {
		t.Fatalf("iface %q", gotIface)
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

func TestNewPathClient_RegistersIdleCloser(t *testing.T) {
	src := &fakeSource{p: Path{kind: kindDirect}}
	_ = NewPathClient(src)
	if len(src.idle) != 1 {
		t.Fatalf("idle closers %d", len(src.idle))
	}
}

func TestNewPathClient_NoClientTimeout(t *testing.T) {
	c := NewPathClient(&fakeSource{p: Path{kind: kindDirect}})
	if c.Timeout != 0 {
		t.Fatalf("Timeout=%s; getUpdates long-polls", c.Timeout)
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "ok")
	}))
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if isPathFailure(err) {
		t.Fatal("success is not a path failure")
	}
}
