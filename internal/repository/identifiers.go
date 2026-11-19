package repository

import (
	"fmt"
	"strings"

	"gorm.io/gorm/clause"
)

// isSafeColumnIdentifier reports whether column is "name" or "table.name" made
// of ASCII letters, digits and '_' (not starting with a digit). The exact string
// is checked: no trimming, so padding cannot slip through.
func isSafeColumnIdentifier(column string) bool {
	parts := strings.Split(column, ".")
	if len(parts) > 2 {
		return false
	}
	for _, part := range parts {
		if !isSafeSegment(part) {
			return false
		}
	}
	return true
}

func isSafeSegment(segment string) bool {
	if segment == "" {
		return false
	}
	for i := 0; i < len(segment); i++ {
		c := segment[i]
		letter := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
		if !letter && (i == 0 || c < '0' || c > '9') {
			return false
		}
	}
	return true
}

func validateColumn(column string) error {
	if !isSafeColumnIdentifier(column) {
		return fmt.Errorf("invalid column: %q", column)
	}
	return nil
}

// columnOf builds a quoted clause.Column from a validated identifier.
func columnOf(column string) clause.Column {
	if table, name, ok := strings.Cut(column, "."); ok {
		return clause.Column{Table: table, Name: name}
	}
	return clause.Column{Name: column}
}

// keyColumn is the key column on the current table, immune to ambiguity when
// BaseQuery joins other tables.
func keyColumn(name string) clause.Column {
	return clause.Column{Table: clause.CurrentTable, Name: name}
}

func whereEq(column string, value any) clause.Expression {
	return clause.Eq{Column: columnOf(column), Value: value}
}

// orderSpec is one validated ORDER BY term.
type orderSpec struct {
	column string
	desc   bool
}

func (o orderSpec) clause() clause.OrderByColumn {
	return clause.OrderByColumn{Column: columnOf(o.column), Desc: o.desc}
}

func parseDirection(direction string) (desc bool, err error) {
	switch strings.ToUpper(strings.TrimSpace(direction)) {
	case "ASC":
		return false, nil
	case "DESC":
		return true, nil
	}
	return false, fmt.Errorf("%w: direction must be asc or desc", ErrInvalidOrderDirection)
}

// parseOrder parses "column asc" or "column desc".
func parseOrder(order string) (orderSpec, error) {
	parts := strings.Fields(order)
	if len(parts) != 2 {
		return orderSpec{}, fmt.Errorf("%w: order must be \"column asc|desc\"", ErrInvalidOrderColumn)
	}
	if !isSafeColumnIdentifier(parts[0]) {
		return orderSpec{}, fmt.Errorf("%w: not a valid identifier", ErrInvalidOrderColumn)
	}
	desc, err := parseDirection(parts[1])
	if err != nil {
		return orderSpec{}, err
	}
	return orderSpec{column: parts[0], desc: desc}, nil
}
