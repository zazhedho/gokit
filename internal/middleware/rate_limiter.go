package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"reflect"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/zazhedho/logus"

	"github.com/zazhedho/gokit/internal/messages"
	"github.com/zazhedho/gokit/internal/response"
)

const (
	// DefaultRateLimitTimeout bounds each Redis call when RateLimitOptions.Timeout is 0.
	DefaultRateLimitTimeout = 200 * time.Millisecond

	errorLogInterval = 10 * time.Second
)

// rateLimitScript increments the counter and makes sure it has a TTL in one
// atomic step: the expiry is set on the first hit, and re-applied if a counter
// is found without one. It returns {count, remaining TTL in ms}.
const rateLimitScript = `
local count = redis.call('INCR', KEYS[1])
if count == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
local ttl = redis.call('PTTL', KEYS[1])
if ttl < 0 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
  ttl = tonumber(ARGV[1])
end
return {count, ttl}
`

var rateLimitEval = redis.NewScript(rateLimitScript)

// RateLimitOptions configures IPRateLimit and EndpointRateLimit.
type RateLimitOptions struct {
	// Redis stores the counters. Any redis.Cmdable works (client, cluster, ring).
	Redis redis.Cmdable
	// Prefix namespaces the keys: rate_limit:<Prefix>:<key>.
	Prefix string
	// Limit and Window must be positive; the constructors panic otherwise.
	Limit  int
	Window time.Duration
	// KeyFunc identifies the caller (hashed into the key). Return only values the
	// client cannot choose, such as an authenticated user ID; a header it sets
	// lets it dodge the limit. Default: ClientIP, IPv6 masked to /64.
	KeyFunc func(*gin.Context) string
	// FailClosed answers 503 when Redis is missing or fails. The default lets the
	// request through (fail open) and logs the error, at most once per 10 seconds.
	FailClosed bool
	// Timeout bounds each Redis call. Default DefaultRateLimitTimeout (200ms).
	Timeout time.Duration
}

// IPRateLimit allows at most opts.Limit requests per opts.Window per caller
// (see KeyFunc), counted across all routes under opts.Prefix.
func IPRateLimit(opts RateLimitOptions) gin.HandlerFunc {
	prefix := opts.Prefix
	if prefix == "" {
		prefix = "ip"
	}
	return newLimiter(opts, func(_ *gin.Context, caller string) string {
		return fmt.Sprintf("rate_limit:%s:%s", prefix, caller)
	}, "Too many requests from this IP, please try again later")
}

// EndpointRateLimit allows at most opts.Limit requests per opts.Window per
// caller and route pattern.
func EndpointRateLimit(opts RateLimitOptions) gin.HandlerFunc {
	return newLimiter(opts, func(c *gin.Context, caller string) string {
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		if opts.Prefix != "" {
			return fmt.Sprintf("rate_limit:%s:endpoint:%s:%s", opts.Prefix, route, caller)
		}
		return fmt.Sprintf("rate_limit:endpoint:%s:%s", route, caller)
	}, "Rate limit exceeded for this endpoint")
}

type limiter struct {
	opts    RateLimitOptions
	keyOf   func(*gin.Context, string) string
	denied  string
	lastLog atomic.Int64
	skipped atomic.Int64
}

func newLimiter(opts RateLimitOptions, keyOf func(*gin.Context, string) string, denied string) gin.HandlerFunc {
	if opts.Limit <= 0 || opts.Window <= 0 {
		panic("gokit: rate limiter needs a positive Limit and Window")
	}
	if opts.Timeout <= 0 {
		opts.Timeout = DefaultRateLimitTimeout
	}
	l := &limiter{opts: opts, keyOf: keyOf, denied: denied}
	return l.handle
}

func isNilCmdable(c redis.Cmdable) bool {
	if c == nil {
		return true
	}
	rv := reflect.ValueOf(c)
	return rv.Kind() == reflect.Pointer && rv.IsNil()
}

func (l *limiter) handle(c *gin.Context) {
	id := logus.RequestID(c) // read once per request
	if isNilCmdable(l.opts.Redis) {
		l.unavailable(c, id, errors.New("no redis client configured"))
		return
	}

	caller := ""
	if l.opts.KeyFunc != nil {
		if custom := l.opts.KeyFunc(c); custom != "" {
			sum := sha256.Sum256([]byte(custom)) // bound the key size, whatever KeyFunc returns
			caller = hex.EncodeToString(sum[:16])
		}
	}
	if caller == "" {
		caller = maskIP(c.ClientIP())
	}

	// Detached from the request: a client that disconnects must not skip counting.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), l.opts.Timeout)
	defer cancel()
	count, ttl, err := l.hit(ctx, l.keyOf(c, caller))
	if err != nil {
		l.unavailable(c, id, err)
		return
	}

	if count > int64(l.opts.Limit) {
		retry := (ttl + time.Second - 1) / time.Second // round up
		if retry < 1 {
			retry = 1
		}
		c.Header("Retry-After", strconv.FormatInt(int64(retry), 10))
		res := response.ErrorResponse(http.StatusTooManyRequests, messages.MsgSomethingWrong, id, l.denied)
		c.AbortWithStatusJSON(http.StatusTooManyRequests, res)
		return
	}
	c.Next()
}

// hit counts one request for key and returns the count and the remaining TTL.
func (l *limiter) hit(ctx context.Context, key string) (int64, time.Duration, error) {
	window := max(l.opts.Window.Milliseconds(), 1)
	raw, err := rateLimitEval.Eval(ctx, l.opts.Redis, []string{key}, window).Result()
	if err != nil {
		return 0, 0, err
	}
	pair, ok := raw.([]any)
	if !ok || len(pair) != 2 {
		return 0, 0, fmt.Errorf("unexpected counter reply %T", raw)
	}
	count, ok1 := pair[0].(int64)
	ttlMS, ok2 := pair[1].(int64)
	if !ok1 || !ok2 {
		return 0, 0, errors.New("unexpected counter reply types")
	}
	return count, time.Duration(ttlMS) * time.Millisecond, nil
}

// unavailable handles a counter that could not be read: fail open or closed.
func (l *limiter) unavailable(c *gin.Context, id uuid.UUID, err error) {
	l.logError(c, id, err)
	if !l.opts.FailClosed {
		c.Next()
		return
	}
	res := response.ErrorResponse(http.StatusServiceUnavailable, "Service Unavailable", id,
		"The service is temporarily unavailable, please try again later")
	c.Header("Retry-After", "1")
	c.AbortWithStatusJSON(http.StatusServiceUnavailable, res)
}

// logError logs at most once per errorLogInterval, counting what it skipped, so
// a Redis outage cannot flood the log at request rate.
func (l *limiter) logError(c *gin.Context, id uuid.UUID, err error) {
	now := time.Now().UnixNano()
	last := l.lastLog.Load()
	if last != 0 && now-last < int64(errorLogInterval) || !l.lastLog.CompareAndSwap(last, now) {
		l.skipped.Add(1)
		return
	}
	skipped := l.skipped.Swap(0)
	logus.LogContextf(logus.WithRequestID(c.Request.Context(), id), logus.LevelError, "rate limiter: counter unavailable (fail closed: %t, %d similar errors suppressed): %v",
		l.opts.FailClosed, skipped, err)
}

// maskIP returns the key for a client address: IPv4 as is, IPv6 reduced to its
// /64 so a client rotating addresses inside its allocation shares one bucket.
// Unparsable input is returned unchanged.
func maskIP(ip string) string {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return ip
	}
	addr = addr.Unmap()
	if addr.Is6() {
		prefix, err := addr.Prefix(64)
		if err != nil {
			return ip
		}
		return prefix.Masked().String()
	}
	return addr.String()
}
