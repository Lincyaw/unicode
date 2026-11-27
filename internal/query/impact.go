package query

import (
	"cmp"
	"slices"

	"github.com/lincyaw/unicode/internal/graph"
)

// Affected is a node a change reaches, with a shortest connected path of
// original graph edges to a changed node. An observes edge can be read in
// either direction.
//
//uc:realizes query.Affected
type Affected struct {
	Node graph.Node   `json:"node"`
	Path []graph.Edge `json:"path"`
}

// impactKinds are the edges along which a change spreads backwards (docs/design.md §7.4).
var impactKinds = map[graph.EdgeKind]bool{
	graph.EdgeKindCalls: true, graph.EdgeKindReaches: true, graph.EdgeKindRefers: true,
	graph.EdgeKindSubscribes: true, graph.EdgeKindEmits: true, graph.EdgeKindRealizes: true,
	graph.EdgeKindTraces: true, graph.EdgeKindExercises: true, graph.EdgeKindDepends: true,
	graph.EdgeKindAbout: true, graph.EdgeKindChecks: true, graph.EdgeKindObserves: true, graph.EdgeKindSubstitutes: true,
}

// Impact returns the salient nodes affected by changes of ids: backwards
// along impactKinds, forwards along observes, and from an observation to
// its owning property. Each carries a shortest explanation path. Results
// are grouped by node kind and sorted by identity within a kind.
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
		visit := func(next string, edge graph.Edge) {
			if _, seen := paths[next]; !seen && !changed[next] {
				paths[next] = append([]graph.Edge{edge}, paths[id]...)
				queue = append(queue, next)
			}
		}
		for _, e := range ix.in[id] {
			if impactKinds[e.Kind] || observationOwner(g, e) {
				visit(e.From, e)
			}
		}
		for _, e := range ix.out[id] {
			if e.Kind == graph.EdgeKindObserves {
				visit(e.To, e)
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

// observationOwner is the one containment relation along which an
// observation change affects its owning property, not every ancestor.
func observationOwner(g *graph.Graph, edge graph.Edge) bool {
	return edge.Kind == graph.EdgeKindContains && g.Node(edge.From).Kind == graph.NodeKindProperty &&
		g.Node(edge.To).Kind == graph.NodeKindObservation
}
