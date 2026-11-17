package gokit

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/zazhedho/gokit/internal/binding"
	"github.com/zazhedho/gokit/internal/messages"
	"github.com/zazhedho/gokit/internal/middleware"
	"github.com/zazhedho/gokit/internal/response"
)

// Standard response messages.
const (
	// MsgInvalidRequest is returned when a request body cannot be bound or validated.
	MsgInvalidRequest = messages.MsgInvalidRequest
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

// ApiResponse is the standard JSON response envelope.
type ApiResponse = response.ApiResponse

// Error is the envelope type used for error responses.
type Error = response.Error

// Errors is a code and message pair for error details.
type Errors = response.Errors

// PaginatedResponse is the JSON envelope for paginated lists.
type PaginatedResponse = response.PaginatedResponse

// Pagination is the envelope type used for paginated responses.
type Pagination = response.Pagination

// Success is the envelope type used for success responses.
type Success = response.Success

// ErrorResponse builds an error envelope with a public error detail.
func ErrorResponse(code int, msg string, logId uuid.UUID, publicError string) *ApiResponse {
	return response.ErrorResponse(code, msg, logId, publicError)
}

// Forbidden builds a 403 envelope.
func Forbidden(logId uuid.UUID, publicError string) *ApiResponse {
	return response.Forbidden(logId, publicError)
}

// InternalServerError builds a 500 envelope that references logId.
func InternalServerError(logId uuid.UUID) *ApiResponse { return response.InternalServerError(logId) }

// Response builds an envelope for code, msg and data.
func Response(code int, msg string, logId uuid.UUID, data any) *ApiResponse {
	return response.Response(code, msg, logId, data)
}

// Unauthorized builds a 401 envelope.
func Unauthorized(logId uuid.UUID, publicError string) *ApiResponse {
	return response.Unauthorized(logId, publicError)
}

// PaginationResponse builds a paginated envelope.
func PaginationResponse(code, total, page, perPage int, logId uuid.UUID, data any) *PaginatedResponse {
	return response.PaginationResponse(code, total, page, perPage, logId, data)
}

// CORS returns the CORS middleware.
func CORS() gin.HandlerFunc { return middleware.CORS() }

// EndpointRateLimitMiddleware limits requests per endpoint using Redis. A nil
// client (or a non-positive limit or window) disables limiting.
func EndpointRateLimitMiddleware(redisClient *redis.Client, limit int, window time.Duration) gin.HandlerFunc {
	return middleware.EndpointRateLimitMiddleware(redisClient, limit, window)
}

// ErrorHandler is a gin.CustomRecovery handler that logs the panic and writes a 500 envelope.
func ErrorHandler(c *gin.Context, err any) { middleware.ErrorHandler(c, err) }

// IPRateLimitMiddleware limits requests per client IP using Redis. A nil
// client (or a non-positive limit or window) disables limiting.
func IPRateLimitMiddleware(redisClient *redis.Client, prefix string, limit int, window time.Duration) gin.HandlerFunc {
	return middleware.IPRateLimitMiddleware(redisClient, prefix, limit, window)
}

// SetContextId returns middleware that assigns a request ID to each request.
func SetContextId() gin.HandlerFunc { return middleware.SetContextId() }

// BindJSON binds the request body into req and writes a 400 envelope on failure.
func BindJSON[T any](ctx *gin.Context, logID uuid.UUID, logPrefix string, req *T) bool {
	return binding.BindJSON(ctx, logID, logPrefix, req)
}

// ValidateUUIDParam validates the "id" route parameter and writes a 400
// envelope on failure.
func ValidateUUIDParam(ctx *gin.Context, logID uuid.UUID) (string, error) {
	return binding.ValidateUUIDParam(ctx, logID)
}
