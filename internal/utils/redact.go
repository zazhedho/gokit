package utils

import (
	"bytes"
	"cmp"
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
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
		if i > 0 && len(cur) > 0 && startsNewWord(runes, i) {
			flush()
		}
		cur = append(cur, r)
	}
	flush()
	return words
}

// startsNewWord reports whether runes[i] opens a new word: a camel-case hump,
// the last capital of an acronym, or a letter/digit edge. It needs i > 0.
func startsNewWord(runes []rune, i int) bool {
	r, prev := runes[i], runes[i-1]
	upperAfterLowerOrDigit := unicode.IsUpper(r) && (unicode.IsLower(prev) || unicode.IsDigit(prev))
	acronymEnd := unicode.IsUpper(r) && unicode.IsUpper(prev) && i+1 < len(runes) && unicode.IsLower(runes[i+1])
	letterDigitEdge := unicode.IsDigit(r) != unicode.IsDigit(prev)
	return upperAfterLowerOrDigit || acronymEnd || letterDigitEdge
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

// RedactSensitivePayload masks sensitive values; JSON in strings and bytes is decoded.
// Undecodable input, unencodable values and depth over 32 become "[UNREDACTABLE]".
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

	generic, err := toGeneric(reflect.ValueOf(input), depth)
	if err != nil {
		return unredactableValue
	}
	return redactGeneric(generic, extra, depth)
}

var (
	jsonMarshalerType = reflect.TypeFor[json.Marshaler]()
	textMarshalerType = reflect.TypeFor[encoding.TextMarshaler]()
	rawMessageType    = reflect.TypeFor[json.RawMessage]()
)

// toGeneric converts v to maps, slices and scalars the way encoding/json would
// name them, except that []byte and json.RawMessage hold JSON to be redacted
// rather than base64 text.
func toGeneric(v reflect.Value, depth int) (any, error) {
	if depth > maxRedactDepth {
		return nil, errors.New("too deep")
	}
	if v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		return indirectToGeneric(v, depth)
	}
	t := v.Type()
	if t == rawMessageType || (t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Uint8) {
		return redactBytes(v.Bytes(), nil, depth), nil
	}
	if isMarshaler(t) || (v.CanAddr() && isMarshaler(reflect.PointerTo(t))) {
		return viaJSON(v)
	}
	return kindToGeneric(v, depth)
}

func indirectToGeneric(v reflect.Value, depth int) (any, error) {
	if v.IsNil() {
		return nil, nil
	}
	if isMarshaler(v.Type()) {
		return viaJSON(v)
	}
	return toGeneric(v.Elem(), depth+1)
}

func kindToGeneric(v reflect.Value, depth int) (any, error) {
	switch v.Kind() {
	case reflect.Struct:
		out := map[string]any{}
		return out, structFields(v, out, depth)
	case reflect.Map:
		return mapToGeneric(v, depth)
	case reflect.Slice, reflect.Array:
		return sliceToGeneric(v, depth)
	case reflect.Chan, reflect.Func, reflect.UnsafePointer, reflect.Complex64, reflect.Complex128:
		return nil, errors.New("unsupported kind")
	}
	return v.Interface(), nil
}

func isMarshaler(t reflect.Type) bool {
	return t.Implements(jsonMarshalerType) || t.Implements(textMarshalerType)
}

func mapToGeneric(v reflect.Value, depth int) (any, error) {
	if v.IsNil() {
		return nil, nil
	}
	out := make(map[string]any, v.Len())
	iter := v.MapRange()
	for iter.Next() {
		val, err := toGeneric(iter.Value(), depth+1)
		if err != nil {
			return nil, err
		}
		out[fmt.Sprint(iter.Key().Interface())] = val
	}
	return out, nil
}

func sliceToGeneric(v reflect.Value, depth int) (any, error) {
	if v.Kind() == reflect.Slice && v.IsNil() {
		return nil, nil
	}
	out := make([]any, v.Len())
	for i := range out {
		val, err := toGeneric(v.Index(i), depth+1)
		if err != nil {
			return nil, err
		}
		out[i] = val
	}
	return out, nil
}

// embeddedStruct returns the struct promoted by an untagged embedded field.
func embeddedStruct(f reflect.StructField, name string, fv reflect.Value) (reflect.Value, bool) {
	if !f.Anonymous || name != "" {
		return reflect.Value{}, false
	}
	inner := reflect.Indirect(fv)
	return inner, inner.Kind() == reflect.Struct
}

func structFields(v reflect.Value, out map[string]any, depth int) error {
	t := v.Type()
	for i := range t.NumField() {
		f := t.Field(i)
		tag, hasTag := f.Tag.Lookup("json")
		name, opts, _ := strings.Cut(tag, ",")
		if name == "-" && opts == "" && hasTag || (!f.IsExported() && !f.Anonymous) {
			continue
		}
		fv := v.Field(i)
		if inner, ok := embeddedStruct(f, name, fv); ok {
			if err := structFields(inner, out, depth+1); err != nil {
				return err
			}
			continue
		}
		if !f.IsExported() || (strings.Contains(","+opts+",", ",omitempty,") && fv.IsZero()) {
			continue
		}
		val, err := toGeneric(fv, depth+1)
		if err != nil {
			return err
		}
		out[cmp.Or(name, f.Name)] = val
	}
	return nil
}

// viaJSON uses the value's own marshaler and decodes the result.
func viaJSON(v reflect.Value) (any, error) {
	raw, err := json.Marshal(v.Interface())
	if err != nil {
		return nil, err
	}
	decoded, ok := decodeJSON(raw)
	if !ok {
		return nil, errors.New("undecodable")
	}
	return decoded, nil
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
