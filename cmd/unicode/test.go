package main

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/lang"
	"github.com/lincyaw/unicode/internal/run"
)

// testCommand is `unicode test [id...] [--since REV] [--explore DURATION]`.
func testCommand(g *globalOptions, stdout, stderr io.Writer) *cobra.Command {
	var explore time.Duration
	var since string
	c := &cobra.Command{
		Use:   "test [id...]",
		Short: "Replay the declared and recorded cases of properties and flows",
		Long: "test builds the graph and runs the generated drivers for the selected properties and flows,\n" +
			"or all executable properties when no identities are given. It records the results in\n" +
			".unicode/results.json. Without --explore it only replays declared and recorded cases;\n" +
			"--explore gives a positive time budget for generating candidate inputs. With --since REV,\n" +
			"only checks affected since that commit are selected; no affected checks leaves prior results intact.",
		RunE: func(c *cobra.Command, ids []string) error {
			if c.Flags().Changed("since") && since == "" {
				return usageError("--since must name a revision")
			}
			if c.Flags().Changed("explore") && explore <= 0 {
				return usageError("--explore must be a positive duration")
			}
			root, err := findRoot(g.root)
			if err != nil {
				return err
			}
			return runTest(root, g.format, ids, explore, since, stdout, stderr)
		},
	}
	c.Flags().StringVar(&since, "since", "", "run only checks affected by changes since this commit")
	c.Flags().DurationVar(&explore, "explore", 0, "generate candidate inputs within this time budget instead of only replaying cases")
	return c
}

type runResultJSON struct {
	ID           string            `json:"id"`
	Passed       bool              `json:"passed"`
	Accepted     int               `json:"accepted"`
	Rejected     int               `json:"rejected"`
	Observations []string          `json:"observations"`
	Differences  []string          `json:"differences"`
	Diagnostics  []diag.Diagnostic `json:"diagnostics"`
}

type runReportJSON struct {
	Results     []runResultJSON   `json:"results"`
	Diagnostics []diag.Diagnostic `json:"diagnostics"`
}

// runTest keeps predicate failures separate from toolchain diagnostics.
func runTest(root, format string, ids []string, explore time.Duration, since string, stdout, stderr io.Writer) error {
	g, diags, err := buildGraph(root)
	if err != nil {
		return err
	}
	selected := ids
	if len(diags) == 0 && since != "" {
		var selectionDiagnostics []diag.Diagnostic
		selected, selectionDiagnostics, err = checksSince(root, since, &g, ids)
		if err != nil {
			return err
		}
		diags = append(diags, selectionDiagnostics...)
	}
	report := lang.RunReport{}
	if len(diags) == 0 && (since == "" || len(selected) > 0) {
		report = run.Execute(root, &g, availableBackends(), selected, explore)
		diags = append(diags, report.Diagnostics...)
	}
	sortDiagnostics(diags)
	out := runReportJSON{Results: []runResultJSON{}, Diagnostics: append([]diag.Diagnostic{}, diags...)}
	failed := false
	var text []byte
	var failures []diag.Diagnostic
	for _, result := range report.Results {
		status := "passed"
		if !result.Passed {
			status, failed = "failed", true
		}
		text = fmt.Appendf(text, "%s %s accepted=%d rejected=%d\n", result.Id, status, result.Accepted, result.Rejected)
		for _, observation := range result.Observations {
			text = fmt.Appendf(text, "  observation: %s\n", observation)
		}
		for _, difference := range result.Differences {
			text = fmt.Appendf(text, "  difference: %s\n", difference)
		}
		failures = append(failures, result.Diagnostics...)
		out.Results = append(out.Results, runResultJSON{
			ID: result.Id, Passed: result.Passed, Accepted: result.Accepted, Rejected: result.Rejected,
			Observations: nonNilStrings(result.Observations), Differences: nonNilStrings(result.Differences),
			Diagnostics: append([]diag.Diagnostic{}, result.Diagnostics...),
		})
	}
	allDiagnostics := append(failures, diags...)
	sortDiagnostics(allDiagnostics)
	if err := writeResult(format, out, text, stdout); err != nil {
		return err
	}
	if format != "json" {
		if _, err := stderr.Write(diagnosticsText(allDiagnostics)); err != nil {
			return &exitError{code: exitUnexpected}
		}
	}
	if len(diags) > 0 {
		return &exitError{code: exitDiagnostics}
	}
	if failed {
		return &exitError{code: exitFailed}
	}
	return nil
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
