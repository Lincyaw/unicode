package check

import (
	"fmt"
	"strings"
	"testing"

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

//uc:checks check.accepts-conforming-graph
func checkAcceptsConformingGraph(t rapid.TB, sample Sample) {
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

func TestAcceptsConformingGraph(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) { checkAcceptsConformingGraph(t, genSample().Draw(t, "sample")) })
}

func FuzzAcceptsConformingGraph(f *testing.F) {
	f.Fuzz(rapid.MakeFuzz(func(t *rapid.T) { checkAcceptsConformingGraph(t, genSample().Draw(t, "sample")) }))
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

func TestRejectsBrokenGraph(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) { checkRejectsBrokenGraph(t, genBroken().Draw(t, "broken")) })
}

func FuzzRejectsBrokenGraph(f *testing.F) {
	f.Fuzz(rapid.MakeFuzz(func(t *rapid.T) { checkRejectsBrokenGraph(t, genBroken().Draw(t, "broken")) }))
}
