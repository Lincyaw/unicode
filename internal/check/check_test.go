package check

import (
	"fmt"
	"slices"
	"strings"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
)

// derived adds a fragment to an empty graph and derives it, failing when
// either step refuses the fragment or reports a diagnostic.
func derived(t rapid.TB, f graph.Fragment) *graph.Graph {
	g := graph.NewGraph()
	if err := g.AddFragment(f); err != nil {
		t.Fatalf("AddFragment refused a fragment whose nodes are distinct and whose edges join its nodes: %v", err)
	}
	if diags := g.Derive(); len(diags) > 0 {
		t.Fatalf("Derive reported diagnostics on a graph whose bindings resolve and whose patterns parse:\n%s", list(diags))
	}
	return &g
}

func list(diags []diag.Diagnostic) string {
	var b strings.Builder
	for _, d := range diags {
		fmt.Fprintf(&b, "  %s %s at %s:%d: %s (fix: %s)\n", d.Code, d.Subject, d.Position.Path, d.Position.Line, d.Message, d.Fix)
	}
	return b.String()
}

// declaredItems lists the keys of every declared node and edge and every depends.
func declaredItems(g *graph.Graph) []string {
	var out []string
	for _, n := range g.Nodes() {
		if n.Source == graph.SourceDeclared {
			out = append(out, n.Id)
		}
	}
	for _, e := range g.Edges() {
		if e.Source == graph.SourceDeclared || e.Kind == graph.EdgeKindDepends {
			out = append(out, edgeID(e))
		}
	}
	return out
}

//uc:checks check.rejects-broken-graph
func checkRejectsBrokenGraph(t rapid.TB, broken Broken) {
	g := graph.NewGraph()
	if err := g.AddFragment(broken.Fragment); err != nil {
		t.Fatalf("AddFragment refused a fragment whose nodes are distinct and whose edges join its nodes: %v", err)
	}
	g.Derive()
	checked := Conform(&g)
	found := false
	for _, d := range checked.Diagnostics {
		if !strings.HasPrefix(d.Code, "check.") || d.Subject == "" || d.Message == "" || d.Fix == "" || d.Position.Path == "" {
			t.Fatalf("the diagnostic %+v lacks a check code, a subject, a message, a fix or a position", d)
		}
		found = found || d.Code == broken.Code && d.Subject == broken.Subject
	}
	if !found {
		t.Fatalf("the graph breaks the constraint of %s at %s, but Conform gave no %s diagnostic about %s; it gave:\n%s",
			broken.Code, broken.Subject, broken.Code, broken.Subject, list(checked.Diagnostics))
	}
	if status, ok := checked.Statuses[broken.Subject]; ok && status == graph.StatusVerified {
		t.Fatalf("Conform reported %s about the declared item %s, yet left its status verified", broken.Code, broken.Subject)
	}
}

// assertConformingGraph validates the baseline of the independent stub predicate.
func assertConformingGraph(t rapid.TB, sample Sample) {
	g := derived(t, sample.Fragment)
	checked := Conform(g)
	if len(checked.Diagnostics) > 0 {
		t.Fatalf("Conform reported diagnostics on a graph that meets every constraint of design.md §5.3:\n%s", list(checked.Diagnostics))
	}
	items := declaredItems(g)
	if len(items) == 0 {
		t.Fatalf("the sample has no declared item, so it checks nothing")
	}
	for _, id := range items {
		if status, ok := checked.Statuses[id]; !ok || status != graph.StatusVerified {
			t.Fatalf("Conform gave the declared item %q of a conforming graph the status %q (present: %v), not verified", id, status, ok)
		}
	}
	if len(checked.Statuses) != len(items) {
		t.Fatalf("Conform gave %d statuses, but the graph has %d declared items and depends", len(checked.Statuses), len(items))
	}
}

//uc:checks check.stubs-stay-incomplete
func checkStubsStayIncomplete(t rapid.TB, sample Sample) {
	assertConformingGraph(t, sample)
	base := derived(t, sample.Fragment)
	// Only actual bindings determine the expected affected checks. Code
	// declaration names and the implementation's indexing are irrelevant.
	targets := map[string]map[string]bool{}
	for _, edge := range base.Edges() {
		if edge.Kind != graph.EdgeKindChecks && edge.Kind != graph.EdgeKindGenerates {
			continue
		}
		code := base.Node(edge.From)
		if code == nil || code.Layer != graph.LayerCode || code.Kind != graph.NodeKindOp {
			continue
		}
		if targets[edge.From] == nil {
			targets[edge.From] = map[string]bool{}
		}
		targets[edge.From][edge.To] = true
	}
	if len(targets) == 0 {
		t.Fatal("the conforming sample has no bound predicate or generator")
		return
	}
	for source, affected := range targets {
		for _, unfinished := range []bool{true, false} {
			fragment := graph.Fragment{Edges: slices.Clone(sample.Fragment.Edges)}
			for _, node := range sample.Fragment.Nodes {
				if node.Id == source {
					attrs := map[string][]string{}
					for key, value := range node.Attrs {
						attrs[key] = slices.Clone(value)
					}
					if unfinished {
						attrs["stub"] = []string{"true"}
					} else {
						attrs["stub"] = []string{"false"}
					}
					node.Attrs = attrs
				}
				fragment.Nodes = append(fragment.Nodes, node)
			}
			checked := Conform(derived(t, fragment))
			if len(checked.Diagnostics) > 0 {
				t.Fatalf("changing only the stub flag of %s produced diagnostics: %s", source, list(checked.Diagnostics))
			}
			items := declaredItems(base)
			if len(checked.Statuses) != len(items) {
				t.Fatalf("stub check returned %d states for %d declarations", len(checked.Statuses), len(items))
			}
			for _, id := range items {
				want := graph.StatusVerified
				if unfinished && affected[id] {
					want = graph.StatusStub
				}
				if got, ok := checked.Statuses[id]; !ok || got != want {
					t.Fatalf("stub=%v at %s gives %s status %s (present %v), want %s", unfinished, source, id, got, ok, want)
				}
			}
		}
	}
}
