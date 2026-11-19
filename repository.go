package gokit

import (
	"context"

	"gorm.io/gorm"

	"github.com/zazhedho/gokit/internal/repository"
)

// Repository is a generic GORM repository for model type T. It is a defined
// type, not an alias, so IDEs resolve its promoted DB field and methods when it
// is embedded.
//
// Column names passed to its methods are validated (ASCII letters, digits and
// "_", optionally "table.column") and quoted; values are always bound as
// parameters.
type Repository[T any] struct {
	DB *gorm.DB
	// KeyColumn is the key used by GetByID, Update, Delete and SoftDelete and as
	// the final ORDER BY tie-breaker. Empty means "id".
	KeyColumn string
}

// CRUDRepository is the contract implemented by Repository; depend on it in
// services so tests can substitute a fake.
type CRUDRepository[T any] interface {
	Store(ctx context.Context, m *T) error
	Upsert(ctx context.Context, values []T, conflictColumns, updateColumns []string) error
	GetByID(ctx context.Context, id string) (T, error)
	GetOneByField(ctx context.Context, field string, value any) (T, error)
	GetManyByField(ctx context.Context, field string, value any, limit int, order string) ([]T, error)
	ExistsByField(ctx context.Context, field string, value any) (bool, error)
	ExistsByFields(ctx context.Context, filters map[string]any) (bool, error)
	GetAll(ctx context.Context, params BaseParams, opts QueryOptions) ([]T, int64, error)
	Update(ctx context.Context, m *T) error
	Delete(ctx context.Context, id string) error
	SoftDelete(ctx context.Context, id, deletedBy string) error
}

// NewRepository returns a Repository for T backed by db, keyed on "id".
func NewRepository[T any](db *gorm.DB) *Repository[T] { return &Repository[T]{DB: db} }

func (r *Repository[T]) impl() *repository.Repository[T] {
	return &repository.Repository[T]{DB: r.DB, KeyColumn: r.KeyColumn}
}

// Store inserts m. Generated IDs and database defaults are written back into m.
func (r *Repository[T]) Store(ctx context.Context, m *T) error { return r.impl().Store(ctx, m) }

// Upsert inserts values and, on a conflict over conflictColumns, updates
// updateColumns from the new row. Both column lists are required. Large slices
// are inserted in batches of the DB's CreateBatchSize (500 when unset).
func (r *Repository[T]) Upsert(ctx context.Context, values []T, conflictColumns, updateColumns []string) error {
	return r.impl().Upsert(ctx, values, conflictColumns, updateColumns)
}

// GetByID returns the row whose key column equals id, or ErrNotFound.
func (r *Repository[T]) GetByID(ctx context.Context, id string) (T, error) {
	return r.impl().GetByID(ctx, id)
}

// GetOneByField returns the first row where field equals value (IS NULL when
// value is nil), or ErrNotFound.
func (r *Repository[T]) GetOneByField(ctx context.Context, field string, value any) (T, error) {
	return r.impl().GetOneByField(ctx, field, value)
}

// GetManyByField returns up to limit rows where field equals value, ordered by
// order ("column asc|desc"; empty orders by the key) and then by the key. limit
// is clamped to 1..DefaultMaxLimit. An invalid order wraps ErrInvalidOrderColumn
// or ErrInvalidOrderDirection.
func (r *Repository[T]) GetManyByField(ctx context.Context, field string, value any, limit int, order string) ([]T, error) {
	return r.impl().GetManyByField(ctx, field, value, limit, order)
}

// ExistsByField reports whether any row has field equal to value (IS NULL when
// value is nil).
func (r *Repository[T]) ExistsByField(ctx context.Context, field string, value any) (bool, error) {
	return r.impl().ExistsByField(ctx, field, value)
}

// ExistsByFields reports whether any row matches every field/value pair. An
// empty map returns an error wrapping ErrInvalidFilter instead of matching
// everything.
func (r *Repository[T]) ExistsByFields(ctx context.Context, filters map[string]any) (bool, error) {
	return r.impl().ExistsByFields(ctx, filters)
}

// GetAll returns one page of rows and the total count of rows matching the
// base query, search and filters.
//
// opts.Validate is checked first. BaseQuery and Search run with their WHERE
// conditions grouped, so an Or() inside cannot widen the query. The search text
// is truncated to MaxSearchLength. Filters apply only for AllowedFilters keys,
// and an order outside AllowedOrderColumns returns ErrInvalidOrderColumn. Rows
// are ordered by the requested or default order and then by the key column.
// The limit is clamped to 1..MaxLimit (default 100) and limit=-1 returns every
// row only with AllowAll. With params.Page of at least 1 the offset is derived
// from Page and the effective limit; otherwise params.Offset is used, never
// below 0.
func (r *Repository[T]) GetAll(ctx context.Context, params BaseParams, opts QueryOptions) ([]T, int64, error) {
	return r.impl().GetAll(ctx, params, opts)
}

// Update writes every column of m except the key, created_at, deleted_at,
// deleted_by and associations to the row whose key equals m's key; zero values
// are written. It never inserts and never revives a soft-deleted row: when no
// live row matches it returns ErrNotFound. A zero key is an error.
func (r *Repository[T]) Update(ctx context.Context, m *T) error { return r.impl().Update(ctx, m) }

// Delete removes the row whose key equals id and returns ErrNotFound when none
// matched. It follows gorm: a model with a gorm.DeletedAt field is soft-deleted
// (deleted_at is set and later queries skip the row); any other model is
// deleted for real. SoftDelete marks rows that keep a plain deleted_at column.
func (r *Repository[T]) Delete(ctx context.Context, id string) error { return r.impl().Delete(ctx, id) }

// SoftDelete sets deleted_by and deleted_at (UTC) on the live row whose key
// equals id. It returns ErrNotFound when no live row matched, so a second call
// fails. The model needs deleted_at and deleted_by columns.
func (r *Repository[T]) SoftDelete(ctx context.Context, id, deletedBy string) error {
	return r.impl().SoftDelete(ctx, id, deletedBy)
}
