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

func TestDirConvertsTopLevelOnly(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.md"), "# A\n")
	writeFile(t, filepath.Join(dir, "b.MD"), "# B\n")
	writeFile(t, filepath.Join(dir, "c.txt"), "not markdown\n")
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(sub, "d.md"), "# D\n")

	var stdout, stderr strings.Builder
	if code := run([]string{dir}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("exit=%d, stderr=%s", code, stderr.String())
	}

	assertExists(t, filepath.Join(dir, "a.html"))
	assertExists(t, filepath.Join(dir, "b.html")) // extension match is case-insensitive
	assertNotExists(t, filepath.Join(dir, "c.html"))
	assertNotExists(t, filepath.Join(sub, "d.html")) // not recursive by default

	for _, want := range []string{"a.md", "b.MD"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout missing converted-file log for %s: %s", want, stdout.String())
		}
	}
}

func TestDirRecursiveSkipsDotDirs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.md"), "# A\n")
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(sub, "d.md"), "# D\n")
	hidden := filepath.Join(dir, ".git")
	if err := os.Mkdir(hidden, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(hidden, "e.md"), "# E\n")

	var stdout, stderr strings.Builder
	if code := run([]string{"-r", dir}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("exit=%d, stderr=%s", code, stderr.String())
	}

	assertExists(t, filepath.Join(dir, "a.html"))
	assertExists(t, filepath.Join(sub, "d.html"))
	assertNotExists(t, filepath.Join(hidden, "e.html"))
}

func TestDirRejectsOutputFlag(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.md"), "# A\n")

	var stdout, stderr strings.Builder
	code := run([]string{"-o", filepath.Join(dir, "out.html"), dir}, strings.NewReader(""), &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected non-zero exit when -o is combined with a directory input")
	}
	if !strings.Contains(stderr.String(), "-o is not supported") {
		t.Errorf("expected -o rejection message, got: %s", stderr.String())
	}
}

func TestDirSkipsExistingHTMLWithoutForce(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.md"), "# A\n")
	existing := filepath.Join(dir, "a.html")
	writeFile(t, existing, "stale")

	var stdout, stderr strings.Builder
	if code := run([]string{dir}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("expected exit 0 on skip, got %d, stderr=%s", code, stderr.String())
	}
	got, err := os.ReadFile(existing)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "stale" {
		t.Errorf("existing .html was overwritten without -force")
	}
	if !strings.Contains(stderr.String(), "already exists") {
		t.Errorf("expected skip warning, got: %s", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"-force", dir}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("exit=%d, stderr=%s", code, stderr.String())
	}
	got, err = os.ReadFile(existing)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) == "stale" {
		t.Errorf("-force did not overwrite existing .html")
	}
}

func TestDirContinuesAfterPerFileError(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "good.md"), "# Good\n")
	bad := filepath.Join(dir, "bad.md")
	writeFile(t, bad, "unreadable")
	if err := os.Chmod(bad, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(bad, 0o644) })

	var stdout, stderr strings.Builder
	code := run([]string{dir}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit=%d, want 1; stderr=%s", code, stderr.String())
	}
	assertExists(t, filepath.Join(dir, "good.html"))
}

func TestDirNoMarkdownFilesIsNotAnError(t *testing.T) {
	dir := t.TempDir()

	var stdout, stderr strings.Builder
	if code := run([]string{dir}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("exit=%d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "no .md files found") {
		t.Errorf("expected no-files warning, got: %s", stderr.String())
	}
}

func TestRecursiveFlagIgnoredForSingleFile(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(in, []byte("# Hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr strings.Builder
	if code := run([]string{"-r", in}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("exit=%d, stderr=%s", code, stderr.String())
	}
	assertExists(t, filepath.Join(dir, "doc.html"))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected %s to exist: %v", path, err)
	}
}

func assertNotExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Errorf("expected %s not to exist", path)
	}
}
