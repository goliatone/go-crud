package querybun

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

func TestBuildQueryPlan_PopulatesIndependentSections(t *testing.T) {
	plan, err := BuildQueryPlan(ListOptions{
		Limit:   2,
		Offset:  1,
		Order:   "age desc",
		Search:  "ali",
		Select:  []string{"id,name"},
		Include: []string{"Profiles,Company"},
		Filters: map[string]any{
			"status__or": "active,pending",
		},
	}, Config{
		AllowedFields: filterAllowedFields(),
		SearchColumns: []string{"name"},
	})
	require.NoError(t, err)

	assert.NotEmpty(t, plan.Pagination)
	assert.NotEmpty(t, plan.Order)
	assert.NotEmpty(t, plan.Filters)
	assert.NotEmpty(t, plan.Search)
	assert.NotEmpty(t, plan.Select)
	assert.Equal(t, []IncludeRequest{{Path: "Profiles"}, {Path: "Company"}}, plan.Includes)
	assert.Empty(t, plan.Unsupported)

	assert.Equal(t, 2, plan.Metadata.Limit)
	assert.Equal(t, 1, plan.Metadata.Offset)
	assert.Equal(t, 1, plan.Metadata.Page)
	assert.Equal(t, "ali", plan.Metadata.Search)
	assert.Equal(t, []Order{{Field: "age", Dir: "DESC"}}, plan.Metadata.Order)
	assert.Equal(t, []string{"id", "name"}, plan.Metadata.Fields)
	assert.Equal(t, []string{"Profiles", "Company"}, plan.Metadata.Include)
}

func TestPlanListCriteriaOrder(t *testing.T) {
	pagination := Criteria(func(q *bun.SelectQuery) *bun.SelectQuery { return q })
	order := Criteria(func(q *bun.SelectQuery) *bun.SelectQuery { return q })
	filter := Criteria(func(q *bun.SelectQuery) *bun.SelectQuery { return q })
	search := Criteria(func(q *bun.SelectQuery) *bun.SelectQuery { return q })
	selectFields := Criteria(func(q *bun.SelectQuery) *bun.SelectQuery { return q })

	plan := Plan{
		Pagination: []Criteria{pagination},
		Order:      []Criteria{order},
		Filters:    []Criteria{filter},
		Search:     []Criteria{search},
		Select:     []Criteria{selectFields},
	}

	got := plan.ListCriteria()
	require.Len(t, got, 5)
	assertSameCriteria(t, pagination, got[0])
	assertSameCriteria(t, order, got[1])
	assertSameCriteria(t, filter, got[2])
	assertSameCriteria(t, search, got[3])
	assertSameCriteria(t, selectFields, got[4])

	read := plan.ReadCriteria()
	require.Len(t, read, 1)
	assertSameCriteria(t, selectFields, read[0])
}

func TestBuildQueryPlan_FilterOnlyCallersCanOmitPaginationAndOrder(t *testing.T) {
	db := setupQueryBunDB(t)
	seedFilterUsers(t, db)

	plan, err := BuildQueryPlan(ListOptions{
		Limit: 1,
		Order: "age desc",
		Filters: map[string]any{
			"status__or": "active,pending",
		},
	}, Config{AllowedFields: filterAllowedFields()})
	require.NoError(t, err)
	require.NotEmpty(t, plan.Filters)

	var rows []filterUser
	query := db.NewSelect().Model(&rows)
	for _, criterion := range plan.Filters {
		query = criterion(query)
	}
	require.NoError(t, query.Order("id ASC").Scan(context.Background()))

	require.Len(t, rows, 2)
	assert.Equal(t, []string{"Alice", "Carol"}, []string{rows[0].Name, rows[1].Name})
}

func TestBuildQueryPlan_UnsupportedRemainsAvailable(t *testing.T) {
	plan, err := BuildQueryPlan(ListOptions{
		Filters: map[string]any{
			"name__unknown": "Alice",
			"bad":           map[string]string{"value": "shape"},
		},
	}, Config{AllowedFields: filterAllowedFields()})
	require.NoError(t, err)
	assert.Empty(t, plan.Filters)
	assert.ElementsMatch(t, []UnsupportedPredicate{
		{Field: "bad", Operator: "eq", RawKey: "bad", RawValue: map[string]string{"value": "shape"}, Reason: UnsupportedValueShape},
		{Field: "name", Operator: "unknown", RawKey: "name__unknown", RawValue: "Alice", Reason: UnsupportedOperator},
	}, plan.Unsupported)
}

func TestBuildQueryPlan_StrictErrorReturnsPlanWithUnsupported(t *testing.T) {
	plan, err := BuildQueryPlan(ListOptions{
		Filters: map[string]any{
			"name__unknown": "Alice",
		},
	}, Config{
		AllowedFields:    filterAllowedFields(),
		StrictValidation: true,
	})
	require.Error(t, err)
	assert.Equal(t, []UnsupportedPredicate{
		{Field: "name", Operator: "unknown", RawKey: "name__unknown", RawValue: "Alice", Reason: UnsupportedOperator},
	}, plan.Unsupported)
}

func assertSameCriteria(t *testing.T, expected, actual Criteria) {
	t.Helper()
	assert.Equal(t, reflect.ValueOf(expected).Pointer(), reflect.ValueOf(actual).Pointer())
}
