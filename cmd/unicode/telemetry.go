//go:build unicode_monitor

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// The monitored build of unicode installs the OpenTelemetry SDK with the
// OTLP/HTTP exporters, configured by the standard OTEL_* environment
// variables, so that the judgments the monitor reports through the API are
// exported (docs/design.md §6.6). The default build has no SDK.
func init() {
	ctx := context.Background()
	traces, traceErr := otlptracehttp.New(ctx)
	metrics, metricErr := otlpmetrichttp.New(ctx)
	if err := errors.Join(traceErr, metricErr); err != nil {
		fmt.Fprintf(os.Stderr, "unicode: the monitored build exports no telemetry: %v\n", err)
		return
	}
	tracer := sdktrace.NewTracerProvider(sdktrace.WithBatcher(traces))
	meter := sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metrics)))
	otel.SetTracerProvider(tracer)
	otel.SetMeterProvider(meter)
	closeTelemetry = func() {
		done, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := errors.Join(tracer.Shutdown(done), meter.Shutdown(done)); err != nil {
			fmt.Fprintf(os.Stderr, "unicode: the monitored build could not export all its telemetry: %v\n", err)
		}
	}
}
