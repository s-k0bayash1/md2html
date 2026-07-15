// md2html converts a Markdown file into a single self-contained HTML file.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Overridden at release time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("md2html", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("o", "", "output file path (default: input file with .html extension, or stdout for stdin input)")
	lang := fs.String("lang", "en", "html lang attribute (front matter \"lang\" takes precedence)")
	noEmbed := fs.Bool("no-embed", false, "do not embed local images as data URIs")
	showVersion := fs.Bool("version", false, "print version and exit")
	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: md2html [flags] [file.md]\n\nReads from stdin when no file (or \"-\") is given.\n\nFlags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	// flag stops at the first positional argument; keep parsing so
	// "md2html file.md -o out.html" works as documented.
	var positional []string
	for rest := fs.Args(); len(rest) > 0; rest = fs.Args() {
		positional = append(positional, rest[0])
		if err := fs.Parse(rest[1:]); err != nil {
			return 2
		}
	}
	if *showVersion {
		fmt.Fprintln(stdout, "md2html "+version)
		return 0
	}
	if len(positional) > 1 {
		fmt.Fprintln(stderr, "md2html: too many arguments")
		fs.Usage()
		return 2
	}

	input := "-"
	if len(positional) == 1 {
		input = positional[0]
	}
	fromStdin := input == "-"

	var src []byte
	var err error
	baseDir := "."
	fallbackTitle := "Document"
	if fromStdin {
		src, err = io.ReadAll(stdin)
	} else {
		src, err = os.ReadFile(input)
		baseDir = filepath.Dir(input)
		base := filepath.Base(input)
		fallbackTitle = strings.TrimSuffix(base, filepath.Ext(base))
	}
	if err != nil {
		fmt.Fprintf(stderr, "md2html: %v\n", err)
		return 1
	}

	res, err := Convert(src, Options{
		Lang:          *lang,
		EmbedImages:   !*noEmbed,
		BaseDir:       baseDir,
		FallbackTitle: fallbackTitle,
	})
	if err != nil {
		fmt.Fprintf(stderr, "md2html: %v\n", err)
		return 1
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(stderr, "md2html: warning: %s\n", w)
	}

	outPath := *out
	if outPath == "" {
		if fromStdin {
			outPath = "-"
		} else {
			outPath = strings.TrimSuffix(input, filepath.Ext(input)) + ".html"
		}
	}
	if outPath == "-" {
		if _, err := stdout.Write(res.HTML); err != nil {
			fmt.Fprintf(stderr, "md2html: %v\n", err)
			return 1
		}
		return 0
	}
	if !fromStdin && samePath(input, outPath) {
		fmt.Fprintf(stderr, "md2html: output %q would overwrite the input file\n", outPath)
		return 1
	}
	if err := os.WriteFile(outPath, res.HTML, 0o644); err != nil {
		fmt.Fprintf(stderr, "md2html: %v\n", err)
		return 1
	}
	return 0
}

func samePath(a, b string) bool {
	aa, errA := filepath.Abs(a)
	bb, errB := filepath.Abs(b)
	return errA == nil && errB == nil && aa == bb
}
