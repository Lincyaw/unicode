package golang

import (
	"fmt"
	"maps"
	"regexp"
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

// digestKeys are the attrs whose values are digests (docs/design.md §5.2).
var digestKeys = []string{"body", "declaration", "globals"}

// digestClasses writes each digest of the nodes, sorted by identity, as the
// class of the nodes that share it: "#0" for the first, "#1" for the next
// other one, and so on, for each key apart. Two lists of nodes then describe
// alike exactly when their digests split them alike, whatever the digests are.
func digestClasses(nodes []graph.Node) []graph.Node {
	out := slices.Clone(nodes)
	for _, key := range digestKeys {
		classes := map[string]string{}
		for i, n := range out {
			values, ok := n.Attrs[key]
			if !ok {
				continue
			}
			n.Attrs = maps.Clone(n.Attrs)
			n.Attrs[key] = nil
			for _, v := range values {
				if _, seen := classes[v]; !seen {
					classes[v] = fmt.Sprintf("#%d", len(classes))
				}
				n.Attrs[key] = append(n.Attrs[key], classes[v])
			}
			out[i] = n
		}
	}
	return out
}

var sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)

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
	dir, remove := writeModule(t, module)
	defer remove()
	extracted, err := NewBackend().Extract(dir, []string{"."})
	if err != nil {
		t.Fatalf("Extract failed on the generated module, which is valid Go unless genModule wrote it wrong: %v\n%s", err, listing(module))
	}
	var nodes, edges, diagnostics []string
	for _, n := range extracted.Fragment.Nodes {
		for _, key := range digestKeys {
			if v, ok := n.Attrs[key]; ok && (len(v) != 1 || !sha256Hex.MatchString(v[0])) {
				t.Fatalf("the %s of %s is %q, not one SHA-256 in lower-case hex", key, n.Id, v)
			}
		}
	}
	for _, n := range digestClasses(extracted.Fragment.Nodes) {
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
