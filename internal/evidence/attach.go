package evidence

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
)

// Attached is the evidence projection. Unrecorded generated inputs are retained
// in run files, not copied into the repository snapshot.
//
//uc:realizes evidence.Attached
type Attached struct {
	Fragment    graph.Fragment    `json:"fragment"`
	Diagnostics []diag.Diagnostic `json:"diagnostics"`
}

type caseIndex struct {
	byCheck    map[string]map[string][]string
	incomplete map[string]bool
}

func currentCases(g *graph.Graph) (caseIndex, []diag.Diagnostic) {
	out := caseIndex{byCheck: map[string]map[string][]string{}, incomplete: map[string]bool{}}
	problems := []diag.Diagnostic{}
	for _, edge := range g.Edges() {
		if edge.Kind != graph.EdgeKindContains {
			continue
		}
		node, check := g.Node(edge.To), g.Node(edge.From)
		if node == nil || node.Layer != graph.LayerDesign || node.Kind != graph.NodeKindCase || check == nil || check.Kind != graph.NodeKindProperty && check.Kind != graph.NodeKindFlow {
			continue
		}
		if out.byCheck[check.Id] == nil {
			out.byCheck[check.Id] = map[string][]string{}
		}
		input, err := graphCaseInput(g, *node, check.Id)
		if err != nil {
			d := diagnostic("evidence.unreplayable-input", node.Id, err.Error(), "restore the case's complete typed input and codec representation")
			if node.Origin.Path != "" {
				d.Position = node.Origin
				if d.Position.Line < 1 {
					d.Position.Line = 1
				}
				if d.Position.Column < 1 {
					d.Position.Column = 1
				}
			}
			problems = append(problems, d)
			out.incomplete[check.Id] = true
			continue
		}
		key := hash(input.Values)
		out.byCheck[check.Id][key] = append(out.byCheck[check.Id][key], node.Id)
	}
	return out, problems
}
func stringsOf[T any](values []T) []string {
	out := []string{}
	for _, value := range values {
		out = append(out, string(canonical(value)))
	}
	return out
}
func diagnosticAttrs(problems []diag.Diagnostic) map[string][]string {
	codes, subjects := []string{}, []string{}
	for _, problem := range problems {
		codes = append(codes, problem.Code)
		subjects = append(subjects, problem.Subject)
	}
	return map[string][]string{"diagnosticCodes": codes, "diagnosticSubjects": subjects, "diagnostics": stringsOf(problems)}
}
func textAttrs(values map[string]string) map[string][]string {
	out := map[string][]string{}
	for key, value := range values {
		out[key] = []string{value}
	}
	return out
}
func applyAttrs(target, extra map[string][]string) {
	for key, value := range extra {
		target[key] = value
	}
}
func existingCheck(g *graph.Graph, id string) bool {
	node := g.Node(id)
	return node != nil && node.Layer == graph.LayerDesign && (node.Kind == graph.NodeKindProperty || node.Kind == graph.NodeKindFlow)
}

func batchBusinessFailure(result lang.RunResult, environment []diag.Diagnostic) bool {
	if result.Passed || result.Accepted <= 0 {
		return false
	}
	for _, problem := range result.Diagnostics {
		associated := slices.ContainsFunc(environment, func(top diag.Diagnostic) bool { return top.Code == problem.Code && top.Subject == problem.Subject })
		if associated {
			continue
		}
		switch problem.Code {
		case "lang.golang.ensure-failed", "lang.golang.frame-violation", "lang.golang.purity-violation", "lang.golang.undeclared-error", "lang.golang.predicate-failed", "lang.golang.call-panicked", "lang.golang.within-exceeded",
			"monitor.ensure-failed", "monitor.frame-violation", "monitor.undeclared-error", "monitor.call-panicked", "monitor.within-exceeded":
			return true
		}
	}
	return false
}

// runtimeAttrs are the attrs of a runtime conclusion: the limits and
// preconditions that failed and the clauses judged, each with its count in a
// parallel attr, the 95th percentile in nanoseconds and the traces checked; a
// replay of collected calls has the first two attrs too.
func runtimeAttrs(result lang.RunResult) map[string][]string {
	out := map[string][]string{}
	if len(result.OutsideDomain) > 0 {
		out["outsideDomain"], out["outsideDomainCalls"] = []string{}, []string{}
		for _, item := range result.OutsideDomain {
			out["outsideDomain"] = append(out["outsideDomain"], item.Clause)
			out["outsideDomainCalls"] = append(out["outsideDomainCalls"], strconv.Itoa(item.Calls))
		}
	}
	if len(result.Clauses) > 0 {
		out["clauses"], out["clausesJudged"] = []string{}, []string{}
		for _, item := range result.Clauses {
			out["clauses"] = append(out["clauses"], item.Clause)
			out["clausesJudged"] = append(out["clausesJudged"], strconv.Itoa(item.Judged))
		}
	}
	if result.P95 > 0 {
		out["p95"] = []string{strconv.Itoa(result.P95)}
	}
	if result.Traces > 0 {
		out["traces"] = []string{strconv.Itoa(result.Traces)}
	}
	return out
}

type coverKey struct{ from, to string }

// Attach projects history and only attributes coverage to current, equal model
// inputs. Matching by hash lets newly recorded names reuse their actual run.
//
//uc:realizes evidence.attach
func Attach(g *graph.Graph, reports []lang.RunReport) Attached {
	out := Attached{Fragment: graph.Fragment{Nodes: []graph.Node{}, Edges: []graph.Edge{}}, Diagnostics: []diag.Diagnostic{}}
	current, problems := currentCases(g)
	out.Diagnostics = append(out.Diagnostics, problems...)
	inputDigest := graph.InputDigest(g)
	seen := map[string]bool{}
	covers := map[coverKey]graph.Edge{}
	addAbout := func(from, to string, at diag.Position) {
		if g.Node(to) != nil {
			out.Fragment.Edges = append(out.Fragment.Edges, graph.Edge{Kind: graph.EdgeKindAbout, From: from, To: to, Source: graph.SourceProduced, Origin: at})
		}
	}
	for _, source := range reports {
		report, err := normalizeReport(source, true)
		if err != nil {
			out.Diagnostics = append(out.Diagnostics, storeDiagnostic("evidence.invalid-report", ".unicode/results.json", err.Error(), "restore the complete measured report before attaching evidence"))
			continue
		}
		id := hash(report)
		if seen[id] {
			continue
		}
		seen[id] = true
		runID := "run:" + id
		at := diag.Position{Path: reportPath(id), Line: 1, Column: 1}
		fresh := report.Version == 1 && report.Snapshot == inputDigest
		venue, form := report.Venue, report.Mode
		if venue == "" {
			venue = lang.VenueTest
		}
		if venue == lang.VenueRuntime {
			form = lang.VenueRuntime
		}
		attrs := textAttrs(map[string]string{"version": strconv.Itoa(report.Version), "snapshot": report.Snapshot, "at": report.At, "commit": report.Commit, "dirty": strconv.FormatBool(report.Dirty), "mode": report.Mode, "fresh": strconv.FormatBool(fresh), "venue": venue})
		if venue == lang.VenueRuntime {
			attrs["from"] = []string{report.From}
		}
		applyAttrs(attrs, diagnosticAttrs(report.Diagnostics))
		out.Fragment.Nodes = append(out.Fragment.Nodes, graph.Node{Id: runID, Kind: graph.NodeKindRun, Form: form, Layer: graph.LayerEvidence, Source: graph.SourceProduced, Origin: at, Attrs: attrs})
		for _, result := range report.Results {
			sourceFresh := fresh && existingCheck(g, result.Id)
			observed := map[string]bool{}
			failed := batchBusinessFailure(result, report.Diagnostics)
			for _, candidate := range result.Cases {
				if !candidate.Rejected {
					observed[candidate.InputHash] = true
					failed = failed || !candidate.Passed
				}
			}
			complete := sourceFresh && result.Accepted > 0 && len(observed) > 0 && !current.incomplete[result.Id]
			for key := range current.byCheck[result.Id] {
				complete = complete && observed[key]
			}
			resultID := "result:" + id + ":check:" + hash(result.Id)
			summary := textAttrs(map[string]string{"check": result.Id, "snapshot": report.Snapshot, "fresh": strconv.FormatBool(sourceFresh), "passed": strconv.FormatBool(result.Passed), "accepted": strconv.Itoa(result.Accepted), "rejected": strconv.Itoa(result.Rejected), "complete": strconv.FormatBool(complete), "businessFailed": strconv.FormatBool(failed), "mode": report.Mode, "at": report.At})
			if result.CodeTotal > 0 {
				summary["codeTotal"] = []string{strconv.Itoa(result.CodeTotal)}
			}
			if result.BranchTotal > 0 {
				summary["branchTotal"] = []string{strconv.Itoa(result.BranchTotal)}
			}
			if result.ErrorTotal > 0 {
				summary["errorTotal"] = []string{strconv.Itoa(result.ErrorTotal)}
			}
			summary["observations"], summary["differences"] = slices.Clone(result.Observations), slices.Clone(result.Differences)
			summary["venue"] = []string{venue}
			applyAttrs(summary, runtimeAttrs(result))
			applyAttrs(summary, diagnosticAttrs(result.Diagnostics))
			out.Fragment.Nodes = append(out.Fragment.Nodes, graph.Node{Id: resultID, Kind: graph.NodeKindResult, Form: "check", Layer: graph.LayerEvidence, Source: graph.SourceProduced, Origin: at, Attrs: summary})
			out.Fragment.Edges = append(out.Fragment.Edges, graph.Edge{Kind: graph.EdgeKindContains, From: runID, To: resultID, Source: graph.SourceProduced, Origin: at})
			addAbout(resultID, result.Id, at)
			for index, candidate := range result.Cases {
				caseID := "result:" + id + ":case:" + hash([]any{result.Id, candidate.Id, candidate.InputHash, index})
				matched := current.byCheck[result.Id][candidate.InputHash]
				// A declared or recorded case is the current evidence of the test
				// venue only (docs/model.md §3.5): a runtime counterexample with
				// the same input joins its history, not its outcome.
				caseFresh := sourceFresh && len(matched) > 0 && venue == lang.VenueTest
				detail := textAttrs(map[string]string{"check": candidate.Check, "case": candidate.Id, "inputHash": candidate.InputHash, "snapshot": report.Snapshot, "fresh": strconv.FormatBool(sourceFresh), "caseFresh": strconv.FormatBool(caseFresh), "passed": strconv.FormatBool(candidate.Passed), "rejected": strconv.FormatBool(candidate.Rejected), "reason": candidate.Reason, "at": report.At, "mode": report.Mode})
				detail["coverage"], detail["trace"] = stringsOf(candidate.Coverage), stringsOf(candidate.Trace)
				detail["duration"] = []string{strconv.Itoa(candidate.Duration)}
				detail["calls"] = []string{}
				for _, c := range candidate.Calls {
					detail["calls"] = append(detail["calls"], c.Op+" "+strconv.Itoa(c.Duration))
				}
				detail["observations"], detail["differences"] = slices.Clone(candidate.Observations), slices.Clone(candidate.Differences)
				detail["venue"] = []string{venue}
				if candidate.Clause != "" {
					detail["clause"] = []string{candidate.Clause}
				}
				if candidate.At != "" {
					// A runtime counterexample is dated by its call, not by the record period.
					detail["at"] = []string{candidate.At}
				}
				if venue == lang.VenueRuntime {
					detail["values"] = stringsOf([]lang.Input{candidate.Input})
				}
				applyAttrs(detail, diagnosticAttrs(candidate.Diagnostics))
				out.Fragment.Nodes = append(out.Fragment.Nodes, graph.Node{Id: caseID, Kind: graph.NodeKindResult, Form: "case", Layer: graph.LayerEvidence, Source: graph.SourceProduced, Origin: at, Attrs: detail})
				out.Fragment.Edges = append(out.Fragment.Edges, graph.Edge{Kind: graph.EdgeKindContains, From: runID, To: caseID, Source: graph.SourceProduced, Origin: at})
				addAbout(caseID, candidate.Check, at)
				if len(matched) > 0 {
					for _, matchedID := range matched {
						addAbout(caseID, matchedID, at)
					}
				}
				if !caseFresh || candidate.Rejected {
					continue
				}
				for _, point := range candidate.Coverage {
					target := g.Node(point.Node)
					valid := target != nil
					if valid {
						switch point.Kind {
						case "code":
							valid = target.Layer == graph.LayerCode && target.Kind == graph.NodeKindOp && point.Path != "" && point.Line > 0 && point.Column > 0 && point.EndLine >= point.Line && point.EndColumn > 0
						case "step", "branch":
							valid = target.Layer == graph.LayerDesign && target.Kind == graph.NodeKindStep
						case "error":
							valid = target.Layer == graph.LayerDesign && target.Kind == graph.NodeKindType && target.Form == "error"
						default:
							valid = false
						}
					}
					if !valid {
						out.Diagnostics = append(out.Diagnostics, storeDiagnostic("evidence.invalid-report", reportPath(id), fmt.Sprintf("coverage point %s does not map to its measured node", point.Node), "restore the measured source or design coverage mapping"))
						continue
					}
					for _, matchedID := range matched {
						key := coverKey{matchedID, point.Node}
						edge, exists := covers[key]
						if !exists {
							edge = graph.Edge{Kind: graph.EdgeKindCovers, From: matchedID, To: point.Node, Source: graph.SourceProduced, Origin: at, Attrs: map[string][]string{"inputHash": {candidate.InputHash}, "snapshot": {report.Snapshot}, "coverage": {}, "results": {}}}
						}
						encoded := string(canonical(point))
						if !slices.Contains(edge.Attrs["coverage"], encoded) {
							edge.Attrs["coverage"] = append(edge.Attrs["coverage"], encoded)
						}
						if !slices.Contains(edge.Attrs["results"], caseID) {
							edge.Attrs["results"] = append(edge.Attrs["results"], caseID)
						}
						covers[key] = edge
					}
				}
			}
		}
	}
	for _, edge := range covers {
		slices.Sort(edge.Attrs["coverage"])
		slices.Sort(edge.Attrs["results"])
		out.Fragment.Edges = append(out.Fragment.Edges, edge)
	}
	slices.SortFunc(out.Fragment.Nodes, func(a, b graph.Node) int { return strings.Compare(a.Id, b.Id) })
	slices.SortFunc(out.Fragment.Edges, func(a, b graph.Edge) int {
		first := string(a.Kind) + "\x00" + a.From + "\x00" + a.To
		second := string(b.Kind) + "\x00" + b.From + "\x00" + b.To
		return strings.Compare(first, second)
	})
	return out
}

// MeanDurations gives each design operation the mean duration, in
// nanoseconds and truncated, of the calls of it that the current evidence
// records (docs/model.md §3.5): the calls of the case results of each
// property's and flow's most recent fresh run, as attach writes them. An
// operation with no such call has none.
//
//uc:realizes evidence.meanDurations
func MeanDurations(g *graph.Graph) map[string]int {
	nodes := g.Nodes()
	fresh := func(n graph.Node, form string) bool {
		return n.Layer == graph.LayerEvidence && n.Kind == graph.NodeKindResult && n.Form == form && first(n.Attrs["fresh"]) == "true"
	}
	at := func(n graph.Node) (time.Time, bool) {
		t, err := time.Parse(time.RFC3339Nano, first(n.Attrs["at"]))
		return t, err == nil
	}
	latest := map[string]time.Time{}
	for _, n := range nodes {
		if t, ok := at(n); ok && fresh(n, "check") && t.After(latest[first(n.Attrs["check"])]) {
			latest[first(n.Attrs["check"])] = t
		}
	}
	// attach writes at from a report whose time it checked to be canonical,
	// and each call as "<op> <ns>"; an evidence node that does not parse so
	// was not written by attach, which reports what it refuses
	// (evidence.invalid-report), and gives no mean.
	total, count := map[string]int{}, map[string]int{}
	for _, n := range nodes {
		if t, ok := at(n); !ok || !fresh(n, "case") || !t.Equal(latest[first(n.Attrs["check"])]) {
			continue
		}
		for _, call := range n.Attrs["calls"] {
			op, text, _ := strings.Cut(call, " ")
			if d, err := strconv.Atoi(text); err == nil {
				total[op] += d
				count[op]++
			}
		}
	}
	out := map[string]int{}
	for op, n := range count {
		if node := g.Node(op); node != nil && node.Layer == graph.LayerDesign && node.Kind == graph.NodeKindOp {
			out[op] = total[op] / n
		}
	}
	return out
}

func first(values []string) string {
	if len(values) > 0 {
		return values[0]
	}
	return ""
}
