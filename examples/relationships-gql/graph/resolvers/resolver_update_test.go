package resolvers

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/goliatone/go-auth"
	relationships "github.com/goliatone/go-crud/examples/relationships-gql"
	"github.com/goliatone/go-crud/examples/relationships-gql/graph/model"
	"github.com/google/uuid"
)

func TestApplyInputPointerPatch(t *testing.T) {
	type input struct {
		Name  *string
		Count *int
	}
	type target struct {
		Name  *string
		Count *int
	}

	originalName := "keep"
	originalCount := 3
	dst := target{Name: &originalName, Count: &originalCount}
	newName := "updated"

	require.NotPanics(t, func() {
		applyInput(&dst, input{Name: &newName})
	})

	require.NotNil(t, dst.Name)
	require.Equal(t, newName, *dst.Name)
	require.NotNil(t, dst.Count)
	require.Equal(t, originalCount, *dst.Count)

	dstNil := target{}
	countVal := 10
	applyInput(&dstNil, input{Count: &countVal})
	require.NotNil(t, dstNil.Count)
	require.Equal(t, countVal, *dstNil.Count)
	require.Nil(t, dstNil.Name)
}

func TestUpdateBookPreservesExistingFields(t *testing.T) {
	resolver, ctx, cleanup := setupResolver(t)
	defer cleanup()

	books, _, err := resolver.BookService().Index(resolver.crudContext(ctx), nil)
	require.NoError(t, err)
	require.NotEmpty(t, books)

	original := books[0]
	newTitle := original.Title + " (rev)"

	updated, err := resolver.UpdateBook(ctx, original.Id, model.UpdateBookInput{
		Title: &newTitle,
	})
	require.NoError(t, err)

	require.Equal(t, newTitle, updated.Title)
	require.Equal(t, original.Status, updated.Status)
	require.Equal(t, original.Isbn, updated.Isbn)
	require.Equal(t, original.PublisherId, updated.PublisherId)
	require.Equal(t, original.AuthorId, updated.AuthorId)
	require.NotNil(t, updated.CreatedAt)
	if original.CreatedAt != nil && updated.CreatedAt != nil {
		require.WithinDuration(t, *original.CreatedAt, *updated.CreatedAt, time.Second)
	}
}

func setupResolver(t *testing.T) (*Resolver, context.Context, func()) {
	t.Helper()

	ctx := context.Background()
	client, err := relationships.SetupDatabase(ctx)
	require.NoError(t, err)
	require.NotNil(t, client)

	db := client.DB()
	require.NoError(t, relationships.MigrateSchema(ctx, db))
	require.NoError(t, relationships.SeedDatabase(ctx, client))

	ctx = auth.WithContext(ctx, &auth.User{
		ID:       uuid.New(),
		Username: "test-user",
		Role:     auth.RoleAdmin,
	})
	ctx = auth.WithActorContext(ctx, &auth.ActorContext{
		ActorID: "graph-test-actor",
		Subject: "test-user",
		Role:    string(auth.RoleAdmin),
	})

	resolver := NewResolver(relationships.RegisterRepositories(db))

	return resolver, ctx, func() {
		if client == nil || client.DB() == nil {
			return
		}
		defer func() { _ = recover() }()
		_ = client.Close()
	}
}
