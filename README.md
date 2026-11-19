# gokit

Shared HTTP and data plumbing for [gin](https://github.com/gin-gonic/gin) + [GORM](https://gorm.io) services: a JSON response envelope, request binding, list-query parsing, a generic repository, base middleware, and configuration helpers.

## Features

- **Responses**: one JSON envelope for success, error and paginated replies, with a log ID on every response.
- **Binding**: `BindJSON` and `ValidateUUIDParam` that write a 400 envelope on failure.
- **List queries**: `GetBaseParams` parses search, filters, paging and ordering from the query string; filters are allow-listed.
- **Repository**: a generic GORM `Repository[T]` with CRUD, upsert, lookups by field and a filtered, ordered, paginated `GetAll`.
- **Middleware**: request ID, panic recovery, CORS, and Redis-backed per-IP and per-endpoint rate limiting.
- **Configuration**: typed `GetEnv` with duration parsing and collected parse errors.
- **Helpers**: UUIDs, string and pointer helpers, JSON helpers, secret redaction.

## Install

```sh
go get github.com/zazhedho/gokit
```

Requires Go 1.27 or newer (the version in `go.mod`). Import the root package only:

```go
import "github.com/zazhedho/gokit"
```

Request IDs and log output go through [github.com/zazhedho/logus](https://github.com/zazhedho/logus), so the ID in the `X-Request-ID` header, the response `log_id` and your log lines is the same value.

## Quick start

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
	r.Use(gokit.SetContextId())                  // first: everything after shares the request ID
	r.Use(gin.CustomRecovery(gokit.ErrorHandler)) // panics become a 500 envelope
	r.Use(gokit.CORS())

	r.POST("/items", func(c *gin.Context) {
		logID := gokit.GenerateLogId(c)

		var req createItemRequest
		if !gokit.BindJSON(c, logID, "[Items][Create]", &req) {
			return // a 400 envelope has already been written
		}

		c.JSON(http.StatusCreated, gokit.Response(http.StatusCreated, gokit.MsgSuccess, logID, req))
	})

	_ = r.Run(":8080")
}
```

`POST /items` with `{"name":"book"}` returns 201:

```json
{"log_id":"0192...","code":201,"status":true,"message":"Success","data":{"name":"book"}}
```

## Usage

### Responses and errors

| Function | Result |
| --- | --- |
| `Response(code, msg, logID, data)` | Success for 2xx (`message` = `msg`, `data`). For any other code, `message` is the HTTP status text and `error` is `{code, message: msg}`. |
| `ErrorResponse(code, msg, logID, publicError)` | Error envelope whose `error.message` is `publicError`. |
| `Unauthorized(logID, publicError)` / `Forbidden(logID, publicError)` | 401 / 403 envelopes. |
| `InternalServerError(logID)` | 500 envelope; the error message tells the caller to quote the log ID. |
| `PaginationResponse(code, total, page, perPage, logID, data)` | `*PaginatedResponse` with `total_data`, `total_pages`, `current_page`, `next_page`, `prev_page`, `limit`. Message is `MsgNotFound` when `total` is 0 or `page` is past the last page. |

Predefined messages: `MsgSuccess`, `MsgInvalidRequest`, `MsgSomethingWrong`, `MsgInternal` (format string taking the log ID), `MsgExists`, `MsgNotFound`, `MsgResourceNotFound`, `MsgAccessDenied`. The envelope types are `ApiResponse` and `PaginatedResponse`; `Success`, `Error` and `Pagination` are same-shaped types, and `Errors` is the `{code, message}` detail.

```go
func notFound(c *gin.Context) {
	logID := gokit.GenerateLogId(c)
	c.JSON(http.StatusNotFound, gokit.ErrorResponse(http.StatusNotFound, gokit.MsgNotFound, logID, "item does not exist"))
}
```

### Request binding and validation

| Function | Behavior |
| --- | --- |
| `BindJSON[T](c, logID, logPrefix, &req) bool` | Binds the JSON body and validates `binding:` tags. On failure logs, writes a 400 envelope whose `error` is a list of `{field, message}`, and returns `false`. Field names come from the `json` tag. |
| `ValidateUUIDParam(c, logID) (string, error)` | Validates the `:id` route parameter and returns it unchanged; on failure writes a 400 envelope. |
| `ValidateUUID(id) (string, error)` | The same check on a plain string; returns `ErrMissingID` or `ErrInvalidUUID`. |
| `ValidateError(err, reflect.Type, tagName) []ValidateMessage` | Converts validator errors to field messages, for validating without `BindJSON`. |

```go
r.GET("/items/:id", func(c *gin.Context) {
	logID := gokit.GenerateLogId(c)
	id, err := gokit.ValidateUUIDParam(c, logID)
	if err != nil {
		return // 400 already written
	}
	c.JSON(http.StatusOK, gokit.Response(http.StatusOK, gokit.MsgSuccess, logID, gin.H{"id": id}))
})
```

### List queries

`GetBaseParams(query, defOrderBy, defOrderDirection, defLimit)` parses `c.Request.URL.Query()` into `BaseParams`:

| Query parameter | Field | Notes |
| --- | --- | --- |
| `search` | `Search` | Free text. |
| `filters[name]=value` | `Filters` | Value is decoded as JSON when valid (`filters[ids]=[1,2]`, `filters[active]=true`), otherwise kept as a string. |
| `order_by`, `order_direction` | `OrderBy`, `OrderDirection` | Empty `order_by` falls back to `defOrderBy`; a direction other than `asc`/`desc` falls back to `defOrderDirection`. |
| `page`, `limit` | `Page`, `Limit`, `Offset` | `page` below 1 becomes 1. `limit` outside 1..10000 becomes `defLimit`. `Offset = (page-1) * limit`. |
| `columns` | `Columns` | Repeated values, passed through. |

A non-numeric `page` or `limit` returns an error; respond with 400.

`limit=-1` means "no limit": `Page` is forced to 1, `Offset` to 0, and `Limit` stays `-1`, which the repository passes to GORM as no `LIMIT`. See [Security defaults](#security-defaults).

Filters are untrusted input. Reduce them to the keys you support before use:

| Function | Behavior |
| --- | --- |
| `WhitelistFilter(filters, allowed)` | Keeps only allowed keys. |
| `WhitelistStringFilter(filters, allowed)` | Same, and converts each value to a string (`nil` becomes `""`, `[]string` is comma-joined). |
| `ConvertValuesToString(filters, keys...)` | Converts the values of the given keys to strings. |

```go
func listParams(c *gin.Context) (gokit.BaseParams, error) {
	params, err := gokit.GetBaseParams(c.Request.URL.Query(), "created_at", "desc", 10)
	if err != nil {
		return params, err
	}
	params.Filters = gokit.WhitelistFilter(params.Filters, []string{"status", "category_id"})
	return params, nil
}
```

### Generic repository

`NewRepository[T](db)` returns a `*Repository[T]` over a GORM model `T`. `T` needs an `id` column for the by-ID methods.

| Method | Behavior |
| --- | --- |
| `Store(ctx, m)` | Insert. |
| `Update(ctx, m)` | Save (full update by primary key). |
| `Upsert(ctx, values, conflictColumns, updateColumns)` | Insert with `ON CONFLICT` update. Both column lists are required. |
| `GetByID(ctx, id)` | First row where `id = ?`; `gorm.ErrRecordNotFound` when absent. |
| `GetOneByField(ctx, field, value)` / `GetManyByField(...)` | One or all rows where `field = ?`. |
| `ExistsByField(ctx, field, value)` / `ExistsByFields(ctx, map)` | Existence checks; the map is AND-ed. |
| `Delete(ctx, id)` | Delete by ID (a soft delete if the model has `gorm.DeletedAt`). |
| `SoftDelete(ctx, id, deletedBy)` | Sets `deleted_by` and `deleted_at` columns. |
| `GetAll(ctx, params, opts)` | Returns `([]T, total int64, error)`: base query, search, filters, count, ordering, then offset and limit. |

Field, column and order names are validated as identifiers (letters, digits, `_`, optional `table.column`); anything else returns an error or is ignored, so they cannot carry SQL.

`GetAll` is driven by `QueryOptions`:

| Field | Purpose |
| --- | --- |
| `BaseQuery` (`QueryFunc`) | Joins, scopes or fixed conditions applied first. |
| `Search` (`SearchFunc`) | Applied when `params.Search` is non-empty. `BuildSearchFunc("name", "email")` does a case-insensitive `LIKE` across those columns. |
| `AllowedFilters` | Filter keys that become `column = ?` (or `IN ?` for slices). When empty, filters are ignored. |
| `FilterSanitizer` | Replaces the default `WhitelistFilter`. |
| `AllowedOrderColumns` | Columns a client may order by. An `order_by` outside this list makes `GetAll` return an error. |
| `DefaultOrders` | Used when no `order_by` and direction are set; each entry is `"column asc"` or `"column desc"`. |

Embed `Repository[T]` in your own repository to inherit the methods and add domain queries through the exported `DB` field:

```go
type Product struct {
	ID       string `gorm:"primaryKey"`
	Name     string
	Category string
}

type ProductRepository struct {
	*gokit.Repository[Product]
}

func NewProductRepository(db *gorm.DB) *ProductRepository {
	return &ProductRepository{Repository: gokit.NewRepository[Product](db)}
}

var productQuery = gokit.QueryOptions{
	Search:              gokit.BuildSearchFunc("name", "category"),
	AllowedFilters:      []string{"category"},
	AllowedOrderColumns: []string{"name", "created_at"},
	DefaultOrders:       []string{"created_at desc"},
}

func (r *ProductRepository) List(ctx context.Context, params gokit.BaseParams) ([]Product, int64, error) {
	return r.GetAll(ctx, params, productQuery)
}

func (r *ProductRepository) Cheapest(ctx context.Context) (Product, error) {
	var p Product
	err := r.DB.WithContext(ctx).Order("price asc").First(&p).Error
	return p, err
}
```

`CRUDRepository[T]` is the interface `Repository[T]` satisfies (`Store`, `GetByID`, `GetAll`, `Update`, `Delete`, `SoftDelete`). Depend on it in services so tests can substitute a fake:

```go
type ProductService struct {
	repo gokit.CRUDRepository[Product]
}

func NewProductService(db *gorm.DB) *ProductService {
	return &ProductService{repo: gokit.NewRepository[Product](db)}
}
```

### Middleware

| Middleware | Behavior |
| --- | --- |
| `SetContextId()` | Uses a valid UUID in the `X-Request-ID` request header, else generates one. Stores it in the gin and request contexts and echoes it in the response header. Register it first. |
| `ErrorHandler` | A `gin.CustomRecovery` handler: logs the panic and writes a 500 envelope with the log ID. |
| `CORS()` | Allows any origin; answers `OPTIONS` with 204. Not configurable. |
| `IPRateLimitMiddleware(rdb, prefix, limit, window)` | At most `limit` requests per client IP per `window`. Key: `rate_limit:<prefix>:<ip>`. |
| `EndpointRateLimitMiddleware(rdb, limit, window)` | Same, counted per route pattern and client IP. |

Rate limiters take a `*redis.Client` from `github.com/redis/go-redis/v9`. Over the limit they respond 429 with a `Retry-After` header. A nil client or a non-positive `limit` or `window` disables limiting.

```go
func setup(rdb *redis.Client) *gin.Engine {
	r := gin.New()
	r.Use(gokit.SetContextId(), gin.CustomRecovery(gokit.ErrorHandler), gokit.CORS())
	r.Use(gokit.IPRateLimitMiddleware(rdb, "global", 100, time.Minute))
	r.POST("/login", gokit.EndpointRateLimitMiddleware(rdb, 5, time.Minute), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	return r
}
```

Request ID helpers: `GenerateLogId(ctx)` (the ID for a gin or standard context), `GetRequestID(ctx)` (as a string), `WithRequestID(ctx, id)` (attach an ID to a context outside gin), and the context key `CtxKeyId`.

### Configuration

`GetEnv[T](key, default)` reads an environment variable as `T`: `int`, `int32`, `int64`, `float32`, `float64`, `bool`, `string` or `time.Duration`.

```go
timeout := gokit.GetEnv("HTTP_READ_TIMEOUT", 30*time.Second)
conns := gokit.GetEnv("DB_MAX_OPEN_CONNS", 100)
mode := gokit.GetEnv("DB_SSLMODE", "disable")

if err := gokit.EnvErrors(); err != nil {
	log.Fatalf("invalid configuration: %v", err) // refuse to start on a typo
}
```

- An unset or blank variable returns the default.
- An unparsable value returns the default and is recorded; `EnvErrors()` reports every such variable by name, and never includes the value.
- Durations accept `60` (seconds), `30s`, `1m`, `1h` (any `time.ParseDuration` form) and `eod`, the time remaining until 23:59:59 in UTC+7 (the default is returned if that moment has passed).
- `ResetEnvErrors()` clears recorded errors; use it in tests.
- `DurationFromEnv(keys, fallback)` returns the first positive duration among several variables. `EnvKey(s)` upper-cases `s` and replaces `-`, `.` and spaces with `_`.

### Helpers

| Area | Functions |
| --- | --- |
| UUID | `CreateUUID()` (UUIDv7 string), `ValidateUUID`, `NormalizeUUIDPointer` (trimmed `*string` or nil) |
| Strings | `FirstNonEmptyString`, `StringOrDefault`, `StringPtrIfNotEmpty`, `HumanizeKey`, `TitleCase`, `TitleHumanized`, `NormalizeKey`, `NormalizeUpperKey`, `StripHTML`, `SanitizeEmail`, `NormalizePhoneTo62` |
| Pointers and conversion | `Int64PtrFromString`, `Int64PtrIfPositive`, `InterfaceString`, `InterfaceBool` |
| JSON | `JsonEncode`, `MustJSON`, `EmptyJSON`, `NormalizePayload`, `MergeMetadata` |
| Redaction | `RedactSensitivePayload`, `RedactSensitiveValue`, `IsSensitiveKey` |

Redaction masks the values of any key containing `password`, `token`, `secret` or `otp` (case-insensitive) with `[REDACTED]`, recursively through maps and slices. Use it before logging request bodies:

```go
func logSafe(payload any) string {
	return gokit.JsonEncode(gokit.RedactSensitivePayload(payload))
}
```

## Security defaults

These describe current behavior. Configure around them as needed.

- **Trusted proxies.** Rate limiters key on `gin.Context.ClientIP()`. gin trusts all proxies by default, so `X-Forwarded-For` is honored from any client. Call `engine.SetTrustedProxies(...)` (or set `TrustedPlatform`); otherwise clients can evade or redirect limits.
- **Rate limiters fail open.** With a nil Redis client, a non-positive limit or window, or a Redis error on the counter, requests pass through (errors are logged).
- **`limit=-1` returns every row.** `GetBaseParams` accepts it and the repository turns it into "no limit". A list endpoint returns all rows in one request unless the handler rejects it, for example `if params.Limit == -1 { ... }`.
- **Page size cap.** Other `limit` values above 10000 are replaced by your default.
- **CORS is open.** `CORS()` sends `Access-Control-Allow-Origin: *`, allows methods `GET, POST, PUT, DELETE` and headers `Content-Type, Authorization, Accept, Access-Control-Allow-Origin, Cache-Control`, and answers `OPTIONS` with 204. It cannot be restricted; use another middleware if you need an origin allow-list.
- **Client-supplied request IDs are trusted.** `SetContextId` accepts an `X-Request-ID` that parses as a UUID, so a client can choose the ID that appears in your logs and responses. Do not treat it as unique or secret.
- **Identifiers are validated, values are bound.** Repository column, field and order names must be plain identifiers, and values are passed as query parameters. Filters and ordering are still only as safe as the allow-lists you set in `QueryOptions`: an empty `AllowedFilters` ignores filters, and `AllowedOrderColumns` must list every sortable column.
- **Errors.** The 500 envelope exposes only the log ID, not the panic or error text.

## Versioning

gokit follows semantic versioning and is pre-1.0 (currently v0.4.0): a minor version bump (v0.4 to v0.5) may change the API, and a patch release (v0.4.0 to v0.4.1) never does. Pin a minor version and read release notes before upgrading.

## Development

```sh
make test   # go test -race ./...
make lint   # golangci-lint (install with: make lint-install)
```

- The root `*.go` files are the entire public API: thin wrappers and type aliases.
- `internal/` holds the implementation (`binding`, `filter`, `messages`, `middleware`, `repository`, `response`, `utils`) and is not importable. Add new behavior there and export it from the root.
- Dependabot opens weekly update PRs for Go modules and GitHub Actions, grouped per ecosystem.

## License

[MIT](LICENSE)
