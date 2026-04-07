package webapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/auth"
)

// sha256Hash is a known SHA-256 shadow hash for password "testpass".
const testSHA256Hash = "$5$testsalt$GR6PqdknD2fHavVjM//Q.4Qni8EXZKnxS838p5GC9r5"

// writeShadowFixture creates a temporary shadow file and returns its path.
func writeShadowFixture(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "shadow")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHandleLogin_ValidCredentials(t *testing.T) {
	shadowPath := writeShadowFixture(t, "admin:"+testSHA256Hash+":19000:0:99999:7:::\n")

	deps := newTestDeps(t)
	deps.Shadow = auth.NewShadowAuth(shadowPath)

	handler := handleLogin(deps)

	body := `{"username":"admin","password":"testpass"}`
	req := httptest.NewRequest("POST", "/api/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Check response body contains token.
	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["token"] == "" {
		t.Error("expected non-empty token in response body")
	}

	// Check HttpOnly cookie is set.
	cookies := rec.Result().Cookies()
	var tokenCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "token" {
			tokenCookie = c
			break
		}
	}
	if tokenCookie == nil {
		t.Fatal("expected 'token' cookie to be set")
	}
	if !tokenCookie.HttpOnly {
		t.Error("expected HttpOnly flag on token cookie")
	}
	if tokenCookie.SameSite != http.SameSiteStrictMode {
		t.Error("expected SameSite=Strict on token cookie")
	}
	if tokenCookie.Value != resp["token"] {
		t.Error("expected cookie token to match response body token")
	}
}

func TestHandleLogin_InvalidCredentials(t *testing.T) {
	shadowPath := writeShadowFixture(t, "admin:"+testSHA256Hash+":19000:0:99999:7:::\n")

	deps := newTestDeps(t)
	deps.Shadow = auth.NewShadowAuth(shadowPath)

	handler := handleLogin(deps)

	body := `{"username":"admin","password":"wrongpassword"}`
	req := httptest.NewRequest("POST", "/api/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleLogin_RateLimited(t *testing.T) {
	shadowPath := writeShadowFixture(t, "admin:"+testSHA256Hash+":19000:0:99999:7:::\n")

	deps := newTestDeps(t)
	deps.Shadow = auth.NewShadowAuth(shadowPath)
	// Use a very low threshold for testing.
	deps.loginLimiter = newRateLimiter(2, 1*time.Minute, 30*time.Second)

	handler := handleLogin(deps)

	// Exhaust rate limit with failed attempts.
	for i := 0; i < 2; i++ {
		body := `{"username":"admin","password":"wrongpassword"}`
		req := httptest.NewRequest("POST", "/api/login", strings.NewReader(body))
		req.RemoteAddr = "10.0.0.1:12345"
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	// Next attempt should be rate limited.
	body := `{"username":"admin","password":"testpass"}`
	req := httptest.NewRequest("POST", "/api/login", strings.NewReader(body))
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleLogin_InvalidJSON(t *testing.T) {
	deps := newTestDeps(t)
	handler := handleLogin(deps)

	req := httptest.NewRequest("POST", "/api/login", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleLogout(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/logout", nil)
	rec := httptest.NewRecorder()

	handleLogout(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Check response body.
	var resp map[string]bool
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp["ok"] {
		t.Error("expected ok: true in response")
	}

	// Check cookie is cleared.
	cookies := rec.Result().Cookies()
	var tokenCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "token" {
			tokenCookie = c
			break
		}
	}
	if tokenCookie == nil {
		t.Fatal("expected 'token' cookie to be set (for clearing)")
	}
	if tokenCookie.MaxAge != -1 {
		t.Errorf("expected MaxAge -1 to clear cookie, got %d", tokenCookie.MaxAge)
	}
}

func TestRemoteIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		want       string
	}{
		{"ip with port", "192.168.1.1:12345", "192.168.1.1"},
		{"ip without port", "192.168.1.1", "192.168.1.1"},
		{"ipv6 with port", "[::1]:12345", "::1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = tt.remoteAddr
			got := remoteIP(r)
			if got != tt.want {
				t.Errorf("remoteIP() = %q, want %q", got, tt.want)
			}
		})
	}
}
