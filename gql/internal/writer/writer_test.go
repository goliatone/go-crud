package writer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteGenerated_FormatsAndSkipsIdentical(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample_gen.go")

	w := New()
	status, err := w.WriteGenerated(path, []byte("package main\nimport \"fmt\"\nfunc hi(){fmt.Println(\"hi\")}\n"))
	if err != nil {
		t.Fatalf("write generated: %v", err)
	}
	if status != StatusWritten {
		t.Fatalf("expected status written, got %s", status)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(content) != "package main\n\nimport \"fmt\"\n\nfunc hi() { fmt.Println(\"hi\") }\n" {
		t.Fatalf("unexpected formatted content:\n%s", string(content))
	}

	status, err = w.WriteGenerated(path, content)
	if err != nil {
		t.Fatalf("write generated second time: %v", err)
	}
	if status != StatusSkippedSame {
		t.Fatalf("expected skip on identical content, got %s", status)
	}
}

func TestWriteCustomOnce_RespectsExistingAndForce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "resolver_custom.go")
	if err := os.WriteFile(path, []byte("existing"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	w := New()
	status, err := w.WriteCustomOnce(path, []byte("new content"))
	if err != nil {
		t.Fatalf("write custom once: %v", err)
	}
	if status != StatusSkippedExists {
		t.Fatalf("expected skip when file exists, got %s", status)
	}

	w = New(WithForce(true))
	status, err = w.WriteCustomOnce(path, []byte("package main\n\nfunc custom() {}\n"))
	if err != nil {
		t.Fatalf("write custom forced: %v", err)
	}
	if status != StatusWritten {
		t.Fatalf("expected write when forced, got %s", status)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "package main\n\nfunc custom() {}\n" {
		t.Fatalf("expected overwritten content, got %q", string(data))
	}
}

func TestWriteGenerated_DryRun(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dry_gen.go")

	w := New(WithDryRun(true))
	status, err := w.WriteGenerated(path, []byte("package main\n\nfunc a(){}"))
	if err != nil {
		t.Fatalf("dry-run write: %v", err)
	}
	if status != StatusDryRun {
		t.Fatalf("expected dry-run status, got %s", status)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected no file written in dry-run, got err=%v", err)
	}
}
