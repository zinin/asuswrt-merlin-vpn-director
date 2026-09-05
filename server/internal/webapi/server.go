package webapi

import (
	"context"
	"crypto/tls"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"time"
)

// writeTimeout bounds how long a single response may take to be written. It is
// minutes rather than seconds because every mutating handler runs
// vpn-director.sh synchronously: `vpn-director.sh update` re-downloads every
// configured country set, and one country can fall through three download
// sources at up to 270 seconds. A seconds-scale deadline tears the connection
// down after the save and the apply have both completed, losing the
// {"saved": true} body the front end needs in order to refresh. The value stays
// finite so a slow-reading client cannot pin a connection forever, and it is
// deliberately generous so a per-command timeout can later cap the shell run
// below this deadline.
const writeTimeout = 10 * time.Minute

// ServerConfig holds HTTP/HTTPS server configuration.
type ServerConfig struct {
	Port     int
	CertFile string
	KeyFile  string
	DevMode  bool // when true, use plain HTTP instead of TLS
}

// ListenAndServe starts the HTTP/HTTPS server and blocks until ctx is cancelled
// or an unrecoverable error occurs. When cfg.DevMode is true it uses plain HTTP;
// otherwise it requires TLS certificates. On context cancellation it performs a
// graceful shutdown with a 5-second deadline.
func ListenAndServe(ctx context.Context, cfg ServerConfig, deps *Deps, staticFS fs.FS) error {
	router := NewRouter(deps, staticFS)

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MB
	}
	if !cfg.DevMode {
		server.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}

	errCh := make(chan error, 1)
	go func() {
		if cfg.DevMode {
			slog.Info("starting HTTP server (dev mode)", "addr", server.Addr)
			errCh <- server.ListenAndServe()
		} else {
			slog.Info("starting HTTPS server", "addr", server.Addr)
			errCh <- server.ListenAndServeTLS(cfg.CertFile, cfg.KeyFile)
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		slog.Info("shutting down server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}
