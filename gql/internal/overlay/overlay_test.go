package overlay

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadYAMLAndMerge(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "overlay.yaml")
	data := `
scalars:
  - name: Date
    description: Custom date scalar
    go_type: time.Time
enums:
  - name: Status
    values:
      - name: OPEN
      - name: CLOSED
inputs:
  - name: CreateFooInput
    fields:
      - name: name
        type: String
        required: true
queries:
  - name: foo
    return_type: Foo
hooks:
  imports:
    - github.com/example/auth
  default:
    auth_guard: auth.Check(ctx)
  entities:
    bar:
      imports:
        - github.com/example/bar
      all:
        preload: addBarPreload(criteria)
      operations:
        list:
          wrap_repo: wrapBarRepo(svc)
`
	require.NoError(t, os.WriteFile(path, []byte(data), 0o644))

	o, err := Load(path)
	require.NoError(t, err)
	require.Len(t, o.Scalars, 1)
	require.Equal(t, "Date", o.Scalars[0].Name)
	require.Len(t, o.Enums, 1)
	require.Equal(t, "Status", o.Enums[0].Name)
	require.Len(t, o.Inputs, 1)
	require.Equal(t, "CreateFooInput", o.Inputs[0].Name)
	require.Len(t, o.Queries, 1)
	require.Equal(t, "foo", o.Queries[0].Name)
	require.Len(t, o.Hooks.Imports, 1)
	require.Equal(t, "github.com/example/auth", o.Hooks.Imports[0])
	require.Equal(t, "auth.Check(ctx)", o.Hooks.Default.AuthGuard)
	require.Contains(t, o.Hooks.Entities, "bar")
	require.Equal(t, "wrapBarRepo(svc)", o.Hooks.Entities["bar"].Operations["list"].WrapRepo)

	base := Overlay{
		Scalars: []Scalar{{Name: "UUID", Description: "base"}},
		Enums:   []Enum{{Name: "Status", Values: []EnumValue{{Name: "BASE"}}}},
		Hooks: Hooks{
			Imports: []string{"github.com/base/auth"},
			Default: HookSet{AuthGuard: "baseAuth(ctx)"},
			Entities: map[string]EntityHooks{
				"bar": {
					All: HookSet{ScopeGuard: "baseScope(ctx)"},
				},
			},
		},
	}
	merged := Merge(base, o)

	require.Len(t, merged.Scalars, 2)
	require.Equal(t, "UUID", merged.Scalars[0].Name)
	require.Equal(t, "Date", merged.Scalars[1].Name)
	require.Equal(t, "Custom date scalar", merged.Scalars[1].Description)

	require.Len(t, merged.Enums, 1)
	require.Equal(t, "Status", merged.Enums[0].Name)
	require.Equal(t, "OPEN", merged.Enums[0].Values[0].Name)

	require.ElementsMatch(t, []string{"github.com/base/auth", "github.com/example/auth"}, merged.Hooks.Imports)
	require.Equal(t, "auth.Check(ctx)", merged.Hooks.Default.AuthGuard, "extra should override base")
	require.Equal(t, "baseScope(ctx)", merged.Hooks.Entities["bar"].All.ScopeGuard)
	require.Equal(t, "wrapBarRepo(svc)", merged.Hooks.Entities["bar"].Operations["list"].WrapRepo)
}
