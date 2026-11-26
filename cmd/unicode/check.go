package main

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"slices"

	"github.com/spf13/cobra"

	"github.com/lincyaw/unicode/internal/check"
	"github.com/lincyaw/unicode/internal/design"
	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
)

// checkCommand is `unicode check [--design-only]`.
func checkCommand(g *globalOptions, stdout, stderr io.Writer) *cobra.Command {
	designOnly := false
	c := &cobra.Command{
		Use:   "check",
		Short: "Build the graph, check every constraint and report every diagnostic",
		Long: "check loads the design and corpus files, extracts the code of every language that design/system.uc\n" +
			"configures, merges and derives the graph, and reports every violation of a constraint (docs/model.md §10):\n" +
			"syntax errors, files that are not in the canonical format, and the diagnostics of the design, the code and\n" +
			"the bindings between them. With --design-only it reads only the design and corpus files and reports the\n" +
			"constraints that need only the design (docs/design.md §5.1).",
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			root, err := findRoot(g.root)
			if err != nil {
				return err
			}
			if designOnly {
				return runCheckDesign(root, g.format, stdout, stderr)
			}
			return runCheckCode(root, g.format, stdout, stderr)
		},
	}
	c.Flags().BoolVar(&designOnly, "design-only", false, "check only the design and corpus files, not the code")
	return c
}

type checkReport struct {
	Diagnostics []diagnosticJSON `json:"diagnostics"`
}

// codeReport is the JSON of check without --design-only: the diagnostics,
// and the status of every declared item keyed by its identity, or by
// `kind from to` for an edge.
type codeReport struct {
	Diagnostics []diagnosticJSON        `json:"diagnostics"`
	Statuses    map[string]graph.Status `json:"statuses"`
}

// runCheckDesign reports the diagnostics of every file under design/ and
// corpus/: those of reading and parsing it, cli.not-canonical when it parses
// but is not canonical, and those of loading every file that parses into the design.
func runCheckDesign(root, format string, stdout, stderr io.Writer) error {
	files, diags, err := readDesign(root)
	if err != nil {
		return err
	}
	diags = append(diags, design.Load(files).Diagnostics...)
	sortDiagnostics(diags)
	return writeReport(format, checkReport{Diagnostics: diagnosticsJSON(diags)}, nil, diags, stdout, stderr)
}

// runCheckCode builds the graph, checks it with check.Conform and reports the
// diagnostics of building and checking it, with the statuses.
func runCheckCode(root, format string, stdout, stderr io.Writer) error {
	g, diags, err := buildGraph(root)
	if err != nil {
		return err
	}
	checked := check.Conform(&g)
	diags = append(diags, checked.Diagnostics...)
	sortDiagnostics(diags)
	return writeReport(format, codeReport{Diagnostics: diagnosticsJSON(diags), Statuses: checked.Statuses}, nil, diags, stdout, stderr)
}

// writeReport writes a command's report: with --format json, the report on
// standard output; otherwise text on standard output and one line per
// diagnostic on standard error. It ends the command with exitDiagnostics
// when there are diagnostics.
func writeReport(format string, report any, text []byte, diags []diag.Diagnostic, stdout, stderr io.Writer) error {
	out, errOut := text, diagnosticsText(diags)
	if format == "json" {
		var err error
		if out, err = encodeJSON(report); err != nil {
			return err
		}
		errOut = nil
	}
	if _, err := stdout.Write(out); err != nil {
		return &exitError{code: exitUnexpected}
	}
	if _, err := stderr.Write(errOut); err != nil {
		return &exitError{code: exitUnexpected}
	}
	if len(diags) > 0 {
		return &exitError{code: exitDiagnostics}
	}
	return nil
}

// encodeJSON writes v indented by two spaces, with a final newline.
func encodeJSON(v any) ([]byte, error) {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, &exitError{code: exitUnexpected, err: fmt.Errorf("cannot encode the JSON output: %w", err)}
	}
	return append(out, '\n'), nil
}

// sortDiagnostics orders diagnostics by file, line, column, code and message.
func sortDiagnostics(diags []diag.Diagnostic) {
	slices.SortStableFunc(diags, func(a, b diag.Diagnostic) int {
		return cmp.Or(cmp.Compare(a.Position.Path, b.Position.Path), cmp.Compare(a.Position.Line, b.Position.Line),
			cmp.Compare(a.Position.Column, b.Position.Column), cmp.Compare(a.Code, b.Code), cmp.Compare(a.Message, b.Message))
	})
}
