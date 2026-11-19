package gokit_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/zazhedho/gokit"
)

type record struct {
	ID string `gorm:"column:id;primaryKey"`
}

func (record) TableName() string { return "records" }

// Compile-time proof that every facade identifier exists with the expected shape.
var (
	// helpers
	_ func(map[string]any, ...string) map[string]any        = gokit.ConvertValuesToString
	_ func() string                                         = gokit.CreateUUID
	_ func([]string, time.Duration) time.Duration           = gokit.DurationFromEnv
	_ func(context.Context) uuid.UUID                       = gokit.RequestID
	_ func(string, int) int                                 = gokit.GetEnv[int]
	_ func() error                                          = gokit.EnvErrors
	_ func()                                                = gokit.ResetEnvErrors
	_ func(any) string                                      = gokit.InterfaceString
	_ func(any) string                                      = gokit.JSONEncode
	_ func(any, ...string) any                              = gokit.RedactSensitivePayload
	_ func(string) (string, error)                          = gokit.ValidateUUID
	_ func(context.Context, uuid.UUID) context.Context      = gokit.WithRequestID
	_ func(error, reflect.Type, string) []gokit.ErrorDetail = gokit.ValidateError
	_ gokit.ErrorDetail
	_ error = gokit.ErrMissingID
	_ error = gokit.ErrInvalidUUID

	// http
	_ func(int, string, uuid.UUID, string) *gokit.APIResponse                  = gokit.ErrorResponse
	_ func(uuid.UUID, string) *gokit.APIResponse                               = gokit.Forbidden
	_ func(uuid.UUID) *gokit.APIResponse                                       = gokit.InternalServerError
	_ func(int, string, uuid.UUID, any) *gokit.APIResponse                     = gokit.Response
	_ func(uuid.UUID, string) *gokit.APIResponse                               = gokit.Unauthorized
	_ func(int, int64, int, int, uuid.UUID, any) *gokit.PaginatedResponse      = gokit.PaginationResponse
	_ func(gokit.CORSOptions) gin.HandlerFunc                                  = gokit.CORS
	_ func(gokit.RateLimitOptions) gin.HandlerFunc                             = gokit.EndpointRateLimit
	_ func() gin.HandlerFunc                                                   = gokit.Recovery
	_ func(gokit.RateLimitOptions) gin.HandlerFunc                             = gokit.IPRateLimit
	_ func() gin.HandlerFunc                                                   = gokit.RequestIDMiddleware
	_ func(*gin.Context, uuid.UUID, string, *record, ...gokit.BindOption) bool = gokit.BindJSON[record]
	_ func(*gin.Context, uuid.UUID) (string, error)                            = gokit.ValidateUUIDParam
	_ gokit.PaginatedResponse

	// data
	_ func(url.Values, string, string, int) (gokit.BaseParams, error) = gokit.GetBaseParams
	_ func(map[string]any, []string) map[string]any                   = gokit.WhitelistFilter
	_ func(map[string]any, []string) map[string]any                   = gokit.WhitelistStringFilter
	_ func(*gorm.DB) *gokit.Repository[record]                        = gokit.NewRepository[record]
	_ func(...string) gokit.SearchFunc                                = gokit.BuildSearchFunc
	_ gokit.QueryFunc
	_ gokit.FilterSanitizer
	_ gokit.QueryOptions
	_ gokit.CRUDRepository[record]
	_ func(gokit.QueryOptions) error                                                       = gokit.QueryOptions.Validate
	_ func(context.Context, *record) error                                                 = (*gokit.Repository[record])(nil).Store
	_ func(context.Context, *record) error                                                 = (*gokit.Repository[record])(nil).Update
	_ func(context.Context, string, any, int, string) ([]record, error)                    = (*gokit.Repository[record])(nil).GetManyByField
	_ func(context.Context, map[string]any) (bool, error)                                  = (*gokit.Repository[record])(nil).ExistsByFields
	_ func(context.Context, gokit.BaseParams, gokit.QueryOptions) ([]record, int64, error) = (*gokit.Repository[record])(nil).GetAll

	// errors
	_ error = gokit.ErrNotFound
	_ error = gokit.ErrInvalidOrderColumn
	_ error = gokit.ErrInvalidOrderDirection
	_ error = gokit.ErrInvalidFilter
	_ error = gokit.ErrInvalidQueryOptions

	// options and settings
	_ gokit.CORSOptions
	_ gokit.RateLimitOptions
	_ gokit.BindOption
	_ func(string, []string) []string   = gokit.GetEnv[[]string]
	_ func(string, func(string) string) = gokit.SetValidationMessage
	_ func(*time.Location)              = gokit.SetEODLocation
	_ func(int64) gokit.BindOption      = gokit.WithMaxBodyBytes
)

// Defaults keep their values.
const (
	_               = int(gokit.DefaultMaxLimit)
	_               = int(gokit.DefaultMaxSearchLength)
	_ time.Duration = gokit.DefaultRateLimitTimeout
)

// Constants keep their string values.
const (
	_ string = gokit.CtxKeyRequestID
	_ string = gokit.MsgInvalidRequest
	_ string = gokit.MsgSomethingWrong
	_ string = gokit.MsgInternal
	_ string = gokit.MsgExists
	_ string = gokit.MsgNotFound
	_ string = gokit.MsgResourceNotFound
	_ string = gokit.MsgSuccess
	_ string = gokit.MsgRequestTooLarge
	_ string = gokit.MsgAccessDenied
)

func TestResponseEnvelope(t *testing.T) {
	id := uuid.New()
	res := gokit.Response(http.StatusOK, gokit.MsgSuccess, id, map[string]string{"k": "v"})
	body, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got["log_id"] != id.String() || got["message"] != gokit.MsgSuccess || got["status"] != true {
		t.Fatalf("unexpected envelope: %s", body)
	}
}

func TestBindJSONBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	type req struct {
		Name string `json:"name" binding:"required"`
	}
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	var r req
	if gokit.BindJSON(ctx, uuid.New(), "[Test]", &r) {
		t.Fatal("expected BindJSON to fail")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestValidateUUIDSentinels(t *testing.T) {
	if _, err := gokit.ValidateUUID(""); !errors.Is(err, gokit.ErrMissingID) {
		t.Fatalf("expected ErrMissingID, got %v", err)
	}
	if _, err := gokit.ValidateUUID("nope"); !errors.Is(err, gokit.ErrInvalidUUID) {
		t.Fatalf("expected ErrInvalidUUID, got %v", err)
	}
}

func TestRateLimiterNilRedisPassesThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, mw := range map[string]gin.HandlerFunc{
		"ip":       gokit.IPRateLimit(gokit.RateLimitOptions{Prefix: "p", Limit: 1, Window: time.Minute}),
		"endpoint": gokit.EndpointRateLimit(gokit.RateLimitOptions{Limit: 1, Window: time.Minute}),
	} {
		r := gin.New()
		r.Use(mw)
		r.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })
		for i := 0; i < 3; i++ {
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
			if rec.Code != http.StatusNoContent {
				t.Fatalf("%s: request %d got %d", name, i, rec.Code)
			}
		}
	}
}

func TestNewRepositoryWithSQLMock(t *testing.T) {
	sqlDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB, PreferSimpleProtocol: true}),
		&gorm.Config{DryRun: true, SkipDefaultTransaction: true})
	if err != nil {
		t.Fatal(err)
	}

	repo := gokit.NewRepository[record](db)
	if err := repo.Store(context.Background(), &record{ID: "1"}); err != nil {
		t.Fatalf("Store: %v", err)
	}
}

func TestRequestIDRoundTrip(t *testing.T) {
	id := uuid.New()
	ctx := gokit.WithRequestID(context.Background(), id)
	if got := gokit.RequestID(ctx); got != id {
		t.Fatalf("got %v, want %v", got, id)
	}
}

func TestBaseParamsAndFilters(t *testing.T) {
	p, err := gokit.GetBaseParams(url.Values{"page": {"2"}, "limit": {"5"}}, "created_at", "desc", 10)
	if err != nil {
		t.Fatal(err)
	}
	if p.Page != 2 || p.Limit != 5 {
		t.Fatalf("unexpected params: %+v", p)
	}
	got := gokit.WhitelistFilter(map[string]any{"a": 1, "b": 2}, []string{"a"})
	if len(got) != 1 || got["a"] != 1 {
		t.Fatalf("unexpected filter: %v", got)
	}
}
