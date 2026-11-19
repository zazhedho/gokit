package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zazhedho/logus"

	"github.com/zazhedho/gokit/internal/utils"
)

func TestRequestIDMiddlewareUsesExistingRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestIDMiddleware())
	router.GET("/ping", func(ctx *gin.Context) {
		if _, ok := ctx.Get(logus.CtxKeyRequestID); !ok {
			t.Fatal("expected context id to be set")
		}
		ctx.String(http.StatusOK, "ok")
	})

	requestID := uuid.NewString()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("X-Request-ID", requestID)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || rec.Header().Get("X-Request-ID") != requestID {
		t.Fatalf("unexpected response: code=%d headers=%v", rec.Code, rec.Header())
	}
}

func TestRequestIDMiddlewareSharesIDWithLogus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestIDMiddleware())

	var fromGin, fromRequestCtx, fromUtils uuid.UUID
	router.GET("/ping", func(c *gin.Context) {
		fromGin = logus.RequestID(c)
		fromRequestCtx = logus.RequestID(c.Request.Context())
		fromUtils = utils.RequestID(c)
		c.String(http.StatusOK, "ok")
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ping", nil))

	want, err := uuid.Parse(rec.Header().Get("X-Request-ID"))
	if err != nil {
		t.Fatalf("expected response request id, got %v", err)
	}
	if fromGin != want || fromRequestCtx != want || fromUtils != want {
		t.Fatalf("ids differ: header=%s gin=%s requestCtx=%s utils=%s", want, fromGin, fromRequestCtx, fromUtils)
	}
}
