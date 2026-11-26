package main

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"slices"

	"github.com/spf13/cobra"

	"github.com/lincyaw/unicode/internal/design"
	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/syntax"
)

// checkCommand is `unicode check [--design-only]`.
func checkCommand(g *globalOptions, stdout, stderr io.Writer) *cobra.Command {
	designOnly := false
	c := &cobra.Command{
		Use:   "check",
		Short: "Check the design and corpus files and report every diagnostic; until M2 only with --design-only",
		Long: "check parses every file under design/ and corpus/ and reports its syntax errors, the files that are not\n" +
			"in the canonical format, and every violation of a constraint that needs only the design (docs/design.md §5.1).\n" +
			"Until M2, which checks the code against the design, check runs only with --design-only.",
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			if !designOnly {
				return usageError("check without --design-only checks the code against the design, which comes in M2; run unicode check --design-only")
			}
			root, err := findRoot(g.root)
			if err != nil {
				return err
			}
			return runCheck(root, g.format, stdout, stderr)
		},
	}
	c.Flags().BoolVar(&designOnly, "design-only", false, "check only the design and corpus files, not the code")
	return c
}

type checkReport struct {
	Diagnostics []diagnosticJSON `json:"diagnostics"`
}

// runCheck reports the diagnostics of every file under design/ and corpus/:
// those of reading and parsing it, cli.not-canonical when it parses but is not
// canonical, and those of loading every file that parses into the design.
func runCheck(root, format string, stdout, stderr io.Writer) error {
	paths, err := fmtPaths(root, nil)
	if err != nil {
		return err
	}
	var diags []diag.Diagnostic
	var files []syntax.File
	for _, rel := range paths {
		src, f, ds := readFile(root, rel)
		diags = append(diags, ds...)
		if f == nil {
			continue
		}
		if out := f.Print(); !bytes.Equal(out, src) {
			diags = append(diags, notCanonical(rel, src, out))
		}
		files = append(files, *f)
	}
	diags = append(diags, design.Load(files).Diagnostics...)
	sortDiagnostics(diags)
	var out, errOut []byte
	if format == "json" {
		if out, err = json.MarshalIndent(checkReport{Diagnostics: diagnosticsJSON(diags)}, "", "  "); err != nil {
			return &exitError{code: exitUnexpected, err: fmt.Errorf("cannot encode the JSON report: %w", err)}
		}
		out = append(out, '\n')
	} else {
		errOut = diagnosticsText(diags)
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

// sortDiagnostics orders diagnostics by file, line, column, code and message.
func sortDiagnostics(diags []diag.Diagnostic) {
	slices.SortStableFunc(diags, func(a, b diag.Diagnostic) int {
		return cmp.Or(cmp.Compare(a.Position.Path, b.Position.Path), cmp.Compare(a.Position.Line, b.Position.Line),
			cmp.Compare(a.Position.Column, b.Position.Column), cmp.Compare(a.Code, b.Code), cmp.Compare(a.Message, b.Message))
	})
}
