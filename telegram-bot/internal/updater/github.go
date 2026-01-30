package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// GitHub API constants.
const (
	repoOwner        = "zinin"
	repoName         = "asuswrt-merlin-vpn-director"
	defaultAPIURL    = "https://api.github.com"
	releasesEndpoint = "/repos/%s/%s/releases/latest"
	apiTimeout       = 30 * time.Second
)

// githubRelease represents the GitHub API response for releases/latest.
type githubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

// githubAsset represents a release asset in the GitHub API response.
type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// GetLatestRelease fetches the latest release info from GitHub API.
func (s *Service) GetLatestRelease(ctx context.Context) (*Release, error) {
	ctx, cancel := context.WithTimeout(ctx, apiTimeout)
	defer cancel()

	baseURL := s.baseURL
	if baseURL == "" {
		baseURL = defaultAPIURL
	}
	url := baseURL + fmt.Sprintf(releasesEndpoint, repoOwner, repoName)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "vpn-director-telegram-bot")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var ghRelease githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&ghRelease); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	release := &Release{
		TagName: ghRelease.TagName,
		Assets:  make([]Asset, len(ghRelease.Assets)),
	}
	for i, a := range ghRelease.Assets {
		release.Assets[i] = Asset{
			Name:        a.Name,
			DownloadURL: a.BrowserDownloadURL,
		}
	}

	return release, nil
}

// ShouldUpdate checks if currentVersion is older than latestTag.
// Returns an error if either version can't be parsed (dev handled by caller).
func (s *Service) ShouldUpdate(currentVersion, latestTag string) (bool, error) {
	current, err := ParseVersion(currentVersion)
	if err != nil {
		return false, fmt.Errorf("parse current version %q: %w", currentVersion, err)
	}

	latest, err := ParseVersion(latestTag)
	if err != nil {
		return false, fmt.Errorf("parse latest version %q: %w", latestTag, err)
	}

	return current.IsOlderThan(latest), nil
}
