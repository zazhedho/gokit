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
	"github.com/redis/go-redis/v9"
	"github.com/zazhedho/gokit"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type record struct {
	ID string `gorm:"column:id;primaryKey"`
}

func (record) TableName() string { return "records" }

// Compile-time proof that every facade identifier exists with the expected shape.
var (
	// helpers
	_ func(map[string]any, ...string) map[string]any            = gokit.ConvertValuesToString
	_ func() string                                             = gokit.CreateUUID
	_ func([]string, time.Duration) time.Duration               = gokit.DurationFromEnv
	_ func() json.RawMessage                                    = gokit.EmptyJSON
	_ func(string) string                                       = gokit.EnvKey
	_ func(...string) string                                    = gokit.FirstNonEmptyString
	_ func(context.Context) uuid.UUID                           = gokit.GenerateLogId
	_ func(string, int) int                                     = gokit.GetEnv[int]
	_ func(context.Context) string                              = gokit.GetRequestID
	_ func(string) string                                       = gokit.HumanizeKey
	_ func(string) *int64                                       = gokit.Int64PtrFromString
	_ func(int64) *int64                                        = gokit.Int64PtrIfPositive
	_ func(any) bool                                            = gokit.InterfaceBool
	_ func(any) string                                          = gokit.InterfaceString
	_ func(string) bool                                         = gokit.IsSensitiveKey
	_ func(any) string                                          = gokit.JsonEncode
	_ func(map[string]any, map[string]any) map[string]any       = gokit.MergeMetadata
	_ func(any) json.RawMessage                                 = gokit.MustJSON
	_ func(string) string                                       = gokit.NormalizeKey
	_ func(any) any                                             = gokit.NormalizePayload
	_ func(string) string                                       = gokit.NormalizePhoneTo62
	_ func(string) *string                                      = gokit.NormalizeUUIDPointer
	_ func(string) string                                       = gokit.NormalizeUpperKey
	_ func(any) any                                             = gokit.RedactSensitivePayload
	_ func(any) any                                             = gokit.RedactSensitiveValue
	_ func(string) string                                       = gokit.SanitizeEmail
	_ func(string, string) string                               = gokit.StringOrDefault
	_ func(string) *string                                      = gokit.StringPtrIfNotEmpty
	_ func(string) string                                       = gokit.StripHTML
	_ func(string) string                                       = gokit.TitleCase
	_ func(string) string                                       = gokit.TitleHumanized
	_ func(string) (string, error)                              = gokit.ValidateUUID
	_ func(context.Context, uuid.UUID) context.Context          = gokit.WithRequestID
	_ func(error, reflect.Type, string) []gokit.ValidateMessage = gokit.ValidateError
	_ gokit.ValidateMessage
	_ error = gokit.ErrMissingID
	_ error = gokit.ErrInvalidUUID

	// http
	_ func(int, string, uuid.UUID, string) *gokit.ApiResponse           = gokit.ErrorResponse
	_ func(uuid.UUID, string) *gokit.ApiResponse                        = gokit.Forbidden
	_ func(uuid.UUID) *gokit.ApiResponse                                = gokit.InternalServerError
	_ func(int, string, uuid.UUID, any) *gokit.ApiResponse              = gokit.Response
	_ func(uuid.UUID, string) *gokit.ApiResponse                        = gokit.Unauthorized
	_ func(int, int, int, int, uuid.UUID, any) *gokit.PaginatedResponse = gokit.PaginationResponse
	_ func() gin.HandlerFunc                                            = gokit.CORS
	_ func(*redis.Client, int, time.Duration) gin.HandlerFunc           = gokit.EndpointRateLimitMiddleware
	_ func(*gin.Context, any)                                           = gokit.ErrorHandler
	_ func(*redis.Client, string, int, time.Duration) gin.HandlerFunc   = gokit.IPRateLimitMiddleware
	_ func() gin.HandlerFunc                                            = gokit.SetContextId
	_ func(*gin.Context, uuid.UUID, string, *record) bool               = gokit.BindJSON[record]
	_ func(*gin.Context, uuid.UUID) (string, error)                     = gokit.ValidateUUIDParam
	_ gokit.Error
	_ gokit.Errors
	_ gokit.Pagination
	_ gokit.Success
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
)

// Constants keep their string values.
const (
	_ string = gokit.CtxKeyId
	_ string = gokit.MsgInvalidRequest
	_ string = gokit.MsgSomethingWrong
	_ string = gokit.MsgInternal
	_ string = gokit.MsgExists
	_ string = gokit.MsgNotFound
	_ string = gokit.MsgResourceNotFound
	_ string = gokit.MsgSuccess
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
		"ip":       gokit.IPRateLimitMiddleware(nil, "p", 1, time.Minute),
		"endpoint": gokit.EndpointRateLimitMiddleware(nil, 1, time.Minute),
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
	if err := repo.Store(context.Background(), record{ID: "1"}); err != nil {
		t.Fatalf("Store: %v", err)
	}
}

func TestRequestIDRoundTrip(t *testing.T) {
	id := uuid.New()
	ctx := gokit.WithRequestID(context.Background(), id)
	if got := gokit.GetRequestID(ctx); got != id.String() {
		t.Fatalf("got %q, want %q", got, id)
	}
	if got := gokit.GenerateLogId(ctx); got != id {
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
