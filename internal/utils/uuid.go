package utils

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/zazhedho/logus"
)

func CreateUUID() string {
	var id string
	if uuid7, err := uuid.NewV7(); err == nil {
		id = uuid7.String()
	} else {
		id = uuid.NewString()
	}

	return id
}

func GenerateLogId(ctx context.Context) uuid.UUID {
	return logus.RequestID(ctx)
}

func NormalizeUUIDPointer(input string) *string {
	value := strings.TrimSpace(input)
	if value == "" {
		return nil
	}

	if _, err := uuid.Parse(value); err != nil {
		return nil
	}

	return &value
}
