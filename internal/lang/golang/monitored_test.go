package golang

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"golang.org/x/mod/modfile"
	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/evidence"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
)

// MonitoredRun is a copy of the monitored module after its monitored
// program ran there: the copy's directory, the runtime run record the
// program wrote (an empty report when nothing was judged), what each call of
// demo.score through its entry wrapper and directly gave, every word of the
// files the program wrote under .unicode, whether monitor.Wrap wrapped
// demo.Counter and left an unregistered interface and function alone,
// whether the context the wrapper passed on carried the call's span as a
// child of the caller's, and whether the race detector reported a race.
//
//uc:realizes lang.golang.MonitoredRun
type MonitoredRun struct {
	Root              string
	Record            lang.RunReport
	Wrapped           []string
	Direct            []string
	Words             []string
	Wraps             bool
	KeepsUnregistered bool
	ContextLinked     bool
	Raced             bool
}

// monitoredFiles is the monitored module, but for its go.mod and go.sum.
// demo.score breaks result >= level on 3 and panics on 4, and demo.ratio
// panics on -3. demo.unlock takes a sensitive Token and a Badge that extends
// it, and by the token's prefix panics, returns an undeclared error or
// returns the declared error Denied, each of them carrying the token's text.
// demo.Tally implements demo.Counter, whose methods take a context first,
// under one lock, and keeps the context its last Add received. demo.observed
// observes a Tally through Tallied, which has no effect, and Stamp, which
// reads the process.
//
// The program takes levels, then "--", then secrets. It calls score through
// monitor.Entry and directly on each level, unlock through monitor.Entry on
// each secret with each prefix, and a Tally through monitor.Wrap: once with
// a caller's span, then from eight goroutines at once, then once more alone.
// It writes the spans it exported to spans.json in UNICODE_MONITOR_DIR, and
// what it saw as JSON on its standard output.
var monitoredFiles = map[string]string{
	"src/demo.go": `package demo

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
)

//uc:realizes demo.score
func Score(level int) int {
	switch level {
	case 3:
		return 2
	case 4:
		panic("score panics on four")
	}
	return level + 1
}

//uc:realizes demo.ratio
func Ratio(level int) int {
	if level == -3 {
		panic("ratio panics on minus three")
	}
	return level
}

//uc:realizes demo.Token
type Token struct {
	Text string
}

//uc:realizes demo.Badge
type Badge struct {
	Token
	Rank int
}

//uc:realizes demo.Denied
type Denied struct {
	Token Token
}

func (value Denied) Error() string { return "denied " + value.Token.Text }

//uc:realizes demo.unlock
func Unlock(token Token, badge Badge) (int, error) {
	switch {
	case strings.HasPrefix(token.Text, "panic-"):
		panic("unlock panics on " + token.Text + " and " + badge.Text)
	case strings.HasPrefix(token.Text, "undeclared-"):
		return 0, errors.New("unlock fails on " + token.Text)
	case strings.HasPrefix(token.Text, "denied-"):
		return 0, Denied{Token: token}
	}
	return len(token.Text), nil
}

//uc:realizes demo.Counter
type Counter interface {
	Add(ctx context.Context, n int) int
	Total(ctx context.Context) int
}

//uc:realizes demo.Tally
type Tally struct {
	Count int
}

var (
	lock sync.Mutex
	seen context.Context
)

func (t *Tally) Add(ctx context.Context, n int) int {
	lock.Lock()
	defer lock.Unlock()
	seen = ctx
	t.Count += n
	return t.Count
}

func (t *Tally) Total(ctx context.Context) int {
	lock.Lock()
	defer lock.Unlock()
	return t.Count
}

// Seen is the context the last Add received.
func Seen() context.Context {
	lock.Lock()
	defer lock.Unlock()
	return seen
}

//uc:observes demo.observed.tallied
func Tallied(tally *Tally) int { return tally.Count }

//uc:observes demo.observed.stamp
func Stamp(tally *Tally) int { return os.Getpid() }
`,
	"src/cmd/demo/main.go": `package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"sync"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"example.test/monitored/src"
	"github.com/lincyaw/unicode/monitor"
)

func call(score func(int) int, level int) (out string) {
	defer func() {
		if p := recover(); p != nil {
			out = fmt.Sprint("panic: ", p)
		}
	}()
	return strconv.Itoa(score(level))
}

func attempt(unlock func(demo.Token, demo.Badge) (int, error), text string) {
	defer func() { recover() }()
	unlock(demo.Token{Text: text}, demo.Badge{Token: demo.Token{Text: text}, Rank: len(text)})
}

// totaller is an interface no monitor file registers.
type totaller interface{ Total(context.Context) int }

func main() {
	exporter := tracetest.NewInMemoryExporter()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter)))
	levels, secrets := os.Args[1:], []string(nil)
	if i := slices.Index(levels, "--"); i >= 0 {
		levels, secrets = levels[:i], levels[i+1:]
	}
	out := struct {
		Wrapped, Direct                        []string
		Wraps, KeepsUnregistered, ContextLinked bool
	}{Wrapped: []string{}, Direct: []string{}}
	score := monitor.Entry(demo.Score)
	for _, arg := range levels {
		level, _ := strconv.Atoi(arg)
		out.Wrapped = append(out.Wrapped, call(score, level))
		out.Direct = append(out.Direct, call(demo.Score, level))
	}
	unlock := monitor.Entry(demo.Unlock)
	for _, secret := range secrets {
		for _, prefix := range []string{"open-", "panic-", "undeclared-", "denied-"} {
			attempt(unlock, prefix+secret)
		}
	}
	tally := &demo.Tally{}
	counter := monitor.Wrap[demo.Counter](tally)
	_, bare := counter.(*demo.Tally)
	out.Wraps = !bare
	out.KeepsUnregistered = monitor.Wrap[totaller](tally) == totaller(tally) &&
		reflect.ValueOf(monitor.Entry(demo.Ratio)).Pointer() == reflect.ValueOf(demo.Ratio).Pointer()
	caller, span := otel.Tracer("demo").Start(context.Background(), "caller")
	counter.Add(caller, 1)
	inner := trace.SpanContextFromContext(demo.Seen())
	span.End()
	var group sync.WaitGroup
	for range 8 {
		group.Go(func() {
			for range 16 {
				counter.Add(context.Background(), 1)
				counter.Total(context.Background())
			}
		})
	}
	group.Wait()
	counter.Total(context.Background())
	monitor.Flush()
	spans := exporter.GetSpans()
	for _, s := range spans {
		if inner.IsValid() && s.SpanContext.SpanID() == inner.SpanID() && s.Parent.SpanID() == span.SpanContext().SpanID() && s.SpanContext.TraceID() == span.SpanContext().TraceID() {
			out.ContextLinked = true
		}
	}
	data, err := json.Marshal(spans)
	if err == nil {
		err = os.WriteFile(filepath.Join(os.Getenv("UNICODE_MONITOR_DIR"), "spans.json"), data, 0o644)
	}
	if err == nil {
		err = json.NewEncoder(os.Stdout).Encode(out)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
`,
}

// monitoredTemp is the process's temporary directory, taken before any case
// points TMPDIR at a directory of its own, which the case's end removes.
var monitoredTemp = os.TempDir()

// monitored is the monitored module, written, generated and built once per
// process: the copies of the cases share its graph and its program.
var monitored struct {
	once    sync.Once
	dir     string
	program string
	graph   *graph.Graph
	err     error
}

// monitoredModule writes the monitored module into a new directory with a
// go.mod that replaces this repository's module by its directory, generates
// its drivers and its monitor file, and builds its program with the build
// constraint unicode_monitor and the race detector.
func monitoredModule() (string, string, *graph.Graph, error) {
	monitored.once.Do(func() {
		monitored.err = func() error {
			// A derivation runs in a directory of its own, so the repository is
			// found from this file's path rather than the working directory.
			_, file, _, ok := runtime.Caller(0)
			if !ok {
				return errors.New("the test file's path is unknown")
			}
			repository := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
			sum, err := os.ReadFile(filepath.Join(repository, "go.sum"))
			if err != nil {
				return err
			}
			dir, err := os.MkdirTemp(monitoredTemp, "monitored-module-")
			if err != nil {
				return err
			}
			// The module requires every module this repository requires, at the
			// same versions, so that its build list is complete without the network.
			modData, err := os.ReadFile(filepath.Join(repository, "go.mod"))
			if err != nil {
				return err
			}
			parsed, err := modfile.Parse("go.mod", modData, nil)
			if err != nil {
				return err
			}
			mod := "module example.test/monitored\n\ngo 1.26.2\n\nrequire (\n\tgithub.com/lincyaw/unicode v0.0.0\n"
			for _, required := range parsed.Require {
				mod += "\t" + required.Mod.Path + " " + required.Mod.Version + "\n"
			}
			mod += ")\n\nreplace github.com/lincyaw/unicode => " + repository + "\n"
			files := map[string]string{"go.mod": mod, "go.sum": string(sum)}
			for name, content := range monitoredFiles {
				files[name] = content
			}
			if err := writeFiles(dir, files); err != nil {
				return err
			}
			g, err := monitoredGraph(dir)
			if err != nil {
				return err
			}
			generated := NewBackend().Generate(lang.GenPlan{Root: dir, Roots: []string{"src"}, Graph: &g})
			if len(generated.Diagnostics) > 0 {
				return fmt.Errorf("the monitored module does not generate: %+v", generated.Diagnostics)
			}
			edits := map[string]string{}
			for _, edit := range generated.Edits {
				edits[edit.Path] = string(edit.Content)
			}
			if err := writeFiles(dir, edits); err != nil {
				return err
			}
			program := filepath.Join(dir, "demo-program")
			build := exec.Command("go", "build", "-race", "-buildvcs=false", "-tags", monitorBuildTag, "-o", program, "./src/cmd/demo")
			build.Dir = dir
			if output, err := build.CombinedOutput(); err != nil {
				return fmt.Errorf("the monitored program does not build: %w: %s", err, output)
			}
			monitored.dir, monitored.program, monitored.graph = dir, program, &g
			return nil
		}()
	})
	return monitored.dir, monitored.program, monitored.graph, monitored.err
}

func writeFiles(root string, files map[string]string) error {
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// monitoredGraph is the monitored module's design, linked with its code:
// the pure operations demo.score and demo.ratio, and demo.bounded judged at
// runtime on score; the pure operation demo.unlock of a sensitive Token and a
// Badge that extends it, raising Denied, and demo.unlocks judged on it; the
// interface demo.Counter and its implementation demo.Tally, and demo.totals
// judged on Tally's total, and demo.observed, judged on it through two named
// observations.
func monitoredGraph(root string) (graph.Graph, error) {
	g := graph.NewGraph()
	var problems []error
	at := diag.Position{Path: "design/demo.uc", Line: 1, Column: 1}
	add := func(id string, kind graph.NodeKind, form, parent string, attrs map[string][]string) {
		problems = append(problems, g.AddNode(graph.Node{Id: id, Kind: kind, Form: form, Layer: graph.LayerDesign, Source: graph.SourceDeclared, Origin: at, Attrs: attrs}))
		if parent != "" {
			problems = append(problems, g.AddEdge(graph.Edge{Kind: graph.EdgeKindContains, From: parent, To: id, Source: graph.SourceDeclared}))
		}
	}
	edge := func(kind graph.EdgeKind, from, to string, clauses ...string) {
		var attrs map[string][]string
		if len(clauses) > 0 {
			attrs = map[string][]string{"clauses": clauses}
		}
		problems = append(problems, g.AddEdge(graph.Edge{Kind: kind, From: from, To: to, Source: graph.SourceDeclared, Attrs: attrs}))
	}
	add("demo", graph.NodeKindContainer, "component", "", map[string][]string{"paths": {"src/**"}})
	add("demo.score", graph.NodeKindOp, "op", "demo", map[string][]string{"signature": {"(level: int) -> int"}, "pure": {"true"}})
	add("demo.ratio", graph.NodeKindOp, "op", "demo", map[string][]string{"signature": {"(level: int) -> int"}, "pure": {"true"}})
	add("demo.bounded", graph.NodeKindProperty, "postcondition", "demo", map[string][]string{
		"symbols": {"level: int"}, "limits": {"level in -5..5"}, "requires": {"level != 0"}, "call": {"score(level)"},
		"ensure": {"result >= level", "ratio(level) == level"},
	})
	edge(graph.EdgeKindRefers, "demo.bounded", "demo.score", "call#1")
	edge(graph.EdgeKindRefers, "demo.bounded", "demo.ratio", "ensure#2")
	add("demo.Token", graph.NodeKindType, "record", "demo", map[string][]string{"fields": {"text: string"}, "labels": {"sensitive"}})
	add("demo.Badge", graph.NodeKindType, "record", "demo", map[string][]string{"fields": {"rank: int"}})
	edge(graph.EdgeKindExtends, "demo.Badge", "demo.Token")
	add("demo.Denied", graph.NodeKindType, "error", "demo", map[string][]string{"fields": {"token: demo.Token"}})
	add("demo.unlock", graph.NodeKindOp, "op", "demo", map[string][]string{"signature": {"(token: demo.Token, badge: demo.Badge) -> int raises demo.Denied"}, "pure": {"true"}})
	add("demo.unlocks", graph.NodeKindProperty, "postcondition", "demo", map[string][]string{
		"symbols": {"token: demo.Token", "badge: demo.Badge"}, "call": {"unlock(token, badge)"},
		"ensure": {"not raised(Denied)", "raised(Denied) or result == size(token.text)"},
	})
	edge(graph.EdgeKindRefers, "demo.unlocks", "demo.unlock", "call#1")
	add("demo.Counter", graph.NodeKindType, "interface", "demo", nil)
	add("demo.Counter.add", graph.NodeKindOp, "op", "demo.Counter", map[string][]string{"signature": {"(n: int) -> int"}})
	add("demo.Counter.total", graph.NodeKindOp, "op", "demo.Counter", map[string][]string{"signature": {"() -> int"}, "pure": {"true"}})
	add("demo.Tally", graph.NodeKindType, "object", "demo", map[string][]string{"fields": {"count: int"}})
	add("demo.Tally.add", graph.NodeKindOp, "op", "demo.Tally", map[string][]string{"signature": {"(n: int) -> int"}})
	add("demo.Tally.total", graph.NodeKindOp, "op", "demo.Tally", map[string][]string{"signature": {"() -> int"}, "pure": {"true"}})
	edge(graph.EdgeKindImplements, "demo.Tally", "demo.Counter")
	add("demo.totals", graph.NodeKindProperty, "postcondition", "demo", map[string][]string{
		"symbols": {"tally: demo.Tally"}, "call": {"tally.total()"}, "ensure": {"result == tally.count"},
	})
	edge(graph.EdgeKindRefers, "demo.totals", "demo.Tally.total", "call#1")
	observation := func(id, parent string) {
		add(id, graph.NodeKindObservation, "snapshot", parent, map[string][]string{"type": {"int"}, "inputs": {"tally"}})
	}
	add("demo.observed", graph.NodeKindProperty, "postcondition", "demo", map[string][]string{
		"symbols": {"tally: demo.Tally"}, "call": {"tally.total()"}, "observe": {"tallied: int from tally", "stamp: int from tally"},
		"ensure": {"result == tallied", "stamp > 0"},
	})
	observation("demo.observed.tallied", "demo.observed")
	observation("demo.observed.stamp", "demo.observed")
	edge(graph.EdgeKindRefers, "demo.observed", "demo.Tally.total", "call#1")
	if err := errors.Join(problems...); err != nil {
		return g, err
	}
	extracted, err := NewBackend().ExtractForGeneration(root, []string{"src"})
	if err != nil {
		return g, err
	}
	if err := g.AddFragment(extracted.Fragment); err != nil {
		return g, err
	}
	if ds := g.Derive(); len(ds) > 0 {
		return g, fmt.Errorf("the monitored module does not link: %+v", ds)
	}
	return g, nil
}

// genSecrets draws one to three secrets, each zq and eight lowercase
// letters, so that no word of a hash or a name the monitor writes is one.
//
//uc:generates lang.golang.monitored-judgments-match-replay.secrets
//uc:generates lang.golang.monitoring-keeps-calls.secrets
//uc:generates lang.golang.monitoring-hides-sensitive-values.secrets
//uc:generates lang.golang.monitoring-wraps-and-links-contexts.secrets
//uc:generates lang.golang.monitoring-reads-receivers-without-races.secrets
//uc:generates lang.golang.monitoring-reads-cheap-observations.secrets
func genSecrets() *rapid.Generator[[]string] {
	return rapid.SliceOfN(rapid.StringMatching("zq[a-z]{8}"), 1, 3)
}

// raceExit is the exit status the monitored program ends with when the
// race detector reported a race.
const raceExit = 66

// deriveMonitoredRun copies the monitored module into a new directory and
// runs its monitored program there with the sampling rate 1 and collection
// on, on levels and secrets, reading back the runtime run record it wrote and
// the words of every file under .unicode.
//
//uc:generates lang.golang.monitored-judgments-match-replay.run
//uc:generates lang.golang.monitoring-keeps-calls.run
//uc:generates lang.golang.monitoring-hides-sensitive-values.run
//uc:generates lang.golang.monitoring-wraps-and-links-contexts.run
//uc:generates lang.golang.monitoring-reads-receivers-without-races.run
//uc:generates lang.golang.monitoring-reads-cheap-observations.run
func deriveMonitoredRun(levels []int, secrets []string) (MonitoredRun, error) {
	module, program, _, err := monitoredModule()
	if err != nil {
		return MonitoredRun{}, err
	}
	root, err := os.MkdirTemp("", "monitored-run-")
	if err != nil {
		return MonitoredRun{}, err
	}
	if err := os.CopyFS(root, os.DirFS(module)); err != nil {
		return MonitoredRun{}, err
	}
	var args []string
	for _, level := range levels {
		args = append(args, strconv.Itoa(level))
	}
	args = append(append(args, "--"), secrets...)
	cmd := exec.Command(program, args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "UNICODE_MONITOR_RATE=1", "UNICODE_MONITOR_TRAFFIC=on", "UNICODE_MONITOR_KEY=monitored-run",
		"UNICODE_MONITOR_DIR="+filepath.Join(root, ".unicode"), "GORACE=exitcode="+strconv.Itoa(raceExit))
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	out := MonitoredRun{Root: root, Record: lang.RunReport{Results: []lang.RunResult{}, Diagnostics: []diag.Diagnostic{}}}
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != raceExit {
			return out, fmt.Errorf("the monitored program failed: %w: %s", err, stderr.Bytes())
		}
		out.Raced = true
	}
	var seen struct {
		Wrapped, Direct                         []string
		Wraps, KeepsUnregistered, ContextLinked bool
	}
	if err := json.Unmarshal(stdout.Bytes(), &seen); err != nil {
		return out, err
	}
	out.Wrapped, out.Direct, out.Wraps, out.KeepsUnregistered, out.ContextLinked = seen.Wrapped, seen.Direct, seen.Wraps, seen.KeepsUnregistered, seen.ContextLinked
	if out.Words, err = monitoredWords(filepath.Join(root, ".unicode")); err != nil {
		return out, err
	}
	loaded := evidence.Read(root)
	if len(loaded.Diagnostics) > 0 {
		return out, fmt.Errorf("the runtime record cannot be read: %+v", loaded.Diagnostics)
	}
	for _, report := range loaded.Reports {
		if report.Venue == lang.VenueRuntime {
			out.Record = report
		}
	}
	return out, nil
}

// monitoredWords are the words of every file under dir, sorted and once
// each: the runs of letters and digits.
func monitoredWords(dir string) ([]string, error) {
	var words []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		words = append(words, strings.FieldsFunc(string(data), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })...)
		return nil
	})
	slices.Sort(words)
	return slices.Compact(words), err
}

// deriveMonitoredPlan is the plan that replays demo.bounded on the copy run
// holds, its inputs those the collected calls give it (docs/language.md E29).
//
//uc:generates lang.golang.monitored-judgments-match-replay.plan
func deriveMonitoredPlan(run MonitoredRun) (lang.RunPlan, error) {
	_, _, g, err := monitoredModule()
	if err != nil {
		return lang.RunPlan{}, err
	}
	traffic := evidence.ReadTraffic(run.Root, g)
	if len(traffic.Diagnostics) > 0 {
		return lang.RunPlan{}, fmt.Errorf("the collected calls cannot be read: %+v", traffic.Diagnostics)
	}
	return lang.RunPlan{Root: run.Root, Roots: []string{"src"}, Graph: g, Ids: []string{"demo.bounded"}, Traffic: evidence.TrafficInputs(g, traffic.Collected)}, nil
}
