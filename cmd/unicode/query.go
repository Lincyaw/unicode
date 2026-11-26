package main

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/query"
)

// queryAnnotations mark a command as a query, which reads the graph of the
// working tree or, with --rev, of a commit.
func queryAnnotations() map[string]string { return map[string]string{readsRev: ""} }

// withGraph builds the graph that a query reads and calls read with the root
// it was built from: the working tree's, or with --rev a checkout of that
// commit in a temporary directory, which is removed afterwards. The
// diagnostics of building are check's to report; a query reads the graph
// that the repository builds to.
func withGraph(g *globalOptions, read func(root string, gr *graph.Graph) error) error {
	root, err := findRoot(g.root)
	if err != nil {
		return err
	}
	if g.rev != "" {
		dir, err := checkoutRev(root, g.rev)
		if err != nil {
			return err
		}
		defer func() { _ = os.RemoveAll(dir) }()
		root = dir
	}
	gr, _, err := buildGraph(root)
	if err != nil {
		return err
	}
	return read(root, &gr)
}

// queryError ends a query refused for an identity that is not a node of the
// graph with exitNotFound; any other refusal is unexpected.
func queryError(err error) error {
	if errors.As(err, new(query.NotFound)) {
		return &exitError{code: exitNotFound, err: err}
	}
	return &exitError{code: exitUnexpected, err: err}
}

// requireNodes ends the command with exitNotFound at the first identity
// that is not a node of the graph.
func requireNodes(gr *graph.Graph, ids ...string) error {
	for _, id := range ids {
		if gr.Node(id) == nil {
			return queryError(query.NotFound{Id: id})
		}
	}
	return nil
}

// writeResult writes a query's result as JSON, or else the text.
func writeResult(format string, result any, text []byte, stdout io.Writer) error {
	out := text
	if format == "json" {
		var err error
		if out, err = encodeJSON(result); err != nil {
			return err
		}
	}
	if _, err := stdout.Write(out); err != nil {
		return &exitError{code: exitUnexpected}
	}
	return nil
}

// The text of a query result: a node or an edge on one line.

func nodeText(n graph.Node) string {
	summary := ""
	if n.Summary != "" {
		summary = ": " + n.Summary
	}
	return fmt.Sprintf("%s (%s %s, %s)%s", n.Id, n.Form, n.Kind, at(n.Origin), summary)
}

func edgeText(e graph.Edge) string { return fmt.Sprintf("%s %s -> %s", e.Kind, e.From, e.To) }

func at(p diag.Position) string { return fmt.Sprintf("%s:%d:%d", p.Path, p.Line, p.Column) }

// attrsText writes a node's attrs, one key a line, sorted by key.
func attrsText(attrs map[string][]string, indent string) string {
	var b strings.Builder
	for _, k := range slices.Sorted(maps.Keys(attrs)) {
		fmt.Fprintf(&b, "%s%s: %s\n", indent, k, strings.Join(attrs[k], ", "))
	}
	return b.String()
}

// section writes a heading and one indented line per item, or "none".
func section(b *strings.Builder, heading string, lines []string) {
	fmt.Fprintf(b, "%s:\n", heading)
	if len(lines) == 0 {
		b.WriteString("  none\n")
	}
	for _, l := range lines {
		fmt.Fprintf(b, "  %s\n", l)
	}
}

func nodeLines(nodes []graph.Node) []string {
	lines := make([]string, len(nodes))
	for i, n := range nodes {
		lines[i] = nodeText(n)
	}
	return lines
}

func edgeLines(edges []graph.Edge) []string {
	lines := make([]string, len(edges))
	for i, e := range edges {
		lines[i] = edgeText(e)
	}
	return lines
}
