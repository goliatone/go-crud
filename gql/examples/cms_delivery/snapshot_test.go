package cms_delivery

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/goliatone/go-crud/gql/internal/formatter"
	"github.com/goliatone/go-crud/gql/internal/metadata"
	"github.com/goliatone/go-crud/gql/internal/templates"
)

func TestCMSDeliverySnapshots(t *testing.T) {
	renderer, err := templates.NewRenderer()
	require.NoError(t, err)

	schemas, err := metadata.FromFile("metadata.json")
	require.NoError(t, err)

	doc, err := formatter.Format(schemas)
	require.NoError(t, err)

	ctx := templates.BuildContext(doc, templates.ContextOptions{
		ConfigPath: filepath.Join("output", "gqlgen.yml"),
		OutDir:     "output",
	})

	assertSnapshotMatch(t, renderer, templates.SchemaTemplate, ctx, filepath.Join("output", "schema.graphql"))
	assertSnapshotMatch(t, renderer, templates.GQLGenConfigTemplate, ctx, filepath.Join("output", "gqlgen.yml"))
	assertSnapshotMatch(t, renderer, templates.ModelsTemplate, ctx, filepath.Join("output", "model", "models_gen.go"))
	assertSnapshotMatch(t, renderer, templates.ResolverGenTemplate, ctx, filepath.Join("output", "resolvers", "resolver_gen.go"))
}

func assertSnapshotMatch(t *testing.T, renderer interface {
	Render(string, any, ...io.Writer) (string, error)
}, template string, ctx any, snapshotPath string) {
	t.Helper()

	out, err := renderer.Render(template, ctx)
	require.NoError(t, err)

	data, err := os.ReadFile(snapshotPath)
	require.NoError(t, err)

	expected := stripBuildTags(string(data))
	if expected != out {
		t.Fatalf("%s did not match snapshot %s", template, snapshotPath)
	}
}

func stripBuildTags(content string) string {
	lines := strings.Split(content, "\n")
	if len(lines) >= 2 && strings.HasPrefix(lines[0], "//go:build") && strings.HasPrefix(lines[1], "// +build") {
		lines = lines[2:]
		if len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
			lines = lines[1:]
		}
	}
	return strings.Join(lines, "\n")
}
