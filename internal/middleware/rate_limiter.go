package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/zazhedho/gokit/internal/messages"
	"github.com/zazhedho/gokit/internal/response"
	"github.com/zazhedho/gokit/internal/utils"
	"github.com/zazhedho/logus"
)

// // IPRateLimitMiddleware rate-limits per client IP via Redis; it passes through
// // when the client is nil or limit/window is not positive.
func IPRateLimitMiddleware(redisClient *redis.Client, prefix string, limit int, window time.Duration) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if redisClient == nil || limit <= 0 || window <= 0 {
			ctx.Next()
			return
		}

		logId := utils.GenerateLogId(ctx)
		logPrefix := fmt.Sprintf("[RateLimiter][%s]", prefix)

		ip := ctx.ClientIP()
		key := fmt.Sprintf("rate_limit:%s:%s", prefix, ip)

		reqCtx, cancel := context.WithTimeout(ctx.Request.Context(), 2*time.Second)
		defer cancel()

		current, err := redisClient.Incr(reqCtx, key).Result()
		if err != nil {
			logus.WriteLogWithContext(ctx, logus.LogLevelError, fmt.Sprintf("%s; redis.Incr error: %v", logPrefix, err))
			ctx.Next()
			return
		}

		ttl := ensureWindowExpiry(ctx, redisClient, key, window, current, logPrefix)

		if current > int64(limit) {
			if ttl > 0 {
				ctx.Header("Retry-After", strconv.Itoa(int(ttl.Seconds())))
			}

			res := response.Response(http.StatusTooManyRequests, messages.MsgSomethingWrong, logId, nil)
			res.Error = response.Errors{
				Code:    http.StatusTooManyRequests,
				Message: "Too many requests from this IP, please try again later",
			}
			ctx.AbortWithStatusJSON(http.StatusTooManyRequests, res)
			return
		}

		ctx.Next()
	}
}

// // EndpointRateLimitMiddleware rate-limits per route and client IP via Redis; it
// // passes through when the client is nil or limit/window is not positive.
func EndpointRateLimitMiddleware(redisClient *redis.Client, limit int, window time.Duration) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if redisClient == nil || limit <= 0 || window <= 0 {
			ctx.Next()
			return
		}

		logId := utils.GenerateLogId(ctx)
		logPrefix := "[EndpointRateLimiter]"

		ip := ctx.ClientIP()
		endpoint := ctx.FullPath()
		key := fmt.Sprintf("rate_limit:endpoint:%s:%s", endpoint, ip)

		reqCtx, cancel := context.WithTimeout(ctx.Request.Context(), 2*time.Second)
		defer cancel()

		current, err := redisClient.Incr(reqCtx, key).Result()
		if err != nil {
			logus.WriteLogWithContext(ctx, logus.LogLevelError, fmt.Sprintf("%s; redis.Incr error: %v", logPrefix, err))
			ctx.Next()
			return
		}

		ttl := ensureWindowExpiry(ctx, redisClient, key, window, current, logPrefix)

		if current > int64(limit) {
			if ttl > 0 {
				ctx.Header("Retry-After", strconv.Itoa(int(ttl.Seconds())))
			}

			res := response.Response(http.StatusTooManyRequests, messages.MsgSomethingWrong, logId, nil)
			res.Error = response.Errors{
				Code:    http.StatusTooManyRequests,
				Message: "Rate limit exceeded for this endpoint",
			}
			ctx.AbortWithStatusJSON(http.StatusTooManyRequests, res)
			return
		}

		ctx.Next()
	}
}

// ensureWindowExpiry guarantees the counter key has a TTL (re-applied if Redis
// reports -1) so a failed first EXPIRE cannot leave it immortal. It uses a
// detached 2s context and returns the remaining TTL, or 0 if unknown.
func ensureWindowExpiry(ctx *gin.Context, redisClient *redis.Client, key string, window time.Duration, current int64, logPrefix string) time.Duration {
	expCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx.Request.Context()), 2*time.Second)
	defer cancel()

	if current > 1 {
		ttl, err := redisClient.TTL(expCtx, key).Result()
		if err != nil || ttl != -1 {
			return ttl
		}
	}

	if err := redisClient.Expire(expCtx, key, window).Err(); err != nil {
		logus.WriteLogWithContext(ctx, logus.LogLevelError, fmt.Sprintf("%s; redis.Expire error: %v", logPrefix, err))
		return 0
	}
	return window
}
