package query

import (
	"cmp"
	"maps"
	"slices"

	"github.com/lincyaw/unicode/internal/graph"
)

// Neighbor is a node one edge away from the focus node Focus, with that edge.
//
//uc:realizes query.Neighbor
type Neighbor struct {
	Focus string
	Edge  graph.Edge
	Node  graph.Node
}

// Context is the slice of a set of nodes, the least context that answers a
// question about them (docs/design.md §7.3). Details counts, for each focus
// node, the distinct detail nodes one edge away that Neighbors does not list.
//
//uc:realizes query.Context
type Context struct {
	Focus     []graph.Node
	Ancestors []graph.Node
	Neighbors []Neighbor
	Details   map[string]int
}

// Slice returns the slice of the nodes ids: the nodes themselves, sorted by
// identity; their ancestors along contains, sorted by identity; and every
// node one edge away from one of them, in either direction. A detail node
// one edge away from a focus node f is listed only when f or the node itself
// is in expand; otherwise it is only counted under f.
//
//uc:realizes query.slice
func Slice(g *graph.Graph, ids []string, expand []string) (Context, error) {
	if err := present(g, ids, expand); err != nil {
		return Context{}, err
	}
	focus := slices.Compact(slices.Sorted(slices.Values(ids)))
	expanded := map[string]bool{}
	for _, id := range expand {
		expanded[id] = true
	}
	ix := newIndex(g)
	c := Context{Focus: []graph.Node{}, Neighbors: []Neighbor{}, Details: map[string]int{}}
	for _, f := range focus {
		c.Focus = append(c.Focus, *g.Node(f))
	}
	c.Ancestors = ancestors(g, ix, focus)
	for _, f := range focus {
		details := map[string]bool{}
		for _, e := range edgesAt(ix, f) {
			other := e.To
			if other == f {
				other = e.From
			}
			n := *g.Node(other)
			if n.Salient || expanded[f] || expanded[other] {
				c.Neighbors = append(c.Neighbors, Neighbor{Focus: f, Edge: e, Node: n})
			} else {
				details[other] = true
			}
		}
		if len(details) > 0 {
			c.Details[f] = len(details)
		}
	}
	return c, nil
}

// edgesAt returns the edges that start or end at id, a loop once.
func edgesAt(ix index, id string) []graph.Edge {
	edges := slices.Clone(ix.out[id])
	for _, e := range ix.in[id] {
		if e.From != id {
			edges = append(edges, e)
		}
	}
	slices.SortFunc(edges, func(a, b graph.Edge) int {
		return cmp.Or(cmp.Compare(a.Kind, b.Kind), cmp.Compare(a.From, b.From), cmp.Compare(a.To, b.To))
	})
	return edges
}

// ancestors returns every node from which a chain of contains edges leads to
// one of focus, focus itself excluded, sorted by identity.
func ancestors(g *graph.Graph, ix index, focus []string) []graph.Node {
	seen := map[string]bool{}
	queue := slices.Clone(focus)
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		for _, e := range ix.in[id] {
			if e.Kind == graph.EdgeKindContains && !seen[e.From] {
				seen[e.From] = true
				queue = append(queue, e.From)
			}
		}
	}
	for _, f := range focus {
		delete(seen, f)
	}
	nodes := []graph.Node{}
	for _, id := range slices.Sorted(maps.Keys(seen)) {
		nodes = append(nodes, *g.Node(id))
	}
	return nodes
}
