package utils

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

type logLevel string
type retries int
type ratio float64
type flag bool

func TestGetEnvHandlesNamedTypesByKind(t *testing.T) {
	ResetEnvErrors()
	t.Cleanup(ResetEnvErrors)
	t.Setenv("T_LEVEL", "debug")
	t.Setenv("T_RETRIES", "5")
	t.Setenv("T_RATIO", "0.25")
	t.Setenv("T_FLAG", "true")

	if got := GetEnv("T_LEVEL", logLevel("info")); got != "debug" {
		t.Fatalf("named string = %q", got)
	}
	if got := GetEnv("T_RETRIES", retries(1)); got != 5 {
		t.Fatalf("named int = %d", got)
	}
	if got := GetEnv("T_RATIO", ratio(1)); got != 0.25 {
		t.Fatalf("named float = %v", got)
	}
	if got := GetEnv("T_FLAG", flag(false)); !got {
		t.Fatal("named bool = false")
	}
	if err := EnvErrors(); err != nil {
		t.Fatalf("unexpected errors: %v", err)
	}
}

func TestGetEnvStringSlice(t *testing.T) {
	ResetEnvErrors()
	t.Cleanup(ResetEnvErrors)
	t.Setenv("T_LIST", " a, b ,,c ")
	got := GetEnv("T_LIST", []string{"x"})
	if !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Fatalf("got %#v", got)
	}
	if got := GetEnv("T_LIST_UNSET", []string{"x"}); !reflect.DeepEqual(got, []string{"x"}) {
		t.Fatalf("unset must return the default, got %#v", got)
	}
}

func TestGetEnvRejectsNaNInfAndOverflow(t *testing.T) {
	ResetEnvErrors()
	t.Cleanup(ResetEnvErrors)
	for key, val := range map[string]string{"T_NAN": "NaN", "T_INF": "Inf", "T_NINF": "-Infinity"} {
		t.Setenv(key, val)
		if got := GetEnv(key, 1.5); got != 1.5 {
			t.Fatalf("%s: got %v, want the default", key, got)
		}
	}
	t.Setenv("T_BIG32", "3000000000")
	if got := GetEnv("T_BIG32", int32(9)); got != 9 {
		t.Fatalf("int32 overflow must fall back, got %d", got)
	}
	err := EnvErrors()
	for _, key := range []string{"T_NAN", "T_INF", "T_NINF", "T_BIG32"} {
		if err == nil || !strings.Contains(err.Error(), key) {
			t.Fatalf("EnvErrors must mention %s, got %v", key, err)
		}
	}
}

func TestEODUsesConfiguredLocation(t *testing.T) {
	t.Cleanup(func() { SetEODLocation(nil) })
	t.Setenv("T_EOD", "eod")

	for _, name := range []string{"Asia/Jakarta", "America/Los_Angeles", "UTC"} {
		loc, err := time.LoadLocation(name)
		if err != nil {
			t.Skipf("tzdata unavailable: %v", err)
		}
		SetEODLocation(loc)
		got := GetEnv("T_EOD", time.Duration(-1))
		now := time.Now().In(loc)
		end := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, loc)
		if got < 0 {
			continue // the day ended during the test
		}
		if diff := end.Sub(now) - got; diff < -2*time.Second || diff > 2*time.Second {
			t.Fatalf("%s: eod = %v, want about %v", name, got, end.Sub(now))
		}
	}
}

func TestToStringIsTheSingleConversion(t *testing.T) {
	big := json.Number("12345678901234567890")
	var nilPtr *json.Number
	for _, tc := range []struct {
		in   any
		want string
	}{
		{nil, ""},
		{"s", "s"},
		{[]byte("b"), "b"},
		{big, "12345678901234567890"},
		{nilPtr, ""},
		{errors.New("e"), "e"},
		{[]string{"a", "b"}, "a,b"},
		{true, "true"},
		{int64(1) << 60, "1152921504606846976"},
		{float64(1e21), "1000000000000000000000"},
		{1.5, "1.5"},
		{[]any{big, "x"}, `[12345678901234567890,"x"]`},
		{map[string]string{"a": "b"}, `{"a":"b"}`},
		{logLevel("debug"), "debug"},
	} {
		if got := ToString(tc.in); got != tc.want {
			t.Errorf("ToString(%#v) = %q, want %q", tc.in, got, tc.want)
		}
		if got := InterfaceString(tc.in); got != tc.want {
			t.Errorf("InterfaceString(%#v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

type shard uint16

func TestGetEnvUnsignedIntegers(t *testing.T) {
	ResetEnvErrors()
	t.Cleanup(ResetEnvErrors)
	t.Setenv("T_U", "7")
	t.Setenv("T_U8", "300")
	t.Setenv("T_NEG", "-1")
	t.Setenv("T_SHARD", " 9 ")
	if GetEnv("T_U", uint(1)) != 7 || GetEnv("T_SHARD", shard(1)) != 9 || GetEnv("T_U64", uint64(5)) != 5 {
		t.Fatal("unsigned parsing failed")
	}
	if GetEnv("T_U8", uint8(2)) != 2 || GetEnv("T_NEG", uint(3)) != 3 {
		t.Fatal("overflow and negatives must fall back")
	}
	err := EnvErrors()
	if err == nil || !strings.Contains(err.Error(), "T_U8") || !strings.Contains(err.Error(), "T_NEG") {
		t.Fatalf("got %v", err)
	}
}

func TestGetEnvRejectsNegativeDurations(t *testing.T) {
	ResetEnvErrors()
	t.Cleanup(ResetEnvErrors)
	for _, v := range []string{"-5", "-5s", "-1h"} {
		t.Setenv("T_NEGDUR", v)
		if got := GetEnv("T_NEGDUR", time.Minute); got != time.Minute {
			t.Fatalf("%s: got %v", v, got)
		}
	}
	if err := EnvErrors(); err == nil || !strings.Contains(err.Error(), "T_NEGDUR") {
		t.Fatalf("negative durations must be recorded: %v", err)
	}
	t.Setenv("T_ZERO", "0")
	if got := GetEnv("T_ZERO", time.Minute); got != 0 {
		t.Fatalf("zero is a valid duration, got %v", got)
	}
}

func TestEODAfterLastSecondReturnsNextDay(t *testing.T) {
	ResetEnvErrors()
	t.Cleanup(func() { ResetEnvErrors(); nowFunc = time.Now; SetEODLocation(nil) })
	loc := time.FixedZone("T", 3600)
	SetEODLocation(loc)
	t.Setenv("T_EOD2", "eod")

	nowFunc = func() time.Time { return time.Date(2026, 11, 19, 23, 59, 59, 500_000_000, loc) }
	got := GetEnv("T_EOD2", time.Duration(-1))
	if want := 24*time.Hour - 500*time.Millisecond; got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
	nowFunc = func() time.Time { return time.Date(2026, 11, 19, 12, 0, 0, 0, loc) }
	if got := GetEnv("T_EOD2", time.Duration(-1)); got != 12*time.Hour-time.Second {
		t.Fatalf("got %v", got)
	}
	if err := EnvErrors(); err != nil {
		t.Fatalf("eod must never be an env error: %v", err)
	}
}

func TestConcurrentSettersAndEnvErrors(t *testing.T) {
	t.Cleanup(func() { ResetEnvErrors(); SetEODLocation(nil); SetValidationMessage("min", nil) })
	t.Setenv("T_CONC_BAD", "x")
	t.Setenv("T_CONC_EOD", "eod")
	done := make(chan struct{})
	for i := range 8 {
		go func() {
			defer func() { done <- struct{}{} }()
			for range 200 {
				switch i % 4 {
				case 0:
					GetEnv("T_CONC_BAD", 1)
				case 1:
					SetEODLocation(time.UTC)
					GetEnv("T_CONC_EOD", time.Second)
				case 2:
					SetValidationMessage("min", func(p string) string { return p })
				default:
					_ = EnvErrors()
					ResetEnvErrors()
				}
			}
		}()
	}
	for range 8 {
		<-done
	}
}
