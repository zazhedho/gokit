package gokit

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/zazhedho/gokit/internal/binding"
	"github.com/zazhedho/gokit/internal/messages"
	"github.com/zazhedho/gokit/internal/middleware"
	"github.com/zazhedho/gokit/internal/response"
)

// Standard response messages.
const (
	// MsgInvalidRequest is returned when a request body cannot be bound or validated.
	MsgInvalidRequest = messages.MsgInvalidRequest
	// MsgRequestTooLarge is returned when a request body exceeds the limit.
	MsgRequestTooLarge = messages.MsgRequestTooLarge
	// MsgSomethingWrong is a generic failure message.
	MsgSomethingWrong = messages.MsgSomethingWrong
	// MsgInternal is a format string taking the log ID.
	MsgInternal = messages.MsgInternal
	// MsgExists is returned when a resource already exists.
	MsgExists = messages.MsgExists
	// MsgNotFound is returned when data is not found.
	MsgNotFound = messages.MsgNotFound
	// MsgResourceNotFound is returned when a requested resource does not exist.
	MsgResourceNotFound = messages.MsgResourceNotFound
	// MsgSuccess is the generic success message.
	MsgSuccess = messages.MsgSuccess
	// MsgAccessDenied is returned when the caller lacks permission.
	MsgAccessDenied = messages.MsgAccessDenied
)

// APIResponse is the standard JSON response envelope.
type APIResponse = response.APIResponse

// ErrorDetail is one entry of an envelope's error list: a field (when the
// error is about one), a machine code and a caller-safe message.
type ErrorDetail = response.ErrorDetail

// PaginatedResponse is the JSON envelope for paginated lists.
type PaginatedResponse = response.PaginatedResponse

// ErrorResponse builds an error envelope with a public error detail.
func ErrorResponse(code int, msg string, logID uuid.UUID, publicError string) *APIResponse {
	return response.ErrorResponse(code, msg, logID, publicError)
}

// Forbidden builds a 403 envelope.
func Forbidden(logID uuid.UUID, publicError string) *APIResponse {
	return response.Forbidden(logID, publicError)
}

// InternalServerError builds a 500 envelope that references logID.
func InternalServerError(logID uuid.UUID) *APIResponse { return response.InternalServerError(logID) }

// Response builds an envelope for code, msg and data.
func Response(code int, msg string, logID uuid.UUID, data any) *APIResponse {
	return response.Response(code, msg, logID, data)
}

// Unauthorized builds a 401 envelope.
func Unauthorized(logID uuid.UUID, publicError string) *APIResponse {
	return response.Unauthorized(logID, publicError)
}

// PaginationResponse builds a paginated envelope.
func PaginationResponse(code int, total int64, page, perPage int, logID uuid.UUID, data any) *PaginatedResponse {
	return response.PaginationResponse(code, total, page, perPage, logID, data)
}

// CORSOptions configures CORS: allowed origins, methods and headers, exposed
// response headers (X-Request-ID is always exposed), credentials and the
// preflight cache age.
type CORSOptions = middleware.CORSOptions

// CORS returns CORS middleware. Requests without an Origin header pass through.
// A real preflight (OPTIONS carrying Origin and Access-Control-Request-Method)
// from an allowed origin gets 204 with the Allow headers, and from any other
// origin 403; other OPTIONS requests reach your handlers. Allowed origins are
// echoed on actual requests with the Expose headers. With no AllowOrigins no
// origin is allowed. Combining the "*" origin with AllowCredentials panics.
func CORS(opts CORSOptions) gin.HandlerFunc { return middleware.CORS(opts) }

// DefaultRateLimitTimeout bounds each Redis call of the rate limiters when
// RateLimitOptions.Timeout is zero.
const DefaultRateLimitTimeout = middleware.DefaultRateLimitTimeout

// RateLimitOptions configures IPRateLimit and EndpointRateLimit: the Redis
// client (any redis.Cmdable), key prefix, limit and window, an optional KeyFunc,
// FailClosed and the Redis Timeout.
type RateLimitOptions = middleware.RateLimitOptions

// IPRateLimit allows at most opts.Limit requests per opts.Window per caller,
// counted in Redis under rate_limit:<Prefix>:<caller>. The caller defaults to
// gin's ClientIP (IPv6 masked to /64), which trusts forwarding headers according
// to the engine's trusted proxies; see the README. Over the limit it answers 429
// with Retry-After rounded up to whole seconds. The counter is one atomic Redis
// script. A missing client, or a non-positive Limit or Window, disables limiting;
// a Redis error lets the request through unless opts.FailClosed, which answers
// 503. Errors are logged at most once per 10 seconds.
func IPRateLimit(opts RateLimitOptions) gin.HandlerFunc { return middleware.IPRateLimit(opts) }

// EndpointRateLimit is IPRateLimit with the counter also keyed on the route
// pattern, so each endpoint has its own budget per caller.
func EndpointRateLimit(opts RateLimitOptions) gin.HandlerFunc {
	return middleware.EndpointRateLimit(opts)
}

// Recovery recovers from panics: it logs the panic and stack, and answers 500
// with an envelope that carries only the log ID. Register it after
// RequestIDMiddleware.
func Recovery() gin.HandlerFunc { return middleware.Recovery() }

// RequestIDMiddleware assigns each request an ID (a valid X-Request-ID UUID,
// else a new one), stores it under CtxKeyRequestID and on the request context,
// and echoes it in the X-Request-ID response header. Register it first.
func RequestIDMiddleware() gin.HandlerFunc { return middleware.RequestIDMiddleware() }

// BindOption configures BindJSON.
type BindOption = binding.Option

// WithMaxBodyBytes sets the largest request body BindJSON reads (default 1 MiB).
// Values below 1 keep the default.
func WithMaxBodyBytes(n int64) BindOption { return binding.WithMaxBodyBytes(n) }

// BindJSON binds the JSON request body into req and validates its binding tags.
// The body is capped (1 MiB unless WithMaxBodyBytes says otherwise). On failure
// it logs the full error with logPrefix, aborts with a 400 envelope whose error
// list names JSON fields (413 for an oversized body), and returns false.
func BindJSON[T any](ctx *gin.Context, logID uuid.UUID, logPrefix string, req *T, opts ...BindOption) bool {
	return binding.BindJSON(ctx, logID, logPrefix, req, opts...)
}

// ValidateUUIDParam validates the "id" route parameter and returns it in
// canonical form. On failure it aborts with a 400 envelope.
func ValidateUUIDParam(ctx *gin.Context, logID uuid.UUID) (string, error) {
	return binding.ValidateUUIDParam(ctx, logID)
}
