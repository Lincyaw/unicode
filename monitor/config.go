package monitor

import (
	crand "crypto/rand"
	"os"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/lincyaw/unicode/internal/diag"
)

// state is the monitor of the program: its configuration from the
// environment, its instruments, the receivers' gates, and the record period
// in progress.
type state struct {
	rates    Rates
	problems []diag.Flat
	key      []byte
	dir      string
	traffic  bool
	build    Build
	gates    *gates

	tracer         trace.Tracer
	applicable     metric.Int64Counter
	counterexample metric.Int64Counter
	outside        metric.Int64Counter
	checkErrors    metric.Int64Counter
	writeFailures  metric.Int64Counter
	duration       metric.Float64Histogram

	mu      sync.Mutex
	from    time.Time
	calls   []JudgedCall
	queue   chan job
	started bool

	// collected counts each operation's collected calls; only the writer
	// touches it. filled holds an atomic.Bool per operation, set once the
	// operation has maxCollected, which Begin reads.
	collected map[string]int
	filled    sync.Map
}

var (
	monitorOnce  sync.Once
	monitorState *state
)

// period is how long a record period lasts.
const period = time.Minute

// current is the program's monitor, configured from the environment on its
// first monitored call.
func current() *state {
	monitorOnce.Do(func() { monitorState = configure() })
	return monitorState
}

func configure() *state {
	s := &state{rates: Rates{Fallback: defaultRate, Ops: map[string]float64{}}, dir: ".unicode", gates: newGates(), collected: map[string]int{}, from: time.Now()}
	if text, ok := os.LookupEnv("UNICODE_MONITOR_RATE"); ok {
		rates, err := ParseRates(text)
		if err != nil {
			s.problems = append(s.problems, diag.Flat{Code: "monitor.bad-rates", Subject: "UNICODE_MONITOR_RATE", Path: "design/system.uc", Line: 1, Column: 1,
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
		// crypto/rand.Read never returns an error: it panics when the system has no randomness.
		_, _ = crand.Read(s.key)
	}
	s.build = buildOf(debug.ReadBuildInfo())
	s.tracer = otel.Tracer(instrumentation)
	s.instruments(otel.Meter(instrumentation))
	return s
}

// instrumentation is the name the monitor's spans and metrics carry.
const instrumentation = "github.com/lincyaw/unicode/monitor"

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

// full tells whether op has maxCollected collected calls.
func (s *state) full(op string) bool {
	flag, ok := s.filled.Load(op)
	return ok && flag.(*atomic.Bool).Load()
}

// fill records that op has maxCollected collected calls.
func (s *state) fill(op string) {
	flag, _ := s.filled.LoadOrStore(op, &atomic.Bool{})
	flag.(*atomic.Bool).Store(true)
}

// Flush writes the run record of the period in progress and the collected
// calls waiting to be written, and starts a new period. A composition root
// calls it before the program exits; when no monitor file registered
// anything, as in the default build, it does nothing at all.
func Flush() {
	if !enabled.Load() {
		return
	}
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

// Conceal redacts value as Redact does, under the program's key:
// UNICODE_MONITOR_KEY, or one the process drew when it is not set. The key
// itself is never written.
func Conceal(value Value, model string, schema Schema) Value {
	return Redact(value, model, schema, current().key)
}
