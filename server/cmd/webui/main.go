package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/auth"
	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/devmode"
	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/paths"
	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/service"
	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/vpnconfig"
	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/webapi"
)

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

func main() {
	configPath := flag.String("config", "/opt/vpn-director/vpn-director.json", "path to vpn-director.json")
	shadowPath := flag.String("shadow", "/etc/shadow", "path to shadow file")
	devFlag := flag.Bool("dev", false, "run in development mode (HTTP, mock executor, testdata paths)")
	flag.Parse()

	// In dev mode, override defaults with testdata paths.
	var p paths.Paths
	if *devFlag {
		p = paths.DevPaths()
		if *configPath == "/opt/vpn-director/vpn-director.json" {
			*configPath = p.ScriptsDir + "/vpn-director.json"
		}
		if *shadowPath == "/etc/shadow" {
			*shadowPath = p.ScriptsDir + "/shadow"
		}
		// Validate testdata/dev exists before proceeding.
		if _, err := os.Stat(p.ScriptsDir); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Error: %s not found\n", p.ScriptsDir)
			fmt.Fprintf(os.Stderr, "Run from server/ directory: cd server && go run ./cmd/webui --dev\n")
			os.Exit(1)
		}
		ensureDevFiles(*configPath, *shadowPath, p.DefaultDataDir)
	} else {
		p = paths.Default()
	}

	slog.Info("starting VPN Director Web UI", "version", Version, "commit", Commit, "dev", *devFlag)

	// Load config
	vpnCfg, err := vpnconfig.LoadVPNDirectorConfig(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	if vpnCfg.WebUI.Port == 0 {
		vpnCfg.WebUI.Port = 8444
	}
	if vpnCfg.WebUI.CertFile == "" {
		vpnCfg.WebUI.CertFile = "/opt/vpn-director/certs/server.crt"
	}
	if vpnCfg.WebUI.KeyFile == "" {
		vpnCfg.WebUI.KeyFile = "/opt/vpn-director/certs/server.key"
	}

	// Auto-generate JWT secret if empty
	if vpnCfg.WebUI.JWTSecret == "" {
		slog.Warn("jwt_secret not set, generating random secret")
		secret := make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			slog.Error("failed to generate jwt secret", "error", err)
			os.Exit(1)
		}
		vpnCfg.WebUI.JWTSecret = base64.StdEncoding.EncodeToString(secret)
		if err := vpnconfig.SaveVPNDirectorConfig(*configPath, vpnCfg); err != nil {
			slog.Warn("failed to save auto-generated jwt_secret", "error", err)
			// Continue anyway — secret is in memory for this session
		}
	}

	// Services — in dev mode use devmode executor for mock shell responses.
	var executor service.ShellExecutor
	if *devFlag {
		executor = devmode.NewExecutor()
	}

	// Derive scripts directory from --config path so runtime reads/writes
	// honour the flag instead of hardcoding /opt/vpn-director.
	scriptsDir := filepath.Dir(*configPath)
	defaultDataDir := filepath.Join(scriptsDir, "data")

	configSvc := service.NewConfigService(scriptsDir, defaultDataDir)
	vpnSvc := service.NewVPNDirectorService(scriptsDir, executor)
	xraySvc := service.NewXrayService(p.XrayTemplate, p.XrayConfig)
	networkSvc := service.NewNetworkService(executor)
	logSvc := service.NewLogService(executor)

	// Auth
	shadowAuth := auth.NewShadowAuth(*shadowPath)
	jwtSvc := auth.NewJWTService(vpnCfg.WebUI.JWTSecret, 24*time.Hour)

	deps := &webapi.Deps{
		Config:  configSvc,
		VPN:     vpnSvc,
		Xray:    xraySvc,
		Network: networkSvc,
		Logs:    logSvc,
		Shadow:  shadowAuth,
		JWT:     jwtSvc,
		Version: Version,
		Commit:  Commit,
		OpMutex: &sync.Mutex{},
	}

	// Embedded SPA files
	var staticFS fs.FS
	sub, err := fs.Sub(staticFiles, "web/dist")
	if err != nil {
		slog.Warn("no embedded static files", "error", err)
	} else {
		staticFS = sub
	}

	// Start server
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	serverCfg := webapi.ServerConfig{
		Port:     vpnCfg.WebUI.Port,
		CertFile: vpnCfg.WebUI.CertFile,
		KeyFile:  vpnCfg.WebUI.KeyFile,
		DevMode:  *devFlag,
	}

	if *devFlag {
		slog.Info("dev mode: HTTP server, mock executor, testdata paths",
			"config", *configPath,
			"shadow", *shadowPath,
			"port", vpnCfg.WebUI.Port,
		)
	}

	if err := webapi.ListenAndServe(ctx, serverCfg, deps, staticFS); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

// ensureDevFiles creates default dev config and shadow files if they don't exist,
// so that `go run ./cmd/webui --dev` works out of the box.
func ensureDevFiles(configPath, shadowPath, dataDir string) {
	// Create data directory if needed.
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		slog.Warn("failed to create data dir", "path", dataDir, "error", err)
	}

	// Shadow file with admin:admin (SHA-256 crypt).
	if _, err := os.Stat(shadowPath); os.IsNotExist(err) {
		// Hash generated with: openssl passwd -5 -salt devsalt admin
		const devShadow = "admin:$5$devsalt$LMFogNzwzA8X4bCMYZf22bdOkeaX6VqsdOAtuYDFXYB:19000:0:99999:7:::\n"
		if err := os.WriteFile(shadowPath, []byte(devShadow), 0600); err != nil {
			slog.Warn("failed to create dev shadow file", "error", err)
		} else {
			slog.Info("created dev shadow file (admin:admin)", "path", shadowPath)
		}
	}

	// VPN Director config with webui section.
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		devConfig := &vpnconfig.VPNDirectorConfig{
			DataDir: dataDir,
			WebUI: vpnconfig.WebUIConfig{
				Port:      8444,
				JWTSecret: "dev-secret-not-for-production-use!!",
			},
			Xray: vpnconfig.XrayConfig{
				Clients:     []string{"192.168.50.0/24"},
				Servers:     []string{},
				ExcludeIPs:  []string{},
				ExcludeSets: []string{"ru"},
			},
			TunnelDirector: vpnconfig.TunnelDirectorConfig{
				Tunnels: map[string]vpnconfig.TunnelConfig{},
			},
		}
		if err := vpnconfig.SaveVPNDirectorConfig(configPath, devConfig); err != nil {
			slog.Warn("failed to create dev config", "error", err)
		} else {
			slog.Info("created dev vpn-director.json", "path", configPath)
		}
	}
}
