package repository

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gorm.io/gorm"

	"github.com/zazhedho/gokit/internal/filter"
)

func TestBuildSearchFuncEscapesLikeWildcards(t *testing.T) {
	for _, tc := range []struct{ search, pattern string }{
		{"plain", `%plain%`},
		{"50%", `%50\%%`},
		{"a_b", `%a\_b%`},
		{`back\slash`, `%back\\slash%`},
		{`%_\`, `%\%\_\\%`},
	} {
		db, mock := newMockDB(t, nil)
		expectList(mock, `LOWER\("name"\) LIKE LOWER\(\$1\) ESCAPE '\\' OR LOWER\("status"\) LIKE LOWER\(\$2\) ESCAPE '\\'`).
			WithArgs(tc.pattern, tc.pattern, 5)
		_, _, err := NewRepository[sampleRecord](db).GetAll(context.Background(),
			filter.BaseParams{Limit: 5, Page: 1, Search: tc.search},
			QueryOptions{Search: BuildSearchFunc("name", "status")})
		if err != nil {
			t.Fatalf("%q: %v", tc.search, err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("%q: %v", tc.search, err)
		}
	}
}

func TestBuildSearchFuncGroupsItsColumns(t *testing.T) {
	db, mock := newMockDB(t, nil)
	mock.ExpectQuery(`WHERE \(+LOWER\("name"\).*OR LOWER\("status"\).*\)+ AND "status" = \$3`).WillReturnRows(countRows(0))
	mock.ExpectQuery(`WHERE \(+LOWER`).WillReturnRows(idRows())
	_, _, err := NewRepository[sampleRecord](db).GetAll(context.Background(),
		filter.BaseParams{Limit: 5, Page: 1, Search: "x", Filters: map[string]any{"status": "a"}},
		QueryOptions{Search: BuildSearchFunc("name", "status"), AllowedFilters: []string{"status"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSearchLengthIsCapped(t *testing.T) {
	long := strings.Repeat("é", 300)
	for name, tc := range map[string]struct {
		max  int
		want int
	}{"default": {0, 100}, "custom": {10, 10}} {
		db, mock := newMockDB(t, nil)
		var got string
		mock.ExpectQuery(`SELECT count\(\*\)`).WillReturnRows(countRows(0))
		mock.ExpectQuery(`SELECT`).WillReturnRows(idRows())
		_, _, err := NewRepository[sampleRecord](db).GetAll(context.Background(),
			filter.BaseParams{Limit: 5, Page: 1, Search: long},
			QueryOptions{MaxSearchLength: tc.max, Search: func(q *gorm.DB, s string) *gorm.DB { got = s; return q }})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if n := len([]rune(got)); n != tc.want {
			t.Errorf("%s: search reached the func with %d runes, want %d", name, n, tc.want)
		}
	}
}

func TestBuildSearchFuncMisconfigurationIsAnError(t *testing.T) {
	repo := NewRepository[sampleRecord](newDryRunDB(t))
	for name, search := range map[string]SearchFunc{
		"no columns":         BuildSearchFunc(),
		"only invalid":       BuildSearchFunc("name OR 1=1"),
		"valid and invalid":  BuildSearchFunc("name", "status) --"),
		"blank":              BuildSearchFunc(""),
		"surrounding spaces": BuildSearchFunc(" name"),
	} {
		_, _, err := repo.GetAll(context.Background(),
			filter.BaseParams{Limit: 1, Search: "x"}, QueryOptions{Search: search})
		if !errors.Is(err, ErrInvalidQueryOptions) {
			t.Errorf("%s: got %v, want ErrInvalidQueryOptions", name, err)
		}
	}
}
