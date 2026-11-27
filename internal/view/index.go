package view

import (
	"github.com/lincyaw/unicode/internal/graph"
)

// index holds a graph's nodes by identity and its edges by start and by
// end, each list sorted by kind, start and end.
type index struct {
	nodes   map[string]graph.Node
	out, in map[string][]graph.Edge
	edges   []graph.Edge
	// system is the identity of the design container of form system, or "".
	system string
}

func newIndex(g *graph.Graph) index {
	ix := index{nodes: map[string]graph.Node{}, out: map[string][]graph.Edge{}, in: map[string][]graph.Edge{}, edges: g.Edges()}
	for _, n := range g.Nodes() {
		ix.nodes[n.Id] = n
		if ix.system == "" && isDesignContainer(n) && n.Form == "system" {
			ix.system = n.Id
		}
	}
	for _, e := range ix.edges {
		ix.out[e.From] = append(ix.out[e.From], e)
		ix.in[e.To] = append(ix.in[e.To], e)
	}
	return ix
}

// targets returns the ends of the edges of the kind that start at id.
func (ix index) targets(id string, kind graph.EdgeKind) []string {
	var ids []string
	for _, e := range ix.out[id] {
		if e.Kind == kind {
			ids = append(ids, e.To)
		}
	}
	return ids
}

// targetsOf returns the ends of the edges of the kinds that start at id.
func (ix index) targetsOf(id string, kinds ...graph.EdgeKind) []string {
	var ids []string
	for _, k := range kinds {
		ids = append(ids, ix.targets(id, k)...)
	}
	return ids
}

// sources returns the starts of the edges of the kind that end at id.
func (ix index) sources(id string, kind graph.EdgeKind) []string {
	var ids []string
	for _, e := range ix.in[id] {
		if e.Kind == kind {
			ids = append(ids, e.From)
		}
	}
	return ids
}

// neighbors returns the nodes one edge of the kinds away from id, in either direction.
func (ix index) neighbors(id string, kinds ...graph.EdgeKind) []string {
	var ids []string
	for _, k := range kinds {
		ids = append(ids, ix.targets(id, k)...)
		ids = append(ids, ix.sources(id, k)...)
	}
	return ids
}

// descendants returns the nodes that id contains along a chain of contains
// edges, breadth first, id excluded.
func (ix index) descendants(id string) []string {
	seen := map[string]bool{id: true}
	var found []string
	for queue := []string{id}; len(queue) > 0; queue = queue[1:] {
		for _, c := range ix.targets(queue[0], graph.EdgeKindContains) {
			if !seen[c] {
				seen[c] = true
				found = append(found, c)
				queue = append(queue, c)
			}
		}
	}
	return found
}

// parent returns the container of id along contains, the first by identity
// if there are several, or "".
func (ix index) parent(id string) string {
	if p := ix.sources(id, graph.EdgeKindContains); len(p) > 0 {
		return p[0]
	}
	return ""
}

// containers returns the nodes that contain id along contains, innermost first.
func (ix index) containers(id string) []string {
	seen := map[string]bool{id: true}
	var chain []string
	for p := ix.parent(id); p != "" && !seen[p]; p = ix.parent(p) {
		seen[p] = true
		chain = append(chain, p)
	}
	return chain
}

// enclosing returns what encloses id, innermost first: a code file's owner
// along owns, else its container along contains, and so on upwards.
func (ix index) enclosing(id string) []string {
	seen := map[string]bool{id: true}
	var chain []string
	for {
		next := ix.parent(id)
		if owners := ix.sources(id, graph.EdgeKindOwns); len(owners) > 0 {
			next = owners[0]
		}
		if next == "" || seen[next] {
			return chain
		}
		seen[next] = true
		chain = append(chain, next)
		id = next
	}
}

// focus returns the focus of a design container, its own or else the
// nearest enclosing design container's; "" for any other node and for a
// container with no focus above it.
func (ix index) focus(id string) string {
	for seen := map[string]bool{}; !seen[id] && isDesignContainer(ix.nodes[id]); id = ix.parent(id) {
		seen[id] = true
		if f := ix.nodes[id].Attrs["focus"]; len(f) > 0 {
			return f[0]
		}
	}
	return ""
}
