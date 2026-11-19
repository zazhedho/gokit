package middleware

import (
	"errors"
	"net"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"syscall"

	"github.com/gin-gonic/gin"
	"github.com/zazhedho/logus"

	"github.com/zazhedho/gokit/internal/response"
)

// Recovery logs a panic with its stack and answers 500 with only the log ID.
// Register it after RequestIDMiddleware so the IDs match.
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			if rec == http.ErrAbortHandler { //nolint:errorlint // net/http compares by identity
				panic(rec)
			}

			id := logus.RequestID(c) // read once; used for the log line and the response
			logCtx := logus.WithRequestID(c.Request.Context(), id)

			if isBrokenPipe(rec) {
				logus.LogContextf(logCtx, logus.LevelWarn, "RECOVERY; client connection broken: %v", rec)
				c.Abort()
				return
			}
			logus.LogContextf(logCtx, logus.LevelError, "RECOVERY; panic: %v; stack: %s", rec, debug.Stack())
			c.AbortWithStatusJSON(http.StatusInternalServerError, response.InternalServerError(id))
		}()
		c.Next()
	}
}

func isBrokenPipe(rec any) bool {
	err, ok := rec.(error)
	if !ok {
		return false
	}
	var opErr *net.OpError
	if !errors.As(err, &opErr) {
		return false
	}
	var sysErr *os.SyscallError
	if !errors.As(opErr.Err, &sysErr) {
		return false
	}
	msg := strings.ToLower(sysErr.Error())
	return errors.Is(sysErr.Err, syscall.EPIPE) || errors.Is(sysErr.Err, syscall.ECONNRESET) ||
		strings.Contains(msg, "broken pipe") || strings.Contains(msg, "connection reset by peer")
}
