package gokit_test

import (
	"context"
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

	"github.com/zazhedho/gokit"
)

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
