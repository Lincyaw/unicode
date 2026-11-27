package golang

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/diag"
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

// describeDiagnostic writes a diagnostic's code and place, without its column.
func describeDiagnostic(d diag.Diagnostic) string {
	return fmt.Sprintf("%s %s:%d", d.Code, d.Position.Path, d.Position.Line)
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
	var nodes, edges, diagnostics []string
	for _, n := range extracted.Fragment.Nodes {
		nodes = append(nodes, describeNode(n))
	}
	for _, e := range extracted.Fragment.Edges {
		edges = append(edges, describeEdge(e))
	}
	for _, d := range extracted.Diagnostics {
		diagnostics = append(diagnostics, describeDiagnostic(d))
		if d.Subject == "" || d.Message == "" || d.Fix == "" {
			t.Fatalf("the diagnostic %s lacks its subject, cause or fix: %+v", describeDiagnostic(d), d)
		}
	}
	for _, c := range []struct {
		what      string
		want, got []string
	}{{"nodes", module.Nodes, nodes}, {"edges", module.Edges, edges}, {"diagnostics", module.Diagnostics, diagnostics}} {
		if missing, extra := difference(c.want, c.got), difference(c.got, c.want); len(missing)+len(extra) > 0 {
			t.Fatalf("extraction gives other %s than the module has\nmissing:\n%s\nunexpected:\n%s\n%s",
				c.what, strings.Join(missing, "\n"), strings.Join(extra, "\n"), listing(module))
		}
	}
}

// difference lists the entries of a that b lacks, counting repeats.
func difference(a, b []string) []string {
	left := slices.Clone(b)
	var out []string
	for _, s := range a {
		if i := slices.Index(left, s); i >= 0 {
			left = slices.Delete(left, i, i+1)
		} else {
			out = append(out, "  "+s)
		}
	}
	return out
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
