package design

import (
	"bytes"
	"fmt"
	"maps"
	"reflect"
	"regexp"
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
	if len(sample.Attrs) == 0 {
		t.Fatalf("setup failed: the sample lists the attributes of no node, not even the system's")
	}
	for _, id := range slices.Sorted(maps.Keys(sample.Attrs)) {
		want, got := sample.Attrs[id], loaded.Graph.Node(id).Attrs
		if !maps.EqualFunc(got, want, slices.Equal) {
			t.Fatalf("the node %s has other attributes than the design gives it\ngot: %v\nwant: %v\n%s", id, got, want, dump(sample.Files))
		}
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

var designCode = regexp.MustCompile(`^design\.[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// wellFormed checks that every diagnostic has a code of the form
// design.<kebab-name>, a cause, a fix, and a position in one of the files.
func wellFormed(t rapid.TB, ds []diag.Diagnostic, files map[string]string) {
	for _, d := range ds {
		_, inFiles := files[d.Position.Path]
		switch {
		case !designCode.MatchString(d.Code):
			t.Fatalf("the code %q is not of the form design.<kebab-name>", d.Code)
		case d.Message == "" || d.Fix == "" || d.Subject == "":
			t.Fatalf("the diagnostic %s lacks its subject, cause or fix: %+v", d.Code, d)
		case d.Position.Line < 1 || d.Position.Column < 1:
			t.Fatalf("the diagnostic %s stands at %d:%d, which is no position", d.Code, d.Position.Line, d.Position.Column)
		case !inFiles && d.Code != "design.missing-system-file":
			t.Fatalf("the diagnostic %s names the path %s, which is not one of the files", d.Code, d.Position.Path)
		case inFiles && d.Position.Line > strings.Count(files[d.Position.Path], "\n")+1:
			t.Fatalf("the diagnostic %s stands on line %d, past the end of %s", d.Code, d.Position.Line, d.Position.Path)
		}
	}
}

//uc:checks design.rejects-broken-design
func checkRejectsBrokenDesign(t rapid.TB, broken Broken) {
	loaded := Load(parseAll(t, broken.Files, slices.Sorted(maps.Keys(broken.Files))))
	wellFormed(t, loaded.Diagnostics, broken.Files)
	found := slices.ContainsFunc(loaded.Diagnostics, func(d diag.Diagnostic) bool {
		return d.Code == broken.Code && d.Position.Path == broken.Path && d.Position.Line == broken.Line
	})
	if !found {
		t.Fatalf("a change that breaks %s at %s:%d got no such diagnostic there; Load gave:%s\n%s",
			broken.Code, broken.Path, broken.Line, listDiagnostics(loaded.Diagnostics), dump(broken.Files))
	}
}

func TestRejectsBrokenDesign(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		checkRejectsBrokenDesign(t, genBroken().Draw(t, "broken"))
	})
}

func FuzzRejectsBrokenDesign(f *testing.F) {
	f.Fuzz(rapid.MakeFuzz(func(t *rapid.T) {
		checkRejectsBrokenDesign(t, genBroken().Draw(t, "broken"))
	}))
}

// genFiles draws the files of a valid design, or of one broken in one place.
//
//uc:generates design.load-ignores-file-order.files
func genFiles() *rapid.Generator[map[string]string] {
	return rapid.Custom(func(t *rapid.T) map[string]string {
		if rapid.Bool().Draw(t, "valid") {
			return genSample().Draw(t, "sample").Files
		}
		return genBroken().Draw(t, "broken").Files
	})
}

func rotate[T any](s []T, by int) []T {
	if len(s) == 0 {
		return s
	}
	by %= len(s)
	return append(slices.Clone(s[by:]), s[:by]...)
}

//uc:checks design.load-ignores-file-order
func checkLoadIgnoresFileOrder(t rapid.TB, files map[string]string, rotation int) {
	paths := slices.Sorted(maps.Keys(files))
	sorted := Load(parseAll(t, files, paths))
	rotated := Load(parseAll(t, files, rotate(paths, rotation)))
	if a, b := sorted.Graph.Encode(), rotated.Graph.Encode(); !bytes.Equal(a, b) {
		t.Fatalf("the files in another order load into another snapshot (rotated by %d)\nsorted:\n%s\nrotated:\n%s", rotation, a, b)
	}
	if !reflect.DeepEqual(sorted.Diagnostics, rotated.Diagnostics) {
		t.Fatalf("the files in another order get other diagnostics (rotated by %d)\nsorted:%s\nrotated:%s",
			rotation, listDiagnostics(sorted.Diagnostics), listDiagnostics(rotated.Diagnostics))
	}
}

func TestLoadIgnoresFileOrder(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		checkLoadIgnoresFileOrder(t, genFiles().Draw(t, "files"), rapid.IntRange(0, 1000).Draw(t, "rotation"))
	})
}

func FuzzLoadIgnoresFileOrder(f *testing.F) {
	f.Fuzz(rapid.MakeFuzz(func(t *rapid.T) {
		checkLoadIgnoresFileOrder(t, genFiles().Draw(t, "files"), rapid.IntRange(0, 1000).Draw(t, "rotation"))
	}))
}
