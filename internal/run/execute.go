package run

import (
	"cmp"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/evidence"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
	"github.com/lincyaw/unicode/internal/repo"
)

// Execute dispatches the checks plan selects to their language runners and
// saves the complete report, including diagnostics, to .unicode/results.json.
// Each runner gets plan's root, graph, exploration budget and recording,
// its language's source roots, its targets, and the inputs plan.Traffic
// gives them from collected calls, replayed with their cases.
//
//uc:realizes run.execute
func Execute(plan lang.RunPlan, backends []lang.Backend) lang.RunReport {
	root, g, ids, explore, record, traffic := plan.Root, plan.Graph, plan.Ids, plan.Explore, plan.Record, plan.Traffic
	out := lang.RunReport{Results: []lang.RunResult{}, Diagnostics: []diag.Diagnostic{}, Version: 1,
		Snapshot: graph.InputDigest(g), At: time.Now().UTC().Format(time.RFC3339Nano), Mode: "replay", Venue: lang.VenueTest}
	if explore > 0 {
		out.Mode = "explore"
	}
	head, err := repo.ReadHead(root, filepath.Join(root, ".unicode"))
	if err != nil {
		out.Diagnostics = append(out.Diagnostics, diagnostic("run.metadata-failed", "design/system.uc", err.Error(), "restore access to the repository metadata before running checks"))
	} else {
		out.Dirty = head.Dirty
		if head.Commit != nil {
			out.Commit = *head.Commit
		}
	}
	available := map[string]lang.Backend{}
	for _, b := range backends {
		name := b.Language()
		if _, exists := available[name]; exists {
			out.Diagnostics = append(out.Diagnostics, diagnostic("run.duplicate-backend", "design/system.uc", "more than one backend provides "+name, "register exactly one backend for each language"))
		}
		available[name] = b
	}
	roots := lang.LanguageRoots(g)
	configured := slices.Sorted(maps.Keys(roots))
	selected := map[string][]string{}
	requested := slices.Clone(ids)
	if len(requested) == 0 {
		for _, n := range g.Nodes() {
			if checkable(n) {
				requested = append(requested, n.Id)
			}
		}
	}
	if len(requested) == 0 {
		out.Diagnostics = append(out.Diagnostics, diagnostic("run.no-checks", "design/system.uc", "the repository has no property or flow to run", "declare a property with executable cases before running unicode test"))
	}
	for _, id := range requested {
		n := g.Node(id)
		if n == nil || !checkable(*n) {
			out.Diagnostics = append(out.Diagnostics, diagnostic("run.unknown-check", "design/system.uc", "the target "+id+" is not a property or flow", "select an existing property or flow identity"))
			continue
		}
		found := false
		for _, name := range configured {
			if checkLanguage(g, id, name) {
				if !slices.Contains(selected[name], id) {
					selected[name] = append(selected[name], id)
				}
				found = true
			}
		}
		if !found {
			d := diagnostic("run.unknown-check", n.Origin.Path, "the target "+id+" has no predicate in a configured language", "generate and bind its predicate in a configured language before running it")
			d.Subject = id
			d.Position = n.Origin
			out.Diagnostics = append(out.Diagnostics, d)
		}
	}
	for _, name := range configured {
		if len(selected[name]) == 0 {
			continue
		}
		if _, ok := available[name].(lang.Runner); !ok {
			out.Diagnostics = append(out.Diagnostics, diagnostic("run.unsupported-language", "design/system.uc", "no execution backend provides "+name, "register a runner for this language, or remove its language roots"))
		}
	}
	if len(out.Diagnostics) == 0 {
		for _, name := range configured {
			if len(selected[name]) == 0 {
				continue
			}
			b := available[name].(lang.Runner)
			slices.Sort(selected[name])
			inputs := map[string][]lang.Input{}
			for _, id := range selected[name] {
				if len(traffic[id]) > 0 {
					inputs[id] = traffic[id]
				}
			}
			report := b.Run(lang.RunPlan{Root: root, Roots: slices.Clone(roots[name]), Graph: g, Ids: slices.Clone(selected[name]), Explore: explore, Record: record, Traffic: inputs})
			expected := map[string]bool{}
			for _, id := range selected[name] {
				expected[id] = false
			}
			for _, result := range report.Results {
				if result.Passed && result.Accepted <= 0 {
					out.Diagnostics = append(out.Diagnostics, diagnostic("run.invalid-report", "design/system.uc", "the "+name+" runner reported success without an effective execution for "+result.Id, "execute at least one valid input before reporting a check as passed"))
				}
				already, selected := expected[result.Id]
				if !selected || already {
					out.Diagnostics = append(out.Diagnostics, diagnostic("run.invalid-report", "design/system.uc", "the "+name+" runner returned an unselected or duplicate result "+result.Id, "make the runner return exactly one result per selected check"))
				}
				if selected {
					expected[result.Id] = true
				}
				if result.Observations == nil {
					result.Observations = []string{}
				}
				if result.Differences == nil {
					result.Differences = []string{}
				}
				if result.Diagnostics == nil {
					result.Diagnostics = []diag.Diagnostic{}
				}
				if result.Cases == nil {
					result.Cases = []lang.CaseResult{}
				}
				out.Results = append(out.Results, result)
			}
			for _, id := range selected[name] {
				if !expected[id] {
					out.Diagnostics = append(out.Diagnostics, diagnostic("run.invalid-report", "design/system.uc", "the "+name+" runner omitted the selected check "+id, "make the runner return exactly one result per selected check"))
				}
			}
			out.Diagnostics = append(out.Diagnostics, report.Diagnostics...)
		}
	}
	slices.SortFunc(out.Results, func(a, b lang.RunResult) int { return strings.Compare(a.Id, b.Id) })
	sortDiagnostics(out.Diagnostics)
	for _, problem := range evidence.Save(root, out) {
		if problem.Code == "evidence.write-failed" {
			problem.Code = "run.results-write-failed"
		}
		out.Diagnostics = append(out.Diagnostics, problem)
	}
	sortDiagnostics(out.Diagnostics)
	return out
}

func checkable(n graph.Node) bool {
	return n.Layer == graph.LayerDesign && (n.Kind == graph.NodeKindProperty || n.Kind == graph.NodeKindFlow)
}

func checkLanguage(g *graph.Graph, id, name string) bool {
	parents := map[string][]string{}
	var pending []string
	for _, e := range g.Edges() {
		if e.Kind == graph.EdgeKindChecks && e.To == id {
			pending = append(pending, e.From)
		}
		if e.Kind == graph.EdgeKindContains {
			parents[e.To] = append(parents[e.To], e.From)
		}
	}
	seen := map[string]bool{}
	for len(pending) > 0 {
		current := pending[0]
		pending = pending[1:]
		if seen[current] {
			continue
		}
		seen[current] = true
		n := g.Node(current)
		if n == nil || n.Layer != graph.LayerCode {
			continue
		}
		if n.Kind == graph.NodeKindContainer && n.Form == "file" && slices.Contains(n.Attrs["language"], name) {
			return true
		}
		pending = append(pending, parents[current]...)
	}
	check := g.Node(id)
	if check == nil || len(check.Attrs["manual"]) > 0 {
		return false
	}
	targets := map[string]bool{}
	for _, edge := range g.Edges() {
		if edge.From == id && edge.Kind == graph.EdgeKindAbout {
			targets[edge.To] = true
		}
	}
	for _, edge := range g.Edges() {
		if edge.Kind == graph.EdgeKindRealizes && targets[edge.To] && codeLanguage(g, edge.From, name, parents) {
			return true
		}
	}
	owners := map[string]bool{}
	pending = []string{id}
	seen = map[string]bool{}
	for len(pending) > 0 {
		current := pending[0]
		pending = pending[1:]
		if seen[current] {
			continue
		}
		seen[current] = true
		if node := g.Node(current); node != nil && node.Layer == graph.LayerDesign && node.Kind == graph.NodeKindContainer {
			owners[current] = true
			break
		}
		pending = append(pending, parents[current]...)
	}
	for _, edge := range g.Edges() {
		if edge.Kind == graph.EdgeKindOwns && owners[edge.From] && codeLanguage(g, edge.To, name, parents) {
			return true
		}
	}
	return false
}

func codeLanguage(g *graph.Graph, id, name string, parents map[string][]string) bool {
	pending, seen := []string{id}, map[string]bool{}
	for len(pending) > 0 {
		current := pending[0]
		pending = pending[1:]
		if seen[current] {
			continue
		}
		seen[current] = true
		n := g.Node(current)
		if n == nil || n.Layer != graph.LayerCode {
			continue
		}
		if n.Kind == graph.NodeKindContainer && n.Form == "file" && slices.Contains(n.Attrs["language"], name) {
			return true
		}
		pending = append(pending, parents[current]...)
	}
	return false
}

func diagnostic(code, path, cause, fix string) diag.Diagnostic {
	if path == "" {
		path = "design/system.uc"
	}
	return diag.Diagnostic{Code: code, Subject: path, Position: diag.Position{Path: path, Line: 1, Column: 1}, Message: cause, Fix: fix}
}

func sortDiagnostics(ds []diag.Diagnostic) {
	slices.SortFunc(ds, func(a, b diag.Diagnostic) int {
		return cmp.Or(strings.Compare(a.Position.Path, b.Position.Path), cmp.Compare(a.Position.Line, b.Position.Line), cmp.Compare(a.Position.Column, b.Position.Column), strings.Compare(a.Code, b.Code), strings.Compare(a.Subject, b.Subject))
	})
}
