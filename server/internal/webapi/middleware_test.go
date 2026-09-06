package webapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/auth"
)

func newTestJWT(t *testing.T) *auth.JWTService {
	t.Helper()
	return auth.NewJWTService("test-secret-key-32bytes!!!!!!!!", 1*time.Hour)
}

// newAuthFixture returns a ShadowAuth over a one-user shadow file, the path of
// that file so a test can rewrite it, and the fingerprint of the hash in it.
func newAuthFixture(t *testing.T) (*auth.ShadowAuth, string, string) {
	t.Helper()
	path := writeShadowFixture(t, "admin:"+testSHA256Hash+":19000:0:99999:7:::\n")
	shadow := auth.NewShadowAuth(path)
	fp, err := shadow.Fingerprint("admin")
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	return shadow, path, fp
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestAuthMiddleware_ValidCookie(t *testing.T) {
	jwt := newTestJWT(t)
	shadow, _, fp := newAuthFixture(t)
	token, err := jwt.Create("admin", fp)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	handler := authMiddleware(jwt, shadow)(okHandler())

	req := httptest.NewRequest("GET", "/api/status", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: token})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAuthMiddleware_ValidBearerHeader(t *testing.T) {
	jwt := newTestJWT(t)
	shadow, _, fp := newAuthFixture(t)
	token, err := jwt.Create("admin", fp)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	handler := authMiddleware(jwt, shadow)(okHandler())

	req := httptest.NewRequest("GET", "/api/status", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAuthMiddleware_NoToken(t *testing.T) {
	jwt := newTestJWT(t)
	shadow, _, _ := newAuthFixture(t)

	handler := authMiddleware(jwt, shadow)(okHandler())

	req := httptest.NewRequest("GET", "/api/status", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestAuthMiddleware_ExpiredToken(t *testing.T) {
	jwt := auth.NewJWTService("test-secret-key-32bytes!!!!!!!!", 1*time.Millisecond)
	shadow, _, fp := newAuthFixture(t)
	token, err := jwt.Create("admin", fp)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	handler := authMiddleware(jwt, shadow)(okHandler())

	req := httptest.NewRequest("GET", "/api/status", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: token})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

// TestAuthMiddleware_RejectsTokenAfterPasswordChange is the whole point of
// spec 7.1: the token is still signed correctly and still unexpired, and it
// must stop working the moment the router password changes.
func TestAuthMiddleware_RejectsTokenAfterPasswordChange(t *testing.T) {
	jwt := newTestJWT(t)
	shadow, path, fp := newAuthFixture(t)
	token, err := jwt.Create("admin", fp)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	// The router password changes: same user, different stored hash.
	const otherHash = "$6$testsalt$zcc0po6c786cz9LdMIli0E4Zox6uXK6Khb536rxCF/JO..UDVYHeg9zCKnpkm0FyMFumVno4DCKiS8pQLicRP."
	if err := os.WriteFile(path, []byte("admin:"+otherHash+":19000:0:99999:7:::\n"), 0600); err != nil {
		t.Fatalf("rewrite shadow: %v", err)
	}

	handler := authMiddleware(jwt, shadow)(okHandler())

	req := httptest.NewRequest("GET", "/api/status", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: token})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 after the password changed, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "password changed") {
		t.Errorf("body = %s, want it to say the password changed", rec.Body.String())
	}
}

// TestAuthMiddleware_RejectsTokenWithoutFingerprint covers every session issued
// before this release: signed by the same secret, but carrying no pwh claim.
func TestAuthMiddleware_RejectsTokenWithoutFingerprint(t *testing.T) {
	const secret = "test-secret-key-32bytes!!!!!!!!"
	svc := auth.NewJWTService(secret, 1*time.Hour)
	shadow, _, _ := newAuthFixture(t)

	now := time.Now()
	legacy := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, jwtlib.MapClaims{
		"sub": "admin",
		"iat": jwtlib.NewNumericDate(now),
		"exp": jwtlib.NewNumericDate(now.Add(time.Hour)),
	})
	signed, err := legacy.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign legacy token: %v", err)
	}

	handler := authMiddleware(svc, shadow)(okHandler())

	req := httptest.NewRequest("GET", "/api/status", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: signed})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for a token without pwh, got %d", rec.Code)
	}
}

func TestAuthMiddleware_UnknownUserIs401(t *testing.T) {
	jwt := newTestJWT(t)
	shadow, path, fp := newAuthFixture(t)
	token, err := jwt.Create("admin", fp)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	// The account disappears from the shadow file.
	if err := os.WriteFile(path, []byte("root:"+testSHA256Hash+":19000:0:99999:7:::\n"), 0600); err != nil {
		t.Fatalf("rewrite shadow: %v", err)
	}

	handler := authMiddleware(jwt, shadow)(okHandler())

	req := httptest.NewRequest("GET", "/api/status", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: token})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for a subject that no longer exists, got %d", rec.Code)
	}
}

// TestAuthMiddleware_UnreadableShadowIs500 keeps the SPA out of a reload loop:
// a 401 here would bounce it to the login page, where login fails with 500 for
// the same reason and the user learns nothing.
func TestAuthMiddleware_UnreadableShadowIs500(t *testing.T) {
	jwt := newTestJWT(t)
	shadow, path, fp := newAuthFixture(t)
	token, err := jwt.Create("admin", fp)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove shadow: %v", err)
	}

	handler := authMiddleware(jwt, shadow)(okHandler())

	req := httptest.NewRequest("GET", "/api/status", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: token})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 when /etc/shadow cannot be read, got %d", rec.Code)
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
