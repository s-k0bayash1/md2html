// md2html converts a Markdown file into a single self-contained HTML file.
package main

import (
	"flag"
	"fmt"
	"io"
	"io/fs"
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
	fset := flag.NewFlagSet("md2html", flag.ContinueOnError)
	fset.SetOutput(stderr)
	out := fset.String("o", "", "output file path (default: input file with .html extension, or stdout for stdin input); not allowed when input is a directory")
	lang := fset.String("lang", "en", "html lang attribute (front matter \"lang\" takes precedence)")
	noEmbed := fset.Bool("no-embed", false, "do not embed local images as data URIs")
	recursive := fset.Bool("r", false, "when input is a directory, also convert files in subdirectories (dot-directories are skipped)")
	force := fset.Bool("force", false, "when input is a directory, overwrite .html files that already exist")
	showVersion := fset.Bool("version", false, "print version and exit")
	fset.Usage = func() {
		fmt.Fprintf(stderr, "Usage: md2html [flags] [file.md|dir]\n\nReads from stdin when no file (or \"-\") is given.\nWhen given a directory, converts every .md file directly under it;\nuse -r to also descend into subdirectories.\n\nFlags:\n")
		fset.PrintDefaults()
	}
	if err := fset.Parse(args); err != nil {
		return 2
	}
	// flag stops at the first positional argument; keep parsing so
	// "md2html file.md -o out.html" works as documented.
	var positional []string
	for rest := fset.Args(); len(rest) > 0; rest = fset.Args() {
		positional = append(positional, rest[0])
		if err := fset.Parse(rest[1:]); err != nil {
			return 2
		}
	}
	if *showVersion {
		fmt.Fprintln(stdout, "md2html "+version)
		return 0
	}
	if len(positional) > 1 {
		fmt.Fprintln(stderr, "md2html: too many arguments")
		fset.Usage()
		return 2
	}

	input := "-"
	if len(positional) == 1 {
		input = positional[0]
	}

	if input != "-" {
		if info, err := os.Stat(input); err == nil && info.IsDir() {
			return runDir(input, *lang, !*noEmbed, *recursive, *force, *out, stdout, stderr, fset)
		}
	}

	return runFile(input, *out, *lang, !*noEmbed, stdin, stdout, stderr)
}

// runFile converts a single Markdown file (or stdin) — the original,
// unchanged single-input behavior.
func runFile(input, out, lang string, embedImages bool, stdin io.Reader, stdout, stderr io.Writer) int {
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
		Lang:          lang,
		EmbedImages:   embedImages,
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

	outPath := out
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

// runDir converts every Markdown file found under dir. Each file is
// written next to itself with a .html extension, same as the single-file
// default. Errors on individual files are reported and skipped rather
// than aborting the whole run.
func runDir(dir, lang string, embedImages, recursive, force bool, out string, stdout, stderr io.Writer, fset *flag.FlagSet) int {
	if out != "" {
		fmt.Fprintln(stderr, "md2html: -o is not supported when input is a directory")
		fset.Usage()
		return 2
	}

	files, err := collectMarkdownFiles(dir, recursive)
	if err != nil {
		fmt.Fprintf(stderr, "md2html: %v\n", err)
		return 1
	}
	if len(files) == 0 {
		fmt.Fprintf(stderr, "md2html: warning: no .md files found under %q\n", dir)
		return 0
	}

	hadError := false
	for _, path := range files {
		if !convertFileInDir(path, lang, embedImages, force, stdout, stderr) {
			hadError = true
		}
	}
	if hadError {
		return 1
	}
	return 0
}

// convertFileInDir converts one file as part of a directory run and
// reports success. Missing an already-converted output isn't an error:
// it's reported as a warning and reported as success so a run without
// -force stays exit-code 0.
func convertFileInDir(path, lang string, embedImages, force bool, stdout, stderr io.Writer) bool {
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "md2html: %v\n", err)
		return false
	}
	base := filepath.Base(path)

	res, err := Convert(src, Options{
		Lang:          lang,
		EmbedImages:   embedImages,
		BaseDir:       filepath.Dir(path),
		FallbackTitle: strings.TrimSuffix(base, filepath.Ext(base)),
	})
	if err != nil {
		fmt.Fprintf(stderr, "md2html: %s: %v\n", path, err)
		return false
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(stderr, "md2html: warning: %s: %s\n", path, w)
	}

	outPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".html"
	if !force {
		if _, err := os.Stat(outPath); err == nil {
			fmt.Fprintf(stderr, "md2html: warning: %s already exists, skipping %s (use -force to overwrite)\n", outPath, path)
			return true
		} else if !os.IsNotExist(err) {
			fmt.Fprintf(stderr, "md2html: %v\n", err)
			return false
		}
	}
	if err := os.WriteFile(outPath, res.HTML, 0o644); err != nil {
		fmt.Fprintf(stderr, "md2html: %v\n", err)
		return false
	}
	fmt.Fprintf(stdout, "converted: %s -> %s\n", path, outPath)
	return true
}

// collectMarkdownFiles lists the .md files directly under dir, or under
// its whole tree when recursive is set. Dot-directories (e.g. ".git")
// are skipped during recursive descent.
func collectMarkdownFiles(dir string, recursive bool) ([]string, error) {
	if !recursive {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		var files []string
		for _, e := range entries {
			if !e.IsDir() && isMarkdownFile(e.Name()) {
				files = append(files, filepath.Join(dir, e.Name()))
			}
		}
		return files, nil
	}

	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != dir && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if isMarkdownFile(d.Name()) {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

// isMarkdownFile reports whether name is a .md file to convert. Dotfiles
// (e.g. ".md" itself, or hidden files) are excluded.
func isMarkdownFile(name string) bool {
	if strings.HasPrefix(name, ".") {
		return false
	}
	return strings.EqualFold(filepath.Ext(name), ".md")
}

func samePath(a, b string) bool {
	aa, errA := filepath.Abs(a)
	bb, errB := filepath.Abs(b)
	return errA == nil && errB == nil && aa == bb
}
