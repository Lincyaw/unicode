package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/google/renameio/v2"
	"github.com/spf13/cobra"
)

// buildCommand is `unicode build [--out DIR]`.
func buildCommand(g *globalOptions, stdout, stderr io.Writer) *cobra.Command {
	out := ""
	c := &cobra.Command{
		Use:   "build",
		Short: "Build the graph and write its snapshot to .unicode/graph.json",
		Long: "build builds the graph as check does, without checking the constraints, and writes its snapshot\n" +
			"(docs/model.md §8) to .unicode/graph.json under the root, or to graph.json in the directory that --out names.\n" +
			"The same repository always gives the same bytes. When building gives diagnostics, build reports them and\n" +
			"writes nothing.",
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			root, err := findRoot(g.root)
			if err != nil {
				return err
			}
			dir := out
			if dir == "" {
				dir = filepath.Join(root, ".unicode")
			}
			return runBuild(root, filepath.Join(dir, "graph.json"), g.format, stdout, stderr)
		},
	}
	c.Flags().StringVar(&out, "out", "", "the directory to write graph.json to (default: .unicode under the root)")
	return c
}

// buildReport is the JSON of build: the diagnostics of building the graph,
// and the path of the snapshot written, "" when there are diagnostics.
type buildReport struct {
	Diagnostics []diagnosticJSON `json:"diagnostics"`
	Snapshot    string           `json:"snapshot"`
}

// runBuild builds the graph and, when that gives no diagnostics, writes its
// snapshot to path atomically. In text mode standard output gives the path.
func runBuild(root, path, format string, stdout, stderr io.Writer) error {
	g, diags, err := buildGraph(root)
	if err != nil {
		return err
	}
	sortDiagnostics(diags)
	report := buildReport{Diagnostics: diagnosticsJSON(diags)}
	var text []byte
	if len(diags) == 0 {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return &exitError{code: exitUnexpected, err: fmt.Errorf("cannot make the directory of the snapshot: %w", err)}
		}
		if err := renameio.WriteFile(path, g.Encode(), 0o644); err != nil {
			return &exitError{code: exitUnexpected, err: fmt.Errorf("cannot write the snapshot: %w", err)}
		}
		report.Snapshot = path
		text = fmt.Appendln(nil, path)
	}
	return writeReport(format, report, text, diags, stdout, stderr)
}
