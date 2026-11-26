package golang

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/graph"
)

// describeNode writes everything about a node that extraction decides:
// identity, kind, form, layer, source, origin, summary and attrs.
func describeNode(n graph.Node) string {
	return fmt.Sprintf("%s %s %s %s %s %s:%d:%d %q%s", n.Id, n.Kind, n.Form, n.Layer, n.Source,
		n.Origin.Path, n.Origin.Line, n.Origin.Column, n.Summary, describeAttrs(n.Attrs))
}

// describeEdge writes an edge's kind, start, end, source and attrs.
func describeEdge(e graph.Edge) string {
	return fmt.Sprintf("%s %s %s %s%s", e.Kind, e.From, e.To, e.Source, describeAttrs(e.Attrs))
}

func describeAttrs(attrs map[string][]string) string {
	var b strings.Builder
	for _, k := range slices.Sorted(maps.Keys(attrs)) {
		fmt.Fprintf(&b, " %s=%q", k, attrs[k])
	}
	return b.String()
}

// bindingCodes are the diagnostics a binding that is not well-formed gets.
var bindingCodes = []string{
	"lang.golang.unknown-binding-verb", "lang.golang.missing-binding-id", "lang.golang.invalid-binding-id",
	"lang.golang.misplaced-binding", "lang.golang.duplicate-binding",
}

//uc:checks lang.golang.extracts-generated-module
func checkExtractsGeneratedModule(t rapid.TB, module Module) {
	dir, err := os.MkdirTemp("", "unicode-golang-")
	if err != nil {
		t.Fatalf("cannot make a directory to write the module in: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	for path, content := range module.Files {
		file := filepath.Join(dir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatalf("cannot make the directory of %s: %v", path, err)
		}
		if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
			t.Fatalf("cannot write %s: %v", path, err)
		}
	}
	extracted, err := NewBackend().Extract(dir, []string{"."})
	if err != nil {
		t.Fatalf("Extract failed on the generated module, which is valid Go unless genModule wrote it wrong: %v\n%s", err, listing(module))
	}
	var nodes, edges []string
	for _, n := range extracted.Fragment.Nodes {
		nodes = append(nodes, describeNode(n))
	}
	wellFormed := 0
	for _, e := range extracted.Fragment.Edges {
		edges = append(edges, describeEdge(e))
		switch {
		case e.Kind == graph.EdgeKindGenerates:
			wellFormed += len(e.Attrs["symbols"])
		case e.Source == graph.SourceDeclared:
			wellFormed++
		}
	}
	if missing, extra := difference(module.Nodes, nodes), difference(nodes, module.Nodes); len(missing)+len(extra) > 0 {
		t.Fatalf("extraction gives other nodes than the module has\nmissing:\n%s\nunexpected:\n%s\n%s",
			strings.Join(missing, "\n"), strings.Join(extra, "\n"), listing(module))
	}
	if missing, extra := difference(module.Edges, edges), difference(edges, module.Edges); len(missing)+len(extra) > 0 {
		t.Fatalf("extraction gives other edges than the module has\nmissing:\n%s\nunexpected:\n%s\n%s",
			strings.Join(missing, "\n"), strings.Join(extra, "\n"), listing(module))
	}
	// Every //uc: line is a well-formed binding, now an edge, or gets exactly one diagnostic.
	lines := bindingLines(module)
	seen := map[string]bool{}
	for _, d := range extracted.Diagnostics {
		at := fmt.Sprintf("%s:%d:%d", d.Position.Path, d.Position.Line, d.Position.Column)
		switch {
		case !slices.Contains(bindingCodes, d.Code):
			t.Fatalf("extraction gave the diagnostic %s at %s, which is not one of a binding: %s", d.Code, at, d.Message)
		case !lines[at]:
			t.Fatalf("extraction gave the diagnostic %s at %s, where no //uc: line starts\n%s", d.Code, at, listing(module))
		case seen[at]:
			t.Fatalf("extraction gave the //uc: line at %s more than one diagnostic\n%s", at, listing(module))
		case d.Subject == "" || d.Message == "" || d.Fix == "":
			t.Fatalf("the diagnostic %s at %s lacks its subject, cause or fix: %+v", d.Code, at, d)
		}
		seen[at] = true
	}
	if len(lines) != wellFormed+len(extracted.Diagnostics) {
		t.Fatalf("the module has %d //uc: lines; extraction read %d of them as bindings and gave %d diagnostics, "+
			"so a line that is not a well-formed binding got no diagnostic\n%s", len(lines), wellFormed, len(extracted.Diagnostics), listing(module))
	}
}

// difference lists the entries of a that b lacks.
func difference(a, b []string) []string {
	var out []string
	for _, s := range a {
		if !slices.Contains(b, s) {
			out = append(out, "  "+s)
		}
	}
	return out
}

// bindingLines are the places, as path:line:column, where a //uc: comment starts.
func bindingLines(module Module) map[string]bool {
	at := map[string]bool{}
	for path, content := range module.Files {
		for i, line := range strings.Split(content, "\n") {
			if column := strings.Index(line, "//uc:"); column >= 0 && strings.HasSuffix(path, ".go") {
				at[path+":"+strconv.Itoa(i+1)+":"+strconv.Itoa(column+1)] = true
			}
		}
	}
	return at
}

// listing writes the module's Go files with their line numbers.
func listing(module Module) string {
	var b strings.Builder
	for _, path := range slices.Sorted(maps.Keys(module.Files)) {
		if !strings.HasSuffix(path, ".go") || strings.HasPrefix(path, "rapid/") {
			continue
		}
		fmt.Fprintf(&b, "--- %s\n", path)
		for i, line := range strings.Split(strings.TrimSuffix(module.Files[path], "\n"), "\n") {
			fmt.Fprintf(&b, "%4d  %s\n", i+1, line)
		}
	}
	return b.String()
}

func TestExtractsGeneratedModule(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		checkExtractsGeneratedModule(t, genModule().Draw(t, "module"))
	})
}

func FuzzExtractsGeneratedModule(f *testing.F) {
	f.Fuzz(rapid.MakeFuzz(func(t *rapid.T) {
		checkExtractsGeneratedModule(t, genModule().Draw(t, "module"))
	}))
}
