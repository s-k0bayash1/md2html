package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func convert(t *testing.T, src string, opts Options) *Result {
	t.Helper()
	res, err := Convert([]byte(src), opts)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	return res
}

func TestBasicDocument(t *testing.T) {
	res := convert(t, "# Hello\n\nsome *text*\n", Options{Lang: "en", FallbackTitle: "fb"})
	html := string(res.HTML)
	for _, want := range []string{
		"<title>Hello</title>",
		`<html lang="en">`,
		"<em>text</em>",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("output missing %q", want)
		}
	}
	if strings.Contains(html, "mermaid.initialize") {
		t.Error("mermaid script included without mermaid blocks")
	}
}

func TestTitleFallbacks(t *testing.T) {
	res := convert(t, "no heading here\n", Options{Lang: "en", FallbackTitle: "myfile"})
	if !strings.Contains(string(res.HTML), "<title>myfile</title>") {
		t.Error("fallback title not used")
	}

	res = convert(t, "---\ntitle: Front Matter Title\nlang: ja\n---\n# H1 Title\n", Options{Lang: "en", FallbackTitle: "fb"})
	html := string(res.HTML)
	if !strings.Contains(html, "<title>Front Matter Title</title>") {
		t.Error("front matter title not used")
	}
	if !strings.Contains(html, `<html lang="ja">`) {
		t.Error("front matter lang not used")
	}
	if strings.Contains(html, "Front Matter Title\nlang") {
		t.Error("front matter leaked into body")
	}
}

func TestGFMTable(t *testing.T) {
	res := convert(t, "| a | b |\n|---|---|\n| 1 | 2 |\n", Options{Lang: "en"})
	html := string(res.HTML)
	if !strings.Contains(html, "<table>") || !strings.Contains(html, "<thead>") {
		t.Error("GFM table not rendered")
	}
	if !strings.Contains(html, "position: sticky") {
		t.Error("sticky header CSS missing")
	}
}

func TestMermaid(t *testing.T) {
	src := "# Doc\n\n```mermaid\ngraph TD;\n  A-->B;\n```\n"
	res := convert(t, src, Options{Lang: "en"})
	html := string(res.HTML)
	if !strings.Contains(html, `<pre class="mermaid">graph TD;`) {
		t.Error("mermaid block not rendered as pre.mermaid")
	}
	if !strings.Contains(html, "mermaid.initialize") {
		t.Error("mermaid init script missing")
	}
	if !strings.Contains(html, "prefers-color-scheme: dark") {
		t.Error("mermaid dark-mode detection missing")
	}
}

func TestSyntaxHighlighting(t *testing.T) {
	src := "```go\nfunc main() {}\n```\n"
	res := convert(t, src, Options{Lang: "en"})
	html := string(res.HTML)
	if !strings.Contains(html, `class="chroma"`) {
		t.Error("chroma classes missing from highlighted code")
	}
	if strings.Contains(html, "mermaid.initialize") {
		t.Error("mermaid script included for a plain code block")
	}
}

func TestRawHTMLPassthrough(t *testing.T) {
	res := convert(t, "<details><summary>open me</summary>hidden</details>\n", Options{Lang: "en"})
	if !strings.Contains(string(res.HTML), "<details>") {
		t.Error("raw HTML was escaped")
	}
}

func TestImageEmbedding(t *testing.T) {
	dir := t.TempDir()
	// Minimal valid 1x1 PNG.
	png, err := base64.StdEncoding.DecodeString(
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "img.png"), png, 0o644); err != nil {
		t.Fatal(err)
	}

	src := "![local](img.png)\n\n![missing](gone.png)\n\n![remote](https://example.com/x.png)\n"
	res := convert(t, src, Options{Lang: "en", EmbedImages: true, BaseDir: dir})
	html := string(res.HTML)

	if !strings.Contains(html, `src="data:image/png;base64,`) {
		t.Error("local image not embedded as data URI")
	}
	if !strings.Contains(html, `src="gone.png"`) {
		t.Error("missing image path not passed through")
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "gone.png") {
		t.Errorf("expected one warning about gone.png, got %v", res.Warnings)
	}
	if !strings.Contains(html, `src="https://example.com/x.png"`) {
		t.Error("remote image was not passed through")
	}
}

func TestNoEmbed(t *testing.T) {
	res := convert(t, "![local](img.png)\n", Options{Lang: "en", EmbedImages: false, BaseDir: "."})
	if !strings.Contains(string(res.HTML), `src="img.png"`) {
		t.Error("image path rewritten despite EmbedImages=false")
	}
	if len(res.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", res.Warnings)
	}
}

func TestHeadingAnchors(t *testing.T) {
	res := convert(t, "## Section One\n", Options{Lang: "en"})
	if !strings.Contains(string(res.HTML), `id="section-one"`) {
		t.Error("auto heading id missing")
	}
}

func TestFootnotes(t *testing.T) {
	res := convert(t, "text[^1]\n\n[^1]: note\n", Options{Lang: "en"})
	if !strings.Contains(string(res.HTML), "footnote") {
		t.Error("footnote not rendered")
	}
}
