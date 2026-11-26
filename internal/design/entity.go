package design

import (
	"strings"

	"github.com/alecthomas/participle/v2/lexer"

	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/syntax"
)

// entity is one design node while the files load: where it was declared,
// its place in the containment tree, and what the checks need to know of it.
type entity struct {
	id       string
	name     string
	kind     graph.NodeKind
	form     string
	source   graph.Source
	parent   *entity
	children map[string]*entity
	file     *syntax.File
	pos      lexer.Position
	summary  string
	attrs    map[string][]string
	// virtual is the root standing in for a missing system file; it is not a node.
	virtual bool

	uses    []*entity // a component's uses
	focus   string    // a component's focus, inherited when it has none
	fields  []member  // a type's fields
	values  []string  // an enum's values
	ops     []*entity // a type's operations, in the order written
	params  []member  // an operation's parameters
	raises  []*entity // an operation's errors
	handles *entity   // the event an operation subscribes to
	symbols []symbol  // a flow's or a property's symbols
	limits  []*limit  // a flow's or a property's limits
}

// member is a field of a type or a parameter of an operation.
type member struct {
	name string
	typ  *typ
}

func (e *entity) is(kind graph.NodeKind, forms ...string) bool {
	if e == nil || e.kind != kind {
		return false
	}
	for _, f := range forms {
		if e.form == f {
			return true
		}
	}
	return len(forms) == 0
}

func (e *entity) setAttr(key string, values []string) {
	if len(values) == 0 {
		return
	}
	if e.attrs == nil {
		e.attrs = map[string][]string{}
	}
	e.attrs[key] = values
}

// describe names what kind of node e is, for messages.
func describe(e *entity) string {
	switch e.kind {
	case graph.NodeKindContainer:
		if e.form == "system" {
			return "the system " + e.id
		}
		return "the component " + e.id
	case graph.NodeKindActor:
		return "the " + e.form + " actor " + e.id
	case graph.NodeKindType:
		return "the " + e.form + " type " + e.id
	case graph.NodeKindOp:
		return "the operation " + e.id
	case graph.NodeKindFlow:
		return "the flow " + e.id
	case graph.NodeKindProperty:
		return "the property " + e.id
	case graph.NodeKindCase:
		return "the case " + e.id
	}
	return "the step " + e.id
}

func isUpper(s string) bool { return s != "" && s[0] >= 'A' && s[0] <= 'Z' }

// resolve finds the node that a reference names (docs/format.md §2): one that
// starts with a type is looked up in scope, a single lower-case segment first
// in scope and then at the root, any other from the root. It returns nil when
// no node has the name.
func (l *loader) resolve(scope *entity, segs []string) *entity {
	e := l.root
	switch {
	case isUpper(segs[0]):
		e = scope
	case len(segs) == 1 && scope != nil && scope.children[segs[0]] != nil:
		return scope.children[segs[0]]
	}
	for _, s := range segs {
		if e == nil {
			return nil
		}
		e = e.children[s]
	}
	return e
}

// accept is what a place may refer to: a test, and its description for the diagnostic.
type accept struct {
	what string
	ok   func(*entity) bool
}

var (
	acceptType       = accept{"a type", func(e *entity) bool { return e.is(graph.NodeKindType) }}
	acceptError      = accept{"an error type", func(e *entity) bool { return e.is(graph.NodeKindType, "error") }}
	acceptOp         = accept{"an operation", func(e *entity) bool { return e.is(graph.NodeKindOp) }}
	acceptCallTarget = accept{"an operation or a type", func(e *entity) bool { return e.is(graph.NodeKindOp) || e.is(graph.NodeKindType) }}
	acceptUses       = accept{"a component or an external actor", func(e *entity) bool {
		return e.is(graph.NodeKindContainer, "component") || e.is(graph.NodeKindActor, "external")
	}}
	acceptParticipant = accept{"an actor, a component or a type", func(e *entity) bool {
		return e.is(graph.NodeKindActor) || e.is(graph.NodeKindContainer, "component") || e.is(graph.NodeKindType)
	}}
	acceptAbout = accept{"a component, a type, an operation or a flow", func(e *entity) bool {
		return e.is(graph.NodeKindContainer, "component") || e.is(graph.NodeKindType) || e.is(graph.NodeKindOp) || e.is(graph.NodeKindFlow)
	}}
	acceptCorpus = accept{"a property or a flow", func(e *entity) bool { return e.is(graph.NodeKindProperty) || e.is(graph.NodeKindFlow) }}
)

// refer resolves a reference written in the declaration of from, and checks
// that it names what its place accepts (docs/model.md §10 rule 1). It returns
// nil, after its diagnostic, when it does not.
func (l *loader) refer(from *entity, scope *entity, segs []string, pos lexer.Position, want accept) *entity {
	ref := strings.Join(segs, ".")
	target := l.resolve(scope, segs)
	switch {
	case target == nil:
		l.report(from.file, pos, "design.unresolved-reference", from.id,
			"the reference "+ref+" names no declared node; here it must name "+want.what,
			"declare it, or correct the reference: a reference that starts with a type is looked up in this component, "+
				"a single lower-case name in this component and then at the root, any other from the root, like order.pricing.Price")
		return nil
	case !want.ok(target):
		l.report(from.file, pos, "design.wrong-reference-kind", from.id,
			"the reference "+ref+" names "+describe(target)+", but here it must name "+want.what,
			"refer to "+want.what+" here")
		return nil
	}
	return target
}
