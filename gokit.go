// Package gokit is a toolkit for Go HTTP services: helpers, a JSON response
// envelope, Gin middleware and a generic GORM repository. Import this path only;
// internal/ may change without notice.
package gokit

import (
	"context"
	"reflect"
	"time"

	"github.com/google/uuid"
	"github.com/zazhedho/gokit/internal/utils"
	"github.com/zazhedho/logus"
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

// ConvertValuesToString returns filters with the values of the given keys converted to strings.
func ConvertValuesToString(filters map[string]any, keys ...string) map[string]any {
	return utils.ConvertValuesToString(filters, keys...)
}

// CreateUUID returns a new random UUID as a string.
func CreateUUID() string { return utils.CreateUUID() }

// DurationFromEnv returns the first positive duration found in the given
// environment variables, or fallback.
func DurationFromEnv(keys []string, fallback time.Duration) time.Duration {
	return utils.DurationFromEnv(keys, fallback)
}

// RequestID returns the request ID for ctx as resolved by logus.
func RequestID(ctx context.Context) uuid.UUID { return utils.RequestID(ctx) }

// EnvValue is the set of types GetEnv can read: the integer kinds up to 64
// bits, float32, float64, bool, string and []string, including named types
// built on them. time.Duration is accepted as an int64 kind and parsed as a
// duration.
type EnvValue = utils.EnvValue

// GetEnv reads an environment variable converted to T. Unset or blank returns
// def; an unparsable value (including NaN, Inf and integer overflow) returns def
// and is reported by EnvErrors. A []string is comma-separated with blanks
// dropped. Durations accept 60 (seconds), 30s, 1m, 1h and "eod" (see
// SetEODLocation).
func GetEnv[T EnvValue](key string, def T) T { return utils.GetEnv(key, def) }

// SetEODLocation sets the zone in which a "eod" duration ends (23:59:59 of the
// current day there). The default, and the result of passing nil, is the
// process's local zone (time.Local).
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

// RedactSensitivePayload returns a copy of input with the values of sensitive
// keys replaced by "[REDACTED]". Keys are matched case-insensitively with '_',
// '-' and '.' ignored. The built-in list is password, passwd, pwd, token,
// secret, otp, apikey, authorization, cookie, credential, privatekey, session,
// jwt, cvv and pin; pwd, otp, jwt, cvv and pin match whole words only, so
// "footprint" is untouched. extraKeys add more names. Maps, slices,
// http.Header, url.Values, structs and pointers are walked recursively
// (structs by JSON field name). A value that cannot be encoded becomes
// "[UNREDACTABLE]" instead of being returned raw.
func RedactSensitivePayload(input any, extraKeys ...string) any {
	return utils.RedactSensitivePayload(input, extraKeys...)
}

// ValidateUUID validates a UUID string and returns it unchanged. It
// returns ErrMissingID or ErrInvalidUUID on failure.
func ValidateUUID(id string) (string, error) { return utils.ValidateUUID(id) }

// WithRequestID returns a context carrying requestID under CtxKeyRequestID.
func WithRequestID(ctx context.Context, requestID uuid.UUID) context.Context {
	return utils.WithRequestID(ctx, requestID)
}

// ValidateError converts validator errors into field messages, using the
// struct tag tagName for field names.
func ValidateError(err error, reflectType reflect.Type, tagName string) []ErrorDetail {
	return utils.ValidateError(err, reflectType, tagName)
}
