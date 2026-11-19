package utils

import "github.com/google/uuid"

// CreateUUID returns a new UUIDv7 as a string, or a v4 if v7 generation fails.
func CreateUUID() string {
	if id, err := uuid.NewV7(); err == nil {
		return id.String()
	}
	return uuid.NewString()
}
