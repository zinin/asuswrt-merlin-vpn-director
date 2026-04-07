# VPN Director Web UI — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a standalone Go HTTPS server with Vue 3 SPA providing full VPN Director management through a web browser, with feature parity to the Telegram bot.

**Architecture:** Go HTTP server (`cmd/webui`) reuses existing `internal/service/*` layer shared with the Telegram bot. Vue 3 SPA is embedded in the Go binary via `go:embed`. JWT authentication verifies credentials against `/etc/shadow`. HTTPS with self-signed certificate.

**Tech Stack:** Go 1.25, Vue 3.5 + TypeScript + Vite, JWT (HMAC-SHA256), TLS

**Design spec:** `docs/superpowers/specs/2026-04-07-webui-design.md`

---

## File Structure

### Go Backend (new/modified files in `server/`)

| File | Responsibility |
|------|----------------|
| `server/cmd/webui/main.go` | Entry point: config loading, service init, HTTP server startup |
| `server/cmd/webui/embed.go` | `go:embed` directive for Vue SPA dist files |
| `server/internal/auth/shadow.go` | Parse `/etc/shadow`, verify password via SHA-256 crypt |
| `server/internal/auth/shadow_test.go` | Tests for shadow parsing |
| `server/internal/auth/jwt.go` | JWT token creation, validation, middleware |
| `server/internal/auth/jwt_test.go` | Tests for JWT |
| `server/internal/webapi/server.go` | HTTP server setup, TLS config, graceful shutdown |
| `server/internal/webapi/router.go` | Route registration, middleware chain |
| `server/internal/webapi/middleware.go` | Auth middleware, logging middleware, CORS |
| `server/internal/webapi/middleware_test.go` | Tests for middleware |
| `server/internal/webapi/handler_auth.go` | Login/logout handlers |
| `server/internal/webapi/handler_auth_test.go` | Tests |
| `server/internal/webapi/handler_status.go` | Status, restart, stop, IP, version handlers |
| `server/internal/webapi/handler_status_test.go` | Tests |
| `server/internal/webapi/handler_servers.go` | Server list, select active, import handlers |
| `server/internal/webapi/handler_servers_test.go` | Tests |
| `server/internal/webapi/handler_clients.go` | Client CRUD handlers |
| `server/internal/webapi/handler_clients_test.go` | Tests |
| `server/internal/webapi/handler_excludes.go` | Exclusion sets and IPs handlers |
| `server/internal/webapi/handler_excludes_test.go` | Tests |
| `server/internal/webapi/handler_logs.go` | Logs and config handlers |
| `server/internal/webapi/handler_logs_test.go` | Tests |
| `server/internal/webapi/response.go` | JSON response helpers |
| `server/internal/vpnconfig/vpnconfig.go` | Add `WebUI` config struct (modify existing) |
| `server/Makefile` | Add webui build targets (modify existing) |

### Vue Frontend (new files in `web/`)

| File | Responsibility |
|------|----------------|
| `web/package.json` | Dependencies: vue, vite, typescript, axios |
| `web/vite.config.ts` | Vite config: SPA build, dev proxy |
| `web/tsconfig.json` | TypeScript config |
| `web/index.html` | HTML entry point |
| `web/src/main.ts` | Vue app bootstrap |
| `web/src/App.vue` | Root component: login gate + tab layout |
| `web/src/api.ts` | Axios client, JWT interceptor, API methods |
| `web/src/types.ts` | TypeScript interfaces matching API responses |
| `web/src/components/LoginPage.vue` | Login form |
| `web/src/components/StatusTab.vue` | Dashboard: status cards, action buttons |
| `web/src/components/ServersTab.vue` | Server list, select, import |
| `web/src/components/ClientsTab.vue` | Client table, add/edit/remove |
| `web/src/components/ExclusionsTab.vue` | Country sets + IP exclusions |
| `web/src/components/LogsTab.vue` | Log viewer with source filter |
| `web/src/components/SettingsTab.vue` | Version, update, config view |
| `web/src/style.css` | Dark theme CSS |

### Deploy files

| File | Responsibility |
|------|----------------|
| `router/opt/etc/init.d/S98vpn-director-webui` | Entware init script |
| `.github/workflows/telegram-bot.yml` | Add webui build (modify existing) |
| `install.sh` | Add webui download + cert gen (modify existing) |
| `Makefile` | Root Makefile for building both Go + Vue |

---

## Phase 1: Go Backend

### Task 1: Rename `telegram-bot/` → `server/`

**Files:**
- Rename: `telegram-bot/` → `server/`
- Modify: `server/go.mod` (module path stays same — it's internal)
- Modify: `.github/workflows/telegram-bot.yml`
- Modify: `install.sh`
- Modify: `.claude/rules/telegram-bot.md`

- [ ] **Step 1: Rename directory**

```bash
git mv telegram-bot server
```

- [ ] **Step 2: Update GitHub Actions workflow**

In `.github/workflows/telegram-bot.yml`, replace all `telegram-bot` directory references with `server`:

```yaml
      - name: Build arm64
        run: make -C server build-arm64

      - name: Build arm
        run: make -C server build-arm

      - name: Move binaries
        run: mkdir -p bin && mv server/bin/* bin/
```

- [ ] **Step 3: Update install.sh**

In `install.sh`, the comment on line 136 references `telegram-bot/internal/updater/downloader.go`. Update to `server/internal/updater/downloader.go`.

- [ ] **Step 4: Update CLAUDE.md rule reference**

In `.claude/rules/telegram-bot.md`, update the directory tree from `telegram-bot/` to `server/` in the Architecture section. Update all path references.

- [ ] **Step 5: Verify build**

```bash
cd server && go build ./... && go test ./...
```

Expected: all passes, no broken imports.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "refactor: rename telegram-bot/ to server/"
```

---

### Task 2: Add WebUI config to VPNDirectorConfig

**Files:**
- Modify: `server/internal/vpnconfig/vpnconfig.go`
- Modify: `server/internal/vpnconfig/vpnconfig_test.go`
- Modify: `router/opt/vpn-director/vpn-director.json.template`

- [ ] **Step 1: Write test for WebUI config parsing**

In `server/internal/vpnconfig/vpnconfig_test.go`, add:

```go
func TestLoadVPNDirectorConfig_WithWebUI(t *testing.T) {
	content := `{
		"data_dir": "/opt/vpn-director/data",
		"webui": {
			"port": 8444,
			"cert_file": "/opt/vpn-director/certs/server.crt",
			"key_file": "/opt/vpn-director/certs/server.key",
			"jwt_secret": "dGVzdC1zZWNyZXQ="
		},
		"xray": {"clients": [], "servers": [], "exclude_ips": [], "exclude_sets": []},
		"tunnel_director": {"tunnels": {}}
	}`
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(content), 0644)

	cfg, err := LoadVPNDirectorConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.WebUI.Port != 8444 {
		t.Errorf("port = %d, want 8444", cfg.WebUI.Port)
	}
	if cfg.WebUI.CertFile != "/opt/vpn-director/certs/server.crt" {
		t.Errorf("cert_file = %q, want server.crt path", cfg.WebUI.CertFile)
	}
	if cfg.WebUI.JWTSecret != "dGVzdC1zZWNyZXQ=" {
		t.Errorf("jwt_secret = %q, want test secret", cfg.WebUI.JWTSecret)
	}
}

func TestLoadVPNDirectorConfig_WithoutWebUI(t *testing.T) {
	content := `{
		"data_dir": "/opt/vpn-director/data",
		"xray": {"clients": [], "servers": [], "exclude_ips": [], "exclude_sets": []},
		"tunnel_director": {"tunnels": {}}
	}`
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(content), 0644)

	cfg, err := LoadVPNDirectorConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.WebUI.Port != 0 {
		t.Errorf("port = %d, want 0 (unset)", cfg.WebUI.Port)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd server && go test -run TestLoadVPNDirectorConfig_WithWebUI ./internal/vpnconfig/
```

Expected: FAIL — `cfg.WebUI` undefined.

- [ ] **Step 3: Add WebUIConfig struct**

In `server/internal/vpnconfig/vpnconfig.go`, add the struct and field:

```go
type WebUIConfig struct {
	Port      int    `json:"port,omitempty"`
	CertFile  string `json:"cert_file,omitempty"`
	KeyFile   string `json:"key_file,omitempty"`
	JWTSecret string `json:"jwt_secret,omitempty"`
}
```

Add field to `VPNDirectorConfig`:

```go
type VPNDirectorConfig struct {
	DataDir        string                 `json:"data_dir"`
	WebUI          WebUIConfig            `json:"webui,omitempty"`
	PausedClients  []string               `json:"paused_clients,omitempty"`
	TunnelDirector TunnelDirectorConfig   `json:"tunnel_director"`
	Xray           XrayConfig             `json:"xray"`
	Advanced       map[string]interface{} `json:"advanced,omitempty"`
}
```

- [ ] **Step 4: Run tests**

```bash
cd server && go test ./internal/vpnconfig/
```

Expected: PASS.

- [ ] **Step 5: Update config template**

In `router/opt/vpn-director/vpn-director.json.template`, add `webui` section:

```json
{
  "data_dir": "/opt/vpn-director/data",
  "webui": {
    "port": 8444,
    "cert_file": "/opt/vpn-director/certs/server.crt",
    "key_file": "/opt/vpn-director/certs/server.key",
    "jwt_secret": ""
  },
  "tunnel_director": {
    "tunnels": {}
  },
  ...
}
```

- [ ] **Step 6: Commit**

```bash
git add server/internal/vpnconfig/vpnconfig.go server/internal/vpnconfig/vpnconfig_test.go router/opt/vpn-director/vpn-director.json.template
git commit -m "feat(webui): add WebUI config to VPNDirectorConfig"
```

---

### Task 3: Auth — shadow file parser

**Files:**
- Create: `server/internal/auth/shadow.go`
- Create: `server/internal/auth/shadow_test.go`

- [ ] **Step 1: Write test for shadow password verification**

```go
// server/internal/auth/shadow_test.go
package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyPassword_Valid(t *testing.T) {
	// SHA-256 hash of "testpass" with salt "rounds=5000$testsalt"
	// Generated via: openssl passwd -5 -salt testsalt testpass
	shadow := `root:!:19000:0:99999:7:::
admin:$5$testsalt$hash_placeholder:19000:0:99999:7:::
nobody:*:19000:0:99999:7:::
`
	path := filepath.Join(t.TempDir(), "shadow")
	os.WriteFile(path, []byte(shadow), 0600)

	s := NewShadowAuth(path)

	// Test parsing works (actual crypt verification tested with real hash)
	entry, err := s.findEntry("admin")
	if err != nil {
		t.Fatalf("findEntry: %v", err)
	}
	if entry.username != "admin" {
		t.Errorf("username = %q, want admin", entry.username)
	}
}

func TestVerifyPassword_UserNotFound(t *testing.T) {
	shadow := `root:!:19000:0:99999:7:::
admin:$5$testsalt$somehash:19000:0:99999:7:::
`
	path := filepath.Join(t.TempDir(), "shadow")
	os.WriteFile(path, []byte(shadow), 0600)

	s := NewShadowAuth(path)
	ok, err := s.Verify("nobody", "pass")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("Verify returned true for non-existent user")
	}
}

func TestVerifyPassword_LockedAccount(t *testing.T) {
	shadow := `admin:!:19000:0:99999:7:::
`
	path := filepath.Join(t.TempDir(), "shadow")
	os.WriteFile(path, []byte(shadow), 0600)

	s := NewShadowAuth(path)
	ok, err := s.Verify("admin", "anything")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("Verify returned true for locked account")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd server && go test -run TestVerifyPassword ./internal/auth/
```

Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement shadow auth**

```go
// server/internal/auth/shadow.go
package auth

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/tredoe/osutil/user/crypt/sha256_crypt"
)

type ShadowAuth struct {
	path string
}

type shadowEntry struct {
	username string
	hash     string
}

func NewShadowAuth(path string) *ShadowAuth {
	return &ShadowAuth{path: path}
}

func (s *ShadowAuth) Verify(username, password string) (bool, error) {
	entry, err := s.findEntry(username)
	if err != nil {
		return false, err
	}
	if entry == nil {
		return false, nil
	}

	// Locked or no-password accounts
	if entry.hash == "!" || entry.hash == "*" || entry.hash == "!!" || entry.hash == "" {
		return false, nil
	}

	crypt := sha256_crypt.New()
	err = crypt.Verify(entry.hash, []byte(password))
	return err == nil, nil
}

func (s *ShadowAuth) findEntry(username string) (*shadowEntry, error) {
	f, err := os.Open(s.path)
	if err != nil {
		return nil, fmt.Errorf("open shadow: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.SplitN(line, ":", 3)
		if len(fields) < 2 {
			continue
		}
		if fields[0] == username {
			return &shadowEntry{
				username: fields[0],
				hash:     fields[1],
			}, nil
		}
	}
	return nil, scanner.Err()
}
```

Note: Add dependency `github.com/tredoe/osutil` for SHA-256 crypt. If this adds too much weight, implement crypt manually using `crypto/sha256` — the $5$ algorithm is well-documented (see glibc sha256-crypt.c). Evaluate during implementation.

- [ ] **Step 4: Add dependency and run tests**

```bash
cd server && go get github.com/tredoe/osutil
go test -run TestVerifyPassword ./internal/auth/
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/internal/auth/shadow.go server/internal/auth/shadow_test.go server/go.mod server/go.sum
git commit -m "feat(webui): add /etc/shadow password verification"
```

---

### Task 4: Auth — JWT token service

**Files:**
- Create: `server/internal/auth/jwt.go`
- Create: `server/internal/auth/jwt_test.go`

- [ ] **Step 1: Write JWT tests**

```go
// server/internal/auth/jwt_test.go (append to existing file)

func TestJWT_CreateAndValidate(t *testing.T) {
	secret := "test-secret-key-32-bytes-long!!"
	j := NewJWTService(secret, 24*time.Hour)

	token, err := j.Create("admin")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if token == "" {
		t.Fatal("token is empty")
	}

	claims, err := j.Validate(token)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if claims.Subject != "admin" {
		t.Errorf("subject = %q, want admin", claims.Subject)
	}
}

func TestJWT_Expired(t *testing.T) {
	secret := "test-secret-key-32-bytes-long!!"
	j := NewJWTService(secret, -1*time.Hour) // expired immediately

	token, err := j.Create("admin")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err = j.Validate(token)
	if err == nil {
		t.Error("Validate should fail for expired token")
	}
}

func TestJWT_InvalidSignature(t *testing.T) {
	j1 := NewJWTService("secret-one-32-bytes-long!!!!!!!!", 24*time.Hour)
	j2 := NewJWTService("secret-two-32-bytes-long!!!!!!!!", 24*time.Hour)

	token, _ := j1.Create("admin")
	_, err := j2.Validate(token)
	if err == nil {
		t.Error("Validate should fail for wrong secret")
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

```bash
cd server && go test -run TestJWT ./internal/auth/
```

Expected: FAIL — `NewJWTService` undefined.

- [ ] **Step 3: Implement JWT service**

```go
// server/internal/auth/jwt.go
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type JWTService struct {
	secret   []byte
	duration time.Duration
}

type Claims struct {
	Subject   string `json:"sub"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

func NewJWTService(secret string, duration time.Duration) *JWTService {
	return &JWTService{
		secret:   []byte(secret),
		duration: duration,
	}
}

func (j *JWTService) Create(subject string) (string, error) {
	now := time.Now()
	claims := Claims{
		Subject:   subject,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(j.duration).Unix(),
	}

	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal claims: %w", err)
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)

	sigInput := header + "." + encodedPayload
	mac := hmac.New(sha256.New, j.secret)
	mac.Write([]byte(sigInput))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return sigInput + "." + sig, nil
}

func (j *JWTService) Validate(token string) (*Claims, error) {
	parts := strings.SplitN(token, ".", 3)
	if len(parts) != 3 {
		return nil, errors.New("invalid token format")
	}

	sigInput := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, j.secret)
	mac.Write([]byte(sigInput))
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(parts[2]), []byte(expectedSig)) {
		return nil, errors.New("invalid signature")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}

	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("unmarshal claims: %w", err)
	}

	if time.Now().Unix() > claims.ExpiresAt {
		return nil, errors.New("token expired")
	}

	return &claims, nil
}
```

Uses `golang-jwt/jwt/v5` library. Add dependency: `go get github.com/golang-jwt/jwt/v5`. Rewrite `Create` and `Validate` using `jwt.NewWithClaims()` and `jwt.Parse()` with `jwt.WithValidMethods([]string{"HS256"})`.

- [ ] **Step 4: Run tests**

```bash
cd server && go test -run TestJWT ./internal/auth/
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/internal/auth/jwt.go server/internal/auth/jwt_test.go
git commit -m "feat(webui): add JWT token service"
```

---

### Task 5: WebAPI — response helpers and router

**Files:**
- Create: `server/internal/webapi/response.go`
- Create: `server/internal/webapi/router.go`
- Create: `server/internal/webapi/middleware.go`
- Create: `server/internal/webapi/middleware_test.go`

- [ ] **Step 1: Create response helpers**

```go
// server/internal/webapi/response.go
package webapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

func jsonOK(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func decodeJSON(r *http.Request, v interface{}) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}
```

- [ ] **Step 2: Create middleware**

```go
// server/internal/webapi/middleware.go
package webapi

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/zinin/asuswrt-merlin-vpn-director/internal/auth"
)

func authMiddleware(jwt *auth.JWTService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var token string

			// Try cookie first
			if cookie, err := r.Cookie("token"); err == nil {
				token = cookie.Value
			}

			// Then Authorization header
			if token == "" {
				if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
					token = strings.TrimPrefix(h, "Bearer ")
				}
			}

			if token == "" {
				jsonError(w, http.StatusUnauthorized, "authentication required")
				return
			}

			claims, err := jwt.Validate(token)
			if err != nil {
				jsonError(w, http.StatusUnauthorized, "invalid or expired token")
				return
			}

			_ = claims // could add to context if needed
			next.ServeHTTP(w, r)
		})
	}
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"duration", time.Since(start).String(),
			"remote", r.RemoteAddr,
		)
	})
}
```

- [ ] **Step 3: Write middleware test**

```go
// server/internal/webapi/middleware_test.go
package webapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zinin/asuswrt-merlin-vpn-director/internal/auth"
)

func TestAuthMiddleware_ValidCookie(t *testing.T) {
	jwt := auth.NewJWTService("test-secret-32-bytes-long!!!!!!!", 24*time.Hour)
	token, _ := jwt.Create("admin")

	handler := authMiddleware(jwt)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/status", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: token})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestAuthMiddleware_ValidBearer(t *testing.T) {
	jwt := auth.NewJWTService("test-secret-32-bytes-long!!!!!!!", 24*time.Hour)
	token, _ := jwt.Create("admin")

	handler := authMiddleware(jwt)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/status", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestAuthMiddleware_NoToken(t *testing.T) {
	jwt := auth.NewJWTService("test-secret-32-bytes-long!!!!!!!", 24*time.Hour)

	handler := authMiddleware(jwt)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/status", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}
```

- [ ] **Step 4: Create router**

```go
// server/internal/webapi/router.go
package webapi

import (
	"io/fs"
	"net/http"

	"github.com/zinin/asuswrt-merlin-vpn-director/internal/auth"
	"github.com/zinin/asuswrt-merlin-vpn-director/internal/service"
)

type Deps struct {
	Config  service.ConfigStore
	VPN     service.VPNDirector
	Xray    service.XrayGenerator
	Network service.NetworkInfo
	Logs    service.LogReader
	Shadow  *auth.ShadowAuth
	JWT     *auth.JWTService
	Paths   interface{ ScriptsDir() string }
	Version string
	Commit  string
}

func NewRouter(deps *Deps, staticFS fs.FS) http.Handler {
	mux := http.NewServeMux()

	authMW := authMiddleware(deps.JWT)

	// Public
	mux.HandleFunc("POST /api/login", handleLogin(deps))

	// Protected API
	protectedMux := http.NewServeMux()
	protectedMux.HandleFunc("POST /api/logout", handleLogout)
	protectedMux.HandleFunc("GET /api/status", handleStatus(deps))
	protectedMux.HandleFunc("POST /api/restart", handleRestart(deps))
	protectedMux.HandleFunc("POST /api/stop", handleStop(deps))
	protectedMux.HandleFunc("GET /api/ip", handleIP(deps))
	protectedMux.HandleFunc("GET /api/version", handleVersion(deps))
	protectedMux.HandleFunc("GET /api/servers", handleListServers(deps))
	protectedMux.HandleFunc("POST /api/servers/active", handleSelectServer(deps))
	protectedMux.HandleFunc("POST /api/servers/import", handleImportServers(deps))
	protectedMux.HandleFunc("GET /api/clients", handleListClients(deps))
	protectedMux.HandleFunc("POST /api/clients", handleAddClient(deps))
	protectedMux.HandleFunc("DELETE /api/clients/{ip}", handleDeleteClient(deps))
	protectedMux.HandleFunc("GET /api/excludes/sets", handleListExcludeSets(deps))
	protectedMux.HandleFunc("POST /api/excludes/sets", handleUpdateExcludeSets(deps))
	protectedMux.HandleFunc("GET /api/excludes/ips", handleListExcludeIPs(deps))
	protectedMux.HandleFunc("POST /api/excludes/ips", handleAddExcludeIP(deps))
	protectedMux.HandleFunc("DELETE /api/excludes/ips/{ip}", handleDeleteExcludeIP(deps))
	protectedMux.HandleFunc("GET /api/logs", handleLogs(deps))
	protectedMux.HandleFunc("GET /api/config", handleConfig(deps))

	mux.Handle("/api/", authMW(protectedMux))

	// Static files (Vue SPA)
	if staticFS != nil {
		fileServer := http.FileServer(http.FS(staticFS))
		mux.Handle("/", spaHandler(fileServer))
	}

	return loggingMiddleware(mux)
}

// spaHandler serves static files, falls back to index.html for SPA routing
func spaHandler(fileServer http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fileServer.ServeHTTP(w, r)
	})
}
```

- [ ] **Step 5: Run tests**

```bash
cd server && go test ./internal/webapi/
```

Expected: PASS (middleware tests pass, handlers are stubs for now).

- [ ] **Step 6: Commit**

```bash
git add server/internal/webapi/
git commit -m "feat(webui): add HTTP router, middleware, response helpers"
```

---

### Task 6: API handlers — auth, status, control

**Files:**
- Create: `server/internal/webapi/handler_auth.go`
- Create: `server/internal/webapi/handler_auth_test.go`
- Create: `server/internal/webapi/handler_status.go`
- Create: `server/internal/webapi/handler_status_test.go`

- [ ] **Step 1: Write auth handler tests**

```go
// server/internal/webapi/handler_auth_test.go
package webapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleLogin_InvalidCredentials(t *testing.T) {
	deps := newTestDeps(t)
	handler := handleLogin(deps)

	body, _ := json.Marshal(map[string]string{
		"username": "admin",
		"password": "wrongpass",
	})
	req := httptest.NewRequest("POST", "/api/login", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}
```

- [ ] **Step 2: Implement auth handlers**

```go
// server/internal/webapi/handler_auth.go
package webapi

import (
	"net/http"
	"time"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token string `json:"token"`
}

func handleLogin(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loginRequest
		if err := decodeJSON(r, &req); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		ok, err := deps.Shadow.Verify(req.Username, req.Password)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "authentication error")
			return
		}
		if !ok {
			jsonError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}

		token, err := deps.JWT.Create(req.Username)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "token creation failed")
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "token",
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteStrictMode,
			MaxAge:   int(24 * time.Hour / time.Second),
		})

		jsonOK(w, loginResponse{Token: token})
	}
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		MaxAge:   -1,
	})
	jsonOK(w, map[string]bool{"ok": true})
}
```

- [ ] **Step 3: Write status handler tests**

```go
// server/internal/webapi/handler_status_test.go
package webapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleStatus(t *testing.T) {
	deps := newTestDeps(t)
	handler := handleStatus(deps)

	req := httptest.NewRequest("GET", "/api/status", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	if _, ok := resp["output"]; !ok {
		t.Error("response missing 'output' field")
	}
}

func TestHandleRestart(t *testing.T) {
	deps := newTestDeps(t)
	handler := handleRestart(deps)

	req := httptest.NewRequest("POST", "/api/restart", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}
```

- [ ] **Step 4: Implement status handlers**

```go
// server/internal/webapi/handler_status.go
package webapi

import "net/http"

func handleStatus(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		output, err := deps.VPN.Status()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonOK(w, map[string]string{"output": output})
	}
}

func handleRestart(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := deps.VPN.Restart(); err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonOK(w, map[string]bool{"ok": true})
	}
}

func handleStop(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := deps.VPN.Stop(); err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonOK(w, map[string]bool{"ok": true})
	}
}

func handleIP(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip, err := deps.Network.GetExternalIP()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonOK(w, map[string]string{"ip": ip})
	}
}

func handleVersion(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jsonOK(w, map[string]string{
			"version": deps.Version,
			"commit":  deps.Commit,
		})
	}
}
```

- [ ] **Step 5: Create test helpers**

```go
// server/internal/webapi/test_helpers_test.go
package webapi

import (
	"testing"
	"time"

	"github.com/zinin/asuswrt-merlin-vpn-director/internal/auth"
	"github.com/zinin/asuswrt-merlin-vpn-director/internal/vpnconfig"
)

// Mock implementations for testing

type mockVPN struct{}

func (m *mockVPN) Status() (string, error) { return "Xray: running\nTunnel: active", nil }
func (m *mockVPN) Apply() error            { return nil }
func (m *mockVPN) Restart() error          { return nil }
func (m *mockVPN) RestartXray() error      { return nil }
func (m *mockVPN) Stop() error             { return nil }

type mockNetwork struct{}

func (m *mockNetwork) GetExternalIP() (string, error) { return "1.2.3.4", nil }

type mockLogs struct{}

func (m *mockLogs) Read(path string, lines int) (string, error) { return "log line 1\nlog line 2", nil }

type mockConfig struct {
	cfg     *vpnconfig.VPNDirectorConfig
	servers []vpnconfig.Server
}

func (m *mockConfig) LoadVPNConfig() (*vpnconfig.VPNDirectorConfig, error) { return m.cfg, nil }
func (m *mockConfig) SaveVPNConfig(cfg *vpnconfig.VPNDirectorConfig) error {
	m.cfg = cfg
	return nil
}
func (m *mockConfig) LoadServers() ([]vpnconfig.Server, error) { return m.servers, nil }
func (m *mockConfig) SaveServers(s []vpnconfig.Server) error {
	m.servers = s
	return nil
}
func (m *mockConfig) DataDir() (string, error)  { return "/tmp/test", nil }
func (m *mockConfig) DataDirOrDefault() string   { return "/tmp/test" }
func (m *mockConfig) ScriptsDir() string          { return "/tmp/test" }

type mockXray struct{}

func (m *mockXray) GenerateConfig(s vpnconfig.Server) error { return nil }

func newTestDeps(t *testing.T) *Deps {
	t.Helper()
	shadow := auth.NewShadowAuth("/dev/null")
	jwt := auth.NewJWTService("test-secret-32-bytes-long!!!!!!!", 24*time.Hour)
	return &Deps{
		Config:  &mockConfig{cfg: &vpnconfig.VPNDirectorConfig{
			Xray: vpnconfig.XrayConfig{
				Clients:     []string{"192.168.50.0/24"},
				ExcludeSets: []string{"ru"},
				ExcludeIPs:  []string{},
			},
			TunnelDirector: vpnconfig.TunnelDirectorConfig{Tunnels: map[string]vpnconfig.TunnelConfig{}},
		}},
		VPN:     &mockVPN{},
		Xray:    &mockXray{},
		Network: &mockNetwork{},
		Logs:    &mockLogs{},
		Shadow:  shadow,
		JWT:     jwt,
		Version: "v1.0.0",
		Commit:  "abc1234",
	}
}
```

- [ ] **Step 6: Run all tests**

```bash
cd server && go test ./internal/webapi/
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add server/internal/webapi/
git commit -m "feat(webui): add auth, status, and control API handlers"
```

---

### Task 7: API handlers — servers, clients, exclusions, logs

**Files:**
- Create: `server/internal/webapi/handler_servers.go`
- Create: `server/internal/webapi/handler_servers_test.go`
- Create: `server/internal/webapi/handler_clients.go`
- Create: `server/internal/webapi/handler_clients_test.go`
- Create: `server/internal/webapi/handler_excludes.go`
- Create: `server/internal/webapi/handler_excludes_test.go`
- Create: `server/internal/webapi/handler_logs.go`
- Create: `server/internal/webapi/handler_logs_test.go`

- [ ] **Step 1: Write servers handler tests**

```go
// server/internal/webapi/handler_servers_test.go
package webapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zinin/asuswrt-merlin-vpn-director/internal/vpnconfig"
)

func TestHandleListServers(t *testing.T) {
	deps := newTestDeps(t)
	deps.Config.(*mockConfig).servers = []vpnconfig.Server{
		{Name: "de-1", Address: "de.example.com", Port: 443, UUID: "uuid-1"},
	}

	req := httptest.NewRequest("GET", "/api/servers", nil)
	rec := httptest.NewRecorder()
	handleListServers(deps)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var resp struct {
		Servers []vpnconfig.Server `json:"servers"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if len(resp.Servers) != 1 {
		t.Errorf("servers count = %d, want 1", len(resp.Servers))
	}
}
```

- [ ] **Step 2: Implement servers handlers**

```go
// server/internal/webapi/handler_servers.go
package webapi

import "net/http"

func handleListServers(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		servers, err := deps.Config.LoadServers()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonOK(w, map[string]interface{}{"servers": servers})
	}
}

func handleSelectServer(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Index int `json:"index"`
		}
		if err := decodeJSON(r, &req); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		servers, err := deps.Config.LoadServers()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if req.Index < 0 || req.Index >= len(servers) {
			jsonError(w, http.StatusBadRequest, "invalid server index")
			return
		}

		if err := deps.Xray.GenerateConfig(servers[req.Index]); err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}

		cfg, err := deps.Config.LoadVPNConfig()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		cfg.Xray.Servers = servers[req.Index].IPs
		if err := deps.Config.SaveVPNConfig(cfg); err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}

		if err := deps.VPN.RestartXray(); err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}

		jsonOK(w, map[string]bool{"ok": true})
	}
}

func handleImportServers(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			URL string `json:"url"`
		}
		if err := decodeJSON(r, &req); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.URL == "" {
			jsonError(w, http.StatusBadRequest, "url is required")
			return
		}

		// Reuse vless parser from existing code
		// Import logic will call vless.ParseSubscription + config.SaveServers
		// Implementation deferred to match existing import handler pattern
		jsonError(w, http.StatusNotImplemented, "import via URL — use existing import_server_list.sh or Telegram bot")
	}
}
```

**Import handler**: Must be fully implemented (not a stub). Wire up `internal/vless/parser.go`: fetch subscription URL, call `vless.ParseSubscription()`, then `config.SaveServers()`. Follow the pattern from `server/internal/handler/import.go`.

**Update handler**: Must be fully implemented. Reuse `internal/updater` package: call `updater.CheckForUpdate()` and `updater.Update()`. Follow pattern from `server/internal/handler/update.go`. Send HTTP response before update script runs (the process will restart).

- [ ] **Step 3: Implement clients handlers**

```go
// server/internal/webapi/handler_clients.go
package webapi

import (
	"net/http"
	"strings"

	"github.com/zinin/asuswrt-merlin-vpn-director/internal/vpnconfig"
)

func handleListClients(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg, err := deps.Config.LoadVPNConfig()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		clients := vpnconfig.CollectClients(cfg)
		jsonOK(w, map[string]interface{}{"clients": clients})
	}
}

func handleAddClient(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			IP    string `json:"ip"`
			Route string `json:"route"`
		}
		if err := decodeJSON(r, &req); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.IP == "" || req.Route == "" {
			jsonError(w, http.StatusBadRequest, "ip and route are required")
			return
		}

		cfg, err := deps.Config.LoadVPNConfig()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}

		if req.Route == "xray" {
			if !contains(cfg.Xray.Clients, req.IP) {
				cfg.Xray.Clients = append(cfg.Xray.Clients, req.IP)
			}
		} else {
			// Tunnel route (wgc1, ovpnc1, etc.)
			tunnel, ok := cfg.TunnelDirector.Tunnels[req.Route]
			if !ok {
				tunnel = vpnconfig.TunnelConfig{
					Clients: []string{},
					Exclude: cfg.Xray.ExcludeSets, // inherit excludes
				}
			}
			if !contains(tunnel.Clients, req.IP) {
				tunnel.Clients = append(tunnel.Clients, req.IP)
			}
			cfg.TunnelDirector.Tunnels[req.Route] = tunnel
		}

		if err := deps.Config.SaveVPNConfig(cfg); err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonOK(w, map[string]bool{"ok": true})
	}
}

func handleDeleteClient(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := r.PathValue("ip")
		if ip == "" {
			jsonError(w, http.StatusBadRequest, "ip is required")
			return
		}

		cfg, err := deps.Config.LoadVPNConfig()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}

		cfg.Xray.Clients = removeString(cfg.Xray.Clients, ip)
		for name, tunnel := range cfg.TunnelDirector.Tunnels {
			tunnel.Clients = removeString(tunnel.Clients, ip)
			if len(tunnel.Clients) == 0 {
				delete(cfg.TunnelDirector.Tunnels, name)
			} else {
				cfg.TunnelDirector.Tunnels[name] = tunnel
			}
		}

		if err := deps.Config.SaveVPNConfig(cfg); err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonOK(w, map[string]bool{"ok": true})
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func removeString(slice []string, item string) []string {
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if !strings.EqualFold(s, item) {
			result = append(result, s)
		}
	}
	return result
}
```

- [ ] **Step 4: Implement exclusions handlers**

```go
// server/internal/webapi/handler_excludes.go
package webapi

import "net/http"

func handleListExcludeSets(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg, err := deps.Config.LoadVPNConfig()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonOK(w, map[string]interface{}{"sets": cfg.Xray.ExcludeSets})
	}
}

func handleUpdateExcludeSets(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Sets []string `json:"sets"`
		}
		if err := decodeJSON(r, &req); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		cfg, err := deps.Config.LoadVPNConfig()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		cfg.Xray.ExcludeSets = req.Sets
		if err := deps.Config.SaveVPNConfig(cfg); err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonOK(w, map[string]bool{"ok": true})
	}
}

func handleListExcludeIPs(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg, err := deps.Config.LoadVPNConfig()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonOK(w, map[string]interface{}{"ips": cfg.Xray.ExcludeIPs})
	}
}

func handleAddExcludeIP(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			IP string `json:"ip"`
		}
		if err := decodeJSON(r, &req); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.IP == "" {
			jsonError(w, http.StatusBadRequest, "ip is required")
			return
		}

		cfg, err := deps.Config.LoadVPNConfig()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !contains(cfg.Xray.ExcludeIPs, req.IP) {
			cfg.Xray.ExcludeIPs = append(cfg.Xray.ExcludeIPs, req.IP)
		}
		if err := deps.Config.SaveVPNConfig(cfg); err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonOK(w, map[string]bool{"ok": true})
	}
}

func handleDeleteExcludeIP(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := r.PathValue("ip")
		if ip == "" {
			jsonError(w, http.StatusBadRequest, "ip is required")
			return
		}

		cfg, err := deps.Config.LoadVPNConfig()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		cfg.Xray.ExcludeIPs = removeString(cfg.Xray.ExcludeIPs, ip)
		if err := deps.Config.SaveVPNConfig(cfg); err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonOK(w, map[string]bool{"ok": true})
	}
}
```

- [ ] **Step 5: Implement logs handler**

```go
// server/internal/webapi/handler_logs.go
package webapi

import (
	"net/http"
	"strconv"
)

var logPaths = map[string]string{
	"vpn": "/tmp/vpn-director.log",
	"bot": "/tmp/telegram-bot.log",
}

func handleLogs(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		source := r.URL.Query().Get("source")
		linesStr := r.URL.Query().Get("lines")

		lines := 50
		if linesStr != "" {
			if n, err := strconv.Atoi(linesStr); err == nil && n > 0 && n <= 500 {
				lines = n
			}
		}

		if source != "" {
			path, ok := logPaths[source]
			if !ok {
				jsonError(w, http.StatusBadRequest, "invalid source: use vpn or bot")
				return
			}
			output, err := deps.Logs.Read(path, lines)
			if err != nil {
				jsonError(w, http.StatusInternalServerError, err.Error())
				return
			}
			jsonOK(w, map[string]string{"output": output, "source": source})
			return
		}

		// All logs
		results := make(map[string]string)
		for name, path := range logPaths {
			output, err := deps.Logs.Read(path, lines)
			if err != nil {
				results[name] = "error: " + err.Error()
			} else {
				results[name] = output
			}
		}
		jsonOK(w, results)
	}
}

func handleConfig(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg, err := deps.Config.LoadVPNConfig()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonOK(w, cfg)
	}
}
```

- [ ] **Step 6: Write tests for all handlers**

Add test functions for each handler in their respective `_test.go` files following the pattern from Task 6 (create request, call handler, check status code and response body). At minimum test happy path and key error cases for each handler.

- [ ] **Step 7: Run all tests**

```bash
cd server && go test ./internal/webapi/
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add server/internal/webapi/
git commit -m "feat(webui): add servers, clients, exclusions, logs API handlers"
```

---

### Task 8: HTTP server and entry point

**Files:**
- Create: `server/internal/webapi/server.go`
- Create: `server/cmd/webui/main.go`
- Create: `server/cmd/webui/embed.go`
- Modify: `server/Makefile`

- [ ] **Step 1: Create HTTP server**

```go
// server/internal/webapi/server.go
package webapi

import (
	"context"
	"crypto/tls"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"time"
)

type ServerConfig struct {
	Port     int
	CertFile string
	KeyFile  string
}

func ListenAndServe(ctx context.Context, cfg ServerConfig, deps *Deps, staticFS fs.FS) error {
	router := NewRouter(deps, staticFS)

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("starting HTTPS server", "port", cfg.Port)
		errCh <- server.ListenAndServeTLS(cfg.CertFile, cfg.KeyFile)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		slog.Info("shutting down server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}
```

- [ ] **Step 2: Create embed.go**

```go
// server/cmd/webui/embed.go
package main

import "embed"

//go:embed web/dist/*
var staticFiles embed.FS
```

Note: During build, `web/dist/` is copied to `server/cmd/webui/web/dist/`. The `go:embed` directive is relative to the file's package directory.

- [ ] **Step 3: Create main.go**

```go
// server/cmd/webui/main.go
package main

import (
	"context"
	"flag"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zinin/asuswrt-merlin-vpn-director/internal/auth"
	"github.com/zinin/asuswrt-merlin-vpn-director/internal/paths"
	"github.com/zinin/asuswrt-merlin-vpn-director/internal/service"
	"github.com/zinin/asuswrt-merlin-vpn-director/internal/shell"
	"github.com/zinin/asuswrt-merlin-vpn-director/internal/vpnconfig"
	"github.com/zinin/asuswrt-merlin-vpn-director/internal/webapi"
)

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

func main() {
	configPath := flag.String("config", "/opt/vpn-director/vpn-director.json", "path to vpn-director.json")
	shadowPath := flag.String("shadow", "/etc/shadow", "path to shadow file")
	flag.Parse()

	slog.Info("starting VPN Director Web UI",
		"version", Version,
		"commit", Commit,
	)

	// Load config
	vpnCfg, err := vpnconfig.LoadVPNDirectorConfig(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	if vpnCfg.WebUI.Port == 0 {
		vpnCfg.WebUI.Port = 8444
	}
	if vpnCfg.WebUI.JWTSecret == "" {
		slog.Error("jwt_secret is not set in config")
		os.Exit(1)
	}

	// Services (same as Telegram bot)
	p := paths.Default()
	executor := shell.Exec
	configSvc := service.NewConfigService(p.ScriptsDir, p.DefaultDataDir)
	vpnSvc := service.NewVPNDirectorService(p.ScriptsDir, executor)
	xraySvc := service.NewXrayService(p.XrayTemplate, p.XrayConfig)
	networkSvc := service.NewNetworkService(executor)
	logSvc := service.NewLogService(executor)

	// Auth
	shadowAuth := auth.NewShadowAuth(*shadowPath)
	jwtSvc := auth.NewJWTService(vpnCfg.WebUI.JWTSecret, 24*time.Hour)

	deps := &webapi.Deps{
		Config:  configSvc,
		VPN:     vpnSvc,
		Xray:    xraySvc,
		Network: networkSvc,
		Logs:    logSvc,
		Shadow:  shadowAuth,
		JWT:     jwtSvc,
		Version: Version,
		Commit:  Commit,
	}

	// Static files
	var staticFS fs.FS
	sub, err := fs.Sub(staticFiles, "web/dist")
	if err != nil {
		slog.Warn("no embedded static files, running without SPA", "error", err)
	} else {
		staticFS = sub
	}

	// Start server
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	serverCfg := webapi.ServerConfig{
		Port:     vpnCfg.WebUI.Port,
		CertFile: vpnCfg.WebUI.CertFile,
		KeyFile:  vpnCfg.WebUI.KeyFile,
	}

	if err := webapi.ListenAndServe(ctx, serverCfg, deps, staticFS); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 4: Update Makefile**

Add to `server/Makefile`:

```makefile
WEBUI_LDFLAGS = -ldflags "-s -w -X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildDate=$(BUILD_DATE)"

build-webui:
	go build -trimpath $(WEBUI_LDFLAGS) -o bin/webui ./cmd/webui

build-webui-arm64:
	GOOS=linux GOARCH=arm64 go build -trimpath $(WEBUI_LDFLAGS) -o bin/webui-arm64 ./cmd/webui

build-webui-arm:
	GOOS=linux GOARCH=arm GOARM=7 go build -trimpath $(WEBUI_LDFLAGS) -o bin/webui-arm ./cmd/webui
```

- [ ] **Step 5: Verify build compiles**

```bash
cd server && go build ./cmd/webui/
```

Expected: compiles (may warn about missing embed files — that's ok for now).

- [ ] **Step 6: Commit**

```bash
git add server/internal/webapi/server.go server/cmd/webui/ server/Makefile
git commit -m "feat(webui): add HTTP server entry point and build targets"
```

---

## Phase 2: Vue Frontend

### Task 9: Vue project scaffolding

**Files:**
- Create: `web/package.json`
- Create: `web/vite.config.ts`
- Create: `web/tsconfig.json`
- Create: `web/index.html`
- Create: `web/src/main.ts`
- Create: `web/src/style.css`
- Create: `web/src/App.vue`
- Create: `web/src/types.ts`
- Create: `web/src/api.ts`

- [ ] **Step 1: Initialize Vue project**

```bash
cd web
npm init -y
npm install vue@^3.5 axios@^1
npm install -D vite@^6 @vitejs/plugin-vue typescript vue-tsc
```

- [ ] **Step 2: Create vite.config.ts**

```typescript
// web/vite.config.ts
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  server: {
    proxy: {
      '/api': {
        target: 'https://localhost:8444',
        secure: false,
      },
    },
  },
  build: {
    outDir: 'dist',
  },
})
```

- [ ] **Step 3: Create tsconfig.json**

```json
{
  "compilerOptions": {
    "target": "ES2020",
    "module": "ESNext",
    "moduleResolution": "bundler",
    "strict": true,
    "jsx": "preserve",
    "paths": { "@/*": ["./src/*"] }
  },
  "include": ["src/**/*.ts", "src/**/*.vue"]
}
```

- [ ] **Step 4: Create index.html**

```html
<!-- web/index.html -->
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>VPN Director</title>
</head>
<body>
  <div id="app"></div>
  <script type="module" src="/src/main.ts"></script>
</body>
</html>
```

- [ ] **Step 5: Create TypeScript types**

```typescript
// web/src/types.ts
export interface Server {
  name: string
  address: string
  port: number
  uuid: string
  ips: string[]
}

export interface ClientInfo {
  ip: string
  route: string
  paused: boolean
}

export interface StatusResponse {
  output: string
}

export interface VersionResponse {
  version: string
  commit: string
}

export interface ServersResponse {
  servers: Server[]
}

export interface ClientsResponse {
  clients: ClientInfo[]
}

export interface ExcludeSetsResponse {
  sets: string[]
}

export interface ExcludeIPsResponse {
  ips: string[]
}

export interface LoginResponse {
  token: string
}
```

- [ ] **Step 6: Create API client**

```typescript
// web/src/api.ts
import axios from 'axios'

const api = axios.create({
  baseURL: '',
  withCredentials: true,
})

api.interceptors.response.use(
  (res) => res,
  (err) => {
    if (err.response?.status === 401) {
      localStorage.removeItem('token')
      window.location.reload()
    }
    return Promise.reject(err)
  },
)

// Add Bearer token for mobile/API clients
api.interceptors.request.use((config) => {
  const token = localStorage.getItem('token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

export default {
  login: (username: string, password: string) =>
    api.post('/api/login', { username, password }),
  logout: () => api.post('/api/logout'),
  getStatus: () => api.get('/api/status'),
  restart: () => api.post('/api/restart'),
  stop: () => api.post('/api/stop'),
  getIP: () => api.get('/api/ip'),
  getVersion: () => api.get('/api/version'),
  getServers: () => api.get('/api/servers'),
  selectServer: (index: number) => api.post('/api/servers/active', { index }),
  importServers: (url: string) => api.post('/api/servers/import', { url }),
  getClients: () => api.get('/api/clients'),
  addClient: (ip: string, route: string) => api.post('/api/clients', { ip, route }),
  deleteClient: (ip: string) => api.delete(`/api/clients/${ip}`),
  getExcludeSets: () => api.get('/api/excludes/sets'),
  updateExcludeSets: (sets: string[]) => api.post('/api/excludes/sets', { sets }),
  getExcludeIPs: () => api.get('/api/excludes/ips'),
  addExcludeIP: (ip: string) => api.post('/api/excludes/ips', { ip }),
  deleteExcludeIP: (ip: string) => api.delete(`/api/excludes/ips/${ip}`),
  getLogs: (source?: string, lines?: number) =>
    api.get('/api/logs', { params: { source, lines } }),
  getConfig: () => api.get('/api/config'),
  update: () => api.post('/api/update'),
}
```

- [ ] **Step 7: Create dark theme CSS**

```css
/* web/src/style.css */
* { margin: 0; padding: 0; box-sizing: border-box; }

body {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
  background: #1a1a2e;
  color: #ccc;
  min-height: 100vh;
}

.topbar {
  background: #12122a;
  padding: 12px 24px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  border-bottom: 1px solid #333;
}

.topbar-title { color: #fc0; font-weight: bold; font-size: 18px; }
.topbar-meta { color: #888; font-size: 12px; }

.tabs {
  background: #12122a;
  display: flex;
  border-bottom: 1px solid #333;
  padding: 0 24px;
}

.tab {
  padding: 10px 20px;
  cursor: pointer;
  color: #888;
  border-bottom: 2px solid transparent;
  font-size: 14px;
  transition: color 0.2s;
}

.tab:hover { color: #ccc; }
.tab.active { color: #fc0; border-bottom-color: #fc0; }

.content { padding: 24px; max-width: 1200px; }

.actions { display: flex; gap: 8px; margin-bottom: 20px; flex-wrap: wrap; }

.btn {
  padding: 8px 20px;
  border: 1px solid;
  border-radius: 4px;
  cursor: pointer;
  font-size: 13px;
  background: transparent;
  transition: opacity 0.2s;
}

.btn:hover { opacity: 0.8; }
.btn:disabled { opacity: 0.4; cursor: not-allowed; }
.btn-green { color: #51cf66; border-color: #51cf66; }
.btn-yellow { color: #fc0; border-color: #fc0; }
.btn-red { color: #ff6b6b; border-color: #ff6b6b; }
.btn-blue { color: #4a9eff; border-color: #4a9eff; }
.btn-primary { background: #fc0; color: #1a1a2e; border-color: #fc0; font-weight: bold; }

.card {
  background: #222240;
  border: 1px solid #333;
  border-radius: 8px;
  padding: 16px;
  margin-bottom: 16px;
}

.card-title { color: #fc0; font-weight: bold; margin-bottom: 12px; }

.badge { padding: 2px 10px; border-radius: 10px; font-size: 11px; }
.badge-green { background: #1a3a1a; color: #51cf66; }
.badge-red { background: #3a1a1a; color: #ff6b6b; }
.badge-grey { background: #2a2a2a; color: #888; }

.grid-2 { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }

.kv { display: grid; grid-template-columns: auto 1fr; gap: 4px 12px; font-size: 13px; }
.kv-label { color: #888; }

table { width: 100%; border-collapse: collapse; font-size: 13px; }
th { text-align: left; color: #888; padding: 8px 12px; border-bottom: 1px solid #333; }
td { padding: 8px 12px; border-bottom: 1px solid #222; }

input, select {
  background: #2a2a4a;
  border: 1px solid #444;
  color: #ccc;
  padding: 8px 12px;
  border-radius: 4px;
  font-size: 14px;
}

input:focus, select:focus { outline: none; border-color: #fc0; }

.login-page {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 100vh;
}

.login-box {
  background: #222240;
  padding: 40px;
  border-radius: 12px;
  border: 1px solid #333;
  width: 360px;
}

.login-box h2 { color: #fc0; margin-bottom: 24px; text-align: center; }
.login-box input { width: 100%; margin-bottom: 16px; }
.login-box .btn { width: 100%; }

.form-group { margin-bottom: 16px; }
.form-group label { display: block; color: #888; font-size: 12px; margin-bottom: 4px; }

.error-msg { color: #ff6b6b; font-size: 13px; margin-bottom: 12px; }

@media (max-width: 768px) {
  .grid-2 { grid-template-columns: 1fr; }
  .tabs { overflow-x: auto; }
  .content { padding: 16px; }
}
```

- [ ] **Step 8: Create main.ts and App.vue**

```typescript
// web/src/main.ts
import { createApp } from 'vue'
import App from './App.vue'
import './style.css'

createApp(App).mount('#app')
```

```vue
<!-- web/src/App.vue -->
<script setup lang="ts">
import { ref, onMounted } from 'vue'
import api from './api'
import LoginPage from './components/LoginPage.vue'
import StatusTab from './components/StatusTab.vue'
import ServersTab from './components/ServersTab.vue'
import ClientsTab from './components/ClientsTab.vue'
import ExclusionsTab from './components/ExclusionsTab.vue'
import LogsTab from './components/LogsTab.vue'
import SettingsTab from './components/SettingsTab.vue'

const authenticated = ref(false)
const activeTab = ref('status')
const version = ref('')

const tabs = [
  { id: 'status', label: 'Status' },
  { id: 'servers', label: 'Servers' },
  { id: 'clients', label: 'Clients' },
  { id: 'exclusions', label: 'Exclusions' },
  { id: 'logs', label: 'Logs' },
  { id: 'settings', label: 'Settings' },
]

async function checkAuth() {
  try {
    const res = await api.getVersion()
    version.value = res.data.version
    authenticated.value = true
  } catch {
    authenticated.value = false
  }
}

function onLogin() {
  authenticated.value = true
  checkAuth()
}

async function logout() {
  await api.logout()
  localStorage.removeItem('token')
  authenticated.value = false
}

onMounted(checkAuth)
</script>

<template>
  <LoginPage v-if="!authenticated" @login="onLogin" />
  <template v-else>
    <div class="topbar">
      <div>
        <span class="topbar-title">VPN Director</span>
        <span class="topbar-meta" style="margin-left: 12px;">{{ version }}</span>
      </div>
      <div>
        <span class="topbar-meta" style="margin-right: 12px;">admin</span>
        <button class="btn btn-red" style="padding: 4px 12px;" @click="logout">Logout</button>
      </div>
    </div>
    <div class="tabs">
      <div
        v-for="tab in tabs"
        :key="tab.id"
        :class="['tab', { active: activeTab === tab.id }]"
        @click="activeTab = tab.id"
      >
        {{ tab.label }}
      </div>
    </div>
    <div class="content">
      <StatusTab v-if="activeTab === 'status'" />
      <ServersTab v-if="activeTab === 'servers'" />
      <ClientsTab v-if="activeTab === 'clients'" />
      <ExclusionsTab v-if="activeTab === 'exclusions'" />
      <LogsTab v-if="activeTab === 'logs'" />
      <SettingsTab v-if="activeTab === 'settings'" />
    </div>
  </template>
</template>
```

- [ ] **Step 9: Verify dev server starts**

```bash
cd web && npx vite --open
```

Expected: Vite dev server starts, shows login page at localhost:5173.

- [ ] **Step 10: Commit**

```bash
git add web/
git commit -m "feat(webui): Vue project scaffolding with dark theme and API client"
```

---

### Task 10: Vue — Login page

**Files:**
- Create: `web/src/components/LoginPage.vue`

- [ ] **Step 1: Create LoginPage component**

```vue
<!-- web/src/components/LoginPage.vue -->
<script setup lang="ts">
import { ref } from 'vue'
import api from '../api'

const emit = defineEmits<{ login: [] }>()

const username = ref('admin')
const password = ref('')
const error = ref('')
const loading = ref(false)

async function submit() {
  error.value = ''
  loading.value = true
  try {
    const res = await api.login(username.value, password.value)
    localStorage.setItem('token', res.data.token)
    emit('login')
  } catch (e: any) {
    error.value = e.response?.data?.error || 'Connection failed'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="login-page">
    <form class="login-box" @submit.prevent="submit">
      <h2>VPN Director</h2>
      <div class="error-msg" v-if="error">{{ error }}</div>
      <input
        v-model="username"
        placeholder="Username"
        autocomplete="username"
      />
      <input
        v-model="password"
        type="password"
        placeholder="Password"
        autocomplete="current-password"
      />
      <button class="btn btn-primary" type="submit" :disabled="loading">
        {{ loading ? 'Logging in...' : 'Login' }}
      </button>
    </form>
  </div>
</template>
```

- [ ] **Step 2: Commit**

```bash
git add web/src/components/LoginPage.vue
git commit -m "feat(webui): add login page component"
```

---

### Task 11: Vue — Status tab

**Files:**
- Create: `web/src/components/StatusTab.vue`

- [ ] **Step 1: Create StatusTab component**

```vue
<!-- web/src/components/StatusTab.vue -->
<script setup lang="ts">
import { ref, onMounted } from 'vue'
import api from '../api'

const status = ref('')
const ip = ref('')
const loading = ref(false)
const actionLoading = ref('')

async function loadStatus() {
  loading.value = true
  try {
    const [statusRes, ipRes] = await Promise.all([
      api.getStatus(),
      api.getIP(),
    ])
    status.value = statusRes.data.output
    ip.value = ipRes.data.ip
  } catch (e: any) {
    status.value = 'Error loading status: ' + (e.response?.data?.error || e.message)
  } finally {
    loading.value = false
  }
}

async function doAction(action: string, fn: () => Promise<any>) {
  actionLoading.value = action
  try {
    await fn()
    await loadStatus()
  } catch (e: any) {
    alert('Error: ' + (e.response?.data?.error || e.message))
  } finally {
    actionLoading.value = ''
  }
}

onMounted(loadStatus)
</script>

<template>
  <div class="actions">
    <button class="btn btn-green" :disabled="!!actionLoading" @click="doAction('restart', api.restart)">
      {{ actionLoading === 'restart' ? '...' : '↻ Restart' }}
    </button>
    <button class="btn btn-red" :disabled="!!actionLoading" @click="doAction('stop', api.stop)">
      {{ actionLoading === 'stop' ? '...' : '■ Stop' }}
    </button>
    <button class="btn btn-blue" :disabled="loading" @click="loadStatus">
      {{ loading ? '...' : '⟳ Refresh' }}
    </button>
  </div>

  <div class="grid-2">
    <div class="card">
      <div class="card-title">Status</div>
      <pre style="font-size: 12px; white-space: pre-wrap; line-height: 1.6;">{{ status || 'Loading...' }}</pre>
    </div>
    <div class="card">
      <div class="card-title">External IP</div>
      <div style="font-size: 20px; margin-top: 8px;">{{ ip || '...' }}</div>
    </div>
  </div>
</template>
```

- [ ] **Step 2: Commit**

```bash
git add web/src/components/StatusTab.vue
git commit -m "feat(webui): add Status tab component"
```

---

### Task 12: Vue — Servers tab

**Files:**
- Create: `web/src/components/ServersTab.vue`

- [ ] **Step 1: Create ServersTab component**

```vue
<!-- web/src/components/ServersTab.vue -->
<script setup lang="ts">
import { ref, onMounted } from 'vue'
import api from '../api'
import type { Server } from '../types'

const servers = ref<Server[]>([])
const loading = ref(false)
const importUrl = ref('')
const importLoading = ref(false)

async function loadServers() {
  loading.value = true
  try {
    const res = await api.getServers()
    servers.value = res.data.servers || []
  } finally {
    loading.value = false
  }
}

async function selectServer(index: number) {
  try {
    await api.selectServer(index)
    alert('Server selected and Xray restarted')
  } catch (e: any) {
    alert('Error: ' + (e.response?.data?.error || e.message))
  }
}

async function importServers() {
  if (!importUrl.value) return
  importLoading.value = true
  try {
    await api.importServers(importUrl.value)
    importUrl.value = ''
    await loadServers()
  } catch (e: any) {
    alert('Error: ' + (e.response?.data?.error || e.message))
  } finally {
    importLoading.value = false
  }
}

onMounted(loadServers)
</script>

<template>
  <div class="card" style="margin-bottom: 20px;">
    <div class="card-title">Import VLESS Subscription</div>
    <div style="display: flex; gap: 8px;">
      <input v-model="importUrl" placeholder="https://subscription-url..." style="flex: 1;" />
      <button class="btn btn-blue" :disabled="importLoading" @click="importServers">
        {{ importLoading ? '...' : 'Import' }}
      </button>
    </div>
  </div>

  <div class="card">
    <div class="card-title">Servers {{ loading ? '(loading...)' : `(${servers.length})` }}</div>
    <table v-if="servers.length">
      <thead>
        <tr>
          <th>#</th>
          <th>Name</th>
          <th>Address</th>
          <th>Port</th>
          <th>Action</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="(server, i) in servers" :key="i">
          <td>{{ i + 1 }}</td>
          <td>{{ server.name }}</td>
          <td>{{ server.address }}</td>
          <td>{{ server.port }}</td>
          <td>
            <button class="btn btn-green" style="padding: 4px 12px;" @click="selectServer(i)">
              Select
            </button>
          </td>
        </tr>
      </tbody>
    </table>
    <p v-else style="color: #888;">No servers. Import a subscription above.</p>
  </div>
</template>
```

- [ ] **Step 2: Commit**

```bash
git add web/src/components/ServersTab.vue
git commit -m "feat(webui): add Servers tab component"
```

---

### Task 13: Vue — Clients tab

**Files:**
- Create: `web/src/components/ClientsTab.vue`

- [ ] **Step 1: Create ClientsTab component**

```vue
<!-- web/src/components/ClientsTab.vue -->
<script setup lang="ts">
import { ref, onMounted } from 'vue'
import api from '../api'
import type { ClientInfo } from '../types'

const clients = ref<ClientInfo[]>([])
const loading = ref(false)
const newIP = ref('')
const newRoute = ref('xray')

const routes = ['xray', 'wgc1', 'wgc2', 'wgc3', 'wgc4', 'wgc5', 'ovpnc1', 'ovpnc2', 'ovpnc3', 'ovpnc4', 'ovpnc5']

async function loadClients() {
  loading.value = true
  try {
    const res = await api.getClients()
    clients.value = res.data.clients || []
  } finally {
    loading.value = false
  }
}

async function addClient() {
  if (!newIP.value) return
  try {
    await api.addClient(newIP.value, newRoute.value)
    newIP.value = ''
    await loadClients()
  } catch (e: any) {
    alert('Error: ' + (e.response?.data?.error || e.message))
  }
}

async function removeClient(ip: string) {
  if (!confirm(`Remove client ${ip}?`)) return
  try {
    await api.deleteClient(ip)
    await loadClients()
  } catch (e: any) {
    alert('Error: ' + (e.response?.data?.error || e.message))
  }
}

onMounted(loadClients)
</script>

<template>
  <div class="card" style="margin-bottom: 20px;">
    <div class="card-title">Add Client</div>
    <div style="display: flex; gap: 8px; align-items: center;">
      <input v-model="newIP" placeholder="192.168.50.10 or 192.168.50.0/24" style="flex: 1;" />
      <select v-model="newRoute">
        <option v-for="r in routes" :key="r" :value="r">{{ r }}</option>
      </select>
      <button class="btn btn-green" @click="addClient">Add</button>
    </div>
  </div>

  <div class="card">
    <div class="card-title">Clients {{ loading ? '(loading...)' : `(${clients.length})` }}</div>
    <table v-if="clients.length">
      <thead>
        <tr>
          <th>IP</th>
          <th>Route</th>
          <th>Status</th>
          <th>Action</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="client in clients" :key="client.ip">
          <td>{{ client.ip }}</td>
          <td>{{ client.route }}</td>
          <td>
            <span :class="['badge', client.paused ? 'badge-grey' : 'badge-green']">
              {{ client.paused ? 'Paused' : 'Active' }}
            </span>
          </td>
          <td>
            <button class="btn btn-red" style="padding: 4px 12px;" @click="removeClient(client.ip)">
              Remove
            </button>
          </td>
        </tr>
      </tbody>
    </table>
    <p v-else style="color: #888;">No clients configured.</p>
  </div>
</template>
```

- [ ] **Step 2: Commit**

```bash
git add web/src/components/ClientsTab.vue
git commit -m "feat(webui): add Clients tab component"
```

---

### Task 14: Vue — Exclusions tab

**Files:**
- Create: `web/src/components/ExclusionsTab.vue`

- [ ] **Step 1: Create ExclusionsTab component**

```vue
<!-- web/src/components/ExclusionsTab.vue -->
<script setup lang="ts">
import { ref, onMounted } from 'vue'
import api from '../api'

const sets = ref<string[]>([])
const ips = ref<string[]>([])
const newSet = ref('')
const newIP = ref('')
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    const [setsRes, ipsRes] = await Promise.all([
      api.getExcludeSets(),
      api.getExcludeIPs(),
    ])
    sets.value = setsRes.data.sets || []
    ips.value = ipsRes.data.ips || []
  } finally {
    loading.value = false
  }
}

async function addSet() {
  const code = newSet.value.trim().toLowerCase()
  if (!code || code.length !== 2) {
    alert('Enter a 2-letter country code (e.g., ru, ua)')
    return
  }
  if (sets.value.includes(code)) return
  try {
    await api.updateExcludeSets([...sets.value, code])
    newSet.value = ''
    await load()
  } catch (e: any) {
    alert('Error: ' + (e.response?.data?.error || e.message))
  }
}

async function removeSet(code: string) {
  try {
    await api.updateExcludeSets(sets.value.filter(s => s !== code))
    await load()
  } catch (e: any) {
    alert('Error: ' + (e.response?.data?.error || e.message))
  }
}

async function addIP() {
  if (!newIP.value.trim()) return
  try {
    await api.addExcludeIP(newIP.value.trim())
    newIP.value = ''
    await load()
  } catch (e: any) {
    alert('Error: ' + (e.response?.data?.error || e.message))
  }
}

async function removeIP(ip: string) {
  try {
    await api.deleteExcludeIP(ip)
    await load()
  } catch (e: any) {
    alert('Error: ' + (e.response?.data?.error || e.message))
  }
}

onMounted(load)
</script>

<template>
  <div class="grid-2">
    <div class="card">
      <div class="card-title">Country Exclusions</div>
      <div style="display: flex; gap: 8px; margin-bottom: 12px;">
        <input v-model="newSet" placeholder="Country code (e.g. ru)" maxlength="2" style="width: 120px;" />
        <button class="btn btn-green" @click="addSet">Add</button>
      </div>
      <div v-if="sets.length" style="display: flex; gap: 8px; flex-wrap: wrap;">
        <span v-for="code in sets" :key="code" class="badge badge-green" style="cursor: pointer; padding: 6px 12px;" @click="removeSet(code)">
          {{ code.toUpperCase() }} ✕
        </span>
      </div>
      <p v-else style="color: #888;">No country exclusions.</p>
    </div>

    <div class="card">
      <div class="card-title">IP Exclusions</div>
      <div style="display: flex; gap: 8px; margin-bottom: 12px;">
        <input v-model="newIP" placeholder="IP or CIDR (e.g. 1.2.3.4/24)" style="flex: 1;" />
        <button class="btn btn-green" @click="addIP">Add</button>
      </div>
      <div v-if="ips.length">
        <div v-for="ip in ips" :key="ip" style="display: flex; justify-content: space-between; padding: 4px 0; border-bottom: 1px solid #222;">
          <span>{{ ip }}</span>
          <span style="color: #ff6b6b; cursor: pointer;" @click="removeIP(ip)">✕</span>
        </div>
      </div>
      <p v-else style="color: #888;">No IP exclusions.</p>
    </div>
  </div>
</template>
```

- [ ] **Step 2: Commit**

```bash
git add web/src/components/ExclusionsTab.vue
git commit -m "feat(webui): add Exclusions tab component"
```

---

### Task 15: Vue — Logs and Settings tabs

**Files:**
- Create: `web/src/components/LogsTab.vue`
- Create: `web/src/components/SettingsTab.vue`

- [ ] **Step 1: Create LogsTab component**

```vue
<!-- web/src/components/LogsTab.vue -->
<script setup lang="ts">
import { ref, onMounted } from 'vue'
import api from '../api'

const source = ref('vpn')
const lines = ref(50)
const output = ref('')
const loading = ref(false)

async function loadLogs() {
  loading.value = true
  try {
    const res = await api.getLogs(source.value, lines.value)
    output.value = res.data.output || res.data[source.value] || ''
  } catch (e: any) {
    output.value = 'Error: ' + (e.response?.data?.error || e.message)
  } finally {
    loading.value = false
  }
}

onMounted(loadLogs)
</script>

<template>
  <div class="card" style="margin-bottom: 16px;">
    <div style="display: flex; gap: 12px; align-items: center;">
      <select v-model="source" @change="loadLogs">
        <option value="vpn">VPN Director</option>
        <option value="bot">Telegram Bot</option>
      </select>
      <label style="color: #888; font-size: 13px;">
        Lines:
        <input v-model.number="lines" type="number" min="10" max="500" style="width: 70px; margin-left: 4px;" />
      </label>
      <button class="btn btn-blue" :disabled="loading" @click="loadLogs">
        {{ loading ? '...' : 'Refresh' }}
      </button>
    </div>
  </div>

  <div class="card">
    <pre style="font-size: 11px; line-height: 1.5; white-space: pre-wrap; max-height: 600px; overflow-y: auto;">{{ output || 'No logs' }}</pre>
  </div>
</template>
```

- [ ] **Step 2: Create SettingsTab component**

```vue
<!-- web/src/components/SettingsTab.vue -->
<script setup lang="ts">
import { ref, onMounted } from 'vue'
import api from '../api'

const version = ref('')
const commit = ref('')
const config = ref<object | null>(null)
const loading = ref(false)
const showConfig = ref(false)

async function load() {
  loading.value = true
  try {
    const [versionRes, configRes] = await Promise.all([
      api.getVersion(),
      api.getConfig(),
    ])
    version.value = versionRes.data.version
    commit.value = versionRes.data.commit
    config.value = configRes.data
  } finally {
    loading.value = false
  }
}

async function doUpdate() {
  if (!confirm('Update VPN Director to the latest version?')) return
  try {
    await api.update()
    alert('Update initiated. The service will restart.')
  } catch (e: any) {
    alert('Error: ' + (e.response?.data?.error || e.message))
  }
}

onMounted(load)
</script>

<template>
  <div class="grid-2">
    <div class="card">
      <div class="card-title">Version</div>
      <div class="kv">
        <span class="kv-label">Version:</span><span>{{ version }}</span>
        <span class="kv-label">Commit:</span><span>{{ commit }}</span>
      </div>
      <button class="btn btn-blue" style="margin-top: 16px;" @click="doUpdate">Check for Update</button>
    </div>

    <div class="card">
      <div class="card-title">Configuration</div>
      <button class="btn btn-yellow" @click="showConfig = !showConfig">
        {{ showConfig ? 'Hide' : 'Show' }} Raw Config
      </button>
      <pre v-if="showConfig" style="margin-top: 12px; font-size: 11px; line-height: 1.5; white-space: pre-wrap; max-height: 400px; overflow-y: auto;">{{ JSON.stringify(config, null, 2) }}</pre>
    </div>
  </div>
</template>
```

- [ ] **Step 3: Commit**

```bash
git add web/src/components/LogsTab.vue web/src/components/SettingsTab.vue
git commit -m "feat(webui): add Logs and Settings tab components"
```

---

## Phase 3: Build & Deploy

### Task 16: Build pipeline

**Files:**
- Create: `Makefile` (root)
- Modify: `.github/workflows/telegram-bot.yml`

- [ ] **Step 1: Create root Makefile**

```makefile
# Makefile (project root)
.PHONY: build-webui build-bot build-all web clean

# Build Vue SPA
web:
	cd web && npm ci && npm run build

# Copy dist into Go embed location
web-embed: web
	rm -rf server/cmd/webui/web/dist
	mkdir -p server/cmd/webui/web
	cp -r web/dist server/cmd/webui/web/dist

# Build webui binary (requires web-embed first)
build-webui: web-embed
	make -C server build-webui

build-webui-arm64: web-embed
	make -C server build-webui-arm64

build-webui-arm: web-embed
	make -C server build-webui-arm

# Build bot (unchanged)
build-bot:
	make -C server build

build-bot-arm64:
	make -C server build-arm64

build-bot-arm:
	make -C server build-arm

# Build all
build-all: build-webui-arm64 build-webui-arm build-bot-arm64 build-bot-arm

clean:
	rm -rf web/dist server/cmd/webui/web/dist
	make -C server clean
```

- [ ] **Step 2: Add web/dist to .gitignore**

Append to `.gitignore`:

```
# Build output
web/dist/
web/node_modules/
server/cmd/webui/web/
```

- [ ] **Step 3: Update GitHub Actions**

Replace `.github/workflows/telegram-bot.yml`:

```yaml
name: Build

on:
  push:
    tags:
      - 'v*'
  workflow_dispatch:

permissions:
  contents: write

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-go@v5
        with:
          go-version: '1.25.5'

      - uses: actions/setup-node@v4
        with:
          node-version: '22'

      - name: Build Vue SPA
        run: cd web && npm ci && npm run build

      - name: Prepare embed
        run: |
          mkdir -p server/cmd/webui/web
          cp -r web/dist server/cmd/webui/web/dist

      - name: Build bot arm64
        run: make -C server build-arm64

      - name: Build bot arm
        run: make -C server build-arm

      - name: Build webui arm64
        run: make -C server build-webui-arm64

      - name: Build webui arm
        run: make -C server build-webui-arm

      - name: Move binaries
        run: mkdir -p bin && mv server/bin/* bin/

      - name: Upload artifacts
        uses: actions/upload-artifact@v4
        with:
          name: binaries
          path: bin/

      - name: Upload to Release
        if: startsWith(github.ref, 'refs/tags/')
        uses: softprops/action-gh-release@v2
        with:
          generate_release_notes: true
          files: |
            bin/telegram-bot-arm64
            bin/telegram-bot-arm
            bin/webui-arm64
            bin/webui-arm
```

- [ ] **Step 4: Verify full build works locally**

```bash
make web-embed
cd server && go build ./cmd/webui/
```

Expected: compiles with embedded SPA.

- [ ] **Step 5: Commit**

```bash
git add Makefile .gitignore .github/workflows/telegram-bot.yml
git commit -m "feat(webui): add build pipeline and CI for webui binary"
```

---

### Task 17: Init script and installer updates

**Files:**
- Create: `router/opt/etc/init.d/S98vpn-director-webui`
- Modify: `install.sh`

- [ ] **Step 1: Create Entware init script**

```bash
#!/opt/bin/bash
# /opt/etc/init.d/S98vpn-director-webui

ENABLED=yes
PROCS=webui
ARGS="--config /opt/vpn-director/vpn-director.json"
PREARGS="nohup"
DESC="VPN Director Web UI"
PATH=/opt/sbin:/opt/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

. /opt/etc/init.d/rc.func
```

- [ ] **Step 2: Add webui download to install.sh**

After the `download_telegram_bot` function in `install.sh`, add:

```bash
download_webui() {
    print_info "Downloading Web UI binary..."

    local arch
    arch=$(uname -m)
    local webui_binary=""
    local release_url="$RELEASE_ASSET_URL"
    local webui_path="$VPD_DIR/webui"
    local tmp_path="${webui_path}.tmp"
    local was_running=false

    case "$arch" in
        aarch64) webui_binary="webui-arm64" ;;
        armv7l)  webui_binary="webui-arm" ;;
        *)
            print_info "Architecture $arch not supported for webui (optional component)"
            return 0
            ;;
    esac

    if ! curl -fsSL "$release_url/$webui_binary" -o "$tmp_path"; then
        print_info "Warning: Failed to download webui (optional component)"
        rm -f "$tmp_path" 2>/dev/null || true
        return 0
    fi

    # Stop running webui before overwriting binary
    if pidof webui >/dev/null 2>&1; then
        was_running=true
        print_info "Stopping running webui..."
        if [[ -x /opt/etc/init.d/S98vpn-director-webui ]]; then
            /opt/etc/init.d/S98vpn-director-webui stop >/dev/null 2>&1 || true
        else
            killall webui 2>/dev/null || true
        fi
        sleep 1
    fi

    mv "$tmp_path" "$webui_path"
    chmod +x "$webui_path"
    print_success "Installed webui"

    if [[ "$was_running" == true ]] && [[ -x /opt/etc/init.d/S98vpn-director-webui ]]; then
        print_info "Starting webui..."
        /opt/etc/init.d/S98vpn-director-webui start >/dev/null 2>&1 || true
    fi
}
```

- [ ] **Step 3: Add TLS cert generation to install.sh**

```bash
generate_tls_cert() {
    local cert_dir="$VPD_DIR/certs"

    if [[ -f "$cert_dir/server.crt" ]] && [[ -f "$cert_dir/server.key" ]]; then
        print_success "TLS certificate already exists"
        return 0
    fi

    print_info "Generating self-signed TLS certificate..."

    mkdir -p "$cert_dir"

    if ! which openssl >/dev/null 2>&1; then
        print_info "openssl not found, installing..."
        opkg install openssl-util 2>/dev/null || true
    fi

    openssl req -x509 -newkey rsa:2048 \
        -keyout "$cert_dir/server.key" \
        -out "$cert_dir/server.crt" \
        -days 3650 -nodes \
        -subj "/CN=vpn-director" 2>/dev/null || {
        print_error "Failed to generate TLS certificate"
        return 1
    }

    chmod 600 "$cert_dir/server.key"
    print_success "TLS certificate generated"
}
```

- [ ] **Step 4: Add JWT secret generation to install.sh**

```bash
generate_jwt_secret() {
    local config_path="$VPD_DIR/vpn-director.json"

    if [[ ! -f "$config_path" ]]; then
        return 0
    fi

    # Check if jwt_secret is already set
    local existing
    existing=$(grep -o '"jwt_secret":\s*"[^"]*"' "$config_path" 2>/dev/null | grep -v '""' || true)
    if [[ -n "$existing" ]]; then
        print_success "JWT secret already set"
        return 0
    fi

    local secret
    secret=$(head -c 32 /dev/urandom | base64 | tr -d '\n')

    # Add jwt_secret to webui section using lightweight JSON manipulation
    # If webui section exists, add jwt_secret; otherwise jq is needed
    if which jq >/dev/null 2>&1; then
        local tmp
        tmp=$(mktemp)
        jq --arg secret "$secret" '.webui.jwt_secret = $secret' "$config_path" > "$tmp" && mv "$tmp" "$config_path"
        print_success "JWT secret generated"
    else
        print_info "Install jq for automatic JWT secret setup: opkg install jq"
        print_info "Then manually set webui.jwt_secret in vpn-director.json"
    fi
}
```

- [ ] **Step 5: Wire up new functions in main()**

In `install.sh`, update the `main()` function to include:

```bash
main() {
    print_header "VPN Director Installer"
    printf "This will install VPN Director scripts to your router.\n\n"

    check_environment
    resolve_release_tag
    create_directories
    download_scripts
    download_telegram_bot
    download_webui
    generate_tls_cert
    generate_jwt_secret
    print_next_steps
}
```

Update `print_next_steps` to include webui info:

```bash
print_next_steps() {
    print_header "Installation Complete ($RELEASE_TAG)"

    printf "Next steps:\n\n"
    printf "  1. Import VLESS servers:\n"
    printf "     ${GREEN}/opt/vpn-director/import_server_list.sh${NC}\n\n"
    printf "  2. Run configuration wizard:\n"
    printf "     ${GREEN}/opt/vpn-director/configure.sh${NC}\n\n"
    printf "  3. (Optional) Setup Telegram bot:\n"
    printf "     ${GREEN}/opt/vpn-director/setup_telegram_bot.sh${NC}\n\n"
    printf "  4. (Optional) Start Web UI:\n"
    printf "     ${GREEN}/opt/etc/init.d/S98vpn-director-webui start${NC}\n"
    printf "     Then open ${GREEN}https://<router-ip>:8444${NC}\n\n"
    printf "Or edit configs manually:\n"
    printf "  /opt/vpn-director/vpn-director.json\n"
    printf "  /opt/etc/xray/config.json\n"
}
```

- [ ] **Step 6: Add init script to download_scripts list**

In `install.sh`, add to the scripts download list:

```bash
"router/opt/etc/init.d/S98vpn-director-webui"
```

- [ ] **Step 7: Commit**

```bash
git add router/opt/etc/init.d/S98vpn-director-webui install.sh
git commit -m "feat(webui): add init script and installer support"
```

---

## Post-Implementation

After all tasks are complete:

1. Run full Go test suite: `cd server && go test ./...`
2. Run full build: `make build-all`
3. Test on router: deploy binaries, verify HTTPS server starts, login works, all tabs functional
4. Update CLAUDE.md with webui architecture documentation
