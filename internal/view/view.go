package view

import (
	"fmt"
	"slices"

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

// View is the data of one view: the nodes and edges it draws, in the order
// the view prescribes; the support containers drawn folded; and the number
// of utility containers it does not draw.
//
//uc:realizes view.View
type View struct {
	Kind   ViewKind     `json:"kind"`
	Nodes  []graph.Node `json:"nodes"`
	Edges  []graph.Edge `json:"edges"`
	Folded []string     `json:"folded"`
	Hidden int          `json:"hidden"`
}

// Select picks the subgraph of the view kind for the node id and folds it
// (docs/design.md §7.1–§7.2). The architecture and events views take "" for
// the whole system. An identity in expand is a container, a salient
// function, or a reaches edge written "reaches <from> <to>"; identities that
// name nothing in the view are ignored.
//
//uc:realizes view.select
func Select(g *graph.Graph, kind ViewKind, id string, expand []string) (View, error) {
	ix := newIndex(g)
	input, err := ix.input(kind, id)
	if err != nil {
		return View{}, err
	}
	sel := selectors[kind](ix, input)
	return ix.fold(kind, input, sel, expand), nil
}

// A selection is what a view picks before folding: candidate nodes, in the
// view's order, and the kinds of the edges it draws between them.
type selection struct {
	nodes []string
	kinds []graph.EdgeKind
	seen  map[string]bool
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

// input checks that the view applies to the node id and returns its
// identity, the system's for "" where the view defaults to it.
func (ix index) input(kind ViewKind, id string) (string, error) {
	na := func(reason string) error { return NotApplicable{Kind: kind, Id: id, Reason: reason} }
	if _, known := selectors[kind]; !known {
		return "", na("there is no such view kind")
	}
	if id == "" {
		if kind != ViewKindArchitecture && kind != ViewKindEvents {
			return "", na("the view needs an input node")
		}
		if ix.system == "" {
			return "", na("the graph has no system container")
		}
		id = ix.system
	}
	n, ok := ix.nodes[id]
	switch {
	case !ok:
		return "", na("the graph has no such node")
	case !n.Salient:
		return "", na("the node is a detail, and views show only salient nodes")
	case !accepts[kind](n):
		return "", na("the view takes " + inputs[kind])
	case (kind == ViewKindDdd || kind == ViewKindC4) && !slices.Contains(ix.nodes[ix.system].Attrs["notations"], string(kind)):
		return "", na("the system file does not declare the notation " + string(kind))
	}
	return id, nil
}

func isDesignContainer(n graph.Node) bool {
	return n.Kind == graph.NodeKindContainer && n.Layer == graph.LayerDesign
}

var inputs = map[ViewKind]string{
	ViewKindArchitecture: "a design container",
	ViewKindClass:        "a container or a type",
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
		return n.Kind == graph.NodeKindContainer || n.Kind == graph.NodeKindType
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
