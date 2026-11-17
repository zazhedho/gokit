package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	redismock "github.com/go-redis/redismock/v9"
	"github.com/redis/go-redis/v9"
)

func TestIPRateLimitMiddlewareBypassesWhenRedisUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/register", IPRateLimitMiddleware(nil, "register", 1, time.Minute), func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/register", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestEndpointRateLimitMiddlewareBypassesWhenDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/items", EndpointRateLimitMiddleware(nil, 0, time.Minute), func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/items", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestIPRateLimitMiddlewareBlocksWhenLimitExceeded(t *testing.T) {
	client, mock := redismock.NewClientMock()
	key := "rate_limit:register:1.2.3.4"
	mock.ExpectIncr(key).SetVal(2)
	mock.ExpectTTL(key).SetVal(30 * time.Second)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/register", IPRateLimitMiddleware(client, "register", 1, time.Minute), func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/register", nil)
	req.RemoteAddr = "1.2.3.4:12345"
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "30" {
		t.Fatalf("expected 429 with retry header, got %d headers=%v body=%s", rec.Code, rec.Header(), rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("redis expectations: %v", err)
	}
}

func TestEndpointRateLimitMiddlewareAllowsFirstRequest(t *testing.T) {
	client, mock := redismock.NewClientMock()
	key := "rate_limit:endpoint:/items:1.2.3.4"
	mock.ExpectIncr(key).SetVal(1)
	mock.ExpectExpire(key, time.Minute).SetVal(true)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/items", EndpointRateLimitMiddleware(client, 1, time.Minute), func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/items", nil)
	req.RemoteAddr = "1.2.3.4:12345"
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("redis expectations: %v", err)
	}
}

func newIPLimitRouter(client *redis.Client, limit int) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/register", IPRateLimitMiddleware(client, "register", limit, time.Minute), func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{"message": "ok"})
	})
	return router
}

func newEndpointLimitRouter(client *redis.Client, limit int) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/items", EndpointRateLimitMiddleware(client, limit, time.Minute), func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{"message": "ok"})
	})
	return router
}

func serve(router *gin.Engine, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", path, nil)
	req.RemoteAddr = "1.2.3.4:12345"
	router.ServeHTTP(rec, req)
	return rec
}

func TestIPRateLimitMiddlewareRecoversKeyWithoutTTL(t *testing.T) {
	client, mock := redismock.NewClientMock()
	key := "rate_limit:register:1.2.3.4"
	mock.ExpectIncr(key).SetVal(5)
	mock.ExpectTTL(key).SetVal(-1)
	mock.ExpectExpire(key, time.Minute).SetVal(true)

	rec := serve(newIPLimitRouter(client, 1), "/register")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d: %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("redis expectations: %v", err)
	}
}

func TestIPRateLimitMiddlewareExpireFailureOnFirstHitIsRetriedLater(t *testing.T) {
	client, mock := redismock.NewClientMock()
	key := "rate_limit:register:1.2.3.4"
	mock.ExpectIncr(key).SetVal(1)
	mock.ExpectExpire(key, time.Minute).SetErr(errors.New("redis blip"))
	mock.ExpectIncr(key).SetVal(2)
	mock.ExpectTTL(key).SetVal(-1)
	mock.ExpectExpire(key, time.Minute).SetVal(true)

	router := newIPLimitRouter(client, 5)
	if rec := serve(router, "/register"); rec.Code != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec := serve(router, "/register"); rec.Code != http.StatusOK {
		t.Fatalf("second request: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("redis expectations: %v", err)
	}
}

func TestEndpointRateLimitMiddlewareRecoversKeyWithoutTTL(t *testing.T) {
	client, mock := redismock.NewClientMock()
	key := "rate_limit:endpoint:/items:1.2.3.4"
	mock.ExpectIncr(key).SetVal(5)
	mock.ExpectTTL(key).SetVal(-1)
	mock.ExpectExpire(key, time.Minute).SetVal(true)

	rec := serve(newEndpointLimitRouter(client, 1), "/items")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d: %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("redis expectations: %v", err)
	}
}

func TestEndpointRateLimitMiddlewareExpireFailureOnFirstHitIsRetriedLater(t *testing.T) {
	client, mock := redismock.NewClientMock()
	key := "rate_limit:endpoint:/items:1.2.3.4"
	mock.ExpectIncr(key).SetVal(1)
	mock.ExpectExpire(key, time.Minute).SetErr(errors.New("redis blip"))
	mock.ExpectIncr(key).SetVal(2)
	mock.ExpectTTL(key).SetVal(-1)
	mock.ExpectExpire(key, time.Minute).SetVal(true)

	router := newEndpointLimitRouter(client, 5)
	if rec := serve(router, "/items"); rec.Code != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec := serve(router, "/items"); rec.Code != http.StatusOK {
		t.Fatalf("second request: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("redis expectations: %v", err)
	}
}
