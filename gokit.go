// Package gokit is a toolkit for Go HTTP services: helpers, a JSON response
// envelope, Gin middleware and a generic GORM repository. Import this path only;
// internal/ may change without notice.
package gokit

import (
	"context"
	"net/url"
	"reflect"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zazhedho/logus"
	"gorm.io/gorm"

	"github.com/zazhedho/gokit/internal/binding"
	"github.com/zazhedho/gokit/internal/filter"
	"github.com/zazhedho/gokit/internal/messages"
	"github.com/zazhedho/gokit/internal/middleware"
	"github.com/zazhedho/gokit/internal/repository"
	"github.com/zazhedho/gokit/internal/response"
	"github.com/zazhedho/gokit/internal/utils"
)

// ---- Responses ----

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

// ---- Binding and validation ----

// BindOption configures BindJSON.
type BindOption = binding.Option

// WithMaxBodyBytes sets the largest request body BindJSON reads (default 1 MiB).
// Values below 1 keep the default.
func WithMaxBodyBytes(n int64) BindOption { return binding.WithMaxBodyBytes(n) }

// BindJSON binds and validates a JSON body capped at 1 MiB (WithMaxBodyBytes).
// On failure it logs err, aborts with a 400 (413 if too large) and returns false.
func BindJSON[T any](ctx *gin.Context, logID uuid.UUID, logPrefix string, req *T, opts ...BindOption) bool {
	return binding.BindJSON(ctx, logID, logPrefix, req, opts...)
}

// ValidateUUIDParam validates the "id" route parameter and returns it in
// canonical form. On failure it aborts with a 400 envelope.
func ValidateUUIDParam(ctx *gin.Context, logID uuid.UUID) (string, error) {
	return binding.ValidateUUIDParam(ctx, logID)
}

// ValidateUUID validates a UUID string and returns its canonical form
// (lower-case, hyphenated), so every spelling uuid.Parse accepts normalizes to
// one value. It returns ErrMissingID or ErrInvalidUUID on failure.
func ValidateUUID(id string) (string, error) { return utils.ValidateUUID(id) }

var (
	// ErrMissingID is returned by ValidateUUID when the ID is empty.
	ErrMissingID = utils.ErrMissingID
	// ErrInvalidUUID is returned by ValidateUUID when the ID is not a valid UUID.
	ErrInvalidUUID = utils.ErrInvalidUUID
)

// ValidateError turns a binding or validation error into one ErrorDetail per field,
// named by JSON path. Decoder errors get fixed messages with no Go names; log err yourself.
func ValidateError(err error, reflectType reflect.Type, tagName string) []ErrorDetail {
	return utils.ValidateError(err, reflectType, tagName)
}

// SetValidationMessage overrides the message ValidateError produces for a
// validator tag such as "min"; fn receives the tag parameter ("3" for min=3).
// A nil fn removes the override. It is safe for concurrent use.
func SetValidationMessage(tag string, fn func(param string) string) {
	utils.SetValidationMessage(tag, fn)
}

// ---- Middleware ----

// CORSOptions configures CORS: allowed origins, methods and headers, exposed
// response headers (X-Request-ID is always exposed), credentials and the
// preflight cache age.
type CORSOptions = middleware.CORSOptions

// CORS is open by default (any origin, no credentials); set AllowOrigins to
// restrict it. Only real preflights get 204. See the README for details.
func CORS(opts CORSOptions) gin.HandlerFunc { return middleware.CORS(opts) }

// DefaultRateLimitTimeout bounds each Redis call of the rate limiters when
// RateLimitOptions.Timeout is zero.
const DefaultRateLimitTimeout = middleware.DefaultRateLimitTimeout

// RateLimitOptions configures IPRateLimit and EndpointRateLimit: the Redis
// client (any redis.Cmdable), key prefix, limit and window, an optional KeyFunc,
// FailClosed and the Redis Timeout.
type RateLimitOptions = middleware.RateLimitOptions

// IPRateLimit allows at most Limit requests per Window per caller (default
// ClientIP, which depends on your trusted proxies; see the README). Limit and
// Window must be positive or it panics. Redis errors fail open unless FailClosed.
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

// ---- Request ID ----

// CtxKeyRequestID is the context key under which the request ID is stored. It is the
// same key logus reads, so IDs set here appear in log output.
const CtxKeyRequestID = logus.CtxKeyRequestID

// RequestIDMiddleware sets the request ID (a valid X-Request-ID or a new UUID).
// Register it first: without it each component may see a different generated ID.
func RequestIDMiddleware() gin.HandlerFunc { return middleware.RequestIDMiddleware() }

// RequestID returns the request ID for ctx as resolved by logus.
func RequestID(ctx context.Context) uuid.UUID { return utils.RequestID(ctx) }

// WithRequestID returns a context carrying requestID under CtxKeyRequestID.
func WithRequestID(ctx context.Context, requestID uuid.UUID) context.Context {
	return utils.WithRequestID(ctx, requestID)
}

// ---- List queries ----

// Defaults applied by Repository.GetAll when the QueryOptions fields are zero.
const (
	// DefaultMaxLimit is the page size cap (QueryOptions.MaxLimit).
	DefaultMaxLimit = repository.DefaultMaxLimit
	// DefaultMaxSearchLength is the search length cap in runes
	// (QueryOptions.MaxSearchLength).
	DefaultMaxSearchLength = repository.DefaultMaxSearchLength
)

var (
	// ErrNotFound is returned when no row matched: by GetByID, GetOneByField,
	// Update, Delete and SoftDelete. It is gorm.ErrRecordNotFound, so errors.Is
	// works with either name.
	ErrNotFound = repository.ErrNotFound

	// ErrInvalidOrderColumn means the requested order column is not allowed. It is
	// caused by client input, so handlers can answer 400. The error text never
	// echoes the input.
	ErrInvalidOrderColumn = repository.ErrInvalidOrderColumn

	// ErrInvalidOrderDirection means the requested direction is neither asc nor
	// desc. Handlers can answer 400.
	ErrInvalidOrderDirection = repository.ErrInvalidOrderDirection

	// ErrInvalidFilter (wrapped with %w) marks unacceptable filter input; answer 400.
	// The text never echoes the input.
	ErrInvalidFilter = filter.ErrInvalidFilter

	// ErrInvalidQueryOptions reports a server-side misconfiguration: invalid
	// QueryOptions, search column or key column, or a filter sanitizer that
	// returned a key outside AllowedFilters. Answer 500, not 400.
	ErrInvalidQueryOptions = repository.ErrInvalidQueryOptions
)

// BaseParams holds the common list-query parameters (search, filters, paging,
// ordering). Build it with GetBaseParams.
type BaseParams = filter.BaseParams

// FilterSanitizer reduces untrusted filters to the allowed keys.
type FilterSanitizer = repository.FilterSanitizer

// QueryFunc customizes a GORM query.
type QueryFunc = repository.QueryFunc

// QueryOptions configures Repository.GetAll: base scope, search, filter and
// order allow-lists, and the MaxLimit and AllowAll paging guards. Validate
// checks it.
type QueryOptions = repository.QueryOptions

// SearchFunc applies a free-text search to a GORM query.
type SearchFunc = repository.SearchFunc

// GetBaseParams parses search, filters, paging and ordering from query. A
// missing, empty, zero or negative (other than -1) limit becomes defLimit (>= 1);
// -1 passes through. A bad page or limit returns an error and a safe struct.
func GetBaseParams(query url.Values, defOrderBy, defOrderDirection string, defLimit int) (req BaseParams, err error) {
	return filter.GetBaseParams(query, defOrderBy, defOrderDirection, defLimit)
}

// WhitelistFilter keeps only the allowed filter keys.
func WhitelistFilter(filters map[string]any, allowed []string) map[string]any {
	return filter.WhitelistFilter(filters, allowed)
}

// WhitelistStringFilter keeps only the allowed filter keys and converts their
// values to strings (see InterfaceString).
func WhitelistStringFilter(filters map[string]any, allowed []string) map[string]any {
	return filter.WhitelistStringFilter(filters, allowed)
}

// ConvertValuesToString returns a copy of filters with the values of keys
// converted to strings (all values when no keys are given).
func ConvertValuesToString(filters map[string]any, keys ...string) map[string]any {
	return utils.ConvertValuesToString(filters, keys...)
}

// BuildSearchFunc matches the search text as an escaped, case-insensitive substring
// on any of columns. No columns or an invalid one fails the query with ErrInvalidQueryOptions.
func BuildSearchFunc(columns ...string) SearchFunc { return repository.BuildSearchFunc(columns...) }

// ---- Repository ----

// Repository is a generic GORM repository. It is a defined type so IDEs resolve DB when
// embedded. Column names are validated and quoted; values are always bound.
type Repository[T any] struct {
	DB *gorm.DB
	// KeyColumn is the key used by GetByID, Update, Delete and SoftDelete and as
	// the final ORDER BY tie-breaker. Empty means "id".
	KeyColumn string
}

// CRUDRepository is the contract implemented by Repository; depend on it in
// services so tests can substitute a fake.
type CRUDRepository[T any] interface {
	Store(ctx context.Context, m *T) error
	Upsert(ctx context.Context, values []T, conflictColumns, updateColumns []string) error
	GetByID(ctx context.Context, id string) (T, error)
	GetOneByField(ctx context.Context, field string, value any) (T, error)
	GetManyByField(ctx context.Context, field string, value any, limit int, order string) ([]T, error)
	ExistsByField(ctx context.Context, field string, value any) (bool, error)
	ExistsByFields(ctx context.Context, filters map[string]any) (bool, error)
	GetAll(ctx context.Context, params BaseParams, opts QueryOptions) ([]T, int64, error)
	Update(ctx context.Context, m *T) error
	Delete(ctx context.Context, id string) error
	SoftDelete(ctx context.Context, id, deletedBy string) error
}

// NewRepository returns a Repository for T backed by db, keyed on "id".
func NewRepository[T any](db *gorm.DB) *Repository[T] { return &Repository[T]{DB: db} }

func (r *Repository[T]) impl() *repository.Repository[T] {
	return &repository.Repository[T]{DB: r.DB, KeyColumn: r.KeyColumn}
}

// Store inserts m. Generated IDs and database defaults are written back into m.
func (r *Repository[T]) Store(ctx context.Context, m *T) error { return r.impl().Store(ctx, m) }

// Upsert inserts values and, on a conflict over conflictColumns, updates
// updateColumns from the new row. Both column lists are required. Large slices
// are inserted in batches of the DB's CreateBatchSize (500 when unset).
func (r *Repository[T]) Upsert(ctx context.Context, values []T, conflictColumns, updateColumns []string) error {
	return r.impl().Upsert(ctx, values, conflictColumns, updateColumns)
}

// GetByID returns the row whose key column equals id, or ErrNotFound.
func (r *Repository[T]) GetByID(ctx context.Context, id string) (T, error) {
	return r.impl().GetByID(ctx, id)
}

// GetOneByField returns the first row where field equals value (IS NULL when
// value is nil), or ErrNotFound.
func (r *Repository[T]) GetOneByField(ctx context.Context, field string, value any) (T, error) {
	return r.impl().GetOneByField(ctx, field, value)
}

// GetManyByField returns up to limit rows (DefaultMaxLimit if out of range) ordered by
// order ("col asc|desc") then the key. Bad orders wrap ErrInvalidOrder*.
func (r *Repository[T]) GetManyByField(ctx context.Context, field string, value any, limit int, order string) ([]T, error) {
	return r.impl().GetManyByField(ctx, field, value, limit, order)
}

// ExistsByField reports whether any row has field equal to value (IS NULL when
// value is nil).
func (r *Repository[T]) ExistsByField(ctx context.Context, field string, value any) (bool, error) {
	return r.impl().ExistsByField(ctx, field, value)
}

// ExistsByFields reports whether any row matches every field/value pair. An
// empty map returns an error wrapping ErrInvalidFilter instead of matching
// everything.
func (r *Repository[T]) ExistsByFields(ctx context.Context, filters map[string]any) (bool, error) {
	return r.impl().ExistsByFields(ctx, filters)
}

// GetAll returns one page of rows and the total count. The limit is
// QueryOptions.PageSize; the offset is derived from Page when it is >= 1.
func (r *Repository[T]) GetAll(ctx context.Context, params BaseParams, opts QueryOptions) ([]T, int64, error) {
	return r.impl().GetAll(ctx, params, opts)
}

// Update writes every column except the key, created_at, deleted_at, deleted_by and
// associations (zero values included) to the live row; ErrNotFound if none.
func (r *Repository[T]) Update(ctx context.Context, m *T) error { return r.impl().Update(ctx, m) }

// Delete follows gorm: models with gorm.DeletedAt are soft-deleted, others removed.
// ErrNotFound when no row matched.
func (r *Repository[T]) Delete(ctx context.Context, id string) error { return r.impl().Delete(ctx, id) }

// SoftDelete sets deleted_by and deleted_at (UTC) on the live row whose key
// equals id. It returns ErrNotFound when no live row matched, so a second call
// fails. The model needs deleted_at and deleted_by columns.
func (r *Repository[T]) SoftDelete(ctx context.Context, id, deletedBy string) error {
	return r.impl().SoftDelete(ctx, id, deletedBy)
}

// ---- Configuration ----

// EnvValue is the set of types GetEnv reads: signed and unsigned integers,
// floats, bool, string and []string, plus named types on them. time.Duration
// is parsed as a duration.
type EnvValue = utils.EnvValue

// GetEnv reads an environment variable (trimmed) as T. Unset or blank returns
// def; an unparsable value, overflow, NaN, Inf or negative duration returns def
// and is reported by EnvErrors.
func GetEnv[T EnvValue](key string, def T) T { return utils.GetEnv(key, def) }

// SetEODLocation sets the zone "eod" durations end in (default time.Local; nil restores it).
func SetEODLocation(loc *time.Location) { utils.SetEODLocation(loc) }

// EnvErrors returns every variable GetEnv could not parse, or nil. Call it once
// after loading configuration and refuse to start when it is non-nil.
func EnvErrors() error { return utils.EnvErrors() }

// DurationFromEnv returns the first positive duration found in the given
// environment variables, or fallback.
func DurationFromEnv(keys []string, fallback time.Duration) time.Duration {
	return utils.DurationFromEnv(keys, fallback)
}

// ---- Helpers ----

// CreateUUID returns a new random UUID as a string.
func CreateUUID() string { return utils.CreateUUID() }

// InterfaceString is the single value-to-string conversion (see the README); the
// filter helpers use it too.
func InterfaceString(data any) string { return utils.InterfaceString(data) }

// JSONEncode marshals data to a JSON string, returning "" on failure.
func JSONEncode(data any) string { return utils.JSONEncode(data) }

// RedactSensitivePayload masks sensitive values as "[REDACTED]"; the README lists the key rules.
// JSON in a string, []byte or RawMessage is decoded first; undecodable bytes become "[UNREDACTABLE]".
func RedactSensitivePayload(input any, extraKeys ...string) any {
	return utils.RedactSensitivePayload(input, extraKeys...)
}

// ---- Test helpers ----

// ResetEnvErrors clears what EnvErrors reports. Tests only: it resets global
// state shared by every GetEnv caller in the process.
func ResetEnvErrors() { utils.ResetEnvErrors() }
