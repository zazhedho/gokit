// Package repository is a generic GORM repository.
package repository

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"

	"github.com/zazhedho/gokit/internal/filter"
)

const (
	defaultKeyColumn   = "id"
	defaultBatchSize   = 500
	deletedAtColumn    = "deleted_at"
	deletedByColumn    = "deleted_by"
	createdAtColumn    = "created_at"
	errRecordRequired  = "record is required"
	errMissingKeyValue = "the record has no value for its key column"
)

// Repository is a generic GORM repository for model type T.
type Repository[T any] struct {
	DB *gorm.DB
	// KeyColumn is the key column used by GetByID, Update, Delete, SoftDelete and
	// the ORDER BY tie-breaker. Empty means "id".
	KeyColumn string
}

// NewRepository returns a Repository for T backed by db, keyed on "id".
func NewRepository[T any](db *gorm.DB) *Repository[T] {
	return &Repository[T]{DB: db}
}

func (r *Repository[T]) key() (string, error) {
	key := r.KeyColumn
	if key == "" {
		key = defaultKeyColumn
	}
	if !isSafeSegment(key) {
		return "", fmt.Errorf("%w: key column %q is not a valid column", ErrInvalidQueryOptions, key)
	}
	return key, nil
}

func (r *Repository[T]) db(ctx context.Context) *gorm.DB { return r.DB.WithContext(ctx) }

func (r *Repository[T]) schema() (*schema.Schema, error) {
	stmt := &gorm.Statement{DB: r.DB}
	if err := stmt.Parse(new(T)); err != nil {
		return nil, err
	}
	return stmt.Schema, nil
}

// notDeleted returns a condition excluding rows deleted through a plain
// deleted_at column. Models using gorm.DeletedAt need none: gorm adds it.
func notDeleted(s *schema.Schema) (clause.Expression, bool) {
	if s.LookUpField(deletedAtColumn) == nil || len(s.QueryClauses) > 0 {
		return nil, false
	}
	return clause.Eq{Column: keyColumn(deletedAtColumn), Value: nil}, true
}

// live scopes reads to rows not marked deleted through a plain deleted_at
// column, so reads agree with Update and SoftDelete.
func (r *Repository[T]) live(query *gorm.DB) (*gorm.DB, error) {
	s, err := r.schema()
	if err != nil {
		return nil, err
	}
	if cond, ok := notDeleted(s); ok {
		query = query.Where(cond)
	}
	return query, nil
}

func (r *Repository[T]) byKey(ctx context.Context, id any) (*gorm.DB, error) {
	key, err := r.key()
	if err != nil {
		return nil, err
	}
	return r.db(ctx).Where(clause.Eq{Column: keyColumn(key), Value: id}), nil
}

// Store inserts m. Generated IDs and database defaults are written back into m.
func (r *Repository[T]) Store(ctx context.Context, m *T) error {
	if m == nil {
		return errors.New(errRecordRequired)
	}
	return r.db(ctx).Create(m).Error
}

// Upsert inserts values and, on a conflict over conflictColumns, updates
// updateColumns from the new row. Large slices are split by the DB's
// CreateBatchSize (500 when unset).
func (r *Repository[T]) Upsert(ctx context.Context, values []T, conflictColumns, updateColumns []string) error {
	if len(values) == 0 {
		return nil
	}
	if len(conflictColumns) == 0 {
		return errors.New("conflict columns are required")
	}
	if len(updateColumns) == 0 {
		return errors.New("update columns are required")
	}
	for _, column := range slices.Concat(conflictColumns, updateColumns) {
		if !isSafeSegment(column) {
			return fmt.Errorf("invalid column: %q (upsert columns must be plain names)", column)
		}
	}

	columns := make([]clause.Column, len(conflictColumns))
	for i, column := range conflictColumns {
		columns[i] = clause.Column{Name: column}
	}
	batch := r.DB.CreateBatchSize
	if batch <= 0 {
		batch = defaultBatchSize
	}

	return r.db(ctx).Clauses(clause.OnConflict{
		Columns:   columns,
		DoUpdates: clause.AssignmentColumns(updateColumns),
	}).CreateInBatches(&values, batch).Error
}

// GetByID returns the row whose key column equals id, or ErrNotFound.
func (r *Repository[T]) GetByID(ctx context.Context, id string) (ret T, err error) {
	query, err := r.byKey(ctx, id)
	if err == nil {
		query, err = r.live(query)
	}
	if err != nil {
		return ret, err
	}
	if err = query.First(&ret).Error; err != nil {
		return zeroValue[T](), err
	}
	return ret, nil
}

// GetOneByField returns the first row where field equals value (IS NULL for a
// nil value), or ErrNotFound.
func (r *Repository[T]) GetOneByField(ctx context.Context, field string, value any) (ret T, err error) {
	if err = validateColumn(field); err != nil {
		return ret, err
	}
	query, err := r.live(r.db(ctx).Where(whereEq(field, value)))
	if err != nil {
		return ret, err
	}
	if err = query.First(&ret).Error; err != nil {
		return zeroValue[T](), err
	}
	return ret, nil
}

// GetManyByField returns up to limit rows where field equals value, ordered by
// order ("column asc|desc", empty for the key column) and then by the key. limit
// is clamped to 1..DefaultMaxLimit.
func (r *Repository[T]) GetManyByField(ctx context.Context, field string, value any, limit int, order string) (ret []T, err error) {
	if err = validateColumn(field); err != nil {
		return nil, err
	}
	key, err := r.key()
	if err != nil {
		return nil, err
	}

	query, err := r.live(r.db(ctx).Where(whereEq(field, value)))
	if err != nil {
		return nil, err
	}
	if order != "" {
		spec, err := parseOrder(order)
		if err != nil {
			return nil, err
		}
		query = query.Order(spec.clause())
	}
	query = query.Order(clause.OrderByColumn{Column: keyColumn(key)})

	if limit < 1 || limit > DefaultMaxLimit {
		limit = DefaultMaxLimit
	}
	if err = query.Limit(limit).Find(&ret).Error; err != nil {
		return nil, err
	}
	return ret, nil
}

// ExistsByField reports whether any row has field equal to value (IS NULL for
// nil).
func (r *Repository[T]) ExistsByField(ctx context.Context, field string, value any) (bool, error) {
	return r.ExistsByFields(ctx, map[string]any{field: value})
}

// ExistsByFields reports whether any row matches every field/value pair. An
// empty map is an error: it would match every row.
func (r *Repository[T]) ExistsByFields(ctx context.Context, filters map[string]any) (bool, error) {
	if len(filters) == 0 {
		return false, fmt.Errorf("%w: at least one field is required", filter.ErrInvalidFilter)
	}
	keys := make([]string, 0, len(filters))
	for key := range filters {
		if err := validateColumn(key); err != nil {
			return false, err
		}
		keys = append(keys, key)
	}
	slices.Sort(keys)

	query, err := r.live(r.db(ctx).Model(new(T)))
	if err != nil {
		return false, err
	}
	for _, key := range keys {
		query = query.Where(whereEq(key, filters[key]))
	}

	var hit []int
	if err := query.Select("1").Limit(1).Scan(&hit).Error; err != nil {
		return false, err
	}
	return len(hit) > 0, nil
}

// GetAll returns one page of rows and the total number of rows matching the
// base query, search and filters.
//
// The pipeline is: options validation, BaseQuery, search (truncated to
// MaxSearchLength), allow-listed filters, count, ordering (requested or default,
// then the key column as tie-breaker), then limit and offset. The limit is
// clamped to 1..MaxLimit, except limit=-1 with AllowAll, which returns every
// row. When params.Page is at least 1 the offset is derived from Page and the
// effective limit; otherwise params.Offset (never below 0) is used.
func (r *Repository[T]) GetAll(ctx context.Context, params filter.BaseParams, opts QueryOptions) (ret []T, totalData int64, err error) {
	if err = opts.Validate(); err != nil {
		return nil, 0, err
	}
	key, err := r.key()
	if err != nil {
		return nil, 0, err
	}

	sch, err := r.schema()
	if err != nil {
		return nil, 0, err
	}
	query, err := r.live(r.db(ctx).Model(new(T)))
	if err != nil {
		return nil, 0, err
	}
	if opts.BaseQuery != nil {
		query = scoped(query, opts.BaseQuery)
	}
	if params.Search != "" && opts.Search != nil {
		search := truncateRunes(params.Search, opts.maxSearchLength())
		query = scoped(query, func(q *gorm.DB) *gorm.DB { return opts.Search(q, search) })
	}
	if query == nil {
		return nil, 0, fmt.Errorf("%w: BaseQuery and Search must return a query", ErrInvalidQueryOptions)
	}
	if query, err = applyFilters(query, params.Filters, opts); err != nil {
		return nil, 0, err
	}
	if err = query.Error; err != nil {
		return nil, 0, err
	}

	base := query.Session(&gorm.Session{})
	if err = base.Count(&totalData).Error; err != nil {
		return nil, 0, err
	}

	page, err := applyOrdering(base, params, opts, sch.Table, key)
	if err != nil {
		return nil, 0, err
	}
	if limit, offset, all := effectivePaging(params, opts); !all {
		page = page.Limit(limit)
		if offset > 0 {
			page = page.Offset(offset)
		}
	}
	if err = page.Find(&ret).Error; err != nil {
		return nil, 0, err
	}
	return ret, totalData, nil
}

func truncateRunes(s string, n int) string {
	if len(s) <= n { // fewer bytes than the cap means fewer runes too
		return s
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}

// Update writes every column of m except the key, created_at, deleted_at,
// deleted_by and associations, to the row whose key equals m's key. Zero values
// are written. It returns ErrNotFound when no live row matched, so a missing or
// soft-deleted row is never created or resurrected, and an error when m has no
// key value. Note that MySQL reports unchanged rows as unmatched.
func (r *Repository[T]) Update(ctx context.Context, m *T) error {
	if m == nil {
		return errors.New(errRecordRequired)
	}
	key, err := r.key()
	if err != nil {
		return err
	}
	s, err := r.schema()
	if err != nil {
		return err
	}
	keyField := s.LookUpField(key)
	if keyField == nil {
		return fmt.Errorf("%w: key column %q is not a column of %s", ErrInvalidQueryOptions, key, s.Table)
	}
	keyValue, zero := keyField.ValueOf(ctx, reflect.ValueOf(m).Elem())
	if zero {
		return errors.New(errMissingKeyValue)
	}

	omit := []string{clause.Associations, keyField.DBName}
	for _, column := range []string{createdAtColumn, deletedAtColumn, deletedByColumn} {
		if s.LookUpField(column) != nil {
			omit = append(omit, column)
		}
	}
	writable := 0
	for _, name := range s.DBNames {
		if f := s.LookUpField(name); !slices.Contains(omit, name) && f != nil && f.Updatable {
			writable++
		}
	}
	if writable == 0 {
		return errors.New("update has no writable columns besides the key")
	}

	query := r.db(ctx).Model(new(T)).Select("*").Omit(omit...).
		Where(clause.Eq{Column: keyColumn(key), Value: keyValue})
	if cond, ok := notDeleted(s); ok {
		query = query.Where(cond)
	}

	res := query.Updates(m)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes the row whose key equals id and returns ErrNotFound when none
// matched. It follows gorm: a model with a gorm.DeletedAt field is soft-deleted
// (deleted_at is set and later queries skip the row), any other model is
// deleted for real. Use SoftDelete to mark rows that keep a plain deleted_at
// column.
func (r *Repository[T]) Delete(ctx context.Context, id string) error {
	query, err := r.byKey(ctx, id)
	if err != nil {
		return err
	}
	res := query.Delete(new(T))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// SoftDelete sets deleted_by and deleted_at (in UTC) on the live row whose key
// equals id, and returns ErrNotFound when no live row matched, so deleting twice
// fails the second time. The model needs deleted_at and deleted_by columns.
func (r *Repository[T]) SoftDelete(ctx context.Context, id, deletedBy string) error {
	query, err := r.byKey(ctx, id)
	if err != nil {
		return err
	}
	s, err := r.schema()
	if err != nil {
		return err
	}
	if cond, ok := notDeleted(s); ok {
		query = query.Where(cond)
	}

	res := query.Model(new(T)).Updates(map[string]any{
		deletedByColumn: deletedBy,
		deletedAtColumn: time.Now().UTC(),
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func zeroValue[T any]() T {
	var zero T
	return zero
}
