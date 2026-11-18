# gokit

Shared building blocks for Go HTTP services built on [gin](https://github.com/gin-gonic/gin) and [GORM](https://gorm.io): response envelopes, filters, a generic repository, base middlewares, and handler helpers.

Module path: `github.com/zazhedho/gokit`

## Relationship to logus and authkit

gokit depends on [logus](https://github.com/zazhedho/logus) for logging and request ids. It has no knowledge of authentication, roles, or permissions; those live in a separate module, authkit, which builds on gokit.

```
authkit -> gokit -> logus
```

The dependency direction is one way: gokit never imports authkit, and logus never imports gokit.

Request ids are owned by logus. `SetContextId` stores the id through logus, and `GenerateLogId` and `WithRequestID` delegate to `logus.RequestID` and `logus.WithRequestID`. The id read from a `*gin.Context`, from `c.Request.Context()`, and from the `X-Request-ID` response header is always the same value.

## Installation

```
go get github.com/zazhedho/gokit@v0.2.0
```

Everything is imported from one path:

```go
import "github.com/zazhedho/gokit"
```

The implementation lives under `internal/` and is not importable; only the `gokit` package is public API.

## API overview

| Group | Contents |
| --- | --- |
| Helpers (`gokit.go`) | Environment (`GetEnv`, `EnvErrors`, `ResetEnvErrors`, `DurationFromEnv`, `EnvKey`), UUID and request-id helpers (`CreateUUID`, `ValidateUUID`, `GenerateLogId`, `WithRequestID`, `GetRequestID`), string and pointer helpers, JSON helpers, validation error formatting (`ValidateError`), and redaction (`RedactSensitivePayload`, `IsSensitiveKey`). |
| HTTP (`http.go`) | Response envelope (`Response`, `ErrorResponse`, `InternalServerError`, `PaginationResponse`, ...), `Msg*` message constants, middlewares (`CORS`, `SetContextId`, `ErrorHandler`, `IPRateLimitMiddleware`, `EndpointRateLimitMiddleware`), and request helpers `BindJSON` and `ValidateUUIDParam`. |
| Data (`data.go`) | Query parsing and filter whitelisting (`BaseParams`, `GetBaseParams`, `WhitelistFilter`, `WhitelistStringFilter`), the GORM-backed `Repository[T]` (`NewRepository`, `QueryOptions`, `SearchFunc`, `BuildSearchFunc`), and the `CRUDRepository[T]` interface. |

`CRUDRepository[T]` is the minimal interface that domain repositories expose; its `GetAll(ctx, params)` takes only `BaseParams`. `Repository[T]` is a building block, not an implementation of that interface: its `GetAll(ctx, params, opts QueryOptions)` takes an extra `QueryOptions` argument (base query, search, filter and order whitelists), so domain repositories embed or wrap it and supply their own `QueryOptions`.

## Security defaults

These describe current behavior; configure around them as needed.

- Rate limiters key on `gin.Context.ClientIP()`. gin trusts all proxies by default, so `X-Forwarded-For` is honored from any client unless the application calls `engine.SetTrustedProxies(...)` (or sets `TrustedPlatform`). Without that, clients can evade or redirect limits.
- Rate limiters fail open: with a nil Redis client, a non-positive limit (or window), or a Redis error on the counter increment, requests pass through.
- `GetBaseParams` accepts `limit=-1`, which the generic repository turns into "no limit". A list endpoint built on them returns all rows in one request unless the handler rejects it.
- `CORS()` sends `Access-Control-Allow-Origin: *` and is not configurable. It allows the methods `GET, POST, PUT, DELETE` and the headers `Content-Type, Authorization, Accept, Access-Control-Allow-Origin, Cache-Control`, and answers `OPTIONS` with 204.
- `SetContextId` trusts a client-supplied `X-Request-ID` when it parses as a UUID.

## Minimal gin wiring

```go
package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/zazhedho/gokit"
)

type createItemRequest struct {
	Name string `json:"name" binding:"required"`
}

func main() {
	r := gin.New()
	r.Use(gokit.SetContextId())
	r.Use(gin.CustomRecovery(gokit.ErrorHandler))
	r.Use(gokit.CORS())

	r.POST("/items", func(c *gin.Context) {
		logID := gokit.GenerateLogId(c)

		var req createItemRequest
		if !gokit.BindJSON(c, logID, "[Items][Create]", &req) {
			return
		}

		c.JSON(http.StatusCreated, gokit.Response(http.StatusCreated, gokit.MsgSuccess, logID, req))
	})

	_ = r.Run(":8080")
}
```

`SetContextId` must come first so that the recovery handler and every later middleware share the same request id. The id is taken from a valid `X-Request-ID` request header, or generated otherwise, and echoed back in the response header.

## Upgrading from v0.1.x

v0.2.0 is a breaking release: the eight sub-packages are gone and everything is imported from `github.com/zazhedho/gokit`. Every `pkg.Name` becomes `gokit.Name`, except for these renames:

| Old import and name | New name |
| --- | --- |
| `repository.GenericRepository` (interface) | `gokit.CRUDRepository` |
| `repository/generic.GenericRepository` (struct) | `gokit.Repository` |
| `repository/generic.New` | `gokit.NewRepository` |
| `handlercommon.ValidateUUID` (gin variant) | `gokit.ValidateUUIDParam` |
| `messages.InvalidRequest` | `gokit.MsgInvalidRequest` |
| `messages.NotFound` | `gokit.MsgResourceNotFound` |
| `messages.AccessDenied` | `gokit.MsgAccessDenied` |

`utils.ValidateUUID` (string variant) keeps its name as `gokit.ValidateUUID`. Other `repository/generic` names (`QueryOptions`, `SearchFunc`, `QueryFunc`, `FilterSanitizer`, `BuildSearchFunc`) keep their names.

Removed from gokit because they are auth-specific and move to another module: `messages.MsgCredential`, `messages.InvalidCred`, and `messages.ErrHashPassword`.

## Development

```
go build ./... && go vet ./... && go test -race -count=1 ./...
make lint
```

logus is a required dependency of gokit.

## Reading configuration

`GetEnv` is the single way to read an environment variable:

```go
timeout := gokit.GetEnv("HTTP_READ_TIMEOUT", 30*time.Second) // 60, 30s, 1m, 1h, "eod"
conns := gokit.GetEnv("DB_MAX_OPEN_CONNS", 100)
mode := gokit.GetEnv("DB_SSLMODE", "disable")

if err := gokit.EnvErrors(); err != nil {
	return fmt.Errorf("invalid configuration: %w", err) // refuse to start on a typo
}
```

- An unset or blank variable returns the default.
- An unparsable value also returns the default, and `EnvErrors` reports it by name. The value itself is never included.
- Tests that set invalid values can call `ResetEnvErrors`.
