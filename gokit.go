// Package gokit is a small toolkit of shared building blocks for Go HTTP
// services: general helpers, a JSON response envelope, Gin middleware and a
// generic GORM repository.
//
// Everything is available from this single import path; the implementation
// lives under internal/ and may change without notice.
//
//	import "github.com/zazhedho/gokit"
package gokit

import (
	"context"
	"encoding/json"
	"reflect"
	"time"

	"github.com/google/uuid"
	"github.com/zazhedho/gokit/internal/utils"
)

// CtxKeyId is the context key under which the request ID is stored. It is the
// same key logus reads, so IDs set here appear in log output.
const CtxKeyId = utils.CtxKeyId

var (
	// ErrMissingID is returned by ValidateUUID when the ID is empty.
	ErrMissingID = utils.ErrMissingID
	// ErrInvalidUUID is returned by ValidateUUID when the ID is not a valid UUID.
	ErrInvalidUUID = utils.ErrInvalidUUID
)

// ValidateMessage describes a single field validation failure.
type ValidateMessage = utils.ValidateMessage

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

// EmptyJSON returns the raw JSON object "{}".
func EmptyJSON() json.RawMessage { return utils.EmptyJSON() }

// EnvKey upper-cases value and replaces "-", "." and spaces with "_".
func EnvKey(value string) string { return utils.EnvKey(value) }

// FirstNonEmptyString returns the first non-empty value, or "".
func FirstNonEmptyString(values ...string) string { return utils.FirstNonEmptyString(values...) }

// GenerateLogId returns the request ID for ctx as resolved by logus.
func GenerateLogId(ctx context.Context) uuid.UUID { return utils.GenerateLogId(ctx) }

// GetEnv reads an environment variable converted to T, returning def when it
// is unset or invalid.
func GetEnv[T any](key string, def T) T { return utils.GetEnv(key, def) }

// GetRequestID returns the request ID stored in ctx as a string.
func GetRequestID(ctx context.Context) string { return utils.GetRequestID(ctx) }

// HumanizeKey replaces "_" and "-" with spaces and collapses whitespace.
func HumanizeKey(value string) string { return utils.HumanizeKey(value) }

// Int64PtrFromString parses value into an *int64, or nil when it is empty or not a number.
func Int64PtrFromString(value string) *int64 { return utils.Int64PtrFromString(value) }

// Int64PtrIfPositive returns a pointer to value when it is greater than zero, else nil.
func Int64PtrIfPositive(value int64) *int64 { return utils.Int64PtrIfPositive(value) }

// InterfaceBool converts a dynamic value to a bool.
func InterfaceBool(data any) bool { return utils.InterfaceBool(data) }

// InterfaceString converts a dynamic value to a string.
func InterfaceString(data any) string { return utils.InterfaceString(data) }

// IsSensitiveKey reports whether key looks like a password, token, secret or OTP field.
func IsSensitiveKey(key string) bool { return utils.IsSensitiveKey(key) }

// JsonEncode marshals data to a JSON string, returning "" on failure.
func JsonEncode(data any) string { return utils.JsonEncode(data) }

// MergeMetadata returns a new map of base overlaid with extra, or nil when both are empty.
func MergeMetadata(base, extra map[string]any) map[string]any {
	return utils.MergeMetadata(base, extra)
}

// MustJSON marshals value to JSON, returning an empty object on failure.
func MustJSON(value any) json.RawMessage { return utils.MustJSON(value) }

// NormalizeKey lower-cases and trims a key.
func NormalizeKey(value string) string { return utils.NormalizeKey(value) }

// NormalizePayload round-trips input through JSON into plain maps, slices and scalars; nil becomes an empty map.
func NormalizePayload(input any) any { return utils.NormalizePayload(input) }

// NormalizePhoneTo62 normalizes an Indonesian phone number to the 62 prefix.
func NormalizePhoneTo62(phoneInput string) string { return utils.NormalizePhoneTo62(phoneInput) }

// NormalizeUUIDPointer returns a pointer to the trimmed input when it is a valid UUID, else nil.
func NormalizeUUIDPointer(input string) *string { return utils.NormalizeUUIDPointer(input) }

// NormalizeUpperKey upper-cases and trims a key.
func NormalizeUpperKey(value string) string { return utils.NormalizeUpperKey(value) }

// RedactSensitivePayload returns input with sensitive fields masked.
func RedactSensitivePayload(input any) any { return utils.RedactSensitivePayload(input) }

// RedactSensitiveValue returns input with sensitive values masked.
func RedactSensitiveValue(input any) any { return utils.RedactSensitiveValue(input) }

// SanitizeEmail lower-cases and trims an email address, returning "" when it is not a valid bare address.
func SanitizeEmail(email string) string { return utils.SanitizeEmail(email) }

// StringOrDefault returns value, or fallback when value is empty.
func StringOrDefault(value, fallback string) string { return utils.StringOrDefault(value, fallback) }

// StringPtrIfNotEmpty returns a pointer to value, or nil when it is empty.
func StringPtrIfNotEmpty(value string) *string { return utils.StringPtrIfNotEmpty(value) }

// StripHTML removes script and style blocks and tags from s, unescapes HTML
// entities and collapses whitespace.
func StripHTML(s string) string { return utils.StripHTML(s) }

// TitleCase converts s to title case using English casing rules.
func TitleCase(s string) string { return utils.TitleCase(s) }

// TitleHumanized humanizes a key and converts it to title case.
func TitleHumanized(value string) string { return utils.TitleHumanized(value) }

// ValidateUUID validates a UUID string and returns its canonical form. It
// returns ErrMissingID or ErrInvalidUUID on failure.
func ValidateUUID(id string) (string, error) { return utils.ValidateUUID(id) }

// WithRequestID returns a context carrying requestID under CtxKeyId.
func WithRequestID(ctx context.Context, requestID uuid.UUID) context.Context {
	return utils.WithRequestID(ctx, requestID)
}

// ValidateError converts validator errors into field messages, using the
// struct tag tagName for field names.
func ValidateError(err error, reflectType reflect.Type, tagName string) []ValidateMessage {
	return utils.ValidateError(err, reflectType, tagName)
}
