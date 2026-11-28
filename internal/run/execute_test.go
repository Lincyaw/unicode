package run

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
	"github.com/lincyaw/unicode/internal/repo"
)

// ExecuteSample supplies configured checks, selected targets, their
// collected inputs, runner reports, the checks selected in each language,
// and the results execute should give.
//
//uc:realizes run.ExecuteSample
type ExecuteSample struct {
	Graph    *graph.Graph
	Ids      []string
	Traffic  map[string][]lang.Input
	Reports  map[string]lang.RunReport
	Selected map[string][]string
	Expected []lang.RunResult
}

// RunReceived is one run input a recording backend was given.
//
//uc:realizes run.RunReceived
type RunReceived struct {
	Root     string
	Roots    []string
	Snapshot []byte
	Ids      []string
	Explore  time.Duration
	Record   bool
	Traffic  map[string][]lang.Input
}

//uc:generates run.execute-dispatches-selected-checks.sample
//uc:generates run.execute-merges-results.sample
//uc:generates run.execute-rejects-unknown-checks.sample
//uc:generates run.execute-saves-its-report.sample
//uc:generates run.execute-rejects-duplicate-backends.sample
//uc:generates run.execute-rejects-backends-without-runner.sample
//uc:generates run.execute-rejects-omitted-results.sample
//uc:generates run.execute-writes-report-json.sample
func genExecuteSample() *rapid.Generator[ExecuteSample] {
	return rapid.Custom(func(t *rapid.T) ExecuteSample {
		g := graph.NewGraph()
		reports := map[string]lang.RunReport{}
		traffic := map[string][]lang.Input{}
		var roots, targets []string
		add := func(n graph.Node) {
			if err := g.AddNode(n); err != nil {
				t.Fatal(err)
			}
		}
		for _, name := range []string{"go", "python"} {
			for range rapid.IntRange(1, 3).Draw(t, name+"roots") {
				roots = append(roots, name+": "+rapid.StringMatching(`[a-z]{1,7}`).Draw(t, "root"))
			}
			fileID := "file:" + name + "/checks_test.go"
			add(graph.Node{Id: fileID, Kind: graph.NodeKindContainer, Form: "file", Layer: graph.LayerCode, Attrs: map[string][]string{"language": {name}, "path": {name + "/checks_test.go"}}})
			report := lang.RunReport{}
			for i := range rapid.IntRange(1, 3).Draw(t, name+"checks") {
				id := fmt.Sprintf("%s.check-%d", name, i)
				prefix := name
				if name == "python" {
					prefix = "py"
				}
				code := prefix + ":example.com/checks.check" + fmt.Sprint(i)
				add(graph.Node{Id: id, Kind: graph.NodeKindProperty, Form: "invariant", Layer: graph.LayerDesign, Origin: diag.Position{Path: "design/" + name + ".uc", Line: i + 1, Column: 1}})
				add(graph.Node{Id: code, Kind: graph.NodeKindOp, Form: "func", Layer: graph.LayerCode})
				if err := g.AddEdge(graph.Edge{Kind: graph.EdgeKindChecks, From: code, To: id}); err != nil {
					t.Fatal(err)
				}
				if err := g.AddEdge(graph.Edge{Kind: graph.EdgeKindContains, From: fileID, To: code}); err != nil {
					t.Fatal(err)
				}
				result := lang.RunResult{Id: id, Passed: rapid.Bool().Draw(t, "passed"), Accepted: rapid.IntRange(1, 50).Draw(t, "accepted"), Rejected: rapid.IntRange(0, 10).Draw(t, "rejected")}
				if !result.Passed {
					result.Diagnostics = []diag.Diagnostic{diagnostic("test.failed", id+".uc", "the check failed", "correct the implementation")}
				}
				report.Results = append(report.Results, result)
				if rapid.Bool().Draw(t, "collected") {
					for range rapid.IntRange(1, 2).Draw(t, "inputs") {
						traffic[id] = append(traffic[id], lang.Input{Values: map[string]lang.Value{"n": {Kind: "int", Text: fmt.Sprint(rapid.IntRange(-9, 9).Draw(t, "n"))}}})
					}
				}
				if rapid.Bool().Draw(t, "selected") {
					targets = append(targets, id)
				}
			}
			reports[name] = report
		}
		add(graph.Node{Id: "system", Kind: graph.NodeKindContainer, Form: "system", Layer: graph.LayerDesign, Origin: diag.Position{Path: "design/system.uc", Line: 1, Column: 1}, Attrs: map[string][]string{"roots": roots}})
		sample := ExecuteSample{Graph: &g, Ids: targets, Traffic: traffic, Reports: reports, Selected: map[string][]string{}, Expected: []lang.RunResult{}}
		for _, name := range []string{"go", "python"} {
			sample.Selected[name] = []string{}
			for _, r := range reports[name].Results {
				if len(targets) == 0 || slices.Contains(targets, r.Id) {
					sample.Selected[name] = append(sample.Selected[name], r.Id)
					sample.Expected = append(sample.Expected, r)
				}
			}
			slices.Sort(sample.Selected[name])
		}
		slices.SortFunc(sample.Expected, func(a, b lang.RunResult) int { return strings.Compare(a.Id, b.Id) })
		if rapid.IntRange(0, 3).Draw(t, "unknown") == 0 {
			sample.Ids = append(sample.Ids, "missing.check")
		}
		return sample
	})
}

type executionBackend struct {
	name   string
	report lang.RunReport
	plans  []lang.RunPlan
	omit   bool
}

func (b *executionBackend) Language() string { return b.name }
func (*executionBackend) Extract(string, []string) (lang.Extracted, error) {
	panic("execution must not extract code")
}
func (*executionBackend) MapName(lang.NameRole, string) string {
	panic("execution orchestration must not map names")
}
func (b *executionBackend) Run(p lang.RunPlan) lang.RunReport {
	b.plans = append(b.plans, p)
	out := lang.RunReport{Diagnostics: b.report.Diagnostics}
	if !b.omit {
		for _, r := range slices.Backward(b.report.Results) {
			if slices.Contains(p.Ids, r.Id) {
				out.Results = append(out.Results, r)
			}
		}
	}
	return out
}

type readOnlyBackend struct{ name string }

func (b readOnlyBackend) Language() string { return b.name }
func (readOnlyBackend) Extract(string, []string) (lang.Extracted, error) {
	panic("execution must not extract code")
}
func (readOnlyBackend) MapName(lang.NameRole, string) string {
	panic("execution orchestration must not map names")
}

//uc:generates run.execute-dispatches-selected-checks.explore
//uc:generates run.execute-merges-results.explore
//uc:generates run.execute-rejects-unknown-checks.explore
//uc:generates run.execute-saves-its-report.explore
//uc:generates run.execute-rejects-duplicate-backends.explore
//uc:generates run.execute-rejects-backends-without-runner.explore
//uc:generates run.execute-rejects-omitted-results.explore
func genExplore() *rapid.Generator[time.Duration] {
	return rapid.Custom(func(t *rapid.T) time.Duration {
		return time.Duration(rapid.IntRange(0, 3).Draw(t, "seconds")) * time.Second
	})
}

//uc:generates run.execute-dispatches-selected-checks.dir
//uc:generates run.execute-merges-results.dir
//uc:generates run.execute-rejects-unknown-checks.dir
//uc:generates run.execute-saves-its-report.dir
//uc:generates run.execute-rejects-duplicate-backends.dir
//uc:generates run.execute-rejects-backends-without-runner.dir
//uc:generates run.execute-rejects-omitted-results.dir
func deriveEmptyDir() (string, error) { return repo.Materialize(repo.Tree{}) }

//uc:generates run.execute-dispatches-selected-checks.record
//uc:generates run.execute-merges-results.record
//uc:generates run.execute-rejects-unknown-checks.record
//uc:generates run.execute-saves-its-report.record
//uc:generates run.execute-rejects-duplicate-backends.record
//uc:generates run.execute-rejects-backends-without-runner.record
//uc:generates run.execute-rejects-omitted-results.record
func genRecord() *rapid.Generator[bool] { return rapid.Bool() }

// derivePlan runs the sample's targets in dir with explore and record and
// the sample's collected inputs.
//
//uc:generates run.execute-dispatches-selected-checks.plan
//uc:generates run.execute-merges-results.plan
//uc:generates run.execute-rejects-unknown-checks.plan
//uc:generates run.execute-saves-its-report.plan
//uc:generates run.execute-rejects-duplicate-backends.plan
//uc:generates run.execute-rejects-backends-without-runner.plan
//uc:generates run.execute-rejects-omitted-results.plan
func derivePlan(sample ExecuteSample, dir string, explore time.Duration, record bool) lang.RunPlan {
	return lang.RunPlan{Root: dir, Graph: sample.Graph, Ids: sample.Ids, Explore: explore, Record: record, Traffic: sample.Traffic}
}

//uc:generates run.execute-dispatches-selected-checks.backends
//uc:generates run.execute-merges-results.backends
//uc:generates run.execute-rejects-unknown-checks.backends
//uc:generates run.execute-saves-its-report.backends
func deriveRecordingBackends(sample ExecuteSample) []lang.Backend {
	var out []lang.Backend
	for _, name := range []string{"go", "python", "unused"} {
		out = append(out, &executionBackend{name: name, report: sample.Reports[name]})
	}
	return out
}

//uc:generates run.execute-rejects-duplicate-backends.backends
func deriveDuplicateBackends(sample ExecuteSample) []lang.Backend {
	backends := deriveRecordingBackends(sample)
	return append(backends, backends[0])
}

//uc:generates run.execute-rejects-backends-without-runner.backends
func deriveReadOnlyBackends() []lang.Backend {
	return []lang.Backend{readOnlyBackend{name: "go"}, readOnlyBackend{name: "python"}}
}

//uc:generates run.execute-rejects-omitted-results.backends
func deriveOmittingBackends(sample ExecuteSample) []lang.Backend {
	var out []lang.Backend
	for _, name := range []string{"go", "python", "unused"} {
		out = append(out, &executionBackend{name: name, report: sample.Reports[name], omit: true})
	}
	return out
}

// received reads the run inputs each recording backend was given, the
// graph as its snapshot.
func received(backends []lang.Backend) (map[string][]RunReceived, error) {
	out := map[string][]RunReceived{}
	for _, b := range backends {
		recording, ok := b.(*executionBackend)
		if !ok {
			return nil, errors.New("a backend of the sample does not record its inputs")
		}
		out[recording.name] = []RunReceived{}
		for _, p := range recording.plans {
			out[recording.name] = append(out[recording.name], RunReceived{Root: p.Root, Roots: slices.Clone(p.Roots), Snapshot: p.Graph.Encode(),
				Ids: slices.Clone(p.Ids), Explore: p.Explore, Record: p.Record, Traffic: maps.Clone(p.Traffic)})
		}
	}
	return out, nil
}

//uc:observes run.execute-dispatches-selected-checks.received
func observeDispatched(backends []lang.Backend) (map[string][]RunReceived, error) {
	return received(backends)
}

//uc:observes run.execute-rejects-unknown-checks.received
func observeRejectedDispatch(backends []lang.Backend) (map[string][]RunReceived, error) {
	return received(backends)
}

// observeStored reads the report in .unicode/results.json under dir, none
// when there is no such file.
//
//uc:observes run.execute-saves-its-report.stored
func observeStored(dir string) (*lang.RunReport, error) {
	data, err := os.ReadFile(filepath.Join(dir, ".unicode", "results.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var report lang.RunReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, err
	}
	return &report, nil
}

//uc:checks run.execute-writes-report-json
func checkExecuteWritesReportJson(t rapid.TB, sample ExecuteSample) {
	root, err := os.MkdirTemp("", "unicode-run-")
	if err != nil {
		t.Fatalf("no temporary directory for the repository: %v", err)
	}
	defer func() { _ = os.RemoveAll(root) }()
	got := Execute(lang.RunPlan{Root: root, Graph: sample.Graph, Ids: sample.Ids, Explore: time.Second, Traffic: sample.Traffic}, deriveRecordingBackends(sample))
	written, err := os.ReadFile(filepath.Join(root, ".unicode", "results.json"))
	if err != nil {
		t.Fatalf("execute wrote no .unicode/results.json: %v", err)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("the returned report does not encode as JSON: %v", err)
	}
	if string(written) != string(encoded)+"\n" {
		t.Fatalf("results.json is not the returned report's JSON and a newline:\nfile:   %s\nreport: %s", written, encoded)
	}
	var report struct {
		Results     []map[string]json.RawMessage `json:"results"`
		Diagnostics json.RawMessage              `json:"diagnostics"`
	}
	if err := json.Unmarshal(written, &report); err != nil {
		t.Fatalf("results.json is not a JSON report: %v", err)
	}
	if report.Results == nil || string(report.Diagnostics) == "null" {
		t.Fatalf("results.json writes the report's results or diagnostics as null: %s", written)
	}
	for _, result := range report.Results {
		for _, key := range []string{"observations", "differences", "diagnostics", "cases"} {
			if raw, ok := result[key]; !ok || string(raw) == "null" {
				t.Fatalf("results.json writes the %s of a result as null or leaves it out: %s", key, written)
			}
		}
	}
}
