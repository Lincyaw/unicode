package design

import (
	"slices"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/syntax"
)

// LoadSources parses every source and loads the trees that parse. The
// diagnostics are those of parsing each source and those of the load, in
// the order of compareDiagnostics, so they do not depend on the order of
// the sources.
//
//uc:realizes design.loadSources
func LoadSources(sources []syntax.Source) Loaded {
	files, diags := parseSources(sources)
	loaded := Load(files)
	loaded.Diagnostics = slices.SortedFunc(slices.Values(append(diags, loaded.Diagnostics...)), compareDiagnostics)
	return loaded
}

// LoadSourcesForExploration parses every source like LoadSources and
// loads the trees that parse with LoadForExploration.
//
//uc:realizes design.loadSourcesForExploration
func LoadSourcesForExploration(sources []syntax.Source) Explored {
	files, diags := parseSources(sources)
	explored := LoadForExploration(files)
	explored.Loaded.Diagnostics = slices.SortedFunc(slices.Values(append(diags, explored.Loaded.Diagnostics...)), compareDiagnostics)
	return explored
}

// parseSources parses each source and returns the trees of those that parse
// and the diagnostics of those that do not.
func parseSources(sources []syntax.Source) ([]syntax.File, []diag.Diagnostic) {
	var files []syntax.File
	var diags []diag.Diagnostic
	for _, s := range sources {
		parsed := syntax.Parse(s.Path, s.Kind, s.Text)
		diags = append(diags, parsed.Diagnostics...)
		if parsed.File != nil {
			files = append(files, *parsed.File)
		}
	}
	return files, diags
}
