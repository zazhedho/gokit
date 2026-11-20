package utils

import (
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/zazhedho/gokit/internal/response"
)

type address struct {
	Street string `json:"street,omitempty" validate:"required"`
}

type item struct {
	Name string `json:"name" validate:"required"`
}

type embedded struct {
	Code string `json:"code" validate:"required"`
}

type nestedRequest struct {
	embedded
	Address  address         `json:"address"`
	Home     *address        `json:"home"`
	Items    []item          `json:"items" validate:"dive"`
	ByKey    map[string]item `json:"by_key" validate:"dive"`
	NoTag    string          `validate:"required"`
	Ignored  string          `json:"-" validate:"required"`
	Optional string          `json:"optional,omitempty" validate:"required"`
}

func fieldMessages(t *testing.T, err error, typ reflect.Type) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, d := range ValidateError(err, typ, "json") {
		out[d.Field] = d.Code + ": " + d.Message
	}
	return out
}

func TestValidateErrorUsesNestedJSONPaths(t *testing.T) {
	req := nestedRequest{
		Home:  &address{},
		Items: []item{{Name: "ok"}, {}},
		ByKey: map[string]item{"k": {}},
	}
	err := validator.New().Struct(req)
	got := fieldMessages(t, err, reflect.TypeOf(req))
	for _, want := range []string{"code", "address.street", "home.street", "items[1].name", "by_key[k].name", "NoTag", "Ignored", "optional"} {
		if _, ok := got[want]; !ok {
			t.Errorf("missing field %q in %v", want, got)
		}
	}
	for field := range got {
		if strings.Contains(field, ",") || strings.Contains(field, "nestedRequest") {
			t.Errorf("field %q leaks tag options or Go type names", field)
		}
	}
}

// requireGuardedDetails checks every detail is complete and, unless the type
// resolves (name "pointer"), that no Go field name leaks into Field.
func requireGuardedDetails(t *testing.T, name string, details []response.ErrorDetail) {
	t.Helper()
	for _, d := range details {
		if d.Message == "" || d.Code == "" {
			t.Fatalf("%s: incomplete detail %+v", name, d)
		}
		if name != "pointer" && d.Field != "" {
			t.Fatalf("%s: an unresolvable path must not fall back to a Go field name, got %q", name, d.Field)
		}
	}
}

func TestValidateErrorGuardsTypes(t *testing.T) {
	err := validator.New().Struct(nestedRequest{})
	for name, typ := range map[string]reflect.Type{
		"nil":     nil,
		"pointer": reflect.TypeOf(&nestedRequest{}),
		"int":     reflect.TypeOf(0),
		"slice":   reflect.TypeOf([]nestedRequest{}),
	} {
		got := ValidateError(err, typ, "json")
		if len(got) == 0 {
			t.Fatalf("%s: expected messages", name)
		}
		requireGuardedDetails(t, name, got)
	}
	if got := fieldMessages(t, err, reflect.TypeOf(&nestedRequest{})); got["address.street"] == "" {
		t.Fatalf("pointer type must be dereferenced, got %v", got)
	}
}

func TestValidateErrorMapsDecoderErrorsWithoutGoNames(t *testing.T) {
	type body struct {
		Age     int `json:"age"`
		Profile struct {
			Active bool `json:"active"`
		} `json:"profile"`
	}
	typeErr := json.Unmarshal([]byte(`{"age":"x"}`), &body{})
	nested := json.Unmarshal([]byte(`{"profile":{"active":"yes"}}`), &body{})
	syntax := json.Unmarshal([]byte(`{"age":`), &body{})
	syntax2 := json.Unmarshal([]byte(`{bad}`), &body{})

	for name, tc := range map[string]struct {
		err        error
		field, msg string
	}{
		"type":   {typeErr, "age", "number"},
		"nested": {nested, "profile.active", "boolean"},
		"syntax": {syntax2, "", "JSON"},
		"eof":    {io.EOF, "", "required"},
		"short":  {syntax, "", "JSON"},
		"other":  {errors.New("internal struct body.Age exploded"), "", "Invalid request"},
	} {
		got := ValidateError(tc.err, reflect.TypeFor[body](), "json")
		if len(got) != 1 || got[0].Field != tc.field || !strings.Contains(got[0].Message, tc.msg) {
			t.Errorf("%s: got %+v", name, got)
			continue
		}
		for _, leak := range []string{"body.", "Go struct", "exploded", "int", "unmarshal"} {
			if strings.Contains(got[0].Message, leak) {
				t.Errorf("%s: message leaks %q: %s", name, leak, got[0].Message)
			}
		}
	}
}

func TestValidateErrorMessagesPerTag(t *testing.T) {
	type request struct {
		Required string   `json:"required" validate:"required"`
		Email    string   `json:"email" validate:"email"`
		AlphaNum string   `json:"alphanum" validate:"alphanum"`
		MinStr   string   `json:"min_str" validate:"min=3"`
		MaxStr   string   `json:"max_str" validate:"max=2"`
		MinNum   int      `json:"min_num" validate:"min=3"`
		MinList  []string `json:"min_list" validate:"min=2"`
		LTE      int      `json:"lte" validate:"lte=3"`
		GTE      int      `json:"gte" validate:"gte=3"`
		LT       int      `json:"lt" validate:"lt=3"`
		GT       int      `json:"gt" validate:"gt=3"`
		Len      string   `json:"len" validate:"len=4"`
		OneOf    string   `json:"oneof" validate:"oneof=red green"`
		UUID     string   `json:"uuid" validate:"uuid"`
		URL      string   `json:"url" validate:"url"`
		LTEField int      `json:"ltefield" validate:"ltefield=GTEField"`
		GTEField int      `json:"gtefield" validate:"gtefield=LTEField"`
		Other    string   `json:"other" validate:"startswith=x"`
	}
	err := validator.New().Struct(request{
		Email: "bad", AlphaNum: "a b", MinStr: "no", MaxStr: "long", MinNum: 1, MinList: []string{"a"},
		LTE: 4, GTE: 2, LT: 3, GT: 3, Len: "abc", OneOf: "blue", UUID: "bad", URL: "bad", LTEField: 10, GTEField: 1, Other: "y",
	})
	got := fieldMessages(t, err, reflect.TypeFor[request]())
	want := map[string]string{
		"required": "required: This field is required",
		"email":    "email: Invalid email",
		"alphanum": "alphanum: Should be alphanumeric",
		"min_str":  "min: Minimum length is 3",
		"max_str":  "max: Maximum length is 2",
		"min_num":  "min: Minimum value is 3",
		"min_list": "min: Minimum 2 items",
		"lte":      "lte: Should be less than or equal to 3",
		"gte":      "gte: Should be greater than or equal to 3",
		"lt":       "lt: Should be less than 3",
		"gt":       "gt: Should be greater than 3",
		"len":      "len: Length should be exactly 4",
		"oneof":    "oneof: Should be one of: red, green",
		"uuid":     "uuid: Should be a valid UUID",
		"url":      "url: Should be a valid URL",
		"ltefield": "ltefield: Should be less than or equal to the related field",
		"gtefield": "gtefield: Should be greater than or equal to the related field",
		"other":    "startswith: Invalid value",
	}
	for field, msg := range want {
		if got[field] != msg {
			t.Errorf("%s: got %q, want %q", field, got[field], msg)
		}
	}
}

func TestSetValidationMessageOverrides(t *testing.T) {
	type request struct {
		Name string `json:"name" validate:"min=3"`
	}
	t.Cleanup(func() { SetValidationMessage("min", nil) })
	SetValidationMessage("min", func(param string) string { return "too short, need " + param })
	err := validator.New().Struct(request{Name: "a"})
	got := ValidateError(err, reflect.TypeFor[request](), "json")
	if got[0].Message != "too short, need 3" {
		t.Fatalf("override ignored: %+v", got)
	}
	SetValidationMessage("min", nil)
	got = ValidateError(err, reflect.TypeFor[request](), "json")
	if !strings.HasPrefix(got[0].Message, "Minimum length") {
		t.Fatalf("override not removed: %+v", got)
	}
}

func TestValidateUUIDReturnsCanonicalForm(t *testing.T) {
	id := uuid.New()
	for _, in := range []string{
		id.String(),
		strings.ToUpper(id.String()),
		strings.ReplaceAll(id.String(), "-", ""),
		"{" + id.String() + "}",
		"urn:uuid:" + strings.ToUpper(id.String()),
	} {
		got, err := ValidateUUID(in)
		if err != nil || got != id.String() {
			t.Errorf("ValidateUUID(%q) = %q, %v; want %q", in, got, err, id)
		}
	}
	if _, err := ValidateUUID(""); !errors.Is(err, ErrMissingID) {
		t.Fatalf("empty: %v", err)
	}
	if _, err := ValidateUUID("not-a-uuid"); !errors.Is(err, ErrInvalidUUID) {
		t.Fatalf("invalid: %v", err)
	}
}
