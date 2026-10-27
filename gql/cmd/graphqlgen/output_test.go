package main

import (
	"io"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

type failedOutput struct{}

func (failedOutput) Write([]byte) (int, error) {
	return 0, io.ErrClosedPipe
}

func TestCLI_OutputError(t *testing.T) {
	outDir := t.TempDir()
	err := run(t.Context(), []string{
		"--metadata-file", filepath.Join("..", "..", "internal", "templates", "testdata", "metadata.json"),
		"--out", outDir,
		"--config", filepath.Join(outDir, "gqlgen.yml"),
		"--dry-run",
		"--goimports=false",
	}, failedOutput{})
	require.ErrorIs(t, err, io.ErrClosedPipe)
}
