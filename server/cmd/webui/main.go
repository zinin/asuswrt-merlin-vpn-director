package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/auth"
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
	flag.Parse()

	slog.Info("starting VPN Director Web UI", "version", Version, "commit", Commit)

	// Load config
	vpnCfg, err := vpnconfig.LoadVPNDirectorConfig(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	if vpnCfg.WebUI.Port == 0 {
		vpnCfg.WebUI.Port = 8444
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

	// Services (same pattern as Telegram bot).
	// Passing nil executor uses the default shell.Exec wrapper.
	p := paths.Default()
	configSvc := service.NewConfigService(p.ScriptsDir, p.DefaultDataDir)
	vpnSvc := service.NewVPNDirectorService(p.ScriptsDir, nil)
	xraySvc := service.NewXrayService(p.XrayTemplate, p.XrayConfig)
	networkSvc := service.NewNetworkService(nil)
	logSvc := service.NewLogService(nil)

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
	}

	if err := webapi.ListenAndServe(ctx, serverCfg, deps, staticFS); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
