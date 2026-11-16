package utils

import (
	"maps"
	"strings"
)

func MergeMetadata(base, extra map[string]any) map[string]any {
	if len(base) == 0 && len(extra) == 0 {
		return nil
	}

	merged := make(map[string]any, len(base)+len(extra))
	maps.Copy(merged, base)
	maps.Copy(merged, extra)

	return merged
}

func RedactSensitivePayload(input any) any {
	normalized := NormalizePayload(input)
	return RedactSensitiveValue(normalized)
}

func RedactSensitiveValue(input any) any {
	switch v := input.(type) {
	case map[string]any:
		return redactSensitiveMap(v)
	case []any:
		return redactSensitiveSlice(v)
	default:
		return v
	}
}

func IsSensitiveKey(key string) bool {
	k := NormalizeKey(key)
	return strings.Contains(k, "password") ||
		strings.Contains(k, "token") ||
		strings.Contains(k, "secret") ||
		strings.Contains(k, "otp")
}

func redactSensitiveMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, val := range in {
		if IsSensitiveKey(k) {
			out[k] = "[REDACTED]"
			continue
		}

		out[k] = RedactSensitiveValue(val)
	}
	return out
}

func redactSensitiveSlice(values []any) []any {
	out := make([]any, 0, len(values))
	for _, val := range values {
		out = append(out, RedactSensitiveValue(val))
	}
	return out
}
