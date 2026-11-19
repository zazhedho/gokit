// Package gokit is a toolkit for Go HTTP services: helpers, a JSON response
// envelope, Gin middleware and a generic GORM repository. Import this path only;
// internal/ may change without notice.
package gokit

import (
	"context"
	"reflect"
	"time"

	"github.com/google/uuid"
	"github.com/zazhedho/logus"

	"github.com/zazhedho/gokit/internal/utils"
)

// CtxKeyRequestID is the context key under which the request ID is stored. It is the
// same key logus reads, so IDs set here appear in log output.
const CtxKeyRequestID = logus.CtxKeyRequestID

var (
	// ErrMissingID is returned by ValidateUUID when the ID is empty.
	ErrMissingID = utils.ErrMissingID
	// ErrInvalidUUID is returned by ValidateUUID when the ID is not a valid UUID.
	ErrInvalidUUID = utils.ErrInvalidUUID
)

// CreateUUID returns a new random UUID as a string.
func CreateUUID() string { return utils.CreateUUID() }

// DurationFromEnv returns the first positive duration found in the given
// environment variables, or fallback.
func DurationFromEnv(keys []string, fallback time.Duration) time.Duration {
	return utils.DurationFromEnv(keys, fallback)
}

// RequestID returns the request ID for ctx as resolved by logus.
func RequestID(ctx context.Context) uuid.UUID { return utils.RequestID(ctx) }

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

// InterfaceString converts a dynamic value to a string: nil and nil pointers
// give "", strings and bytes pass through, fmt.Stringer and error use their
// text, []string is comma-joined, numbers keep their exact decimal form, and
// anything else is JSON-encoded. ConvertValuesToString and WhitelistStringFilter
// use the same rules.
func InterfaceString(data any) string { return utils.InterfaceString(data) }

// JSONEncode marshals data to a JSON string, returning "" on failure.
func JSONEncode(data any) string { return utils.JSONEncode(data) }

// RedactSensitivePayload returns a copy of input with sensitive values masked as
// "[REDACTED]" (key rules in the README). JSON held in a string, []byte or
// json.RawMessage is decoded first; a plain non-JSON string is returned as is.
// Undecodable bytes, unencodable values and structures over 32 levels deep become
// "[UNREDACTABLE]".
func RedactSensitivePayload(input any, extraKeys ...string) any {
	return utils.RedactSensitivePayload(input, extraKeys...)
}

// ValidateUUID validates a UUID string and returns its canonical form
// (lower-case, hyphenated), so every spelling uuid.Parse accepts normalizes to
// one value. It returns ErrMissingID or ErrInvalidUUID on failure.
func ValidateUUID(id string) (string, error) { return utils.ValidateUUID(id) }

// WithRequestID returns a context carrying requestID under CtxKeyRequestID.
func WithRequestID(ctx context.Context, requestID uuid.UUID) context.Context {
	return utils.WithRequestID(ctx, requestID)
}

// ValidateError converts a binding or validation error into error details,
// one per failed field. Fields are JSON paths taken from the struct tag tagName
// (options such as ",omitempty" are stripped; nested paths use "." and indexes
// "[i]"). JSON syntax and type errors map to fixed messages that never echo Go
// type or field names; log err itself server-side.
func ValidateError(err error, reflectType reflect.Type, tagName string) []ErrorDetail {
	return utils.ValidateError(err, reflectType, tagName)
}

// SetValidationMessage overrides the message ValidateError produces for a
// validator tag such as "min"; fn receives the tag parameter ("3" for min=3).
// A nil fn removes the override. It is safe for concurrent use.
func SetValidationMessage(tag string, fn func(param string) string) {
	utils.SetValidationMessage(tag, fn)
}
