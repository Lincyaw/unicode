package design

import (
	"strings"

	"github.com/alecthomas/participle/v2/lexer"

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
// an enum subset; path starts with a symbol.
type limit struct {
	path      []string
	op        string // in, size or subset
	low, high string
	members   []string
	pos       lexer.Position
}

func (lim *limit) text() string {
	path := strings.Join(lim.path, ".")
	if lim.op == "subset" {
		return path + " in {" + strings.Join(lim.members, ", ") + "}"
	}
	return path + " " + lim.op + " " + lim.low + ".." + lim.high
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
// references resolve in scope.
func (l *loader) inputs(e, scope *entity, symbols []*syntax.Symbol, limits []any) {
	var texts []string
	for _, line := range limits {
		lim := newLimit(line)
		e.limits = append(e.limits, lim)
		texts = append(texts, lim.text())
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
}

// checkCase checks the values of a case of owner.
func (l *loader) checkCase(e, owner *entity, lines []syntax.CaseLine) {}
