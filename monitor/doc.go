// Package monitor judges a deployed program's real calls by the properties
// of its design (docs/design.md §6.6). The program's composition root wraps
// what it hands out with Wrap and Entry; the monitor files that unicode gen
// writes, built in by the build tag unicode_monitor, register the wrappers
// and the judges they call. Without them Wrap and Entry return their argument
// as it is, so the composition root is the same source in both builds.
//
// Sampled calls are judged; the conclusions go out as OpenTelemetry events,
// counters and a duration histogram through the API alone, the program
// choosing the SDK, and every record period they are written as a runtime run
// record under UNICODE_MONITOR_DIR/runs. With UNICODE_MONITOR_TRAFFIC=on the
// inputs of sampled calls are collected under UNICODE_MONITOR_DIR/traffic.
// Values of types labelled sensitive are redacted wherever they are written.
//
// The package depends on the standard library and the OpenTelemetry API only,
// so that any program can import it.
package monitor
