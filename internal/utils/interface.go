package utils

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// ToString is the single any-to-string conversion. nil and nil pointers become
// "", strings and bytes pass through, fmt.Stringer and error use their text,
// []string is comma-joined, numbers use their shortest exact form, and anything
// else is JSON-encoded (json.Number keeps its written form).
func ToString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []byte:
		return string(t)
	case bool:
		return strconv.FormatBool(t)
	case int:
		return strconv.FormatInt(int64(t), 10)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(t), 'f', -1, 32)
	case []string:
		return strings.Join(t, ",")
	}

	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer && rv.IsNil() {
		return ""
	}
	switch t := v.(type) {
	case fmt.Stringer:
		return t.String()
	case error:
		return t.Error()
	}

	switch rv.Kind() {
	case reflect.String:
		return rv.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(rv.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(rv.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(rv.Float(), 'f', -1, rv.Type().Bits())
	case reflect.Bool:
		return strconv.FormatBool(rv.Bool())
	}

	encoded, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(encoded)
}

// InterfaceString converts a dynamic value to a string; see ToString.
func InterfaceString(data any) string { return ToString(data) }

// ConvertValuesToString returns a copy of filters with the values of keys
// converted to strings (all values when no keys are given).
func ConvertValuesToString(filters map[string]any, keys ...string) map[string]any {
	if filters == nil {
		return nil
	}
	target := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		target[k] = struct{}{}
	}

	out := make(map[string]any, len(filters))
	for k, v := range filters {
		if _, ok := target[k]; ok || len(target) == 0 {
			out[k] = ToString(v)
		} else {
			out[k] = v
		}
	}
	return out
}
