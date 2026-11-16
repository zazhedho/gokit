# gokit

Shared building blocks for Go HTTP services built on [gin](https://github.com/gin-gonic/gin) and [GORM](https://gorm.io): response envelopes, filters, a generic repository, base middlewares, and handler helpers.

Module path: `github.com/zazhedho/gokit`

## Relationship to logus and authkit

gokit depends on [logus](https://github.com/zazhedho/logus) for logging and request ids. It has no knowledge of authentication, roles, or permissions; those live in a separate module, authkit, which builds on gokit.

```
authkit -> gokit -> logus
```

The dependency direction is one way: gokit never imports authkit, and logus never imports gokit.

Request ids are owned by logus. `middlewares.SetContextId` stores the id through logus, and `utils.GenerateLogId` and `utils.WithRequestID` delegate to `logus.RequestID` and `logus.WithRequestID`. The id read from a `*gin.Context`, from `c.Request.Context()`, and from the `X-Request-ID` response header is always the same value.

## Packages

| Package | Purpose |
| --- | --- |
| `utils` | Environment helpers, UUID and request-id helpers, validation error formatting, redaction, and small string/JSON helpers. |
| `messages` | Shared response message constants. |
| `response` | API response envelope types and constructors (`Response`, `ErrorResponse`, `InternalServerError`, `PaginationResponse`, ...). |
| `filter` | Query-string parsing into pagination/sort parameters and filter whitelisting. |
| `repository` | The generic repository contract. |
| `repository/generic` | A GORM implementation of the repository contract. |
| `middlewares` | `CORS`, `SetContextId`, `ErrorHandler` (panic recovery), and Redis-backed `IPRateLimitMiddleware` / `EndpointRateLimitMiddleware`. |
| `handlercommon` | Handler helpers: `BindJSON` and `ValidateUUID`, which write standard 400 responses. |

## Minimal gin wiring

```go
package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/zazhedho/gokit/handlercommon"
	"github.com/zazhedho/gokit/messages"
	"github.com/zazhedho/gokit/middlewares"
	"github.com/zazhedho/gokit/response"
	"github.com/zazhedho/gokit/utils"
)

type createItemRequest struct {
	Name string `json:"name" binding:"required"`
}

func main() {
	r := gin.New()
	r.Use(middlewares.SetContextId())
	r.Use(gin.CustomRecovery(middlewares.ErrorHandler))
	r.Use(middlewares.CORS())

	r.POST("/items", func(c *gin.Context) {
		logID := utils.GenerateLogId(c)

		var req createItemRequest
		if !handlercommon.BindJSON(c, logID, "[Items][Create]", &req) {
			return
		}

		c.JSON(http.StatusCreated, response.Response(http.StatusCreated, messages.MsgSuccess, logID, req))
	})

	_ = r.Run(":8080")
}
```

`SetContextId` must come first so that the recovery handler and every later middleware share the same request id. The id is taken from a valid `X-Request-ID` request header, or generated otherwise, and echoed back in the response header.

## Development

```
go build ./... && go vet ./... && go test -race -count=1 ./...
make lint
```

`go.mod` uses a local `replace` for logus (`../logus`) while the modules are developed side by side.
