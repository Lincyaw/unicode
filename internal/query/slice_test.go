package query

import (
	"cmp"
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
	extras := map[string]bool{}
	for f := range focus {
		counted := map[string]bool{}
		for a := range added(question.Fragment, nodes, f) {
			switch {
			case focus[a]:
			case nodes[a].Salient || expanded[f] || expanded[a]:
				extras[a] = true
			default:
				counted[a] = true
			}
		}
		for _, e := range question.Fragment.Edges {
			if e.From != f && e.To != f {
				continue
			}
			other := e.From
			if other == f {
				other = e.To
			}
			if nodes[other].Salient || expanded[f] || expanded[other] {
				want[entry{f, edgeKey{e.Kind, e.From, e.To}}] = other
			} else {
				counted[other] = true
			}
		}
		if len(counted) > 0 {
			details[f] = len(counted)
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
	if problem := sameNodes(got.Extras, slices.Sorted(maps.Keys(extras)), nodes); problem != "" {
		t.Fatalf("the slice's extras are not the salient or expanded nodes that the focus nodes' kinds add by §7.3, focus nodes excluded, sorted by identity: %s", problem)
	}
	if !maps.Equal(got.Details, details) {
		t.Fatalf("the slice counts the unexpanded detail nodes one edge away from each focus node or among its additions as %v, not %v", got.Details, details)
	}

	inSlice := maps.Clone(lineage)
	maps.Copy(inSlice, extras)
	for _, other := range want {
		inSlice[other] = true
	}
	var contains []graph.Edge
	for _, e := range question.Fragment.Edges {
		if e.Kind == graph.EdgeKindContains && inSlice[e.From] && inSlice[e.To] {
			contains = append(contains, e)
		}
	}
	slices.SortFunc(contains, func(a, b graph.Edge) int { return cmp.Or(cmp.Compare(a.From, b.From), cmp.Compare(a.To, b.To)) })
	if len(got.Edges) != len(contains) {
		t.Fatalf("the slice gives %d contains edges, yet %d contains edges join two of its nodes\ngot: %+v\nwant: %+v", len(got.Edges), len(contains), got.Edges, contains)
	}
	for i, e := range got.Edges {
		if !reflect.DeepEqual(e, contains[i]) {
			t.Fatalf("edge %d of the slice is %+v, yet the contains edges that join two of its nodes, sorted by kind, start and end, have %+v there", i+1, e, contains[i])
		}
	}
}

// added returns what docs/design.md §7.3 adds for the kind of the focus node
// f beyond one edge: an operation's containing type and that type's other
// operations; the types a property reaches along refers edges through types;
// a flow's steps at every depth and the ends of their traces edges.
func added(f graph.Fragment, nodes map[string]graph.Node, focus string) map[string]bool {
	found := map[string]bool{}
	grow := func(kind graph.EdgeKind, from map[string]bool, of graph.NodeKind) {
		for grown := true; grown; {
			grown = false
			for _, e := range f.Edges {
				if e.Kind == kind && (e.From == focus || from[e.From]) && nodes[e.To].Kind == of && e.To != focus && !found[e.To] {
					found[e.To], grown = true, true
				}
			}
		}
	}
	switch nodes[focus].Kind {
	case graph.NodeKindOp:
		owners := map[string]bool{}
		for _, e := range f.Edges {
			if e.Kind == graph.EdgeKindContains && e.To == focus && nodes[e.From].Kind == graph.NodeKindType {
				owners[e.From], found[e.From] = true, true
			}
		}
		for _, e := range f.Edges {
			if e.Kind == graph.EdgeKindContains && owners[e.From] && nodes[e.To].Kind == graph.NodeKindOp && e.To != focus {
				found[e.To] = true
			}
		}
	case graph.NodeKindProperty:
		grow(graph.EdgeKindRefers, found, graph.NodeKindType)
	case graph.NodeKindFlow:
		grow(graph.EdgeKindContains, found, graph.NodeKindStep)
		steps := maps.Clone(found)
		for _, e := range f.Edges {
			if e.Kind == graph.EdgeKindTraces && steps[e.From] {
				found[e.To] = true
			}
		}
	}
	return found
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
