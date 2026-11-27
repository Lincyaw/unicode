package check

import (
	"slices"
	"strings"

	"github.com/lincyaw/unicode/internal/graph"
)

// MemberKind tells whether a member of a type is a field or an enum value.
//
//uc:realizes check.MemberKind
type MemberKind string

const (
	MemberKindField MemberKind = "field"
	MemberKindValue MemberKind = "value"
)

// Member is a field or an enum value of a design type, compared with the
// code type that realizes it (docs/model.md §10 rule 18). Type and CodeType
// are a field's; Refers lists the design types its type names; Status is
// nil when no code type is compared.
//
//uc:realizes check.Member
type Member struct {
	Kind     MemberKind    `json:"kind"`
	Name     string        `json:"name"`
	Type     string        `json:"type,omitempty"`
	CodeType string        `json:"codeType,omitempty"`
	InDesign bool          `json:"inDesign"`
	InCode   bool          `json:"inCode"`
	Refers   []string      `json:"refers"`
	Status   *graph.Status `json:"status,omitempty"`
}

// Members lists the members of each design type in ids (docs/model.md §10
// rule 18): the fields the design writes, in their order, then an enum's
// values and the values only the code has; each compared with the one code
// type that realizes the type, and without a status when none or several do.
//
//uc:realizes check.members
func Members(g *graph.Graph, ids []string) map[string][]Member {
	node := func(id string) (graph.Node, bool) {
		if n := g.Node(id); n != nil {
			return *n, true
		}
		return graph.Node{}, false
	}
	wanted := map[string]bool{}
	for _, id := range ids {
		wanted[id] = true
	}
	realizers := map[string][]string{}
	for _, e := range g.Edges() {
		if r, ok := node(e.From); e.Kind == graph.EdgeKindRealizes && wanted[e.To] && e.Source == graph.SourceDeclared && ok &&
			r.Layer == graph.LayerCode && r.Kind == graph.NodeKindType {
			realizers[e.To] = append(realizers[e.To], e.From)
		}
	}
	out := map[string][]Member{}
	for id := range wanted {
		if n, ok := node(id); ok && n.Layer == graph.LayerDesign && n.Kind == graph.NodeKindType {
			r := ""
			if len(realizers[id]) == 1 {
				r = realizers[id][0]
			}
			out[id] = compare(node, id, r)
		}
	}
	return out
}

// compare lists the members of the design type id compared with the code
// type r, or with none when r is "": the fields the design writes, then an
// enum's values and the values only the code has. A field is verified when
// the code has it with the same type, missing when the code lacks it, and
// contradicted when the types differ; an enum value is verified when the
// code has a constant for it, missing when it has none, and contradicted
// when only the code has it.
func compare(node func(string) (graph.Node, bool), id, r string) []Member {
	design, _ := node(id)
	var code graph.Node
	if r != "" {
		code, _ = node(r)
	}
	status := func(s graph.Status) *graph.Status {
		if r == "" {
			return nil
		}
		return &s
	}
	var out []Member
	types := map[string]string{}
	for _, f := range code.Attrs["fields"] {
		n, typ, _ := strings.Cut(f, ": ")
		types[n] = typ
	}
	for _, f := range design.Attrs["fields"] {
		n, typ, _ := strings.Cut(f, ": ")
		got, in := types[n]
		s := graph.StatusVerified
		switch {
		case !in:
			s = graph.StatusMissing
		case !sameType(node, typ, got):
			s = graph.StatusContradicted
		}
		out = append(out, Member{Kind: MemberKindField, Name: n, Type: typ, CodeType: got, InDesign: true, InCode: in,
			Refers: designTypes(node, typ), Status: status(s)})
	}
	if design.Form != "enum" {
		return out
	}
	values, constants := design.Attrs["values"], code.Attrs["values"]
	for _, v := range values {
		in := slices.Contains(constants, v)
		s := graph.StatusMissing
		if in {
			s = graph.StatusVerified
		}
		out = append(out, Member{Kind: MemberKindValue, Name: v, InDesign: true, InCode: in, Refers: []string{}, Status: status(s)})
	}
	for _, v := range constants {
		if !slices.Contains(values, v) {
			out = append(out, Member{Kind: MemberKindValue, Name: v, InCode: true, Refers: []string{}, Status: status(graph.StatusContradicted)})
		}
	}
	return out
}

// designTypes lists, in the order they first appear, the design types that
// the type typ names.
func designTypes(node func(string) (graph.Node, bool), typ string) []string {
	out := []string{}
	var walk func(t typeExpr)
	walk = func(t typeExpr) {
		if n, ok := node(t.name); ok && n.Layer == graph.LayerDesign && n.Kind == graph.NodeKindType && !slices.Contains(out, t.name) {
			out = append(out, t.name)
		}
		for _, a := range t.args {
			walk(a)
		}
	}
	if t, ok := parseType(typ); ok {
		walk(t)
	}
	return out
}
