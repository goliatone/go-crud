package dataloader_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	relationships "github.com/goliatone/go-crud/examples/relationships-gql"
	"github.com/goliatone/go-crud/examples/relationships-gql/graph/dataloader"
	"github.com/goliatone/go-crud/examples/relationships-gql/graph/model"
	"github.com/goliatone/go-crud/examples/relationships-gql/graph/resolvers"
)

func TestLoader_GroupsDeterministically(t *testing.T) {
	ctx := context.Background()
	client, err := relationships.SetupDatabase(ctx)
	require.NoError(t, err)
	defer func() {
		if db := client.DB(); db != nil {
			_ = db.Close()
		}
	}()

	db := client.DB()
	repos := relationships.RegisterRepositories(db)
	require.NoError(t, relationships.MigrateSchema(ctx, db))
	require.NoError(t, relationships.SeedDatabase(ctx, client))

	resolver := resolvers.NewResolver(repos)
	loader := dataloader.New(dataloader.Services{
		Author:          resolver.AuthorSvc,
		AuthorProfile:   resolver.AuthorProfileSvc,
		Book:            resolver.BookSvc,
		Chapter:         resolver.ChapterSvc,
		Headquarters:    resolver.HeadquartersSvc,
		PublishingHouse: resolver.PublishingHouseSvc,
		Tag:             resolver.TagSvc,
	}, dataloader.WithDB(db), dataloader.WithContextFactory(resolver.ContextFactory))
	require.NotNil(t, loader)

	authors, _, err := resolver.AuthorSvc.Index(resolver.ContextFactory(ctx), nil)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(authors), 2)

	keys := []string{authors[0].Id, authors[1].Id}
	first, err := loader.AuthorBooks.LoadMany(ctx, keys)
	require.NoError(t, err)
	require.Len(t, first, len(keys))

	second, err := loader.AuthorBooks.LoadMany(ctx, []string{keys[1], keys[0]})
	require.NoError(t, err)

	for _, key := range keys {
		require.True(t, isSorted(first[key], func(item *model.Book) string { return item.Id }))
		require.Equal(t, first[key], second[key], "loader cache should be deterministic for %s", key)
	}

	tagsFirst, err := loader.AuthorTags.LoadMany(ctx, keys)
	require.NoError(t, err)

	tagsSecond, err := loader.AuthorTags.LoadMany(ctx, []string{keys[1], keys[0]})
	require.NoError(t, err)

	for _, key := range keys {
		require.True(t, isSorted(tagsFirst[key], func(item *model.Tag) string { return item.Id }))
		require.Equal(t, tagsFirst[key], tagsSecond[key], "pivot-backed loaders should stay deterministic for %s", key)
	}
}

func isSorted[T any](items []*T, key func(*T) string) bool {
	for i := 1; i < len(items); i++ {
		if key(items[i-1]) > key(items[i]) {
			return false
		}
	}
	return true
}
