package utils

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"unicode"
)

const (
	redactedValue     = "[REDACTED]"
	unredactableValue = "[UNREDACTABLE]"
)

// sensitiveSubstrings match anywhere in a normalized (lower-cased, separator
// free) key, so "accessToken" and "X-Api-Key" are caught.
var sensitiveSubstrings = []string{
	"password", "passwd", "token", "secret", "apikey", "authorization",
	"cookie", "credential", "privatekey", "session",
}

// sensitiveTokens must equal a whole word of the key. They are short and would
// otherwise match inside words such as "footprint" or "shipping".
var sensitiveTokens = map[string]struct{}{
	"pwd": {}, "otp": {}, "jwt": {}, "cvv": {}, "pin": {},
}

// normalizeSensitiveKey lower-cases key and removes '_', '-' and '.'.
func normalizeSensitiveKey(key string) string {
	var b strings.Builder
	for _, r := range key {
		switch r {
		case '_', '-', '.':
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// keyWords splits a key into lower-cased words on separators and on
// lower-to-upper camel-case boundaries ("userOTP" gives "user", "otp").
func keyWords(key string) []string {
	var words []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			words = append(words, strings.ToLower(string(cur)))
			cur = cur[:0]
		}
	}
	runes := []rune(key)
	for i, r := range runes {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			flush()
			continue
		}
		if i > 0 && unicode.IsUpper(r) && len(cur) > 0 &&
			(unicode.IsLower(runes[i-1]) || unicode.IsDigit(runes[i-1]) ||
				(i+1 < len(runes) && unicode.IsLower(runes[i+1]))) {
			flush()
		}
		cur = append(cur, r)
	}
	flush()
	return words
}

func isSensitiveKey(key string, extra []string) bool {
	normalized := normalizeSensitiveKey(key)
	for _, s := range sensitiveSubstrings {
		if strings.Contains(normalized, s) {
			return true
		}
	}
	for _, word := range keyWords(key) {
		if _, ok := sensitiveTokens[word]; ok {
			return true
		}
	}
	for _, e := range extra {
		if e = normalizeSensitiveKey(e); e != "" && strings.Contains(normalized, e) {
			return true
		}
	}
	return false
}

// RedactSensitivePayload returns a copy of input with the values of sensitive
// keys replaced by "[REDACTED]". Keys are matched case-insensitively with '_',
// '-' and '.' ignored; extraKeys add to the built-in list (password, passwd,
// pwd, token, secret, otp, apikey, authorization, cookie, credential,
// privatekey, session, jwt, cvv, pin). Maps, slices, http.Header, url.Values,
// structs and pointers are walked recursively; structs and other values are
// matched by their JSON field names. Scalars pass through. A value that cannot
// be encoded is replaced by "[UNREDACTABLE]" rather than returned raw.
func RedactSensitivePayload(input any, extraKeys ...string) any {
	switch v := input.(type) {
	case nil:
		return nil
	case string, bool, int, int64, float64:
		return v
	case map[string]any:
		return redactMap(v, extraKeys)
	case []any:
		return redactSlice(v, extraKeys)
	case []map[string]any:
		out := make([]map[string]any, len(v))
		for i, m := range v {
			out[i] = redactMap(m, extraKeys)
		}
		return out
	case map[string]string:
		out := make(map[string]string, len(v))
		for k, val := range v {
			if isSensitiveKey(k, extraKeys) {
				val = redactedValue
			}
			out[k] = val
		}
		return out
	case http.Header:
		return http.Header(redactValues(v, extraKeys))
	case url.Values:
		return url.Values(redactValues(v, extraKeys))
	}

	switch reflect.ValueOf(input).Kind() {
	case reflect.Bool, reflect.String,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return input
	}

	raw, err := json.Marshal(input)
	if err != nil {
		return unredactableValue
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var generic any
	if err := dec.Decode(&generic); err != nil {
		return unredactableValue
	}
	return redactGeneric(generic, extraKeys)
}

func redactValues(in map[string][]string, extra []string) map[string][]string {
	out := make(map[string][]string, len(in))
	for k, vals := range in {
		if isSensitiveKey(k, extra) {
			masked := make([]string, len(vals))
			for i := range masked {
				masked[i] = redactedValue
			}
			out[k] = masked
			continue
		}
		out[k] = append([]string(nil), vals...)
	}
	return out
}

func redactGeneric(v any, extra []string) any {
	switch t := v.(type) {
	case map[string]any:
		return redactMap(t, extra)
	case []any:
		return redactSlice(t, extra)
	default:
		return v
	}
}

func redactMap(in map[string]any, extra []string) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, val := range in {
		if isSensitiveKey(k, extra) {
			out[k] = redactedValue
			continue
		}
		out[k] = RedactSensitivePayload(val, extra...)
	}
	return out
}

func redactSlice(in []any, extra []string) []any {
	if in == nil {
		return nil
	}
	out := make([]any, len(in))
	for i, val := range in {
		out[i] = RedactSensitivePayload(val, extra...)
	}
	return out
}
