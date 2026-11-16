package utils

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/zazhedho/logus"
)

func WithRequestID(ctx context.Context, requestID uuid.UUID) context.Context {
	return logus.WithRequestID(ctx, requestID)
}

func GetRequestID(ctx context.Context) string {
	if ctx != nil {
		if id, ok := logus.RequestIDFromContext(ctx); ok {
			return id.String()
		}
		if getter, ok := ctx.(interface{ Get(string) (any, bool) }); ok {
			if raw, exists := getter.Get(CtxKeyId); exists && raw != nil {
				if s, ok := requestIDString(raw); ok {
					return s
				}
			}
		}
		if raw := ctx.Value(CtxKeyId); raw != nil { //nolint:staticcheck // plain string key kept for backward compatibility
			if s, ok := requestIDString(raw); ok {
				return s
			}
		}
	}

	return GenerateLogId(ctx).String()
}

func requestIDString(raw any) (string, bool) {
	switch v := raw.(type) {
	case uuid.UUID:
		return v.String(), true
	case string:
		return strings.TrimSpace(v), true
	}
	return "", false
}
