package webapi

import (
	"io/fs"
	"net/http"
	"sync"
	"time"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/auth"
	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/service"
)

// Deps holds all dependencies required by the HTTP API handlers.
type Deps struct {
	Config       service.ConfigStore
	VPN          service.VPNDirector
	Xray         service.XrayGenerator
	Network      service.NetworkInfo
	Logs         service.LogReader
	Shadow       *auth.ShadowAuth
	JWT          *auth.JWTService
	Version      string
	Commit       string
	OpMutex      *sync.Mutex  // serializes mutating shell operations
	loginLimiter *rateLimiter // rate limiter for login endpoint
}

// NewRouter creates the top-level HTTP handler with all routes registered.
// staticFS provides the embedded Vue SPA assets; pass nil to disable SPA serving.
func NewRouter(deps *Deps, staticFS fs.FS) http.Handler {
	mux := http.NewServeMux()

	deps.loginLimiter = newRateLimiter(5, 1*time.Minute, 30*time.Second)

	// Public routes (no auth required).
	mux.HandleFunc("POST /api/login", handleLogin(deps))

	// Protected routes (require valid JWT).
	protectedMux := http.NewServeMux()
	registerProtectedRoutes(protectedMux, deps)

	authMW := authMiddleware(deps.JWT)
	mux.Handle("/api/", authMW(protectedMux))

	// SPA fallback: serve static files and fall back to index.html.
	if staticFS != nil {
		mux.Handle("/", spaHandler(staticFS))
	}

	return loggingMiddleware(mux)
}

// registerProtectedRoutes adds all authenticated API endpoints to the mux.
func registerProtectedRoutes(mux *http.ServeMux, deps *Deps) {
	// Auth
	mux.HandleFunc("POST /api/logout", handleLogout)

	// Status & control
	mux.HandleFunc("GET /api/status", handleStatus(deps))
	mux.HandleFunc("POST /api/apply", handleApply(deps))
	mux.HandleFunc("POST /api/restart", handleRestart(deps))
	mux.HandleFunc("POST /api/stop", handleStop(deps))

	// IPSets
	mux.HandleFunc("POST /api/ipsets/update", handleUpdateIPsets(deps))

	// Info
	mux.HandleFunc("GET /api/ip", handleIP(deps))
	mux.HandleFunc("GET /api/version", handleVersion(deps))

	// Servers
	mux.HandleFunc("GET /api/servers", handleListServers(deps))
	mux.HandleFunc("POST /api/servers/active", handleSelectServer(deps))
	mux.HandleFunc("POST /api/servers/import", handleImportServers(deps))

	// Clients
	mux.HandleFunc("GET /api/clients", handleListClients(deps))
	mux.HandleFunc("POST /api/clients", handleAddClient(deps))
	mux.HandleFunc("POST /api/clients/pause", handlePauseClient(deps))
	mux.HandleFunc("POST /api/clients/resume", handleResumeClient(deps))
	mux.HandleFunc("DELETE /api/clients", handleDeleteClient(deps))

	// Exclusions — sets
	mux.HandleFunc("GET /api/excludes/sets", handleListExcludeSets(deps))
	mux.HandleFunc("POST /api/excludes/sets", handleUpdateExcludeSets(deps))

	// Exclusions — IPs
	mux.HandleFunc("GET /api/excludes/ips", handleListExcludeIPs(deps))
	mux.HandleFunc("POST /api/excludes/ips", handleAddExcludeIP(deps))
	mux.HandleFunc("DELETE /api/excludes/ips", handleDeleteExcludeIP(deps))

	// Logs & config
	mux.HandleFunc("GET /api/logs", handleLogs(deps))
	mux.HandleFunc("GET /api/config", handleConfig(deps))

	// Self-update
	mux.HandleFunc("POST /api/update", handleUpdate(deps))
}

// spaHandler serves static files from the embedded filesystem. If a file is
// not found and the request path does not start with "/api/", it falls back to
// index.html so the Vue SPA router can handle the path.
func spaHandler(staticFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(staticFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to open the requested file.
		path := r.URL.Path
		if path == "/" {
			path = "index.html"
		} else {
			// Strip leading slash for fs.Open.
			path = path[1:]
		}

		f, err := staticFS.Open(path)
		if err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}

		// File not found — serve index.html for SPA routing.
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}
