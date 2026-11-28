package run

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
)

// ExecuteSample supplies configured checks, selected targets and runner reports.
//
//uc:realizes run.ExecuteSample
type ExecuteSample struct {
	Graph   *graph.Graph
	Ids     []string
	Traffic map[string][]lang.Input
	Reports map[string]lang.RunReport
}

//uc:generates run.execute-follows-language-roots.sample
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
		if rapid.IntRange(0, 3).Draw(t, "unknown") == 0 {
			targets = append(targets, "missing.check")
		}
		return ExecuteSample{Graph: &g, Ids: targets, Traffic: traffic, Reports: reports}
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
		for _, r := range b.report.Results {
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

//uc:checks run.execute-follows-language-roots
func checkExecuteFollowsLanguageRoots(t rapid.TB, sample ExecuteSample) {
	root, err := os.MkdirTemp("", "unicode-run-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(root) }()
	backends := []lang.Backend{}
	byName := map[string]*executionBackend{}
	for _, name := range []string{"go", "python", "unused"} {
		b := &executionBackend{name: name, report: sample.Reports[name]}
		byName[name] = b
		backends = append(backends, b)
	}
	budget := time.Second
	plan := lang.RunPlan{Root: root, Graph: sample.Graph, Ids: sample.Ids, Explore: budget, Traffic: sample.Traffic}
	got := Execute(plan, backends)
	unknown := slices.Contains(sample.Ids, "missing.check")
	if unknown {
		if !slices.ContainsFunc(got.Diagnostics, func(d diag.Diagnostic) bool { return d.Code == "run.unknown-check" }) {
			t.Fatal("Execute accepted an unknown target")
		}
		for name, b := range byName {
			if len(b.plans) != 0 {
				t.Fatalf("Execute called %s after rejecting a target", name)
			}
		}
	} else {
		if len(got.Diagnostics) != 0 {
			t.Fatalf("Execute rejected selected checks: %+v", got.Diagnostics)
		}
		want := []lang.RunResult{}
		for name, b := range byName {
			var roots, ids []string
			for _, entry := range sample.Graph.Node("system").Attrs["roots"] {
				language, dir, _ := strings.Cut(entry, ": ")
				if language == name {
					roots = append(roots, dir)
				}
			}
			for _, r := range b.report.Results {
				if len(sample.Ids) == 0 || slices.Contains(sample.Ids, r.Id) {
					ids = append(ids, r.Id)
					r.Cases = []lang.CaseResult{}
					r.Observations = []string{}
					r.Differences = []string{}
					if r.Diagnostics == nil {
						r.Diagnostics = []diag.Diagnostic{}
					}
					want = append(want, r)
				}
			}
			slices.Sort(ids)
			if len(ids) == 0 {
				if len(b.plans) != 0 {
					t.Fatalf("Execute called %s without selected checks", name)
				}
				continue
			}
			if len(b.plans) != 1 {
				t.Fatalf("Execute called %s %d times", name, len(b.plans))
			}
			p := b.plans[0]
			if p.Root != root || p.Graph != sample.Graph || p.Explore != budget || !slices.Equal(p.Roots, roots) || !slices.Equal(p.Ids, ids) {
				t.Fatalf("Execute passed wrong inputs to %s: %+v", name, p)
			}
			traffic := map[string][]lang.Input{}
			for _, id := range ids {
				if len(sample.Traffic[id]) > 0 {
					traffic[id] = sample.Traffic[id]
				}
			}
			if !reflect.DeepEqual(p.Traffic, traffic) {
				t.Fatalf("Execute passed %s the collected inputs %+v; want those of its targets %+v", name, p.Traffic, traffic)
			}
		}
		slices.SortFunc(want, func(a, b lang.RunResult) int { return strings.Compare(a.Id, b.Id) })
		if !reflect.DeepEqual(got.Results, want) {
			t.Fatalf("Execute changed runner results: %+v; want %+v", got.Results, want)
		}
	}
	encoded, err := os.ReadFile(filepath.Join(root, ".unicode", "results.json"))
	if err != nil {
		t.Fatal(err)
	}
	wantJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != string(wantJSON)+"\n" {
		t.Fatalf("Execute did not persist its complete report: %s", encoded)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &object); err != nil {
		t.Fatal(err)
	}
	if string(object["diagnostics"]) == "null" || string(object["results"]) == "null" {
		t.Fatal("report arrays encoded as null")
	}
	if !unknown {
		plan.Explore = 0
		duplicate := Execute(plan, append(backends, backends[0]))
		if !slices.ContainsFunc(duplicate.Diagnostics, func(d diag.Diagnostic) bool { return d.Code == "run.duplicate-backend" }) {
			t.Fatal("Execute accepted duplicate backends")
		}
		unsupported := Execute(plan, []lang.Backend{readOnlyBackend{name: "go"}, readOnlyBackend{name: "python"}})
		if !slices.ContainsFunc(unsupported.Diagnostics, func(d diag.Diagnostic) bool { return d.Code == "run.unsupported-language" }) {
			t.Fatal("Execute accepted backends without execution capability")
		}
		for _, b := range byName {
			b.omit = true
		}
		omitted := Execute(plan, backends)
		if !slices.ContainsFunc(omitted.Diagnostics, func(d diag.Diagnostic) bool { return d.Code == "run.invalid-report" }) {
			t.Fatal("Execute accepted missing execution results")
		}
	}
}
