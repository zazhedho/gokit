package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	redismock "github.com/go-redis/redismock/v9"
	"github.com/redis/go-redis/v9"
)

const windowMS = int64(60000)

func okHandler(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"message": "ok"}) }

func newRouter(mw gin.HandlerFunc, trustProxies bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	if !trustProxies {
		_ = router.SetTrustedProxies(nil)
	}
	router.GET("/items", mw, okHandler)
	return router
}

func serve(router *gin.Engine, remote string, headers map[string]string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/items", nil)
	req.RemoteAddr = remote
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	router.ServeHTTP(rec, req)
	return rec
}

// expectHit registers one atomic counter call for key.
func expectHit(mock redismock.ClientMock, key string, count, ttlMS int64) {
	mock.ExpectEval(rateLimitScript, []string{key}, windowMS).SetVal([]any{count, ttlMS})
}

func TestLimitersPassThroughWithoutRedis(t *testing.T) {
	var typedNil *redis.Client
	for name, opts := range map[string]RateLimitOptions{
		"nil redis":       {Prefix: "p", Limit: 1, Window: time.Minute},
		"typed nil redis": {Redis: typedNil, Prefix: "p", Limit: 1, Window: time.Minute},
	} {
		for kind, mw := range map[string]gin.HandlerFunc{"ip": IPRateLimit(opts), "endpoint": EndpointRateLimit(opts)} {
			router := newRouter(mw, false)
			for i := range 3 {
				if rec := serve(router, "192.0.2.1:1", nil); rec.Code != http.StatusOK {
					t.Fatalf("%s/%s: request %d got %d", name, kind, i, rec.Code)
				}
			}
		}
	}
}

func TestNonPositiveLimitOrWindowPanicsAtConstruction(t *testing.T) {
	client, _ := redismock.NewClientMock()
	for name, opts := range map[string]RateLimitOptions{
		"zero limit":      {Redis: client, Limit: 0, Window: time.Minute},
		"negative limit":  {Redis: client, Limit: -1, Window: time.Minute},
		"zero window":     {Redis: client, Limit: 1},
		"negative window": {Redis: client, Limit: 1, Window: -time.Second},
	} {
		for kind, build := range map[string]func(RateLimitOptions) gin.HandlerFunc{"ip": IPRateLimit, "endpoint": EndpointRateLimit} {
			func() {
				defer func() {
					if recover() == nil {
						t.Errorf("%s/%s: must panic", name, kind)
					}
				}()
				build(opts)
			}()
		}
	}
}

func hashed(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:16])
}

func TestCustomKeysAreHashedAndBounded(t *testing.T) {
	long := strings.Repeat("x", 5000)
	sum := sha256.Sum256([]byte(long))
	want := "rate_limit:p:" + hex.EncodeToString(sum[:16])
	client, mock := redismock.NewClientMock()
	expectHit(mock, want, 1, windowMS)
	router := newRouter(IPRateLimit(RateLimitOptions{
		Redis: client, Prefix: "p", Limit: 5, Window: time.Minute,
		KeyFunc: func(*gin.Context) string { return long },
	}), false)
	serve(router, "192.0.2.1:1", nil)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("custom keys must be hashed to a fixed length: %v", err)
	}
	if len(want) > 64 {
		t.Fatalf("key too long: %d", len(want))
	}
}

func TestRedisCallSurvivesClientDisconnect(t *testing.T) {
	p := &probe{}
	router := newRouter(IPRateLimit(RateLimitOptions{Redis: p, Prefix: "p", Limit: 1, Window: time.Minute}), false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/items", nil).WithContext(ctx)
	router.ServeHTTP(rec, req)
	if p.ctxErr != nil {
		t.Fatalf("a canceled request must not cancel the counter call: %v", p.ctxErr)
	}
}

func TestFailClosedRejectsWhenRedisIsMissing(t *testing.T) {
	router := newRouter(IPRateLimit(RateLimitOptions{Prefix: "p", Limit: 1, Window: time.Minute, FailClosed: true}), false)
	rec := serve(router, "192.0.2.1:1", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestIPRateLimitCountsAtomicallyAndAllows(t *testing.T) {
	client, mock := redismock.NewClientMock()
	expectHit(mock, "rate_limit:register:192.0.2.1", 1, windowMS)
	router := newRouter(IPRateLimit(RateLimitOptions{Redis: client, Prefix: "register", Limit: 1, Window: time.Minute}), false)

	if rec := serve(router, "192.0.2.1:5000", nil); rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("exactly one Eval and no separate INCR/EXPIRE/TTL calls expected: %v", err)
	}
}

func TestRateLimitBlocksWithRoundedUpRetryAfter(t *testing.T) {
	for _, tc := range []struct {
		ttlMS int64
		want  string
	}{{30000, "30"}, {1500, "2"}, {1, "1"}, {59001, "60"}} {
		client, mock := redismock.NewClientMock()
		expectHit(mock, "rate_limit:register:192.0.2.1", 2, tc.ttlMS)
		router := newRouter(IPRateLimit(RateLimitOptions{Redis: client, Prefix: "register", Limit: 1, Window: time.Minute}), false)

		rec := serve(router, "192.0.2.1:1", nil)
		if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != tc.want {
			t.Fatalf("ttl %dms: got %d Retry-After=%q", tc.ttlMS, rec.Code, rec.Header().Get("Retry-After"))
		}
		var body struct {
			Error []struct{ Code, Message string } `json:"error"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body.Error) != 1 || body.Error[0].Code != "too_many_requests" {
			t.Fatalf("expected the error list shape, got %s", rec.Body.String())
		}
	}
}

func TestEndpointRateLimitKeysOnRouteAndClient(t *testing.T) {
	client, mock := redismock.NewClientMock()
	expectHit(mock, "rate_limit:endpoint:/items:192.0.2.1", 2, windowMS)
	router := newRouter(EndpointRateLimit(RateLimitOptions{Redis: client, Limit: 1, Window: time.Minute}), false)
	if rec := serve(router, "192.0.2.1:1", nil); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("got %d", rec.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSpoofedForwardedHeaderDoesNotChangeTheKey(t *testing.T) {
	client, mock := redismock.NewClientMock()
	mock.MatchExpectationsInOrder(false)
	expectHit(mock, "rate_limit:p:192.0.2.1", 1, windowMS)
	expectHit(mock, "rate_limit:p:192.0.2.1", 2, windowMS)
	router := newRouter(IPRateLimit(RateLimitOptions{Redis: client, Prefix: "p", Limit: 1, Window: time.Minute}), false)

	// With no trusted proxy, X-Forwarded-For is ignored: a rotating spoofed value shares one bucket.
	serve(router, "192.0.2.1:1", map[string]string{"X-Forwarded-For": "10.0.0.1"})
	serve(router, "192.0.2.1:2", map[string]string{"X-Forwarded-For": "10.0.0.2"})
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("spoofed headers must not create separate buckets: %v", err)
	}
}

func TestKeyFuncOverridesTheDefault(t *testing.T) {
	client, mock := redismock.NewClientMock()
	expectHit(mock, "rate_limit:p:"+hashed("user-42"), 1, windowMS)
	router := newRouter(IPRateLimit(RateLimitOptions{
		Redis: client, Prefix: "p", Limit: 5, Window: time.Minute,
		KeyFunc: func(c *gin.Context) string { return c.GetHeader("X-User") },
	}), false)
	serve(router, "192.0.2.1:1", map[string]string{"X-User": "user-42"})
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}

	// An empty key falls back to the client address instead of sharing one bucket.
	expectHit(mock, "rate_limit:p:192.0.2.1", 1, windowMS)
	serve(router, "192.0.2.1:1", nil)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultKeyMasksIPv6To64(t *testing.T) {
	for in, want := range map[string]string{
		"192.0.2.1":                      "192.0.2.1",
		"2001:db8:1:2:aaaa:bbbb:cccc:1":  "2001:db8:1:2::/64",
		"2001:db8:1:2:1111:2222:3333:99": "2001:db8:1:2::/64",
		"::ffff:192.0.2.7":               "192.0.2.7",
		"not-an-ip":                      "not-an-ip",
	} {
		if got := maskIP(in); got != want {
			t.Errorf("maskIP(%q) = %q, want %q", in, got, want)
		}
	}

	client, mock := redismock.NewClientMock()
	expectHit(mock, "rate_limit:p:2001:db8:1:2::/64", 1, windowMS)
	router := newRouter(IPRateLimit(RateLimitOptions{Redis: client, Prefix: "p", Limit: 5, Window: time.Minute}), false)
	serve(router, "[2001:db8:1:2:dead:beef:0:1]:9", nil)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("the default key must use the /64: %v", err)
	}
}

func TestRedisErrorsFailOpenByDefaultAndClosedOnRequest(t *testing.T) {
	for name, tc := range map[string]struct {
		failClosed bool
		want       int
	}{"open": {false, http.StatusOK}, "closed": {true, http.StatusServiceUnavailable}} {
		client, mock := redismock.NewClientMock()
		mock.ExpectEval(rateLimitScript, []string{"rate_limit:p:192.0.2.1"}, windowMS).SetErr(errors.New("redis down"))
		router := newRouter(IPRateLimit(RateLimitOptions{Redis: client, Prefix: "p", Limit: 1, Window: time.Minute, FailClosed: tc.failClosed}), false)
		rec := serve(router, "192.0.2.1:1", nil)
		if rec.Code != tc.want {
			t.Fatalf("%s: got %d, want %d", name, rec.Code, tc.want)
		}
		if tc.failClosed && strings.Contains(rec.Body.String(), "redis down") {
			t.Fatalf("503 body must not leak the Redis error: %s", rec.Body.String())
		}
	}
}

// probe is a redis.Cmdable that records the deadline of the context it is given.
type probe struct {
	redis.Cmdable
	mu        sync.Mutex
	remaining time.Duration
	calls     int
	ctxErr    error
}

func (p *probe) Eval(ctx context.Context, _ string, _ []string, _ ...any) *redis.Cmd {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	p.ctxErr = ctx.Err()
	if deadline, ok := ctx.Deadline(); ok {
		p.remaining = time.Until(deadline)
	}
	cmd := redis.NewCmd(ctx)
	cmd.SetErr(errors.New("probe"))
	return cmd
}

func TestRedisTimeoutIsConfigurableWithA200msDefault(t *testing.T) {
	for name, tc := range map[string]struct {
		timeout time.Duration
		lo, hi  time.Duration
	}{
		"default": {0, 100 * time.Millisecond, 200 * time.Millisecond},
		"custom":  {time.Second, 500 * time.Millisecond, time.Second},
	} {
		p := &probe{}
		router := newRouter(IPRateLimit(RateLimitOptions{Redis: p, Prefix: "p", Limit: 1, Window: time.Minute, Timeout: tc.timeout}), false)
		serve(router, "192.0.2.1:1", nil)
		if p.calls != 1 || p.remaining <= tc.lo || p.remaining > tc.hi {
			t.Fatalf("%s: calls=%d remaining=%v, want within (%v, %v]", name, p.calls, p.remaining, tc.lo, tc.hi)
		}
	}
}

func TestLimiterAcceptsAnyCmdable(t *testing.T) {
	var _ redis.Cmdable = (*redis.ClusterClient)(nil)
	var _ redis.Cmdable = (*redis.Client)(nil)
	var _ redis.Cmdable = redis.Pipeliner(nil)
	_ = IPRateLimit(RateLimitOptions{Redis: &probe{}, Limit: 1, Window: time.Second})
}

func TestRedisErrorLogIsRateLimited(t *testing.T) {
	logs := captureLogs(t)
	p := &probe{}
	router := newRouter(IPRateLimit(RateLimitOptions{Redis: p, Prefix: "p", Limit: 1, Window: time.Minute}), false)
	for range 25 {
		serve(router, "192.0.2.1:1", nil)
	}
	if n := strings.Count(logs.String(), "rate limiter"); n != 1 {
		t.Fatalf("expected one log line for 25 identical failures, got %d:\n%s", n, logs.String())
	}
}

func TestErrorLogThrottleIsSafeUnderConcurrency(t *testing.T) {
	logs := captureLogs(t)
	router := newRouter(IPRateLimit(RateLimitOptions{Redis: &probe{}, Prefix: "p", Limit: 1, Window: time.Minute}), false)
	var wg sync.WaitGroup
	for range 40 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 10 {
				router.ServeHTTP(httptest.NewRecorder(), func() *http.Request {
					r := httptest.NewRequest(http.MethodGet, "/items", nil)
					r.RemoteAddr = "192.0.2.1:1"
					return r
				}())
			}
		}()
	}
	wg.Wait()
	if n := strings.Count(logs.String(), "rate limiter"); n != 1 {
		t.Fatalf("400 concurrent failures must log once, got %d", n)
	}
}
