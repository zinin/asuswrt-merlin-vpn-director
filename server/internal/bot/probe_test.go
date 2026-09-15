package bot

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestProbePath_AnyHTTPIsLive(t *testing.T) {
	codes := []int{200, 401, 429, 500}
	for _, code := range codes {
		t.Run(http.StatusText(code), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/botTOKEN/getMe" {
					t.Errorf("path %s", r.URL.Path)
				}
				w.WriteHeader(code)
			}))
			t.Cleanup(srv.Close)
			err := probePath(context.Background(), srv.URL, "TOKEN", Path{kind: kindDirect})
			if err != nil {
				t.Fatalf("code %d: %v", code, err)
			}
		})
	}
}

// http.Transport dials with context.WithoutCancel(req.Context()), so the 8s
// probe deadline is gone unless DialContext puts it back. Otherwise DialPath
// uses 30s and the first of two A records can eat the whole probe.
func TestProbePath_DialSharesProbeBudget(t *testing.T) {
	old := productionDialer
	t.Cleanup(func() { productionDialer = old })

	var mu sync.Mutex
	var firstBudget time.Duration
	productionDialer.lookupIPv4 = func(ctx context.Context, host string) ([]net.IP, error) {
		return []net.IP{net.IPv4(1, 2, 3, 4), net.IPv4(5, 6, 7, 8)}, nil
	}
	productionDialer.tcpDial = func(ctx context.Context, network, addr string, control func(string, string, syscall.RawConn) error) (net.Conn, error) {
		host, _, _ := net.SplitHostPort(addr)
		if host == "1.2.3.4" {
			if dl, ok := ctx.Deadline(); ok {
				mu.Lock()
				firstBudget = time.Until(dl)
				mu.Unlock()
			}
			<-ctx.Done()
			return nil, ctx.Err()
		}
		c1, c2 := net.Pipe()
		c2.Close()
		return c1, nil
	}

	_ = probePath(context.Background(), "http://probe.test", "token", Path{kind: kindDirect})

	mu.Lock()
	defer mu.Unlock()
	if firstBudget == 0 {
		t.Fatal("first address had no deadline")
	}
	if firstBudget > 6*time.Second {
		t.Fatalf("first address budget %s; Transport stripped the 8s probe deadline", firstBudget)
	}
}

func TestProbePath_DialErrorIsDead(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	err := probePath(ctx, "http://127.0.0.1:1", "TOKEN", Path{kind: kindDirect})
	if err == nil {
		t.Fatal("expected error")
	}
}
