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
	Focus string     `json:"focus"`
	Edge  graph.Edge `json:"edge"`
	Node  graph.Node `json:"node"`
}

// Context is the slice of a set of nodes, the least context that answers a
// question about them (docs/design.md §7.3). Extras are the nodes that the
// kind of a focus node adds beyond one edge. Details counts, for each focus
// node, the distinct detail nodes one edge away or among its additions that
// Neighbors and Extras do not list.
//
//uc:realizes query.Context
type Context struct {
	Focus     []graph.Node   `json:"focus"`
	Ancestors []graph.Node   `json:"ancestors"`
	Neighbors []Neighbor     `json:"neighbors"`
	Extras    []graph.Node   `json:"extras"`
	Details   map[string]int `json:"details"`
}

// Slice returns the slice of the nodes ids: the nodes themselves, sorted by
// identity; their ancestors along contains, sorted by identity; every node
// one edge away from one of them, in either direction; and, sorted by
// identity, the additions of each focus node that are not focus nodes
// themselves. A detail node one edge away from a focus node f, or among
// f's additions, is listed only when f or the node itself is in expand;
// otherwise it is only counted under f.
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
	c := Context{Focus: []graph.Node{}, Neighbors: []Neighbor{}, Extras: []graph.Node{}, Details: map[string]int{}}
	for _, f := range focus {
		c.Focus = append(c.Focus, *g.Node(f))
	}
	c.Ancestors = ancestors(g, ix, focus)
	extras := map[string]bool{}
	for _, f := range focus {
		details := map[string]bool{}
		for _, a := range additions(g, ix, f) {
			switch {
			case slices.Contains(focus, a):
				continue
			case g.Node(a).Salient || expanded[f] || expanded[a]:
				extras[a] = true
			default:
				details[a] = true
			}
		}
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
	for _, id := range slices.Sorted(maps.Keys(extras)) {
		c.Extras = append(c.Extras, *g.Node(id))
	}
	return c, nil
}

// additions returns what docs/design.md §7.3 adds for the kind of the node
// f beyond one edge: for an operation, the type that contains it and that
// type's other operations; for a property, the types it reaches along
// refers edges through types, its symbols' types and theirs; for a flow,
// every step of its step tree and the functions those steps trace to.
func additions(g *graph.Graph, ix index, f string) []string {
	var found []string
	switch g.Node(f).Kind {
	case graph.NodeKindOp:
		for _, owner := range reach(g, ix.in[f], graph.EdgeKindContains, graph.NodeKindType, false) {
			found = append(found, owner)
			for _, op := range reach(g, ix.out[owner], graph.EdgeKindContains, graph.NodeKindOp, true) {
				if op != f {
					found = append(found, op)
				}
			}
		}
	case graph.NodeKindProperty:
		found = closure(g, ix, f, graph.EdgeKindRefers, graph.NodeKindType)
	case graph.NodeKindFlow:
		found = closure(g, ix, f, graph.EdgeKindContains, graph.NodeKindStep)
		for _, step := range slices.Clone(found) {
			found = append(found, reach(g, ix.out[step], graph.EdgeKindTraces, "", true)...)
		}
	}
	return found
}

// reach returns the other ends of the edges of the kind among edges, those
// edges leaving their node when forward and reaching it otherwise, kept
// when the end is of the node kind or kind is "".
func reach(g *graph.Graph, edges []graph.Edge, kind graph.EdgeKind, of graph.NodeKind, forward bool) []string {
	var ends []string
	for _, e := range edges {
		end := e.From
		if forward {
			end = e.To
		}
		if e.Kind == kind && (of == "" || g.Node(end).Kind == of) {
			ends = append(ends, end)
		}
	}
	return ends
}

// closure returns the nodes of the node kind that chains of edges of the
// kind lead to from start, every node after start being of that kind.
func closure(g *graph.Graph, ix index, start string, kind graph.EdgeKind, of graph.NodeKind) []string {
	seen := map[string]bool{start: true}
	var found []string
	for queue := []string{start}; len(queue) > 0; queue = queue[1:] {
		for _, next := range reach(g, ix.out[queue[0]], kind, of, true) {
			if !seen[next] {
				seen[next] = true
				found = append(found, next)
				queue = append(queue, next)
			}
		}
	}
	return found
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
