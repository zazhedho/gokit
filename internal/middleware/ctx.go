package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zazhedho/logus"

	"github.com/zazhedho/gokit/internal/utils"
)

// RequestIDMiddleware gives each request an ID: a valid UUID in X-Request-ID,
// else a new one. The ID is stored under logus.CtxKeyRequestID, on the request
// context and in the X-Request-ID response header.
func RequestIDMiddleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var id uuid.UUID
		if header := ctx.GetHeader(requestIDHeader); header != "" {
			if parsed, err := uuid.Parse(header); err == nil {
				id = parsed
			}
		}

		if id == uuid.Nil {
			newID, err := uuid.NewV7()
			if err != nil {
				newID = uuid.New()
			}
			id = newID
		}

		ctx.Set(logus.CtxKeyRequestID, id)
		ctx.Request = ctx.Request.WithContext(utils.WithRequestID(ctx.Request.Context(), id))
		ctx.Writer.Header().Set(requestIDHeader, id.String())
		ctx.Next()
	}
}
