package utils

import (
	"errors"
	"fmt"
	"math"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	envErrMu sync.Mutex
	envErrs  = map[string]string{}
)

// EnvErrors reports every variable GetEnv could not parse, so a service can
// refuse to start on a typo instead of running on a silent default.
func EnvErrors() error {
	envErrMu.Lock()
	defer envErrMu.Unlock()
	if len(envErrs) == 0 {
		return nil
	}
	keys := make([]string, 0, len(envErrs))
	for k := range envErrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	errs := make([]error, 0, len(keys))
	for _, k := range keys {
		errs = append(errs, fmt.Errorf("%s must be a valid %s", k, envErrs[k]))
	}
	return errors.Join(errs...)
}

// ResetEnvErrors clears the recorded errors; meant for tests.
func ResetEnvErrors() {
	envErrMu.Lock()
	defer envErrMu.Unlock()
	clear(envErrs)
}

func recordEnvError(key, kind string) {
	envErrMu.Lock()
	defer envErrMu.Unlock()
	envErrs[key] = kind
}

// EnvValue lists the types GetEnv can read. Named types are handled by their
// underlying kind; time.Duration is recognized exactly.
type EnvValue interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~float32 | ~float64 | ~bool | ~string | ~[]string
}

var (
	durationType = reflect.TypeFor[time.Duration]()
	eodLocation  atomic.Pointer[time.Location]
)

// SetEODLocation sets the zone "eod" durations are computed in. nil restores
// the default, the process's local zone (time.Local).
func SetEODLocation(loc *time.Location) { eodLocation.Store(loc) }

func eodLoc() *time.Location {
	if loc := eodLocation.Load(); loc != nil {
		return loc
	}
	return time.Local
}

// GetEnv treats an unset or blank variable as absent and returns def. An
// unparsable value also returns def but is recorded for EnvErrors.
func GetEnv[T EnvValue](key string, def T) T {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return def
	}

	out := def
	rv := reflect.ValueOf(&out).Elem()
	kind, ok := parseEnv(rv, val)
	if ok {
		return out
	}
	recordEnvError(key, kind)
	return def
}

// parseEnv stores val into rv. When it cannot, it returns a description of
// the expected type for the error report.
func parseEnv(rv reflect.Value, val string) (string, bool) {
	typ := rv.Type()
	if typ == durationType {
		d, ok := parseDuration(val)
		if ok {
			rv.SetInt(int64(d))
		}
		return "duration such as 60, 30s, 1m, 1h or eod", ok
	}

	switch typ.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		parsed, err := strconv.ParseInt(val, 10, typ.Bits())
		if err == nil {
			rv.SetInt(parsed)
		}
		return "integer", err == nil
	case reflect.Float32, reflect.Float64:
		parsed, err := strconv.ParseFloat(val, typ.Bits())
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
			return "finite number", false
		}
		rv.SetFloat(parsed)
		return "", true
	case reflect.Bool:
		parsed, err := strconv.ParseBool(val)
		if err == nil {
			rv.SetBool(parsed)
		}
		return "boolean", err == nil
	case reflect.String:
		rv.SetString(val)
		return "", true
	case reflect.Slice: // ~[]string, the only slice EnvValue admits
		items := make([]string, 0)
		for _, item := range strings.Split(val, ",") {
			if item = strings.TrimSpace(item); item != "" {
				items = append(items, item)
			}
		}
		rv.Set(reflect.ValueOf(items).Convert(typ))
		return "", true
	}
	return typ.String(), false
}

// parseDuration accepts time.ParseDuration syntax ("10s", "1h"), bare seconds
// ("60") and "eod": the time remaining until 23:59:59 in the EOD location. When
// that moment has passed, "eod" is reported as not parsed so the default is used.
func parseDuration(val string) (time.Duration, bool) {
	if strings.EqualFold(val, "eod") {
		loc := eodLoc()
		now := time.Now().In(loc)
		end := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, loc)
		if end.After(now) {
			return end.Sub(now), true
		}
		return 0, false
	}
	if d, err := time.ParseDuration(val); err == nil {
		return d, true
	}
	if sec, err := strconv.ParseInt(val, 10, 64); err == nil {
		return time.Duration(sec) * time.Second, true
	}
	return 0, false
}

// DurationFromEnv returns the first positive duration found in the given
// environment variables, or fallback.
func DurationFromEnv(keys []string, fallback time.Duration) time.Duration {
	for _, key := range keys {
		value := GetEnv(key, time.Duration(0))
		if value > 0 {
			return value
		}
	}
	return fallback
}
