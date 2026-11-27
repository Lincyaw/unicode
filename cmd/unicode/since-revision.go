package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/repo"
)

// sinceRevisionGraph keeps the baseline's build diagnostics so incremental
// selection cannot omit checks because it read an incomplete graph.
func sinceRevisionGraph(root, rev string) (g graph.Graph, diagnostics []diag.Diagnostic, err error) {
	commit, err := repo.ReadRevision(root, rev)
	if err != nil {
		return graph.Graph{}, nil, err
	}
	dir, err := checkoutRev(root, commit.Commit)
	if err != nil {
		return graph.Graph{}, nil, err
	}
	defer func() {
		if cleanup := os.RemoveAll(dir); cleanup != nil {
			err = &exitError{code: exitUnexpected, err: fmt.Errorf("cannot remove the temporary checkout of %s: %w", rev, errors.Join(err, cleanup))}
		}
	}()
	return buildGraph(dir)
}

// checksSince validates explicit targets before intersecting them with the
// checks affected by the baseline and current graphs.
func checksSince(root, rev string, head *graph.Graph, ids []string) ([]string, []diag.Diagnostic, error) {
	var diagnostics []diag.Diagnostic
	for _, id := range ids {
		n := head.Node(id)
		if n == nil || n.Layer != graph.LayerDesign || (n.Kind != graph.NodeKindProperty && n.Kind != graph.NodeKindFlow) {
			diagnostics = append(diagnostics, diag.Diagnostic{
				Code: "run.unknown-check", Subject: "design/system.uc",
				Position: diag.Position{Path: "design/system.uc", Line: 1, Column: 1},
				Message:  "the target " + id + " is not a property or flow",
				Fix:      "select an existing property or flow identity",
			})
		}
	}
	if len(diagnostics) > 0 {
		return nil, diagnostics, nil
	}
	base, diagnostics, err := sinceRevisionGraph(root, rev)
	if err != nil || len(diagnostics) > 0 {
		return nil, diagnostics, err
	}
	return AffectedChecks(&base, head, ids), nil, nil
}
