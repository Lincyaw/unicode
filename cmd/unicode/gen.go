package main

import (
	"fmt"
	"io"
	"maps"
	"slices"

	"github.com/spf13/cobra"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/gen"
	"github.com/lincyaw/unicode/internal/lang"
)

// genCommand is `unicode gen [--dry-run | --check]`.
func genCommand(g *globalOptions, stdout, stderr io.Writer) *cobra.Command {
	dryRun := false
	check := false
	c := &cobra.Command{
		Use:   "gen",
		Short: "Generate predicates and test drivers from the design",
		Long: "gen builds the graph and asks each configured language backend for its generation plan.\n" +
			"It writes the planned files only when building and planning give no diagnostics, and only\n" +
			"replaces files owned by unicode. With --dry-run it reports the plan and writes nothing.\n" +
			"With --check it verifies that all generated files are current and writes nothing.\n" +
			"JSON includes each file's relative path and complete source text; text lists their paths,\n" +
			"except that a successful --check is silent.",
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			root, err := findRoot(g.root)
			if err != nil {
				return err
			}
			return runGen(root, g.format, dryRun, check, stdout, stderr)
		},
	}
	c.Flags().BoolVar(&dryRun, "dry-run", false, "report the generation plan and write nothing")
	c.Flags().BoolVar(&check, "check", false, "check generated files against the plan and write nothing")
	c.MarkFlagsMutuallyExclusive("dry-run", "check")
	return c
}

// availableBackends orders the installed backends by language.
func availableBackends() []lang.Backend {
	available := backends()
	out := make([]lang.Backend, 0, len(available))
	for _, name := range slices.Sorted(maps.Keys(available)) {
		out = append(out, available[name].Backend)
	}
	return out
}

type editJSON struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type generationJSON struct {
	Edits       []editJSON        `json:"edits"`
	Diagnostics []diag.Diagnostic `json:"diagnostics"`
}

// runGen plans before writing, so a diagnostic prevents applying any edit.
func runGen(root, format string, dryRun, check bool, stdout, stderr io.Writer) error {
	g, diags, err := buildGraphForGeneration(root)
	if err != nil {
		return err
	}
	generated := lang.Generation{}
	if len(diags) == 0 {
		generated = gen.Plan(root, &g, availableBackends())
		diags = append(diags, generated.Diagnostics...)
	}
	if len(diags) == 0 {
		switch {
		case check:
			diags = append(diags, gen.CheckFiles(root, generated.Edits)...)
		case !dryRun:
			diags = append(diags, gen.Write(root, generated.Edits)...)
		}
	}
	sortDiagnostics(diags)
	report := generationJSON{Edits: []editJSON{}, Diagnostics: append([]diag.Diagnostic{}, diags...)}
	var text []byte
	for _, edit := range generated.Edits {
		report.Edits = append(report.Edits, editJSON{Path: edit.Path, Content: string(edit.Content)})
		if len(diags) == 0 && !check {
			text = fmt.Appendln(text, edit.Path)
		}
	}
	err = writeReport(format, report, text, diags, stdout, stderr)
	if exit, ok := err.(*exitError); ok && exit.code == exitDiagnostics {
		switch {
		case slices.ContainsFunc(diags, func(d diag.Diagnostic) bool { return d.Code == "gen.read-failed" }):
			exit.code = exitEnvironment
		case slices.ContainsFunc(diags, func(d diag.Diagnostic) bool { return d.Code == "gen.unowned-file" }):
			exit.code = exitConflict
		}
	}
	return err
}
