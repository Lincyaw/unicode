package view

import (
	"slices"
	"strings"

	"github.com/lincyaw/unicode/internal/check"
	"github.com/lincyaw/unicode/internal/graph"
)

// The cards a view's nodes carry, and the labels of its edges, straight
// from docs/design.md §7.5.1 and docs/model.md §10 rule 18.

func (w world) designType(id string) bool {
	n, ok := w.nodes[id]
	return ok && n.Kind == graph.NodeKindType && n.Layer == graph.LayerDesign
}

// cardType is the design type whose members the card of id lists: id when
// it is a design type, else the first design type a code type realizes.
func (w world) cardType(id string) string {
	n := w.nodes[id]
	switch {
	case n.Kind != graph.NodeKindType:
		return ""
	case n.Layer == graph.LayerDesign:
		return id
	}
	for _, t := range w.to(id, graph.EdgeKindRealizes) {
		if w.designType(t) {
			return t
		}
	}
	return ""
}

// members are the fields of the design type id, in the design's order,
// then its enum values and the code's other values, each compared with the
// one code type that realizes it. On the generated graphs, a code field's
// type is alike when it is the design's, or optional<T> for a design
// object type T.
func (w world) members(id string) []check.Member {
	if !w.designType(id) {
		return nil
	}
	var realizers []string
	for _, r := range w.from(id, graph.EdgeKindRealizes) {
		if n := w.nodes[r]; n.Layer == graph.LayerCode && n.Kind == graph.NodeKindType {
			realizers = append(realizers, r)
		}
	}
	compared := len(realizers) == 1
	var code graph.Node
	if compared {
		code = w.nodes[realizers[0]]
	}
	status := func(s graph.Status) *graph.Status {
		if !compared {
			return nil
		}
		return &s
	}
	t := w.nodes[id]
	var out []check.Member
	for _, f := range t.Attrs["fields"] {
		name, typ, _ := strings.Cut(f, ": ")
		m := check.Member{Kind: check.MemberKindField, Name: name, Type: typ, InDesign: true, Refers: []string{}}
		for _, c := range code.Attrs["fields"] {
			if n, ct, _ := strings.Cut(c, ": "); n == name {
				m.InCode, m.CodeType = true, ct
			}
		}
		for _, word := range strings.FieldsFunc(typ, func(r rune) bool { return strings.ContainsRune("<>, ", r) }) {
			if w.designType(word) && !slices.Contains(m.Refers, word) {
				m.Refers = append(m.Refers, word)
			}
		}
		switch alike := m.CodeType == typ || m.CodeType == "optional<"+typ+">" && w.designType(typ) && w.nodes[typ].Form == "object"; {
		case !m.InCode:
			m.Status = status(graph.StatusMissing)
		case !alike:
			m.Status = status(graph.StatusContradicted)
		default:
			m.Status = status(graph.StatusVerified)
		}
		out = append(out, m)
	}
	if t.Form != "enum" {
		return out
	}
	for _, v := range t.Attrs["values"] {
		in := slices.Contains(code.Attrs["values"], v)
		s := graph.StatusMissing
		if in {
			s = graph.StatusVerified
		}
		out = append(out, check.Member{Kind: check.MemberKindValue, Name: v, InDesign: true, InCode: in, Refers: []string{}, Status: status(s)})
	}
	for _, v := range code.Attrs["values"] {
		if !slices.Contains(t.Attrs["values"], v) {
			out = append(out, check.Member{Kind: check.MemberKindValue, Name: v, InCode: true, Refers: []string{}, Status: status(graph.StatusContradicted)})
		}
	}
	return out
}

// contents is what the card of the drawn node id lists: in the
// architecture view, for an expanded design container, the types (each
// with its members and operations), operations, flows and properties it
// declares; in any view, for a folded container, the types in it; by
// identity.
func (w world) contents(kind ViewKind, id string, folded bool) []ViewNode {
	var out []ViewNode
	switch {
	case folded:
		for _, d := range keys(w.below(id)) {
			if w.nodes[d].Kind == graph.NodeKindType {
				out = append(out, ViewNode{Node: w.nodes[d]})
			}
		}
	case kind == ViewKindArchitecture && w.designContainer(id):
		for _, d := range w.to(id, graph.EdgeKindContains) {
			n := w.nodes[d]
			switch n.Kind {
			case graph.NodeKindType:
				c := ViewNode{Node: n, Members: w.members(d)}
				for _, op := range w.to(d, graph.EdgeKindContains) {
					if w.nodes[op].Kind == graph.NodeKindOp {
						c.Contents = append(c.Contents, ViewNode{Node: w.nodes[op]})
					}
				}
				out = append(out, c)
			case graph.NodeKindOp, graph.NodeKindFlow, graph.NodeKindProperty:
				out = append(out, ViewNode{Node: n})
			}
		}
	}
	return out
}

// details is how many detail functions a drawn code file or code type
// contains directly that the view does not draw.
func (w world) details(id string, drawn map[string]bool) int {
	n := w.nodes[id]
	if codeFileOrType := n.Kind == graph.NodeKindType || n.Kind == graph.NodeKindContainer && n.Form == "file"; n.Layer != graph.LayerCode || !codeFileOrType {
		return 0
	}
	count := 0
	for _, c := range w.to(id, graph.EdgeKindContains) {
		if w.detailFunction(c) && !drawn[c] {
			count++
		}
	}
	return count
}

// side is where the drawn node id stands in the calls view on input
// (docs/design.md §7.5.1): a root when it is the input or an operation
// realizing it; a callee when it is a node the roots reach along calls and
// reaches edges, or a folded container a reached node is placed into; a
// caller otherwise.
func (w world) side(input, id string, x expectation) CallSide {
	roots := map[string]bool{input: true}
	for _, r := range w.from(input, graph.EdgeKindRealizes) {
		if w.nodes[r].Kind == graph.NodeKindOp {
			roots[r] = true
		}
	}
	if roots[id] {
		return CallSideRoot
	}
	reached := map[string]bool{}
	for grown := true; grown; {
		grown = false
		for _, e := range w.edges {
			if (e.Kind == graph.EdgeKindCalls || e.Kind == graph.EdgeKindReaches) && (roots[e.From] || reached[e.From]) && !reached[e.To] {
				reached[e.To], grown = true, true
			}
		}
	}
	for r := range reached {
		if r == id || x.home[r] == id && x.folded[id] {
			return CallSideCallee
		}
	}
	return CallSideCaller
}

// labels are the operations an edge from the node from carries, each with
// its identity and its text, given the edges of the graph it stands for: for
// a depends, those the callees of its calls realize, by design name; for a
// calls edge from a person, each step's operation, written as the flow
// writes the call, name(arguments).
func (w world) labels(from string, stands []graph.Edge) []EdgeLabel {
	name := func(id string) string {
		parts := strings.Split(id, ".")
		return parts[len(parts)-1]
	}
	var ops []EdgeLabel
	for _, e := range stands {
		var label []EdgeLabel
		switch {
		case e.Kind == graph.EdgeKindDepends:
			for _, call := range e.Attrs["calls"] {
				callee := call[strings.Index(call, " ")+1:]
				for _, o := range w.to(callee, graph.EdgeKindRealizes) {
					if n := w.nodes[o]; n.Kind == graph.NodeKindOp && n.Layer == graph.LayerDesign {
						label = append(label, EdgeLabel{Op: o, Text: name(o)})
					}
				}
			}
		case e.Kind == graph.EdgeKindCalls && w.nodes[from].Kind == graph.NodeKindActor && w.nodes[from].Form == "person":
			label = append(label, EdgeLabel{Op: e.To, Text: name(e.To) + "(" + strings.Join(w.nodes[e.From].Attrs["args"], ", ") + ")"})
		}
		for _, l := range label {
			if !slices.Contains(ops, l) {
				ops = append(ops, l)
			}
		}
	}
	slices.SortFunc(ops, func(a, b EdgeLabel) int {
		if a.Text != b.Text {
			return strings.Compare(a.Text, b.Text)
		}
		return strings.Compare(a.Op, b.Op)
	})
	return ops
}
