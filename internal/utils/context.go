package utils

import (
	"context"

	"github.com/google/uuid"
	"github.com/zazhedho/logus"
)

// WithRequestID returns a context carrying requestID.
func WithRequestID(ctx context.Context, requestID uuid.UUID) context.Context {
	return logus.WithRequestID(ctx, requestID)
}

// RequestID returns the request ID carried by ctx, or a newly generated one
// when it carries none.
func RequestID(ctx context.Context) uuid.UUID {
	return logus.RequestID(ctx)
}
