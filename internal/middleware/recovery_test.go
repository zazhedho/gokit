package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zazhedho/logus"
)

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	logus.SetOutput(buf)
	t.Cleanup(func() { logus.SetOutput(nil) })
	return buf
}

func TestRecoveryReturns500WithoutInternalsAndLogsStack(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logs := captureLogs(t)
	router := gin.New()
	router.Use(RequestIDMiddleware(), Recovery())
	router.GET("/panic", func(*gin.Context) { panic("boom-secret") })

	id := uuid.New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	req.Header.Set("X-Request-ID", id.String())
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, leak := range []string{"boom-secret", "goroutine", ".go:", "panic"} {
		if strings.Contains(body, leak) {
			t.Fatalf("body leaks %q: %s", leak, body)
		}
	}
	var parsed map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid json body: %v", err)
	}
	if parsed["log_id"] != id.String() {
		t.Fatalf("expected log_id %s, got %v", id, parsed["log_id"])
	}
	if got := rec.Header().Get("X-Request-ID"); got != id.String() {
		t.Fatalf("expected X-Request-ID %s, got %s", id, got)
	}

	out := logs.String()
	if !strings.Contains(out, "boom-secret") || !strings.Contains(out, "recovery_test.go") || !strings.Contains(out, id.String()) {
		t.Fatalf("log must carry the panic value, the stack and the request id, got: %s", out)
	}
}

func TestRecoveryRepanicsOnAbortHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Recovery())
	router.GET("/abort", func(*gin.Context) { panic(http.ErrAbortHandler) })

	defer func() {
		if r := recover(); r != http.ErrAbortHandler { //nolint:errorlint // panic value identity
			t.Fatalf("expected http.ErrAbortHandler to propagate, got %v", r)
		}
	}()
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/abort", nil))
}

func TestRecoveryPassesThroughWithoutPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Recovery())
	router.GET("/ok", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ok", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
}
