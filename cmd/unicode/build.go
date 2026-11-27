package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
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
			"The same repository always gives the same bytes. Beside it, build.json records the commit the repository is\n" +
			"at and whether its working tree differs from it: {\"commit\": null, \"dirty\": true} outside a Git work tree\n" +
			"with a commit. The two files build writes do not count as changes. When building gives diagnostics, build\n" +
			"reports them and writes nothing.",
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
			return runBuild(root, dir, g.format, stdout, stderr)
		},
	}
	c.Flags().StringVar(&out, "out", "", "the directory to write graph.json and build.json to (default: .unicode under the root)")
	return c
}

// The files that build writes to its directory.
const (
	snapshotFile = "graph.json"
	headFile     = "build.json"
)

// buildHead is build.json: the commit the repository is at, nil outside a
// Git work tree with a commit, and whether the working tree differs from it,
// which it always does when there is no commit.
type buildHead struct {
	Commit *string `json:"commit"`
	Dirty  bool    `json:"dirty"`
}

// readHead reads the head record of the repository at root, whose build
// writes to the directory out.
func readHead(root, out string) (buildHead, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return buildHead{Dirty: true}, nil
	}
	commit, err := git(root, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	if err != nil {
		return buildHead{Dirty: true}, nil
	}
	dirty, err := isDirty(root, out)
	return buildHead{Commit: &commit, Dirty: dirty}, err
}

// buildReport is the JSON of build: the diagnostics of building the graph,
// and the path of the snapshot written, "" when there are diagnostics.
type buildReport struct {
	Diagnostics []diagnosticJSON `json:"diagnostics"`
	Snapshot    string           `json:"snapshot"`
}

// runBuild reads the head, builds the graph and, when that gives no
// diagnostics, writes its snapshot and the head record to the directory out,
// each atomically. In text mode standard output gives the snapshot's path.
func runBuild(root, out, format string, stdout, stderr io.Writer) error {
	head, err := readHead(root, out)
	if err != nil {
		return err
	}
	g, diags, err := buildGraph(root)
	if err != nil {
		return err
	}
	sortDiagnostics(diags)
	report := buildReport{Diagnostics: diagnosticsJSON(diags)}
	var text []byte
	if len(diags) == 0 {
		record, err := encodeJSON(head)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(out, 0o755); err != nil {
			return &exitError{code: exitUnexpected, err: fmt.Errorf("cannot make the directory of the snapshot: %w", err)}
		}
		path := filepath.Join(out, snapshotFile)
		if err := renameio.WriteFile(path, g.Encode(), 0o644); err != nil {
			return &exitError{code: exitUnexpected, err: fmt.Errorf("cannot write the snapshot: %w", err)}
		}
		if err := renameio.WriteFile(filepath.Join(out, headFile), record, 0o644); err != nil {
			return &exitError{code: exitUnexpected, err: fmt.Errorf("cannot write the head record: %w", err)}
		}
		report.Snapshot = path
		text = fmt.Appendln(nil, path)
	}
	return writeReport(format, report, text, diags, stdout, stderr)
}
