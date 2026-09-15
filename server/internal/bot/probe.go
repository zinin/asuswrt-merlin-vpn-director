package bot

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

const probeTimeout = 8 * time.Second

func probePath(ctx context.Context, apiBase, token string, p Path) error {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	// Transport dials with context.WithoutCancel(req.Context()), which drops
	// the deadline. Put it back so DialPath splits the 8s probe, not 30s.
	deadline, hasDeadline := ctx.Deadline()
	client := &http.Client{
		Timeout: probeTimeout,
		Transport: &http.Transport{
			Proxy: func(*http.Request) (*url.URL, error) { return nil, nil },
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				if hasDeadline {
					var cancel context.CancelFunc
					ctx, cancel = context.WithDeadline(ctx, deadline)
					defer cancel()
				}
				return DialPath(ctx, p, network, addr)
			},
			ForceAttemptHTTP2: true,
		},
	}
	defer client.CloseIdleConnections()
	u := fmt.Sprintf("%s/bot%s/getMe", apiBase, token)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
