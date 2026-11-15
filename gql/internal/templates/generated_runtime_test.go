package templates

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goliatone/go-crud/gql/internal/formatter"
	"github.com/goliatone/go-crud/gql/internal/hooks"
	"github.com/goliatone/go-crud/gql/internal/metadata"
	"github.com/goliatone/go-crud/gql/internal/overlay"
	"github.com/stretchr/testify/require"

	// Dependencies used by the generated runtime fixture.
	_ "github.com/mattn/go-sqlite3"
	_ "github.com/uptrace/bun/dialect/sqlitedialect"
)

// Compile and execute freshly rendered templates, so lifecycle regressions cannot
// pass solely by updating golden text. The temporary package uses this module's
// source and dependency graph, without a workspace or replacement module.
func TestGeneratedResolverRuntime(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	// Copy only the runtime source and locks, also supporting a read-only module cache.
	moduleDir := t.TempDir()
	files := []string{"go.mod", "go.sum"}
	helpers, err := filepath.Glob(filepath.Join(root, "helpers", "*.go"))
	require.NoError(t, err)
	for _, file := range helpers {
		if !strings.HasSuffix(file, "_test.go") {
			files = append(files, filepath.Join("helpers", filepath.Base(file)))
		}
	}
	for _, file := range files {
		content, err := os.ReadFile(filepath.Join(root, file))
		require.NoError(t, err)
		target := filepath.Join(moduleDir, file)
		require.NoError(t, os.MkdirAll(filepath.Dir(target), 0755))
		require.NoError(t, os.WriteFile(target, content, 0644))
	}
	dir := filepath.Join(moduleDir, "generatedfixture")
	pkg := "github.com/goliatone/go-crud/gql/generatedfixture"
	schemas, err := metadata.FromFile(filepath.Join("testdata", "metadata.json"))
	require.NoError(t, err)
	doc, err := formatter.Format(schemas)
	require.NoError(t, err)
	ctx := BuildContext(doc, ContextOptions{EmitSubscriptions: true, SubscriptionEvents: []string{"created"}, HookOptions: hooks.Options{Overlay: overlay.Hooks{Entities: map[string]overlay.EntityHooks{"post": {Operations: map[string]overlay.HookSet{"subscribe": {AuthGuard: `if ctx.Value(actorKey{}) != true { return nil, errors.New("unauthorized") }`, ScopeGuard: `if ctx.Value(denyKey{}) == true { return nil, errors.New("scope denied") }`}}}}}}})
	ctx.ModelPackage = pkg + "/model"
	batch := ctx.Subscriptions[0]
	require.Equal(t, "Post", batch.Entity)
	batch.List = true
	batch.MethodName = "PostBatch"
	ctx.Subscriptions = append(ctx.Subscriptions, batch)
	renderer, err := NewRenderer()
	require.NoError(t, err)
	for name, target := range map[string]string{ModelsTemplate: "model/models_gen.go", ResolverGenTemplate: "resolver_gen.go", ResolverCustomTemplate: "resolver_custom.go"} {
		content, err := renderer.Render(name, ctx)
		require.NoError(t, err)
		dest := filepath.Join(dir, target)
		require.NoError(t, os.MkdirAll(filepath.Dir(dest), 0755))
		require.NoError(t, os.WriteFile(dest, []byte(content), 0644))
	}
	tests, err := os.ReadFile(filepath.Join("testdata", "runtime_test.go.txt"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "runtime_test.go"), []byte(strings.ReplaceAll(string(tests), "FIXTURE_MODEL", pkg+"/model")), 0644))
	cmd := exec.Command("go", "test", "-mod=readonly", "-race", "-count=1", "-timeout=60s", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "generated runtime: %s", out)
}
