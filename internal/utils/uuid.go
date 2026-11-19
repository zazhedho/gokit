package utils

import (
	"errors"

	"github.com/google/uuid"
)

// CreateUUID returns a new UUIDv7 as a string, or a v4 if v7 generation fails.
func CreateUUID() string {
	if id, err := uuid.NewV7(); err == nil {
		return id.String()
	}
	return uuid.NewString()
}

var (
	// ErrMissingID is returned by ValidateUUID for an empty ID.
	ErrMissingID = errors.New("missing ID")
	// ErrInvalidUUID is returned by ValidateUUID for an ID that is not a UUID.
	ErrInvalidUUID = errors.New("invalid UUID")
)

// ValidateUUID parses id and returns its canonical form (lower-case, hyphenated),
// so the hyphenless, braced, urn: and upper-case spellings that uuid.Parse
// accepts all compare equal afterwards. It returns ErrMissingID or ErrInvalidUUID.
func ValidateUUID(id string) (string, error) {
	if id == "" {
		return "", ErrMissingID
	}
	parsed, err := uuid.Parse(id)
	if err != nil {
		return "", ErrInvalidUUID
	}
	return parsed.String(), nil
}
