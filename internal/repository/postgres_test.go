package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/zazhedho/gokit/internal/filter"
)

// These tests need a real PostgreSQL: set GOKIT_TEST_DATABASE_URL to a DSN. They
// create and drop a private schema and are skipped otherwise.
func realDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("GOKIT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("GOKIT_TEST_DATABASE_URL is not set")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1) // search_path is per connection
	schemaName := fmt.Sprintf("gokit_test_%d", time.Now().UnixNano())
	if err := db.Exec("CREATE SCHEMA " + schemaName).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec("DROP SCHEMA " + schemaName + " CASCADE")
		_ = sqlDB.Close()
	})
	if err := db.Exec("SET search_path TO " + schemaName).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&sampleRecord{}, &softRecord{}, &flagRecord{}, &generatedRecord{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func idsOf[T any](rows []T, id func(T) string) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = id(r)
	}
	return out
}

func TestPostgresSearchEscapesWildcards(t *testing.T) {
	db := realDB(t)
	repo := NewRepository[sampleRecord](db)
	for i, name := range []string{"50% off", "500 off", "a_b", "axb", `back\slash`, "backslash"} {
		if err := repo.Store(ctx(), &sampleRecord{ID: fmt.Sprint(i), Name: name}); err != nil {
			t.Fatal(err)
		}
	}
	opts := QueryOptions{Search: BuildSearchFunc("name")}
	for search, want := range map[string][]string{"50%": {"50% off"}, "a_b": {"a_b"}, `\`: {`back\slash`}, "OFF": {"50% off", "500 off"}} {
		rows, total, err := repo.GetAll(ctx(), filter.BaseParams{Limit: 10, Page: 1, Search: search}, opts)
		if err != nil {
			t.Fatalf("%q: %v", search, err)
		}
		got := idsOf(rows, func(r sampleRecord) string { return r.Name })
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) || total != int64(len(want)) {
			t.Errorf("search %q: got %v (total %d), want %v", search, got, total, want)
		}
	}
}

func TestPostgresRowsAffectedSemantics(t *testing.T) {
	repo := NewRepository[sampleRecord](realDB(t))
	if err := repo.Update(ctx(), &sampleRecord{ID: "ghost", Name: "x"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Update missing: %v", err)
	}
	if err := repo.Delete(ctx(), "ghost"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete missing: %v", err)
	}
	if err := repo.Store(ctx(), &sampleRecord{ID: "1", Name: "old", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(ctx(), &sampleRecord{ID: "1", Name: "new"}); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetByID(ctx(), "1")
	if err != nil || got.Name != "new" || got.Status != "" {
		t.Fatalf("Update writes zero values too: %+v %v", got, err)
	}
	if err := repo.Delete(ctx(), "1"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetByID(ctx(), "1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestPostgresSoftDeleteReadWriteSemantics(t *testing.T) {
	db := realDB(t)

	flags := NewRepository[flagRecord](db)
	if err := flags.Store(ctx(), &flagRecord{ID: "f1", Name: "n"}); err != nil {
		t.Fatal(err)
	}
	if err := flags.SoftDelete(ctx(), "f1", "admin"); err != nil {
		t.Fatal(err)
	}
	if err := flags.SoftDelete(ctx(), "f1", "admin"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second SoftDelete: %v", err)
	}
	if err := flags.Update(ctx(), &flagRecord{ID: "f1", Name: "revived"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Update must not revive: %v", err)
	}
	if _, err := flags.GetByID(ctx(), "f1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByID: %v", err)
	}
	if ok, err := flags.ExistsByField(ctx(), "name", "n"); err != nil || ok {
		t.Fatalf("Exists: %v %v", ok, err)
	}
	if _, total, err := flags.GetAll(ctx(), filter.BaseParams{Limit: 5, Page: 1}, QueryOptions{}); err != nil || total != 0 {
		t.Fatalf("GetAll total %d: %v", total, err)
	}
	var stored flagRecord
	if err := db.First(&stored, "id = ?", "f1").Error; err != nil || stored.DeletedAt == nil {
		t.Fatalf("row must still exist with deleted_at set: %+v %v", stored, err)
	}

	softs := NewRepository[softRecord](db)
	if err := softs.Store(ctx(), &softRecord{ID: "s1", Name: "n"}); err != nil {
		t.Fatal(err)
	}
	if err := softs.Delete(ctx(), "s1"); err != nil {
		t.Fatal(err)
	}
	if err := softs.Update(ctx(), &softRecord{ID: "s1", Name: "x"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Update of a gorm soft-deleted row: %v", err)
	}
	if err := softs.Delete(ctx(), "s1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second Delete: %v", err)
	}
	var n int64
	db.Unscoped().Model(&softRecord{}).Count(&n)
	if n != 1 {
		t.Fatalf("Delete must soft-delete, rows left: %d", n)
	}
}

func TestPostgresTieBreakerGivesStablePages(t *testing.T) {
	repo := NewRepository[sampleRecord](realDB(t))
	for i := range 7 {
		if err := repo.Store(ctx(), &sampleRecord{ID: fmt.Sprintf("id%d", i), Status: "same"}); err != nil {
			t.Fatal(err)
		}
	}
	var all []string
	for page := 1; page <= 4; page++ {
		rows, total, err := repo.GetAll(ctx(), filter.BaseParams{Page: page, Limit: 2, OrderBy: "status", OrderDirection: "asc"},
			QueryOptions{AllowedOrderColumns: []string{"status"}})
		if err != nil || total != 7 {
			t.Fatalf("page %d: total %d %v", page, total, err)
		}
		all = append(all, idsOf(rows, func(r sampleRecord) string { return r.ID })...)
	}
	if want := []string{"id0", "id1", "id2", "id3", "id4", "id5", "id6"}; !slices.Equal(all, want) {
		t.Fatalf("pages must partition the rows in key order: %v", all)
	}
}

func TestPostgresStoreFlowsGeneratedValuesBack(t *testing.T) {
	m := &generatedRecord{}
	if err := NewRepository[generatedRecord](realDB(t)).Store(context.Background(), m); err != nil || m.ID == "" || m.Name != "anonymous" {
		t.Fatalf("%+v %v", m, err)
	}
}
