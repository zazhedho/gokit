package filter

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"strconv"
	"strings"

	"github.com/zazhedho/gokit/internal/utils"
)

// MaxFilterItems caps the number of elements in an array filter value.
const MaxFilterItems = 100

const maxFilterItems = MaxFilterItems

// ErrInvalidFilter is returned (wrapped) when a filter value is not acceptable:
// an object, a nested array, an array with too many items, or a filter key
// outside the allow-list. The error text never echoes the offending input.
var ErrInvalidFilter = errors.New("invalid filter")

// BaseParams holds the common list-query parameters.
type BaseParams struct {
	// Search is free text, applied by QueryOptions.Search.
	Search string `json:"search" form:"search"`
	// Filters maps filter names to scalar or array-of-scalar values.
	Filters map[string]any `json:"filters" form:"filters"`
	// OrderBy and OrderDirection select the ordering.
	OrderBy        string `json:"order_by" form:"order_by"`
	OrderDirection string `json:"order_direction" form:"order_direction"`
	// Page is 1-based. When it is at least 1, the repository derives the offset
	// from Page and the effective limit.
	Page int `json:"page" form:"page"`
	// Limit is the requested page size; -1 asks for every row, which the
	// repository grants only with QueryOptions.AllowAll.
	Limit int `json:"limit" form:"limit"`
	// Offset is the row offset; the repository uses it only when Page is below 1.
	Offset int `json:"offset" form:"offset"`
}

// GetBaseParams parses search, filters, paging and ordering from query.
// defLimit, which must be at least 1, replaces a missing or non-positive limit.
// A limit above the repository cap is left as is; the repository clamps it.
// limit=-1 is passed through with page 1 and offset 0. A non-numeric page or
// limit returns an error, and so does an unacceptable filter value
// (ErrInvalidFilter).
func GetBaseParams(query url.Values, defOrderBy, defOrderDirection string, defLimit int) (req BaseParams, err error) {
	if defLimit < 1 {
		return req, fmt.Errorf("default limit must be at least 1, got %d", defLimit)
	}
	req.Search = query.Get("search")
	req.OrderBy = query.Get("order_by")
	req.OrderDirection = query.Get("order_direction")
	if req.Page, err = parseQueryInt(query, "page"); err != nil {
		return
	}
	if req.Limit, err = parseQueryInt(query, "limit"); err != nil {
		return
	}

	normalizePagination(&req, defLimit)
	if req.OrderBy == "" {
		req.OrderBy = defOrderBy
	}
	if direction := utils.NormalizeKey(req.OrderDirection); direction != "asc" && direction != "desc" {
		req.OrderDirection = defOrderDirection
	}
	req.Filters, err = parseFilters(query)
	return
}

func parseQueryInt(query url.Values, key string) (int, error) {
	values, ok := query[key]
	if !ok {
		return 0, nil
	}
	value := ""
	if len(values) > 0 {
		value = values[0]
	}
	return strconv.Atoi(value)
}

func normalizePagination(req *BaseParams, defaultLimit int) {
	if req.Page < 1 {
		req.Page = 1
	}
	if req.Limit == -1 {
		req.Page = 1
		req.Offset = 0
		return
	}
	if req.Limit < 1 {
		req.Limit = defaultLimit
	}
	req.Offset = SaturatingOffset(req.Page, req.Limit)
}

// SaturatingOffset returns (page-1)*limit, or math.MaxInt when that would
// overflow, so a huge page asks for an empty slice instead of a negative offset.
func SaturatingOffset(page, limit int) int {
	if page <= 1 || limit <= 0 {
		return 0
	}
	if page-1 > math.MaxInt/limit {
		return math.MaxInt
	}
	return (page - 1) * limit
}

func parseFilters(query url.Values) (map[string]any, error) {
	filters := make(map[string]any)
	for key, values := range query {
		if !strings.HasPrefix(key, "filters[") || !strings.HasSuffix(key, "]") {
			continue
		}
		value := ""
		if len(values) > 0 {
			value = values[0]
		}
		filterKey := strings.TrimSuffix(strings.TrimPrefix(key, "filters["), "]")
		decoded, ok := decodeFilterValue(value)
		if !ok {
			filters[filterKey] = value
			continue
		}
		if err := checkFilterValue(decoded); err != nil {
			return nil, err
		}
		filters[filterKey] = decoded
	}
	return filters, nil
}

// decodeFilterValue decodes value as one JSON document, keeping numbers as
// json.Number so they retain their written form. ok is false when value is not
// valid JSON (it is then used as a plain string).
func decodeFilterValue(value string) (decoded any, ok bool) {
	dec := json.NewDecoder(strings.NewReader(value))
	dec.UseNumber()
	if err := dec.Decode(&decoded); err != nil {
		return nil, false
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, false
	}
	return decoded, true
}

func checkFilterValue(v any) error {
	switch t := v.(type) {
	case map[string]any:
		return fmt.Errorf("%w: object values are not allowed", ErrInvalidFilter)
	case []any:
		if len(t) > maxFilterItems {
			return fmt.Errorf("%w: at most %d values are allowed", ErrInvalidFilter, maxFilterItems)
		}
		for _, item := range t {
			switch item.(type) {
			case map[string]any, []any:
				return fmt.Errorf("%w: array items must be scalars", ErrInvalidFilter)
			}
		}
	}
	return nil
}

func whitelistTransform(
	filters map[string]any,
	allowed []string,
	transform func(any) any,
) map[string]any {
	if filters == nil {
		return nil
	}

	allowedSet := make(map[string]struct{}, len(allowed))
	for _, k := range allowed {
		allowedSet[k] = struct{}{}
	}

	out := make(map[string]any, len(allowedSet))
	for k, v := range filters {
		if _, ok := allowedSet[k]; ok {
			if transform != nil {
				out[k] = transform(v)
			} else {
				out[k] = v
			}
		}
	}
	return out
}

func WhitelistFilter(filters map[string]any, allowed []string) map[string]any {
	return whitelistTransform(filters, allowed, nil)
}

func WhitelistStringFilter(filters map[string]any, allowed []string) map[string]any {
	return whitelistTransform(filters, allowed, func(v any) any { return utils.ToString(v) })
}
