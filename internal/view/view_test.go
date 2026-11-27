package view

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

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
	got, err := Select(&g, request.Kind, request.Id, request.Expand, request.Layer, request.Level)
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
		if !reflect.DeepEqual(n.Node, w.nodes[n.Id]) {
			t.Fatalf("the %s view of %q draws the node %+v, which the graph does not hold", request.Kind, request.Id, n)
		}
	}
	graphEdges := map[edgeKey]graph.Edge{}
	for _, e := range request.Fragment.Edges {
		graphEdges[edgeKey{e.Kind, e.From, e.To}] = e
	}
	edges := map[edgeKey]ViewEdge{}
	for _, e := range got.Edges {
		k := edgeKey{e.Kind, e.From, e.To}
		_, twice := edges[k]
		switch {
		case twice:
			t.Fatalf("the %s view of %q draws the edge %s %q %q twice", request.Kind, request.Id, e.Kind, e.From, e.To)
		case e.Count == 0 && !reflect.DeepEqual(e.Edge, graphEdges[k]):
			t.Fatalf("the %s view of %q draws the edge %+v without a count, yet the graph does not hold it", request.Kind, request.Id, e)
		case !drawn[e.From] || !drawn[e.To]:
			t.Fatalf("the %s view of %q draws the edge %s %q %q, an end of which it does not draw", request.Kind, request.Id, e.Kind, e.From, e.To)
		}
		edges[k] = e
	}

	if len(request.Expand) == 0 {
		for _, n := range got.Nodes {
			if !n.Salient {
				t.Fatalf("the %s view of %q, with nothing expanded, draws the detail node %q", request.Kind, request.Id, n.Id)
			}
		}
	}
	exempt, home := w.exempt(request.Kind, input, request.Expand), w.home(asked{request.Kind, input, request.Layer, request.Level})
	if !slices.Contains(foldingKinds, request.Kind) && (len(got.Folded) > 0 || got.Hidden > 0) {
		t.Fatalf("the %s view of %q folds %q and leaves out %d utility containers, yet it does not fold by focus", request.Kind, request.Id, got.Folded, got.Hidden)
	}
	folded := set(got.Folded)
	for _, id := range got.Folded {
		if !drawn[id] || exempt[id] || !w.designContainer(id) || w.foldFocus(id, home) != "support" {
			t.Fatalf("the %s view of %q lists %q as folded, which is not a drawn, unexpanded support container, nor in the class view another container than the input's", request.Kind, request.Id, id)
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
			switch w.foldFocus(n.Id, home) {
			case "utility":
				t.Fatalf("the %s view of %q draws the unexpanded utility container %q, which it should leave out", request.Kind, request.Id, n.Id)
			case "support":
				if !folded[n.Id] {
					t.Fatalf("the %s view of %q draws the unexpanded container %q without folding it, though it is a support container or, in the class view, outside the input's container", request.Kind, request.Id, n.Id)
				}
			}
		}
	}

	want := w.expected(asked{request.Kind, input, request.Layer, request.Level}, request.Expand)
	if missing := difference(want.nodes, drawn, strings.Compare); len(missing) > 0 {
		t.Fatalf("the %s view of %q, expanding %q, leaves out %q, which §7.1–§7.2 draw", request.Kind, request.Id, request.Expand, missing)
	}
	if extra := difference(drawn, want.nodes, strings.Compare); len(extra) > 0 {
		t.Fatalf("the %s view of %q, expanding %q, draws %q, which §7.1–§7.2 do not", request.Kind, request.Id, request.Expand, extra)
	}
	if missing := difference(present(want.edges), present(edges), compareKeys); len(missing) > 0 {
		t.Fatalf("the %s view of %q, expanding %q, leaves out the edges %v between the nodes that stand for their ends", request.Kind, request.Id, request.Expand, missing)
	}
	if extra := difference(present(edges), present(want.edges), compareKeys); len(extra) > 0 {
		t.Fatalf("the %s view of %q, expanding %q, draws the edges %v, which the view does not pick", request.Kind, request.Id, request.Expand, extra)
	}
	for k, stands := range want.edges {
		switch e := edges[k]; {
		case len(stands) == 0 && e.Count != 0:
			t.Fatalf("the %s view of %q counts %d edges in the edge %s %q %q, which is an edge of the graph between drawn nodes", request.Kind, request.Id, e.Count, k.kind, k.from, k.to)
		case len(stands) > 0 && e.Count != len(stands):
			t.Fatalf("the %s view of %q merges %d edges into the edge %s %q %q to a folded container, not the %d it stands for: %v", request.Kind, request.Id, e.Count, k.kind, k.from, k.to, len(stands), stands)
		case len(stands) > 0 && !reflect.DeepEqual(e.Edge, mergedOf(k, stands)):
			t.Fatalf("the %s view of %q draws the merged edge %+v, not one derived from the edges it stands for, at the first one's origin, with their attributes: %+v", request.Kind, request.Id, e.Edge, mergedOf(k, stands))
		}
	}
	if !reflect.DeepEqual(folded, want.folded) {
		t.Fatalf("the %s view of %q, expanding %q, folds %q, not %q", request.Kind, request.Id, request.Expand, got.Folded, keys(want.folded))
	}
	if got.Hidden != len(want.hiders) {
		t.Fatalf("the %s view of %q, expanding %q, counts %d utility containers left out, not %d: %q", request.Kind, request.Id, request.Expand, got.Hidden, len(want.hiders), keys(want.hiders))
	}
}

// present returns the keys of a map as a set.
func present[V any](m map[edgeKey]V) map[edgeKey]bool {
	s := map[edgeKey]bool{}
	for k := range m {
		s[k] = true
	}
	return s
}

// mergedOf is the edge k that stands for the edges of the graph stands: a
// derived edge at the origin of the first of them by kind, start and end,
// with the values of their attributes in that order, each once.
func mergedOf(k edgeKey, stands []graph.Edge) graph.Edge {
	sorted := slices.SortedFunc(slices.Values(stands), func(a, b graph.Edge) int {
		return compareKeys(edgeKey{a.Kind, a.From, a.To}, edgeKey{b.Kind, b.From, b.To})
	})
	e := graph.Edge{Kind: k.kind, From: k.from, To: k.to, Source: graph.SourceDerived, Origin: sorted[0].Origin}
	for _, x := range sorted {
		for key, values := range x.Attrs {
			for _, v := range values {
				if e.Attrs == nil {
					e.Attrs = map[string][]string{}
				}
				if !slices.Contains(e.Attrs[key], v) {
					e.Attrs[key] = append(e.Attrs[key], v)
				}
			}
		}
	}
	return e
}

func set(ids []string) map[string]bool {
	s := map[string]bool{}
	for _, id := range ids {
		s[id] = true
	}
	return s
}

//uc:checks view.sequence-follows-steps
func checkSequenceFollowsSteps(t rapid.TB, request Request) {
	g := graphOf(t, request.Fragment)
	w := newWorld(request.Fragment)
	got, err := Select(&g, request.Kind, request.Id, request.Expand, request.Layer, request.Level)
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
	var lifelines []string
	for _, p := range w.participants(request.Id) {
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

//uc:checks view.view-names-input-ancestors
func checkViewNamesInputAncestors(t rapid.TB, request Request) {
	g := graphOf(t, request.Fragment)
	w := newWorld(request.Fragment)
	input, applies := w.applicable(request.Kind, request.Id)
	got, err := Select(&g, request.Kind, request.Id, request.Expand, request.Layer, request.Level)
	if !applies {
		return
	}
	if err != nil {
		t.Fatalf("Select refused the %s view of %q, a node it applies to: %v", request.Kind, request.Id, err)
	}
	if !reflect.DeepEqual(got.Input, w.nodes[input]) {
		t.Fatalf("the %s view of %q gives the input %+v, not the node %q", request.Kind, request.Id, got.Input, input)
	}
	var ancestors []string
	for _, a := range got.Ancestors {
		if !reflect.DeepEqual(a, w.nodes[a.Id]) {
			t.Fatalf("the %s view of %q gives the ancestor %+v, which the graph does not hold", request.Kind, request.Id, a)
		}
		ancestors = append(ancestors, a.Id)
	}
	var want []string
	for at := input; ; {
		parents := w.from(at, graph.EdgeKindContains)
		if len(parents) == 0 || parents[0] == input || slices.Contains(want, parents[0]) {
			break
		}
		want = append([]string{parents[0]}, want...)
		at = parents[0]
	}
	if !slices.Equal(ancestors, want) {
		t.Fatalf("the %s view of %q gives the ancestors %q, not the nodes that contain %q, the outermost first: %q", request.Kind, request.Id, ancestors, input, want)
	}
}

//uc:checks view.view-cards-show-contents
func checkViewCardsShowContents(t rapid.TB, request Request) {
	g := graphOf(t, request.Fragment)
	w := newWorld(request.Fragment)
	input, applies := w.applicable(request.Kind, request.Id)
	if !applies {
		return
	}
	got, err := Select(&g, request.Kind, request.Id, request.Expand, request.Layer, request.Level)
	if err != nil {
		t.Fatalf("Select refused the %s view of %q, a node it applies to: %v", request.Kind, request.Id, err)
	}
	want := w.expected(asked{request.Kind, input, request.Layer, request.Level}, request.Expand)
	for _, n := range got.Nodes {
		if members := w.members(w.cardType(n.Id)); !reflect.DeepEqual(n.Members, members) {
			t.Fatalf("the %s view of %q gives %q the members %s, not the comparison of its design type %q with the code: %s",
				request.Kind, request.Id, n.Id, show(n.Members), w.cardType(n.Id), show(members))
		}
		if contents := w.contents(request.Kind, n.Id, want.folded[n.Id]); !reflect.DeepEqual(n.Contents, contents) {
			t.Fatalf("the %s view of %q gives the card of %q the contents %s, not %s", request.Kind, request.Id, n.Id, show(n.Contents), show(contents))
		}
		if details := w.details(n.Id, want.nodes); n.Details != details {
			t.Fatalf("the %s view of %q counts %d detail functions in %q that it does not draw, not %d", request.Kind, request.Id, n.Details, n.Id, details)
		}
	}
	for _, e := range got.Edges {
		stands := want.edges[edgeKey{e.Kind, e.From, e.To}]
		if stands == nil {
			stands = []graph.Edge{e.Edge}
		}
		if labels := w.labels(e.From, stands); !slices.Equal(e.Labels, labels) || e.Labels != nil && len(e.Labels) == 0 {
			t.Fatalf("the %s view of %q labels the edge %s %q %q with the operations %q, not %q", request.Kind, request.Id, e.Kind, e.From, e.To, e.Labels, labels)
		}
	}
}

// show writes a value of a card for a failure message.
func show(v any) string {
	out, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%+v", v)
	}
	return string(out)
}
