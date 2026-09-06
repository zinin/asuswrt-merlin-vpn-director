package webapi

import (
	"log/slog"
	"net"
	"net/http"
	"time"
)

// loginRequest is the expected JSON body for POST /api/login.
type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// handleLogin returns a handler that authenticates the user against the shadow
// file, creates a JWT, and sets it as an HttpOnly cookie.
func handleLogin(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loginRequest
		if err := decodeJSON(r, &req); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		ip := remoteIP(r)

		// Rate limit check.
		if !deps.loginLimiter.allow(ip) {
			jsonError(w, http.StatusTooManyRequests, "too many login attempts")
			return
		}

		// Verify credentials.
		ok, err := deps.Shadow.Verify(req.Username, req.Password)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "authentication error")
			return
		}
		if !ok {
			deps.loginLimiter.record(ip)
			jsonError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}

		// Bind the token to the password it was issued under: authMiddleware
		// re-checks this fingerprint on every request. Verify has just
		// succeeded, so an error here means the file changed under us between
		// the two reads - it became unreadable, or the account was locked. Log
		// it as the middleware does: a 500 with no log line leaves the operator
		// nothing to read.
		fingerprint, err := deps.Shadow.Fingerprint(req.Username)
		if err != nil {
			slog.Error("cannot read the password fingerprint", "error", err)
			jsonError(w, http.StatusInternalServerError, "authentication error")
			return
		}

		// Create JWT.
		token, err := deps.JWT.Create(req.Username, fingerprint)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "create token")
			return
		}

		// Set HttpOnly cookie.
		http.SetCookie(w, &http.Cookie{
			Name:     "token",
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteStrictMode,
			MaxAge:   int(24 * time.Hour / time.Second),
		})

		jsonOK(w, map[string]bool{"ok": true})
	}
}

// handleLogout clears the authentication cookie.
func handleLogout(w http.ResponseWriter, _ *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})

	jsonOK(w, map[string]bool{"ok": true})
}

// remoteIP extracts the IP address from the request's RemoteAddr,
// stripping the port number if present.
func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
