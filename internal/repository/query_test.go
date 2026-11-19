package repository

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/gorm"

	"github.com/zazhedho/gokit/internal/filter"
)

// expectList registers a count query and a list query and returns the list
// expectation so a test can pin its SQL and arguments.
func expectList(mock sqlmock.Sqlmock, listSQL string) *sqlmock.ExpectedQuery {
	mock.ExpectQuery(`SELECT count\(\*\) FROM "sample_records"`).WillReturnRows(countRows(1))
	return mock.ExpectQuery(listSQL).WillReturnRows(idRows("1"))
}

func TestGetAllClampsLimitToMaxLimit(t *testing.T) {
	for name, tc := range map[string]struct {
		params  filter.BaseParams
		opts    QueryOptions
		wantSQL string
		args    []driver.Value
	}{
		"default cap":                          {filter.BaseParams{Limit: 500, Page: 1}, QueryOptions{}, `LIMIT \$1`, []driver.Value{100}},
		"custom cap":                           {filter.BaseParams{Limit: 500, Page: 1}, QueryOptions{MaxLimit: 20}, `LIMIT \$1`, []driver.Value{20}},
		"under cap":                            {filter.BaseParams{Limit: 7, Page: 1}, QueryOptions{}, `LIMIT \$1`, []driver.Value{7}},
		"minus one":                            {filter.BaseParams{Limit: -1, Page: 1}, QueryOptions{}, `LIMIT \$1`, []driver.Value{1}},
		"zero":                                 {filter.BaseParams{Limit: 0, Page: 1}, QueryOptions{}, `LIMIT \$1`, []driver.Value{1}},
		"page uses cap":                        {filter.BaseParams{Limit: 500, Page: 3}, QueryOptions{}, `LIMIT \$1 OFFSET \$2`, []driver.Value{100, 200}},
		"negative offset":                      {filter.BaseParams{Limit: 10, Offset: -5}, QueryOptions{}, `LIMIT \$1$`, []driver.Value{10}},
		"offset w/o page":                      {filter.BaseParams{Limit: 10, Offset: 30}, QueryOptions{}, `LIMIT \$1 OFFSET \$2`, []driver.Value{10, 30}},
		"allow-all keeps cap for other limits": {filter.BaseParams{Limit: 500, Page: 1}, QueryOptions{AllowAll: true}, `LIMIT \$1`, []driver.Value{100}},
	} {
		t.Run(name, func(t *testing.T) {
			db, mock := newMockDB(t, nil)
			expectList(mock, tc.wantSQL).WithArgs(tc.args...)
			_, _, err := NewRepository[sampleRecord](db).GetAll(context.Background(), tc.params, tc.opts)
			if err != nil {
				t.Fatalf("GetAll: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestGetAllHonorsMinusOneOnlyWithAllowAll(t *testing.T) {
	db, mock := newMockDB(t, nil)
	mock.ExpectQuery(`SELECT count\(\*\)`).WillReturnRows(countRows(3))
	mock.ExpectQuery(`^SELECT \* FROM "sample_records" ORDER BY .*"sample_records"."id"$`).WillReturnRows(idRows("1", "2", "3"))
	rows, total, err := NewRepository[sampleRecord](db).GetAll(context.Background(),
		filter.BaseParams{Limit: -1, Page: 5, Offset: 40}, QueryOptions{AllowAll: true})
	if err != nil || total != 3 || len(rows) != 3 {
		t.Fatalf("rows=%d total=%d err=%v", len(rows), total, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGetAllGuardsOffsetOverflow(t *testing.T) {
	db, mock := newMockDB(t, nil)
	expectList(mock, `LIMIT \$1 OFFSET \$2`).WithArgs(100, int64(math.MaxInt))
	_, _, err := NewRepository[sampleRecord](db).GetAll(context.Background(),
		filter.BaseParams{Limit: 100, Page: math.MaxInt}, QueryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGetAllAppendsPrimaryKeyTieBreaker(t *testing.T) {
	for name, tc := range map[string]struct {
		params filter.BaseParams
		opts   QueryOptions
		want   string
	}{
		"requested order": {
			filter.BaseParams{OrderBy: "name", OrderDirection: "desc", Limit: 5, Page: 1},
			QueryOptions{AllowedOrderColumns: []string{"name"}},
			`ORDER BY "name" DESC,"sample_records"."id"`,
		},
		"default orders": {
			filter.BaseParams{Limit: 5, Page: 1},
			QueryOptions{DefaultOrders: []string{"status asc", "name desc"}},
			`ORDER BY "status","name" DESC,"sample_records"."id"`,
		},
		"no order": {
			filter.BaseParams{Limit: 5, Page: 1}, QueryOptions{},
			`ORDER BY "sample_records"."id"`,
		},
		"already ordered by key": {
			filter.BaseParams{OrderBy: "id", OrderDirection: "desc", Limit: 5, Page: 1},
			QueryOptions{AllowedOrderColumns: []string{"id"}},
			`ORDER BY "id" DESC LIMIT`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			db, mock := newMockDB(t, nil)
			expectList(mock, regexp.QuoteMeta(tc.want))
			if _, _, err := NewRepository[sampleRecord](db).GetAll(context.Background(), tc.params, tc.opts); err != nil {
				t.Fatal(err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestGetAllOrderErrorsAreSentinelsWithoutEcho(t *testing.T) {
	repo := NewRepository[sampleRecord](newDryRunDB(t))
	for name, tc := range map[string]struct {
		params filter.BaseParams
		opts   QueryOptions
		want   error
		leak   string
	}{
		"column": {
			filter.BaseParams{OrderBy: "secret_column", OrderDirection: "asc", Limit: 1},
			QueryOptions{AllowedOrderColumns: []string{"name"}}, ErrInvalidOrderColumn, "secret_column",
		},
		"direction": {
			filter.BaseParams{OrderBy: "name", OrderDirection: "sideways", Limit: 1},
			QueryOptions{AllowedOrderColumns: []string{"name"}}, ErrInvalidOrderDirection, "sideways",
		},
	} {
		_, _, err := repo.GetAll(context.Background(), tc.params, tc.opts)
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: got %v, want %v", name, err, tc.want)
			continue
		}
		if strings.Contains(err.Error(), tc.leak) {
			t.Errorf("%s: error echoes input: %v", name, err)
		}
	}
}

func TestQueryOptionsValidate(t *testing.T) {
	if err := (QueryOptions{
		AllowedFilters: []string{"status", "t.name"}, AllowedOrderColumns: []string{"name"},
		DefaultOrders: []string{"name asc"}, MaxLimit: 10, MaxSearchLength: 5,
	}).Validate(); err != nil {
		t.Fatalf("valid options rejected: %v", err)
	}
	for name, opts := range map[string]QueryOptions{
		"filter key":      {AllowedFilters: []string{"status", "x) OR 1=1"}},
		"order column":    {AllowedOrderColumns: []string{"name;"}},
		"default order":   {DefaultOrders: []string{"name sideways"}},
		"default shape":   {DefaultOrders: []string{"name"}},
		"negative limit":  {MaxLimit: -1},
		"negative search": {MaxSearchLength: -1},
	} {
		if err := opts.Validate(); !errors.Is(err, ErrInvalidQueryOptions) {
			t.Errorf("%s: got %v, want ErrInvalidQueryOptions", name, err)
		}
	}

	_, _, err := NewRepository[sampleRecord](newDryRunDB(t)).GetAll(context.Background(),
		filter.BaseParams{Limit: 1}, QueryOptions{AllowedFilters: []string{"bad key"}})
	if !errors.Is(err, ErrInvalidQueryOptions) {
		t.Fatalf("GetAll must check Validate, got %v", err)
	}
}

func TestSanitizerOutputOutsideAllowListIsAnError(t *testing.T) {
	repo := NewRepository[sampleRecord](newDryRunDB(t))
	leaky := func(filters map[string]any, _ []string) map[string]any { return filters }
	_, _, err := repo.GetAll(context.Background(),
		filter.BaseParams{Limit: 1, Filters: map[string]any{"status": "a", "password": "x"}},
		QueryOptions{AllowedFilters: []string{"status"}, FilterSanitizer: leaky})
	if !errors.Is(err, ErrInvalidQueryOptions) {
		t.Fatalf("got %v", err)
	}
}

func TestGetAllFilterValues(t *testing.T) {
	big := json.Number("12345678901234567890")
	for name, tc := range map[string]struct {
		filters map[string]any
		sql     string
		args    []driver.Value
	}{
		"string":      {map[string]any{"status": "active"}, `WHERE "status" = \$1 `, []driver.Value{"active"}},
		"small int":   {map[string]any{"status": json.Number("42")}, `WHERE "status" = \$1 `, []driver.Value{int64(42)}},
		"float":       {map[string]any{"status": json.Number("1.50")}, `WHERE "status" = \$1 `, []driver.Value{1.5}},
		"huge number": {map[string]any{"status": big}, `WHERE "status" = \$1 `, []driver.Value{"12345678901234567890"}},
		"in list":     {map[string]any{"status": []any{"a", json.Number("2")}}, `WHERE "status" IN \(\$1,\$2\)`, []driver.Value{"a", int64(2)}},
		"typed list":  {map[string]any{"status": []string{"a", "b"}}, `WHERE "status" IN \(\$1,\$2\)`, []driver.Value{"a", "b"}},
		"null":        {map[string]any{"status": nil}, `FROM "sample_records" ORDER`, nil},
		"blank":       {map[string]any{"status": "  "}, `FROM "sample_records" ORDER`, nil},
		"bool":        {map[string]any{"status": true}, `WHERE "status" = \$1 `, []driver.Value{true}},
	} {
		t.Run(name, func(t *testing.T) {
			db, mock := newMockDB(t, nil)
			mock.ExpectQuery(`SELECT count\(\*\)`).WillReturnRows(countRows(0))
			exp := mock.ExpectQuery(tc.sql).WillReturnRows(idRows())
			if tc.args != nil {
				exp.WithArgs(append(tc.args, 5)...)
			}
			_, _, err := NewRepository[sampleRecord](db).GetAll(context.Background(),
				filter.BaseParams{Limit: 5, Page: 1, Filters: tc.filters},
				QueryOptions{AllowedFilters: []string{"status"}})
			if err != nil {
				t.Fatal(err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestGetAllRejectsObjectAndOversizedFilterValues(t *testing.T) {
	repo := NewRepository[sampleRecord](newDryRunDB(t))
	tooMany := make([]any, 101)
	for name, value := range map[string]any{
		"map":         map[string]any{"$ne": 1},
		"typed map":   map[string]string{"a": "b"},
		"nested map":  []any{map[string]any{"a": 1}},
		"nested list": []any{[]any{1}},
		"too many":    tooMany,
	} {
		_, _, err := repo.GetAll(context.Background(),
			filter.BaseParams{Limit: 1, Filters: map[string]any{"status": value}},
			QueryOptions{AllowedFilters: []string{"status"}})
		if !errors.Is(err, filter.ErrInvalidFilter) {
			t.Errorf("%s: got %v, want ErrInvalidFilter", name, err)
		}
	}
}

func TestGetAllGroupsCustomQueriesSoOrCannotBreakScoping(t *testing.T) {
	scopeWithOr := func(q *gorm.DB) *gorm.DB {
		return q.Where("tenant_id = ?", 7).Or("shared = ?", true)
	}
	searchWithOr := func(q *gorm.DB, s string) *gorm.DB {
		return q.Where("name = ?", s).Or("alias = ?", s)
	}

	db, mock := newMockDB(t, nil)
	mock.ExpectQuery(`SELECT count\(\*\) FROM "sample_records" WHERE \(tenant_id = \$1 OR shared = \$2\) AND \(name = \$3 OR alias = \$4\) AND "status" = \$5`).
		WillReturnRows(countRows(0))
	mock.ExpectQuery(`WHERE \(tenant_id = \$1 OR shared = \$2\) AND \(name = \$3 OR alias = \$4\) AND "status" = \$5 `).
		WillReturnRows(idRows())
	_, _, err := NewRepository[sampleRecord](db).GetAll(context.Background(),
		filter.BaseParams{Limit: 5, Page: 1, Search: "x", Filters: map[string]any{"status": "a"}},
		QueryOptions{BaseQuery: scopeWithOr, Search: searchWithOr, AllowedFilters: []string{"status"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGetAllSurfacesErrorsAddedByCustomFuncs(t *testing.T) {
	boom := errors.New("boom")
	_, _, err := NewRepository[sampleRecord](newDryRunDB(t)).GetAll(context.Background(),
		filter.BaseParams{Limit: 1}, QueryOptions{BaseQuery: func(q *gorm.DB) *gorm.DB { _ = q.AddError(boom); return q }})
	if !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
}
