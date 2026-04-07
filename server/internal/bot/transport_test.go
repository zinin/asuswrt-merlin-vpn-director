package bot

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewHTTPClient_NoProxy(t *testing.T) {
	client, err := NewHTTPClient("", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.Transport != nil {
		t.Errorf("expected nil Transport for default client, got %T", client.Transport)
	}
}

func TestNewHTTPClient_InvalidProxyScheme(t *testing.T) {
	schemes := []string{"http://127.0.0.1:1080", "https://127.0.0.1:1080", "ftp://127.0.0.1:1080"}
	for _, proxyURL := range schemes {
		t.Run(proxyURL, func(t *testing.T) {
			_, err := NewHTTPClient(proxyURL, false)
			if err == nil {
				t.Fatal("expected error for non-socks5 scheme")
			}
		})
	}
}

func TestNewHTTPClient_ValidSOCKS5(t *testing.T) {
	client, err := NewHTTPClient("socks5://127.0.0.1:1080", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.Transport == nil {
		t.Fatal("expected non-nil Transport for SOCKS5 client")
	}
}

func TestNewHTTPClient_FallbackDirect(t *testing.T) {
	// Start a real HTTP server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	// Use a dead proxy address — nothing is listening on port 19999
	client, err := NewHTTPClient("socks5://127.0.0.1:19999", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// With fallback enabled, the request should succeed via direct connection
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("expected request to succeed with fallback, got error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestNewHTTPClient_NoFallback_Fails(t *testing.T) {
	// Start a real HTTP server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	// Use a dead proxy address — nothing is listening on port 19999
	client, err := NewHTTPClient("socks5://127.0.0.1:19999", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Without fallback, the request should fail because the proxy is unreachable
	_, err = client.Get(srv.URL)
	if err == nil {
		t.Fatal("expected request to fail without fallback, but it succeeded")
	}
}
