package utils

import (
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"sync"

	"github.com/go-playground/validator/v10"

	"github.com/zazhedho/gokit/internal/response"
)

var (
	messageMu        sync.RWMutex
	messageOverrides = map[string]func(param string) string{}
)

// SetValidationMessage overrides the message for a validator tag, for example
// "min". fn receives the tag's parameter ("3" for min=3). A nil fn removes the
// override. It is safe for concurrent use.
func SetValidationMessage(tag string, fn func(param string) string) {
	messageMu.Lock()
	defer messageMu.Unlock()
	if fn == nil {
		delete(messageOverrides, tag)
		return
	}
	messageOverrides[tag] = fn
}

func isCollection(k reflect.Kind) bool {
	return k == reflect.Slice || k == reflect.Array || k == reflect.Map
}

// sized describes a bound for the field's kind: strings are measured by length,
// collections by item count and numbers by value.
func sized(fe validator.FieldError, number, text, items string) string {
	switch {
	case fe.Kind() == reflect.String:
		return text
	case isCollection(fe.Kind()):
		return items
	}
	return number
}

func mapValidateMessage(fe validator.FieldError) string {
	messageMu.RLock()
	override := messageOverrides[fe.Tag()]
	messageMu.RUnlock()
	if override != nil {
		return override(fe.Param())
	}

	p := fe.Param()
	switch fe.Tag() {
	case "required":
		return "This field is required"
	case "email":
		return "Invalid email"
	case "alphanum":
		return "Should be alphanumeric"
	case "min":
		return sized(fe, "Minimum value is "+p, "Minimum length is "+p, "Minimum "+p+" items")
	case "max":
		return sized(fe, "Maximum value is "+p, "Maximum length is "+p, "Maximum "+p+" items")
	case "len":
		return sized(fe, "Should be exactly "+p, "Length should be exactly "+p, "Should contain exactly "+p+" items")
	case "lt":
		return sized(fe, "Should be less than "+p, "Length should be less than "+p, "Should contain fewer than "+p+" items")
	case "lte":
		return sized(fe, "Should be less than or equal to "+p, "Length should be at most "+p, "Should contain at most "+p+" items")
	case "gt":
		return sized(fe, "Should be greater than "+p, "Length should be greater than "+p, "Should contain more than "+p+" items")
	case "gte":
		return sized(fe, "Should be greater than or equal to "+p, "Length should be at least "+p, "Should contain at least "+p+" items")
	case "ltefield":
		return "Should be less than or equal to the related field"
	case "gtefield":
		return "Should be greater than or equal to the related field"
	case "ltfield":
		return "Should be less than the related field"
	case "gtfield":
		return "Should be greater than the related field"
	case "oneof":
		return "Should be one of: " + strings.Join(strings.Fields(p), ", ")
	case "uuid", "uuid3", "uuid4", "uuid5", "uuid_rfc4122", "uuid3_rfc4122", "uuid4_rfc4122", "uuid5_rfc4122":
		return "Should be a valid UUID"
	case "url", "http_url", "uri":
		return "Should be a valid URL"
	}

	return "Invalid value"
}

// jsonFieldPath turns a validator namespace such as "Req.Items[1].Name" into the
// JSON path "items[1].name" using the struct tags of typ. ok is false when the
// path cannot be resolved against typ.
func jsonFieldPath(typ reflect.Type, namespace, tagName string) (string, bool) {
	if typ == nil {
		return "", false
	}
	_, rest, found := strings.Cut(namespace, ".")
	if !found {
		return "", false
	}

	cur := typ
	var parts []string
	for _, segment := range strings.Split(rest, ".") {
		name, indexes, _ := strings.Cut(segment, "[")
		for cur.Kind() == reflect.Pointer {
			cur = cur.Elem()
		}
		if cur.Kind() != reflect.Struct {
			return "", false
		}
		field, ok := cur.FieldByName(name)
		if !ok {
			return "", false
		}

		jsonName, _, _ := strings.Cut(field.Tag.Get(tagName), ",")
		switch {
		case jsonName == "" && field.Anonymous:
			// embedded structs are flattened in JSON; contribute no segment
		case jsonName == "" || jsonName == "-":
			parts = append(parts, field.Name)
		default:
			parts = append(parts, jsonName)
		}
		if indexes != "" {
			parts[len(parts)-1] += "[" + indexes
		}

		cur = field.Type
		for indexes != "" {
			for cur.Kind() == reflect.Pointer {
				cur = cur.Elem()
			}
			if !isCollection(cur.Kind()) {
				return "", false
			}
			cur = cur.Elem()
			_, indexes, _ = strings.Cut(indexes, "[")
		}
	}
	return strings.Join(parts, "."), len(parts) > 0
}

func expectedJSONType(k reflect.Kind) string {
	switch k {
	case reflect.Bool:
		return "boolean"
	case reflect.String:
		return "string"
	case reflect.Slice, reflect.Array:
		return "array"
	case reflect.Struct, reflect.Map:
		return "object"
	case reflect.Invalid, reflect.Uintptr, reflect.Complex64, reflect.Complex128, reflect.Chan, reflect.Func,
		reflect.Interface, reflect.Pointer, reflect.UnsafePointer:
		return "value"
	default:
		return "number"
	}
}

// ValidateError converts a binding or validator error into error details. Field
// names are the JSON paths from tagName (tag options stripped, nested paths
// joined with "."). Decoder errors are mapped to fixed messages and never echo
// Go type or field names; callers should log err itself server-side.
func ValidateError(err error, reflectType reflect.Type, tagName string) []response.ErrorDetail {
	if ve, ok := errors.AsType[validator.ValidationErrors](err); ok {
		out := make([]response.ErrorDetail, len(ve))
		for i, fe := range ve {
			field, _ := jsonFieldPath(reflectType, fe.StructNamespace(), tagName) // "" when unresolvable: never a Go name
			out[i] = response.ErrorDetail{Field: field, Code: fe.Tag(), Message: mapValidateMessage(fe)}
		}
		return out
	}

	if typeErr, ok := errors.AsType[*json.UnmarshalTypeError](err); ok {
		return []response.ErrorDetail{{
			Field:   typeErr.Field,
			Code:    "invalid_type",
			Message: "Should be a " + expectedJSONType(typeErr.Type.Kind()),
		}}
	}
	if _, ok := errors.AsType[*json.SyntaxError](err); ok || errors.Is(err, io.ErrUnexpectedEOF) {
		return []response.ErrorDetail{{Code: "invalid_json", Message: "Malformed JSON"}}
	}
	if errors.Is(err, io.EOF) {
		return []response.ErrorDetail{{Code: "required", Message: "Request body is required"}}
	}
	return []response.ErrorDetail{{Code: "invalid_request", Message: "Invalid request"}}
}
