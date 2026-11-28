package monitor

import (
	"reflect"
	"sync"
)

// Overlap is decided per receiver (docs/design.md §6.6): only monitored calls
// on the same receiver object overlap. A judgment reads the receiver only
// while no monitored real call on it is in progress, and a monitored call on
// that receiver starts its real call only once the reading is done, so the
// reads never race the writes of monitored calls. The wait has no bound:
// while a judgment reads, the only code of the program it runs on a live
// object is a codec's encoder, so no monitored call it makes waits on it.
// Calls without a receiver, such as entries, have nothing to wait for.

// gate is the monitored activity on one receiver.
type gate struct {
	calls   int    // monitored real calls in progress
	reading int    // judgments reading the receiver now
	starts  uint64 // monitored real calls started so far
}

// gates are the gates of the receivers with monitored activity.
type gates struct {
	mu    sync.Mutex
	ready *sync.Cond
	byKey map[any]*gate
}

func newGates() *gates {
	g := &gates{byKey: map[any]*gate{}}
	g.ready = sync.NewCond(&g.mu)
	return g
}

// window is one monitored call's view of its receiver's gate; a call
// without a receiver of its own identity has none. joined records that
// another monitored real call on the receiver was in progress when this one
// started.
type window struct {
	gates  *gates
	key    any
	start  uint64
	joined bool
	reader bool
}

// open gives the window of a call on receiver.
func (g *gates) open(receiver any) *window {
	key, ok := identity(receiver)
	if !ok {
		return &window{}
	}
	return &window{gates: g, key: key}
}

// read reports whether the call may read its receiver now, before its real
// call: no monitored real call on the receiver is in progress. While it
// reads, monitored calls on the receiver wait to start; done ends the
// reading.
func (w *window) read() bool {
	if w.gates == nil {
		return false
	}
	w.gates.mu.Lock()
	defer w.gates.mu.Unlock()
	g := w.gates.of(w.key)
	if g.calls > 0 {
		return false
	}
	g.reading++
	w.reader = true
	return true
}

// done ends the call's reading of its receiver.
func (w *window) done() {
	if !w.reader {
		return
	}
	w.gates.mu.Lock()
	defer w.gates.mu.Unlock()
	w.reader = false
	g := w.gates.of(w.key)
	g.reading--
	w.gates.release(w.key, g)
	w.gates.ready.Broadcast()
}

// begin marks the call's real call as started, once no judgment reads the
// receiver.
func (w *window) begin() {
	if w.gates == nil {
		return
	}
	w.gates.mu.Lock()
	defer w.gates.mu.Unlock()
	for w.gates.of(w.key).reading > 0 {
		w.gates.ready.Wait()
	}
	g := w.gates.of(w.key)
	w.joined = g.calls > 0
	g.calls++
	g.starts++
	w.start = g.starts
}

// end marks the call's real call as returned, and reports whether the call
// may read its receiver after it: no other monitored real call on the
// receiver overlapped it, that is, none was in progress when it started and
// none started since. When it may, the reading lasts until done.
func (w *window) end() bool {
	if w.gates == nil {
		return false
	}
	w.gates.mu.Lock()
	defer w.gates.mu.Unlock()
	g := w.gates.of(w.key)
	g.calls--
	readable := !w.joined && g.starts == w.start
	if readable {
		g.reading++
		w.reader = true
	}
	w.gates.release(w.key, g)
	return readable
}

// of is the gate of key, made when missing; g.mu is held.
func (g *gates) of(key any) *gate {
	if g.byKey[key] == nil {
		g.byKey[key] = &gate{}
	}
	return g.byKey[key]
}

// release forgets an idle gate; g.mu is held.
func (g *gates) release(key any, idle *gate) {
	if idle.calls == 0 && idle.reading == 0 {
		delete(g.byKey, key)
	}
}

// identity is the key that tells whether two calls share a receiver: its
// pointer for a receiver that refers to shared state, none for a value that
// each call copies.
func identity(receiver any) (any, bool) {
	value := reflect.ValueOf(receiver)
	switch value.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Chan, reflect.UnsafePointer:
		if value.IsNil() {
			return nil, false
		}
		return value.Pointer(), true
	}
	return nil, false
}
