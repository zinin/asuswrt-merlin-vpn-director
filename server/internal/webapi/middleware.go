package webapi

import (
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/auth"
)

// authMiddleware returns HTTP middleware that validates JWT tokens.
// It checks the "token" cookie first, then the Authorization: Bearer header.
// Returns 401 if no valid token is found.
func authMiddleware(jwt *auth.JWTService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractToken(r)
			if token == "" {
				jsonError(w, http.StatusUnauthorized, "authentication required")
				return
			}

			_, err := jwt.Validate(token)
			if err != nil {
				jsonError(w, http.StatusUnauthorized, "invalid or expired token")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// extractToken retrieves the JWT from the request. It checks the "token"
// cookie first, then falls back to the Authorization: Bearer header.
func extractToken(r *http.Request) string {
	if cookie, err := r.Cookie("token"); err == nil && cookie.Value != "" {
		return cookie.Value
	}

	header := r.Header.Get("Authorization")
	if strings.HasPrefix(header, "Bearer ") {
		return strings.TrimPrefix(header, "Bearer ")
	}

	return ""
}

// attemptInfo tracks failed login attempts from a single IP address.
type attemptInfo struct {
	count     int
	firstSeen time.Time
	lockedAt  time.Time
}

// rateLimiter enforces per-IP rate limits on failed login attempts.
type rateLimiter struct {
	mu          sync.Mutex
	attempts    map[string]*attemptInfo
	maxAttempts int
	window      time.Duration
	lockout     time.Duration
}

// newRateLimiter creates a rate limiter that allows maxAttempts failed attempts
// within window duration, then locks out the IP for lockout duration.
// It starts a background goroutine to clean up stale entries every minute.
func newRateLimiter(maxAttempts int, window, lockout time.Duration) *rateLimiter {
	rl := &rateLimiter{
		attempts:    make(map[string]*attemptInfo),
		maxAttempts: maxAttempts,
		window:      window,
		lockout:     lockout,
	}
	go rl.cleanupLoop()
	return rl
}

// allow returns true if the IP is permitted to attempt a login.
func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	info, ok := rl.attempts[ip]
	if !ok {
		return true
	}

	now := time.Now()

	// If currently locked out, check if lockout has expired.
	if !info.lockedAt.IsZero() {
		if now.Before(info.lockedAt.Add(rl.lockout)) {
			return false
		}
		// Lockout expired — reset.
		delete(rl.attempts, ip)
		return true
	}

	// If the window has expired, reset.
	if now.After(info.firstSeen.Add(rl.window)) {
		delete(rl.attempts, ip)
		return true
	}

	return true
}

// record registers a failed login attempt for the given IP.
func (rl *rateLimiter) record(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	info, ok := rl.attempts[ip]
	if !ok {
		rl.attempts[ip] = &attemptInfo{
			count:     1,
			firstSeen: now,
		}
		return
	}

	// If the window expired, start fresh.
	if now.After(info.firstSeen.Add(rl.window)) {
		rl.attempts[ip] = &attemptInfo{
			count:     1,
			firstSeen: now,
		}
		return
	}

	info.count++
	if info.count >= rl.maxAttempts {
		info.lockedAt = now
	}
}

// cleanupLoop periodically removes stale entries from the rate limiter.
func (rl *rateLimiter) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.cleanup()
	}
}

// cleanup removes entries whose window + lockout have both expired.
func (rl *rateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for ip, info := range rl.attempts {
		expiry := info.firstSeen.Add(rl.window)
		if !info.lockedAt.IsZero() {
			lockExpiry := info.lockedAt.Add(rl.lockout)
			if lockExpiry.After(expiry) {
				expiry = lockExpiry
			}
		}
		if now.After(expiry) {
			delete(rl.attempts, ip)
		}
	}
}

// loggingMiddleware logs each HTTP request's method, path, duration,
// status code, and remote address using slog.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		slog.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"duration", time.Since(start).String(),
			"remote", r.RemoteAddr,
		)
	})
}

// statusWriter wraps http.ResponseWriter to capture the status code.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}
