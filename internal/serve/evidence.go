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

func (st *state) resultNodes(id, form string, current bool) []string {
	out := []string{}
	times := map[string]time.Time{}
	for _, source := range st.sources(id, graph.EdgeKindAbout) {
		node := st.nodes[source]
		if node.Layer != graph.LayerEvidence || node.Kind != graph.NodeKindResult || node.Form != form || current && st.attr(source, "fresh") != "true" {
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
		summaries := st.resultNodes(id, "check", true)
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
	if node.Layer == graph.LayerDesign && node.Kind == graph.NodeKindCase {
		history := []RunResult{}
		for _, candidate := range st.resultNodes(id, "case", false) {
			if st.attr(candidate, "rejected") == "true" {
				continue
			}
			at, err := time.Parse(time.RFC3339Nano, st.attr(candidate, "at"))
			if err != nil {
				continue
			}
			run := st.parent(candidate)
			history = append(history, RunResult{At: at, Commit: st.attr(run, "commit"), Mode: DriverMode(st.attr(candidate, "mode")), Outcome: outcome(st.attr(candidate, "passed") == "true")})
			if out.Outcome == nil && st.attr(candidate, "caseFresh") == "true" {
				value := outcome(st.attr(candidate, "passed") == "true")
				out.Outcome = &value
			}
		}
		if len(history) > 0 {
			out.History, present = &history, true
		}
		if len(st.resultNodes(id, "case", true)) > 0 {
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
