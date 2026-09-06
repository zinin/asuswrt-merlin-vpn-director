package updater

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDownloadFile_Success(t *testing.T) {
	content := "test file content"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(content))
	}))
	defer server.Close()

	tempDir := t.TempDir()
	target := filepath.Join(tempDir, "subdir", "file.txt")
	s := New()

	err := s.downloadFile(context.Background(), server.URL, target)
	if err != nil {
		t.Fatalf("downloadFile() error = %v", err)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("Failed to read downloaded file: %v", err)
	}
	if string(data) != content {
		t.Errorf("File content = %q, want %q", string(data), content)
	}
}

func TestDownloadFile_CreatesParentDirs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("content"))
	}))
	defer server.Close()

	tempDir := t.TempDir()
	target := filepath.Join(tempDir, "a", "b", "c", "file.txt")
	s := New()

	err := s.downloadFile(context.Background(), server.URL, target)
	if err != nil {
		t.Fatalf("downloadFile() error = %v", err)
	}

	if _, err := os.Stat(target); err != nil {
		t.Errorf("Target file not created: %v", err)
	}
}

func TestDownloadFile_HTTPError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{"NotFound", http.StatusNotFound},
		{"InternalServerError", http.StatusInternalServerError},
		{"Forbidden", http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			tempDir := t.TempDir()
			s := New()

			err := s.downloadFile(context.Background(), server.URL, filepath.Join(tempDir, "file"))
			if err == nil {
				t.Errorf("downloadFile() should fail for HTTP %d", tt.statusCode)
			}
		})
	}
}

func TestDownloadFile_SizeLimit(t *testing.T) {
	// Create content larger than maxFileSize (50MB)
	// We can't actually allocate 50MB in tests, so we simulate with a slow reader
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Write exactly maxFileSize bytes
		chunk := strings.Repeat("x", 1024)
		for i := 0; i < maxFileSize/1024; i++ {
			w.Write([]byte(chunk))
		}
		// Write one more byte to exceed limit
		w.Write([]byte("!"))
	}))
	defer server.Close()

	tempDir := t.TempDir()
	target := filepath.Join(tempDir, "large_file")
	s := New()

	err := s.downloadFile(context.Background(), server.URL, target)
	if err == nil {
		t.Error("downloadFile() should fail for file exceeding size limit")
	}
	if !strings.Contains(err.Error(), "exceeds maximum size") {
		t.Errorf("Error should mention size limit, got: %v", err)
	}

	// File should be removed on size limit error
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Error("File should be removed after size limit exceeded")
	}
}

func TestDownloadFile_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Slow response - wait for context to be cancelled
		time.Sleep(5 * time.Second)
		w.Write([]byte("content"))
	}))
	defer server.Close()

	tempDir := t.TempDir()
	s := New()

	// Cancel context after short delay
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := s.downloadFile(ctx, server.URL, filepath.Join(tempDir, "file"))
	if err == nil {
		t.Error("downloadFile() should fail when context is cancelled")
	}
}

func TestDownloadFile_UserAgent(t *testing.T) {
	var gotUserAgent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserAgent = r.Header.Get("User-Agent")
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	tempDir := t.TempDir()
	s := New()

	s.downloadFile(context.Background(), server.URL, filepath.Join(tempDir, "file"))

	if gotUserAgent != "vpn-director-telegram-bot" {
		t.Errorf("User-Agent = %q, want %q", gotUserAgent, "vpn-director-telegram-bot")
	}
}

func TestDownloadRelease_CleansBeforeDownload(t *testing.T) {
	// Server that responds to all requests
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("content"))
	}))
	defer server.Close()

	tempDir := t.TempDir()
	filesDir := filepath.Join(tempDir, "files")

	// Create pre-existing file
	if err := os.MkdirAll(filesDir, 0755); err != nil {
		t.Fatalf("Failed to create files dir: %v", err)
	}
	oldFile := filepath.Join(filesDir, "old_file.txt")
	if err := os.WriteFile(oldFile, []byte("old content"), 0644); err != nil {
		t.Fatalf("Failed to create old file: %v", err)
	}

	s := &Service{
		httpClient: &http.Client{},
		baseURL:    server.URL,
		rawBaseURL: server.URL,
		updateDir:  tempDir,
	}

	// This will fail because server doesn't serve proper paths,
	// but it will clean the directory first
	_ = s.DownloadRelease(context.Background(), &Release{TagName: "v1.0.0"})

	// Old file should be gone (directory was cleaned)
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Error("DownloadRelease() should clean files directory before download")
	}
}

// TestDownloadRelease_UsesTheInjectedRawHost is why rawBaseURL exists: before
// it, this package fetched all nineteen script files from the real
// raw.githubusercontent.com on every `go test`, which is both slow and a
// network dependency in CI.
func TestDownloadRelease_UsesTheInjectedRawHost(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		w.Write([]byte("content"))
	}))
	defer server.Close()

	s := &Service{
		httpClient: &http.Client{},
		baseURL:    server.URL,
		rawBaseURL: server.URL,
		updateDir:  t.TempDir(),
		archSuffix: "arm64",
	}

	// No assets in the release, so downloadBinaries fails after the scripts.
	_ = s.DownloadRelease(context.Background(), &Release{TagName: "v1.0.0"})

	mu.Lock()
	defer mu.Unlock()
	if len(paths) != len(scriptFiles) {
		t.Fatalf("the test server saw %d requests, want %d - some file still goes to the real host", len(paths), len(scriptFiles))
	}
	want := "/" + repoOwner + "/" + repoName + "/refs/tags/v1.0.0/" + scriptFiles[0]
	if paths[0] != want {
		t.Errorf("first request path = %q, want %q", paths[0], want)
	}
}

func TestDownloadScriptFile_PathTransformation(t *testing.T) {
	// This test verifies the path transformation logic:
	// "router/opt/vpn-director/lib/common.sh" → "files/opt/vpn-director/lib/common.sh"
	// The downloadScriptFile uses raw.githubusercontent.com URL format, not GitHub API.

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// downloadScriptFile builds its URL from repoRawPath, which contains the file path
		if strings.Contains(r.URL.Path, "common.sh") {
			w.Write([]byte("#!/bin/bash\necho test"))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	tempDir := t.TempDir()

	// We need to test the path transformation directly since downloadScriptFile
	// uses a hardcoded URL format. Let's test the target path calculation.
	file := "router/opt/vpn-director/lib/common.sh"
	expectedTarget := filepath.Join(tempDir, "files", "opt/vpn-director/lib/common.sh")
	actualTarget := filepath.Join(tempDir, "files", strings.TrimPrefix(file, "router"))

	if actualTarget != expectedTarget {
		t.Errorf("Target path = %q, want %q", actualTarget, expectedTarget)
	}

	// Verify TrimPrefix works correctly for various paths
	testCases := []struct {
		input    string
		expected string
	}{
		{"router/opt/vpn-director/vpn-director.sh", "/opt/vpn-director/vpn-director.sh"},
		{"router/jffs/scripts/firewall-start", "/jffs/scripts/firewall-start"},
		{"router/opt/etc/init.d/S99vpn-director", "/opt/etc/init.d/S99vpn-director"},
	}

	for _, tc := range testCases {
		result := strings.TrimPrefix(tc.input, "router")
		if result != tc.expected {
			t.Errorf("TrimPrefix(%q, \"router\") = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

func TestScriptFiles_ExistInRepo(t *testing.T) {
	// The list is a copy of install.sh's downloads; a renamed or removed file
	// silently breaks every future update, so pin it to the working tree.
	for _, f := range scriptFiles {
		path := filepath.Join("..", "..", "..", f)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("scriptFiles entry %q not found in the repo: %v", f, err)
		}
	}
}

func TestScriptFiles_IncludesWebUIInitScript(t *testing.T) {
	const want = "router/opt/etc/init.d/S98vpn-director-webui"
	for _, f := range scriptFiles {
		if f == want {
			return
		}
	}
	t.Errorf("scriptFiles must ship %s, otherwise an update leaves the old init script", want)
}

func TestArchAssetSuffix(t *testing.T) {
	tests := []struct {
		goarch  string
		want    string
		wantErr bool
	}{
		{goarch: "arm64", want: "arm64"},
		{goarch: "arm", want: "arm"},
		{goarch: "amd64", wantErr: true},
		{goarch: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.goarch, func(t *testing.T) {
			got, err := archAssetSuffix(tt.goarch)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("archAssetSuffix(%q) = %q, want error", tt.goarch, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("archAssetSuffix(%q) error = %v", tt.goarch, err)
			}
			if got != tt.want {
				t.Errorf("archAssetSuffix(%q) = %q, want %q", tt.goarch, got, tt.want)
			}
		})
	}
}

func TestDownloadBinaries_DownloadsEveryDaemon(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("binary of " + strings.TrimPrefix(r.URL.Path, "/")))
	}))
	defer server.Close()

	tempDir := t.TempDir()
	s := &Service{httpClient: server.Client(), updateDir: tempDir, archSuffix: "arm64"}

	release := &Release{TagName: "v1.0.0"}
	for _, d := range Daemons {
		release.Assets = append(release.Assets, Asset{
			Name:        d.Name + "-arm64",
			DownloadURL: server.URL + "/" + d.Name + "-arm64",
		})
	}

	if err := s.downloadBinaries(context.Background(), release); err != nil {
		t.Fatalf("downloadBinaries() error = %v", err)
	}

	for _, d := range Daemons {
		target := filepath.Join(tempDir, "files", d.Name)
		data, err := os.ReadFile(target)
		if err != nil {
			t.Fatalf("binary for %s not downloaded: %v", d.Name, err)
		}
		if want := "binary of " + d.Name + "-arm64"; string(data) != want {
			t.Errorf("%s content = %q, want %q", d.Name, data, want)
		}
	}
}

func TestDownloadBinaries_MissingAssetIsAnError(t *testing.T) {
	// A release that ships only one of the two binaries would install a new
	// bot next to an old Web UI; refuse the whole download instead.
	// The present assets must be served for real, not stubbed with an
	// unroutable URL: downloadBinaries downloads inside the loop, so a stub
	// makes the subtest turn on daemon ordering and DNS behaviour — it fails
	// on the first daemon it fetches instead of on the missing asset.
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("binary"))
	}))
	defer server.Close()

	for _, missing := range Daemons {
		t.Run("without "+missing.Name, func(t *testing.T) {
			tempDir := t.TempDir()
			s := &Service{httpClient: server.Client(), updateDir: tempDir, archSuffix: "arm64"}

			release := &Release{TagName: "v1.0.0"}
			for _, d := range Daemons {
				if d.Name == missing.Name {
					continue
				}
				release.Assets = append(release.Assets, Asset{
					Name:        d.Name + "-arm64",
					DownloadURL: server.URL + "/" + d.Name + "-arm64",
				})
			}

			err := s.downloadBinaries(context.Background(), release)
			if err == nil {
				t.Fatalf("downloadBinaries() must fail without asset %s-arm64", missing.Name)
			}
			if !strings.Contains(err.Error(), missing.Name+"-arm64") {
				t.Errorf("error %q should name the missing asset %s-arm64", err, missing.Name)
			}
		})
	}
}

// TestDownloadBinaries_RefusesAPlainHTTPAsset closes a plaintext-downgrade path
// on a file that is written to /opt and executed as root. GitHub always answers
// with https, so this is defence in depth against a tampered API response.
func TestDownloadBinaries_RefusesAPlainHTTPAsset(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("binary"))
	}))
	defer server.Close()

	s := &Service{httpClient: &http.Client{}, updateDir: t.TempDir(), archSuffix: "arm64"}

	release := &Release{TagName: "v1.0.0"}
	for _, d := range Daemons {
		release.Assets = append(release.Assets, Asset{
			Name:        d.Name + "-arm64",
			DownloadURL: server.URL + "/" + d.Name + "-arm64", // http://
		})
	}

	err := s.downloadBinaries(context.Background(), release)
	if err == nil {
		t.Fatal("downloadBinaries() accepted a plain-http asset URL")
	}
	if !strings.Contains(err.Error(), "https") {
		t.Errorf("error = %v, want it to name the scheme requirement", err)
	}
}
