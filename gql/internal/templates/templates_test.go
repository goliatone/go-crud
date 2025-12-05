package templates

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/goliatone/go-crud/gql/internal/formatter"
	"github.com/goliatone/go-crud/gql/internal/metadata"
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

func assertRenderMatches(t *testing.T, renderer interface {
	Render(string, any, ...io.Writer) (string, error)
}, tpl string, ctx any, golden string) {
	t.Helper()
	out, err := renderer.Render(tpl, ctx)
	require.NoError(t, err)

	expected, err := os.ReadFile(filepath.Join("testdata", golden))
	require.NoError(t, err)

	if string(expected) != out {
		t.Fatalf("%s did not match golden (expected len %d, got %d).\nExpected:\n%q\nGot:\n%q", tpl, len(expected), len(out), string(expected), out)
	}
}
