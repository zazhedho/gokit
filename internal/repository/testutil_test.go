package repository

import (
	"testing"
	"time"

	"github.com/zazhedho/gokit/internal/filter"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type sampleRecord struct {
	ID     string `gorm:"column:id;primaryKey"`
	Name   string `gorm:"column:name"`
	Status string `gorm:"column:status"`
}

func (sampleRecord) TableName() string { return "sample_records" }

// softRecord uses gorm's own soft delete.
type softRecord struct {
	ID        string `gorm:"column:id;primaryKey"`
	Name      string `gorm:"column:name"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at"`
	DeletedBy *string        `gorm:"column:deleted_by"`
}

func (softRecord) TableName() string { return "soft_records" }

// flagRecord has a plain deleted_at column, which gorm does not filter on its own.
type flagRecord struct {
	ID        string `gorm:"column:id;primaryKey"`
	Name      string `gorm:"column:name"`
	CreatedAt time.Time
	DeletedAt *time.Time `gorm:"column:deleted_at"`
	DeletedBy *string    `gorm:"column:deleted_by"`
}

func (flagRecord) TableName() string { return "flag_records" }

type generatedRecord struct {
	ID   string `gorm:"column:id;primaryKey;default:gen_random_uuid()"`
	Name string `gorm:"column:name;default:'anonymous'"`
}

func (generatedRecord) TableName() string { return "generated_records" }

type uuidKeyed struct {
	UUID string `gorm:"column:uuid;primaryKey"`
	Name string `gorm:"column:name"`
}

func (uuidKeyed) TableName() string { return "uuid_keyed" }

func newMockDB(t *testing.T, cfg *gorm.Config) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	if cfg == nil {
		cfg = &gorm.Config{}
	}
	cfg.SkipDefaultTransaction = true
	cfg.Logger = logger.Default.LogMode(logger.Silent)
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB, PreferSimpleProtocol: true}), cfg)
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}
	return db, mock
}

func newDryRunDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, _ := newMockDB(t, &gorm.Config{DryRun: true})
	return db
}

func countRows(n int64) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"count"}).AddRow(n)
}

func idRows(ids ...string) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"id"})
	for _, id := range ids {
		rows.AddRow(id)
	}
	return rows
}

func baseParams() filter.BaseParams { return filter.BaseParams{Limit: 5, Page: 1} }
