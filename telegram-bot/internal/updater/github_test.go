package updater

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetLatestRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify headers
		if r.Header.Get("Accept") != "application/vnd.github.v3+json" {
			t.Errorf("Expected Accept header 'application/vnd.github.v3+json', got %q", r.Header.Get("Accept"))
		}
		if r.Header.Get("User-Agent") != "vpn-director-telegram-bot" {
			t.Errorf("Expected User-Agent header 'vpn-director-telegram-bot', got %q", r.Header.Get("User-Agent"))
		}

		// Verify endpoint
		expectedPath := "/repos/zinin/asuswrt-merlin-vpn-director/releases/latest"
		if r.URL.Path != expectedPath {
			t.Errorf("Expected path %q, got %q", expectedPath, r.URL.Path)
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"tag_name": "v1.2.3",
			"assets": [
				{"name": "telegram-bot-arm64", "browser_download_url": "https://example.com/arm64"},
				{"name": "telegram-bot-arm", "browser_download_url": "https://example.com/arm"}
			]
		}`))
	}))
	defer server.Close()

	s := NewWithBaseURL(server.URL)
	release, err := s.GetLatestRelease(context.Background())
	if err != nil {
		t.Fatalf("GetLatestRelease() error = %v", err)
	}

	if release.TagName != "v1.2.3" {
		t.Errorf("TagName = %q, want %q", release.TagName, "v1.2.3")
	}
	if len(release.Assets) != 2 {
		t.Fatalf("len(Assets) = %d, want 2", len(release.Assets))
	}
	if release.Assets[0].Name != "telegram-bot-arm64" {
		t.Errorf("Assets[0].Name = %q, want %q", release.Assets[0].Name, "telegram-bot-arm64")
	}
	if release.Assets[0].DownloadURL != "https://example.com/arm64" {
		t.Errorf("Assets[0].DownloadURL = %q, want %q", release.Assets[0].DownloadURL, "https://example.com/arm64")
	}
	if release.Assets[1].Name != "telegram-bot-arm" {
		t.Errorf("Assets[1].Name = %q, want %q", release.Assets[1].Name, "telegram-bot-arm")
	}
}

func TestGetLatestRelease_IncludesBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := `{
			"tag_name": "v1.0.0",
			"body": "## Changelog\n- Fix bug\n- Add feature",
			"assets": []
		}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(response))
	}))
	defer server.Close()

	svc := NewWithBaseURL(server.URL)
	release, err := svc.GetLatestRelease(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "## Changelog\n- Fix bug\n- Add feature"
	if release.Body != expected {
		t.Errorf("expected body %q, got %q", expected, release.Body)
	}
}

func TestGetLatestRelease_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	s := NewWithBaseURL(server.URL)
	_, err := s.GetLatestRelease(context.Background())
	if err == nil {
		t.Error("Expected error for 404 response")
	}
}

func TestGetLatestRelease_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{invalid json`))
	}))
	defer server.Close()

	s := NewWithBaseURL(server.URL)
	_, err := s.GetLatestRelease(context.Background())
	if err == nil {
		t.Error("Expected error for invalid JSON response")
	}
}

func TestGetLatestRelease_EmptyAssets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"tag_name": "v1.0.0", "assets": []}`))
	}))
	defer server.Close()

	s := NewWithBaseURL(server.URL)
	release, err := s.GetLatestRelease(context.Background())
	if err != nil {
		t.Fatalf("GetLatestRelease() error = %v", err)
	}
	if release.TagName != "v1.0.0" {
		t.Errorf("TagName = %q, want %q", release.TagName, "v1.0.0")
	}
	if len(release.Assets) != 0 {
		t.Errorf("len(Assets) = %d, want 0", len(release.Assets))
	}
}

func TestShouldUpdate(t *testing.T) {
	s := New()

	tests := []struct {
		name           string
		currentVersion string
		latestTag      string
		want           bool
		wantErr        bool
	}{
		{
			name:           "older version should update",
			currentVersion: "v1.0.0",
			latestTag:      "v1.1.0",
			want:           true,
		},
		{
			name:           "same version should not update",
			currentVersion: "v1.1.0",
			latestTag:      "v1.1.0",
			want:           false,
		},
		{
			name:           "newer version should not update",
			currentVersion: "v1.2.0",
			latestTag:      "v1.1.0",
			want:           false,
		},
		{
			name:           "patch update",
			currentVersion: "v1.0.0",
			latestTag:      "v1.0.1",
			want:           true,
		},
		{
			name:           "major update",
			currentVersion: "v1.9.9",
			latestTag:      "v2.0.0",
			want:           true,
		},
		{
			name:           "without v prefix - older",
			currentVersion: "1.0.0",
			latestTag:      "v1.1.0",
			want:           true,
		},
		{
			name:           "without v prefix - same",
			currentVersion: "1.1.0",
			latestTag:      "1.1.0",
			want:           false,
		},
		{
			name:           "dev version should error",
			currentVersion: "dev",
			latestTag:      "v1.1.0",
			wantErr:        true,
		},
		{
			name:           "pre-release current version should error",
			currentVersion: "v1.2.3-rc1",
			latestTag:      "v1.2.4",
			wantErr:        true,
		},
		{
			name:           "pre-release latest tag should error",
			currentVersion: "v1.0.0",
			latestTag:      "v1.1.0-beta",
			wantErr:        true,
		},
		{
			name:           "invalid current version",
			currentVersion: "invalid",
			latestTag:      "v1.0.0",
			wantErr:        true,
		},
		{
			name:           "invalid latest tag",
			currentVersion: "v1.0.0",
			latestTag:      "invalid",
			wantErr:        true,
		},
		{
			name:           "empty current version",
			currentVersion: "",
			latestTag:      "v1.0.0",
			wantErr:        true,
		},
		{
			name:           "empty latest tag",
			currentVersion: "v1.0.0",
			latestTag:      "",
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.ShouldUpdate(tt.currentVersion, tt.latestTag)
			if (err != nil) != tt.wantErr {
				t.Errorf("ShouldUpdate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ShouldUpdate(%q, %q) = %v, want %v",
					tt.currentVersion, tt.latestTag, got, tt.want)
			}
		})
	}
}

func TestGetLatestRelease_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Slow response - wait for context to be cancelled
		time.Sleep(5 * time.Second)
		w.Write([]byte(`{"tag_name": "v1.0.0", "assets": []}`))
	}))
	defer server.Close()

	s := NewWithBaseURL(server.URL)

	// Cancel context after short delay
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := s.GetLatestRelease(ctx)
	if err == nil {
		t.Error("GetLatestRelease() should fail when context is cancelled")
	}
}

func TestGetLatestRelease_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Slow response exceeding the per-request timeout
		time.Sleep(5 * time.Second)
		w.Write([]byte(`{"tag_name": "v1.0.0", "assets": []}`))
	}))
	defer server.Close()

	s := NewWithBaseURL(server.URL)

	// Use a short parent context timeout to test timeout behavior
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := s.GetLatestRelease(ctx)
	if err == nil {
		t.Error("GetLatestRelease() should fail when request times out")
	}
}
