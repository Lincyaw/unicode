package graph

import (
	"cmp"
	"fmt"
	"maps"
	"slices"

	"github.com/lincyaw/unicode/internal/diag"
)

// Layer is the abstraction level of a node (docs/model.md §1).
//
//uc:realizes graph.Layer
type Layer string

const (
	LayerDesign   Layer = "design"
	LayerCode     Layer = "code"
	LayerEvidence Layer = "evidence"
	LayerReview   Layer = "review"
)

// Source is where a node or an edge comes from (docs/model.md §2).
//
//uc:realizes graph.Source
type Source string

const (
	SourceDeclared  Source = "declared"
	SourceExtracted Source = "extracted"
	SourceDerived   Source = "derived"
	SourceProduced  Source = "produced"
)

// Status is how a declared item compares with the code (docs/model.md §6).
//
//uc:realizes graph.Status
type Status string

const (
	StatusVerified     Status = "verified"
	StatusMissing      Status = "missing"
	StatusStub         Status = "stub"
	StatusContradicted Status = "contradicted"
)

// NodeKind is the kind of a node (docs/model.md §3).
//
//uc:realizes graph.NodeKind
type NodeKind string

const (
	NodeKindContainer NodeKind = "container"
	NodeKindType      NodeKind = "type"
	NodeKindOp        NodeKind = "op"
	NodeKindActor     NodeKind = "actor"
	NodeKindFlow      NodeKind = "flow"
	NodeKindStep      NodeKind = "step"
	NodeKindProperty  NodeKind = "property"
	NodeKindCase      NodeKind = "case"
	NodeKindRun       NodeKind = "run"
	NodeKindResult    NodeKind = "result"
	NodeKindReview    NodeKind = "review"
)

// EdgeKind is the kind of an edge (docs/model.md §4).
//
//uc:realizes graph.EdgeKind
type EdgeKind string

const (
	EdgeKindContains   EdgeKind = "contains"
	EdgeKindUses       EdgeKind = "uses"
	EdgeKindExtends    EdgeKind = "extends"
	EdgeKindImplements EdgeKind = "implements"
	EdgeKindRefers     EdgeKind = "refers"
	EdgeKindSubscribes EdgeKind = "subscribes"
	EdgeKindImports    EdgeKind = "imports"
	EdgeKindCalls      EdgeKind = "calls"
	EdgeKindFrom       EdgeKind = "from"
	EdgeKindEmits      EdgeKind = "emits"
	EdgeKindHandles    EdgeKind = "handles"
	EdgeKindAbout      EdgeKind = "about"
	EdgeKindRealizes   EdgeKind = "realizes"
	EdgeKindChecks     EdgeKind = "checks"
	EdgeKindGenerates  EdgeKind = "generates"
	EdgeKindOwns       EdgeKind = "owns"
	EdgeKindDepends    EdgeKind = "depends"
	EdgeKindTraces     EdgeKind = "traces"
	EdgeKindExercises  EdgeKind = "exercises"
	EdgeKindReaches    EdgeKind = "reaches"
	EdgeKindCovers     EdgeKind = "covers"
)

// Node is one node of the graph. Attrs hold everything but the summary, as
// lists of strings in the canonical format (docs/design.md §5.2).
//
//uc:realizes graph.Node
type Node struct {
	Id      string
	Kind    NodeKind
	Form    string
	Layer   Layer
	Source  Source
	Origin  diag.Position
	Summary string
	Salient bool
	Attrs   map[string][]string
}

// Fragment is a set of nodes and edges: the code layer one backend extracts,
// or the input of a property.
//
//uc:realizes graph.Fragment
type Fragment struct {
	Nodes []Node
	Edges []Edge
}

// Edge is one edge of the graph; its identity is its kind, start and end.
//
//uc:realizes graph.Edge
type Edge struct {
	Kind   EdgeKind
	From   string
	To     string
	Source Source
	Origin diag.Position
	Attrs  map[string][]string
}

// DuplicateNode is the error of AddNode for an identity the graph already holds.
//
//uc:realizes graph.DuplicateNode
type DuplicateNode struct{ Id string }

func (e DuplicateNode) Error() string { return fmt.Sprintf("the graph already has a node %s", e.Id) }

// DuplicateEdge is the error of AddEdge for an edge the graph already holds.
//
//uc:realizes graph.DuplicateEdge
type DuplicateEdge struct {
	Kind     EdgeKind
	From, To string
}

func (e DuplicateEdge) Error() string {
	return fmt.Sprintf("the graph already has the edge %s %s %s", e.Kind, e.From, e.To)
}

// DanglingEdge is the error of AddEdge for an edge whose start or end, Missing,
// is not a node of the graph.
//
//uc:realizes graph.DanglingEdge
type DanglingEdge struct {
	Kind     EdgeKind
	From, To string
	Missing  string
}

func (e DanglingEdge) Error() string {
	return fmt.Sprintf("the edge %s %s %s ends at %s, which is not a node of the graph", e.Kind, e.From, e.To, e.Missing)
}

// Graph is a set of nodes indexed by identity and of edges indexed by kind,
// start and end. Its zero value is an empty graph. It keeps copies, so a
// caller that changes a node or an edge after adding it does not change the graph.
//
//uc:realizes graph.Graph
type Graph struct {
	nodes map[string]Node
	edges map[edgeKey]Edge
	// pending holds the edges of fragments that wait for Derive: bindings
	// whose target is not a node yet, and the realizes candidates of methods.
	pending []Edge
}

type edgeKey struct {
	kind     EdgeKind
	from, to string
}

// NewGraph returns an empty graph.
//
//uc:realizes graph.newGraph
func NewGraph() Graph { return Graph{} }

func (g *Graph) AddNode(node Node) error {
	if _, ok := g.nodes[node.Id]; ok {
		return DuplicateNode{Id: node.Id}
	}
	if g.nodes == nil {
		g.nodes = map[string]Node{}
	}
	node.Attrs = cloneAttrs(node.Attrs)
	g.nodes[node.Id] = node
	return nil
}

func (g *Graph) AddEdge(edge Edge) error {
	key := edgeKey{edge.Kind, edge.From, edge.To}
	if _, ok := g.edges[key]; ok {
		return DuplicateEdge{Kind: edge.Kind, From: edge.From, To: edge.To}
	}
	for _, end := range []string{edge.From, edge.To} {
		if _, ok := g.nodes[end]; !ok {
			return DanglingEdge{Kind: edge.Kind, From: edge.From, To: edge.To, Missing: end}
		}
	}
	if g.edges == nil {
		g.edges = map[edgeKey]Edge{}
	}
	edge.Attrs = cloneAttrs(edge.Attrs)
	g.edges[key] = edge
	return nil
}

// Node returns a copy of the node with the identity id, or nil.
func (g *Graph) Node(id string) *Node {
	n, ok := g.nodes[id]
	if !ok {
		return nil
	}
	n.Attrs = cloneAttrs(n.Attrs)
	return &n
}

// Nodes returns copies of all nodes, sorted by identity.
func (g *Graph) Nodes() []Node {
	nodes := make([]Node, 0, len(g.nodes))
	for _, id := range slices.Sorted(maps.Keys(g.nodes)) {
		n := g.nodes[id]
		n.Attrs = cloneAttrs(n.Attrs)
		nodes = append(nodes, n)
	}
	return nodes
}

// Edges returns copies of all edges, sorted by kind, start and end.
func (g *Graph) Edges() []Edge {
	edges := make([]Edge, 0, len(g.edges))
	for _, e := range g.edges {
		e.Attrs = cloneAttrs(e.Attrs)
		edges = append(edges, e)
	}
	slices.SortFunc(edges, func(a, b Edge) int {
		return cmp.Or(cmp.Compare(a.Kind, b.Kind), cmp.Compare(a.From, b.From), cmp.Compare(a.To, b.To))
	})
	return edges
}

// cloneAttrs copies attrs so that the graph and its callers share nothing.
// It keeps the difference between a missing key and an empty list.
func cloneAttrs(attrs map[string][]string) map[string][]string {
	if attrs == nil {
		return nil
	}
	out := make(map[string][]string, len(attrs))
	for k, v := range attrs {
		out[k] = slices.Clone(v)
	}
	return out
}
