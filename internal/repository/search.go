package repository

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// likeEscaper escapes the LIKE metacharacters, backslash first.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// BuildSearchFunc matches the text as an escaped, case-insensitive substring
// (ESCAPE '\') on any column. Bad columns fail the query with ErrInvalidQueryOptions.
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
