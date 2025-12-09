package hooks

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/goliatone/go-crud/gql/internal/formatter"
	"github.com/goliatone/go-crud/gql/internal/overlay"
)

func TestBuild_NoHooksStillProvidesEntities(t *testing.T) {
	doc := formatter.Document{
		Entities: []formatter.Entity{
			{Name: "Post", RawName: "post"},
			{Name: "User", RawName: "user"},
		},
	}

	result := Build(doc, Options{})
	require.Len(t, result.Entities, 2)
	require.Contains(t, result.Entities, "Post")
	require.Contains(t, result.Entities, "User")
	require.Empty(t, result.Imports)
	require.Empty(t, result.Entities["Post"].Get.AuthGuard)
}

func TestBuild_WithAuthAndEntityHooks(t *testing.T) {
	doc := formatter.Document{
		Entities: []formatter.Entity{
			{Name: "Post", RawName: "post"},
		},
	}

	opts := Options{
		AuthPackage: "github.com/goliatone/go-auth",
		AuthGuard:   "auth.FromContext(ctx)",
		Overlay: overlay.Hooks{
			Default: overlay.HookSet{
				ScopeGuard: "r.ScopeHook(ctx, entity, action)",
			},
			Entities: map[string]overlay.EntityHooks{
				"post": {
					Operations: map[string]overlay.HookSet{
						"list": {
							Preload: "criteria = append(criteria, repository.SelectRelation(\"Author\"))",
						},
					},
				},
			},
		},
	}

	result := Build(doc, opts)
	require.Contains(t, result.Imports, "github.com/goliatone/go-auth")
	require.Contains(t, result.Imports, "errors", "default auth-fail should add errors import")

	postHooks := result.Entities["Post"]
	require.NotEmpty(t, postHooks.Get.AuthGuard)
	require.Contains(t, postHooks.Get.AuthGuard, "auth.FromContext(ctx)")
	require.Contains(t, postHooks.Delete.AuthGuard, "return false", "delete should use false for boolean return")

	require.Equal(t, "r.ScopeHook(ctx, entity, action)", postHooks.Get.ScopeGuard)
	require.Equal(t, "criteria = append(criteria, repository.SelectRelation(\"Author\"))", postHooks.List.Preload)
	require.Empty(t, postHooks.Create.Preload)
}

func TestBuildAuthSnippetUsesZeroValues(t *testing.T) {
	getSnippet := BuildAuthSnippet(OperationGet, "auth.FromContext(ctx)", "errors.New(\"boom\")")
	require.Contains(t, getSnippet, "return nil, errors.New(\"boom\")")

	deleteSnippet := BuildAuthSnippet(OperationDelete, "auth.FromContext(ctx)", "errors.New(\"boom\")")
	require.Contains(t, deleteSnippet, "return false, errors.New(\"boom\")")
}
