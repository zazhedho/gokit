package gokit_test

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/zazhedho/gokit"
)

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

	mock.ExpectQuery(regexp.QuoteMeta(`WHERE id = $1`)).WithArgs("1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("1"))
	if got, err := repo.GetByID(ctx, "1"); err != nil || got.ID != "1" {
		t.Fatalf("GetByID = %+v, %v", got, err)
	}

	mock.ExpectQuery(regexp.QuoteMeta(`WHERE name = $1`)).WithArgs("a", 1).
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
