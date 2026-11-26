package design

import (
	"fmt"
	"slices"
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

// enclosing is what a list of steps is part of: inside is set within a call or
// an async, whose receiver acts in its steps; receiver is nil when the
// operation does not resolve.
type enclosing struct {
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
func (s *stepper) list(parent *entity, lines []syntax.Step, w enclosing, branch string) {
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
			s.list(e, x.Lines, enclosing{inside: true, receiver: receiver(op)}, "")
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
			s.list(e, x.Lines, enclosing{inside: true, receiver: receiver(op)}, "")
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
// records its edges and checks it (docs/model.md §10 rules 7 and 8); it
// returns the operation, or nil.
func (s *stepper) invoke(e *entity, from syntax.Participant, opRef syntax.OpRef, args int, w enclosing) *entity {
	l := s.l
	sender := s.sender(e, from, w, acceptParticipant)
	op := l.refer(e, s.scope, opRef.Segments, opRef.Pos, acceptOp)
	l.edge(graph.EdgeKindCalls, e, op, e.file, opRef.Pos, graph.SourceDeclared, nil)
	if op == nil {
		return nil
	}
	if args != len(op.params) {
		s.argumentCount(e, op.id, "parameters", args, len(op.params))
	}
	if sender != nil {
		if from, to := s.senderHome(sender), home(op); !allowed(from, to) {
			l.undeclared(e, opRef.Pos, from, to, op.id)
		}
	}
	return op
}

// senderHome is the container whose dependencies a sender's calls follow: an
// actor's is the system, which contains it.
func (s *stepper) senderHome(sender *entity) *entity {
	if sender.kind == graph.NodeKindActor {
		return sender.parent
	}
	return home(sender)
}

func (s *stepper) argumentCount(e *entity, target, what string, got, want int) {
	s.l.report(e.file, e.pos, "design.argument-count", e.id,
		fmt.Sprintf("the step passes %d arguments, but %s has %d %s", got, target, want, what),
		fmt.Sprintf("pass exactly %d arguments", want))
}

// sender resolves the sender of a step, records its from edge, and checks
// it against what the step is part of: within a call or an async, the
// receiver acts, and a person acts only in the outermost steps (docs/model.md
// §10 rules 7, 8 and 11).
func (s *stepper) sender(e *entity, from syntax.Participant, w enclosing, want accept) *entity {
	l := s.l
	sender := l.refer(e, s.scope, from.Segments, from.Pos, want)
	l.edge(graph.EdgeKindFrom, e, sender, e.file, from.Pos, graph.SourceDeclared, nil)
	switch {
	case sender == nil || !w.inside:
	case sender.is(graph.NodeKindActor, "person"):
		l.report(e.file, from.Pos, "design.nested-person-sender", e.id,
			fmt.Sprintf("the person %s sends a step inside a call or an async, but a person sends only outermost steps", sender.id),
			"move the step out to the top of the flow, or let the receiver of the enclosing call send it")
	case w.receiver != nil && sender != w.receiver:
		l.report(e.file, from.Pos, "design.sender-not-receiver", e.id,
			fmt.Sprintf("inside a call or an async, %s acts, not %s", w.receiver.id, sender.id),
			"make "+w.receiver.id+" the sender, or move the step out of the enclosing call or async")
	}
	return sender
}

var acceptEmitter = accept{"a component or a type, since only code emits events", func(e *entity) bool {
	return e.is(graph.NodeKindContainer, "component") || e.is(graph.NodeKindType)
}}

// emit checks an emit step (docs/model.md §10 rule 9).
func (s *stepper) emit(e *entity, x *syntax.Emit, w enclosing) {
	l := s.l
	s.sender(e, x.From, w, acceptEmitter)
	event := l.refer(e, s.scope, x.Event.Segments, x.Event.Pos, acceptType)
	l.edge(graph.EdgeKindEmits, e, event, e.file, x.Event.Pos, graph.SourceDeclared, nil)
	switch {
	case event == nil:
	case event.form != "event":
		l.notAnEvent(e, x.Event.Pos, event, "a step can emit only an event")
	case len(x.Args) != len(event.fields):
		s.argumentCount(e, event.id, "fields", len(x.Args), len(event.fields))
		s.emitted[event] = true
	default:
		s.emitted[event] = true
	}
}

// on checks that an on block handles an error that the operation of the call
// before it raises (docs/model.md §10 rule 10); call is nil when it did not resolve.
func (s *stepper) on(e *entity, x *syntax.OnBlock, call *entity) {
	l := s.l
	handled := l.refer(e, s.scope, x.Error.Segments, x.Error.Pos, acceptType)
	l.edge(graph.EdgeKindHandles, e, handled, e.file, x.Error.Pos, graph.SourceDeclared, nil)
	if handled != nil && call != nil && !slices.Contains(call.raises, handled) {
		l.report(e.file, x.Error.Pos, "design.unraised-error", e.id,
			fmt.Sprintf("%s does not raise %s, so no on block can handle it after the call", call.id, handled.id),
			fmt.Sprintf("handle one of the errors that %s raises, or add %s to its raises", call.id, handled.id))
	}
}

// async checks that an async follows what makes it run: an event emitted
// before it that its operation subscribes to, or a send of its operation
// before it (docs/model.md §10 rule 11).
func (s *stepper) async(e *entity, op *entity, pos lexer.Position) {
	if op == nil || s.sent[op] || op.handles != nil && s.emitted[op.handles] {
		return
	}
	s.l.report(e.file, pos, "design.async-without-trigger", e.id,
		fmt.Sprintf("nothing before the async starts %s: no earlier step emits an event it subscribes to, and none sends it", op.id),
		fmt.Sprintf("emit an event that %s subscribes to, or send %s, before the async", op.id, op.id))
}
