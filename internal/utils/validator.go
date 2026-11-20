package utils

import (
	"encoding/json"
	"errors"
	"fmt"
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

// sizedMessage holds one message per field kind; %s is the tag's parameter.
type sizedMessage struct{ number, text, items string }

// sizedMessages are the tags whose wording depends on whether the field is a
// number, a string (measured by length) or a collection (measured by item count).
var sizedMessages = map[string]sizedMessage{
	"min": {"Minimum value is %s", "Minimum length is %s", "Minimum %s items"},
	"max": {"Maximum value is %s", "Maximum length is %s", "Maximum %s items"},
	"len": {"Should be exactly %s", "Length should be exactly %s", "Should contain exactly %s items"},
	"lt":  {"Should be less than %s", "Length should be less than %s", "Should contain fewer than %s items"},
	"lte": {"Should be less than or equal to %s", "Length should be at most %s", "Should contain at most %s items"},
	"gt":  {"Should be greater than %s", "Length should be greater than %s", "Should contain more than %s items"},
	"gte": {"Should be greater than or equal to %s", "Length should be at least %s", "Should contain at least %s items"},
}

// format picks the wording for the field's kind and fills in the parameter.
func (m sizedMessage) format(fe validator.FieldError) string {
	template := m.number
	switch {
	case fe.Kind() == reflect.String:
		template = m.text
	case isCollection(fe.Kind()):
		template = m.items
	}
	return fmt.Sprintf(template, fe.Param())
}

// fixedMessages are the tags whose message does not depend on the field.
var fixedMessages = map[string]string{
	"required":      "This field is required",
	"email":         "Invalid email",
	"alphanum":      "Should be alphanumeric",
	"ltefield":      "Should be less than or equal to the related field",
	"gtefield":      "Should be greater than or equal to the related field",
	"ltfield":       "Should be less than the related field",
	"gtfield":       "Should be greater than the related field",
	"uuid":          "Should be a valid UUID",
	"uuid3":         "Should be a valid UUID",
	"uuid4":         "Should be a valid UUID",
	"uuid5":         "Should be a valid UUID",
	"uuid_rfc4122":  "Should be a valid UUID",
	"uuid3_rfc4122": "Should be a valid UUID",
	"uuid4_rfc4122": "Should be a valid UUID",
	"uuid5_rfc4122": "Should be a valid UUID",
	"url":           "Should be a valid URL",
	"http_url":      "Should be a valid URL",
	"uri":           "Should be a valid URL",
}

func mapValidateMessage(fe validator.FieldError) string {
	messageMu.RLock()
	override := messageOverrides[fe.Tag()]
	messageMu.RUnlock()
	if override != nil {
		return override(fe.Param())
	}

	if m, ok := sizedMessages[fe.Tag()]; ok {
		return m.format(fe)
	}
	if m, ok := fixedMessages[fe.Tag()]; ok {
		return m
	}
	if fe.Tag() == "oneof" {
		return "Should be one of: " + strings.Join(strings.Fields(fe.Param()), ", ")
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
		field, ok := structField(cur, name)
		if !ok {
			return "", false
		}
		parts = appendJSONName(parts, field, tagName)
		if indexes != "" {
			parts[len(parts)-1] += "[" + indexes
		}
		if cur, ok = elemAfterIndexes(field.Type, indexes); !ok {
			return "", false
		}
	}
	return strings.Join(parts, "."), len(parts) > 0
}

func derefType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

// structField looks up name in t, which must be a struct or pointer to one.
func structField(t reflect.Type, name string) (reflect.StructField, bool) {
	t = derefType(t)
	if t.Kind() != reflect.Struct {
		return reflect.StructField{}, false
	}
	return t.FieldByName(name)
}

// appendJSONName adds the JSON name of field to parts; embedded structs are
// flattened in JSON, so they add nothing.
func appendJSONName(parts []string, field reflect.StructField, tagName string) []string {
	jsonName, _, _ := strings.Cut(field.Tag.Get(tagName), ",")
	switch {
	case jsonName == "" && field.Anonymous:
		return parts
	case jsonName == "" || jsonName == "-":
		return append(parts, field.Name)
	}
	return append(parts, jsonName)
}

// elemAfterIndexes steps t through each "[...]" index in indexes and returns
// the element type reached.
func elemAfterIndexes(t reflect.Type, indexes string) (reflect.Type, bool) {
	for indexes != "" {
		t = derefType(t)
		if !isCollection(t.Kind()) {
			return nil, false
		}
		t = t.Elem()
		_, indexes, _ = strings.Cut(indexes, "[")
	}
	return t, true
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

// ValidateError maps errors to details named by JSON path; decoder errors get fixed
// messages with no Go names. Log err itself.
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
