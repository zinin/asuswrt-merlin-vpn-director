package updater

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Download constants.
const (
	repoRawURL      = "https://raw.githubusercontent.com/%s/%s/refs/tags/%s/%s"
	downloadTimeout = 2 * time.Minute
	maxFileSize     = 50 * 1024 * 1024 // 50MB
)

// scriptFiles lists all files to download from the repository.
// NOTE: Keep in sync with install.sh file list.
// See: install.sh (search for "download_file" calls)
var scriptFiles = []string{
	"router/opt/vpn-director/vpn-director.sh",
	"router/opt/vpn-director/configure.sh",
	"router/opt/vpn-director/import_server_list.sh",
	"router/opt/vpn-director/setup_telegram_bot.sh",
	"router/opt/vpn-director/vpn-director.json.template",
	"router/opt/vpn-director/lib/common.sh",
	"router/opt/vpn-director/lib/firewall.sh",
	"router/opt/vpn-director/lib/config.sh",
	"router/opt/vpn-director/lib/ipset.sh",
	"router/opt/vpn-director/lib/tunnel.sh",
	"router/opt/vpn-director/lib/tproxy.sh",
	"router/opt/vpn-director/lib/send-email.sh",
	"router/opt/etc/xray/config.json.template",
	"router/opt/etc/init.d/S99vpn-director",
	"router/opt/etc/init.d/S98telegram-bot",
	"router/jffs/scripts/firewall-start",
	"router/jffs/scripts/wan-event",
}

// DownloadRelease downloads all files for the given release.
// Cleans files/ directory before starting.
func (s *Service) DownloadRelease(ctx context.Context, release *Release) error {
	filesDir := s.getFilesDir()

	// Clean before download to ensure fresh state
	os.RemoveAll(filesDir)

	// Create directory structure
	if err := os.MkdirAll(filesDir, 0755); err != nil {
		return fmt.Errorf("create files directory: %w", err)
	}

	// Download scripts
	for _, file := range scriptFiles {
		if err := s.downloadScriptFile(ctx, release.TagName, file); err != nil {
			return fmt.Errorf("download %s: %w", file, err)
		}
	}

	// Download bot binary
	if err := s.downloadBotBinary(ctx, release); err != nil {
		return fmt.Errorf("download bot binary: %w", err)
	}

	return nil
}

func (s *Service) downloadScriptFile(ctx context.Context, tag, file string) error {
	url := fmt.Sprintf(repoRawURL, repoOwner, repoName, tag, file)

	// Target: "router/opt/vpn-director/lib/common.sh" → "files/opt/vpn-director/lib/common.sh"
	target := filepath.Join(s.getFilesDir(), strings.TrimPrefix(file, "router"))

	return s.downloadFile(ctx, url, target)
}

func (s *Service) downloadBotBinary(ctx context.Context, release *Release) error {
	arch := runtime.GOARCH
	var assetName string

	switch arch {
	case "arm64":
		assetName = "telegram-bot-arm64"
	case "arm":
		assetName = "telegram-bot-arm"
	default:
		return fmt.Errorf("unsupported architecture: %s", arch)
	}

	// Find asset URL
	var downloadURL string
	for _, asset := range release.Assets {
		if asset.Name == assetName {
			downloadURL = asset.DownloadURL
			break
		}
	}
	if downloadURL == "" {
		return fmt.Errorf("binary for architecture %s not found in release", arch)
	}

	target := filepath.Join(s.getFilesDir(), "telegram-bot")
	return s.downloadFile(ctx, downloadURL, target)
}

func (s *Service) downloadFile(ctx context.Context, url, target string) error {
	// Per-request timeout
	ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()

	// Create parent directory
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "vpn-director-telegram-bot")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(target)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	// Limit download size to prevent memory exhaustion
	limitedReader := io.LimitReader(resp.Body, maxFileSize)
	written, err := io.Copy(f, limitedReader)
	if err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	// Check if we hit the limit (file was truncated)
	if written == maxFileSize {
		// Try to read one more byte - if successful, file was too large
		buf := make([]byte, 1)
		if n, _ := resp.Body.Read(buf); n > 0 {
			os.Remove(target)
			return fmt.Errorf("file exceeds maximum size (%d MB)", maxFileSize/1024/1024)
		}
	}

	return nil
}
