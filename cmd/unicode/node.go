package main

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/query"
)

// nodeKinds are the kinds that node search --kind takes (docs/model.md §3).
var nodeKinds = []graph.NodeKind{graph.NodeKindContainer, graph.NodeKindType, graph.NodeKindOp, graph.NodeKindActor,
	graph.NodeKindFlow, graph.NodeKindStep, graph.NodeKindProperty, graph.NodeKindObservation, graph.NodeKindCase, graph.NodeKindRun,
	graph.NodeKindResult, graph.NodeKindReview}

// nodeCommand is `unicode node get|search|refs`.
func nodeCommand(g *globalOptions, stdout io.Writer) *cobra.Command {
	c := &cobra.Command{
		Use:   "node",
		Short: "Look up nodes of the graph: one with its edges, a search, or the edges that reach one",
		Args:  cobra.NoArgs,
		RunE:  func(*cobra.Command, []string) error { return usageError("missing subcommand: get, search or refs") },
	}
	c.AddCommand(nodeGetCommand(g, stdout), nodeSearchCommand(g, stdout), nodeRefsCommand(g, stdout))
	return c
}

func nodeGetCommand(g *globalOptions, stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:         "get <id>",
		Short:       "Give a node and the edges that leave it and reach it",
		Args:        cobra.ExactArgs(1),
		Annotations: queryAnnotations(),
		RunE: func(_ *cobra.Command, args []string) error {
			return withGraph(g, func(_ string, gr *graph.Graph) error {
				v, err := query.Get(gr, args[0])
				if err != nil {
					return queryError(err)
				}
				var b strings.Builder
				fmt.Fprintf(&b, "%s\n%s", nodeText(v.Node), attrsText(v.Node.Attrs, "  "))
				section(&b, "outgoing", edgeLines(v.Outgoing))
				section(&b, "incoming", edgeLines(v.Incoming))
				return writeResult(g.format, v, []byte(b.String()), stdout)
			})
		},
	}
}

func nodeSearchCommand(g *globalOptions, stdout io.Writer) *cobra.Command {
	kind := ""
	c := &cobra.Command{
		Use:         "search <text>",
		Short:       "Give the nodes whose identity or summary contains the text, sorted by identity",
		Args:        cobra.ExactArgs(1),
		Annotations: queryAnnotations(),
		RunE: func(_ *cobra.Command, args []string) error {
			var of *graph.NodeKind
			if kind != "" {
				k := graph.NodeKind(kind)
				if !slices.Contains(nodeKinds, k) {
					return usageError("--kind %s is not a node kind; the kinds are %s", kind, kindList())
				}
				of = &k
			}
			return withGraph(g, func(_ string, gr *graph.Graph) error {
				found := query.Search(gr, args[0], of)
				return writeResult(g.format, found, []byte(strings.Join(append(nodeLines(found), ""), "\n")), stdout)
			})
		},
	}
	c.Flags().StringVar(&kind, "kind", "", "give only nodes of this kind: "+kindList())
	return c
}

func nodeRefsCommand(g *globalOptions, stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:         "refs <id>",
		Short:       "Give the edges that reach a node",
		Args:        cobra.ExactArgs(1),
		Annotations: queryAnnotations(),
		RunE: func(_ *cobra.Command, args []string) error {
			return withGraph(g, func(_ string, gr *graph.Graph) error {
				refs, err := query.Refs(gr, args[0])
				if err != nil {
					return queryError(err)
				}
				return writeResult(g.format, refs, []byte(strings.Join(append(edgeLines(refs), ""), "\n")), stdout)
			})
		},
	}
}

func kindList() string {
	names := make([]string, len(nodeKinds))
	for i, k := range nodeKinds {
		names[i] = string(k)
	}
	return strings.Join(names, ", ")
}
