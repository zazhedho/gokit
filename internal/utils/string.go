package utils

import "strings"

// NormalizeKey lower-cases and trims a key.
func NormalizeKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// NormalizeUpperKey upper-cases and trims a key.
func NormalizeUpperKey(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}
