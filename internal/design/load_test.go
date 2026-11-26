package design

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/syntax"
)

// parseAll parses the files in the order of paths; every generated file parses.
func parseAll(t rapid.TB, files map[string]string, paths []string) []syntax.File {
	var out []syntax.File
	for _, p := range paths {
		kind, err := syntax.KindOf(p)
		if err != nil {
			t.Fatalf("setup failed: the generated path %s is not a design or corpus file: %v", p, err)
		}
		parsed := syntax.Parse(p, kind, []byte(files[p]))
		if len(parsed.Diagnostics) > 0 {
			t.Fatalf("setup failed: the generated file %s does not parse:%s\n%s", p, listDiagnostics(parsed.Diagnostics), files[p])
		}
		out = append(out, *parsed.File)
	}
	return out
}

func listDiagnostics(ds []diag.Diagnostic) string {
	var b strings.Builder
	for _, d := range ds {
		fmt.Fprintf(&b, "\n  %s:%d:%d: %s: %s", d.Position.Path, d.Position.Line, d.Position.Column, d.Code, d.Message)
	}
	return b.String()
}

func dump(files map[string]string) string {
	var b strings.Builder
	for _, p := range slices.Sorted(maps.Keys(files)) {
		fmt.Fprintf(&b, "--- %s\n%s", p, files[p])
	}
	return b.String()
}

// difference lists what one sorted list has and the other lacks.
func difference(got, want []string) (missing, extra []string) {
	for _, w := range want {
		if !slices.Contains(got, w) {
			missing = append(missing, w)
		}
	}
	for _, g := range got {
		if !slices.Contains(want, g) {
			extra = append(extra, g)
		}
	}
	return missing, extra
}

//uc:checks design.accepts-valid-design
func checkAcceptsValidDesign(t rapid.TB, sample Sample) {
	loaded := Load(parseAll(t, sample.Files, slices.Sorted(maps.Keys(sample.Files))))
	if len(loaded.Diagnostics) > 0 {
		t.Fatalf("Load gave diagnostics for a valid design:%s\n%s", listDiagnostics(loaded.Diagnostics), dump(sample.Files))
	}
	var nodes, edges []string
	for _, n := range loaded.Graph.Nodes() {
		nodes = append(nodes, n.Id)
	}
	for _, e := range loaded.Graph.Edges() {
		edges = append(edges, string(e.Kind)+" "+e.From+" "+e.To)
	}
	if missing, extra := difference(nodes, sample.Nodes); len(missing)+len(extra) > 0 {
		t.Fatalf("the graph's nodes are not those the design declares\nmissing: %v\nextra: %v\n%s", missing, extra, dump(sample.Files))
	}
	if missing, extra := difference(edges, sample.Edges); len(missing)+len(extra) > 0 {
		t.Fatalf("the graph's edges are not those the design declares\nmissing: %v\nextra: %v\n%s", missing, extra, dump(sample.Files))
	}
	if len(nodes) != len(sample.Nodes) || len(edges) != len(sample.Edges) {
		t.Fatalf("the design declares %d nodes and %d edges, the graph holds %d and %d", len(sample.Nodes), len(sample.Edges), len(nodes), len(edges))
	}
}

func TestAcceptsValidDesign(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		checkAcceptsValidDesign(t, genSample().Draw(t, "sample"))
	})
}

func FuzzAcceptsValidDesign(f *testing.F) {
	f.Fuzz(rapid.MakeFuzz(func(t *rapid.T) {
		checkAcceptsValidDesign(t, genSample().Draw(t, "sample"))
	}))
}
