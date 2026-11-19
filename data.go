package gokit

import (
	"net/url"

	"github.com/zazhedho/gokit/internal/filter"
	"github.com/zazhedho/gokit/internal/repository"
)

// BaseParams holds the common list-query parameters (search, filters, paging, ordering).
type BaseParams = filter.BaseParams

// FilterSanitizer sanitizes filters against an allow-list.
type FilterSanitizer = repository.FilterSanitizer

// QueryFunc customizes a GORM query.
type QueryFunc = repository.QueryFunc

// QueryOptions configures Repository.GetAll.
type QueryOptions = repository.QueryOptions

// SearchFunc applies a free-text search to a GORM query.
type SearchFunc = repository.SearchFunc

// ErrInvalidFilter is returned, wrapped with %w, for filter input that is not
// acceptable: an object value, a nested array, more than 100 array items, or a
// key outside the allow-list. Handlers can map it to a 400. The error text never
// echoes the input.
var ErrInvalidFilter = filter.ErrInvalidFilter

// GetBaseParams parses search, filters, paging and ordering from query.
//
// defLimit must be at least 1 (it replaces a missing or non-positive limit);
// otherwise an error is returned. A limit above the repository cap is left as
// is and clamped by Repository.GetAll. limit=-1 passes through with page 1 and
// offset 0, and is honored by GetAll only when QueryOptions.AllowAll is set.
// Filter values are decoded as JSON when valid, keeping numbers as json.Number
// so large IDs keep their written form, and otherwise kept as strings; objects,
// nested arrays and arrays over 100 items return ErrInvalidFilter. A non-numeric
// page or limit also returns an error; respond with 400.
func GetBaseParams(query url.Values, defOrderBy, defOrderDirection string, defLimit int) (req BaseParams, err error) {
	return filter.GetBaseParams(query, defOrderBy, defOrderDirection, defLimit)
}

// WhitelistFilter keeps only the allowed filter keys.
func WhitelistFilter(filters map[string]any, allowed []string) map[string]any {
	return filter.WhitelistFilter(filters, allowed)
}

// WhitelistStringFilter keeps only the allowed filter keys and converts their values to strings.
func WhitelistStringFilter(filters map[string]any, allowed []string) map[string]any {
	return filter.WhitelistStringFilter(filters, allowed)
}

// BuildSearchFunc returns a SearchFunc matching search against the given columns.
func BuildSearchFunc(columns ...string) SearchFunc { return repository.BuildSearchFunc(columns...) }
