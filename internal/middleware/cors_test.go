package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func corsRouter(opts CORSOptions) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CORS(opts))
	router.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	router.OPTIONS("/ping", func(c *gin.Context) { c.String(http.StatusOK, "handler") })
	return router
}

func send(router *gin.Engine, method string, headers map[string]string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, "/ping", nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	router.ServeHTTP(rec, req)
	return rec
}

func TestCORSPreflight(t *testing.T) {
	router := corsRouter(CORSOptions{AllowOrigins: []string{"https://app.example"}, MaxAge: 10 * time.Minute})
	rec := send(router, http.MethodOptions, map[string]string{
		"Origin": "https://app.example", "Access-Control-Request-Method": "PATCH",
	})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("real preflight must get 204, got %d", rec.Code)
	}
	h := rec.Header()
	if h.Get("Access-Control-Allow-Origin") != "https://app.example" || h.Get("Access-Control-Max-Age") != "600" {
		t.Fatalf("unexpected headers: %v", h)
	}
	for _, m := range []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"} {
		if !strings.Contains(h.Get("Access-Control-Allow-Methods"), m) {
			t.Errorf("default methods must include %s: %s", m, h.Get("Access-Control-Allow-Methods"))
		}
	}
	if !strings.Contains(h.Get("Access-Control-Allow-Headers"), "X-Request-ID") && !strings.Contains(h.Get("Access-Control-Allow-Headers"), "Authorization") {
		t.Errorf("default allowed headers missing: %s", h.Get("Access-Control-Allow-Headers"))
	}
	if !strings.Contains(h.Values("Vary")[0], "Origin") {
		t.Errorf("Vary: Origin is required for a specific origin: %v", h.Values("Vary"))
	}
}

func TestCORSOnlyRealPreflightsGet204(t *testing.T) {
	router := corsRouter(CORSOptions{AllowOrigins: []string{"https://app.example"}})
	for name, headers := range map[string]map[string]string{
		"no origin":                    {},
		"origin but no request-method": {"Origin": "https://app.example"},
		"request-method but no origin": {"Access-Control-Request-Method": "GET"},
	} {
		rec := send(router, http.MethodOptions, headers)
		if rec.Code != http.StatusOK || rec.Body.String() != "handler" {
			t.Errorf("%s: a plain OPTIONS must reach the handler, got %d %q", name, rec.Code, rec.Body.String())
		}
	}
}

func TestCORSActualRequests(t *testing.T) {
	router := corsRouter(CORSOptions{AllowOrigins: []string{"https://app.example"}, Expose: []string{"X-Total"}, AllowCredentials: true})

	rec := send(router, http.MethodGet, map[string]string{"Origin": "https://app.example"})
	h := rec.Header()
	if rec.Code != http.StatusOK || h.Get("Access-Control-Allow-Origin") != "https://app.example" ||
		h.Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("allowed origin: %d %v", rec.Code, h)
	}
	if expose := h.Get("Access-Control-Expose-Headers"); !strings.Contains(expose, "X-Request-ID") || !strings.Contains(expose, "X-Total") {
		t.Fatalf("expose must include X-Request-ID and the configured headers: %q", expose)
	}

	rec = send(router, http.MethodGet, map[string]string{"Origin": "https://evil.example"})
	if rec.Code != http.StatusOK || rec.Header().Get("Access-Control-Allow-Origin") != "" || rec.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Fatalf("disallowed origin must get no CORS headers: %v", rec.Header())
	}

	rec = send(router, http.MethodGet, nil)
	if rec.Code != http.StatusOK || rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("a request without Origin is not a CORS request: %v", rec.Header())
	}
}

func TestCORSRejectsPreflightFromDisallowedOrigin(t *testing.T) {
	router := corsRouter(CORSOptions{AllowOrigins: []string{"https://app.example"}})
	rec := send(router, http.MethodOptions, map[string]string{
		"Origin": "https://evil.example", "Access-Control-Request-Method": "GET",
	})
	if rec.Code != http.StatusForbidden || rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("got %d %v", rec.Code, rec.Header())
	}
}

func TestCORSIsOpenByDefault(t *testing.T) {
	for name, opts := range map[string]CORSOptions{"empty": {}, "explicit star": {AllowOrigins: []string{"*"}}} {
		router := corsRouter(opts)
		rec := send(router, http.MethodGet, map[string]string{"Origin": "https://any.example"})
		h := rec.Header()
		if h.Get("Access-Control-Allow-Origin") != "*" || h.Get("Access-Control-Allow-Credentials") != "" || len(h.Values("Vary")) != 0 {
			t.Fatalf("%s: %v", name, h)
		}
		rec = send(router, http.MethodOptions, map[string]string{"Origin": "https://any.example", "Access-Control-Request-Method": "PUT"})
		if rec.Code != http.StatusNoContent || rec.Header().Get("Access-Control-Allow-Origin") != "*" {
			t.Fatalf("%s preflight: %d %v", name, rec.Code, rec.Header())
		}
	}
}

func TestCORSVaryOriginForSpecificOrigins(t *testing.T) {
	router := corsRouter(CORSOptions{AllowOrigins: []string{"https://app.example"}})
	for _, origin := range []string{"https://app.example", "https://evil.example"} {
		rec := send(router, http.MethodGet, map[string]string{"Origin": origin})
		if !strings.Contains(strings.Join(rec.Header().Values("Vary"), ","), "Origin") {
			t.Fatalf("Vary: Origin is required for %s: %v", origin, rec.Header())
		}
	}
}

func TestCORSVaryOriginWithoutOriginHeader(t *testing.T) {
	rec := send(corsRouter(CORSOptions{AllowOrigins: []string{"https://app.example"}}), http.MethodGet, nil)
	if !strings.Contains(strings.Join(rec.Header().Values("Vary"), ","), "Origin") {
		t.Fatalf("a cache must not reuse a no-Origin response for other origins: %v", rec.Header())
	}
	rec = send(corsRouter(CORSOptions{}), http.MethodGet, nil)
	if len(rec.Header().Values("Vary")) != 0 {
		t.Fatalf("the open default sends no Vary: %v", rec.Header())
	}
}

func TestCORSRejectsMalformedOrigins(t *testing.T) {
	for _, bad := range []string{"https://app.example/", "https://app.example/path", "app.example", "https://", "https://a b.example", "*.example.com"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%q must be rejected at construction", bad)
				}
			}()
			CORS(CORSOptions{AllowOrigins: []string{bad}})
		}()
	}
	CORS(CORSOptions{AllowOrigins: []string{"https://app.example", "http://localhost:3000"}})
}

func TestCORSRejectsCredentialsWithWildcard(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("wildcard origin with credentials must panic at construction")
		}
	}()
	CORS(CORSOptions{AllowOrigins: []string{"*"}, AllowCredentials: true})
}

func TestCORSRejectsCredentialsWithOpenDefault(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("credentials with no AllowOrigins (open) must panic")
		}
	}()
	CORS(CORSOptions{AllowCredentials: true})
}
