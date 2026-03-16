package bot

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"

	"golang.org/x/net/proxy"
)

// NewHTTPClient creates an HTTP client optionally configured with a SOCKS5 proxy.
// If proxyURL is empty, a default http.Client is returned.
// If fallbackDirect is true and the proxy is unreachable, requests fall back to a direct connection.
func NewHTTPClient(proxyURL string, fallbackDirect bool) (*http.Client, error) {
	if proxyURL == "" {
		return &http.Client{}, nil
	}

	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy URL: %w", err)
	}

	if u.Scheme != "socks5" {
		return nil, fmt.Errorf("unsupported proxy scheme %q: only socks5 is supported", u.Scheme)
	}

	dialer, err := proxy.FromURL(u, proxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("failed to create SOCKS5 dialer: %w", err)
	}

	ctxDialer, ok := dialer.(proxy.ContextDialer)
	if !ok {
		return nil, fmt.Errorf("SOCKS5 dialer does not support ContextDialer interface")
	}

	slog.Info("Using proxy for Telegram API", "proxy", u.Redacted())

	// Clone DefaultTransport to preserve standard timeouts, override DialContext for SOCKS5
	baseTransport := http.DefaultTransport.(*http.Transport).Clone()
	baseTransport.DialContext = ctxDialer.DialContext
	proxyTransport := baseTransport

	if !fallbackDirect {
		return &http.Client{Transport: proxyTransport}, nil
	}

	return &http.Client{
		Transport: &fallbackTransport{
			primary:  proxyTransport,
			fallback: http.DefaultTransport,
		},
	}, nil
}

// fallbackTransport implements http.RoundTripper. It tries the primary transport first
// and falls back to the fallback transport on dial errors.
type fallbackTransport struct {
	primary  http.RoundTripper
	fallback http.RoundTripper
}

func (t *fallbackTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.primary.RoundTrip(req)
	if err != nil {
		if isDialError(err) {
			slog.Warn("Proxy connection failed, falling back to direct", "error", err)
			cloned := req.Clone(req.Context())
			return t.fallback.RoundTrip(cloned)
		}
		return nil, err
	}
	return resp, nil
}

// isDialError reports whether err is a network dial error,
// indicating the proxy is unreachable.
func isDialError(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return opErr.Op == "dial" || opErr.Op == "socks connect"
	}
	return false
}

// PermanentError wraps errors that should not be retried (config errors, auth failures).
type PermanentError struct {
	Err error
}

func (e *PermanentError) Error() string { return e.Err.Error() }
func (e *PermanentError) Unwrap() error { return e.Err }
