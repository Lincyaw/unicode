package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/query"
)

// impactCommand is `unicode impact <id>...`.
func impactCommand(g *globalOptions, stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "impact <id>...",
		Short: "Give the salient nodes that a change to the nodes affects, each with a shortest path that says why",
		Long: "impact gives the salient nodes affected by changes. Changes spread backwards along calls, reaches,\n" +
			"refers, subscribes, emits, realizes, traces, exercises, depends, about, checks and observes.\n" +
			"An adapter change also reaches its observation, and an observation change reaches its owning property\n" +
			"(docs/design.md §7.4). Results are grouped by kind, each with one shortest connected path of original edges.",
		Args:        cobra.MinimumNArgs(1),
		Annotations: queryAnnotations(),
		RunE: func(_ *cobra.Command, ids []string) error {
			return withGraph(g, func(_ string, gr *graph.Graph) error {
				affected, err := query.Impact(gr, ids)
				if err != nil {
					return queryError(err)
				}
				return writeResult(g.format, affected, impactText(affected), stdout)
			})
		},
	}
}

func impactText(affected []query.Affected) []byte {
	var b strings.Builder
	var kind graph.NodeKind
	for _, a := range affected {
		if a.Node.Kind != kind {
			kind = a.Node.Kind
			fmt.Fprintf(&b, "%s:\n", kind)
		}
		fmt.Fprintf(&b, "  %s\n    via %s\n", nodeText(a.Node), strings.Join(edgeLines(a.Path), ", "))
	}
	return []byte(b.String())
}
