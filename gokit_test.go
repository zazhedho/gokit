package gokit_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"regexp"
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

func TestErrorHelpers(t *testing.T) {
	id := uuid.New()
	for name, res := range map[string]*gokit.APIResponse{
		"error":        gokit.ErrorResponse(http.StatusConflict, "Conflict", id, "taken"),
		"forbidden":    gokit.Forbidden(id, "no"),
		"unauthorized": gokit.Unauthorized(id, "login"),
		"internal":     gokit.InternalServerError(id),
	} {
		if res.Status || len(res.Error) != 1 || res.LogID != id {
			t.Errorf("%s: %+v", name, res)
		}
	}
}

func TestValidateUUIDParamFacade(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Params = gin.Params{{Key: "id", Value: "nope"}}
	if _, err := gokit.ValidateUUIDParam(c, uuid.New()); err == nil || rec.Code != http.StatusBadRequest {
		t.Fatalf("got %v %d", err, rec.Code)
	}
}

func TestConfigAndConversionFacade(t *testing.T) {
	t.Setenv("FACADE_LIST", "a, b")
	t.Setenv("FACADE_BAD", "x")
	t.Setenv("FACADE_D", "90s")
	t.Cleanup(gokit.ResetEnvErrors)
	t.Cleanup(func() { gokit.SetEODLocation(nil) })

	if got := gokit.GetEnv("FACADE_LIST", []string(nil)); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("got %v", got)
	}
	gokit.GetEnv("FACADE_BAD", 1)
	if err := gokit.EnvErrors(); err == nil || !strings.Contains(err.Error(), "FACADE_BAD") {
		t.Fatalf("got %v", err)
	}
	gokit.ResetEnvErrors()
	if gokit.EnvErrors() != nil {
		t.Fatal("reset must clear errors")
	}
	if got := gokit.DurationFromEnv([]string{"FACADE_MISSING", "FACADE_D"}, time.Second); got != 90*time.Second {
		t.Fatalf("got %v", got)
	}
	gokit.SetEODLocation(time.UTC)

	if _, err := uuid.Parse(gokit.CreateUUID()); err != nil {
		t.Fatal(err)
	}
	if gokit.InterfaceString(42) != "42" || gokit.JSONEncode(map[string]int{"a": 1}) != `{"a":1}` {
		t.Fatal("conversion helpers")
	}
	if got := gokit.ConvertValuesToString(map[string]any{"a": 1, "b": 2}, "a"); got["a"] != "1" || got["b"] != 2 {
		t.Fatalf("got %v", got)
	}
	if got := gokit.WhitelistStringFilter(map[string]any{"a": 1, "b": 2}, []string{"a"}); got["a"] != "1" || len(got) != 1 {
		t.Fatalf("got %v", got)
	}
	got := gokit.RedactSensitivePayload(map[string]any{"password": "p", "zip": "1"}, "zip").(map[string]any)
	if got["password"] != "[REDACTED]" || got["zip"] != "[REDACTED]" {
		t.Fatalf("got %v", got)
	}
}

func TestValidateErrorAndMessagesFacade(t *testing.T) {
	type req struct {
		Name string `json:"name" binding:"required"`
	}
	err := errors.New("raw decoder text")
	if got := gokit.ValidateError(err, reflect.TypeFor[req](), "json"); len(got) != 1 || strings.Contains(got[0].Message, "raw") {
		t.Fatalf("got %+v", got)
	}
	gokit.SetValidationMessage("required", func(string) string { return "needed" })
	gokit.SetValidationMessage("required", nil)
}

func TestRepositoryFacadeMethods(t *testing.T) {
	repo, mock := newMockRepo(t)
	ctx := context.Background()
	r := repo.Repository

	mock.ExpectExec(`INSERT INTO "records"`).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := r.Upsert(ctx, []record{{ID: "1"}}, []string{"id"}, []string{"id"}); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(`LIMIT`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("1"))
	if rows, err := r.GetManyByField(ctx, "id", "1", 5, "id asc"); err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	mock.ExpectQuery(`SELECT 1`).WillReturnRows(sqlmock.NewRows([]string{"x"}).AddRow(1))
	if ok, err := r.ExistsByField(ctx, "id", "1"); err != nil || !ok {
		t.Fatal(ok, err)
	}
	mock.ExpectQuery(`SELECT 1`).WillReturnRows(sqlmock.NewRows([]string{"x"}))
	if ok, err := r.ExistsByFields(ctx, map[string]any{"id": "2"}); err != nil || ok {
		t.Fatal(ok, err)
	}
	if _, err := r.ExistsByFields(ctx, nil); !errors.Is(err, gokit.ErrInvalidFilter) {
		t.Fatalf("got %v", err)
	}
	if err := r.Update(ctx, &record{ID: "1"}); err == nil {
		t.Fatal("a record with only a key has nothing to update")
	}
	mock.ExpectExec(`DELETE FROM "records"`).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := r.Delete(ctx, "9"); !errors.Is(err, gokit.ErrNotFound) {
		t.Fatalf("got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

type flagged struct {
	ID        string `gorm:"column:id;primaryKey"`
	Name      string
	DeletedAt *time.Time
	DeletedBy *string
}

func (flagged) TableName() string { return "flagged" }

func TestSoftDeleteAndSearchFacade(t *testing.T) {
	repo, mock := newMockRepo(t)
	_ = repo
	db := repo.DB
	flags := gokit.NewRepository[flagged](db)
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "flagged"`)).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := flags.SoftDelete(context.Background(), "1", "me"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}

	_, _, err := repo.GetAll(context.Background(), gokit.BaseParams{Limit: 1, Search: "x"},
		gokit.QueryOptions{Search: gokit.BuildSearchFunc()})
	if err == nil {
		t.Fatal("a search without columns must fail")
	}
	q := url.Values{"limit": {"5"}}
	if p, err := gokit.GetBaseParams(q, "id", "asc", 10); err != nil || p.Limit != 5 {
		t.Fatal(p, err)
	}
	if got := (gokit.QueryOptions{MaxLimit: 20}).PageSize(gokit.BaseParams{Limit: 999}); got != 20 {
		t.Fatalf("PageSize = %d", got)
	}
}

// The contract must be satisfied by the concrete type; checked at compile time.
var _ gokit.CRUDRepository[record] = (*gokit.Repository[record])(nil)

// userRepo embeds the generic repository the way consuming modules do.
type userRepo struct {
	*gokit.Repository[record]
}

func newMockRepo(t *testing.T) (*userRepo, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB, PreferSimpleProtocol: true}),
		&gorm.Config{SkipDefaultTransaction: true})
	if err != nil {
		t.Fatal(err)
	}
	return &userRepo{Repository: gokit.NewRepository[record](db)}, mock
}

func TestEmbeddedRepositoryExposesDBAndMethods(t *testing.T) {
	repo, mock := newMockRepo(t)
	ctx := context.Background()

	if repo.DB == nil {
		t.Fatal("embedded DB must be reachable")
	}

	mock.ExpectQuery(regexp.QuoteMeta(`WHERE "records"."id" = $1`)).WithArgs("1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("1"))
	if got, err := repo.GetByID(ctx, "1"); err != nil || got.ID != "1" {
		t.Fatalf("GetByID = %+v, %v", got, err)
	}

	mock.ExpectQuery(regexp.QuoteMeta(`WHERE "name" = $1`)).WithArgs("a", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("2"))
	if got, err := repo.GetOneByField(ctx, "name", "a"); err != nil || got.ID != "2" {
		t.Fatalf("GetOneByField = %+v, %v", got, err)
	}

	mock.ExpectQuery(regexp.QuoteMeta(`count(*)`)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "records"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("3"))
	items, total, err := repo.GetAll(ctx, gokit.BaseParams{Page: 1, Limit: 10}, gokit.QueryOptions{})
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("GetAll = %v, %d, %v", items, total, err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
