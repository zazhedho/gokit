package repository

import (
	"fmt"
	"slices"

	"gorm.io/gorm"

	"github.com/zazhedho/gokit/internal/filter"
)

const (
	// DefaultMaxLimit is the page size cap used when QueryOptions.MaxLimit is 0.
	DefaultMaxLimit = 100
	// DefaultMaxSearchLength is the search length cap used when
	// QueryOptions.MaxSearchLength is 0.
	DefaultMaxSearchLength = 100
)

// SearchFunc applies a free-text search to a GORM query.
type SearchFunc func(query *gorm.DB, search string) *gorm.DB

// QueryFunc customizes a GORM query.
type QueryFunc func(query *gorm.DB) *gorm.DB

// FilterSanitizer reduces untrusted filters to the allowed keys.
type FilterSanitizer func(filters map[string]any, allowed []string) map[string]any

// QueryOptions configures Repository.GetAll.
type QueryOptions struct {
	// BaseQuery adds joins, scopes or fixed conditions. Its WHERE conditions are
	// grouped in parentheses, so an Or() inside cannot widen the rest of the query.
	BaseQuery QueryFunc
	// Search is applied when params.Search is non-empty, with its WHERE
	// conditions grouped like BaseQuery's. BuildSearchFunc builds one.
	Search SearchFunc
	// AllowedFilters are the filter keys that become conditions. When empty,
	// filters are ignored.
	AllowedFilters []string
	// FilterSanitizer replaces filter.WhitelistFilter. Keys it returns outside
	// AllowedFilters make GetAll fail with ErrInvalidQueryOptions.
	FilterSanitizer FilterSanitizer
	// AllowedOrderColumns are the columns a client may order by.
	AllowedOrderColumns []string
	// DefaultOrders, each "column asc" or "column desc", apply when the request
	// names no order.
	DefaultOrders []string
	// MaxLimit caps the page size (default DefaultMaxLimit). The effective limit
	// is always within 1..MaxLimit, except for limit=-1 with AllowAll.
	MaxLimit int
	// AllowAll honors limit=-1 as "every row". Off by default: without it -1 is
	// clamped like any other out-of-range limit.
	AllowAll bool
	// MaxSearchLength caps the search text in runes (default
	// DefaultMaxSearchLength); longer input is truncated before it reaches Search.
	MaxSearchLength int
}

func (o QueryOptions) maxLimit() int {
	if o.MaxLimit > 0 {
		return o.MaxLimit
	}
	return DefaultMaxLimit
}

func (o QueryOptions) maxSearchLength() int {
	if o.MaxSearchLength > 0 {
		return o.MaxSearchLength
	}
	return DefaultMaxSearchLength
}

// Validate reports a misconfiguration as an error wrapping ErrInvalidQueryOptions:
// a negative MaxLimit or MaxSearchLength, or an AllowedFilters,
// AllowedOrderColumns or DefaultOrders entry that is not a valid identifier or
// "column asc|desc". GetAll calls it on every query.
func (o QueryOptions) Validate() error {
	if o.MaxLimit < 0 || o.MaxSearchLength < 0 {
		return fmt.Errorf("%w: MaxLimit and MaxSearchLength must not be negative", ErrInvalidQueryOptions)
	}
	for _, key := range o.AllowedFilters {
		if !isSafeColumnIdentifier(key) {
			return fmt.Errorf("%w: AllowedFilters entry %q is not a valid column", ErrInvalidQueryOptions, key)
		}
	}
	for _, column := range o.AllowedOrderColumns {
		if !isSafeColumnIdentifier(column) {
			return fmt.Errorf("%w: AllowedOrderColumns entry %q is not a valid column", ErrInvalidQueryOptions, column)
		}
	}
	for _, order := range o.DefaultOrders {
		if _, err := parseOrder(order); err != nil {
			return fmt.Errorf("%w: DefaultOrders entry %q must be \"column asc|desc\"", ErrInvalidQueryOptions, order)
		}
	}
	return nil
}

func (o QueryOptions) allowsFilter(key string) bool { return slices.Contains(o.AllowedFilters, key) }

// PageSize returns the limit GetAll applies for p: MaxLimit when p.Limit is
// below 1 or above MaxLimit, p.Limit otherwise, and -1 for limit=-1 with AllowAll.
func (o QueryOptions) PageSize(p filter.BaseParams) int {
	if o.AllowAll && p.Limit == -1 {
		return -1
	}
	if limit := o.maxLimit(); p.Limit < 1 || p.Limit > limit {
		return limit
	}
	return p.Limit
}
