// Package binding binds and validates request input and writes the error
// envelope on failure.
package binding

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zazhedho/logus"

	"github.com/zazhedho/gokit/internal/messages"
	"github.com/zazhedho/gokit/internal/response"
	"github.com/zazhedho/gokit/internal/utils"
)

// DefaultMaxBodyBytes is the request body limit BindJSON applies: 1 MiB.
const DefaultMaxBodyBytes int64 = 1 << 20

// Option configures BindJSON.
type Option func(*options)

type options struct{ maxBodyBytes int64 }

// WithMaxBodyBytes sets the largest request body BindJSON reads. Values below 1
// keep the default (DefaultMaxBodyBytes).
func WithMaxBodyBytes(n int64) Option {
	return func(o *options) {
		if n > 0 {
			o.maxBodyBytes = n
		}
	}
}

// BindJSON caps the body with http.MaxBytesReader, binds and validates it, and on
// failure logs the full error and aborts with a 400 (413 when too large).
func BindJSON[T any](ctx *gin.Context, logID uuid.UUID, logPrefix string, req *T, opts ...Option) bool {
	cfg := options{maxBodyBytes: DefaultMaxBodyBytes}
	for _, opt := range opts {
		opt(&cfg)
	}
	if ctx.Request.Body != nil {
		ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, cfg.maxBodyBytes)
	}

	err := ctx.ShouldBindJSON(req)
	if err == nil {
		return true
	}
	logus.LogContextf(ctx, logus.LevelError, "%s; BindJSON ERROR: %v;", logPrefix, err)

	if _, tooLarge := errors.AsType[*http.MaxBytesError](err); tooLarge {
		res := response.ErrorResponse(http.StatusRequestEntityTooLarge, messages.MsgRequestTooLarge, logID,
			fmt.Sprintf("Request body must not exceed %d bytes", cfg.maxBodyBytes))
		ctx.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, res)
		return false
	}

	res := response.Response(http.StatusBadRequest, messages.MsgInvalidRequest, logID, nil)
	res.Error = utils.ValidateError(err, reflect.TypeFor[T](), "json")
	ctx.AbortWithStatusJSON(http.StatusBadRequest, res)
	return false
}
