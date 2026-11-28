package monitor

import (
	"context"
	crand "crypto/rand"
	"fmt"
	"math/rand/v2"
	"os"
	"reflect"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// Reads are the values a judgment may read: the receiver, when no other
// monitored call on the same receiver overlaps this one, and the state that
// named observations and the frame read, when no other monitored call
// overlaps it at all (docs/design.md §6.6).
type Reads struct {
	Receiver bool
	State    bool
}

// Observed is what a judge sees of a monitored call before it runs: the
// receiver, nil for an operation of a container, the arguments in order,
// without a leading context.Context, and what it may read now.
type Observed struct {
	Receiver any
	Args     []any
	Reads    Reads
}

// Finish completes a judgment after the call: results are the call's results
// without its trailing error, failure that error, panicked the value the call
// panicked with or nil, and reads what may still be read. It reports false
// when the property does not judge this call.
type Finish func(results []any, failure error, panicked any, reads Reads) (Judgment, bool)

// Judge judges one property or flow on the calls of one operation: Before does
// the part of docs/language.md E26 that precedes the call and returns the
// Finish that does the rest, or nil when the property does not judge the call.
type Judge struct {
	Check  string
	Before func(Observed) Finish
}

// Operation is what a monitor file registers for a design operation realized
// by a Go method or function: the operation's identity, its judges, and, when
// its calls may be collected, how to collect one.
type Operation struct {
	Op      string
	Judges  []Judge
	Collect func(receiver any, args []any) (Collected, error)
}

// target is how a monitored call finds its operation: the implementation's
// Go type and the operation's name for a method, or no type and the
// operation's identity for a function.
type target struct {
	receiver reflect.Type
	name     string
}

var registry struct {
	sync.RWMutex
	wrappers   map[reflect.Type]func(any) any
	entries    map[uintptr]any
	operations map[target]Operation
}

// RegisterWrapper registers the interface wrapper of the Go interface iface:
// wrap returns a value of iface that calls next, the real implementation.
// Monitor files call it from init.
func RegisterWrapper(iface reflect.Type, wrap func(next any) any) {
	registry.Lock()
	defer registry.Unlock()
	if registry.wrappers == nil {
		registry.wrappers = map[reflect.Type]func(any) any{}
	}
	registry.wrappers[iface] = wrap
}

// RegisterEntry registers wrapped, a function of fn's type that calls fn, as
// the entry wrapper of fn. Monitor files call it from init.
func RegisterEntry(fn, wrapped any) {
	registry.Lock()
	defer registry.Unlock()
	if registry.entries == nil {
		registry.entries = map[uintptr]any{}
	}
	registry.entries[reflect.ValueOf(fn).Pointer()] = wrapped
}

// RegisterMethod registers the operation that the method name of the Go type
// receiver realizes; RegisterFunction registers the container operation op
// that a Go function realizes. Monitor files call them from init.
func RegisterMethod(receiver reflect.Type, name string, op Operation) {
	register(target{receiver, name}, op)
}

// RegisterFunction registers the container operation that a Go function
// realizes, by the operation's identity.
func RegisterFunction(op Operation) { register(target{nil, op.Op}, op) }

func register(at target, op Operation) {
	registry.Lock()
	defer registry.Unlock()
	if registry.operations == nil {
		registry.operations = map[target]Operation{}
	}
	registry.operations[at] = op
}

// Wrap returns the interface wrapper of impl when a monitor file registered
// one for I, and impl itself otherwise.
func Wrap[I any](impl I) I {
	registry.RLock()
	wrap := registry.wrappers[reflect.TypeFor[I]()]
	registry.RUnlock()
	if wrap == nil {
		return impl
	}
	if wrapped, ok := wrap(impl).(I); ok {
		return wrapped
	}
	return impl
}

// Entry returns the entry wrapper of fn when a monitor file registered one,
// and fn itself otherwise.
func Entry[F any](fn F) F {
	value := reflect.ValueOf(fn)
	if value.Kind() != reflect.Func || value.IsNil() {
		return fn
	}
	registry.RLock()
	wrapped := registry.entries[value.Pointer()]
	registry.RUnlock()
	if f, ok := wrapped.(F); ok {
		return f
	}
	return fn
}

// Call is one monitored call in progress.
type Call struct {
	op       string
	ctx      context.Context
	span     trace.Span
	start    time.Time
	ended    bool
	judged   string
	pending  []pendingJudge
	overlaps *overlap
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
	c := &Call{op: op}
	c.ctx, c.span = s.tracer.Start(ctx, op, trace.WithSpanKind(trace.SpanKindInternal))
	// The duration runs from the call's start to its return, without the time
	// judging takes (docs/language.md E28).
	defer func() { c.start = time.Now() }()
	at := target{nil, op}
	if receiver != nil {
		at = target{reflect.TypeOf(receiver), op[strings.LastIndexByte(op, '.')+1:]}
	}
	registry.RLock()
	operation, registered := registry.operations[at]
	registry.RUnlock()
	c.overlaps = s.enter(receiver)
	if !registered || !Sampled(s.rates, op, rand.Float64()) {
		return c
	}
	c.judged = operation.Op
	reads := c.overlaps.reads()
	for _, judge := range operation.Judges {
		finish, problem := before(judge, Observed{Receiver: receiver, Args: args, Reads: reads})
		switch {
		case problem != "":
			c.pending = append(c.pending, pendingJudge{judge.Check, failed(judge.Check, problem)})
		case finish != nil:
			c.pending = append(c.pending, pendingJudge{judge.Check, finish})
		}
	}
	if s.traffic && operation.Collect != nil {
		s.collect(operation, receiver, args)
	}
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
	reads := s.leave(c.overlaps)
	var judgments []Judgment
	for _, p := range c.pending {
		j, ok := finish(p, results, failure, panicked, reads)
		if !ok {
			continue
		}
		judgments = append(judgments, j)
		s.report(c.ctx, c.span, j)
	}
	s.duration.Record(c.ctx, duration.Seconds(), metric.WithAttributes(attribute.String("unicode.op", c.op)))
	if len(judgments) > 0 {
		s.add(JudgedCall{Op: c.judged, At: c.start.UTC(), Duration: duration, Judgments: judgments})
	}
	c.span.End()
}

// before runs a judge's first half, recovering a panic of the judging code as
// a check error: the real call goes on either way.
func before(judge Judge, observed Observed) (finish Finish, problem string) {
	defer func() {
		if p := recover(); p != nil {
			finish, problem = nil, fmt.Sprintf("the judgment panicked before the call: %v", p)
		}
	}()
	return judge.Before(observed), ""
}

// finish runs a judge's second half, recovering a panic of the judging code
// as a check error.
func finish(p pendingJudge, results []any, failure error, panicked any, reads Reads) (j Judgment, ok bool) {
	defer func() {
		if problem := recover(); problem != nil {
			j, ok = Judgment{Check: p.check, Verdict: VerdictCheckError, Message: fmt.Sprintf("the judgment panicked after the call: %v", problem)}, true
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

// overlap tracks whether other monitored calls overlap one call.
type overlap struct {
	key        any
	starts     uint64
	keyStarts  uint64
	sameActive bool
	anyActive  bool
}

func (o *overlap) reads() Reads { return Reads{Receiver: !o.sameActive, State: !o.anyActive} }

// state is the monitor of the program: its configuration from the
// environment, its instruments, and the record period in progress.
type state struct {
	rates    Rates
	problems []Diagnostic
	key      []byte
	dir      string
	traffic  bool
	build    Build

	tracer         trace.Tracer
	applicable     metric.Int64Counter
	counterexample metric.Int64Counter
	outside        metric.Int64Counter
	checkErrors    metric.Int64Counter
	writeFailures  metric.Int64Counter
	duration       metric.Float64Histogram

	mu        sync.Mutex
	from      time.Time
	calls     []JudgedCall
	active    int
	starts    uint64
	receivers map[any]*activity
	collected map[string]int
	queue     chan job
	started   bool
}

type activity struct {
	active int
	starts uint64
}

var (
	monitorOnce  sync.Once
	monitorState *state
)

// period is how long a record period lasts.
const period = time.Minute

// current is the program's monitor, configured from the environment on first
// use.
func current() *state {
	monitorOnce.Do(func() { monitorState = configure() })
	return monitorState
}

func configure() *state {
	s := &state{rates: Rates{Fallback: defaultRate, Ops: map[string]float64{}}, dir: ".unicode", receivers: map[any]*activity{}, collected: map[string]int{}, from: time.Now()}
	if text, ok := os.LookupEnv("UNICODE_MONITOR_RATE"); ok {
		rates, err := ParseRates(text)
		if err != nil {
			s.problems = append(s.problems, Diagnostic{Code: "monitor.bad-rates", Subject: "UNICODE_MONITOR_RATE", Path: "design/system.uc", Line: 1, Column: 1,
				Message: err.Error() + "; the monitor samples by the default rate", Fix: "set UNICODE_MONITOR_RATE to fallback[,operation=rate]... with rates from 0 to 1"})
		} else {
			s.rates = rates
		}
	}
	if dir := os.Getenv("UNICODE_MONITOR_DIR"); dir != "" {
		s.dir = dir
	}
	s.traffic = os.Getenv("UNICODE_MONITOR_TRAFFIC") == "on"
	if key := os.Getenv("UNICODE_MONITOR_KEY"); key != "" {
		s.key = []byte(key)
	} else {
		s.key = make([]byte, 32)
		_, _ = crand.Read(s.key)
	}
	s.build = buildOf(debug.ReadBuildInfo())
	meter := otel.Meter("github.com/lincyaw/unicode/monitor")
	s.tracer = otel.Tracer("github.com/lincyaw/unicode/monitor")
	s.applicable, _ = meter.Int64Counter("unicode.monitor.applicable", metric.WithDescription("monitored calls applicable to a property or flow"))
	s.counterexample, _ = meter.Int64Counter("unicode.monitor.counterexamples", metric.WithDescription("monitored calls that are counterexamples"))
	s.outside, _ = meter.Int64Counter("unicode.monitor.outside_domain", metric.WithDescription("monitored calls outside a property's domain"))
	s.checkErrors, _ = meter.Int64Counter("unicode.monitor.check_errors", metric.WithDescription("judgments that could not be made"))
	s.writeFailures, _ = meter.Int64Counter("unicode.monitor.write_failures", metric.WithDescription("run records and collected calls that could not be written"))
	s.duration, _ = meter.Float64Histogram("unicode.monitor.duration", metric.WithUnit("s"), metric.WithDescription("durations of monitored calls"))
	return s
}

// buildOf reads the commit and whether the working tree had changes from the
// build information Go writes into the program.
func buildOf(info *debug.BuildInfo, ok bool) Build {
	out := Build{Dirty: true}
	if !ok {
		return out
	}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			out.Commit = setting.Value
		case "vcs.modified":
			out.Dirty = setting.Value == "true"
		}
	}
	if out.Commit == "" {
		out.Dirty = true
	}
	return out
}

// enter counts a call as active, on its receiver too when the receiver has
// an identity of its own.
func (s *state) enter(receiver any) *overlap {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.start()
	s.active++
	s.starts++
	o := &overlap{starts: s.starts, anyActive: s.active > 1}
	if key, ok := identity(receiver); ok {
		a := s.receivers[key]
		if a == nil {
			a = &activity{}
			s.receivers[key] = a
		}
		a.active++
		a.starts++
		o.key, o.keyStarts, o.sameActive = key, a.starts, a.active > 1
	}
	return o
}

// leave ends a call's activity and tells what may be read after it: nothing
// another call could have changed while it ran.
func (s *state) leave(o *overlap) Reads {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active--
	if s.starts != o.starts {
		o.anyActive = true
	}
	if a := s.receivers[o.key]; o.key != nil && a != nil {
		if a.starts != o.keyStarts {
			o.sameActive = true
		}
		if a.active--; a.active == 0 {
			delete(s.receivers, o.key)
		}
	}
	return o.reads()
}

// identity is the key that tells whether two calls share a receiver: its
// pointer for a receiver that refers to shared state, none for a value that
// each call copies.
func identity(receiver any) (any, bool) {
	value := reflect.ValueOf(receiver)
	switch value.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func, reflect.UnsafePointer:
		return value.Pointer(), true
	}
	return nil, false
}

// report puts a judgment on the call's span and counters.
func (s *state) report(ctx context.Context, span trace.Span, j Judgment) {
	check := metric.WithAttributes(attribute.String("unicode.check", j.Check))
	event := func(name string) {
		span.AddEvent(name, trace.WithAttributes(attribute.String("unicode.check", j.Check), attribute.String("unicode.clause", j.Clause), attribute.String("unicode.input_hash", j.InputHash)))
	}
	switch j.Verdict {
	case VerdictPassed:
		s.applicable.Add(ctx, 1, check)
	case VerdictCounterexample:
		s.applicable.Add(ctx, 1, check)
		s.counterexample.Add(ctx, 1, check)
		event("unicode.counterexample")
	case VerdictOutsideDomain:
		s.outside.Add(ctx, 1, check)
		event("unicode.outside_domain")
	case VerdictCheckError:
		s.checkErrors.Add(ctx, 1, check)
		event("unicode.check_error")
	}
}

// add keeps a judged call in the period in progress.
func (s *state) add(call JudgedCall) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, call)
}

// job is work for the background writer: a collected call to write, or a
// request to write the period in progress, done when the work is done.
type job struct {
	collected *Collected
	flush     bool
	done      chan struct{}
}

// start starts the background writer once; s.mu is held.
func (s *state) start() {
	if s.started {
		return
	}
	s.started = true
	s.queue = make(chan job, 256)
	go s.write()
	go func() {
		for range time.Tick(period) {
			s.queue <- job{flush: true}
		}
	}()
}

// write is the background writer: it writes collected calls and, every
// period or on Flush, the run record of the period in progress.
func (s *state) write() {
	for j := range s.queue {
		switch {
		case j.collected != nil:
			s.writeCollected(*j.collected)
		case j.flush:
			s.writePeriod()
		}
		if j.done != nil {
			close(j.done)
		}
	}
}

// collect queues a sampled call's input for writing, as long as its operation
// has fewer than maxCollected collected calls.
func (s *state) collect(operation Operation, receiver any, args []any) {
	collected, err := collectSafely(operation, receiver, args)
	if err != nil {
		s.writeFailures.Add(context.Background(), 1)
		return
	}
	collected.Op, collected.Commit, collected.At = operation.Op, s.build.Commit, time.Now().UTC()
	select {
	case s.queue <- job{collected: &collected}:
	default:
		s.writeFailures.Add(context.Background(), 1)
	}
}

func collectSafely(operation Operation, receiver any, args []any) (collected Collected, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("collecting the call panicked: %v", p)
		}
	}()
	return operation.Collect(receiver, args)
}

// Flush writes the run record of the period in progress and the collected
// calls waiting to be written, and starts a new period. A composition root
// calls it before the program exits; without a monitor file it does nothing.
func Flush() {
	s := current()
	s.mu.Lock()
	started := s.started
	s.mu.Unlock()
	if !started {
		return
	}
	done := make(chan struct{})
	s.queue <- job{flush: true, done: done}
	<-done
}

// writePeriod ends the period in progress and writes its run record when a
// property or flow judged a call in it.
func (s *state) writePeriod() {
	s.mu.Lock()
	calls, from, to := s.calls, s.from, time.Now()
	s.calls, s.from = nil, to
	s.mu.Unlock()
	if len(calls) == 0 {
		return
	}
	record := Summarize(calls, s.build, from, to)
	record.Diagnostics = append(record.Diagnostics, s.problems...)
	if err := writeRecord(s.dir, record); err != nil {
		s.writeFailures.Add(context.Background(), 1)
	}
}

// writeCollected writes one collected call under traffic/<op>/<hash>.json,
// at most maxCollected for one operation.
func (s *state) writeCollected(collected Collected) {
	if err := writeTraffic(s.dir, collected, s.collected); err != nil {
		s.writeFailures.Add(context.Background(), 1)
	}
}

// Conceal redacts value as Redact does, under the program's key:
// UNICODE_MONITOR_KEY, or one the process drew when it is not set. The key
// itself is never written.
func Conceal(value Value, model string, schema Schema) Value {
	return Redact(value, model, schema, current().key)
}
