package design

import (
	"strconv"

	"github.com/alecthomas/participle/v2/lexer"

	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/syntax"
)

// stepper declares the steps of one flow, numbered in the order of the text
// (docs/model.md §7), and checks them (§10 rules 7–11). It keeps the events
// emitted and the operations sent so far, which an async may follow.
type stepper struct {
	l       *loader
	flow    *entity
	scope   *entity
	count   int
	emitted map[*entity]bool
	sent    map[*entity]bool
}

// within is what a list of steps is part of: inside is set within a call or
// an async, whose receiver acts in its steps; receiver is nil when the
// operation does not resolve.
type within struct {
	inside   bool
	receiver *entity
}

// step declares the next step under parent; branch is the label of the alt
// branch it is in, or "".
func (s *stepper) step(parent *entity, form string, pos lexer.Position, branch string) *entity {
	s.count++
	n := strconv.Itoa(s.count)
	e := &entity{id: s.flow.id + "#" + n, name: "#" + n, kind: graph.NodeKindStep, form: form, file: s.flow.file, pos: pos}
	var attrs map[string][]string
	if branch != "" {
		attrs = map[string][]string{"branch": {branch}}
	}
	s.l.declare(parent, e, attrs)
	e.setAttr("index", []string{n})
	return e
}

func names(ls []syntax.Lower) []string {
	out := make([]string, len(ls))
	for i, n := range ls {
		out[i] = string(n)
	}
	return out
}

// list declares and checks one list of steps under parent.
func (s *stepper) list(parent *entity, lines []syntax.Step, w within, branch string) {
	l := s.l
	var call *entity // the operation of the call that the next on blocks follow
	for _, line := range lines {
		if _, on := line.(*syntax.OnBlock); !on {
			if _, comment := line.(*syntax.Comment); !comment {
				call = nil
			}
		}
		switch x := line.(type) {
		case *syntax.Call:
			e := s.step(parent, "call", x.Pos, branch)
			e.setAttr("args", names(x.Args))
			op := s.invoke(e, x.From, x.Op, len(x.Args), w)
			call = op
			s.list(e, x.Lines, within{inside: true, receiver: receiver(op)}, "")
		case *syntax.Send:
			e := s.step(parent, "send", x.Pos, branch)
			e.setAttr("args", names(x.Args))
			if op := s.invoke(e, x.From, x.Op, len(x.Args), w); op != nil {
				s.sent[op] = true
			}
		case *syntax.Emit:
			e := s.step(parent, "emit", x.Pos, branch)
			e.setAttr("args", names(x.Args))
			s.emit(e, x, w)
		case *syntax.OnBlock:
			e := s.step(parent, "on", x.Pos, branch)
			s.on(e, x, call)
			s.list(e, x.Lines, w, "")
		case *syntax.Async:
			e := s.step(parent, "async", x.Pos, branch)
			op := l.refer(e, s.scope, x.Op.Segments, x.Op.Pos, acceptOp)
			l.edge(graph.EdgeKindCalls, e, op, e.file, x.Op.Pos, graph.SourceDeclared, nil)
			s.async(e, op, x.Pos)
			s.list(e, x.Lines, within{inside: true, receiver: receiver(op)}, "")
		case *syntax.Alt:
			e := s.step(parent, "alt", x.Pos, branch)
			var labels []string
			for _, b := range x.Branches {
				labels = append(labels, string(b.Label))
			}
			e.setAttr("label", labels)
			for _, b := range x.Branches {
				s.list(e, b.Lines, w, string(b.Label))
			}
		case *syntax.Opt:
			e := s.step(parent, "opt", x.Pos, branch)
			e.setAttr("label", []string{string(x.Label)})
			s.list(e, x.Lines, w, "")
		case *syntax.Loop:
			e := s.step(parent, "loop", x.Pos, branch)
			e.setAttr("label", []string{string(x.Label)})
			s.list(e, x.Lines, w, "")
		}
	}
}

// receiver is who receives a call of op: its type, its component or its
// external actor.
func receiver(op *entity) *entity {
	if op == nil {
		return nil
	}
	return op.parent
}

// invoke resolves the sender and the operation of a call or a send step e,
// records its edges and checks it; it returns the operation, or nil.
func (s *stepper) invoke(e *entity, from syntax.Participant, opRef syntax.OpRef, args int, w within) *entity {
	l := s.l
	s.sender(e, from, w)
	op := l.refer(e, s.scope, opRef.Segments, opRef.Pos, acceptOp)
	l.edge(graph.EdgeKindCalls, e, op, e.file, opRef.Pos, graph.SourceDeclared, nil)
	return op
}

// sender resolves the sender of a step and records its from edge.
func (s *stepper) sender(e *entity, from syntax.Participant, w within) *entity {
	l := s.l
	sender := l.refer(e, s.scope, from.Segments, from.Pos, acceptParticipant)
	l.edge(graph.EdgeKindFrom, e, sender, e.file, from.Pos, graph.SourceDeclared, nil)
	return sender
}

func (s *stepper) emit(e *entity, x *syntax.Emit, w within) {
	l := s.l
	s.sender(e, x.From, w)
	event := l.refer(e, s.scope, x.Event.Segments, x.Event.Pos, acceptType)
	l.edge(graph.EdgeKindEmits, e, event, e.file, x.Event.Pos, graph.SourceDeclared, nil)
	if event != nil {
		s.emitted[event] = true
	}
}

func (s *stepper) on(e *entity, x *syntax.OnBlock, call *entity) {
	l := s.l
	handled := l.refer(e, s.scope, x.Error.Segments, x.Error.Pos, acceptType)
	l.edge(graph.EdgeKindHandles, e, handled, e.file, x.Error.Pos, graph.SourceDeclared, nil)
}

func (s *stepper) async(e *entity, op *entity, pos lexer.Position) {}
