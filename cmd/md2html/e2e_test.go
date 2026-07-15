package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// Regenerate golden files with:
//
//	go test ./cmd/md2html -run TestE2E -update
var update = flag.Bool("update", false, "update golden files")

// buildBinary compiles md2html once per test run and returns its path.
var buildBinary = sync.OnceValues(func() (string, error) {
	dir, err := os.MkdirTemp("", "md2html-e2e")
	if err != nil {
		return "", err
	}
	bin := filepath.Join(dir, "md2html")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", bin, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("go build: %v\n%s", err, out)
	}
	return bin, nil
})

func binary(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	bin, err := buildBinary()
	if err != nil {
		t.Fatal(err)
	}
	return bin
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestE2EGolden converts testdata/doc.md and compares the produced HTML
// byte-for-byte with testdata/doc.golden.html.
func TestE2EGolden(t *testing.T) {
	bin := binary(t)
	dir := t.TempDir()

	// The input directory doubles as the image base dir and the default
	// output location, so run against copies in a temp dir to keep
	// testdata pristine.
	copyFile(t, filepath.Join("testdata", "doc.md"), filepath.Join(dir, "doc.md"))
	copyFile(t, filepath.Join("testdata", "img.png"), filepath.Join(dir, "img.png"))

	cmd := exec.Command(bin, filepath.Join(dir, "doc.md"))
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("md2html failed: %v\nstderr: %s", err, stderr.String())
	}

	// The missing image in doc.md must produce a warning but not fail the run.
	if !strings.Contains(stderr.String(), "gone.png") {
		t.Errorf("expected warning about gone.png on stderr, got: %s", stderr.String())
	}

	got, err := os.ReadFile(filepath.Join(dir, "doc.html"))
	if err != nil {
		t.Fatalf("default output file not written: %v", err)
	}

	goldenPath := filepath.Join("testdata", "doc.golden.html")
	if *update {
		if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("updated %s", goldenPath)
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden file: %v (run with -update to create it)", err)
	}
	if !bytes.Equal(got, want) {
		gotPath := filepath.Join(t.TempDir(), "doc.got.html")
		if err := os.WriteFile(gotPath, got, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Errorf("output differs from %s\ngot written to %s\ndiff with: diff %s %s\nregenerate with: go test ./cmd/md2html -run TestE2E -update",
			goldenPath, gotPath, goldenPath, gotPath)
	}
}

// TestE2EMermaid is a substring check rather than a golden file: the
// output embeds the multi-megabyte mermaid.min.js, which is too large
// to keep as a fixture.
func TestE2EMermaid(t *testing.T) {
	bin := binary(t)

	cmd := exec.Command(bin)
	cmd.Stdin = strings.NewReader("# Diagram\n\n```mermaid\ngraph TD;\n  A-->B;\n```\n")
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("md2html failed: %v\nstderr: %s", err, stderr.String())
	}

	html := stdout.String()
	for _, want := range []string{
		`<pre class="mermaid">graph TD;`,
		"mermaid.initialize",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestE2EStdinToStdout(t *testing.T) {
	bin := binary(t)

	cmd := exec.Command(bin)
	cmd.Stdin = strings.NewReader("# From Stdin\n\nhello\n")
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("md2html failed: %v\nstderr: %s", err, stderr.String())
	}

	html := stdout.String()
	for _, want := range []string{
		"<title>From Stdin</title>",
		"<p>hello</p>",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("stdout missing %q", want)
		}
	}
}

func TestE2EMissingInputFile(t *testing.T) {
	bin := binary(t)

	cmd := exec.Command(bin, filepath.Join(t.TempDir(), "nope.md"))
	var stderr strings.Builder
	cmd.Stderr = &stderr
	err := cmd.Run()
	exit, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected non-zero exit, got err=%v", err)
	}
	if code := exit.ExitCode(); code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "md2html:") {
		t.Errorf("expected error message on stderr, got: %s", stderr.String())
	}
}
