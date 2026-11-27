package view

import (
	"slices"
	"strings"

	"github.com/lincyaw/unicode/internal/check"
	"github.com/lincyaw/unicode/internal/graph"
)

// cards fills in what the cards of a view show beyond its nodes
// (docs/design.md §7.5.1): the members of every drawn type that is a design
// type or realizes one; the contents of the architecture view's expanded
// containers and of every folded container; and how many detail functions
// each drawn code file and code type holds that the view does not draw.
func (ix index) cards(g *graph.Graph, v *View) {
	drawn, folded := map[string]bool{}, map[string]bool{}
	for _, n := range v.Nodes {
		drawn[n.Id] = true
	}
	for _, id := range v.Folded {
		folded[id] = true
	}
	expanded := func(id string) bool {
		return v.Kind == ViewKindArchitecture && isDesignContainer(ix.nodes[id]) && !folded[id]
	}
	var types []string
	for _, n := range v.Nodes {
		types = append(types, ix.designTypeOf(n.Id))
		if expanded(n.Id) {
			types = append(types, ix.declared(n.Id)...)
		}
	}
	members := check.Members(g, types)
	for i := range v.Nodes {
		n := &v.Nodes[i]
		n.Members = members[ix.designTypeOf(n.Id)]
		switch {
		case folded[n.Id]:
			for _, d := range ix.descendants(n.Id) {
				if ix.nodes[d].Kind == graph.NodeKindType {
					n.Contents = append(n.Contents, ViewNode{Node: ix.nodes[d]})
				}
			}
			slices.SortFunc(n.Contents, func(a, b ViewNode) int { return strings.Compare(a.Id, b.Id) })
		case expanded(n.Id):
			for _, d := range ix.declared(n.Id) {
				c := ViewNode{Node: ix.nodes[d], Members: members[d]}
				for _, op := range ix.targets(d, graph.EdgeKindContains) {
					if ix.nodes[op].Kind == graph.NodeKindOp {
						c.Contents = append(c.Contents, ViewNode{Node: ix.nodes[op]})
					}
				}
				n.Contents = append(n.Contents, c)
			}
		}
		if n.Layer == graph.LayerCode && (n.Kind == graph.NodeKindType || n.Kind == graph.NodeKindContainer && n.Form == "file") {
			for _, c := range ix.targets(n.Id, graph.EdgeKindContains) {
				if isDetailFunction(ix.nodes[c]) && !drawn[c] {
					n.Details++
				}
			}
		}
	}
}

// designTypeOf is the design type whose members the card of id lists: id
// itself when it is a design type, the first design type it realizes when
// it is a code type, and "" otherwise.
func (ix index) designTypeOf(id string) string {
	isDesignType := func(id string) bool {
		n := ix.nodes[id]
		return n.Kind == graph.NodeKindType && n.Layer == graph.LayerDesign
	}
	switch n := ix.nodes[id]; {
	case n.Kind != graph.NodeKindType:
	case n.Layer == graph.LayerDesign:
		return id
	default:
		for _, t := range ix.targets(id, graph.EdgeKindRealizes) {
			if isDesignType(t) {
				return t
			}
		}
	}
	return ""
}

// declared returns what the design container c declares directly that its
// expanded card lists: types, operations, flows and properties, by identity.
func (ix index) declared(c string) []string {
	var ids []string
	for _, d := range ix.targets(c, graph.EdgeKindContains) {
		switch ix.nodes[d].Kind {
		case graph.NodeKindType, graph.NodeKindOp, graph.NodeKindFlow, graph.NodeKindProperty:
			ids = append(ids, d)
		}
	}
	return ids
}

// labels returns the design operations that a drawn edge carries, sorted:
// for a depends, those that the callees of the calls it stands for realize;
// for a calls edge from a person, the operations the steps it stands for call.
func (ix index) labels(from string, stands []graph.Edge) []string {
	var ops []string
	for _, e := range stands {
		switch {
		case e.Kind == graph.EdgeKindDepends:
			for _, call := range e.Attrs["calls"] {
				_, callee, _ := strings.Cut(call, " ")
				for _, o := range ix.targets(callee, graph.EdgeKindRealizes) {
					if n := ix.nodes[o]; n.Kind == graph.NodeKindOp && n.Layer == graph.LayerDesign {
						ops = append(ops, o)
					}
				}
			}
		case e.Kind == graph.EdgeKindCalls && isPerson(ix.nodes[from]):
			ops = append(ops, e.To)
		}
	}
	slices.Sort(ops)
	return slices.Compact(ops)
}

func isPerson(n graph.Node) bool { return n.Kind == graph.NodeKindActor && n.Form == "person" }
