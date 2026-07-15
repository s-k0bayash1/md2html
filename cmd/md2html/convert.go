package main

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	meta "github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

//go:embed assets/style.css
var baseCSS string

//go:embed assets/mermaid.min.js
var mermaidJS string

//go:embed assets/page.tmpl
var pageTemplateSrc string

var pageTemplate = template.Must(template.New("page").Parse(pageTemplateSrc))

// Options controls a single Markdown-to-HTML conversion.
type Options struct {
	Lang          string // html lang attribute; front matter "lang" wins
	EmbedImages   bool   // embed local images as data URIs
	BaseDir       string // directory local image paths are resolved against
	FallbackTitle string // used when neither front matter title nor an h1 exists
}

// Result is the outcome of a conversion.
type Result struct {
	HTML     []byte
	Warnings []string
}

type pageData struct {
	Lang      string
	Title     string
	CSS       template.CSS
	Body      template.HTML
	MermaidJS template.JS
}

// Convert renders Markdown source into a self-contained HTML document.
func Convert(src []byte, opts Options) (*Result, error) {
	embedder := &imageEmbedder{baseDir: opts.BaseDir}
	parserOpts := []parser.Option{parser.WithAutoHeadingID()}
	if opts.EmbedImages {
		parserOpts = append(parserOpts, parser.WithASTTransformers(util.Prioritized(embedder, 200)))
	}

	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Footnote,
			meta.Meta,
			highlighting.NewHighlighting(
				highlighting.WithFormatOptions(chromahtml.WithClasses(true)),
			),
			mermaidExtension{},
		),
		goldmark.WithParserOptions(parserOpts...),
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)

	ctx := parser.NewContext()
	doc := md.Parser().Parse(text.NewReader(src), parser.WithContext(ctx))
	var body bytes.Buffer
	if err := md.Renderer().Render(&body, src, doc); err != nil {
		return nil, fmt.Errorf("render: %w", err)
	}
	metaData := meta.Get(ctx)

	title := opts.FallbackTitle
	if h1 := firstH1(doc, src); h1 != "" {
		title = h1
	}
	if t, ok := metaData["title"].(string); ok && strings.TrimSpace(t) != "" {
		title = t
	}
	lang := opts.Lang
	if l, ok := metaData["lang"].(string); ok && strings.TrimSpace(l) != "" {
		lang = l
	}

	data := pageData{
		Lang:  lang,
		Title: title,
		CSS:   template.CSS(pageCSS),
		Body:  template.HTML(body.String()),
	}
	if containsMermaid(doc) {
		data.MermaidJS = template.JS(mermaidJS)
	}

	var out bytes.Buffer
	if err := pageTemplate.Execute(&out, data); err != nil {
		return nil, fmt.Errorf("template: %w", err)
	}
	return &Result{HTML: out.Bytes(), Warnings: embedder.warnings}, nil
}

// pageCSS combines the base stylesheet with chroma syntax-highlighting
// styles. Each chroma palette is scoped to its own media query: the two
// styles define different token sets, so an unscoped light rule would
// bleed into dark mode as near-invisible dark-on-dark text.
var pageCSS = buildCSS()

func buildCSS() string {
	var sb strings.Builder
	sb.WriteString(baseCSS)
	f := chromahtml.New(chromahtml.WithClasses(true))
	for _, p := range []struct{ style, scheme string }{
		{"github", "light"},
		{"github-dark", "dark"},
	} {
		var buf bytes.Buffer
		if err := f.WriteCSS(&buf, styles.Get(p.style)); err != nil {
			panic(err)
		}
		sb.WriteString("@media (prefers-color-scheme: " + p.scheme + "){")
		sb.Write(buf.Bytes())
		sb.WriteString("}")
	}
	return sb.String()
}

func firstH1(doc ast.Node, src []byte) string {
	var title string
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok && h.Level == 1 {
			title = nodeText(h, src)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return strings.TrimSpace(title)
}

func nodeText(n ast.Node, src []byte) string {
	var sb strings.Builder
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch t := c.(type) {
		case *ast.Text:
			sb.Write(t.Segment.Value(src))
		case *ast.String:
			sb.Write(t.Value)
		}
		return ast.WalkContinue, nil
	})
	return sb.String()
}

// --- mermaid ---

// mermaidExtension renders ```mermaid fenced code blocks as
// <pre class="mermaid"> elements for client-side mermaid.js.
type mermaidExtension struct{}

func (mermaidExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(parser.WithASTTransformers(util.Prioritized(&mermaidTransformer{}, 100)))
	m.Renderer().AddOptions(renderer.WithNodeRenderers(util.Prioritized(&mermaidRenderer{}, 100)))
}

type mermaidBlock struct {
	ast.BaseBlock
}

var kindMermaidBlock = ast.NewNodeKind("MermaidBlock")

func (*mermaidBlock) Kind() ast.NodeKind { return kindMermaidBlock }

func (n *mermaidBlock) Dump(src []byte, level int) {
	ast.DumpHelper(n, src, level, nil, nil)
}

// mermaidTransformer swaps ```mermaid fenced code blocks for mermaidBlock
// nodes so the highlighting extension never sees them.
type mermaidTransformer struct{}

func (t *mermaidTransformer) Transform(doc *ast.Document, reader text.Reader, pc parser.Context) {
	var targets []*ast.FencedCodeBlock
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if fcb, ok := n.(*ast.FencedCodeBlock); ok {
			if string(fcb.Language(reader.Source())) == "mermaid" {
				targets = append(targets, fcb)
			}
		}
		return ast.WalkContinue, nil
	})
	for _, fcb := range targets {
		m := &mermaidBlock{}
		m.SetLines(fcb.Lines())
		fcb.Parent().ReplaceChild(fcb.Parent(), fcb, m)
	}
}

func containsMermaid(doc ast.Node) bool {
	found := false
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering && n.Kind() == kindMermaidBlock {
			found = true
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return found
}

type mermaidRenderer struct{}

func (r *mermaidRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindMermaidBlock, r.render)
}

func (r *mermaidRenderer) render(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString(`<pre class="mermaid">`)
		lines := node.Lines()
		for i := 0; i < lines.Len(); i++ {
			seg := lines.At(i)
			_, _ = w.Write(util.EscapeHTML(seg.Value(source)))
		}
	} else {
		_, _ = w.WriteString("</pre>\n")
	}
	return ast.WalkContinue, nil
}

// --- image embedding ---

// imageEmbedder rewrites local image destinations into data URIs so the
// output HTML stays self-contained.
type imageEmbedder struct {
	baseDir  string
	warnings []string
}

func (t *imageEmbedder) Transform(doc *ast.Document, reader text.Reader, pc parser.Context) {
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		img, ok := n.(*ast.Image)
		if !ok {
			return ast.WalkContinue, nil
		}
		dest := string(img.Destination)
		if dest == "" || isRemote(dest) {
			return ast.WalkContinue, nil
		}
		uri, err := t.dataURI(dest)
		if err != nil {
			t.warnings = append(t.warnings, fmt.Sprintf("could not embed image %q: %v", dest, err))
			return ast.WalkContinue, nil
		}
		img.Destination = uri
		return ast.WalkContinue, nil
	})
}

func (t *imageEmbedder) dataURI(dest string) ([]byte, error) {
	p := dest
	if u, err := url.PathUnescape(p); err == nil {
		p = u
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(t.baseDir, filepath.FromSlash(p))
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	prefix := "data:" + imageMIME(p, data) + ";base64,"
	uri := make([]byte, len(prefix)+base64.StdEncoding.EncodedLen(len(data)))
	copy(uri, prefix)
	base64.StdEncoding.Encode(uri[len(prefix):], data)
	return uri, nil
}

func isRemote(dest string) bool {
	if strings.HasPrefix(dest, "//") {
		return true
	}
	u, err := url.Parse(dest)
	if err != nil {
		return true // leave anything unparseable untouched
	}
	// A one-letter scheme is more likely a Windows drive path than a URL.
	return len(u.Scheme) > 1
}

func imageMIME(path string, data []byte) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	case ".webp":
		return "image/webp"
	case ".avif":
		return "image/avif"
	case ".bmp":
		return "image/bmp"
	case ".ico":
		return "image/x-icon"
	default:
		return http.DetectContentType(data)
	}
}
