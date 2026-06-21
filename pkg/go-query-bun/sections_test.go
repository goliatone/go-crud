package querybun

import (
	"context"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

func TestBuildSearchCriteria_SQLiteFallback(t *testing.T) {
	db := setupQueryBunDB(t)
	seedFilterUsers(t, db)

	criteria, search, err := BuildSearchCriteria("ali", Config{
		Fields: FieldConfig{Resolver: NewMapResolver(filterAllowedFields())},
		Search: SearchConfig{Fields: []string{"name", "status"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "ali", search)
	require.NotEmpty(t, criteria)

	query := db.NewSelect().Model((*filterUser)(nil))
	for _, criterion := range criteria {
		query = criterion(query)
	}
	sqlStr := query.String()
	assert.Contains(t, sqlStr, "LOWER(name) LIKE LOWER('%ali%')")
	assert.Contains(t, sqlStr, "LOWER(status) LIKE LOWER('%ali%')")
}

func TestBuildSearchCriteria_StrictColumns(t *testing.T) {
	_, _, err := BuildSearchCriteria("ali", Config{
		Fields:     FieldConfig{Resolver: NewMapResolver(filterAllowedFields())},
		Search:     SearchConfig{Fields: []string{"missing"}},
		Validation: ValidationConfig{Mode: ValidationStrict, StrictSearchFields: true},
	})
	require.Error(t, err)

	var validationErr *ValidationError
	require.ErrorAs(t, err, &validationErr)
	assert.Equal(t, ValidationSearchColumnsRequired, validationErr.Code)
}

func TestBuildOrderCriteria(t *testing.T) {
	criteria, orders := BuildOrderCriteria("name desc,missing asc,age sideways", Config{
		Fields: FieldConfig{Resolver: NewMapResolver(filterAllowedFields())},
	})
	require.NotEmpty(t, criteria)
	assert.Equal(t, []Order{
		{Field: "name", Dir: "DESC"},
		{Field: "age", Dir: "ASC"},
	}, orders)

	db := setupQueryBunDB(t)
	query := db.NewSelect().Model((*filterUser)(nil))
	for _, criterion := range criteria {
		query = criterion(query)
	}
	sqlStr := query.String()
	assert.Contains(t, sqlStr, "ORDER BY name DESC, age ASC")
}

func TestBuildSelectCriteria(t *testing.T) {
	criteria, fields := BuildSelectCriteria([]string{"id,name", "missing"}, Config{
		Fields: FieldConfig{Resolver: NewMapResolver(filterAllowedFields())},
	})
	require.NotEmpty(t, criteria)
	assert.Equal(t, []string{"id", "name"}, fields)

	db := setupQueryBunDB(t)
	query := db.NewSelect().Model((*filterUser)(nil))
	for _, criterion := range criteria {
		query = criterion(query)
	}
	sqlStr := query.String()
	assert.Contains(t, sqlStr, "\"u\".\"id\"")
	assert.Contains(t, sqlStr, "\"u\".\"name\"")
	assert.NotContains(t, sqlStr, "missing")
}

func TestBuildPaginationCriteria(t *testing.T) {
	criteria, limit, offset, page := BuildPaginationCriteria(ListOptions{
		Page:    3,
		PerPage: 10,
	}, Config{})
	require.NotEmpty(t, criteria)
	assert.Equal(t, 10, limit)
	assert.Equal(t, 20, offset)
	assert.Equal(t, 3, page)

	db := setupQueryBunDB(t)
	query := db.NewSelect().Model((*filterUser)(nil))
	for _, criterion := range criteria {
		query = criterion(query)
	}
	sqlStr := query.String()
	assert.Contains(t, sqlStr, "LIMIT 10")
	assert.Contains(t, sqlStr, "OFFSET 20")
}

func TestBuildPaginationCriteria_ExplicitZeroLimit(t *testing.T) {
	_, limit, offset, page := BuildPaginationCriteria(ListOptions{
		Limit:     0,
		LimitSet:  true,
		Offset:    5,
		OffsetSet: true,
	}, Config{})

	assert.Equal(t, 0, limit)
	assert.Equal(t, 5, offset)
	assert.Equal(t, 1, page)
}

func TestBuildPaginationCriteria_ExplicitLimitOffsetWinsOverPage(t *testing.T) {
	_, limit, offset, page := BuildPaginationCriteria(ListOptions{
		Page:      3,
		PerPage:   100,
		Limit:     25,
		LimitSet:  true,
		Offset:    10,
		OffsetSet: true,
	}, Config{})

	assert.Equal(t, 25, limit)
	assert.Equal(t, 10, offset)
	assert.Equal(t, 1, page)
}

func TestBuildPaginationCriteria_ExplicitOffsetUsesDefaultLimit(t *testing.T) {
	_, limit, offset, page := BuildPaginationCriteria(ListOptions{
		Offset:    50,
		OffsetSet: true,
	}, Config{})

	assert.Equal(t, DefaultLimit, limit)
	assert.Equal(t, 50, offset)
	assert.Equal(t, 3, page)
}

func TestPlanSectionHelpers(t *testing.T) {
	filter := Criteria(func(q *bun.SelectQuery) *bun.SelectQuery { return q })
	search := Criteria(func(q *bun.SelectQuery) *bun.SelectQuery { return q })
	order := Criteria(func(q *bun.SelectQuery) *bun.SelectQuery { return q })
	page := Criteria(func(q *bun.SelectQuery) *bun.SelectQuery { return q })
	selectFields := Criteria(func(q *bun.SelectQuery) *bun.SelectQuery { return q })

	plan := Plan{
		Filters:    []Criteria{filter},
		Search:     []Criteria{search},
		Order:      []Criteria{order},
		Pagination: []Criteria{page},
		Select:     []Criteria{selectFields},
	}

	assert.Len(t, plan.WhereCriteria(), 2)
	assertSameCriteria(t, filter, plan.WhereCriteria()[0])
	assertSameCriteria(t, search, plan.WhereCriteria()[1])
	assert.Len(t, plan.CountCriteria(), 2)
	assertSameCriteria(t, filter, plan.CountCriteria()[0])
	assertSameCriteria(t, search, plan.CountCriteria()[1])
	assert.Len(t, plan.PageCriteria(), 1)
	assertSameCriteria(t, page, plan.PageCriteria()[0])
}

func TestCriteriaSectionsExecuteTogether(t *testing.T) {
	db := setupQueryBunDB(t)
	seedFilterUsers(t, db)

	searchCriteria, _, err := BuildSearchCriteria("a", Config{
		Fields: FieldConfig{Resolver: NewMapResolver(filterAllowedFields())},
		Search: SearchConfig{Fields: []string{"name"}},
	})
	require.NoError(t, err)
	orderCriteria, _ := BuildOrderCriteria("age desc", filterConfig())
	paginationCriteria, _, _, _ := BuildPaginationCriteria(ListOptions{Limit: 1}, Config{})

	var rows []filterUser
	query := db.NewSelect().Model(&rows)
	for _, group := range [][]Criteria{searchCriteria, orderCriteria, paginationCriteria} {
		for _, criterion := range group {
			query = criterion(query)
		}
	}
	require.NoError(t, query.Scan(context.Background()))
	require.Len(t, rows, 1)
	assert.Equal(t, "Carol", rows[0].Name)
}

func TestResolveSearchColumnsDedupesTrustedColumns(t *testing.T) {
	columns := ResolveSearchColumns([]string{"name", "name", "status", "missing"}, filterConfig())
	assert.Equal(t, []string{"name", "status"}, columns)
}

func TestBuildSearchCriteria_NoColumnsNoOp(t *testing.T) {
	criteria, search, err := BuildSearchCriteria("ali", Config{})
	require.NoError(t, err)
	assert.Equal(t, "ali", search)
	assert.Empty(t, criteria)
}

func TestBuildOrderCriteria_NoValidFields(t *testing.T) {
	criteria, orders := BuildOrderCriteria("missing desc", filterConfig())
	assert.Empty(t, criteria)
	assert.Empty(t, orders)
}

func TestBuildSelectCriteria_NoValidFields(t *testing.T) {
	criteria, fields := BuildSelectCriteria([]string{"missing"}, filterConfig())
	assert.Empty(t, criteria)
	assert.Empty(t, fields)
}

func TestBuildSearchCriteria_SQLiteExecution(t *testing.T) {
	db := setupQueryBunDB(t)
	seedFilterUsers(t, db)

	criteria, _, err := BuildSearchCriteria("ali", Config{
		Fields: FieldConfig{Resolver: NewMapResolver(filterAllowedFields())},
		Search: SearchConfig{Fields: []string{"name"}},
	})
	require.NoError(t, err)

	rows := executeFilterUserCriteria(t, db, criteria)
	require.Len(t, rows, 1)
	assert.Equal(t, "Alice", rows[0].Name)
}

func TestBuildOrderCriteria_SQLiteExecution(t *testing.T) {
	db := setupQueryBunDB(t)
	seedFilterUsers(t, db)

	criteria, orders := BuildOrderCriteria("age desc", filterConfig())
	require.NotEmpty(t, orders)

	var rows []filterUser
	query := db.NewSelect().Model(&rows)
	for _, criterion := range criteria {
		query = criterion(query)
	}
	require.NoError(t, query.Scan(context.Background()))
	require.Len(t, rows, 3)
	assert.Equal(t, "Carol", rows[0].Name)
}

func TestBuildSelectCriteria_SQLiteExecution(t *testing.T) {
	db := setupQueryBunDB(t)
	seedFilterUsers(t, db)

	criteria, fields := BuildSelectCriteria([]string{"name"}, filterConfig())
	require.Equal(t, []string{"name"}, fields)

	var rows []filterUser
	query := db.NewSelect().Model(&rows)
	for _, criterion := range criteria {
		query = criterion(query)
	}
	require.NoError(t, query.Scan(context.Background()))
	require.Len(t, rows, 3)
	assert.Equal(t, 0, rows[0].ID)
	assert.NotEmpty(t, rows[0].Name)
}

func TestBuildPaginationCriteria_SQLiteExecution(t *testing.T) {
	db := setupQueryBunDB(t)
	seedFilterUsers(t, db)

	criteria, _, _, _ := BuildPaginationCriteria(ListOptions{Limit: 2, Offset: 1}, Config{})
	rows := executeFilterUserCriteria(t, db, criteria)
	require.Len(t, rows, 2)
	assert.Equal(t, "Bob", rows[0].Name)
}

func TestBuildSearchCriteria_ColumnNameAllowed(t *testing.T) {
	criteria, _, err := BuildSearchCriteria("ali", Config{
		Fields: FieldConfig{Resolver: NewMapResolver(filterAllowedFields())},
		Search: SearchConfig{Fields: []string{"name"}},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, criteria)

	criteria, _, err = BuildSearchCriteria("ali", Config{
		Fields: FieldConfig{Resolver: NewMapResolver(filterAllowedFields())},
		Search: SearchConfig{Fields: []string{"missing"}},
	})
	require.NoError(t, err)
	assert.Empty(t, criteria)
}

func TestBuildSearchCriteria_SQLiteFallbackDoesNotUseILikeOperator(t *testing.T) {
	db := setupQueryBunDB(t)
	criteria, _, err := BuildSearchCriteria("ali", Config{
		Fields: FieldConfig{Resolver: NewMapResolver(filterAllowedFields())},
		Search: SearchConfig{Fields: []string{"name"}},
	})
	require.NoError(t, err)

	query := db.NewSelect().Model((*filterUser)(nil))
	for _, criterion := range criteria {
		query = criterion(query)
	}
	assert.Contains(t, query.String(), "LOWER(name) LIKE LOWER('%ali%')")
}
