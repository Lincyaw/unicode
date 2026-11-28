package serve

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
)

// resultNodes are the result nodes of form about id, newest first: those of
// the venue given, or of either when it is empty, and only the current ones
// when current is set.
func (st *state) resultNodes(id, form, venue string, current bool) []string {
	out := []string{}
	times := map[string]time.Time{}
	for _, source := range st.sources(id, graph.EdgeKindAbout) {
		node := st.nodes[source]
		if node.Layer != graph.LayerEvidence || node.Kind != graph.NodeKindResult || node.Form != form || current && st.attr(source, "fresh") != "true" || venue != "" && st.venue(source) != venue {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, st.attr(source, "at"))
		if err != nil {
			// Invalid reports already have evidence diagnostics and cannot select an outcome.
			continue
		}
		times[source] = at
		out = append(out, source)
	}
	slices.SortFunc(out, func(a, b string) int {
		return cmp.Or(times[b].Compare(times[a]), cmp.Compare(b, a))
	})
	return out
}

// venue is where a result was taken: test, unless its node says runtime.
func (st *state) venue(id string) string {
	if st.attr(id, "venue") == lang.VenueRuntime {
		return lang.VenueRuntime
	}
	return lang.VenueTest
}

func outcome(passed bool) Outcome {
	if passed {
		return OutcomePassed
	}
	return OutcomeFailed
}

func (st *state) evidenceOf(id string) *Evidence {
	node := st.nodes[id]
	out := &Evidence{}
	present := false
	if node.Layer == graph.LayerDesign && (node.Kind == graph.NodeKindProperty || node.Kind == graph.NodeKindFlow) {
		if len(st.incoming(id, graph.EdgeKindChecks)) > 0 {
			count := int32(len(st.children(id, graph.NodeKindCase)))
			drivers := []Driver{{Mode: DriverModeReplay, Tool: "go test", Cases: &count}, {Mode: DriverModeExplore, Tool: "rapid"}, {Mode: DriverModeFuzz, Tool: "go test -fuzz"}}
			out.Drivers, present = &drivers, true
		}
		summaries := st.resultNodes(id, "check", lang.VenueTest, true)
		if len(summaries) > 0 {
			summary := summaries[0]
			if st.attr(summary, "businessFailed") == "true" || st.attr(summary, "complete") == "true" && st.attr(summary, "passed") == "true" {
				value := outcome(st.attr(summary, "passed") == "true" && st.attr(summary, "businessFailed") != "true")
				out.Outcome, present = &value, true
			}
			parent := st.parent(summary)
			points := map[string]map[string]bool{"code": {}, "branch": {}, "error": {}}
			for _, candidate := range st.children(parent, graph.NodeKindResult) {
				if st.nodes[candidate].Form != "case" || st.attr(candidate, "check") != id || st.attr(candidate, "rejected") == "true" {
					continue
				}
				for _, point := range coverage(st.nodes[candidate].Attrs["coverage"]) {
					if _, ok := points[point.Kind]; ok {
						points[point.Kind][coverageID(point)] = true
					}
				}
			}
			adequacy := &Adequacy{}
			for kind, target := range map[string]**float64{"code": &adequacy.Code, "branch": &adequacy.Branch, "error": &adequacy.Error} {
				total, err := strconv.Atoi(st.attr(summary, kind+"Total"))
				if err == nil && total > 0 {
					ratio := float64(len(points[kind])) / float64(total)
					*target = &ratio
				}
			}
			if adequacy.Code != nil || adequacy.Branch != nil || adequacy.Error != nil {
				out.Adequacy, present = adequacy, true
			}
		}
	}
	if node.Layer == graph.LayerDesign && (node.Kind == graph.NodeKindProperty || node.Kind == graph.NodeKindFlow) {
		if runtime := st.runtimeEvidence(id); runtime != nil {
			out.Runtime, present = runtime, true
		}
	}
	if node.Layer == graph.LayerDesign && node.Kind == graph.NodeKindCase {
		history := []RunResult{}
		for _, candidate := range st.resultNodes(id, "case", "", false) {
			if st.attr(candidate, "rejected") == "true" {
				continue
			}
			at, err := time.Parse(time.RFC3339Nano, st.attr(candidate, "at"))
			if err != nil {
				continue
			}
			run := st.parent(candidate)
			result := outcome(st.attr(candidate, "passed") == "true")
			var entry RunResult
			// From… only marshals a struct of strings and times, which cannot fail.
			if st.venue(candidate) == lang.VenueRuntime {
				_ = entry.FromRuntimeRunResult(RuntimeRunResult{Venue: RuntimeRunResultVenueRuntime, Commit: st.attr(run, "commit"), At: at, Outcome: result})
			} else {
				_ = entry.FromTestRunResult(TestRunResult{Venue: TestRunResultVenueTest, Commit: st.attr(run, "commit"), At: at, Mode: DriverMode(st.attr(candidate, "mode")), Outcome: result})
			}
			history = append(history, entry)
			// The case's outcome is its current test evidence; a runtime result
			// joins only its history (docs/model.md §3.5).
			if out.Outcome == nil && st.venue(candidate) == lang.VenueTest && st.attr(candidate, "caseFresh") == "true" {
				value := outcome(st.attr(candidate, "passed") == "true")
				out.Outcome = &value
			}
		}
		if len(history) > 0 {
			out.History, present = &history, true
		}
		if len(st.resultNodes(id, "case", lang.VenueTest, true)) > 0 {
			gain := st.coverageGain(id)
			out.CoverageGain, present = &gain, true
		}
	}
	if node.Kind == graph.NodeKindStep || node.Layer == graph.LayerCode && node.Kind == graph.NodeKindOp {
		byLabel := map[string]map[string]bool{}
		for _, edge := range st.incoming(id, graph.EdgeKindCovers) {
			for _, point := range coverage(edge.Attrs["coverage"]) {
				if point.Kind != "branch" && point.Kind != "error" {
					continue
				}
				label := point.Label
				if label == "" {
					label = point.Node
				}
				if byLabel[label] == nil {
					byLabel[label] = map[string]bool{}
				}
				byLabel[label][edge.From] = true
			}
		}
		labels := []string{}
		for label := range byLabel {
			labels = append(labels, label)
		}
		slices.Sort(labels)
		branches := []BranchCount{}
		for _, label := range labels {
			branches = append(branches, BranchCount{Label: label, Cases: int32(len(byLabel[label]))})
		}
		if len(branches) > 0 {
			out.Branches, present = &branches, true
		}
	}
	if !present {
		return nil
	}
	return out
}

func coverage(encoded []string) []lang.Coverage {
	out := []lang.Coverage{}
	for _, value := range encoded {
		var point lang.Coverage
		if json.Unmarshal([]byte(value), &point) == nil {
			out = append(out, point)
		}
	}
	return out
}

func coverageID(point lang.Coverage) string {
	if point.Kind == "branch" || point.Kind == "error" {
		return point.Node + ":" + point.Label
	}
	return fmt.Sprintf("%s:%s:%d:%d:%d:%d", point.Node, point.Path, point.Line, point.Column, point.EndLine, point.EndColumn)
}

func (st *state) coverageGain(id string) int32 {
	own, others := map[string]bool{}, map[string]bool{}
	for _, candidate := range st.children(st.parent(id), graph.NodeKindCase) {
		points := own
		if candidate != id {
			points = others
		}
		for _, edge := range st.outgoing(candidate, graph.EdgeKindCovers) {
			for _, point := range coverage(edge.Attrs["coverage"]) {
				if point.Kind == "code" {
					points[coverageID(point)] = true
				}
			}
		}
	}
	var count int32
	for point := range own {
		if !others[point] {
			count++
		}
	}
	return count
}

// runtimeEvidence is the current runtime evidence of a property or flow: the
// conclusion of the newest current runtime record about it, with the
// counterexamples that record keeps (docs/design.md §7.5).
func (st *state) runtimeEvidence(id string) *RuntimeEvidence {
	summaries := st.resultNodes(id, "check", lang.VenueRuntime, true)
	if len(summaries) == 0 {
		return nil
	}
	summary := summaries[0]
	run := st.parent(summary)
	from, errFrom := time.Parse(time.RFC3339Nano, st.attr(run, "from"))
	to, errTo := time.Parse(time.RFC3339Nano, st.attr(run, "at"))
	if errFrom != nil || errTo != nil {
		return nil
	}
	// attach writes these counts with strconv.Itoa, so they always read back.
	count := func(text string) int64 { n, _ := strconv.ParseInt(text, 10, 64); return n }
	out := &RuntimeEvidence{Commit: st.attr(run, "commit"), From: from, To: to, Applicable: int32(count(st.attr(summary, "accepted"))),
		OutsideDomain: []ClauseCalls{}, Clauses: []ClauseJudged{}, Counterexamples: []RuntimeCounterexample{}}
	attrs := st.nodes[summary].Attrs
	for i, clause := range attrs["outsideDomain"] {
		if i < len(attrs["outsideDomainCalls"]) {
			out.OutsideDomain = append(out.OutsideDomain, ClauseCalls{Clause: clause, Calls: int32(count(attrs["outsideDomainCalls"][i]))})
		}
	}
	for i, clause := range attrs["clauses"] {
		if i < len(attrs["clausesJudged"]) {
			out.Clauses = append(out.Clauses, ClauseJudged{Clause: clause, Judged: int32(count(attrs["clausesJudged"][i]))})
		}
	}
	if p95 := st.attr(summary, "p95"); p95 != "" {
		value := count(p95)
		out.P95 = &value
	}
	if traces := st.attr(summary, "traces"); traces != "" {
		value := int32(count(traces))
		out.Traces = &value
	}
	for _, candidate := range st.children(run, graph.NodeKindResult) {
		if st.nodes[candidate].Form != "case" || st.attr(candidate, "check") != id {
			continue
		}
		// load checked every runtime case's time and values (runtimeCases).
		at, _ := time.Parse(time.RFC3339Nano, st.attr(candidate, "at"))
		var input lang.Input
		_ = json.Unmarshal([]byte(st.attr(candidate, "values")), &input)
		values := map[string]ModelValue{}
		for name, value := range input.Values {
			values[name] = modelValue(value)
		}
		out.Counterexamples = append(out.Counterexamples, RuntimeCounterexample{InputHash: st.attr(candidate, "inputHash"), Clause: st.attr(candidate, "clause"), Values: values, At: at})
	}
	slices.SortFunc(out.Counterexamples, func(a, b RuntimeCounterexample) int {
		return cmp.Or(a.At.Compare(b.At), cmp.Compare(a.InputHash, b.InputHash))
	})
	return out
}

// modelValue is a model value as the interface writes it.
func modelValue(v lang.Value) ModelValue {
	out := ModelValue{Kind: v.Kind, Text: v.Text}
	if v.Items != nil {
		items := make([]ModelValue, 0, len(v.Items))
		for _, item := range v.Items {
			items = append(items, modelValue(item))
		}
		out.Items = &items
	}
	if v.Entries != nil {
		entries := make([]ModelValueEntry, 0, len(v.Entries))
		for _, entry := range v.Entries {
			entries = append(entries, ModelValueEntry{Key: modelValue(entry.Key), Value: modelValue(entry.Value)})
		}
		out.Entries = &entries
	}
	return out
}

// runtimeCases checks that every runtime counterexample the snapshot holds
// has the time and the values unicode build writes, so that the runtime
// evidence is never shown with a counterexample left out.
func runtimeCases(st *state) error {
	for id, n := range st.nodes {
		if n.Layer != graph.LayerEvidence || n.Kind != graph.NodeKindResult || n.Form != "case" || st.attr(id, "venue") != lang.VenueRuntime {
			continue
		}
		var input lang.Input
		if _, err := time.Parse(time.RFC3339Nano, st.attr(id, "at")); err != nil {
			return fmt.Errorf("the runtime counterexample %s has no time: %w", id, err)
		}
		if err := json.Unmarshal([]byte(st.attr(id, "values")), &input); err != nil {
			return fmt.Errorf("the runtime counterexample %s has no values: %w", id, err)
		}
	}
	return nil
}
