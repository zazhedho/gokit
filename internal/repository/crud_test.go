package repository

import (
	"context"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/gorm"
)

func ctx() context.Context { return context.Background() }

func TestStoreFlowsGeneratedValuesBack(t *testing.T) {
	db, mock := newMockDB(t, nil)
	mock.ExpectQuery(`INSERT INTO "generated_records".*RETURNING`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow("generated-id", "anonymous"))

	m := &generatedRecord{}
	if err := NewRepository[generatedRecord](db).Store(ctx(), m); err != nil {
		t.Fatal(err)
	}
	if m.ID != "generated-id" || m.Name != "anonymous" {
		t.Fatalf("generated ID and defaults must reach the caller, got %+v", m)
	}
	if err := NewRepository[generatedRecord](db).Store(ctx(), nil); err == nil {
		t.Fatal("a nil record must be rejected")
	}
}

func TestUpdateScopesByKeyAndNeverSaves(t *testing.T) {
	db, mock := newMockDB(t, nil)
	mock.ExpectExec(`^UPDATE "soft_records" SET .*WHERE "soft_records"."id" = \$\d+ AND "soft_records"."deleted_at" IS NULL$`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	m := &softRecord{ID: "r1", Name: "new"}
	if err := NewRepository[softRecord](db).Update(ctx(), m); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateWritesAllColumnsExceptProtectedOnes(t *testing.T) {
	db := newDryRunDB(t)
	m := &softRecord{ID: "r1"}
	repo := NewRepository[softRecord](db)
	var sql string
	if err := db.Callback().Update().After("gorm:update").Register("capture", func(tx *gorm.DB) { sql = tx.Statement.SQL.String() }); err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(ctx(), m); !errors.Is(err, ErrNotFound) {
		t.Fatalf("dry run affects no rows: %v", err)
	}
	for _, col := range []string{`"name"`, `"updated_at"`} {
		if !strings.Contains(sql, col) {
			t.Errorf("zero-valued columns must still be written, missing %s in %s", col, sql)
		}
	}
	setPart := sql[:strings.Index(sql, "WHERE")]
	for _, col := range []string{`"created_at"`, `"deleted_at"`, `"deleted_by"`, `"id"`} {
		if strings.Contains(setPart, col) {
			t.Errorf("%s must not be in SET: %s", col, setPart)
		}
	}
}

func TestUpdateOnMissingRowIsErrNotFound(t *testing.T) {
	db, mock := newMockDB(t, nil)
	mock.ExpectExec(`UPDATE "sample_records"`).WillReturnResult(sqlmock.NewResult(0, 0))
	err := NewRepository[sampleRecord](db).Update(ctx(), &sampleRecord{ID: "ghost", Name: "x"})
	if !errors.Is(err, ErrNotFound) || !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestUpdateNeverResurrectsSoftDeletedRows(t *testing.T) {
	// gorm soft delete: the scope comes from gorm itself.
	db, mock := newMockDB(t, nil)
	mock.ExpectExec(`"soft_records"."deleted_at" IS NULL`).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := NewRepository[softRecord](db).Update(ctx(), &softRecord{ID: "gone"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("soft-deleted row (gorm.DeletedAt): got %v", err)
	}

	// plain deleted_at column: the repository adds the scope and never writes the column.
	db, mock = newMockDB(t, nil)
	mock.ExpectExec(`^UPDATE "flag_records" SET ((?:"name"|"updated_at"|=|\$\d+|,| )+) WHERE "flag_records"."id" = \$\d+ AND "flag_records"."deleted_at" IS NULL$`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := NewRepository[flagRecord](db).Update(ctx(), &flagRecord{ID: "gone", Name: "x"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("soft-deleted row (plain column): got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateRequiresAKey(t *testing.T) {
	err := NewRepository[sampleRecord](newDryRunDB(t)).Update(ctx(), &sampleRecord{Name: "x"})
	if err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("a zero key must be rejected outright, got %v", err)
	}
	if err := NewRepository[sampleRecord](newDryRunDB(t)).Update(ctx(), nil); err == nil {
		t.Fatal("nil must be rejected")
	}
}

func TestDeleteAndSoftDeleteReturnErrNotFoundOnZeroRows(t *testing.T) {
	db, mock := newMockDB(t, nil)
	mock.ExpectExec(`DELETE FROM "sample_records" WHERE`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE "flag_records"`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`DELETE FROM "sample_records" WHERE`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "flag_records"`).WillReturnResult(sqlmock.NewResult(0, 1))

	samples, flags := NewRepository[sampleRecord](db), NewRepository[flagRecord](db)
	if err := samples.Delete(ctx(), "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete: %v", err)
	}
	if err := flags.SoftDelete(ctx(), "x", "admin"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SoftDelete: %v", err)
	}
	if err := samples.Delete(ctx(), "x"); err != nil {
		t.Fatalf("Delete existing: %v", err)
	}
	if err := flags.SoftDelete(ctx(), "x", "admin"); err != nil {
		t.Fatalf("SoftDelete existing: %v", err)
	}
}

type utcArg struct{}

func (utcArg) Match(v driver.Value) bool {
	ts, ok := v.(time.Time)
	return ok && ts.Location() == time.UTC && time.Since(ts) < time.Minute
}

func TestSoftDeleteUsesUTCAndSkipsAlreadyDeletedRows(t *testing.T) {
	db, mock := newMockDB(t, nil)
	mock.ExpectExec(`^UPDATE "flag_records" SET .*WHERE "flag_records"."id" = \$\d+ AND "flag_records"."deleted_at" IS NULL$`).
		WithArgs(utcArg{}, "admin", "r1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := NewRepository[flagRecord](db).SoftDelete(ctx(), "r1", "admin"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}

	db, mock = newMockDB(t, nil)
	mock.ExpectExec(`"soft_records"."deleted_at" IS NULL`).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := NewRepository[softRecord](db).SoftDelete(ctx(), "r1", "admin"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second soft delete of a gorm.DeletedAt model: %v", err)
	}
}

func TestDeleteFollowsGormSoftDelete(t *testing.T) {
	db, mock := newMockDB(t, nil)
	mock.ExpectExec(`^UPDATE "soft_records" SET "deleted_at"=\$1 WHERE "soft_records"."id" = \$2 AND "soft_records"."deleted_at" IS NULL$`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := NewRepository[softRecord](db).Delete(ctx(), "r1"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestExistsByFields(t *testing.T) {
	repo := func(t *testing.T) (*Repository[sampleRecord], sqlmock.Sqlmock) {
		db, mock := newMockDB(t, nil)
		return NewRepository[sampleRecord](db), mock
	}

	t.Run("empty map is an error", func(t *testing.T) {
		r, _ := repo(t)
		for _, filters := range []map[string]any{nil, {}} {
			if _, err := r.ExistsByFields(ctx(), filters); err == nil {
				t.Fatal("an empty filter must not match every row")
			}
		}
	})
	t.Run("sorted keys and LIMIT 1", func(t *testing.T) {
		r, mock := repo(t)
		mock.ExpectQuery(`^SELECT 1 FROM "sample_records" WHERE "name" = \$1 AND "status" = \$2 LIMIT \$3$`).
			WithArgs("Jane", "active", 1).
			WillReturnRows(sqlmock.NewRows([]string{"?column?"}).AddRow(1))
		ok, err := r.ExistsByFields(ctx(), map[string]any{"status": "active", "name": "Jane"})
		if err != nil || !ok {
			t.Fatalf("ok=%v err=%v", ok, err)
		}
	})
	t.Run("no rows", func(t *testing.T) {
		r, mock := repo(t)
		mock.ExpectQuery(`SELECT 1`).WillReturnRows(sqlmock.NewRows([]string{"?column?"}))
		if ok, err := r.ExistsByField(ctx(), "name", "x"); err != nil || ok {
			t.Fatalf("ok=%v err=%v", ok, err)
		}
	})
	t.Run("nil uses IS NULL", func(t *testing.T) {
		r, mock := repo(t)
		mock.ExpectQuery(`WHERE "name" IS NULL LIMIT`).WillReturnRows(sqlmock.NewRows([]string{"?column?"}).AddRow(1))
		if ok, err := r.ExistsByField(ctx(), "name", nil); err != nil || !ok {
			t.Fatalf("ok=%v err=%v", ok, err)
		}
	})
	t.Run("unsafe column", func(t *testing.T) {
		r, _ := repo(t)
		if _, err := r.ExistsByFields(ctx(), map[string]any{"status OR 1=1": "x"}); err == nil || !strings.Contains(err.Error(), "invalid column") {
			t.Fatalf("got %v", err)
		}
	})
}

func TestFieldLookupsQuoteIdentifiersAndValidateExactly(t *testing.T) {
	db, mock := newMockDB(t, nil)
	repo := NewRepository[sampleRecord](db)

	mock.ExpectQuery(`WHERE "users"."email" = \$1 ORDER BY .* LIMIT`).WillReturnRows(idRows("1"))
	if _, err := repo.GetOneByField(ctx(), "users.email", "a@b"); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"name OR 1=1", " name", "name\n"} {
		if _, err := repo.GetOneByField(ctx(), field, "x"); err == nil {
			t.Errorf("%q must be rejected", field)
		}
		if _, err := repo.GetManyByField(ctx(), field, "x", 5, ""); err == nil {
			t.Errorf("%q must be rejected by GetManyByField", field)
		}
	}
}

func TestGetManyByFieldHasLimitAndOrder(t *testing.T) {
	for name, tc := range map[string]struct {
		limit int
		order string
		sql   string
		arg   int
	}{
		"explicit": {5, "name desc", `WHERE "status" = \$1 ORDER BY "name" DESC,"sample_records"."id" LIMIT \$2`, 5},
		"default":  {5, "", `ORDER BY "sample_records"."id" LIMIT \$2`, 5},
		"capped":   {5000, "", `LIMIT \$2`, DefaultMaxLimit},
		"floor":    {0, "", `LIMIT \$2`, 1},
	} {
		db, mock := newMockDB(t, nil)
		mock.ExpectQuery(tc.sql).WithArgs("active", tc.arg).WillReturnRows(idRows("1"))
		if _, err := NewRepository[sampleRecord](db).GetManyByField(ctx(), "status", "active", tc.limit, tc.order); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}

	repo := NewRepository[sampleRecord](newDryRunDB(t))
	if _, err := repo.GetManyByField(ctx(), "status", "x", 5, "name sideways"); !errors.Is(err, ErrInvalidOrderDirection) {
		t.Fatalf("got %v", err)
	}
	if _, err := repo.GetManyByField(ctx(), "status", "x", 5, "name; DROP"); err == nil {
		t.Fatal("unsafe order must be rejected")
	}
	if _, err := repo.GetManyByField(ctx(), "status", "x", 5, "na me desc"); !errors.Is(err, ErrInvalidOrderColumn) {
		t.Fatalf("got %v", err)
	}
}

func TestKeyColumnIsConfigurable(t *testing.T) {
	db, mock := newMockDB(t, nil)
	repo := NewRepository[uuidKeyed](db)
	repo.KeyColumn = "uuid"

	mock.ExpectQuery(`WHERE "uuid_keyed"."uuid" = \$1`).WillReturnRows(sqlmock.NewRows([]string{"uuid", "name"}).AddRow("u1", "n"))
	if got, err := repo.GetByID(ctx(), "u1"); err != nil || got.UUID != "u1" {
		t.Fatalf("GetByID: %+v %v", got, err)
	}
	mock.ExpectExec(`DELETE FROM "uuid_keyed" WHERE "uuid_keyed"."uuid" = \$1`).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := repo.Delete(ctx(), "u1"); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec(`UPDATE "uuid_keyed" SET .*WHERE "uuid_keyed"."uuid" = \$\d+`).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := repo.Update(ctx(), &uuidKeyed{UUID: "u1", Name: "n"}); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(`SELECT count\(\*\)`).WillReturnRows(countRows(0))
	mock.ExpectQuery(`ORDER BY "uuid_keyed"."uuid"`).WillReturnRows(sqlmock.NewRows([]string{"uuid", "name"}))
	if _, _, err := repo.GetAll(ctx(), baseParams(), QueryOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}

	repo.KeyColumn = "bad key"
	if _, err := repo.GetByID(ctx(), "x"); !errors.Is(err, ErrInvalidQueryOptions) {
		t.Fatalf("an invalid key column must be rejected, got %v", err)
	}
}

func TestUpsertHonorsCreateBatchSize(t *testing.T) {
	db, mock := newMockDB(t, &gorm.Config{CreateBatchSize: 2})
	for range 3 { // 5 rows in batches of 2, 2, 1
		mock.ExpectExec(`INSERT INTO "sample_records".*ON CONFLICT \("id"\) DO UPDATE SET "name"="excluded"."name"`).
			WillReturnResult(sqlmock.NewResult(0, 2))
	}
	rows := []sampleRecord{{ID: "1"}, {ID: "2"}, {ID: "3"}, {ID: "4"}, {ID: "5"}}
	if err := NewRepository[sampleRecord](db).Upsert(ctx(), rows, []string{"id"}, []string{"name"}); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUpsertValidation(t *testing.T) {
	repo := NewRepository[sampleRecord](newDryRunDB(t))
	one := []sampleRecord{{ID: "1"}}

	if err := repo.Upsert(ctx(), nil, []string{"id"}, []string{"name"}); err != nil {
		t.Fatalf("empty upsert: %v", err)
	}
	for name, args := range map[string][2][]string{
		"no conflict":    {nil, {"name"}},
		"no update":      {{"id"}, nil},
		"bad conflict":   {{"id OR 1=1"}, {"name"}},
		"bad update":     {{"id"}, {"name; --"}},
		"padded":         {{"id"}, {" name"}},
		"too many parts": {{"a.b.c"}, {"name"}},
	} {
		if err := repo.Upsert(ctx(), one, args[0], args[1]); err == nil {
			t.Errorf("%s must be rejected", name)
		}
	}
}

func TestGetByIDMapsNotFound(t *testing.T) {
	db, mock := newMockDB(t, nil)
	mock.ExpectQuery(`SELECT`).WillReturnRows(idRows())
	if _, err := NewRepository[sampleRecord](db).GetByID(ctx(), "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v", err)
	}
}
