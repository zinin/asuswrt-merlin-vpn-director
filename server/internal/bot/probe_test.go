package bot

import (
	"context"
	"net/http"
	"net/http/httptest"
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

func TestProbePath_DialErrorIsDead(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	err := probePath(ctx, "http://127.0.0.1:1", "TOKEN", Path{kind: kindDirect})
	if err == nil {
		t.Fatal("expected error")
	}
}
