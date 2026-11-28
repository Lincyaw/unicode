package monitor

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"

	"go.opentelemetry.io/otel/trace"
)

// Call is one monitored call in progress.
type Call struct {
	op      string
	judged  string
	ctx     context.Context
	span    trace.Span
	start   time.Time
	ended   bool
	window  *window
	pending []pendingJudge
}

type pendingJudge struct {
	check  string
	finish Finish
}

// Begin starts a monitored call of the design operation op, with receiver nil
// for an operation of a container and the arguments without a leading
// context.Context: it starts the call's span as a child of ctx's, or as a
// root when ctx is nil, and, when the call is sampled, collects its input and
// does the first half of each judgment. The wrapper then calls the real
// implementation with Context, defers Recover and calls End.
func Begin(ctx context.Context, op string, receiver any, args ...any) *Call {
	if ctx == nil {
		ctx = context.Background()
	}
	s := current()
	c := &Call{op: op, window: s.gates.open(receiver)}
	c.ctx, c.span = s.tracer.Start(ctx, op, trace.WithSpanKind(trace.SpanKindInternal))
	operation, ok := registered(op, receiver)
	if ok && Sampled(s.rates, op, rand.Float64()) {
		c.judged = operation.Op
		readable := c.window.read()
		reads := Reads{Receiver: readable, State: readable || c.window.gates == nil}
		for _, judge := range operation.Judges {
			finish, problem := before(judge, Observed{Receiver: receiver, Args: args, Reads: reads})
			switch {
			case problem != "":
				c.pending = append(c.pending, pendingJudge{judge.Check, failed(judge.Check, problem)})
			case finish != nil:
				c.pending = append(c.pending, pendingJudge{judge.Check, finish})
			}
		}
		// A call of an operation of a type is collected with its receiver, which
		// is read only when no other monitored call on it overlaps.
		if s.traffic && operation.Collect != nil && (receiver == nil || readable) && !s.full(operation.Op) {
			s.collect(operation, receiver, args)
		}
		c.window.done()
	}
	c.window.begin()
	// The duration runs from the call's start to its return, without the time
	// judging takes (docs/language.md E28).
	c.start = time.Now()
	return c
}

// Context is the context to pass to the real implementation: ctx with the
// call's span.
func (c *Call) Context() context.Context { return c.ctx }

// End finishes a call that returned: results are its results without the
// trailing error, failure that error.
func (c *Call) End(results []any, failure error) { c.end(results, failure, nil) }

// Recover, deferred by the wrapper before it calls the real implementation,
// handles a call that did not return: when it panicked, the call is a
// counterexample of every property judging it, and the panic goes on with the
// same value.
func (c *Call) Recover() {
	if c.ended {
		return
	}
	p := recover()
	if p == nil {
		// runtime.Goexit: the call neither returned nor panicked, so there is
		// nothing to judge.
		c.pending = nil
		c.end(nil, nil, nil)
		return
	}
	c.end(nil, nil, p)
	panic(p)
}

func (c *Call) end(results []any, failure error, panicked any) {
	duration := time.Since(c.start)
	c.ended = true
	s := current()
	readable := c.window.end()
	reads := Reads{Receiver: readable, State: readable || c.window.gates == nil}
	var judgments []Judgment
	for _, p := range c.pending {
		if j, ok := finish(p, results, failure, panicked, reads); ok {
			judgments = append(judgments, j)
			s.report(c.ctx, c.span, j)
		}
	}
	c.window.done()
	s.observe(c.ctx, c.op, duration)
	if len(judgments) > 0 {
		s.add(JudgedCall{Op: c.judged, At: c.start.UTC(), Duration: duration, Judgments: judgments})
	}
	c.span.End()
}

// before runs a judge's first half, recovering a panic of the judging code as
// a check error: the real call goes on either way. The error names the
// panic's type only, since its text may carry the program's data.
func before(judge Judge, observed Observed) (finish Finish, problem string) {
	defer func() {
		if p := recover(); p != nil {
			finish, problem = nil, fmt.Sprintf("the judgment panicked before the call with a value of type %T", p)
		}
	}()
	return judge.Before(observed), ""
}

// finish runs a judge's second half, recovering a panic of the judging code
// as a check error named by the panic's type.
func finish(p pendingJudge, results []any, failure error, panicked any, reads Reads) (j Judgment, ok bool) {
	defer func() {
		if problem := recover(); problem != nil {
			j, ok = Judgment{Check: p.check, Verdict: VerdictCheckError, Message: fmt.Sprintf("the judgment panicked after the call with a value of type %T", problem)}, true
		}
	}()
	return p.finish(results, failure, panicked, reads)
}

// failed is the Finish of a judgment whose first half failed.
func failed(check, problem string) Finish {
	return func([]any, error, any, Reads) (Judgment, bool) {
		return Judgment{Check: check, Verdict: VerdictCheckError, Message: problem}, true
	}
}
