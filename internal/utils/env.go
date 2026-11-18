package utils

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
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

// GetEnv treats an unset or blank variable as absent and returns def. An
// unparsable value also returns def but is recorded for EnvErrors.
func GetEnv[T any](key string, def T) T {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return def
	}

	switch any(def).(type) {
	case int:
		if parsed, err := strconv.Atoi(val); err == nil {
			return any(parsed).(T)
		}

	case int32:
		if parsed, err := strconv.ParseInt(val, 10, 32); err == nil {
			return any(int32(parsed)).(T)
		}

	case int64:
		if parsed, err := strconv.ParseInt(val, 10, 64); err == nil {
			return any(parsed).(T)
		}

	case float32:
		if parsed, err := strconv.ParseFloat(val, 32); err == nil {
			return any(float32(parsed)).(T)
		}

	case float64:
		if parsed, err := strconv.ParseFloat(val, 64); err == nil {
			return any(parsed).(T)
		}

	case bool:
		if parsed, err := strconv.ParseBool(val); err == nil {
			return any(parsed).(T)
		}

	case string:
		return any(val).(T)

	case time.Duration:
		// Accepts time.ParseDuration syntax ("10s", "1h"), bare seconds ("60"),
		// and "eod": the time remaining until 23:59:59 in WIB (UTC+7).
		if strings.EqualFold(val, "eod") {
			loc := time.FixedZone("WIB", 7*3600)
			now := time.Now().In(loc)
			end := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, loc)
			if end.After(now) {
				return any(end.Sub(now)).(T)
			}
			return def
		}

		if d, err := time.ParseDuration(val); err == nil {
			return any(d).(T)
		}
		if sec, err := strconv.ParseInt(val, 10, 64); err == nil {
			return any(time.Duration(sec) * time.Second).(T)
		}
		recordEnvError(key, "duration such as 60, 30s, 1m or 1h")
		return def
	}

	recordEnvError(key, fmt.Sprintf("%T", def))
	return def
}

func DurationFromEnv(keys []string, fallback time.Duration) time.Duration {
	for _, key := range keys {
		value := GetEnv(key, time.Duration(0))
		if value > 0 {
			return value
		}
	}
	return fallback
}
