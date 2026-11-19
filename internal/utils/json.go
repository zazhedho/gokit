package utils

import (
	"encoding/json"
)

// JSONEncode marshals data to a JSON string, returning "" on failure.
func JSONEncode(data any) string {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return ""
	}
	return string(jsonData)
}

func NormalizePayload(input any) any {
	if input == nil {
		return map[string]any{}
	}

	raw, err := json.Marshal(input)
	if err != nil {
		return input
	}

	var normalized any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return input
	}

	return normalized
}
