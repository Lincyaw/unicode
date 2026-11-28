package monitor

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
)

// instruments makes the monitor's counters and histogram (docs/design.md
// §6.6). The API refuses only an invalid name or option; then the instrument
// is a no-op and the rest of the reporting goes on.
func (s *state) instruments(meter metric.Meter) {
	none := noop.NewMeterProvider().Meter(instrumentation)
	counter := func(name, description string) metric.Int64Counter {
		c, err := meter.Int64Counter(name, metric.WithDescription(description))
		if err != nil {
			// The no-op meter accepts every instrument.
			c, _ = none.Int64Counter(name)
		}
		return c
	}
	s.applicable = counter("unicode.monitor.applicable", "monitored calls applicable to a property or flow")
	s.counterexample = counter("unicode.monitor.counterexamples", "monitored calls that are counterexamples")
	s.outside = counter("unicode.monitor.outside_domain", "monitored calls outside a property's domain")
	s.checkErrors = counter("unicode.monitor.check_errors", "judgments that could not be made")
	s.writeFailures = counter("unicode.monitor.write_failures", "run records and collected calls that could not be written")
	histogram, err := meter.Float64Histogram("unicode.monitor.duration", metric.WithUnit("s"), metric.WithDescription("durations of monitored calls"))
	if err != nil {
		// The no-op meter accepts every instrument.
		histogram, _ = none.Float64Histogram("unicode.monitor.duration")
	}
	s.duration = histogram
}

// report puts a judgment on the call's span and counters: identities,
// clauses and input hashes only, never a value.
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

// observe records a monitored call's duration by its operation.
func (s *state) observe(ctx context.Context, op string, duration time.Duration) {
	s.duration.Record(ctx, duration.Seconds(), metric.WithAttributes(attribute.String("unicode.op", op)))
}
