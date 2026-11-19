package utils

import (
	"context"

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
