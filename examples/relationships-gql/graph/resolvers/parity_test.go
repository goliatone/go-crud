package resolvers

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/goliatone/go-crud"
	"github.com/goliatone/go-crud/examples/relationships-gql/graph/model"
)

const (
	auroraPublisherID = "ddfe89a9-c118-334b-ad2f-941166ef26f4"
	auroraAuthorID    = "06ef3339-bd72-333e-9c28-352b9c2cc612"
	sciFiTagID        = "c13a4cbf-e9ee-3573-8fdd-4ef70a8d1bb0"
	nimbusPublisherID = "83669b17-c772-3c97-8556-23f067d05ba3"
)

func TestGraphQLAndREST_EnforceScopePolicyAndVirtuals(t *testing.T) {
	resolver, ctx, cleanup := setupResolver(t)
	defer cleanup()

	ctx = crud.ContextWithActor(ctx, crud.ActorContext{
		ActorID:  "actor-aurora",
		TenantID: auroraPublisherID,
		Role:     "guest",
	})

	beforeVirtual := resolver.inst.virtualAfter["author"]

	author, err := resolver.GetAuthor(ctx, auroraAuthorID)
	require.NoError(t, err)
	require.NotNil(t, author)
	require.Empty(t, author.Email, "field policy should remove email for guests")
	require.Empty(t, author.PenName, "field policy should remove pen name for guests")
	require.Equal(t, auroraPublisherID, author.PublisherId)

	restAuthor, err := resolver.rest.author.Show(resolver.crudContext(ctx), auroraAuthorID, nil)
	require.NoError(t, err)
	restModel := toModelAuthor(restAuthor, true)
	require.NotNil(t, restModel)
	require.Equal(t, author.Email, restModel.Email)
	require.Equal(t, author.PublisherId, restModel.PublisherId)

	conn, err := resolver.ListAuthor(ctx, nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, conn)
	require.Greater(t, resolver.inst.virtualAfter["author"], beforeVirtual, "virtual field wrapper should run on list/show")

	mismatchCtx := crud.ContextWithActor(ctx, crud.ActorContext{
		ActorID:  "actor-nimbus",
		TenantID: nimbusPublisherID,
		Role:     "guest",
	})

	_, err = resolver.GetAuthor(mismatchCtx, auroraAuthorID)
	require.Error(t, err, "scope guard should block cross-tenant lookups in GraphQL")

	_, err = resolver.rest.author.Show(resolver.crudContext(mismatchCtx), auroraAuthorID, nil)
	require.Error(t, err, "scope guard should block cross-tenant lookups in REST/domain service")
}

func TestGraphQLAndREST_EmitActivity(t *testing.T) {
	resolver, ctx, cleanup := setupResolver(t)
	defer cleanup()

	ctx = crud.ContextWithActor(ctx, crud.ActorContext{
		ActorID:  "actor-aurora",
		TenantID: auroraPublisherID,
		Role:     "admin",
	})

	start := len(resolver.inst.activities)

	updatedName := fmt.Sprintf("Science Fiction %d", time.Now().UnixNano())
	_, err := resolver.UpdateTag(ctx, sciFiTagID, model.UpdateTagInput{
		Name: &updatedName,
	})
	require.NoError(t, err)
	require.Equal(t, start+1, len(resolver.inst.activities))

	restCtx := resolver.crudContext(ctx)
	restTag, err := resolver.rest.tag.Show(restCtx, sciFiTagID, nil)
	require.NoError(t, err)
	require.NotNil(t, restTag)

	restTag.Name = updatedName + " REST"
	restTag.Description = "updated via rest parity"
	restTag.Category = "genre"

	_, err = resolver.rest.tag.Update(restCtx, restTag)
	require.NoError(t, err)
	require.Equal(t, start+2, len(resolver.inst.activities))

	last := resolver.inst.activities[len(resolver.inst.activities)-1]
	require.Equal(t, "update", last.Verb)
	require.NotEmpty(t, last.ObjectID)
	require.NotEmpty(t, last.ObjectType)
	require.Equal(t, "gql", last.Channel)
}
