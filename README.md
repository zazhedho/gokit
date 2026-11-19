# gokit

Shared HTTP and data plumbing for [gin](https://github.com/gin-gonic/gin) + [GORM](https://gorm.io) services: a JSON response envelope, request binding, list-query parsing, a generic repository, base middleware, and configuration helpers.

## Features

- **Responses**: one JSON envelope for success, error and paginated replies. Every response carries a `log_id`, and every error is a list of `{field, code, message}`.
- **Binding**: `BindJSON` (size-limited, no Go internals in errors) and `ValidateUUIDParam`.
- **List queries**: `GetBaseParams` parses search, filters, paging and ordering; filters, order columns and page size are allow-listed and capped.
- **Repository**: a generic GORM `Repository[T]` with CRUD, upsert, lookups, existence checks and a filtered, ordered, paginated `GetAll`.
- **Middleware**: request ID, panic recovery, CORS, and Redis-backed per-caller and per-endpoint rate limiting.
- **Configuration**: typed `GetEnv` with duration parsing and collected parse errors.
- **Helpers**: UUIDs, value-to-string conversion, JSON encoding, secret redaction.

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
	r.Use(gokit.RequestIDMiddleware()) // first: everything after shares the request ID
	r.Use(gokit.Recovery())            // panics become a 500 envelope
	r.Use(gokit.CORS(gokit.CORSOptions{AllowOrigins: []string{"https://app.example.com"}}))

	r.POST("/items", func(c *gin.Context) {
		logID := gokit.RequestID(c)

		var req createItemRequest
		if !gokit.BindJSON(c, logID, "[Items][Create]", &req) {
			return // an error envelope has already been written
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

With `{}` it returns 400:

```json
{"log_id":"0192...","code":400,"status":false,"message":"Invalid request format. Please ensure the structure is correct and matches the expected data format.","error":[{"field":"name","code":"required","message":"This field is required"}]}
```

## Usage

### Responses and errors

| Function | Result |
| --- | --- |
| `Response(code, msg, logID, data)` | For 2xx: `message` is `msg` and `data` the payload. For any other code: `message` is `msg` (the status text when empty) and `error` holds one detail with that message. |
| `ErrorResponse(code, msg, logID, publicError)` | Error envelope: `message` is `msg`, and the single `error` entry carries `publicError`. |
| `Unauthorized(logID, publicError)` / `Forbidden(logID, publicError)` | 401 / 403 envelopes. |
| `InternalServerError(logID)` | 500 envelope whose error tells the caller to quote the log ID. |
| `PaginationResponse(code, total, page, perPage, logID, data)` | `*PaginatedResponse` with `total_data` (`int64`), `total_pages`, `current_page`, `next_page`, `prev_page`, `limit`. An empty page, or a page past the last, keeps `status: true` and carries `MsgNotFound`. |

Both envelope types are `APIResponse` and `PaginatedResponse`. `log_id` is always a UUID string. The `error` field is always a list of `ErrorDetail`:

```json
{"error":[{"field":"email","code":"email","message":"Invalid email"}]}
```

- `field` is present when the error belongs to a request field.
- `code` is a machine code: the validator tag (`required`, `min`), or one derived from the status (`not_found`, `too_many_requests`).
- `message` is safe to show to callers.

A nil slice passed as `data` is sent as `[]`, never `null`; in a paginated response `data` is always present.

Predefined messages: `MsgSuccess`, `MsgInvalidRequest`, `MsgSomethingWrong`, `MsgInternal` (format string taking the log ID), `MsgExists`, `MsgNotFound`, `MsgResourceNotFound`, `MsgRequestTooLarge`, `MsgAccessDenied`.

```go
func notFound(c *gin.Context) {
	logID := gokit.RequestID(c)
	c.JSON(http.StatusNotFound, gokit.ErrorResponse(http.StatusNotFound, gokit.MsgNotFound, logID, "item does not exist"))
}
```

### Request binding and validation

| Function | Behavior |
| --- | --- |
| `BindJSON[T](c, logID, logPrefix, &req, opts...) bool` | Reads at most 1 MiB (`WithMaxBodyBytes(n)` changes it), binds the JSON body and validates `binding:` tags. On failure it logs the full error with `logPrefix`, aborts with a 400 envelope (413 when the body is too large) and returns `false`. |
| `ValidateUUIDParam(c, logID) (string, error)` | Validates the `:id` route parameter and returns it in canonical form; on failure aborts with a 400 envelope. |
| `ValidateUUID(id) (string, error)` | The same check on a string. Returns the canonical lower-case hyphenated form for every spelling `uuid.Parse` accepts, or `ErrMissingID` / `ErrInvalidUUID`. |
| `ValidateError(err, reflect.Type, tagName) []ErrorDetail` | The conversion `BindJSON` uses, for validating without it. |
| `SetValidationMessage(tag, fn)` | Overrides the message for a validator tag; `fn` receives the tag parameter. `nil` removes the override. |

`ValidateError` names fields by the JSON path from the struct tag: options such as `,omitempty` are stripped and nested fields read `address.street` or `items[1].name`. JSON syntax and type errors become fixed messages (`Malformed JSON`, `Should be a number`) that never contain Go type or field names; the real error is only logged.

```go
type signup struct {
	Email string   `json:"email,omitempty" binding:"required,email"`
	Role  string   `json:"role" binding:"oneof=admin member"`
	Tags  []string `json:"tags" binding:"max=5"`
}

func signupHandler(c *gin.Context) {
	logID := gokit.RequestID(c)
	var req signup
	if !gokit.BindJSON(c, logID, "[Signup]", &req, gokit.WithMaxBodyBytes(64<<10)) {
		return
	}
	c.Status(http.StatusNoContent)
}

func init() {
	gokit.SetValidationMessage("oneof", func(param string) string { return "Pick one of: " + param })
}
```

```go
func getItem(c *gin.Context) {
	logID := gokit.RequestID(c)
	id, err := gokit.ValidateUUIDParam(c, logID)
	if err != nil {
		return // 400 already written
	}
	c.JSON(http.StatusOK, gokit.Response(http.StatusOK, gokit.MsgSuccess, logID, gin.H{"id": id}))
}
```

### List queries

`GetBaseParams(query, defOrderBy, defOrderDirection, defLimit)` parses `c.Request.URL.Query()` into `BaseParams`:

| Query parameter | Field | Notes |
| --- | --- | --- |
| `search` | `Search` | Free text. |
| `filters[name]=value` | `Filters` | See below. |
| `order_by`, `order_direction` | `OrderBy`, `OrderDirection` | Empty `order_by` falls back to `defOrderBy`; a direction other than `asc`/`desc` falls back to `defOrderDirection`. |
| `page`, `limit` | `Page`, `Limit`, `Offset` | `page` below 1 becomes 1. A `limit` below 1 becomes `defLimit`. A larger `limit` is left for the repository to clamp. `Offset = (page-1) * limit`, saturating instead of overflowing. |

- `defLimit` must be at least 1, otherwise `GetBaseParams` returns an error.
- A non-numeric `page` or `limit` returns an error; respond with 400.
- `limit=-1` ("every row") passes through with `Page` 1 and `Offset` 0; the repository honors it only with `QueryOptions.AllowAll`.

**Filter values** are decoded as JSON when valid and kept as strings otherwise:

- Scalars: `filters[active]=true`, `filters[role]="admin"` or plain text.
- Arrays of scalars, at most 100 items: `filters[ids]=[1,2]`.
- Numbers keep their written form as `json.Number`, so `12345678901234567890` is not rounded.
- Objects, nested arrays and longer arrays return an error wrapping `ErrInvalidFilter`; map it to 400.

Filters are untrusted input. Reduce them to the keys you support before use:

| Function | Behavior |
| --- | --- |
| `WhitelistFilter(filters, allowed)` | Keeps only allowed keys. |
| `WhitelistStringFilter(filters, allowed)` | Same, and converts each value to a string. |
| `ConvertValuesToString(filters, keys...)` | Converts the values of the given keys (all keys when none are given) to strings. |

```go
func listParams(c *gin.Context) (gokit.BaseParams, error) {
	params, err := gokit.GetBaseParams(c.Request.URL.Query(), "created_at", "desc", 10)
	if err != nil {
		return params, err // 400
	}
	params.Filters = gokit.WhitelistFilter(params.Filters, []string{"status", "category_id"})
	return params, nil
}
```

### Generic repository

`NewRepository[T](db)` returns a `*Repository[T]` over a GORM model `T`. Its key column is `id`; set the exported `KeyColumn` field to use another one.

| Method | Behavior |
| --- | --- |
| `Store(ctx, *T)` | Insert. Generated IDs and database defaults are written back into the value. |
| `Update(ctx, *T)` | Writes every column of the record, except the key, `created_at`, `deleted_at`, `deleted_by` and associations, to the row with the same key. Zero values are written. Returns `ErrNotFound` when no live row matched; it never inserts or revives a soft-deleted row. A zero key is an error. |
| `Upsert(ctx, values, conflictColumns, updateColumns)` | Insert with `ON CONFLICT` update, in batches of the DB's `CreateBatchSize` (500 when unset). Both column lists are required. |
| `GetByID(ctx, id)` | The row whose key equals `id`, or `ErrNotFound`. |
| `GetOneByField(ctx, field, value)` | The first row where `field = value` (`IS NULL` for `nil`), or `ErrNotFound`. |
| `GetManyByField(ctx, field, value, limit, order)` | Up to `limit` rows (clamped to 1..`DefaultMaxLimit`), ordered by `order` (`"name desc"`; empty means by key) and then by key. |
| `ExistsByField(ctx, field, value)` / `ExistsByFields(ctx, map)` | Existence checks with `LIMIT 1`; the map is AND-ed with sorted keys, and an empty map is an error wrapping `ErrInvalidFilter`. |
| `Delete(ctx, id)` | Deletes by key; `ErrNotFound` when nothing matched. It follows gorm: a model with a `gorm.DeletedAt` field is soft-deleted, any other model is deleted for real. |
| `SoftDelete(ctx, id, deletedBy)` | Sets `deleted_by` and `deleted_at` (UTC) on a live row; `ErrNotFound` when no live row matched, so a second call fails. The model needs both columns. |
| `GetAll(ctx, params, opts)` | `([]T, total int64, error)`: base query, search, filters, count, ordering, then offset and limit. |

Column, field and order names are validated as identifiers (ASCII letters, digits, `_`, optionally `table.column`) on the exact string, then quoted; values are always bound as parameters.

`GetAll` is driven by `QueryOptions`:

| Field | Purpose |
| --- | --- |
| `BaseQuery` (`QueryFunc`) | Joins, scopes or fixed conditions applied first. Its `WHERE` conditions are grouped, so an `Or()` inside cannot widen the rest of the query. |
| `Search` (`SearchFunc`) | Applied when `params.Search` is non-empty, grouped like `BaseQuery`. `BuildSearchFunc("name", "email")` does a case-insensitive substring match across those columns; `%`, `_` and `\` in the text match themselves. |
| `MaxSearchLength` | Search text longer than this many runes is truncated. Default 100. |
| `AllowedFilters` | Filter keys that become `column = ?` (or `IN` for arrays). When empty, filters are ignored. Blank strings and `nil` mean no filter. |
| `FilterSanitizer` | Replaces the default `WhitelistFilter`. Keys it returns outside `AllowedFilters` fail the query with `ErrInvalidQueryOptions`. |
| `AllowedOrderColumns` | Columns a client may order by. |
| `DefaultOrders` | Used when no `order_by` and direction are set; each entry is `"column asc"` or `"column desc"`. |
| `MaxLimit` | Page size cap. Default 100 (`DefaultMaxLimit`). |
| `AllowAll` | Honors `limit=-1` as "every row". Off by default. |

How `GetAll` pages:

- The limit is clamped to `1..MaxLimit`. `limit=-1` without `AllowAll` is clamped like any other out-of-range value.
- When `params.Page` is at least 1, the offset is derived from `Page` and the clamped limit. Otherwise `params.Offset` is used, never below 0.
- Rows are ordered by the requested or default order, then by the key column, so pages are stable.

`opts.Validate()` checks the options (valid identifiers, non-negative caps) and `GetAll` calls it on every query. A bad option returns an error wrapping `ErrInvalidQueryOptions`, as does `BuildSearchFunc` with no columns or an invalid one: misconfiguration is an error, never an unfiltered query.

Sentinel errors, for choosing the HTTP status:

| Error | Meaning | Status |
| --- | --- | --- |
| `ErrNotFound` | No row matched (it is `gorm.ErrRecordNotFound`). | 404 |
| `ErrInvalidOrderColumn`, `ErrInvalidOrderDirection` | The client asked for a disallowed order. | 400 |
| `ErrInvalidFilter` | An unacceptable filter value. | 400 |
| `ErrInvalidQueryOptions` | The server is misconfigured. | 500 |

The error text of the 400-class errors never echoes the client's input.

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
	MaxLimit:            50,
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

A list handler turns the sentinel errors into statuses:

```go
func listProducts(repo *ProductRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		logID := gokit.RequestID(c)
		params, err := gokit.GetBaseParams(c.Request.URL.Query(), "created_at", "desc", 20)
		if err != nil {
			c.JSON(http.StatusBadRequest, gokit.ErrorResponse(http.StatusBadRequest, gokit.MsgInvalidRequest, logID, "invalid paging or filter parameter"))
			return
		}

		items, total, err := repo.List(c.Request.Context(), params)
		switch {
		case errors.Is(err, gokit.ErrInvalidOrderColumn), errors.Is(err, gokit.ErrInvalidOrderDirection), errors.Is(err, gokit.ErrInvalidFilter):
			c.JSON(http.StatusBadRequest, gokit.ErrorResponse(http.StatusBadRequest, gokit.MsgInvalidRequest, logID, "unsupported ordering or filter"))
		case err != nil:
			c.JSON(http.StatusInternalServerError, gokit.InternalServerError(logID))
		default:
			perPage := min(max(params.Limit, 1), productQuery.MaxLimit) // what GetAll clamped to
			c.JSON(http.StatusOK, gokit.PaginationResponse(http.StatusOK, total, params.Page, perPage, logID, items))
		}
	}
}
```

`CRUDRepository[T]` is the interface `Repository[T]` satisfies (all the methods above). Depend on it in services so tests can substitute a fake:

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
| `RequestIDMiddleware()` | Uses a valid UUID in the `X-Request-ID` request header, else generates one. Stores it in the gin and request contexts and echoes it in the response header. Register it first. |
| `Recovery()` | Recovers panics: logs the panic value and stack with the request ID, then answers 500 with an envelope that carries only the log ID. Register it after `RequestIDMiddleware`. |
| `CORS(CORSOptions)` | Origin allow-list; see below. |
| `IPRateLimit(RateLimitOptions)` | At most `Limit` requests per `Window` per caller. Key: `rate_limit:<Prefix>:<caller>`. |
| `EndpointRateLimit(RateLimitOptions)` | The same, counted per route pattern and caller. |

**CORS.** `CORSOptions` fields:

| Field | Meaning |
| --- | --- |
| `AllowOrigins` | Origins allowed to call the API; `"*"` allows any. Empty allows none. |
| `Methods` | Default: `GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS`. |
| `Headers` | Allowed request headers. Default: `Content-Type, Authorization, Accept, Cache-Control, X-Request-ID`. |
| `Expose` | Readable response headers; `X-Request-ID` is always included. |
| `AllowCredentials` | Sends `Access-Control-Allow-Credentials: true`. Combining it with `"*"` panics at construction. |
| `MaxAge` | Preflight cache age. |

- Requests without an `Origin` header pass through untouched.
- Only a real preflight (`OPTIONS` with `Origin` and `Access-Control-Request-Method`) is answered, with 204 for an allowed origin and 403 otherwise. Any other `OPTIONS` request reaches your handlers.
- An allowed origin is echoed back (with `Vary: Origin`) on actual requests, along with the `Expose` headers.

**Rate limiting.** `RateLimitOptions` fields:

| Field | Meaning |
| --- | --- |
| `Redis` | Any `redis.Cmdable` from `github.com/redis/go-redis/v9`: a client, a cluster client, a ring. |
| `Prefix` | Key namespace. |
| `Limit`, `Window` | Requests allowed per window. A non-positive value disables limiting. |
| `KeyFunc` | Chooses the caller. Default: `ClientIP()` with IPv6 addresses masked to their `/64`. An empty result falls back to the default. |
| `FailClosed` | Answer 503 when Redis is missing or fails. Default: let the request through. |
| `Timeout` | Bound for each Redis call. Default 200ms (`DefaultRateLimitTimeout`). |

- The counter is one atomic Redis script (increment, set the TTL on the first hit, read the TTL), so a crash cannot leave a counter without an expiry.
- Over the limit the response is 429 with a `Retry-After` header rounded up to whole seconds.
- A Redis error is logged at most once every 10 seconds, however many requests fail.
- A nil client or a non-positive `Limit` or `Window` disables limiting, or answers 503 for a nil client with `FailClosed`.

```go
func setup(rdb redis.Cmdable) *gin.Engine {
	r := gin.New()
	_ = r.SetTrustedProxies([]string{"10.0.0.0/8"}) // see "Security notes"
	r.Use(gokit.RequestIDMiddleware(), gokit.Recovery())
	r.Use(gokit.CORS(gokit.CORSOptions{
		AllowOrigins:     []string{"https://app.example.com"},
		AllowCredentials: true,
		MaxAge:           10 * time.Minute,
	}))
	r.Use(gokit.IPRateLimit(gokit.RateLimitOptions{Redis: rdb, Prefix: "global", Limit: 100, Window: time.Minute}))
	r.POST("/login", gokit.EndpointRateLimit(gokit.RateLimitOptions{
		Redis:      rdb,
		Limit:      5,
		Window:     time.Minute,
		FailClosed: true, // refuse logins rather than run unthrottled
		KeyFunc:    func(c *gin.Context) string { return c.GetHeader("X-Api-Client") },
	}), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	return r
}
```

Request ID helpers: `RequestID(ctx)` returns the ID carried by a gin context, request context or plain context, or a new one when there is none; `WithRequestID(ctx, id)` attaches an ID to a context outside gin; `CtxKeyRequestID` is the gin context key.

### Configuration

`GetEnv[T](key, default)` reads an environment variable as `T`: any integer kind up to 64 bits, `float32`, `float64`, `bool`, `string`, `[]string` or `time.Duration`, including named types built on them.

```go
type Config struct {
	ReadTimeout time.Duration
	MaxConns    int
	SSLMode     string
	Origins     []string
}

func loadConfig() Config {
	cfg := Config{
		ReadTimeout: gokit.GetEnv("HTTP_READ_TIMEOUT", 30*time.Second),
		MaxConns:    gokit.GetEnv("DB_MAX_OPEN_CONNS", 100),
		SSLMode:     gokit.GetEnv("DB_SSLMODE", "disable"),
		Origins:     gokit.GetEnv("CORS_ORIGINS", []string{"https://app.example.com"}),
	}
	if err := gokit.EnvErrors(); err != nil {
		log.Fatalf("invalid configuration: %v", err) // refuse to start on a typo
	}
	return cfg
}
```

- An unset or blank variable returns the default.
- An unparsable value returns the default and is recorded; `EnvErrors()` reports every such variable by name, and never includes the value. Integer overflow, `NaN` and `Inf` count as unparsable.
- A `[]string` is comma-separated; items are trimmed and blanks dropped.
- Durations accept `60` (seconds), `30s`, `1m`, `1h` (any `time.ParseDuration` form) and `eod`, the time remaining until 23:59:59 today. `eod` uses the process's local time zone; call `SetEODLocation(loc)` to pick another (`nil` restores the default). If that moment has passed the default is returned.
- `DurationFromEnv(keys, fallback)` returns the first positive duration among several variables.
- `ResetEnvErrors()` clears recorded errors. It exists for tests only.

### Helpers

| Function | Behavior |
| --- | --- |
| `CreateUUID()` | A UUIDv7 string. |
| `ValidateUUID(id)` | See [Request binding](#request-binding-and-validation). |
| `InterfaceString(v)` | The single value-to-string conversion: `nil` and nil pointers give `""`, strings and bytes pass through, `fmt.Stringer` and `error` use their text, `[]string` is comma-joined, numbers keep their exact decimal form, anything else is JSON. `ConvertValuesToString` and `WhitelistStringFilter` use it too. |
| `JSONEncode(v)` | `json.Marshal` to a string; `""` on failure. |
| `RedactSensitivePayload(v, extraKeys...)` | A copy of `v` with sensitive values masked as `[REDACTED]`. |

`RedactSensitivePayload` matches keys case-insensitively with `_`, `-` and `.` ignored. The built-in list is `password`, `passwd`, `pwd`, `token`, `secret`, `otp`, `apikey`, `authorization`, `cookie`, `credential`, `privatekey`, `session`, `jwt`, `cvv` and `pin`. The short names (`pwd`, `otp`, `jwt`, `cvv`, `pin`) match whole words only, so `userOTP` is masked and `footprint` is not. `extraKeys` add more names.

It walks maps, slices, `http.Header`, `url.Values`, structs (by JSON field name) and pointers, and keeps large integers exact. A value that cannot be encoded comes back as `"[UNREDACTABLE]"` rather than raw. Use it before logging request bodies or headers:

```go
func logSafe(payload any, header http.Header) string {
	return gokit.JSONEncode(map[string]any{
		"body":    gokit.RedactSensitivePayload(payload, "ssn"),
		"headers": gokit.RedactSensitivePayload(header),
	})
}
```

## Security notes

- **Trusted proxies and the rate-limit key.** The default caller key is `gin.Context.ClientIP()`. gin trusts every proxy by default, so any client can send `X-Forwarded-For` and be counted as someone else, which evades the limit or burns another caller's budget. Call `engine.SetTrustedProxies(...)` with your real proxy ranges (or `nil` to ignore forwarding headers) before relying on the limiters, or supply a `KeyFunc` that identifies callers some other way, such as an authenticated user ID. IPv6 callers are keyed by their `/64`, so rotating addresses inside one allocation shares a budget.
- **Rate limiters fail open by default.** A Redis error, or a nil client, lets requests through (the error is logged, throttled). Set `FailClosed` on endpoints where running unthrottled is worse than refusing, such as login and password reset.
- **Page size is capped.** `GetAll` clamps `limit` to `MaxLimit` (default 100), and `limit=-1` returns every row only when `AllowAll` is set. Keep `AllowAll` off for endpoints reachable by untrusted clients.
- **Filters, orders and search are allow-listed.** An empty `AllowedFilters` ignores filters, and `AllowedOrderColumns` must list every sortable column. Search text is escaped for `LIKE` and capped in length.
- **Identifiers are validated, values are bound.** Column, field and order names must be plain identifiers and are quoted; values go through query parameters.
- **Misconfiguration is an error.** Invalid `QueryOptions`, a `BuildSearchFunc` without valid columns, or a sanitizer that returns a disallowed key fail the query instead of running it unfiltered.
- **Request bodies are bounded.** `BindJSON` reads at most 1 MiB unless you raise it. Apply `http.MaxBytesReader` yourself for handlers that read the body another way.
- **CORS is closed by default.** With no `AllowOrigins` no origin is allowed. List every origin explicitly; `"*"` cannot be combined with credentials.
- **Client-supplied request IDs are trusted.** `RequestIDMiddleware` accepts an `X-Request-ID` that parses as a UUID, so a client can choose the ID that appears in your logs and responses. Do not treat it as unique or secret.
- **Errors do not leak internals.** The 500 envelope exposes only the log ID. Binding errors use JSON field names and fixed messages, never Go type or field names or decoder text.

## Versioning

gokit follows semantic versioning and is pre-1.0: a minor version bump may change the API, and a patch release never does. Pin a minor version and read the release notes before upgrading.

## Development

```sh
make test   # go test -race -count=1 ./...
make vet    # go vet ./...
make cover  # tests with a coverage summary (coverage.out)
make lint   # golangci-lint (install with: make lint-install)
```

- The root `*.go` files are the entire public API: thin wrappers and type aliases.
- `internal/` holds the implementation (`binding`, `filter`, `messages`, `middleware`, `repository`, `response`, `utils`) and is not importable. Add new behavior there and export it from the root.
- CI runs lint, `go vet`, a `go mod tidy` check, the tests and `govulncheck`, with every action pinned by commit SHA. Dependabot opens weekly update PRs for Go modules and GitHub Actions, grouped per ecosystem.

## License

[MIT](LICENSE)
