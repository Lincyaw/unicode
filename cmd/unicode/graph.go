package main

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"os/exec"
	"slices"
	"strings"

	"github.com/lincyaw/unicode/internal/design"
	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/evidence"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
	"github.com/lincyaw/unicode/internal/lang/golang"
	"github.com/lincyaw/unicode/internal/review"
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

// readDesign reads every file under design/ and corpus/ as a source. Its
// diagnostics are those of reading each file, and cli.not-canonical for a
// file that parses but is not canonical; the diagnostics of parsing and
// loading the sources are design.loadSources's.
func readDesign(root string) ([]syntax.Source, []diag.Diagnostic, error) {
	paths, err := fmtPaths(root, nil)
	if err != nil {
		return nil, nil, err
	}
	var diags []diag.Diagnostic
	var sources []syntax.Source
	for _, rel := range paths {
		source, ds := readSource(root, rel)
		diags = append(diags, ds...)
		if source == nil {
			continue
		}
		if f := syntax.Parse(rel, source.Kind, source.Text).File; f != nil {
			if out := f.Print(); !bytes.Equal(out, source.Text) {
				diags = append(diags, notCanonical(rel, source.Text, out))
			}
		}
		sources = append(sources, *source)
	}
	return sources, diags, nil
}

// buildGraph builds the graph of the repository at root (docs/design.md §5):
// it loads the design, extracts the code under the roots of every language
// the system file configures with that language's backend, adds each
// fragment and derives. The diagnostics are those of reading the files,
// loading the design, extracting the code and deriving, unsorted.
func buildGraph(root string) (graph.Graph, []diag.Diagnostic, error) {
	return attachedGraph(root, false)
}

// buildSnapshotGraph is buildGraph for unicode build, which alone gives the
// runtime run records the execution-input digests of their commits, so that
// their conclusions can be current evidence (docs/design.md §6.6).
func buildSnapshotGraph(root string) (graph.Graph, []diag.Diagnostic, error) {
	return attachedGraph(root, true)
}

func attachedGraph(root string, digests bool) (graph.Graph, []diag.Diagnostic, error) {
	g, diagnostics, err := buildInputGraph(root)
	if err != nil {
		return g, diagnostics, err
	}
	runs := evidence.Read(root)
	if digests {
		diagnostics = append(diagnostics, runtimeDigests(root, &g, runs.Reports)...)
	}
	reviews := review.Read(root)
	diagnostics = append(diagnostics, runs.Diagnostics...)
	diagnostics = append(diagnostics, reviews.Diagnostics...)
	measured := evidence.Attach(&g, runs.Reports)
	reviewed := review.Attach(&g, reviews.Records)
	diagnostics = append(diagnostics, measured.Diagnostics...)
	diagnostics = append(diagnostics, reviewed.Diagnostics...)
	for _, fragment := range []graph.Fragment{measured.Fragment, reviewed.Fragment} {
		if err := g.AddFragment(fragment); err != nil {
			return g, diagnostics, &exitError{code: exitUnexpected, err: err}
		}
	}
	return g, diagnostics, nil
}

func buildInputGraph(root string) (graph.Graph, []diag.Diagnostic, error) {
	return buildGraphWithMode(root, graphLoadStrict)
}

// buildGraphForGeneration lets a backend repair stale generated output while
// keeping the ordinary extraction path strict.
func buildGraphForGeneration(root string) (graph.Graph, []diag.Diagnostic, error) {
	return buildGraphWithMode(root, graphLoadGeneration)
}

func buildGraphForExploration(root string) (graph.Graph, []diag.Diagnostic, error) {
	return buildGraphWithMode(root, graphLoadExploration)
}

func generationExtract(b lang.Backend, root string, roots []string) (lang.Extracted, error) {
	if extractor, ok := b.(lang.GenerationExtractor); ok {
		return extractor.ExtractForGeneration(root, roots)
	}
	return b.Extract(root, roots)
}

type graphLoadMode uint8

const (
	graphLoadStrict graphLoadMode = iota
	graphLoadGeneration
	graphLoadExploration
)

func buildGraphWithMode(root string, mode graphLoadMode) (graph.Graph, []diag.Diagnostic, error) {
	sources, diags, err := readDesign(root)
	if err != nil {
		return graph.Graph{}, nil, err
	}
	var loaded design.Loaded
	if mode == graphLoadExploration {
		loaded = design.LoadSourcesForExploration(sources).Loaded
	} else {
		loaded = design.LoadSources(sources)
	}
	g := loaded.Graph
	diags = append(diags, loaded.Diagnostics...)
	available := backends()
	for _, r := range languageRoots(&g) {
		at := languagePosition(sources, r.language)
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
		var extracted lang.Extracted
		if mode == graphLoadStrict {
			extracted, err = b.Extract(root, r.dirs)
		} else {
			extracted, err = generationExtract(b.Backend, root, r.dirs)
		}
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

// languageRoots lists the configured languages, sorted, each with its
// source roots as lang.LanguageRoots reads them.
func languageRoots(g *graph.Graph) []languageRoot {
	roots := lang.LanguageRoots(g)
	var out []languageRoot
	for _, language := range slices.Sorted(maps.Keys(roots)) {
		out = append(out, languageRoot{language: language, dirs: roots[language]})
	}
	return out
}

// languagePosition is where the system file declares the language.
func languagePosition(sources []syntax.Source, language string) diag.Position {
	at := diag.Position{Path: "design/system.uc", Line: 1, Column: 1}
	for _, s := range sources {
		if s.Path != at.Path {
			continue
		}
		f := syntax.Parse(s.Path, s.Kind, s.Text).File
		if f == nil {
			break
		}
		for _, d := range f.Decls {
			if d, ok := d.(*syntax.LanguageDecl); ok && d.Name == language {
				return diag.Position{Path: f.Path, Line: d.Pos.Line, Column: d.Pos.Column}
			}
		}
	}
	return at
}
