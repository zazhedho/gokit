package utils

import (
	"strings"
	"testing"
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
