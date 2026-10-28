package templates

import (
	"testing"

	"github.com/goliatone/go-crud/gql/internal/formatter"
	"github.com/goliatone/go-crud/gql/internal/overlay"
	"github.com/stretchr/testify/require"
)

func TestBuildContext_ModelTypesAndSubscriptionArguments(t *testing.T) {
	doc := formatter.Document{Entities: []formatter.Entity{{
		Name: "Article", RawName: "article",
		Fields: []formatter.Field{
			{Name: "id", OriginalName: "id", Type: "UUID", Required: true},
			{Name: "title", OriginalName: "title", Type: "String", Required: true},
			{Name: "publishedAt", OriginalName: "published_at", Type: "Time", Required: true, Nullable: true},
			{Name: "labels", OriginalName: "labels", Type: "String", IsList: true},
		},
	}}}
	ctx := BuildContext(doc, ContextOptions{
		ConfigPath: "gqlgen.yml", OutDir: "graph", EmitSubscriptions: true,
		Overlay: overlay.Overlay{Subscriptions: []overlay.Subscription{{
			Name: " customArticle ", ReturnType: " Article ", Events: []string{" UPDATED ", "created"},
			Args: []overlay.Argument{
				{Name: " ids ", Type: " UUID ", Required: true, List: true, Description: "selected IDs"},
				{Name: " ", Type: "String"},
				{Name: "invalid", Type: " "},
			},
		}}},
	})
	var fields []ModelField
	for _, model := range ctx.ModelStructs {
		if model.Name == "Article" {
			fields = model.Fields
		}
	}
	require.Equal(t, []ModelField{
		{Name: "Id", JSONName: "id", GoType: "string"},
		{Name: "Title", JSONName: "title", GoType: "string"},
		{Name: "PublishedAt", JSONName: "published_at", GoType: "*time.Time"},
		{Name: "Labels", JSONName: "labels", GoType: "[]string"},
	}, fields)
	require.Contains(t, ctx.ModelImports, "time")
	for _, sub := range ctx.Subscriptions {
		if sub.Name != "customArticle" {
			continue
		}
		require.Equal(t, "Article", sub.ReturnType)
		require.Equal(t, "updated", sub.Event)
		require.Equal(t, "article.updated", sub.Topic)
		require.Equal(t, []TemplateArgument{{Name: "ids", Type: "UUID", Required: true, List: true, Description: "selected IDs"}}, sub.Args)
		return
	}
	t.Fatal("custom subscription missing")
}
