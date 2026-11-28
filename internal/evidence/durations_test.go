package evidence

import (
	"fmt"
	"strings"
	"time"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
)

// DurationSample is a graph, run reports about its properties, and the
// mean duration of each design operation that docs/model.md §3.5 gives,
// computed from the reports.
//
//uc:realizes evidence.DurationSample
type DurationSample struct {
	Graph    *graph.Graph
	Reports  []lang.RunReport
	Expected map[string]int
}

var (
	durationOps    = []string{"domain.a", "domain.b", "domain.c"}
	durationCalled = []string{"domain.a", "domain.b", "domain.c", "domain.gone"}
	durationChecks = []string{"domain.p0", "domain.p1"}
)

// genDurationSample draws one to four reports about the properties
// domain.p0 and domain.p1 of a graph with the operations domain.a, b and c.
// About three in four are fresh, the others carry another execution-input
// digest; their times are distinct, some a fraction of a second apart. Each
// carries a result for each property with chance three in four, and for
// domain.p1 when it has none for domain.p0, each with zero
// to three passing cases, each lasting 1 to 1000000 ns and calling zero to
// three of the operations or domain.gone, which is not one, for at most as
// long. It computes, for each operation, the truncated mean duration of its
// calls in the cases of each property's most recent fresh result.
//
//uc:generates evidence.mean-durations-follow-current-evidence.sample
func genDurationSample() *rapid.Generator[DurationSample] {
	return rapid.Custom(func(t *rapid.T) DurationSample {
		g := graph.NewGraph()
		nodes := []graph.Node{{Id: "domain", Kind: graph.NodeKindContainer, Form: "component", Layer: graph.LayerDesign}}
		for _, op := range durationOps {
			nodes = append(nodes, graph.Node{Id: op, Kind: graph.NodeKindOp, Form: "op", Layer: graph.LayerDesign})
		}
		for _, p := range durationChecks {
			nodes = append(nodes, graph.Node{Id: p, Kind: graph.NodeKindProperty, Form: "postcondition", Layer: graph.LayerDesign, Attrs: map[string][]string{"symbols": {"payload: int"}}})
		}
		for _, n := range nodes {
			if err := g.AddNode(n); err != nil {
				panic(err)
			}
			if n.Id != "domain" {
				if err := g.AddEdge(graph.Edge{Kind: graph.EdgeKindContains, From: "domain", To: n.Id}); err != nil {
					panic(err)
				}
			}
		}
		current := graph.InputDigest(&g)
		stale := strings.Repeat("0", 64)
		base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		offsets := rapid.SliceOfNDistinct(rapid.IntRange(0, 40), 1, 4, rapid.ID).Draw(t, "offsets")
		type latest struct {
			at    time.Time
			cases []lang.CaseResult
		}
		chosen := map[string]latest{}
		var reports []lang.RunReport
		for i, offset := range offsets {
			// Half-second steps make some times differ only in their fraction.
			at := base.Add(time.Duration(offset) * 500 * time.Millisecond)
			report := lang.RunReport{Version: 1, Snapshot: current, At: at.Format(time.RFC3339Nano), Mode: "replay", Results: []lang.RunResult{}, Diagnostics: []diag.Diagnostic{}}
			fresh := rapid.IntRange(0, 3).Draw(t, "fresh") > 0
			if !fresh {
				report.Snapshot = stale
			}
			for k, check := range durationChecks {
				// A report holds a result: when the first property has none,
				// the second one does.
				if rapid.IntRange(0, 3).Draw(t, "result") == 0 && (k == 0 || len(report.Results) > 0) {
					continue
				}
				result := lang.RunResult{Id: check, Passed: true, Cases: []lang.CaseResult{}, Observations: []string{}, Differences: []string{}, Diagnostics: []diag.Diagnostic{}}
				for j := range rapid.IntRange(0, 3).Draw(t, "cases") {
					input, err := NormalizeInput(lang.Input{Values: map[string]lang.Value{"payload": intValue(i*10 + j)}})
					if err != nil {
						panic(err)
					}
					duration := rapid.IntRange(1, 1000000).Draw(t, "duration")
					candidate := lang.CaseResult{Check: check, Form: "recorded", Reason: "coverage", Input: input, InputHash: hash(input.Values), Passed: true,
						Coverage: []lang.Coverage{}, Trace: []lang.TraceEvent{}, Diagnostics: []diag.Diagnostic{}, Duration: duration, Calls: []lang.CallTime{}}
					for range rapid.IntRange(0, 3).Draw(t, "calls") {
						op := rapid.SampledFrom(durationCalled).Draw(t, "op")
						candidate.Calls = append(candidate.Calls, lang.CallTime{Op: op, Duration: rapid.IntRange(0, duration).Draw(t, "callDuration")})
					}
					result.Cases = append(result.Cases, candidate)
				}
				result.Accepted = len(result.Cases)
				report.Results = append(report.Results, result)
				if fresh && at.After(chosen[check].at) {
					chosen[check] = latest{at: at, cases: result.Cases}
				}
			}
			reports = append(reports, report)
		}
		total, count := map[string]int{}, map[string]int{}
		for _, l := range chosen {
			for _, c := range l.cases {
				for _, call := range c.Calls {
					total[call.Op] += call.Duration
					count[call.Op]++
				}
			}
		}
		expected := map[string]int{}
		for _, op := range durationOps {
			if count[op] > 0 {
				expected[op] = total[op] / count[op]
			}
		}
		return DurationSample{Graph: &g, Reports: reports, Expected: expected}
	})
}

// deriveDurationSnapshot adds to a copy of the sample's graph the evidence
// layer that Attach gives for its reports.
//
//uc:generates evidence.mean-durations-follow-current-evidence.snapshot
func deriveDurationSnapshot(sample DurationSample) (*graph.Graph, error) {
	g := graph.NewGraph()
	if err := g.AddFragment(graph.Fragment{Nodes: sample.Graph.Nodes(), Edges: sample.Graph.Edges()}); err != nil {
		return nil, err
	}
	attached := Attach(&g, sample.Reports)
	if len(attached.Diagnostics) > 0 {
		return nil, fmt.Errorf("attach refused the sample's reports: %+v", attached.Diagnostics)
	}
	if err := g.AddFragment(attached.Fragment); err != nil {
		return nil, err
	}
	return &g, nil
}
