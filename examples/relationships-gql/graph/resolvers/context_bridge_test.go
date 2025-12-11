package resolvers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/goliatone/go-auth"
	"github.com/goliatone/go-crud"
)

func TestGraphQLContext_AttachesAuthActor(t *testing.T) {
	authCtx := context.Background()
	authActor := &auth.ActorContext{
		ActorID:        "actor-123",
		Subject:        "user-123",
		Role:           "admin",
		ResourceRoles:  map[string]string{"book": "write"},
		TenantID:       "tenant-xyz",
		OrganizationID: "org-abc",
		Metadata:       map[string]any{"foo": "bar"},
		ImpersonatorID: "imp-999",
		IsImpersonated: true,
	}
	authCtx = auth.WithActorContext(authCtx, authActor)

	crudCtx := GraphQLContext(authCtx)
	require.NotNil(t, crudCtx, "crud context should be constructed")

	actor := crud.ActorFromContext(crudCtx.UserContext())
	require.Equal(t, authActor.ActorID, actor.ActorID)
	require.Equal(t, authActor.Subject, actor.Subject)
	require.Equal(t, authActor.Role, actor.Role)
	require.Equal(t, authActor.TenantID, actor.TenantID)
	require.Equal(t, authActor.OrganizationID, actor.OrganizationID)
	require.Equal(t, authActor.ImpersonatorID, actor.ImpersonatorID)
	require.True(t, actor.IsImpersonated)
	require.Equal(t, authActor.ResourceRoles["book"], actor.ResourceRoles["book"])
	require.Equal(t, authActor.Metadata["foo"], actor.Metadata["foo"])
}

func TestGraphQLContext_PreservesCrudActor(t *testing.T) {
	base := crud.ContextWithActor(context.Background(), crud.ActorContext{
		ActorID:  "crud-actor",
		TenantID: "tenant-123",
	})

	crudCtx := GraphQLContext(base)
	require.NotNil(t, crudCtx, "crud context should be constructed")

	actor := crud.ActorFromContext(crudCtx.UserContext())
	require.Equal(t, "crud-actor", actor.ActorID)
	require.Equal(t, "tenant-123", actor.TenantID)
}
