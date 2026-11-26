package query

import (
	"fmt"
	"strings"

	"github.com/lincyaw/unicode/internal/graph"
)

// NotFound is the error of a query given an identity that is not a node of the graph.
//
//uc:realizes query.NotFound
type NotFound struct{ Id string }

func (e NotFound) Error() string { return fmt.Sprintf("the graph has no node %q", e.Id) }

// NodeView is one node with the edges that leave it and the edges that reach it.
//
//uc:realizes query.NodeView
type NodeView struct {
	Node     graph.Node   `json:"node"`
	Outgoing []graph.Edge `json:"outgoing"`
	Incoming []graph.Edge `json:"incoming"`
}

// Get returns the node id with its edges, each list sorted by kind, start and end.
//
//uc:realizes query.get
func Get(g *graph.Graph, id string) (NodeView, error) {
	n := g.Node(id)
	if n == nil {
		return NodeView{}, NotFound{Id: id}
	}
	ix := newIndex(g)
	return NodeView{Node: *n, Outgoing: ix.out[id], Incoming: ix.in[id]}, nil
}

// Refs returns the edges that end at the node id, sorted by kind, start and end.
//
//uc:realizes query.refs
func Refs(g *graph.Graph, id string) ([]graph.Edge, error) {
	v, err := Get(g, id)
	return v.Incoming, err
}

// Search returns the nodes whose identity or summary contains text, of the
// given kind when kind is not nil, sorted by identity.
//
//uc:realizes query.search
func Search(g *graph.Graph, text string, kind *graph.NodeKind) []graph.Node {
	found := []graph.Node{}
	for _, n := range g.Nodes() {
		if (kind == nil || n.Kind == *kind) && (strings.Contains(n.Id, text) || strings.Contains(n.Summary, text)) {
			found = append(found, n)
		}
	}
	return found
}

// index holds the edges of a graph by start and by end, each list sorted
// by kind, start and end, and never nil.
type index struct {
	out, in map[string][]graph.Edge
}

func newIndex(g *graph.Graph) index {
	ix := index{out: map[string][]graph.Edge{}, in: map[string][]graph.Edge{}}
	for _, n := range g.Nodes() {
		ix.out[n.Id], ix.in[n.Id] = []graph.Edge{}, []graph.Edge{}
	}
	for _, e := range g.Edges() {
		ix.out[e.From] = append(ix.out[e.From], e)
		ix.in[e.To] = append(ix.in[e.To], e)
	}
	return ix
}

// present returns NotFound for the first identity that is not a node of g.
func present(g *graph.Graph, ids ...[]string) error {
	for _, list := range ids {
		for _, id := range list {
			if g.Node(id) == nil {
				return NotFound{Id: id}
			}
		}
	}
	return nil
}
