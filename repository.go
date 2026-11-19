package gokit

import (
	"context"

	"gorm.io/gorm"

	"github.com/zazhedho/gokit/internal/repository"
)

// Repository is a generic GORM repository for model type T. It is a defined
// type, not an alias, so IDEs resolve its promoted DB field and methods when it
// is embedded.
type Repository[T any] struct {
	DB *gorm.DB
}

// CRUDRepository is the basic CRUD contract implemented by Repository.
type CRUDRepository[T any] interface {
	Store(ctx context.Context, data T) error
	GetByID(ctx context.Context, id string) (T, error)
	GetAll(ctx context.Context, params BaseParams, opts QueryOptions) ([]T, int64, error)
	Update(ctx context.Context, data T) error
	Delete(ctx context.Context, id string) error
	SoftDelete(ctx context.Context, id, deletedBy string) error
}

// NewRepository returns a Repository for T backed by db.
func NewRepository[T any](db *gorm.DB) *Repository[T] { return &Repository[T]{DB: db} }

func (r *Repository[T]) impl() *repository.Repository[T] {
	return &repository.Repository[T]{DB: r.DB}
}

func (r *Repository[T]) Store(ctx context.Context, m T) error { return r.impl().Store(ctx, m) }

func (r *Repository[T]) Upsert(ctx context.Context, values []T, conflictColumns, updateColumns []string) error {
	return r.impl().Upsert(ctx, values, conflictColumns, updateColumns)
}

func (r *Repository[T]) GetByID(ctx context.Context, id string) (T, error) {
	return r.impl().GetByID(ctx, id)
}

func (r *Repository[T]) GetOneByField(ctx context.Context, field string, value any) (T, error) {
	return r.impl().GetOneByField(ctx, field, value)
}

func (r *Repository[T]) GetManyByField(ctx context.Context, field string, value any) ([]T, error) {
	return r.impl().GetManyByField(ctx, field, value)
}

func (r *Repository[T]) ExistsByField(ctx context.Context, field string, value any) (bool, error) {
	return r.impl().ExistsByField(ctx, field, value)
}

func (r *Repository[T]) ExistsByFields(ctx context.Context, filters map[string]any) (bool, error) {
	return r.impl().ExistsByFields(ctx, filters)
}

func (r *Repository[T]) GetAll(ctx context.Context, params BaseParams, opts QueryOptions) ([]T, int64, error) {
	return r.impl().GetAll(ctx, params, opts)
}

func (r *Repository[T]) Update(ctx context.Context, m T) error { return r.impl().Update(ctx, m) }

func (r *Repository[T]) Delete(ctx context.Context, id string) error { return r.impl().Delete(ctx, id) }

func (r *Repository[T]) SoftDelete(ctx context.Context, id, deletedBy string) error {
	return r.impl().SoftDelete(ctx, id, deletedBy)
}
