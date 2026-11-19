package repository

import (
	"errors"

	"gorm.io/gorm"
)

var (
	// ErrNotFound is returned when no row matched: by GetByID, GetOneByField,
	// Update, Delete and SoftDelete. It is gorm.ErrRecordNotFound, so
	// errors.Is works with either name.
	ErrNotFound = gorm.ErrRecordNotFound

	// ErrInvalidOrderColumn means the requested order column is not in
	// AllowedOrderColumns or is not a valid identifier. It is caused by client
	// input; handlers can answer 400. The error text never echoes the input.
	ErrInvalidOrderColumn = errors.New("invalid order column")

	// ErrInvalidOrderDirection means the requested direction is neither asc nor
	// desc. Handlers can answer 400.
	ErrInvalidOrderDirection = errors.New("invalid order direction")

	// ErrInvalidQueryOptions reports a server-side misconfiguration: invalid
	// QueryOptions, an invalid search column or key column, or a filter sanitizer
	// that returned a key outside AllowedFilters. Answer 500, not 400.
	ErrInvalidQueryOptions = errors.New("invalid query options")
)
