package main

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"

	"github.com/lincyaw/unicode/internal/design"
	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
	"github.com/lincyaw/unicode/internal/lang/golang"
	"github.com/lincyaw/unicode/internal/syntax"
)

// A backend is how the toolchain extracts one language: the backend itself,
// and the command its toolchain needs on PATH.
type backend struct {
	lang.Backend
	tool string
}

// backends are the languages the toolchain can extract, by the name the
// system file gives them; a language not here is reported, not extracted.
func backends() map[string]backend {
	return map[string]backend{"go": {Backend: golang.NewBackend(), tool: "go"}}
}

// readDesign reads and parses every file under design/ and corpus/. Its
// diagnostics are those of reading and parsing each file, and
// cli.not-canonical for a file that parses but is not canonical.
func readDesign(root string) ([]syntax.File, []diag.Diagnostic, error) {
	paths, err := fmtPaths(root, nil)
	if err != nil {
		return nil, nil, err
	}
	var diags []diag.Diagnostic
	var files []syntax.File
	for _, rel := range paths {
		src, f, ds := readFile(root, rel)
		diags = append(diags, ds...)
		if f == nil {
			continue
		}
		if out := f.Print(); !bytes.Equal(out, src) {
			diags = append(diags, notCanonical(rel, src, out))
		}
		files = append(files, *f)
	}
	return files, diags, nil
}

// buildGraph builds the graph of the repository at root (docs/design.md §5):
// it loads the design, extracts the code under the roots of every language
// the system file configures with that language's backend, adds each
// fragment and derives. The diagnostics are those of reading the files,
// loading the design, extracting the code and deriving, unsorted.
func buildGraph(root string) (graph.Graph, []diag.Diagnostic, error) {
	return buildGraphWithExtractor(root, lang.Backend.Extract)
}

// buildGraphForGeneration lets a backend repair stale generated output while
// keeping the ordinary extraction path strict.
func buildGraphForGeneration(root string) (graph.Graph, []diag.Diagnostic, error) {
	return buildGraphWithExtractor(root, func(b lang.Backend, root string, roots []string) (lang.Extracted, error) {
		if extractor, ok := b.(lang.GenerationExtractor); ok {
			return extractor.ExtractForGeneration(root, roots)
		}
		return b.Extract(root, roots)
	})
}

func buildGraphWithExtractor(root string, extract func(lang.Backend, string, []string) (lang.Extracted, error)) (graph.Graph, []diag.Diagnostic, error) {
	files, diags, err := readDesign(root)
	if err != nil {
		return graph.Graph{}, nil, err
	}
	loaded := design.Load(files)
	g := loaded.Graph
	diags = append(diags, loaded.Diagnostics...)
	available := backends()
	for _, r := range languageRoots(&g) {
		at := languagePosition(files, r.language)
		b, known := available[r.language]
		if !known {
			diags = append(diags, diag.Diagnostic{
				Code: "cli.unsupported-language", Subject: at.Path, Position: at,
				Message: fmt.Sprintf("the system file configures %s, which this version of unicode cannot extract yet; its code is not checked", r.language),
				Fix:     fmt.Sprintf("remove the language %s from design/system.uc until unicode supports it", r.language),
			})
			continue
		}
		if _, err := exec.LookPath(b.tool); err != nil {
			return graph.Graph{}, nil, &exitError{code: exitEnvironment,
				err: fmt.Errorf("extracting the %s code needs the command %s on PATH: %w", r.language, b.tool, err)}
		}
		extracted, err := extract(b.Backend, root, r.dirs)
		var failed lang.ExtractFailed
		switch {
		case errors.As(err, &failed):
			diags = append(diags, diag.Diagnostic{
				Code: "cli.extract-failed", Subject: at.Path, Position: at,
				Message: strings.ReplaceAll(err.Error(), "\n", "; "),
				Fix:     fmt.Sprintf("make the %s code under the roots %s load without errors, or correct the roots in design/system.uc", r.language, strings.Join(r.dirs, ", ")),
			})
			continue
		case err != nil:
			return graph.Graph{}, nil, &exitError{code: exitUnexpected, err: err}
		}
		if err := g.AddFragment(extracted.Fragment); err != nil {
			return graph.Graph{}, nil, &exitError{code: exitUnexpected, err: fmt.Errorf("the %s code does not merge into the graph: %w", r.language, err)}
		}
		diags = append(diags, extracted.Diagnostics...)
	}
	return g, append(diags, g.Derive()...), nil
}

// languageRoot is one configured language and its source roots.
type languageRoot struct {
	language string
	dirs     []string
}

// languageRoots reads the roots of the system node, each written
// `language: dir`, grouped by language in the order they first appear.
func languageRoots(g *graph.Graph) []languageRoot {
	var out []languageRoot
	for _, n := range g.Nodes() {
		if n.Layer != graph.LayerDesign || n.Kind != graph.NodeKindContainer || n.Form != "system" {
			continue
		}
		for _, r := range n.Attrs["roots"] {
			language, dir, _ := strings.Cut(r, ": ")
			i := slices.IndexFunc(out, func(l languageRoot) bool { return l.language == language })
			if i < 0 {
				out = append(out, languageRoot{language: language})
				i = len(out) - 1
			}
			out[i].dirs = append(out[i].dirs, dir)
		}
	}
	return out
}

// languagePosition is where the system file declares the language.
func languagePosition(files []syntax.File, language string) diag.Position {
	at := diag.Position{Path: "design/system.uc", Line: 1, Column: 1}
	for _, f := range files {
		if f.Path != at.Path {
			continue
		}
		for _, d := range f.Decls {
			if d, ok := d.(*syntax.LanguageDecl); ok && d.Name == language {
				return diag.Position{Path: f.Path, Line: d.Pos.Line, Column: d.Pos.Column}
			}
		}
	}
	return at
}
