package run

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/google/renameio/v2"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
)

// Execute dispatches the selected checks to their language runners and saves
// the complete report, including diagnostics, to .unicode/results.json.
//
//uc:realizes run.execute
func Execute(root string, g *graph.Graph, backends []lang.Backend, ids []string, explore time.Duration) lang.RunReport {
	out := lang.RunReport{Results: []lang.RunResult{}, Diagnostics: []diag.Diagnostic{}}
	available := map[string]lang.Backend{}
	for _, b := range backends {
		name := b.Language()
		if _, exists := available[name]; exists {
			out.Diagnostics = append(out.Diagnostics, diagnostic("run.duplicate-backend", "design/system.uc", "more than one backend provides "+name, "register exactly one backend for each language"))
		}
		available[name] = b
	}
	configs := languages(g)
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
		for _, config := range configs {
			if checkLanguage(g, id, config.name) {
				if !slices.Contains(selected[config.name], id) {
					selected[config.name] = append(selected[config.name], id)
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
	for _, config := range configs {
		if len(selected[config.name]) == 0 {
			continue
		}
		if _, ok := available[config.name].(lang.Runner); !ok {
			out.Diagnostics = append(out.Diagnostics, diagnostic("run.unsupported-language", config.at.Path, "no execution backend provides "+config.name, "register a runner for this language, or remove its language roots"))
		}
	}
	if len(out.Diagnostics) == 0 {
		for _, config := range configs {
			if len(selected[config.name]) == 0 {
				continue
			}
			b := available[config.name].(lang.Runner)
			slices.Sort(selected[config.name])
			report := b.Run(lang.RunPlan{Root: root, Roots: slices.Clone(config.roots), Graph: g, Ids: slices.Clone(selected[config.name]), Explore: explore})
			expected := map[string]bool{}
			for _, id := range selected[config.name] {
				expected[id] = false
			}
			for _, result := range report.Results {
				if result.Passed && result.Accepted <= 0 {
					out.Diagnostics = append(out.Diagnostics, diagnostic("run.invalid-report", config.at.Path, "the "+config.name+" runner reported success without an effective execution for "+result.Id, "execute at least one valid input before reporting a check as passed"))
				}
				already, selected := expected[result.Id]
				if !selected || already {
					out.Diagnostics = append(out.Diagnostics, diagnostic("run.invalid-report", config.at.Path, "the "+config.name+" runner returned an unselected or duplicate result "+result.Id, "make the runner return exactly one result per selected check"))
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
				out.Results = append(out.Results, result)
			}
			for _, id := range selected[config.name] {
				if !expected[id] {
					out.Diagnostics = append(out.Diagnostics, diagnostic("run.invalid-report", config.at.Path, "the "+config.name+" runner omitted the selected check "+id, "make the runner return exactly one result per selected check"))
				}
			}
			out.Diagnostics = append(out.Diagnostics, report.Diagnostics...)
		}
	}
	slices.SortFunc(out.Results, func(a, b lang.RunResult) int { return strings.Compare(a.Id, b.Id) })
	sortDiagnostics(out.Diagnostics)
	if d := save(root, out); d != nil {
		out.Diagnostics = append(out.Diagnostics, *d)
		sortDiagnostics(out.Diagnostics)
	}
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
	return false
}

type languageRoots struct {
	name  string
	roots []string
	at    diag.Position
}

func languages(g *graph.Graph) []languageRoots {
	var out []languageRoots
	for _, n := range g.Nodes() {
		if n.Layer != graph.LayerDesign || n.Kind != graph.NodeKindContainer || n.Form != "system" {
			continue
		}
		for _, entry := range n.Attrs["roots"] {
			name, root, _ := strings.Cut(entry, ": ")
			i := slices.IndexFunc(out, func(c languageRoots) bool { return c.name == name })
			if i < 0 {
				out = append(out, languageRoots{name: name, at: n.Origin})
				i = len(out) - 1
			}
			out[i].roots = append(out[i].roots, root)
		}
	}
	return out
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

func save(root string, report lang.RunReport) *diag.Diagnostic {
	fail := func(err error) *diag.Diagnostic {
		d := diagnostic("run.results-write-failed", ".unicode/results.json", fmt.Sprintf("cannot save the execution report: %v", err), "make .unicode a writable directory without symbolic links")
		return &d
	}
	dir := filepath.Join(root, ".unicode")
	for _, p := range []string{root, dir, filepath.Join(dir, "results.json")} {
		info, err := os.Lstat(p)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fail(err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fail(fmt.Errorf("%s is a symbolic link", p))
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fail(err)
	}
	content, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fail(err)
	}
	if err := renameio.WriteFile(filepath.Join(dir, "results.json"), append(content, '\n'), 0o644); err != nil {
		return fail(err)
	}
	return nil
}
