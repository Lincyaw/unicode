package design

import (
	"fmt"
	"slices"
	"strings"

	"github.com/alecthomas/participle/v2/lexer"

	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/syntax"
)

// vocabulary is the built-in labels of each notation and the kind of element
// each may be put on (docs/model.md §5).
var vocabulary = map[string]struct {
	notation string
	on       graph.NodeKind
}{
	"bounded-context": {"ddd", graph.NodeKindContainer},
	"aggregate":       {"ddd", graph.NodeKindType},
	"entity":          {"ddd", graph.NodeKindType},
	"value":           {"ddd", graph.NodeKindType},
	"domain-service":  {"ddd", graph.NodeKindType},
	"repository":      {"ddd", graph.NodeKindType},
	"factory":         {"ddd", graph.NodeKindType},
	"software-system": {"c4", graph.NodeKindContainer},
	"container":       {"c4", graph.NodeKindContainer},
	"component":       {"c4", graph.NodeKindContainer},
}

// label checks that a label of e belongs to a notation the system file
// declares and may be put on e (docs/model.md §10 rule 5).
func (l *loader) label(e *entity, label *syntax.Label) {
	name := string(label.Name)
	v, ok := vocabulary[name]
	switch {
	case !ok:
		l.report(e.file, label.Pos, "design.unknown-label", e.id,
			fmt.Sprintf("%s is not a label of any notation", name),
			"use a label of ddd (bounded-context, aggregate, entity, value, domain-service, repository, factory) "+
				"or of c4 (software-system, container, component), or remove it")
	case !l.notations[v.notation]:
		l.report(e.file, label.Pos, "design.undeclared-notation", e.id,
			fmt.Sprintf("the label %s belongs to the notation %s, which the system file does not declare", name, v.notation),
			fmt.Sprintf("add notation %s to design/system.uc, or remove the label", v.notation))
	case v.on != e.kind:
		l.report(e.file, label.Pos, "design.label-not-allowed", e.id,
			fmt.Sprintf("the label %s goes on a %s, but %s is a %s", name, v.on, e.id, e.kind),
			fmt.Sprintf("put the label %s on a %s, or remove it", name, v.on))
	}
}

// home is the component, system or external actor whose code holds e: the
// container that dependencies are counted from (docs/model.md §10 rules 4, 7, 13).
func home(e *entity) *entity {
	for x := e; x != nil; x = x.parent {
		if x.kind == graph.NodeKindContainer || x.is(graph.NodeKindActor, "external") {
			return x
		}
	}
	return nil
}

// allowed tells whether a may depend on b (docs/model.md §10 rule 13): b is
// a itself or a child component of it, or a or an ancestor of a uses b. An
// end that is not known allows everything, so that an unresolved reference
// gets no second diagnostic.
func allowed(a, b *entity) bool {
	if a == nil || b == nil || a == b || b.is(graph.NodeKindContainer, "component") && b.parent == a {
		return true
	}
	for x := a; x != nil; x = x.parent {
		if slices.Contains(x.uses, b) {
			return true
		}
	}
	return false
}

// depends checks that the types e refers to are in containers that e's
// container may depend on (docs/model.md §10 rule 4).
func (l *loader) depends(e *entity, refs []ref) {
	from := home(e)
	for _, r := range refs {
		if r.target == nil {
			continue
		}
		if to := home(r.target); !allowed(from, to) {
			l.undeclared(e, r.pos, from, to, r.written)
		}
	}
}

func (l *loader) undeclared(e *entity, pos lexer.Position, from, to *entity, what string) {
	l.report(e.file, pos, "design.undeclared-dependency", e.id,
		fmt.Sprintf("%s refers to %s in %s, which %s may not depend on", e.id, what, to.id, from.id),
		fmt.Sprintf("add uses %s to the component %s or to one of its ancestors, or refer to something %s may depend on", to.id, from.id, from.id))
}

// subscription checks an operation that subscribes to an event: it has
// exactly one parameter, of that event's type (docs/model.md §10 rule 6).
func (l *loader) subscription(op *entity, pos lexer.Position) {
	event := op.handles
	if event == nil {
		return
	}
	if event.form != "event" {
		l.notAnEvent(op, pos, event, "an operation can subscribe only to an event")
		return
	}
	if len(op.params) == 1 && !op.params[0].typ.resolved() {
		return
	}
	if len(op.params) != 1 || op.params[0].typ.named() != event {
		params := strings.Join(members(op.params), ", ")
		l.report(op.file, op.pos, "design.invalid-subscription", op.id,
			fmt.Sprintf("%s subscribes to %s, so it takes exactly one parameter of type %s, not (%s)", op.id, event.id, event.id, params),
			fmt.Sprintf("give the operation the single parameter event: %s", event.id))
	}
}

func (l *loader) notAnEvent(e *entity, pos lexer.Position, t *entity, why string) {
	l.report(e.file, pos, "design.not-an-event", e.id,
		fmt.Sprintf("%s is %s, not an event: %s", t.id, describe(t), why),
		"refer to a type of form event, or make "+t.id+" an event")
}
