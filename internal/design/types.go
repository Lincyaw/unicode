package design

import (
	"slices"
	"strings"

	"github.com/alecthomas/participle/v2/lexer"

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
	case "call":
		texts := make([]string, len(t.refs))
		for i, r := range t.refs {
			texts[i] = r.text()
		}
		return "call<" + strings.Join(texts, ", ") + ">"
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
