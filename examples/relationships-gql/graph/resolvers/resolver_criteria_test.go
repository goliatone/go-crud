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

	conn, err := resolver.ListBook(ctx, &model.PaginationInput{Limit: &limit, Offset: &offset}, order, nil)
	require.NoError(t, err)
	require.NotNil(t, conn)
	require.Len(t, conn.Edges, 1)
	require.NotNil(t, conn.Edges[0].Node)
	require.NotEmpty(t, conn.Edges[0].Node.Title)
	require.Equal(t, encodeCursor(offset), conn.Edges[0].Cursor)

	require.NotNil(t, conn.PageInfo)
	require.Equal(t, 3, conn.PageInfo.Total)
	require.True(t, conn.PageInfo.HasNextPage)
	require.True(t, conn.PageInfo.HasPreviousPage)
	require.Equal(t, encodeCursor(offset), conn.PageInfo.StartCursor)
	require.Equal(t, encodeCursor(offset), conn.PageInfo.EndCursor)
}

func TestListBookFiltersSupportInAndNestedRelations(t *testing.T) {
	resolver, ctx, cleanup := setupResolver(t)
	defer cleanup()

	filters := []*model.FilterInput{
		{Field: "status", Operator: model.FilterOperatorIN, Value: "in_print,editing"},
		{Field: "author.fullName", Operator: model.FilterOperatorEQ, Value: "Miles Dorsey"},
	}

	conn, err := resolver.ListBook(ctx, nil, nil, filters)
	require.NoError(t, err)
	require.NotNil(t, conn)
	require.Len(t, conn.Edges, 1)
	require.Equal(t, "Microburst Protocol", conn.Edges[0].Node.Title)
	require.Equal(t, "Miles Dorsey", conn.Edges[0].Node.Author.FullName)

	require.NotNil(t, conn.PageInfo)
	require.Equal(t, 1, conn.PageInfo.Total)
	require.False(t, conn.PageInfo.HasNextPage)
	require.False(t, conn.PageInfo.HasPreviousPage)
	require.Equal(t, encodeCursor(0), conn.PageInfo.StartCursor)
	require.Equal(t, encodeCursor(0), conn.PageInfo.EndCursor)
}

func TestListBookPageInfoEndBoundary(t *testing.T) {
	resolver, ctx, cleanup := setupResolver(t)
	defer cleanup()

	limit := 1
	offset := 2
	order := []*model.OrderByInput{
		{Field: "title"},
	}

	conn, err := resolver.ListBook(ctx, &model.PaginationInput{Limit: &limit, Offset: &offset}, order, nil)
	require.NoError(t, err)
	require.NotNil(t, conn)
	require.Len(t, conn.Edges, 1)
	require.NotNil(t, conn.Edges[0].Node)

	require.NotNil(t, conn.PageInfo)
	require.Equal(t, 3, conn.PageInfo.Total)
	require.False(t, conn.PageInfo.HasNextPage)
	require.True(t, conn.PageInfo.HasPreviousPage)
	require.Equal(t, encodeCursor(offset), conn.PageInfo.StartCursor)
	require.Equal(t, encodeCursor(offset), conn.PageInfo.EndCursor)
}
