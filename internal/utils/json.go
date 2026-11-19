// Package utils holds the helpers behind the gokit facade: environment
// parsing, redaction, validation messages, UUIDs and value conversion.
package utils

import "encoding/json"

// JSONEncode marshals data to a JSON string, returning "" on failure.
func JSONEncode(data any) string {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return ""
	}
	return string(jsonData)
}
