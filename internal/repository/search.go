package repository

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// likeEscaper escapes the LIKE metacharacters, backslash first.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// BuildSearchFunc returns a SearchFunc that matches the search text, case
// insensitively and as a plain substring, against any of columns. The text is
// escaped, so "%", "_" and "\" match themselves, and the condition carries
// ESCAPE '\'. Columns are quoted identifiers ("name" or "table.name").
//
// With no columns, or any invalid column, the returned func fails the query with
// an error wrapping ErrInvalidQueryOptions instead of searching nothing.
func BuildSearchFunc(columns ...string) SearchFunc {
	var cfgErr error
	switch {
	case len(columns) == 0:
		cfgErr = fmt.Errorf("%w: BuildSearchFunc needs at least one column", ErrInvalidQueryOptions)
	default:
		for _, column := range columns {
			if !isSafeColumnIdentifier(column) {
				cfgErr = fmt.Errorf("%w: search column %q is not a valid column", ErrInvalidQueryOptions, column)
				break
			}
		}
	}
	columns = append([]string(nil), columns...)

	return func(query *gorm.DB, search string) *gorm.DB {
		if cfgErr != nil {
			_ = query.AddError(cfgErr)
			return query
		}

		pattern := "%" + likeEscaper.Replace(search) + "%"
		terms := make([]string, len(columns))
		vars := make([]any, 0, 2*len(columns))
		for i, column := range columns {
			terms[i] = `LOWER(?) LIKE LOWER(?) ESCAPE '\'`
			vars = append(vars, columnOf(column), pattern)
		}
		return query.Where(clause.Expr{SQL: "(" + strings.Join(terms, " OR ") + ")", Vars: vars})
	}
}
