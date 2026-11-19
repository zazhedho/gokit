package gokit

import (
	"net/url"

	"github.com/zazhedho/gokit/internal/filter"
	"github.com/zazhedho/gokit/internal/repository"
	"github.com/zazhedho/gokit/internal/utils"
)

// Defaults applied by Repository.GetAll when the QueryOptions fields are zero.
const (
	// DefaultMaxLimit is the page size cap (QueryOptions.MaxLimit).
	DefaultMaxLimit = repository.DefaultMaxLimit
	// DefaultMaxSearchLength is the search length cap in runes
	// (QueryOptions.MaxSearchLength).
	DefaultMaxSearchLength = repository.DefaultMaxSearchLength
)

var (
	// ErrNotFound is returned when no row matched: by GetByID, GetOneByField,
	// Update, Delete and SoftDelete. It is gorm.ErrRecordNotFound, so errors.Is
	// works with either name.
	ErrNotFound = repository.ErrNotFound

	// ErrInvalidOrderColumn means the requested order column is not allowed. It is
	// caused by client input, so handlers can answer 400. The error text never
	// echoes the input.
	ErrInvalidOrderColumn = repository.ErrInvalidOrderColumn

	// ErrInvalidOrderDirection means the requested direction is neither asc nor
	// desc. Handlers can answer 400.
	ErrInvalidOrderDirection = repository.ErrInvalidOrderDirection

	// ErrInvalidFilter is returned, wrapped with %w, for filter input that is not
	// acceptable: an object value, a nested array, more than 100 array items, or
	// an empty ExistsByFields map. Handlers can map it to a 400. The error text
	// never echoes the input.
	ErrInvalidFilter = filter.ErrInvalidFilter

	// ErrInvalidQueryOptions reports a server-side misconfiguration: invalid
	// QueryOptions, search column or key column, or a filter sanitizer that
	// returned a key outside AllowedFilters. Answer 500, not 400.
	ErrInvalidQueryOptions = repository.ErrInvalidQueryOptions
)

// BaseParams holds the common list-query parameters (search, filters, paging,
// ordering). Build it with GetBaseParams.
type BaseParams = filter.BaseParams

// FilterSanitizer reduces untrusted filters to the allowed keys.
type FilterSanitizer = repository.FilterSanitizer

// QueryFunc customizes a GORM query.
type QueryFunc = repository.QueryFunc

// QueryOptions configures Repository.GetAll: base scope, search, filter and
// order allow-lists, and the MaxLimit and AllowAll paging guards. Validate
// checks it.
type QueryOptions = repository.QueryOptions

// SearchFunc applies a free-text search to a GORM query.
type SearchFunc = repository.SearchFunc

// GetBaseParams parses search, filters, paging and ordering from query. A
// missing, empty, zero or negative (other than -1) limit becomes defLimit (>= 1);
// -1 passes through. A bad page or limit returns an error and a safe struct.
func GetBaseParams(query url.Values, defOrderBy, defOrderDirection string, defLimit int) (req BaseParams, err error) {
	return filter.GetBaseParams(query, defOrderBy, defOrderDirection, defLimit)
}

// WhitelistFilter keeps only the allowed filter keys.
func WhitelistFilter(filters map[string]any, allowed []string) map[string]any {
	return filter.WhitelistFilter(filters, allowed)
}

// WhitelistStringFilter keeps only the allowed filter keys and converts their
// values to strings (see InterfaceString).
func WhitelistStringFilter(filters map[string]any, allowed []string) map[string]any {
	return filter.WhitelistStringFilter(filters, allowed)
}

// ConvertValuesToString returns a copy of filters with the values of keys
// converted to strings (all values when no keys are given).
func ConvertValuesToString(filters map[string]any, keys ...string) map[string]any {
	return utils.ConvertValuesToString(filters, keys...)
}

// BuildSearchFunc returns a SearchFunc that matches the search text, case
// insensitively and as a plain substring, against any of columns ("name" or
// "table.name"). "%", "_" and "\" in the text match themselves. With no columns,
// or an invalid one, the query fails with ErrInvalidQueryOptions instead of
// silently searching nothing.
func BuildSearchFunc(columns ...string) SearchFunc { return repository.BuildSearchFunc(columns...) }
