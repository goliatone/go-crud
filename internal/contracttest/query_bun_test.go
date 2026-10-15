package contracttest

import (
	"context"
	"database/sql"
	"testing"

	querybun "github.com/goliatone/go-crud/pkg/go-query-bun"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
)

// This fixture imports only public interfaces and can be copied into an admin
// adapter's test package. It deliberately uses every grouped Config field.
func adminQueryConfig() querybun.Config {
	return querybun.Config{
		Syntax: querybun.DefaultRESTSyntax(),
		Fields: querybun.FieldConfig{Resolver: querybun.NewMapResolver(map[string]string{
			"id": "id", "name": "name", "group_id": "group_id",
		})},
		Operators: querybun.DefaultOperatorSet(),
		Search: querybun.SearchConfig{
			Aliases: []string{"_search", "search", "q"}, Fields: []string{"name"},
			Pattern: querybun.PatternConfig{Mode: querybun.PatternContainsEscaped},
			Case:    querybun.CaseInsensitive,
		},
		Predicates: querybun.PredicateBuilderSet{Fields: map[string]querybun.PredicateBuilder{
			"membership": querybun.PredicateBuilderFunc(func(in querybun.PredicateBuilderInput) (querybun.PredicateBuilderResult, error) {
				if in.Operator.Canonical != querybun.OperatorEq {
					return querybun.PredicateUnhandled(), nil
				}
				return querybun.PredicateHandled(in.Helpers.Compare("group_id", "=", in.Values)), nil
			}),
		}},
		Pagination: querybun.PaginationConfig{DefaultLimit: 25, DefaultOffset: 0},
		Validation: querybun.ValidationConfig{Mode: querybun.ValidationStrict},
	}
}

var _ func(querybun.ListOptions, querybun.Config) (querybun.Plan, error) = querybun.BuildQueryPlan
var _ func(querybun.ListOptions, querybun.Config) (int, int, int) = querybun.NormalizePagination
var _ querybun.FieldResolver = querybun.MapResolver{}
var _ querybun.FieldResolver = querybun.SafeIdentifierResolver{}

func TestQueryBunCurrentContract(t *testing.T) {
	cfg := adminQueryConfig()
	// Runtime strings require an explicit conversion; unknown kinds are still
	// validated by the planner and never silently become equality operators.
	operator := querybun.OperatorKind(string(querybun.OperatorEq))
	plan, err := querybun.BuildQueryPlan(querybun.ListOptions{
		Limit: 1, LimitSet: true, Offset: 1, OffsetSet: true, Page: 9, PerPage: 50,
		Order: "id asc", Select: []string{"id"},
		Predicates: []querybun.Predicate{{Field: "membership", Operator: operator, Values: []string{"A"}}},
		Filters:    map[string]any{"group_id": "B", "q": "opaque"},
	}, cfg)
	require.NoError(t, err)
	require.Empty(t, plan.Unsupported)
	require.Equal(t, 1, plan.Metadata.Limit)
	require.Equal(t, 1, plan.Metadata.Offset)
	require.Equal(t, "opaque", plan.Metadata.Search)

	sqldb, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	sqldb.SetMaxOpenConns(1)
	db := bun.NewDB(sqldb, sqlitedialect.New())
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	ctx := context.Background()
	_, err = db.ExecContext(ctx, "CREATE TABLE contract_rows (id INTEGER PRIMARY KEY, name TEXT, group_id TEXT)")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "INSERT INTO contract_rows VALUES (1,'opaque-a','A'),(2,'opaque-b','A'),(3,'opaque-c','B')")
	require.NoError(t, err)
	apply := func(criteria []querybun.Criteria) *bun.SelectQuery {
		q := db.NewSelect().Table("contract_rows")
		for _, c := range criteria {
			q = c(q)
		}
		return q
	}
	countSQL := apply(plan.CountCriteria()).String()
	require.NotContains(t, countSQL, "LIMIT")
	require.NotContains(t, countSQL, "ORDER BY")
	count, err := apply(plan.CountCriteria()).Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, count, "explicit predicates override non-search legacy filters; count precedes paging")
	var ids []int
	require.NoError(t, apply(plan.ListCriteria()).Scan(ctx, &ids))
	require.Equal(t, []int{2}, ids)

	for _, alias := range cfg.Search.Aliases {
		p, err := querybun.BuildQueryPlan(querybun.ListOptions{Filters: map[string]any{alias: "opaque-a"}}, cfg)
		require.NoError(t, err)
		n, err := apply(p.CountCriteria()).Count(ctx)
		require.NoError(t, err)
		require.Equal(t, 1, n)
	}
	_, err = querybun.BuildQueryPlan(querybun.ListOptions{Predicates: []querybun.Predicate{
		{Field: "name", Operator: querybun.OperatorKind("unknown"), Values: []string{"opaque-a"}},
	}}, cfg)
	require.Error(t, err)
	cfg.Validation.Mode = querybun.ValidationCollect
	p, err := querybun.BuildQueryPlan(querybun.ListOptions{Filters: map[string]any{"name__unknown": "opaque-a"}}, cfg)
	require.NoError(t, err)
	require.Len(t, p.Unsupported, 1)
	require.Empty(t, p.Filters)

	limit, offset, _ := querybun.NormalizePagination(querybun.ListOptions{}, cfg)
	require.Equal(t, 25, limit)
	require.Zero(t, offset)
	limit, offset, _ = querybun.NormalizePagination(querybun.ListOptions{LimitSet: true, Limit: 0, OffsetSet: true, Offset: 0, Page: 4}, cfg)
	require.Zero(t, limit, "presence flags preserve an explicit zero; there is no maximum-limit config")
	require.Zero(t, offset)
}
