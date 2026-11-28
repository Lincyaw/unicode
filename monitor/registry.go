package monitor

import (
	"reflect"
	"sync"
	"sync/atomic"
)

// Reads are the values a judgment may read: the receiver, when no other
// monitored call on the same receiver overlaps the reading, and the state
// that the frame and named observations read, under the same rule for a call
// with a receiver and always for a call without one (docs/design.md §6.6).
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

// enabled is set once a monitor file registers anything: without one, as in
// the default build, nothing of the monitor runs or is allocated.
var enabled atomic.Bool

// RegisterWrapper registers the interface wrapper of the Go interface iface:
// wrap returns a value of iface that calls next, the real implementation.
// Monitor files call it from init.
func RegisterWrapper(iface reflect.Type, wrap func(next any) any) {
	registry.Lock()
	defer registry.Unlock()
	enabled.Store(true)
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
	enabled.Store(true)
	if registry.entries == nil {
		registry.entries = map[uintptr]any{}
	}
	registry.entries[reflect.ValueOf(fn).Pointer()] = wrapped
}

// RegisterMethod registers the operation that the method name of the Go type
// receiver realizes. Monitor files call it from init.
func RegisterMethod(receiver reflect.Type, name string, op Operation) {
	register(target{receiver, name}, op)
}

// RegisterFunction registers the container operation that a Go function
// realizes, by the operation's identity.
func RegisterFunction(op Operation) { register(target{nil, op.Op}, op) }

func register(at target, op Operation) {
	registry.Lock()
	defer registry.Unlock()
	enabled.Store(true)
	if registry.operations == nil {
		registry.operations = map[target]Operation{}
	}
	registry.operations[at] = op
}

// registered is the operation a call of op on receiver finds.
func registered(op string, receiver any) (Operation, bool) {
	at := target{nil, op}
	if receiver != nil {
		at = target{reflect.TypeOf(receiver), lastSegment(op)}
	}
	registry.RLock()
	defer registry.RUnlock()
	operation, ok := registry.operations[at]
	return operation, ok
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

// lastSegment is the name an operation's identity ends with.
func lastSegment(id string) string {
	for i := len(id) - 1; i >= 0; i-- {
		if id[i] == '.' {
			return id[i+1:]
		}
	}
	return id
}
