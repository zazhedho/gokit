package utils

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

func TestJSONAndStringHelpers(t *testing.T) {
	if got := JSONEncode(map[string]string{"name": "Jane"}); !strings.Contains(got, "Jane") {
		t.Fatalf("unexpected JSON encoding: %q", got)
	}
	if NormalizePayload(struct {
		Name string `json:"name"`
	}{Name: "Jane"}) == nil {
		t.Fatal("expected normalized payload")
	}
	if CreateUUID() == "" {
		t.Fatal("expected uuid")
	}
	if got := JSONEncode(make(chan int)); got != "" {
		t.Fatalf("expected empty string for unsupported JSON value, got %q", got)
	}
	ch := make(chan int)
	if NormalizePayload(ch) != ch {
		t.Fatal("expected unsupported payload to be returned unchanged")
	}
}

func TestValidateErrorAndValidateUUID(t *testing.T) {
	type request struct {
		Email string `json:"email" validate:"required,email"`
	}
	validate := validator.New()
	err := validate.Struct(request{})
	got := ValidateError(err, reflect.TypeFor[request](), "json")
	if len(got) != 1 || got[0].Field != "email" || got[0].Message == "" {
		t.Fatalf("unexpected validation mapping: %+v", got)
	}

	got = ValidateError(errors.New("plain error"), reflect.TypeFor[request](), "json")
	if len(got) != 1 || got[0].Message != "plain error" {
		t.Fatalf("unexpected plain error mapping: %+v", got)
	}

	if _, err := ValidateUUID("not-a-uuid"); !errors.Is(err, ErrInvalidUUID) {
		t.Fatalf("expected invalid uuid error, got %v", err)
	}

	if _, err := ValidateUUID(""); !errors.Is(err, ErrMissingID) {
		t.Fatalf("expected missing id error, got %v", err)
	}

	id := uuid.NewString()
	gotID, err := ValidateUUID(id)
	if err != nil || gotID != id {
		t.Fatalf("expected valid uuid, id=%q err=%v", gotID, err)
	}
}

func TestValidateErrorMapsKnownTags(t *testing.T) {
	type request struct {
		Required string `json:"required" validate:"required"`
		Email    string `json:"email" validate:"email"`
		AlphaNum string `json:"alphanum" validate:"alphanum"`
		Min      string `json:"min" validate:"min=3"`
		Max      string `json:"max" validate:"max=2"`
		LTE      int    `json:"lte" validate:"lte=3"`
		GTE      int    `json:"gte" validate:"gte=3"`
		LTEField int    `json:"ltefield" validate:"ltefield=GTEField"`
		GTEField int    `json:"gtefield" validate:"gtefield=LTEField"`
		UUID     string `json:"uuid" validate:"uuid"`
	}
	validate := validator.New()
	err := validate.Struct(request{
		Email:    "bad",
		AlphaNum: "with space",
		Min:      "no",
		Max:      "too-long",
		LTE:      4,
		GTE:      2,
		LTEField: 10,
		GTEField: 1,
		UUID:     "bad",
	})
	got := ValidateError(err, reflect.TypeFor[request](), "json")
	messages := map[string]string{}
	for _, item := range got {
		messages[item.Field] = item.Message
	}

	want := map[string]string{
		"required": "This field is required",
		"email":    "Invalid email",
		"alphanum": "Should be alphanumeric",
		"min":      "Minimum 3",
		"max":      "Maximum 2",
		"lte":      "Should be less than 3",
		"gte":      "Should be greater than 3",
		"ltefield": "Should be less than GTEField",
		"gtefield": "Should be greater than LTEField",
		"uuid":     "Invalid value",
	}
	for field, message := range want {
		if messages[field] != message {
			t.Fatalf("expected %s=%q, got %q in %#v", field, message, messages[field], messages)
		}
	}
}
