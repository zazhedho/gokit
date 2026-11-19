package utils

import (
	"errors"
	"reflect"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/zazhedho/gokit/internal/response"
)

func mapValidateMessage(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "This field is required"
	case "email":
		return "Invalid email"
	case "alphanum":
		return "Should be alphanumeric"
	case "min":
		return "Minimum " + fe.Param()
	case "max":
		return "Maximum " + fe.Param()
	case "lte":
		return "Should be less than " + fe.Param()
	case "gte":
		return "Should be greater than " + fe.Param()
	case "ltefield":
		return "Should be less than " + fe.Param()
	case "gtefield":
		return "Should be greater than " + fe.Param()
	}

	return "Invalid value"
}

func ValidateError(err error, reflectType reflect.Type, tagName string) []response.ErrorDetail {
	if ve, ok := errors.AsType[validator.ValidationErrors](err); ok {
		out := make([]response.ErrorDetail, len(ve))
		for i, fe := range ve {
			field := fe.Field()
			if structField, ok := reflectType.FieldByName(fe.Field()); ok {
				field = structField.Tag.Get(tagName)
			}
			out[i] = response.ErrorDetail{Field: field, Code: fe.Tag(), Message: mapValidateMessage(fe)}
		}
		return out
	}
	return []response.ErrorDetail{{Code: "invalid_request", Message: err.Error()}}
}

var (
	ErrMissingID   = errors.New("missing ID")
	ErrInvalidUUID = errors.New("invalid UUID")
)

func ValidateUUID(id string) (string, error) {
	if id == "" {
		return "", ErrMissingID
	}

	if _, err := uuid.Parse(id); err != nil {
		return "", ErrInvalidUUID
	}

	return id, nil
}
