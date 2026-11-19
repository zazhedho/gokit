package filter

import (
	"encoding/json"
	"errors"
	"math"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestGetBaseParamsAppliesDefaultsAndParsesFilters(t *testing.T) {
	query, err := url.ParseQuery("page=2&limit=25&order_by=name&order_direction=DESC&filters[role]=\"admin\"&filters[ids]=[\"1\",\"2\"]")
	if err != nil {
		t.Fatal(err)
	}

	got, err := GetBaseParams(query, "created_at", "desc", 10)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	if got.Page != 2 || got.Limit != 25 || got.Offset != 25 {
		t.Fatalf("unexpected pagination: %+v", got)
	}
	if got.OrderBy != "name" || got.OrderDirection != "DESC" {
		t.Fatalf("unexpected ordering: %+v", got)
	}
	if got.Filters["role"] != "admin" {
		t.Fatalf("expected role filter, got %#v", got.Filters["role"])
	}
	ids, ok := got.Filters["ids"].([]any)
	if !ok || len(ids) != 2 {
		t.Fatalf("expected parsed ids filter, got %#v", got.Filters["ids"])
	}
}

func TestGetBaseParamsDefaultsInvalidValues(t *testing.T) {
	query, err := url.ParseQuery("page=0&limit=0&order_direction=sideways")
	if err != nil {
		t.Fatal(err)
	}

	got, err := GetBaseParams(query, "created_at", "desc", 50)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	if got.Page != 1 || got.Limit != 50 || got.Offset != 0 {
		t.Fatalf("unexpected pagination defaults: %+v", got)
	}
	if got.OrderBy != "created_at" || got.OrderDirection != "desc" {
		t.Fatalf("unexpected order defaults: %+v", got)
	}
}

func TestGetBaseParamsLeavesCappingToTheRepository(t *testing.T) {
	got, err := GetBaseParams(url.Values{"limit": {"20000"}, "page": {"3"}}, "id", "asc", 10)
	if err != nil {
		t.Fatal(err)
	}
	if got.Limit != 20000 {
		t.Fatalf("a large limit must not be reset to the default, got %d", got.Limit)
	}
}

func TestGetBaseParamsKeepsMinusOneForTheRepository(t *testing.T) {
	got, err := GetBaseParams(url.Values{"limit": {"-1"}, "page": {"4"}}, "id", "asc", 10)
	if err != nil {
		t.Fatal(err)
	}
	if got.Limit != -1 || got.Page != 1 || got.Offset != 0 {
		t.Fatalf("limit=-1 passes through with page 1: %+v", got)
	}
}

func TestGetBaseParamsRejectsBadDefaultLimit(t *testing.T) {
	for _, def := range []int{0, -1, -50} {
		if _, err := GetBaseParams(url.Values{}, "id", "asc", def); err == nil {
			t.Fatalf("defLimit %d must be rejected", def)
		}
	}
}

func TestGetBaseParamsGuardsOffsetOverflow(t *testing.T) {
	got, err := GetBaseParams(url.Values{"page": {strconv.Itoa(math.MaxInt)}, "limit": {"100"}}, "id", "asc", 10)
	if err != nil {
		t.Fatal(err)
	}
	if got.Offset < 0 {
		t.Fatalf("offset overflowed to %d", got.Offset)
	}
	got, err = GetBaseParams(url.Values{"page": {"9223372036854775807"}, "limit": {"9223372036854775807"}}, "id", "asc", 10)
	if err != nil || got.Offset < 0 || got.Page < 1 {
		t.Fatalf("overflow must saturate: %+v, %v", got, err)
	}
}

func filtersFor(t *testing.T, raw string) (map[string]any, error) {
	t.Helper()
	q := url.Values{}
	q.Set("filters[x]", raw)
	got, err := GetBaseParams(q, "id", "asc", 10)
	return got.Filters, err
}

func TestFilterValuesDecodeScalarsAndKeepNumbersVerbatim(t *testing.T) {
	cases := map[string]any{
		`true`:                   true,
		`"quoted"`:               "quoted",
		`plain text`:             "plain text",
		`0123`:                   "0123",
		`null`:                   nil,
		`12345678901234567890`:   json.Number("12345678901234567890"),
		`1.50`:                   json.Number("1.50"),
		`1 2`:                    "1 2",
		`[1,"a",true]`:           []any{json.Number("1"), "a", true},
		`[12345678901234567890]`: []any{json.Number("12345678901234567890")},
	}
	for raw, want := range cases {
		got, err := filtersFor(t, raw)
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if !reflect.DeepEqual(got["x"], want) {
			t.Errorf("%s: got %#v (%T), want %#v", raw, got["x"], got["x"], want)
		}
	}
}

func TestFilterValuesRejectObjectsNestedAndOversizedArrays(t *testing.T) {
	big := "[" + strings.Repeat("1,", maxFilterItems) + "1]" // 101 items
	for name, raw := range map[string]string{
		"object":        `{"$gt":1}`,
		"nested object": `[{"a":1}]`,
		"nested array":  `[[1]]`,
		"too many":      big,
	} {
		_, err := filtersFor(t, raw)
		if !errors.Is(err, ErrInvalidFilter) {
			t.Errorf("%s: got %v, want ErrInvalidFilter", name, err)
			continue
		}
		if strings.Contains(err.Error(), "$gt") || strings.Contains(err.Error(), "filters[x]") {
			t.Errorf("%s: error echoes input: %v", name, err)
		}
	}
	ok := "[" + strings.Repeat("1,", maxFilterItems-1) + "1]" // exactly 100
	if _, err := filtersFor(t, ok); err != nil {
		t.Fatalf("100 items must be accepted: %v", err)
	}
}

func TestGetBaseParamsRejectsInvalidInteger(t *testing.T) {
	query := url.Values{"page": {"invalid"}}
	if _, err := GetBaseParams(query, "created_at", "desc", 10); err == nil {
		t.Fatal("expected invalid page to return an error")
	}
}

func TestWhitelistFilterKeepsAllowedKeysOnly(t *testing.T) {
	got := WhitelistFilter(map[string]any{
		"role":  "admin",
		"email": "admin@example.com",
	}, []string{"role"})

	want := map[string]any{"role": "admin"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestWhitelistStringFilterConvertsValues(t *testing.T) {
	got := WhitelistStringFilter(map[string]any{
		"id":    10,
		"roles": []string{"admin", "viewer"},
		"skip":  true,
	}, []string{"id", "roles"})

	if got["id"] != "10" {
		t.Fatalf("expected string id, got %#v", got["id"])
	}
	if got["roles"] != "admin,viewer" {
		t.Fatalf("expected joined roles, got %#v", got["roles"])
	}
	if _, ok := got["skip"]; ok {
		t.Fatal("expected disallowed key to be removed")
	}
}
