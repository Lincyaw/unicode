package query

import (
	"maps"
	"reflect"
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/graph"
)

type edgeKey struct {
	kind     graph.EdgeKind
	from, to string
}

//uc:checks query.slice-is-minimal-context
func checkSliceIsMinimalContext(t rapid.TB, question Question) {
	g := graphOf(t, question.Fragment)
	got, err := Slice(&g, question.Ids, question.Expand)
	if err != nil {
		t.Fatalf("Slice refused the focus %q and the expansion %q, all nodes of the graph: %v", question.Ids, question.Expand, err)
	}
	nodes := nodesById(question.Fragment)
	edges := map[edgeKey]graph.Edge{}
	for _, e := range question.Fragment.Edges {
		edges[edgeKey{e.Kind, e.From, e.To}] = e
	}
	focus := set(question.Ids)
	if problem := sameNodes(got.Focus, slices.Sorted(maps.Keys(focus)), nodes); problem != "" {
		t.Fatalf("the slice's focus is not the asked nodes, each once, sorted by identity: %s", problem)
	}

	lineage := maps.Clone(focus)
	for grown := true; grown; {
		grown = false
		for _, e := range question.Fragment.Edges {
			if e.Kind == graph.EdgeKindContains && lineage[e.To] && !lineage[e.From] {
				lineage[e.From], grown = true, true
			}
		}
	}
	var ancestors []string
	for id := range lineage {
		if !focus[id] {
			ancestors = append(ancestors, id)
		}
	}
	slices.Sort(ancestors)
	if problem := sameNodes(got.Ancestors, ancestors, nodes); problem != "" {
		t.Fatalf("the slice's ancestors are not the nodes that contain a focus node along a chain of contains edges, sorted by identity: %s", problem)
	}

	expanded := set(question.Expand)
	type entry struct {
		focus string
		edge  edgeKey
	}
	listed := map[entry]bool{}
	want := map[entry]string{}
	details := map[string]int{}
	for f := range focus {
		counted := map[string]bool{}
		for _, e := range question.Fragment.Edges {
			if e.From != f && e.To != f {
				continue
			}
			other := e.From
			if other == f {
				other = e.To
			}
			switch {
			case nodes[other].Salient || expanded[f] || expanded[other]:
				want[entry{f, edgeKey{e.Kind, e.From, e.To}}] = other
			case !counted[other]:
				counted[other] = true
				details[f]++
			}
		}
	}
	for _, n := range got.Neighbors {
		k := edgeKey{n.Edge.Kind, n.Edge.From, n.Edge.To}
		other, ok := want[entry{n.Focus, k}]
		switch {
		case !ok:
			t.Fatalf("the slice lists the neighbor %q of %q along %s %q %q, which is not a salient or expanded node one edge away from that focus node", n.Node.Id, n.Focus, k.kind, k.from, k.to)
		case listed[entry{n.Focus, k}]:
			t.Fatalf("the slice lists the neighbor %q of %q along %s %q %q twice", n.Node.Id, n.Focus, k.kind, k.from, k.to)
		case !reflect.DeepEqual(n.Edge, edges[k]):
			t.Fatalf("the slice gives the edge %s %q %q as %+v, yet the graph holds %+v", k.kind, k.from, k.to, n.Edge, edges[k])
		case n.Node.Id != other || !reflect.DeepEqual(n.Node, nodes[other]):
			t.Fatalf("the slice gives the neighbor of %q along %s %q %q as %+v, yet the graph's node at the other end is %+v", n.Focus, k.kind, k.from, k.to, n.Node, nodes[other])
		}
		listed[entry{n.Focus, k}] = true
	}
	if len(listed) != len(want) {
		for e, other := range want {
			if !listed[e] {
				t.Fatalf("the slice leaves out the neighbor %q of %q along %s %q %q, which is salient or expanded", other, e.focus, e.edge.kind, e.edge.from, e.edge.to)
			}
		}
	}
	if !maps.Equal(got.Details, details) {
		t.Fatalf("the slice counts the unexpanded detail nodes one edge away from each focus node as %v, not %v", got.Details, details)
	}
}

func TestSliceIsMinimalContext(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		checkSliceIsMinimalContext(t, genSliceQuestion().Draw(t, "question"))
	})
}

func FuzzSliceIsMinimalContext(f *testing.F) {
	f.Fuzz(rapid.MakeFuzz(func(t *rapid.T) {
		checkSliceIsMinimalContext(t, genSliceQuestion().Draw(t, "question"))
	}))
}
