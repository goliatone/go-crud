package templates

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/goliatone/go-crud/gql/internal/formatter"
	"github.com/goliatone/go-crud/gql/internal/hooks"
	"github.com/goliatone/go-crud/gql/internal/metadata"
	"github.com/goliatone/go-crud/gql/internal/overlay"
)

func TestTemplates_RenderGolden(t *testing.T) {
	renderer, err := NewRenderer()
	require.NoError(t, err)

	schemas, err := metadata.FromFile(filepath.Join("testdata", "metadata.json"))
	require.NoError(t, err)

	doc, err := formatter.Format(schemas)
	require.NoError(t, err)

	ctx := BuildContext(doc, ContextOptions{
		ConfigPath: "gqlgen.yml",
		OutDir:     "graph",
	})
	require.NotEmpty(t, ctx.Scalars, "expected default scalars")
	require.Equal(t, "UUID", ctx.Scalars[0].Name)

	assertRenderMatches(t, renderer, SchemaTemplate, ctx, "schema.graphql.golden")
	assertRenderMatches(t, renderer, GQLGenConfigTemplate, ctx, "gqlgen.yml.golden")
	assertRenderMatches(t, renderer, ModelsTemplate, ctx, "models_gen.go.golden")
	assertRenderMatches(t, renderer, ModelsCustomTemplate, ctx, "models_custom.go.golden")
	assertRenderMatches(t, renderer, ResolverGenTemplate, ctx, "resolver_gen.go.golden")
	assertRenderMatches(t, renderer, ResolverCustomTemplate, ctx, "resolver_custom.go.golden")
	assertRenderMatches(t, renderer, DataloaderTemplate, ctx, "dataloader.go.golden")
}

func TestTemplates_RenderWithHooks(t *testing.T) {
	renderer, err := NewRenderer()
	require.NoError(t, err)

	schemas, err := metadata.FromFile(filepath.Join("testdata", "metadata.json"))
	require.NoError(t, err)

	doc, err := formatter.Format(schemas)
	require.NoError(t, err)

	ctx := BuildContext(doc, ContextOptions{
		ConfigPath: "gqlgen.yml",
		OutDir:     "graph",
		HookOptions: hooks.Options{
			AuthPackage: "github.com/goliatone/go-auth",
			AuthGuard:   "auth.FromContext(ctx)",
			Overlay: overlay.Hooks{
				Entities: map[string]overlay.EntityHooks{
					"post": {
						Operations: map[string]overlay.HookSet{
							"list": {Preload: "criteria = append(criteria, repository.SelectRelation(\"Author\"))"},
						},
					},
				},
			},
		},
	})

	require.Contains(t, ctx.Hooks.Imports, "github.com/goliatone/go-auth")
	assertRenderMatches(t, renderer, ResolverGenTemplate, ctx, "resolver_gen_hooks.go.golden")
}

func TestBuildContext_OmitsMutationFields(t *testing.T) {
	schemas, err := metadata.FromFile(filepath.Join("testdata", "metadata.json"))
	require.NoError(t, err)

	doc, err := formatter.Format(schemas)
	require.NoError(t, err)

	for i := range doc.Entities {
		if doc.Entities[i].RawName != "post" {
			continue
		}
		for j := range doc.Entities[i].Fields {
			if doc.Entities[i].Fields[j].OriginalName == "title" {
				doc.Entities[i].Fields[j].OmitFromMutations = true
			}
		}
	}

	ctx := BuildContext(doc, ContextOptions{
		ConfigPath: "gqlgen.yml",
		OutDir:     "graph",
	})

	create := findInput(ctx.Inputs, "CreatePostInput")
	require.NotNil(t, create, "create input should exist")
	require.False(t, hasInputField(*create, "title"), "field flagged for omission should be excluded from create input")

	update := findInput(ctx.Inputs, "UpdatePostInput")
	require.NotNil(t, update, "update input should exist")
	require.False(t, hasInputField(*update, "title"), "field flagged for omission should be excluded from update input")
}

func assertRenderMatches(t *testing.T, renderer interface {
	Render(string, any, ...io.Writer) (string, error)
}, tpl string, ctx any, golden string) {
	t.Helper()
	out, err := renderer.Render(tpl, ctx)
	require.NoError(t, err)

	goldenPath := filepath.Join("testdata", golden)
	if os.Getenv("UPDATE_GQL_TEMPLATES") == "1" {
		require.NoError(t, os.WriteFile(goldenPath, []byte(out), 0o644))
	}

	expected, err := os.ReadFile(goldenPath)
	require.NoError(t, err)

	if string(expected) != out {
		t.Fatalf("%s did not match golden (expected len %d, got %d).\nExpected:\n%q\nGot:\n%q", tpl, len(expected), len(out), string(expected), out)
	}
}

func findInput(inputs []TemplateInput, name string) *TemplateInput {
	for i := range inputs {
		if inputs[i].Name == name {
			return &inputs[i]
		}
	}
	return nil
}

func hasInputField(input TemplateInput, name string) bool {
	for _, f := range input.Fields {
		if f.Name == name {
			return true
		}
	}
	return false
}
