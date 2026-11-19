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
	if result == nil || result.Statement != query.Statement {
		return result
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
		if rv.Len() > filter.MaxFilterItems {
			return nil, false, fmt.Errorf("%w: at most %d values are allowed", filter.ErrInvalidFilter, filter.MaxFilterItems)
		}
		values := make([]any, rv.Len())
		for i := range values {
			item := rv.Index(i).Interface()
			if k := reflect.ValueOf(item).Kind(); k == reflect.Map || k == reflect.Slice || k == reflect.Array {
				if _, isBytes := item.([]byte); !isBytes {
					return nil, false, fmt.Errorf("%w: array items must be scalars", filter.ErrInvalidFilter)
				}
			}
			values[i] = bindValue(item)
		}
		return clause.IN{Column: columnOf(key), Values: values}, true, nil
	}
	return whereEq(key, bindValue(value)), true, nil
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

func applyOrdering(query *gorm.DB, params filter.BaseParams, opts QueryOptions, key string) (*gorm.DB, error) {
	var orders []orderSpec

	if params.OrderBy != "" && params.OrderDirection != "" {
		if !slices.Contains(opts.AllowedOrderColumns, params.OrderBy) || !isSafeColumnIdentifier(params.OrderBy) {
			return nil, fmt.Errorf("%w: column is not in AllowedOrderColumns", ErrInvalidOrderColumn)
		}
		desc, err := parseDirection(params.OrderDirection)
		if err != nil {
			return nil, err
		}
		orders = append(orders, orderSpec{column: params.OrderBy, desc: desc})
	} else {
		for _, order := range opts.DefaultOrders {
			spec, err := parseOrder(order)
			if err != nil {
				return nil, fmt.Errorf("%w: %w", ErrInvalidQueryOptions, err)
			}
			orders = append(orders, spec)
		}
	}

	hasKey := false
	for _, o := range orders {
		query = query.Order(o.clause())
		if _, name, ok := strings.Cut(o.column, "."); (ok && name == key) || o.column == key {
			hasKey = true
		}
	}
	if !hasKey {
		query = query.Order(clause.OrderByColumn{Column: keyColumn(key)})
	}
	return query, nil
}

// effectivePaging resolves the limit and offset GetAll uses. all is true for
// limit=-1 with AllowAll, which means no LIMIT and no OFFSET.
func effectivePaging(params filter.BaseParams, opts QueryOptions) (limit, offset int, all bool) {
	if opts.AllowAll && params.Limit == -1 {
		return 0, 0, true
	}
	limit = min(max(params.Limit, 1), opts.maxLimit())
	if params.Page >= 1 {
		return limit, filter.SaturatingOffset(params.Page, limit), false
	}
	return limit, max(params.Offset, 0), false
}
