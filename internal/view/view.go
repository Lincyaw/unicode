package view

import (
	"fmt"
	"slices"

	"github.com/lincyaw/unicode/internal/check"
	"github.com/lincyaw/unicode/internal/graph"
)

// ViewKind is the kind of a view (docs/design.md §7.1).
//
//uc:realizes view.ViewKind
type ViewKind string

const (
	ViewKindArchitecture ViewKind = "architecture"
	ViewKindClass        ViewKind = "class"
	ViewKindSequence     ViewKind = "sequence"
	ViewKindActivity     ViewKind = "activity"
	ViewKindEvents       ViewKind = "events"
	ViewKindCalls        ViewKind = "calls"
	ViewKindProperty     ViewKind = "property"
	ViewKindCode         ViewKind = "code"
	ViewKindDdd          ViewKind = "ddd"
	ViewKindC4           ViewKind = "c4"
)

// ClassLayer is the layer the class view draws: the design's types, or the
// code types that realize them.
//
//uc:realizes view.ClassLayer
type ClassLayer string

const (
	ClassLayerDesign ClassLayer = "design"
	ClassLayerCode   ClassLayer = "code"
)

// C4Level is the level the C4 view draws: the system with its persons and
// external systems, or the containers inside it.
//
//uc:realizes view.C4Level
type C4Level string

const (
	C4LevelContext   C4Level = "context"
	C4LevelContainer C4Level = "container"
)

// NotApplicable is the error of Select for an input the view does not
// apply to, or a labelled notation the system file does not declare.
//
//uc:realizes view.NotApplicable
type NotApplicable struct {
	Kind   ViewKind
	Id     string
	Reason string
}

func (e NotApplicable) Error() string {
	return fmt.Sprintf("the %s view does not apply to %q: %s", e.Kind, e.Id, e.Reason)
}

// ViewNode is a node a view draws, with what its card shows beyond the node.
//
//uc:realizes view.ViewNode
type ViewNode struct {
	graph.Node
	Members  []check.Member `json:"members,omitempty"`
	Contents []ViewNode     `json:"contents,omitempty"`
	Details  int            `json:"details,omitempty"`
}

// EdgeLabel is an operation an edge carries: its identity, and how the
// edge's label writes it.
//
//uc:realizes view.EdgeLabel
type EdgeLabel struct {
	Op   string `json:"op"`
	Text string `json:"text"`
}

// ViewEdge is an edge a view draws. When an end is a folded container or a
// C4 element that stands for the nodes in it, Count is how many edges of
// the graph it stands for, and it is not an edge of the graph. Labels are
// the operations it carries.
//
//uc:realizes view.ViewEdge
type ViewEdge struct {
	graph.Edge
	Count  int         `json:"count,omitempty"`
	Labels []EdgeLabel `json:"labels,omitempty"`
}

// View is the data of one view: its input node with the nodes that contain
// it, the request's expansions, the class view's layer and the C4 view's
// level; the nodes and edges it draws, in the order the view prescribes;
// the containers drawn folded; and the number of utility containers it does
// not draw.
//
//uc:realizes view.View
type View struct {
	Kind      ViewKind     `json:"kind"`
	Input     graph.Node   `json:"input"`
	Ancestors []graph.Node `json:"ancestors"`
	Expand    []string     `json:"expand"`
	Layer     *ClassLayer  `json:"layer,omitempty"`
	Level     *C4Level     `json:"level,omitempty"`
	Nodes     []ViewNode   `json:"nodes"`
	Edges     []ViewEdge   `json:"edges"`
	Folded    []string     `json:"folded"`
	Hidden    int          `json:"hidden"`
}

// Select picks the subgraph of the view kind for the node id and folds it
// (docs/design.md §7.1–§7.2). The architecture, events, DDD and C4 views take
// "" for the whole system. An identity in expand is a container, a salient
// function, or a reaches edge written "reaches <from> <to>"; identities that
// name nothing in the view are ignored. Only the class view reads layer, and
// only the C4 view reads level.
//
//uc:realizes view.select
func Select(g *graph.Graph, kind ViewKind, id string, expand []string, layer ClassLayer, level C4Level) (View, error) {
	ix := newIndex(g)
	r, err := ix.request(kind, id, layer, level)
	if err != nil {
		return View{}, err
	}
	v := ix.fold(r, selectors[kind](ix, r), expand)
	ix.cards(g, &v)
	v.Input = ix.nodes[r.input]
	v.Ancestors = []graph.Node{}
	for _, a := range slices.Backward(ix.containers(r.input)) {
		v.Ancestors = append(v.Ancestors, ix.nodes[a])
	}
	v.Expand = append([]string{}, expand...)
	switch kind {
	case ViewKindClass:
		v.Layer = &r.layer
	case ViewKindC4:
		v.Level = &r.level
	}
	return v, nil
}

// Inputs are the nodes the view of the kind takes as its input, by
// identity: those for which Select, with no expansion, applies.
//
//uc:realizes view.inputs
func Inputs(g *graph.Graph, kind ViewKind) []string {
	ix := newIndex(g)
	ids := []string{}
	for id := range ix.nodes {
		if _, err := ix.request(kind, id, ClassLayerDesign, C4LevelContainer); err == nil {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids
}

// request is a view request that Select accepted: its input is a node the
// view applies to.
type request struct {
	kind  ViewKind
	input string
	layer ClassLayer
	level C4Level
}

// A selection is what a view picks before folding: candidate nodes, in the
// view's order, and the kinds of the edges it draws between them.
type selection struct {
	nodes []string
	kinds []graph.EdgeKind
	seen  map[string]bool
	// home, when set, is the container outside which every design container
	// folds whatever its focus; only utility ones are still left out.
	home string
	// boxOf, when set, maps a node that is not drawn to the node that stands
	// for it in the view, which is drawn, folded or left out in its turn; ""
	// when nothing stands for it.
	boxOf func(string) string
}

func (s *selection) add(ids ...string) {
	if s.seen == nil {
		s.seen = map[string]bool{}
	}
	for _, id := range ids {
		if !s.seen[id] {
			s.seen[id] = true
			s.nodes = append(s.nodes, id)
		}
	}
}

// wholeSystem are the views whose input defaults to the system.
var wholeSystem = []ViewKind{ViewKindArchitecture, ViewKindEvents, ViewKindDdd, ViewKindC4}

// request checks that the view applies to the node id, with the layer or
// level the view reads, and resolves "" to the system where the view
// defaults to it.
func (ix index) request(kind ViewKind, id string, layer ClassLayer, level C4Level) (request, error) {
	na := func(reason string) error { return NotApplicable{Kind: kind, Id: id, Reason: reason} }
	switch _, known := selectors[kind]; {
	case !known:
		return request{}, na("there is no such view kind")
	case kind == ViewKindClass && layer != ClassLayerDesign && layer != ClassLayerCode:
		return request{}, na(fmt.Sprintf("the class view draws the layer design or code, not %q", layer))
	case kind == ViewKindC4 && level != C4LevelContext && level != C4LevelContainer:
		return request{}, na(fmt.Sprintf("the C4 view draws the level context or container, not %q", level))
	}
	input := id
	if id == "" {
		if !slices.Contains(wholeSystem, kind) {
			return request{}, na("the view needs an input node")
		}
		if ix.system == "" {
			return request{}, na("the graph has no system container")
		}
		input = ix.system
	}
	n, ok := ix.nodes[input]
	switch {
	case !ok:
		return request{}, na("the graph has no such node")
	case !n.Salient:
		return request{}, na("the node is a detail, and views show only salient nodes")
	case !accepts[kind](n):
		return request{}, na("the view takes " + inputs[kind])
	case (kind == ViewKindDdd || kind == ViewKindC4) && !slices.Contains(ix.nodes[ix.system].Attrs["notations"], string(kind)):
		return request{}, na("the system file does not declare the notation " + string(kind))
	}
	return request{kind: kind, input: input, layer: layer, level: level}, nil
}

func isDesignContainer(n graph.Node) bool {
	return n.Kind == graph.NodeKindContainer && n.Layer == graph.LayerDesign
}

var inputs = map[ViewKind]string{
	ViewKindArchitecture: "a design container",
	ViewKindClass:        "a design container or a design type",
	ViewKindSequence:     "a flow",
	ViewKindActivity:     "a flow",
	ViewKindEvents:       "a design container",
	ViewKindCalls:        "an operation or a function",
	ViewKindProperty:     "a property or a design container",
	ViewKindCode:         "a code file or a code type",
	ViewKindDdd:          "a design container",
	ViewKindC4:           "a design container",
}

var accepts = map[ViewKind]func(graph.Node) bool{
	ViewKindArchitecture: isDesignContainer,
	ViewKindClass: func(n graph.Node) bool {
		return n.Layer == graph.LayerDesign && (n.Kind == graph.NodeKindContainer || n.Kind == graph.NodeKindType)
	},
	ViewKindSequence: func(n graph.Node) bool { return n.Kind == graph.NodeKindFlow },
	ViewKindActivity: func(n graph.Node) bool { return n.Kind == graph.NodeKindFlow },
	ViewKindEvents:   isDesignContainer,
	ViewKindCalls:    func(n graph.Node) bool { return n.Kind == graph.NodeKindOp },
	ViewKindProperty: func(n graph.Node) bool { return n.Kind == graph.NodeKindProperty || isDesignContainer(n) },
	ViewKindCode: func(n graph.Node) bool {
		return n.Layer == graph.LayerCode && (n.Kind == graph.NodeKindType || n.Kind == graph.NodeKindContainer && n.Form == "file")
	},
	ViewKindDdd: isDesignContainer,
	ViewKindC4:  isDesignContainer,
}
