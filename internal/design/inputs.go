package design

import (
	"fmt"
	"math/big"
	"slices"
	"strings"

	"github.com/alecthomas/participle/v2/lexer"

	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/syntax"
)

// symbol is an input of a flow or a property.
type symbol struct {
	name      string
	typ       *typ
	generated bool
	pos       lexer.Position
}

// limit is a `where` of a flow or a property: an interval (in), a size, or
// an enum subset; path starts with a symbol. at is the type in the symbol's
// type that it applies to, once its path resolves.
type limit struct {
	path      []string
	op        string // in, size or subset
	low, high string
	members   []string
	pos       lexer.Position
	at        *typ
}

func (lim *limit) text() string {
	path := strings.Join(lim.path, ".")
	if lim.op == "subset" {
		return path + " in {" + strings.Join(lim.members, ", ") + "}"
	}
	return path + " " + lim.op + " " + lim.low + ".." + lim.high
}

// empty tells whether an interval's low bound exceeds its high bound (docs/language.md §3).
func (lim *limit) empty() bool {
	return lim.op != "subset" && rat(lim.low).Cmp(rat(lim.high)) > 0
}

func newLimit(line any) *limit {
	switch x := line.(type) {
	case *syntax.IntervalLimit:
		return &limit{path: names(x.Path), op: x.Op, low: string(x.Low), high: string(x.High), pos: x.Pos}
	case *syntax.SubsetLimit:
		return &limit{path: names(x.Path), op: "subset", members: names(x.Members), pos: x.Pos}
	}
	panic("design: a limit of an unknown kind")
}

// inputs reads the symbols and limits of a flow or a property e, whose
// references resolve in scope, and checks their names against its cases'.
func (l *loader) inputs(e, scope *entity, symbols []*syntax.Symbol, limits []any, cases []*syntax.CaseDecl) {
	names := map[string]bool{}
	for _, sy := range symbols {
		l.unique(e, names, string(sy.Name), sy.Pos)
	}
	for _, c := range cases {
		if names[string(c.Name)] {
			l.unique(e, names, string(c.Name), c.Pos)
		}
	}
	var texts []string
	for _, line := range limits {
		lim := newLimit(line)
		e.limits = append(e.limits, lim)
		texts = append(texts, lim.text())
		if lim.empty() {
			l.report(e.file, lim.pos, "design.empty-interval", e.id,
				fmt.Sprintf("the limit %s admits no value: its low bound %s exceeds its high bound %s", lim.text(), lim.low, lim.high),
				fmt.Sprintf("write the low bound first, like %s..%s", lim.high, lim.low))
		}
	}
	e.setAttr("limits", texts)
	l.signatures = append(l.signatures, func() {
		var texts []string
		for _, sy := range symbols {
			t := l.typeOf(e, scope, sy.Type)
			s := symbol{name: string(sy.Name), typ: t, generated: sy.Domain != nil, pos: sy.Pos}
			e.symbols = append(e.symbols, s)
			l.refers(e, t)
			text := s.name + ": " + t.text()
			if s.generated {
				text += " generated"
			}
			texts = append(texts, text)
		}
		e.setAttr("symbols", texts)
	})
	l.domains = append(l.domains, func() {
		replyTargets := map[string]bool{}
		for _, s := range e.symbols {
			if containsReplies(s.typ) {
				if s.typ.kind != "replies" {
					l.report(e.file, s.pos, "design.invalid-replies-type", e.id, "a response script must be a direct input type, not nested in another type", "declare forall answers: replies<external.operation>")
				} else if len(s.typ.refs) > 0 {
					target := s.typ.refs[0].text()
					if replyTargets[target] {
						l.report(e.file, s.pos, "design.invalid-replies-type", e.id, "two response symbols target the same external operation", "keep exactly one response symbol for each external operation")
					}
					replyTargets[target] = true
				}
			}
			if why := generable(s.typ, map[*entity]bool{}, map[*entity]bool{}); !s.generated && why != "" {
				l.report(e.file, s.pos, "design.ungenerable-symbol", e.id,
					fmt.Sprintf("the symbol %s cannot be generated: %s", s.name, why),
					"give the symbol a type whose values can be built, or mark it generated with a description of its domain")
			}
		}
		for _, lim := range e.limits {
			l.place(e, lim)
		}
	})
}

func (e *entity) symbol(name string) *symbol {
	for i := range e.symbols {
		if e.symbols[i].name == name {
			return &e.symbols[i]
		}
	}
	return nil
}

// generable tells why values of t cannot be generated (docs/model.md §10 rule
// 23), or returns "". required holds the types that the value being built
// must contain, through fields that cannot be left empty: meeting one again is
// a recursion that no value ends. seen holds every type being checked, so
// that a recursion through a list, a set, a map or an optional ends.
func generable(t *typ, required, seen map[*entity]bool) string {
	switch t.kind {
	case "list", "set", "optional", "map":
		for _, e := range t.elems {
			if why := generable(e, map[*entity]bool{}, seen); why != "" {
				return why
			}
		}
		return ""
	case "replies":
		for _, op := range t.ops() {
			if op.result != nil {
				if why := generable(op.result, required, seen); why != "" {
					return why
				}
			}
			for _, failure := range op.raises {
				for _, f := range failure.fields {
					if why := generable(f.typ, required, seen); why != "" {
						return why
					}
				}
			}
		}
		return ""
	case "call":
		names := map[string]bool{}
		for _, op := range t.ops() {
			if names[op.name] {
				return fmt.Sprintf("%s has two operations named %s", t.text(), op.name)
			}
			names[op.name] = true
			for _, p := range op.params {
				if why := generable(p.typ, required, seen); why != "" {
					return why
				}
			}
		}
		return ""
	}
	n := t.named()
	switch {
	case n == nil:
		return ""
	case n.form == "interface" || n.form == "error":
		return describe(n) + " has no values to build"
	case required[n]:
		return n.id + " contains itself through fields that cannot be empty"
	case seen[n]:
		return ""
	}
	required[n], seen[n] = true, true
	defer delete(required, n)
	defer delete(seen, n)
	for _, f := range n.fields {
		base := f.typ
		for base.kind == "optional" {
			base = base.elems[0]
		}
		if base.named().is(graph.NodeKindType, "interface") {
			continue
		}
		if why := generable(f.typ, required, seen); why != "" {
			return why
		}
	}
	return ""
}

// fits tells whether a limit of kind op applies to t (docs/model.md §10 rule
// 25): an interval to int and float, a subset to an enum, a size to a list,
// a set, a map, a string or bytes.
func fits(op string, t *typ) bool {
	switch op {
	case "in":
		return t.builtin == "int" || t.builtin == "float"
	case "size":
		return t.kind == "list" || t.kind == "set" || t.kind == "map" || t.kind == "replies" || t.builtin == "string" || t.builtin == "bytes"
	}
	return t.named().is(graph.NodeKindType, "enum")
}

var limitKinds = map[string]string{
	"in":     "an interval applies to an int or a float",
	"size":   "a size applies to a list, a set, a map, a string or bytes",
	"subset": "a subset of values applies to an enum",
}

// place resolves the path of a limit of e from its symbol (docs/model.md §10
// rules 24 and 25): a list, a set or an optional passes on to its element and
// a map to its value; in a record the next segment is a field, in call<…> an
// operation and then one of its parameters. At its end, the limit applies to
// the first type along the elements that it fits.
func (l *loader) place(e *entity, lim *limit) {
	path := strings.Join(lim.path, ".")
	bad := func(why string) {
		l.report(e.file, lim.pos, "design.invalid-limit-path", e.id,
			fmt.Sprintf("the limit's path %s does not resolve: %s", path, why),
			"start the path with a symbol, and follow it through fields, or through an operation and one of its parameters for call<…>")
	}
	s := e.symbol(lim.path[0])
	switch {
	case s == nil:
		bad(lim.path[0] + " is not a symbol of " + e.id)
		return
	case s.generated:
		l.report(e.file, lim.pos, "design.limited-generated-symbol", e.id,
			fmt.Sprintf("the symbol %s is generated, so it takes no limits", s.name),
			"remove the limit, or describe the restriction in the generated symbol's domain")
		return
	}
	t, segs := s.typ, lim.path[1:]
	for t.resolved() {
		if len(segs) == 0 {
			for x := t; x != nil; x = x.elem() {
				if fits(lim.op, x) {
					l.settle(e, lim, x)
					return
				}
			}
			l.report(e.file, lim.pos, "design.invalid-limit", e.id,
				fmt.Sprintf("the limit on %s does not fit its type %s: %s", path, t.text(), limitKinds[lim.op]),
				"limit something of a type that this kind of limit applies to, or use another kind of limit")
			return
		}
		switch n := t.named(); {
		case t.elem() != nil:
			t = t.elem()
		case t.kind == "call":
			op := findEntity(t.ops(), segs[0])
			if op == nil || len(segs) < 2 {
				bad(fmt.Sprintf("after %s comes an operation of %s and then one of its parameters", strings.Join(lim.path[:len(lim.path)-len(segs)], "."), t.text()))
				return
			}
			p := findMember(op.params, segs[1])
			if p == nil {
				bad(fmt.Sprintf("%s has no parameter %s", op.id, segs[1]))
				return
			}
			t, segs = p.typ, segs[2:]
		case n != nil && len(n.fields) > 0:
			f := findMember(n.fields, segs[0])
			if f == nil {
				bad(fmt.Sprintf("%s has no field %s", n.id, segs[0]))
				return
			}
			t, segs = f.typ, segs[1:]
		default:
			bad(fmt.Sprintf("%s has no fields, so the path cannot go on to %s", t.text(), segs[0]))
			return
		}
	}
}

// settle records that a limit applies at t, once the values of an enum subset
// are among its values. An empty interval, already reported, stays unsettled,
// so that no case gets a second diagnostic for it.
func (l *loader) settle(e *entity, lim *limit, t *typ) {
	if lim.empty() {
		return
	}
	for _, m := range lim.members {
		if !slices.Contains(t.named().values, m) {
			l.report(e.file, lim.pos, "design.invalid-limit", e.id,
				fmt.Sprintf("%s is not a value of %s", m, t.named().id),
				"list only values of "+t.named().id)
			return
		}
	}
	lim.at = t
}

func findEntity(es []*entity, name string) *entity {
	for _, e := range es {
		if e.name == name {
			return e
		}
	}
	return nil
}

func findMember(ms []member, name string) *member {
	for i := range ms {
		if ms[i].name == name {
			return &ms[i]
		}
	}
	return nil
}

// rat reads a number of the format exactly.
func rat(n string) *big.Rat {
	r, ok := new(big.Rat).SetString(n)
	if !ok {
		panic("design: syntax admits only decimal numbers, yet " + n + " is not one")
	}
	return r
}
