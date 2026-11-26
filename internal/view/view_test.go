package view

import (
	"cmp"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/graph"
)

// graphOf adds the nodes, then the edges, of a fragment whose identities are distinct.
func graphOf(t rapid.TB, f graph.Fragment) graph.Graph {
	g := graph.NewGraph()
	for _, n := range f.Nodes {
		if err := g.AddNode(n); err != nil {
			t.Fatalf("AddNode refused the node %q, whose identity is new: %v", n.Id, err)
		}
	}
	for _, e := range f.Edges {
		if err := g.AddEdge(e); err != nil {
			t.Fatalf("AddEdge refused the new edge %s %q %q, whose ends are nodes of the graph: %v", e.Kind, e.From, e.To, err)
		}
	}
	return g
}

// difference returns the members of a that b lacks, sorted.
func difference[K comparable](a, b map[K]bool, less func(K, K) int) []K {
	var d []K
	for k := range a {
		if !b[k] {
			d = append(d, k)
		}
	}
	slices.SortFunc(d, less)
	return d
}

func compareKeys(a, b edgeKey) int {
	return cmp.Or(cmp.Compare(a.kind, b.kind), cmp.Compare(a.from, b.from), cmp.Compare(a.to, b.to))
}

//uc:checks view.view-is-folded-subgraph
func checkViewIsFoldedSubgraph(t rapid.TB, request Request) {
	g := graphOf(t, request.Fragment)
	w := newWorld(request.Fragment)
	got, err := Select(&g, request.Kind, request.Id, request.Expand)
	input, applies := w.applicable(request.Kind, request.Id)
	if !applies {
		if !errors.As(err, new(NotApplicable)) {
			t.Fatalf("the %s view of %q, a node it does not apply to, gave %v rather than NotApplicable", request.Kind, request.Id, err)
		}
		return
	}
	if err != nil {
		t.Fatalf("Select refused the %s view of %q, a node it applies to: %v", request.Kind, request.Id, err)
	}
	if got.Kind != request.Kind {
		t.Fatalf("the %s view of %q says it is a %s view", request.Kind, request.Id, got.Kind)
	}

	drawn := map[string]bool{}
	for _, n := range got.Nodes {
		if drawn[n.Id] {
			t.Fatalf("the %s view of %q draws the node %q twice", request.Kind, request.Id, n.Id)
		}
		drawn[n.Id] = true
		if !reflect.DeepEqual(n, w.nodes[n.Id]) {
			t.Fatalf("the %s view of %q draws the node %+v, which the graph does not hold", request.Kind, request.Id, n)
		}
	}
	graphEdges := map[edgeKey]graph.Edge{}
	for _, e := range request.Fragment.Edges {
		graphEdges[edgeKey{e.Kind, e.From, e.To}] = e
	}
	edges := map[edgeKey]bool{}
	for _, e := range got.Edges {
		k := edgeKey{e.Kind, e.From, e.To}
		switch {
		case edges[k]:
			t.Fatalf("the %s view of %q draws the edge %s %q %q twice", request.Kind, request.Id, e.Kind, e.From, e.To)
		case !reflect.DeepEqual(e, graphEdges[k]):
			t.Fatalf("the %s view of %q draws the edge %+v, which the graph does not hold", request.Kind, request.Id, e)
		case !drawn[e.From] || !drawn[e.To]:
			t.Fatalf("the %s view of %q draws the edge %s %q %q, an end of which it does not draw", request.Kind, request.Id, e.Kind, e.From, e.To)
		}
		edges[k] = true
	}

	if len(request.Expand) == 0 {
		for _, n := range got.Nodes {
			if !n.Salient {
				t.Fatalf("the %s view of %q, with nothing expanded, draws the detail node %q", request.Kind, request.Id, n.Id)
			}
		}
	}
	exempt := w.exempt(input, request.Expand)
	folded := set(got.Folded)
	for _, id := range got.Folded {
		if !drawn[id] || exempt[id] || !w.designContainer(id) || w.focus(id) != "support" {
			t.Fatalf("the %s view of %q lists %q as folded, which is not a drawn, unexpanded support container", request.Kind, request.Id, id)
		}
	}
	for _, n := range got.Nodes {
		for _, c := range w.up(n.Id) {
			switch {
			case folded[c]:
				t.Fatalf("the %s view of %q draws %q inside the support container %q, which it folds into one node", request.Kind, request.Id, n.Id, c)
			case !exempt[c] && w.focus(c) == "utility":
				t.Fatalf("the %s view of %q draws %q inside the unexpanded utility container %q, which it should leave out", request.Kind, request.Id, n.Id, c)
			}
		}
		if !exempt[n.Id] && w.designContainer(n.Id) {
			switch w.focus(n.Id) {
			case "utility":
				t.Fatalf("the %s view of %q draws the unexpanded utility container %q, which it should leave out", request.Kind, request.Id, n.Id)
			case "support":
				if !folded[n.Id] {
					t.Fatalf("the %s view of %q draws the unexpanded support container %q without folding it", request.Kind, request.Id, n.Id)
				}
			}
		}
	}

	want := w.expected(request.Kind, input, request.Expand)
	if missing := difference(want.nodes, drawn, strings.Compare); len(missing) > 0 {
		t.Fatalf("the %s view of %q, expanding %q, leaves out %q, which §7.1–§7.2 draw", request.Kind, request.Id, request.Expand, missing)
	}
	if extra := difference(drawn, want.nodes, strings.Compare); len(extra) > 0 {
		t.Fatalf("the %s view of %q, expanding %q, draws %q, which §7.1–§7.2 do not", request.Kind, request.Id, request.Expand, extra)
	}
	if missing := difference(want.edges, edges, compareKeys); len(missing) > 0 {
		t.Fatalf("the %s view of %q, expanding %q, leaves out the edges %v between drawn nodes", request.Kind, request.Id, request.Expand, missing)
	}
	if extra := difference(edges, want.edges, compareKeys); len(extra) > 0 {
		t.Fatalf("the %s view of %q, expanding %q, draws the edges %v, which the view does not pick", request.Kind, request.Id, request.Expand, extra)
	}
	if !reflect.DeepEqual(folded, want.folded) {
		t.Fatalf("the %s view of %q, expanding %q, folds %q, not %q", request.Kind, request.Id, request.Expand, got.Folded, keys(want.folded))
	}
	if got.Hidden != len(want.hiders) {
		t.Fatalf("the %s view of %q, expanding %q, counts %d utility containers left out, not %d: %q", request.Kind, request.Id, request.Expand, got.Hidden, len(want.hiders), keys(want.hiders))
	}
}

func set(ids []string) map[string]bool {
	s := map[string]bool{}
	for _, id := range ids {
		s[id] = true
	}
	return s
}

func TestViewIsFoldedSubgraph(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		checkViewIsFoldedSubgraph(t, genRequest().Draw(t, "request"))
	})
}

func FuzzViewIsFoldedSubgraph(f *testing.F) {
	f.Fuzz(rapid.MakeFuzz(func(t *rapid.T) {
		checkViewIsFoldedSubgraph(t, genRequest().Draw(t, "request"))
	}))
}

//uc:checks view.sequence-follows-steps
func checkSequenceFollowsSteps(t rapid.TB, request Request) {
	g := graphOf(t, request.Fragment)
	w := newWorld(request.Fragment)
	got, err := Select(&g, request.Kind, request.Id, request.Expand)
	if err != nil {
		t.Fatalf("Select refused the %s view of the flow %q: %v", request.Kind, request.Id, err)
	}
	steps := w.stepsOf(request.Id)
	if len(steps) == 0 {
		t.Fatalf("the generated flow %q has no steps", request.Id)
	}
	position := map[string]int{}
	var drawnSteps []string
	for i, n := range got.Nodes {
		position[n.Id] = i
		if n.Kind == graph.NodeKindStep {
			drawnSteps = append(drawnSteps, n.Id)
		}
	}
	if !slices.Equal(drawnSteps, steps) {
		t.Fatalf("the %s view of %q lists the steps %q, not the flow's steps in the order of their index, %q", request.Kind, request.Id, drawnSteps, steps)
	}
	exempt := w.exempt(request.Id, request.Expand)
	var lifelines []string
	for _, p := range w.participants(request.Id) {
		switch c, f := w.folder(p, exempt); f {
		case "utility":
			continue
		case "support":
			p = c
		}
		if !slices.Contains(lifelines, p) {
			lifelines = append(lifelines, p)
		}
	}
	for i, l := range lifelines {
		at, ok := position[l]
		switch {
		case !ok:
			t.Fatalf("the %s view of %q leaves out the lifeline %q", request.Kind, request.Id, l)
		case i > 0 && at < position[lifelines[i-1]]:
			t.Fatalf("the %s view of %q draws the lifeline %q before %q, though %q appears first in the flow", request.Kind, request.Id, l, lifelines[i-1], lifelines[i-1])
		}
	}
}

func TestSequenceFollowsSteps(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		checkSequenceFollowsSteps(t, genFlowRequest().Draw(t, "request"))
	})
}

func FuzzSequenceFollowsSteps(f *testing.F) {
	f.Fuzz(rapid.MakeFuzz(func(t *rapid.T) {
		checkSequenceFollowsSteps(t, genFlowRequest().Draw(t, "request"))
	}))
}
