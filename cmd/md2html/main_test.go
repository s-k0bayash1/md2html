package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFlagsAfterFileArgument(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(in, []byte("# Hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out.html")

	var stdout, stderr strings.Builder
	if code := run([]string{in, "-o", out}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("exit=%d, stderr=%s", code, stderr.String())
	}
	if _, err := os.Stat(out); err != nil {
		t.Errorf("output file not written: %v", err)
	}
}

func TestRefusesToOverwriteInput(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(in, []byte("# Hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr strings.Builder
	if code := run([]string{"-o", in, in}, strings.NewReader(""), &stdout, &stderr); code == 0 {
		t.Fatal("expected non-zero exit when output would overwrite input")
	}
	if !strings.Contains(stderr.String(), "overwrite") {
		t.Errorf("expected overwrite error, got: %s", stderr.String())
	}
}
