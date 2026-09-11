package updater

import (
	"context"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Download constants.
const (
	defaultRawURL   = "https://raw.githubusercontent.com"
	repoRawPath     = "/%s/%s/refs/tags/%s/%s"
	downloadTimeout = 2 * time.Minute
	maxFileSize     = 50 * 1024 * 1024 // 50MB
)

// DownloadRelease downloads the release manifest, then every file it lists
// for this platform, then the daemon binaries. Cleans files/ before starting.
func (s *Service) DownloadRelease(ctx context.Context, release *Release) error {
	filesDir := s.getFilesDir()

	// Clean before download to ensure fresh state
	os.RemoveAll(filesDir)

	// Create directory structure
	if err := os.MkdirAll(filesDir, 0755); err != nil {
		return fmt.Errorf("create files directory: %w", err)
	}

	entries, err := s.fetchManifest(ctx, release.TagName)
	if err != nil {
		return err
	}

	// A selection of nothing would swap the daemon binaries and refresh no
	// script and no init file - worse than no update, and silent, because
	// downloadFile accepts a zero-byte HTTP 200 as a manifest. install.sh
	// refuses the same state on its side.
	files := manifestFilesFor(entries, s.getPlatform())
	if len(files) == 0 {
		return fmt.Errorf("release %s: files.manifest lists no file for platform %s", release.TagName, s.getPlatform())
	}

	for _, file := range files {
		if err := s.downloadScriptFile(ctx, release.TagName, file); err != nil {
			return fmt.Errorf("download %s: %w", file, err)
		}
	}

	// Download daemon binaries
	if err := s.downloadBinaries(ctx, release); err != nil {
		return fmt.Errorf("download binaries: %w", err)
	}

	return nil
}

// fetchManifest downloads router/files.manifest of the release into files/
// (the update script reads it from there) and parses it. A release without a
// manifest cannot be installed.
func (s *Service) fetchManifest(ctx context.Context, tag string) ([]ManifestEntry, error) {
	if err := s.downloadScriptFile(ctx, tag, manifestPath); err != nil {
		return nil, fmt.Errorf("release %s has no files.manifest: %w", tag, err)
	}
	f, err := os.Open(filepath.Join(s.getFilesDir(), "files.manifest"))
	if err != nil {
		return nil, fmt.Errorf("open downloaded files.manifest: %w", err)
	}
	defer f.Close()
	return parseManifest(f)
}

// getRawBaseURL returns the host that serves repository files at a tag. Tests
// point it at an httptest server, so the unit suite never reaches GitHub - the
// same seam baseURL provides for the API.
func (s *Service) getRawBaseURL() string {
	if s.rawBaseURL != "" {
		return s.rawBaseURL
	}
	return defaultRawURL
}

func (s *Service) downloadScriptFile(ctx context.Context, tag, file string) error {
	url := s.getRawBaseURL() + fmt.Sprintf(repoRawPath, repoOwner, repoName, tag, file)

	// Target: "router/opt/vpn-director/lib/common.sh" → "files/opt/vpn-director/lib/common.sh"
	target := filepath.Join(s.getFilesDir(), strings.TrimPrefix(file, "router"))

	return s.downloadFile(ctx, url, target)
}

// archAssetSuffix maps a Go architecture to the release asset suffix.
func archAssetSuffix(goarch string) (string, error) {
	switch goarch {
	case "arm64":
		return "arm64", nil
	case "arm":
		return "arm", nil
	default:
		return "", fmt.Errorf("unsupported architecture: %s", goarch)
	}
}

// getArchSuffix returns the release asset suffix for this build.
func (s *Service) getArchSuffix() (string, error) {
	if s.archSuffix != "" {
		return s.archSuffix, nil
	}
	return archAssetSuffix(runtime.GOARCH)
}

// downloadBinaries downloads one binary per daemon into files/<name>.
// A release missing any of them is a download error: installing a new bot
// next to an old Web UI leaves two halves of different versions on the router.
func (s *Service) downloadBinaries(ctx context.Context, release *Release) error {
	suffix, err := s.getArchSuffix()
	if err != nil {
		return err
	}

	for _, d := range Daemons {
		target := filepath.Join(s.getFilesDir(), d.Name)
		// Step 2 of a self-update is this daemon's binary of the release it
		// installs; fetching that asset again would download the same bytes.
		if s.selfBinary != "" && d.Name == s.daemon {
			if err := linkOrCopy(s.selfBinary, target); err != nil {
				return fmt.Errorf("take %s from %s: %w", d.Name, s.selfBinary, err)
			}
			continue
		}
		assetName := d.Name + "-" + suffix
		url := assetURL(release, assetName)
		if url == "" {
			return fmt.Errorf("asset %s not found in release", assetName)
		}
		if err := requireHTTPS(url); err != nil {
			return fmt.Errorf("asset %s: %w", assetName, err)
		}
		if err := s.downloadFile(ctx, url, target); err != nil {
			return fmt.Errorf("download %s: %w", assetName, err)
		}
	}
	return nil
}

// assetURL returns the download URL of the named asset, or "" when the
// release does not carry it.
func assetURL(release *Release, name string) string {
	for _, a := range release.Assets {
		if a.Name == name {
			return a.DownloadURL
		}
	}
	return ""
}

// requireHTTPS rejects an asset URL that is not https. The URL comes verbatim
// from the GitHub API response and the file it names is written to /opt and
// executed as root, so the one scheme downgrade that would hand a network
// attacker that file is refused here rather than trusted away.
func requireHTTPS(rawURL string) error {
	u, err := neturl.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse download url: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("refusing a non-https download url (scheme %q)", u.Scheme)
	}
	return nil
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

// linkOrCopy puts the file src at dst: a hard link when both sit on one
// filesystem, as the installer and files/ do in /tmp, else a copy.
func linkOrCopy(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	if err := os.Link(src, dst); err == nil {
		return nil
	}
	return copyExecutable(src, dst)
}

// copyExecutable copies src to dst, creating dst with mode 0755.
func copyExecutable(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	return out.Close()
}
