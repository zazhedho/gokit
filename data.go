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

// GetBaseParams parses list-query parameters, applying the given defaults.
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
