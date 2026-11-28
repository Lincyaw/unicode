package main

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/evidence"
	"github.com/lincyaw/unicode/internal/gen"
	"github.com/lincyaw/unicode/internal/lang"
	"github.com/lincyaw/unicode/internal/run"
)

// testCommand is `unicode test [id...] [--since REV] [--traffic] [--explore DURATION] [--record]`.
func testCommand(g *globalOptions, stdout, stderr io.Writer) *cobra.Command {
	var options testOptions
	c := &cobra.Command{
		Use:   "test [id...]",
		Short: "Replay the declared and recorded cases of properties and flows",
		Long: "test builds the graph and runs the generated drivers for the selected properties and flows,\n" +
			"or all executable properties when no identities are given. It records the results in\n" +
			".unicode/results.json. Without --explore it only replays declared and recorded cases;\n" +
			"--explore gives a positive time budget for generating candidate inputs; --record saves the selected\n" +
			"replayable inputs and refreshes their generated drivers. With --since REV,\n" +
			"only checks affected since that commit are selected; no affected checks leaves prior results intact.\n" +
			"With --traffic, the calls a monitored program collected under .unicode/traffic are replayed as cases\n" +
			"too; a collected call outside a property's domain is counted with the limit or precondition it breaks.\n" +
			"The viewer shows the results once unicode build compiles them into the snapshot.",
		RunE: func(c *cobra.Command, ids []string) error {
			options.ids = ids
			if c.Flags().Changed("since") && options.since == "" {
				return usageError("--since must name a revision")
			}
			if c.Flags().Changed("explore") && options.explore <= 0 {
				return usageError("--explore must be a positive duration")
			}
			if options.record && options.explore <= 0 {
				return usageError("--record requires a positive --explore duration")
			}
			root, err := findRoot(g.root)
			if err != nil {
				return err
			}
			return runTest(root, g.format, options, stdout, stderr)
		},
	}
	c.Flags().StringVar(&options.since, "since", "", "run only checks affected by changes since this commit")
	c.Flags().DurationVar(&options.explore, "explore", 0, "generate candidate inputs within this time budget instead of only replaying cases")
	c.Flags().BoolVar(&options.record, "record", false, "save selected replayable inputs and refresh generated drivers (requires --explore)")
	c.Flags().BoolVar(&options.traffic, "traffic", false, "also replay the calls a monitored program collected under .unicode/traffic")
	return c
}

// testOptions are unicode test's arguments: the selected identities, the
// exploration budget, the base of --since, and whether to record and to
// replay collected calls.
type testOptions struct {
	ids     []string
	explore time.Duration
	since   string
	record  bool
	traffic bool
}

// runTest keeps predicate failures separate from toolchain diagnostics.
func runTest(root, format string, options testOptions, stdout, stderr io.Writer) error {
	ids, explore, since, record, traffic := options.ids, options.explore, options.since, options.record, options.traffic
	builder := buildInputGraph
	if explore > 0 {
		builder = buildGraphForExploration
	}
	g, diags, err := builder(root)
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
	var inputs map[string][]lang.Input
	if traffic && len(diags) == 0 {
		collected := evidence.ReadTraffic(root, &g)
		diags = append(diags, collected.Diagnostics...)
		inputs = evidence.TrafficInputs(&g, collected.Collected)
		// The version differential of --since (docs/design.md §6.5) takes
		// collected.Collected as its inputs too, aligned by operation and input
		// hash; it is to be called here once unicode test --since has it.
	}
	report := lang.RunReport{Results: []lang.RunResult{}, Diagnostics: []diag.Diagnostic{}}
	if len(diags) == 0 && (since == "" || len(selected) > 0) {
		report = run.Execute(lang.RunPlan{Root: root, Graph: &g, Ids: selected, Explore: explore, Record: record, Traffic: inputs}, availableBackends())
		if record && len(report.Diagnostics) == 0 {
			var cases []lang.CaseResult
			for _, result := range report.Results {
				cases = append(cases, result.Cases...)
			}
			recording := evidence.Recordings(root, &g, cases)
			report.Diagnostics = append(report.Diagnostics, recording.Diagnostics...)
			if len(report.Diagnostics) == 0 {
				report.Diagnostics = append(report.Diagnostics, evidence.ApplyRecordings(root, recording)...)
			}
			if len(report.Diagnostics) == 0 {
				report.Diagnostics = append(report.Diagnostics, refreshRecordedDrivers(root)...)
			}
			if len(report.Diagnostics) > 0 {
				report.Diagnostics = append(report.Diagnostics, evidence.Save(root, report)...)
			}
		}
		diags = append(diags, report.Diagnostics...)
	}
	sortDiagnostics(diags)
	report.Diagnostics = append([]diag.Diagnostic{}, diags...)
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
	}
	allDiagnostics := append(failures, diags...)
	sortDiagnostics(allDiagnostics)
	if err := writeResult(format, report, text, stdout); err != nil {
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

// refreshRecordedDrivers uses the ordinary generation safety checks after corpus publication.
func refreshRecordedDrivers(root string) []diag.Diagnostic {
	g, diagnostics, err := buildGraphForGeneration(root)
	if err != nil {
		return append(diagnostics, diag.Diagnostic{Code: "cli.record-generation-failed", Subject: "design/system.uc", Position: diag.Position{Path: "design/system.uc", Line: 1, Column: 1}, Message: err.Error(), Fix: "repair the generation environment and run unicode gen before replaying the recorded inputs"})
	}
	if len(diagnostics) > 0 {
		return diagnostics
	}
	plan := gen.Plan(root, &g, availableBackends())
	if len(plan.Diagnostics) > 0 {
		return plan.Diagnostics
	}
	return gen.Write(root, plan.Edits)
}
