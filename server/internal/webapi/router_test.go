package webapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func testStaticFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":    {Data: []byte("<html>spa</html>")},
		"assets/app.js": {Data: []byte("console.log('app')")},
	}
}

func TestRouter_UnknownAPIPathIsJSON404(t *testing.T) {
	deps := newTestDeps(t)
	router := NewRouter(deps, testStaticFS())

	token, err := deps.JWT.Create("admin")
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	req := httptest.NewRequest("GET", "/api/nope", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: token})
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected JSON content type, got %q", ct)
	}
	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "not found" {
		t.Errorf("unexpected error text: %q", resp["error"])
	}
}

func TestRouter_UnknownAPIPathStillRequiresAuth(t *testing.T) {
	deps := newTestDeps(t)
	router := NewRouter(deps, testStaticFS())

	req := httptest.NewRequest("GET", "/api/nope", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 before the 404 fallback, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSPAHandler_CacheHeaders(t *testing.T) {
	handler := spaHandler(testStaticFS())

	cases := []struct {
		path      string
		wantCache string
		wantBody  string
	}{
		{"/", "no-cache", "<html>spa</html>"},
		{"/clients", "no-cache", "<html>spa</html>"},
		{"/assets/app.js", "public, max-age=31536000, immutable", "console.log('app')"},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			req := httptest.NewRequest("GET", c.path, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Cache-Control"); got != c.wantCache {
				t.Errorf("Cache-Control = %q, want %q", got, c.wantCache)
			}
			body, _ := io.ReadAll(rec.Body)
			if string(body) != c.wantBody {
				t.Errorf("body = %q, want %q", string(body), c.wantBody)
			}
		})
	}
}
