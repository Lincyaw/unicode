package query

import (
	"cmp"
	"slices"

	"github.com/lincyaw/unicode/internal/graph"
)

// Affected is a node a change reaches, with a shortest path of edges that
// leads from it to a changed node and so tells why.
//
//uc:realizes query.Affected
type Affected struct {
	Node graph.Node
	Path []graph.Edge
}

// impactKinds are the edges along which a change spreads backwards (docs/design.md §7.4).
var impactKinds = map[graph.EdgeKind]bool{
	graph.EdgeKindCalls: true, graph.EdgeKindReaches: true, graph.EdgeKindRefers: true,
	graph.EdgeKindSubscribes: true, graph.EdgeKindEmits: true, graph.EdgeKindRealizes: true,
	graph.EdgeKindTraces: true, graph.EdgeKindExercises: true, graph.EdgeKindDepends: true,
	graph.EdgeKindAbout: true, graph.EdgeKindChecks: true,
}

// Impact returns the salient nodes, other than the changed nodes ids, from
// which a path along the edges of impactKinds leads to one of ids; each comes
// with one shortest such path. The result is grouped by node kind and sorted
// by identity within a kind.
//
//uc:realizes query.impact
func Impact(g *graph.Graph, ids []string) ([]Affected, error) {
	if err := present(g, ids); err != nil {
		return nil, err
	}
	ix := newIndex(g)
	changed := map[string]bool{}
	paths := map[string][]graph.Edge{}
	queue := slices.Compact(slices.Sorted(slices.Values(ids)))
	for _, id := range queue {
		changed[id] = true
	}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		for _, e := range ix.in[id] {
			if _, seen := paths[e.From]; impactKinds[e.Kind] && !seen && !changed[e.From] {
				paths[e.From] = append([]graph.Edge{e}, paths[id]...)
				queue = append(queue, e.From)
			}
		}
	}
	affected := []Affected{}
	for id, path := range paths {
		if n := g.Node(id); n.Salient {
			affected = append(affected, Affected{Node: *n, Path: path})
		}
	}
	slices.SortFunc(affected, func(a, b Affected) int {
		return cmp.Or(cmp.Compare(a.Node.Kind, b.Node.Kind), cmp.Compare(a.Node.Id, b.Node.Id))
	})
	return affected, nil
}
