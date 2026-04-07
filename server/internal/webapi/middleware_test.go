package webapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/auth"
)

func newTestJWT(t *testing.T) *auth.JWTService {
	t.Helper()
	return auth.NewJWTService("test-secret-key-32bytes!!!!!!!!", 1*time.Hour)
}

func TestAuthMiddleware_ValidCookie(t *testing.T) {
	jwt := newTestJWT(t)
	token, err := jwt.Create("admin")
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	handler := authMiddleware(jwt)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/status", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: token})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestAuthMiddleware_ValidBearerHeader(t *testing.T) {
	jwt := newTestJWT(t)
	token, err := jwt.Create("admin")
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	handler := authMiddleware(jwt)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/status", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestAuthMiddleware_NoToken(t *testing.T) {
	jwt := newTestJWT(t)

	handler := authMiddleware(jwt)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/status", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestAuthMiddleware_ExpiredToken(t *testing.T) {
	// Create a JWT service with a very short duration.
	jwt := auth.NewJWTService("test-secret-key-32bytes!!!!!!!!", 1*time.Millisecond)
	token, err := jwt.Create("admin")
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	// Wait for token to expire.
	time.Sleep(10 * time.Millisecond)

	handler := authMiddleware(jwt)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/status", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: token})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestRateLimiter_FirstAttemptAllowed(t *testing.T) {
	rl := newRateLimiter(5, 1*time.Minute, 30*time.Second)

	if !rl.allow("192.168.1.1") {
		t.Error("first attempt should be allowed")
	}
}

func TestRateLimiter_SixthAttemptBlocked(t *testing.T) {
	rl := newRateLimiter(5, 1*time.Minute, 30*time.Second)

	ip := "10.0.0.1"
	for i := 0; i < 5; i++ {
		if !rl.allow(ip) {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
		rl.record(ip)
	}

	if rl.allow(ip) {
		t.Error("6th attempt should be blocked after 5 failed attempts")
	}
}

func TestRateLimiter_BlockedThenAllowedAfterLockout(t *testing.T) {
	// Use very short durations for testing.
	rl := newRateLimiter(2, 1*time.Second, 50*time.Millisecond)

	ip := "172.16.0.1"
	for i := 0; i < 2; i++ {
		rl.allow(ip)
		rl.record(ip)
	}

	if rl.allow(ip) {
		t.Error("should be blocked immediately after exceeding limit")
	}

	// Wait for lockout to expire.
	time.Sleep(60 * time.Millisecond)

	if !rl.allow(ip) {
		t.Error("should be allowed after lockout expires")
	}
}
