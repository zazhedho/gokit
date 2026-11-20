package repository

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/zazhedho/gokit/internal/filter"
)

// scoped runs apply on query and wraps the WHERE conditions it added in one
// group, so an Or() inside cannot change how the surrounding conditions bind.
func scoped(query *gorm.DB, apply func(*gorm.DB) *gorm.DB) *gorm.DB {
	before := 0
	if c, ok := query.Statement.Clauses["WHERE"]; ok {
		if w, ok := c.Expression.(clause.Where); ok {
			before = len(w.Exprs)
		}
	}

	result := apply(query)
	if result == nil {
		return nil
	}
	if result.Statement != query.Statement {
		// A new session would escape the grouping, so refuse it.
		_ = query.AddError(fmt.Errorf("%w: BaseQuery and Search must extend the query they receive", ErrInvalidQueryOptions))
		return query
	}
	c, ok := result.Statement.Clauses["WHERE"]
	if !ok {
		return result
	}
	w, ok := c.Expression.(clause.Where)
	if !ok || len(w.Exprs) <= before {
		return result
	}

	added := slices.Clone(w.Exprs[before:])
	grouped := append(slices.Clone(w.Exprs[:before]), clause.AndConditions{Exprs: added})
	c.Expression = clause.Where{Exprs: grouped}
	result.Statement.Clauses["WHERE"] = c
	return result
}

func applyFilters(query *gorm.DB, filters map[string]any, opts QueryOptions) (*gorm.DB, error) {
	if len(opts.AllowedFilters) == 0 {
		return query, nil
	}

	sanitizer := opts.FilterSanitizer
	if sanitizer == nil {
		sanitizer = filter.WhitelistFilter
	}
	safe := sanitizer(filters, opts.AllowedFilters)

	keys := make([]string, 0, len(safe))
	for key := range safe {
		if !opts.allowsFilter(key) {
			return nil, fmt.Errorf("%w: the filter sanitizer returned a key outside AllowedFilters", ErrInvalidQueryOptions)
		}
		keys = append(keys, key)
	}
	slices.Sort(keys)

	for _, key := range keys {
		condition, ok, err := filterCondition(key, safe[key])
		if err != nil {
			return nil, err
		}
		if ok {
			query = query.Where(condition)
		}
	}
	return query, nil
}

// filterCondition builds the condition for one allowed filter. ok is false for
// values that mean "no filter" (nil and blank strings).
func filterCondition(key string, value any) (cond clause.Expression, ok bool, err error) {
	if err := validateColumn(key); err != nil {
		return nil, false, fmt.Errorf("%w: %w", ErrInvalidQueryOptions, err)
	}
	if value == nil {
		return nil, false, nil
	}
	if s, isString := value.(string); isString {
		if strings.TrimSpace(s) == "" {
			return nil, false, nil
		}
		return whereEq(key, s), true, nil
	}

	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Map:
		return nil, false, fmt.Errorf("%w: object values are not allowed", filter.ErrInvalidFilter)
	case reflect.Slice, reflect.Array:
		if rv.Type().Elem().Kind() == reflect.Uint8 {
			break // []byte is a scalar
		}
		cond, err := inCondition(key, rv)
		return cond, err == nil, err
	}
	return whereEq(key, bindValue(value)), true, nil
}

// inCondition builds an IN condition from a slice or array of scalar values.
func inCondition(key string, rv reflect.Value) (clause.Expression, error) {
	if rv.Len() > filter.MaxFilterItems {
		return nil, fmt.Errorf("%w: at most %d values are allowed", filter.ErrInvalidFilter, filter.MaxFilterItems)
	}
	values := make([]any, rv.Len())
	for i := range values {
		item := rv.Index(i).Interface()
		if !isScalarItem(item) {
			return nil, fmt.Errorf("%w: array items must be scalars", filter.ErrInvalidFilter)
		}
		values[i] = bindValue(item)
	}
	return clause.IN{Column: columnOf(key), Values: values}, nil
}

// isScalarItem reports whether item is neither a map nor a slice/array; a
// []byte counts as a scalar.
func isScalarItem(item any) bool {
	switch reflect.ValueOf(item).Kind() {
	case reflect.Map, reflect.Slice, reflect.Array:
		_, isBytes := item.([]byte)
		return isBytes
	}
	return true
}

// bindValue turns a json.Number into the narrowest native value that keeps its
// written form: int64, then float64, else the digits as a string.
func bindValue(v any) any {
	n, ok := v.(json.Number)
	if !ok {
		return v
	}
	if i, err := n.Int64(); err == nil {
		return i
	}
	if strings.ContainsAny(n.String(), ".eE") {
		if f, err := n.Float64(); err == nil && !math.IsInf(f, 0) {
			return f
		}
	}
	return n.String()
}

func applyOrdering(query *gorm.DB, params filter.BaseParams, opts QueryOptions, table, key string) (*gorm.DB, error) {
	orders, err := requestedOrders(params, opts)
	if err != nil {
		return nil, err
	}
	hasKey := false
	for _, o := range orders {
		query = query.Order(o.clause())
		hasKey = hasKey || o.column == key || o.column == table+"."+key
	}
	if !hasKey {
		query = query.Order(clause.OrderByColumn{Column: keyColumn(key)})
	}
	return query, nil
}

// requestedOrders validates the client's order against the allow-list, or
// falls back to the trusted DefaultOrders.
func requestedOrders(params filter.BaseParams, opts QueryOptions) ([]orderSpec, error) {
	if params.OrderBy == "" || params.OrderDirection == "" {
		return defaultOrders(opts)
	}
	if !slices.Contains(opts.AllowedOrderColumns, params.OrderBy) || !isSafeColumnIdentifier(params.OrderBy) {
		return nil, fmt.Errorf("%w: column is not in AllowedOrderColumns", ErrInvalidOrderColumn)
	}
	desc, err := parseDirection(params.OrderDirection)
	if err != nil {
		return nil, err
	}
	return []orderSpec{{column: params.OrderBy, desc: desc}}, nil
}

func defaultOrders(opts QueryOptions) ([]orderSpec, error) {
	orders := make([]orderSpec, 0, len(opts.DefaultOrders))
	for _, order := range opts.DefaultOrders {
		spec, err := parseOrder(order)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrInvalidQueryOptions, err)
		}
		orders = append(orders, spec)
	}
	return orders, nil
}

// effectivePaging resolves limit and offset from PageSize; all means no LIMIT or OFFSET.
func effectivePaging(params filter.BaseParams, opts QueryOptions) (limit, offset int, all bool) {
	limit = opts.PageSize(params)
	if limit == -1 {
		return 0, 0, true
	}
	if params.Page >= 1 {
		return limit, filter.SaturatingOffset(params.Page, limit), false
	}
	return limit, max(params.Offset, 0), false
}
