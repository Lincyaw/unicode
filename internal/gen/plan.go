package gen

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
)

// Plan asks each configured language's generator for its files. It does not
// require missing implementations or predicates to pass conformance first.
//
//uc:realizes gen.plan
func Plan(root string, g *graph.Graph, backends []lang.Backend) lang.Generation {
	out := lang.Generation{}
	available := map[string]lang.Backend{}
	for _, b := range backends {
		name := b.Language()
		if _, exists := available[name]; exists {
			out.Diagnostics = append(out.Diagnostics, diagnostic("gen.duplicate-backend", "design/system.uc", "more than one backend provides "+name, "register exactly one backend for each language"))
		}
		available[name] = b
	}
	roots := lang.LanguageRoots(g)
	for _, name := range slices.Sorted(maps.Keys(roots)) {
		b, ok := available[name].(lang.Generator)
		if !ok {
			out.Diagnostics = append(out.Diagnostics, diagnostic("gen.unsupported-language", "design/system.uc", "no generation backend provides "+name, "register a generator for this language, or remove its language roots"))
			continue
		}
		generated := b.Generate(lang.GenPlan{Root: root, Roots: roots[name], Graph: g})
		out.Edits = append(out.Edits, generated.Edits...)
		out.Diagnostics = append(out.Diagnostics, generated.Diagnostics...)
	}
	slices.SortFunc(out.Edits, func(a, b lang.Edit) int { return strings.Compare(a.Path, b.Path) })
	for i := 1; i < len(out.Edits); i++ {
		if out.Edits[i-1].Path == out.Edits[i].Path {
			out.Diagnostics = append(out.Diagnostics, diagnostic("gen.duplicate-path", out.Edits[i].Path, "more than one generated file targets this path", "give every generated file a distinct repository path"))
		}
	}
	sortDiagnostics(out.Diagnostics)
	return out
}

func diagnostic(code, path, cause, fix string) diag.Diagnostic {
	if path == "" {
		path = "design/system.uc"
	}
	return diag.Diagnostic{Code: code, Subject: path, Position: diag.Position{Path: path, Line: 1, Column: 1}, Message: cause, Fix: fix}
}

func sortDiagnostics(ds []diag.Diagnostic) {
	slices.SortFunc(ds, func(a, b diag.Diagnostic) int {
		return cmp.Or(strings.Compare(a.Position.Path, b.Position.Path), cmp.Compare(a.Position.Line, b.Position.Line), cmp.Compare(a.Position.Column, b.Position.Column), strings.Compare(a.Code, b.Code), strings.Compare(a.Subject, b.Subject))
	})
}

func ioDiagnostic(path string, err error) diag.Diagnostic {
	return diagnostic("gen.write-failed", path, fmt.Sprintf("cannot prepare or write generated file: %v", err), "make this repository path a readable regular file in a writable directory")
}
