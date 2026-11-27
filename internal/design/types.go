package design

import (
	"slices"
	"strings"

	"github.com/alecthomas/participle/v2/lexer"

	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/syntax"
)

// typ is a type expression with its references resolved: a builtin, a named
// type (refs[0]), or a list, set, optional, map or call<…> (kind).
type typ struct {
	builtin string
	kind    string // list, set, optional, map or call
	elems   []*typ // list, set, optional: the element; map: the key and the value
	refs    []ref  // a named type, or the targets of call<…>
}

// ref is one reference inside a type expression: how it is written, where,
// and what it names (nil when it does not resolve).
type ref struct {
	written string
	pos     lexer.Position
	target  *entity
}

func (r ref) text() string {
	if r.target != nil {
		return r.target.id
	}
	return r.written
}

// named is the type that a named type expression names, or nil.
func (t *typ) named() *entity {
	if t.builtin != "" || t.kind != "" {
		return nil
	}
	return t.refs[0].target
}

// resolved tells whether every reference in t resolves.
func (t *typ) resolved() bool {
	return !slices.ContainsFunc(t.allRefs(), func(r ref) bool { return r.target == nil })
}

// elem is what a limit's path passes through t to: the element of a list, a
// set or an optional, the value of a map (docs/model.md §10 rule 25), or nil.
func (t *typ) elem() *typ {
	switch t.kind {
	case "list", "set", "optional":
		return t.elems[0]
	case "map":
		return t.elems[1]
	}
	return nil
}

// ops lists the operations a value of call<…> may call: each target
// operation, and every operation of each target type.
func (t *typ) ops() []*entity {
	var out []*entity
	for _, r := range t.refs {
		switch {
		case r.target.is(graph.NodeKindOp):
			out = append(out, r.target)
		case r.target != nil:
			out = append(out, r.target.ops...)
		}
	}
	return out
}

// typeOf resolves a type expression written in the declaration of from.
func (l *loader) typeOf(from, scope *entity, expr syntax.TypeExpr) *typ {
	switch x := expr.(type) {
	case *syntax.Builtin:
		return &typ{builtin: x.Name}
	case *syntax.NamedType:
		target := l.refer(from, scope, x.Ref.Segments, x.Ref.Pos, acceptType)
		return &typ{refs: []ref{{strings.Join(x.Ref.Segments, "."), x.Ref.Pos, target}}}
	case *syntax.ElemType:
		return &typ{kind: x.Kind, elems: []*typ{l.typeOf(from, scope, x.Elem)}}
	case *syntax.MapType:
		return &typ{kind: "map", elems: []*typ{l.typeOf(from, scope, x.Key), l.typeOf(from, scope, x.Value)}}
	case *syntax.RepliesType:
		r := x.Target
		target := l.refer(from, scope, r.Segments, r.Pos, acceptOp)
		if target != nil && !target.parent.is(graph.NodeKindActor, "external") {
			l.report(from.file, r.Pos, "design.invalid-replies-type", from.id, "a replies type must name an operation directly provided by an external system", "name one external operation in replies<external.operation>")
		}
		return &typ{kind: "replies", refs: []ref{{strings.Join(r.Segments, "."), r.Pos, target}}}
	case *syntax.CallType:
		t := &typ{kind: "call"}
		for _, r := range x.Targets {
			target := l.refer(from, scope, r.Segments, r.Pos, acceptCallTarget)
			t.refs = append(t.refs, ref{strings.Join(r.Segments, "."), r.Pos, target})
		}
		return t
	}
	panic("design: a type expression of an unknown kind")
}

// text writes t in the canonical format, every reference as the identity of
// what it names (docs/design.md §5.2).
func (t *typ) text() string {
	switch t.kind {
	case "":
		if t.builtin != "" {
			return t.builtin
		}
		return t.refs[0].text()
	case "map":
		return "map<" + t.elems[0].text() + ", " + t.elems[1].text() + ">"
	case "call", "replies":
		texts := make([]string, len(t.refs))
		for i, r := range t.refs {
			texts[i] = r.text()
		}
		return t.kind + "<" + strings.Join(texts, ", ") + ">"
	}
	return t.kind + "<" + t.elems[0].text() + ">"
}

// allRefs lists every reference in t, its elements' included, in the order written.
func (t *typ) allRefs() []ref {
	refs := slices.Clone(t.refs)
	for _, e := range t.elems {
		refs = append(refs, e.allRefs()...)
	}
	return refs
}

// members writes fields or parameters as `name: type`.
func members(ms []member) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.name + ": " + m.typ.text()
	}
	return out
}
