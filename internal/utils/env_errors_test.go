package utils

import (
	"strings"
	"testing"
	"time"
)

func TestGetEnvRecordsInvalidValues(t *testing.T) {
	ResetEnvErrors()
	t.Cleanup(ResetEnvErrors)
	t.Setenv("TEST_BAD_INT", "abc")
	t.Setenv("TEST_BAD_DURATION", "3o s")
	t.Setenv("TEST_BAD_BOOL", "maybe")

	if got := GetEnv("TEST_BAD_INT", 7); got != 7 {
		t.Fatalf("invalid int must fall back to the default, got %d", got)
	}
	GetEnv("TEST_BAD_DURATION", time.Second)
	GetEnv("TEST_BAD_BOOL", true)

	err := EnvErrors()
	if err == nil {
		t.Fatal("EnvErrors must report invalid values")
	}
	for _, want := range []string{"TEST_BAD_INT", "TEST_BAD_DURATION", "TEST_BAD_BOOL", "duration"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("EnvErrors() = %q, want it to mention %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "3o s") {
		t.Errorf("EnvErrors() must not echo raw values, got %q", err)
	}
}

func TestGetEnvRecordsEachKeyOnce(t *testing.T) {
	ResetEnvErrors()
	t.Cleanup(ResetEnvErrors)
	t.Setenv("TEST_BAD_TWICE", "abc")
	GetEnv("TEST_BAD_TWICE", 1)
	GetEnv("TEST_BAD_TWICE", 1)
	if n := strings.Count(EnvErrors().Error(), "TEST_BAD_TWICE"); n != 1 {
		t.Fatalf("key reported %d times, want 1", n)
	}
}

func TestGetEnvTreatsEmptyAsUnset(t *testing.T) {
	ResetEnvErrors()
	t.Cleanup(ResetEnvErrors)
	t.Setenv("TEST_EMPTY_STRING", "")
	t.Setenv("TEST_BLANK_INT", "   ")

	if got := GetEnv("TEST_EMPTY_STRING", "disable"); got != "disable" {
		t.Fatalf("empty value must use the default, got %q", got)
	}
	if got := GetEnv("TEST_BLANK_INT", 5); got != 5 {
		t.Fatalf("blank value must use the default, got %d", got)
	}
	if err := EnvErrors(); err != nil {
		t.Fatalf("empty values are not errors, got %v", err)
	}
}

func TestEnvErrorsNilWhenAllValid(t *testing.T) {
	ResetEnvErrors()
	t.Cleanup(ResetEnvErrors)
	t.Setenv("TEST_GOOD_DURATION", "60")
	if got := GetEnv("TEST_GOOD_DURATION", time.Second); got != time.Minute {
		t.Fatalf("bare seconds must parse, got %v", got)
	}
	if err := EnvErrors(); err != nil {
		t.Fatalf("EnvErrors() = %v, want nil", err)
	}
}
