package resolvers

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/goliatone/go-crud/examples/relationships-gql/graph/model"
)

func TestListBookSupportsOrderAndPagination(t *testing.T) {
	resolver, ctx, cleanup := setupResolver(t)
	defer cleanup()

	limit := 1
	offset := 1
	dir := model.OrderDirectionDESC
	order := []*model.OrderByInput{
		{Field: "title", Direction: &dir},
	}

	items, err := resolver.ListBook(ctx, &model.PaginationInput{Limit: &limit, Offset: &offset}, order, nil)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "Microburst Protocol", items[0].Title)
}

func TestListBookFiltersSupportInAndNestedRelations(t *testing.T) {
	resolver, ctx, cleanup := setupResolver(t)
	defer cleanup()

	filters := []*model.FilterInput{
		{Field: "status", Operator: model.FilterOperatorIN, Value: "in_print,editing"},
		{Field: "author.fullName", Operator: model.FilterOperatorEQ, Value: "Miles Dorsey"},
	}

	items, err := resolver.ListBook(ctx, nil, nil, filters)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "Microburst Protocol", items[0].Title)
	require.Equal(t, "Miles Dorsey", items[0].Author.FullName)
}
