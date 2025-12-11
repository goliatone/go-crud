package writer

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Status string

const (
	StatusWritten       Status = "written"
	StatusSkippedSame   Status = "skipped_same"
	StatusSkippedExists Status = "skipped_exists"
	StatusDryRun        Status = "dry_run"
)

// Options configure write behaviour.
type Options struct {
	Force        bool
	DryRun       bool
	RunGoImports bool
}

// Option mutates Options.
type Option func(*Options)

// WithForce forces rewrites for files guarded by WriteCustomOnce.
func WithForce(force bool) Option {
	return func(o *Options) {
		o.Force = force
	}
}

// WithDryRun enables dry-run mode (no writes).
func WithDryRun(dry bool) Option {
	return func(o *Options) {
		o.DryRun = dry
	}
}

// WithGoImports toggles goimports formatting for Go files.
func WithGoImports(enabled bool) Option {
	return func(o *Options) {
		o.RunGoImports = enabled
	}
}

// Writer performs diff-aware writes with optional formatting.
type Writer struct {
	options Options
}

// New creates a writer with provided options.
func New(opts ...Option) *Writer {
	options := Options{
		RunGoImports: true,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	return &Writer{options: options}
}

// WriteGenerated writes (or updates) a generated file, applying formatting for Go files.
func (w *Writer) WriteGenerated(path string, content []byte) (Status, error) {
	formatted, err := w.maybeFormat(path, content)
	if err != nil {
		return Status(""), err
	}
	return w.writeFile(path, formatted, false)
}

// WriteCustomOnce writes a custom stub only if it does not exist (unless forced).
func (w *Writer) WriteCustomOnce(path string, content []byte) (Status, error) {
	if !w.options.Force {
		if _, err := os.Stat(path); err == nil {
			return StatusSkippedExists, nil
		}
	}

	formatted, err := w.maybeFormat(path, content)
	if err != nil {
		return Status(""), err
	}
	return w.writeFile(path, formatted, true)
}

func (w *Writer) writeFile(path string, content []byte, skipSame bool) (Status, error) {
	if skipSame {
		if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, content) {
			return StatusSkippedSame, nil
		}
	} else {
		if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, content) {
			return StatusSkippedSame, nil
		}
	}

	if w.options.DryRun {
		return StatusDryRun, nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Status(""), fmt.Errorf("create dir: %w", err)
	}

	if err := os.WriteFile(path, content, 0o644); err != nil {
		return Status(""), fmt.Errorf("write file: %w", err)
	}

	return StatusWritten, nil
}

func (w *Writer) maybeFormat(path string, content []byte) ([]byte, error) {
	if !strings.HasSuffix(path, ".go") {
		return content, nil
	}

	if w.options.RunGoImports {
		if formatted, err := runGoimports(path, content); err == nil {
			return formatted, nil
		}
		// Fall back to gofmt if goimports fails.
	}

	formatted, err := format.Source(content)
	if err != nil {
		return nil, fmt.Errorf("format go source: %w", err)
	}
	return formatted, nil
}

func runGoimports(path string, content []byte) ([]byte, error) {
	cmd := exec.Command("goimports")
	cmd.Stdin = bytes.NewReader(content)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("run goimports: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	return stdout.Bytes(), nil
}
