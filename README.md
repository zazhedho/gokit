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
| `repository` | The minimal repository interface (`GenericRepository[T]`) that domain repositories expose. |
| `repository/generic` | A GORM-backed building block (`GenericRepository[T]` struct) that domain repositories embed or wrap. |
| `middlewares` | `CORS`, `SetContextId`, `ErrorHandler` (panic recovery), and Redis-backed `IPRateLimitMiddleware` / `EndpointRateLimitMiddleware`. |
| `handlercommon` | Handler helpers: `BindJSON` and `ValidateUUID`, which write standard 400 responses. |

## Repository packages

`repository` defines `GenericRepository[T]`, the minimal interface that domain repositories expose to their callers. Its `GetAll(ctx, params)` takes only `filter.BaseParams`.

`repository/generic` provides a GORM-backed `GenericRepository[T]` struct. It is a building block, not an implementation of the interface above: its `GetAll(ctx, params, opts QueryOptions)` takes an extra `QueryOptions` argument (base query, search, filter and order whitelists), so the struct does not satisfy the `repository` interface. Domain repositories embed or wrap it and supply their own `QueryOptions`, then expose the interface's `GetAll(ctx, params)` themselves.

The packages are named after their directories. Consumers migrating from the old package names (`interfacegeneric`, `repositorygeneric`) can keep their identifiers with an import alias:

```go
import (
	interfacegeneric "github.com/zazhedho/gokit/repository"
	repositorygeneric "github.com/zazhedho/gokit/repository/generic"
)
```

## Security defaults

These describe current behavior; configure around them as needed.

- Rate limiters key on `gin.Context.ClientIP()`. gin trusts all proxies by default, so `X-Forwarded-For` is honored from any client unless the application calls `engine.SetTrustedProxies(...)` (or sets `TrustedPlatform`). Without that, clients can evade or redirect limits.
- Rate limiters fail open: with a nil Redis client, a non-positive limit (or window), or a Redis error on the counter increment, requests pass through.
- `filter.GetBaseParams` accepts `limit=-1`, which the generic repository turns into "no limit". A list endpoint built on them returns all rows in one request unless the handler rejects it.
- `middlewares.CORS()` sends `Access-Control-Allow-Origin: *` and is not configurable. It allows the methods `GET, POST, PUT, DELETE` and the headers `Content-Type, Authorization, Accept, Access-Control-Allow-Origin, Cache-Control`, and answers `OPTIONS` with 204.
- `middlewares.SetContextId` trusts a client-supplied `X-Request-ID` when it parses as a UUID.

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

logus is a required dependency of gokit.
