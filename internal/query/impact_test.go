package query

import (
	"cmp"
	"reflect"
	"slices"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/graph"
)

// spreading are the edge kinds of docs/design.md §7.4, along which a change spreads backwards.
var spreading = []graph.EdgeKind{graph.EdgeKindCalls, graph.EdgeKindReaches, graph.EdgeKindRefers,
	graph.EdgeKindSubscribes, graph.EdgeKindEmits, graph.EdgeKindRealizes, graph.EdgeKindTraces,
	graph.EdgeKindExercises, graph.EdgeKindDepends, graph.EdgeKindAbout, graph.EdgeKindChecks, graph.EdgeKindObserves, graph.EdgeKindSubstitutes}

//uc:checks query.impact-is-reverse-reachability
func checkImpactIsReverseReachability(t rapid.TB, question Question) {
	g := graphOf(t, question.Fragment)
	got, err := Impact(&g, question.Ids)
	if err != nil {
		t.Fatalf("Impact refused the changed nodes %q, all nodes of the graph: %v", question.Ids, err)
	}
	nodes := nodesById(question.Fragment)
	edges := map[edgeKey]graph.Edge{}
	for _, e := range question.Fragment.Edges {
		edges[edgeKey{e.Kind, e.From, e.To}] = e
	}

	// distance is the length of a shortest path from a node to a changed one,
	// found level by level over the fragment's edge list.
	distance := map[string]int{}
	frontier := set(question.Ids)
	for level := 0; len(frontier) > 0; level++ {
		next := map[string]bool{}
		for id := range frontier {
			distance[id] = level
		}
		for _, e := range question.Fragment.Edges {
			owner := e.Kind == graph.EdgeKindContains && nodes[e.From].Kind == graph.NodeKindProperty && nodes[e.To].Kind == graph.NodeKindObservation
			if _, known := distance[e.From]; (slices.Contains(spreading, e.Kind) || owner) && frontier[e.To] && !known {
				next[e.From] = true
			}
			if _, known := distance[e.To]; slices.Contains([]graph.EdgeKind{graph.EdgeKindRealizes, graph.EdgeKindChecks, graph.EdgeKindGenerates, graph.EdgeKindObserves, graph.EdgeKindSubstitutes}, e.Kind) && frontier[e.From] && !known {
				next[e.To] = true
			}
		}
		frontier = next
	}
	var want []string
	for id, d := range distance {
		if d > 0 && nodes[id].Salient {
			want = append(want, id)
		}
	}
	slices.Sort(want)

	var ids []string
	for _, a := range got {
		ids = append(ids, a.Node.Id)
	}
	slices.Sort(ids)
	if !slices.Equal(ids, want) {
		t.Fatalf("the impact of %q is %q, yet the salient nodes, other than the changed ones, from which the edges of §7.4 lead to a changed node are %q", question.Ids, ids, want)
	}
	if !slices.IsSortedFunc(got, func(a, b Affected) int {
		return cmp.Or(cmp.Compare(a.Node.Kind, b.Node.Kind), cmp.Compare(a.Node.Id, b.Node.Id))
	}) {
		t.Fatalf("the impact of %q is not grouped by kind and sorted by identity within a kind: %q", question.Ids, ids)
	}
	changed := set(question.Ids)
	for _, a := range got {
		id := a.Node.Id
		if !reflect.DeepEqual(a.Node, nodes[id]) {
			t.Fatalf("the impact gives the node %q as %+v, yet the graph holds %+v", id, a.Node, nodes[id])
		}
		if len(a.Path) != distance[id] {
			t.Fatalf("the path that explains why %q is affected has %d edges, yet the shortest has %d: %+v", id, len(a.Path), distance[id], a.Path)
		}
		at := id
		for i, e := range a.Path {
			k := edgeKey{e.Kind, e.From, e.To}
			owner := e.Kind == graph.EdgeKindContains && nodes[e.From].Kind == graph.NodeKindProperty && nodes[e.To].Kind == graph.NodeKindObservation
			forward := e.From == at && (slices.Contains(spreading, e.Kind) || owner)
			backward := e.To == at && slices.Contains([]graph.EdgeKind{graph.EdgeKindRealizes, graph.EdgeKindChecks, graph.EdgeKindGenerates, graph.EdgeKindObserves, graph.EdgeKindSubstitutes}, e.Kind)
			switch {
			case !forward && !backward:
				t.Fatalf("edge %d of the path for %q cannot explain a change from %q along %s: %+v", i+1, id, at, e.Kind, a.Path)
			case !reflect.DeepEqual(e, edges[k]):
				t.Fatalf("edge %d of the path for %q is %+v, which is not an edge of the graph", i+1, id, e)
			}
			if forward {
				at = e.To
			} else {
				at = e.From
			}
		}
		if !changed[at] {
			t.Fatalf("the path for %q ends at %q, which is not a changed node: %+v", id, at, a.Path)
		}
	}
}
