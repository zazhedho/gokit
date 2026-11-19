package utils

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
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
	"passphrase", "passcode", "accesskey", "bearer", "signature", "assertion", "cardnumber",
}

// sensitiveTokens must equal a whole word of the key. They are short and would
// otherwise match inside words such as "footprint" or "shipping".
var sensitiveTokens = map[string]struct{}{
	"pwd": {}, "otp": {}, "jwt": {}, "cvv": {}, "pin": {}, "pass": {}, "auth": {}, "ssn": {}, "pan": {},
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
		if i > 0 && len(cur) > 0 {
			prev := runes[i-1]
			upperAfterLowerOrDigit := unicode.IsUpper(r) && (unicode.IsLower(prev) || unicode.IsDigit(prev))
			acronymEnd := unicode.IsUpper(r) && unicode.IsUpper(prev) && i+1 < len(runes) && unicode.IsLower(runes[i+1])
			letterDigitEdge := unicode.IsDigit(r) != unicode.IsDigit(prev)
			if upperAfterLowerOrDigit || acronymEnd || letterDigitEdge {
				flush()
			}
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

// maxRedactDepth bounds recursion so a self-referential map cannot hang the caller.
const maxRedactDepth = 32

// RedactSensitivePayload returns a copy of input with sensitive values masked as
// "[REDACTED]"; see the README for the key rules. A string, []byte or
// json.RawMessage that holds JSON is decoded and redacted. A plain string
// scalar is returned as is; undecodable bytes, values that cannot be encoded
// and structures deeper than 32 levels become "[UNREDACTABLE]".
func RedactSensitivePayload(input any, extraKeys ...string) any {
	return redact(input, extraKeys, 0)
}

func redact(input any, extra []string, depth int) any {
	if depth > maxRedactDepth {
		return unredactableValue
	}
	switch v := input.(type) {
	case nil:
		return nil
	case bool, int, int64, float64:
		return v
	case string:
		if decoded, ok := decodeJSON([]byte(v)); ok {
			switch decoded.(type) {
			case map[string]any, []any:
				return redactGeneric(decoded, extra, depth)
			}
		}
		return v
	case []byte:
		return redactBytes(v, extra, depth)
	case json.RawMessage:
		return redactBytes(v, extra, depth)
	case map[string]any:
		return redactMap(v, extra, depth)
	case []any:
		return redactSlice(v, extra, depth)
	case []map[string]any:
		out := make([]map[string]any, len(v))
		for i, m := range v {
			out[i] = redactMap(m, extra, depth)
		}
		return out
	case map[string]string:
		out := make(map[string]string, len(v))
		for k, val := range v {
			if isSensitiveKey(k, extra) {
				val = redactedValue
			}
			out[k] = val
		}
		return out
	case http.Header:
		return http.Header(redactValues(v, extra))
	case url.Values:
		return url.Values(redactValues(v, extra))
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
	return redactBytes(raw, extra, depth)
}

func decodeJSON(raw []byte) (any, bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, false
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, false
	}
	return v, true
}

func redactBytes(raw []byte, extra []string, depth int) any {
	decoded, ok := decodeJSON(raw)
	if !ok {
		return unredactableValue
	}
	return redactGeneric(decoded, extra, depth)
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

func redactGeneric(v any, extra []string, depth int) any {
	switch t := v.(type) {
	case map[string]any:
		return redactMap(t, extra, depth)
	case []any:
		return redactSlice(t, extra, depth)
	default:
		return v
	}
}

func redactMap(in map[string]any, extra []string, depth int) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, val := range in {
		if isSensitiveKey(k, extra) {
			out[k] = redactedValue
			continue
		}
		out[k] = redact(val, extra, depth+1)
	}
	return out
}

func redactSlice(in []any, extra []string, depth int) []any {
	if in == nil {
		return nil
	}
	out := make([]any, len(in))
	for i, val := range in {
		out[i] = redact(val, extra, depth+1)
	}
	return out
}
