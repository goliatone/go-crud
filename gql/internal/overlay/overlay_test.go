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

	base := Overlay{
		Scalars: []Scalar{{Name: "UUID", Description: "base"}},
		Enums:   []Enum{{Name: "Status", Values: []EnumValue{{Name: "BASE"}}}},
	}
	merged := Merge(base, o)

	require.Len(t, merged.Scalars, 2)
	require.Equal(t, "UUID", merged.Scalars[0].Name)
	require.Equal(t, "Date", merged.Scalars[1].Name)
	require.Equal(t, "Custom date scalar", merged.Scalars[1].Description)

	require.Len(t, merged.Enums, 1)
	require.Equal(t, "Status", merged.Enums[0].Name)
	require.Equal(t, "OPEN", merged.Enums[0].Values[0].Name)
}
