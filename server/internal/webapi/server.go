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

// ServerConfig holds HTTPS server configuration.
type ServerConfig struct {
	Port     int
	CertFile string
	KeyFile  string
}

// ListenAndServe starts the HTTPS server and blocks until ctx is cancelled or
// an unrecoverable error occurs. On context cancellation it performs a graceful
// shutdown with a 5-second deadline.
func ListenAndServe(ctx context.Context, cfg ServerConfig, deps *Deps, staticFS fs.FS) error {
	router := NewRouter(deps, staticFS)

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MB
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("starting HTTPS server", "addr", server.Addr)
		errCh <- server.ListenAndServeTLS(cfg.CertFile, cfg.KeyFile)
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
