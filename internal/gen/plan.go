package gen

import (
	"bytes"
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
)

// Plan asks each configured language's generator for its files and merges
// them with the diagnostics of duplicate backends and of languages without
// a generator. It does not require missing implementations or predicates to
// pass conformance first.
//
//uc:realizes gen.plan
func Plan(root string, g *graph.Graph, backends []lang.Backend) lang.Generation {
	own := lang.Generation{}
	available := map[string]lang.Backend{}
	for _, b := range backends {
		name := b.Language()
		if _, exists := available[name]; exists {
			own.Diagnostics = append(own.Diagnostics, diagnostic("gen.duplicate-backend", "design/system.uc", "more than one backend provides "+name, "register exactly one backend for each language"))
		}
		available[name] = b
	}
	var generations []lang.Generation
	roots := lang.LanguageRoots(g)
	for _, name := range slices.Sorted(maps.Keys(roots)) {
		b, ok := available[name].(lang.Generator)
		if !ok {
			own.Diagnostics = append(own.Diagnostics, diagnostic("gen.unsupported-language", "design/system.uc", "no generation backend provides "+name, "register a generator for this language, or remove its language roots"))
			continue
		}
		generations = append(generations, b.Generate(lang.GenPlan{Root: root, Roots: roots[name], Graph: g}))
	}
	return Merge(append(generations, own))
}

// Merge joins generation plans into one: the files sorted by path, content
// and createOnly, the diagnostics sorted, and a gen.duplicate-path for every
// file of a path after its first, so that the result does not depend on the
// order of the plans.
//
//uc:realizes gen.merge
func Merge(generations []lang.Generation) lang.Generation {
	out := lang.Generation{Edits: []lang.Edit{}, Diagnostics: []diag.Diagnostic{}}
	for _, g := range generations {
		out.Edits = append(out.Edits, g.Edits...)
		out.Diagnostics = append(out.Diagnostics, g.Diagnostics...)
	}
	slices.SortFunc(out.Edits, func(a, b lang.Edit) int {
		return cmp.Or(strings.Compare(a.Path, b.Path), bytes.Compare(a.Content, b.Content), cmpBool(a.CreateOnly, b.CreateOnly))
	})
	for i := 1; i < len(out.Edits); i++ {
		if out.Edits[i-1].Path == out.Edits[i].Path {
			out.Diagnostics = append(out.Diagnostics, diagnostic("gen.duplicate-path", out.Edits[i].Path, "more than one generated file targets this path", "give every generated file a distinct repository path"))
		}
	}
	sortDiagnostics(out.Diagnostics)
	return out
}

func cmpBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	}
	return -1
}

func diagnostic(code, path, cause, fix string) diag.Diagnostic {
	if path == "" {
		path = "design/system.uc"
	}
	return diag.Diagnostic{Code: code, Subject: path, Position: diag.Position{Path: path, Line: 1, Column: 1}, Message: cause, Fix: fix}
}

func sortDiagnostics(ds []diag.Diagnostic) {
	slices.SortFunc(ds, func(a, b diag.Diagnostic) int {
		return cmp.Or(strings.Compare(a.Position.Path, b.Position.Path), cmp.Compare(a.Position.Line, b.Position.Line), cmp.Compare(a.Position.Column, b.Position.Column),
			strings.Compare(a.Code, b.Code), strings.Compare(a.Subject, b.Subject), strings.Compare(a.Message, b.Message), strings.Compare(a.Fix, b.Fix))
	})
}

func ioDiagnostic(path string, err error) diag.Diagnostic {
	return diagnostic("gen.write-failed", path, fmt.Sprintf("cannot prepare or write generated file: %v", err), "make this repository path a readable regular file in a writable directory")
}
