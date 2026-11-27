package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/lincyaw/unicode/internal/graph"
)

// diffCommand is `unicode diff <rev> [--to REV]`.
func diffCommand(g *globalOptions, stdout io.Writer) *cobra.Command {
	to := ""
	c := &cobra.Command{
		Use:   "diff <rev>",
		Short: "Compare the graph of a commit with the graph of the working tree, or of another commit",
		Long: "diff builds the graph of the commit rev as a query's --rev does, builds the graph of the working tree, or\n" +
			"with --to that of another commit, and aligns their nodes and edges by identity (docs/model.md §8). It lists\n" +
			"the nodes that are added, removed or modified (their content hash differs) and the edges that are added,\n" +
			"removed or modified (their attrs differ), with the commits at both ends. A revision that git cannot read\n" +
			"is a usage error.",
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			root, err := findRoot(g.root)
			if err != nil {
				return err
			}
			return runDiff(root, args[0], to, g.format, stdout)
		},
	}
	c.Flags().StringVar(&to, "to", "", "compare with the graph of this commit instead of the working tree")
	return c
}

// diffReport is the JSON of diff, the comparison of api/compare.tsp: the
// base commit, the head (a commit, or the working tree with whether it has
// changes), and the changed nodes and edges.
type diffReport struct {
	Base revision    `json:"base"`
	Head workingTree `json:"head"`
	graph.Comparison
}

// runDiff compares the graph of the commit rev with the graph of the working
// tree at root, or with to's when it is given.
func runDiff(root, rev, to, format string, stdout io.Writer) error {
	baseGraph, base, err := revisionGraph(root, rev)
	if err != nil {
		return err
	}
	var headGraph graph.Graph
	var head workingTree
	if to != "" {
		headGraph, head.revision, err = revisionGraph(root, to)
	} else if head, err = readWorkingTree(root, filepath.Join(root, ".unicode")); err == nil {
		headGraph, _, err = buildGraph(root)
	}
	if err != nil {
		return err
	}
	report := diffReport{Base: base, Head: head, Comparison: graph.Compare(&baseGraph, &headGraph)}
	return writeResult(format, report, diffText(report, to == ""), stdout)
}

// revisionGraph builds the graph of the commit rev in a checkout of it,
// which it removes afterwards. As for a query, the diagnostics of building
// are check's to report.
func revisionGraph(root, rev string) (graph.Graph, revision, error) {
	r, err := readRevision(root, rev)
	if err != nil {
		return graph.Graph{}, revision{}, err
	}
	dir, err := checkoutRev(root, r.Commit)
	if err != nil {
		return graph.Graph{}, revision{}, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	g, _, err := buildGraph(dir)
	return g, r, err
}

// diffText writes the two ends, then one line per changed node or edge
// under its heading, a modified node followed by a line per changed attrs key.
func diffText(r diffReport, workTree bool) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "base %s %s\n", r.Base.Short, r.Base.Subject)
	switch {
	case !workTree:
		fmt.Fprintf(&b, "head %s %s\n", r.Head.Short, r.Head.Subject)
	case r.Head.Dirty:
		fmt.Fprintf(&b, "head the working tree at %s %s, with changes\n", r.Head.Short, r.Head.Subject)
	default:
		fmt.Fprintf(&b, "head the working tree at %s %s\n", r.Head.Short, r.Head.Subject)
	}
	var nodes []string
	for _, c := range r.Nodes {
		n := c.After
		if n == nil {
			n = c.Before
		}
		nodes = append(nodes, fmt.Sprintf("%s %s", c.Change, nodeText(*n)))
		if c.Change != graph.ChangeKindModified {
			continue
		}
		for _, a := range c.Attrs {
			nodes = append(nodes, fmt.Sprintf("  %s: removed %q, added %q", a.Key, a.Removed, a.Added))
		}
	}
	section(&b, "nodes", nodes)
	var edges []string
	for _, c := range r.Edges {
		edges = append(edges, fmt.Sprintf("%s %s %s -> %s", c.Change, c.Kind, c.From, c.To))
	}
	section(&b, "edges", edges)
	return []byte(b.String())
}
