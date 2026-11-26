package main

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/view"
)

// viewCommand is `unicode view <kind> [<id>] [--expand ID]...`.
func viewCommand(g *globalOptions, stdout io.Writer) *cobra.Command {
	var expand []string
	c := &cobra.Command{
		Use:   "view <kind> [<id>]",
		Short: "Give the data of a view: the nodes and edges it draws, folded (docs/design.md §7.1-§7.2)",
		Long: "view selects the subgraph of the view kind for the node id and folds it. The kinds are architecture,\n" +
			"class, sequence, activity, events, calls, property, code, ddd and c4; architecture and events take the whole\n" +
			"system when no id is given. --expand opens a folded container, a salient function, or a reaches edge\n" +
			"written \"reaches <from> <to>\".",
		Args:        cobra.RangeArgs(1, 2),
		Annotations: queryAnnotations(),
		RunE: func(_ *cobra.Command, args []string) error {
			kind, id := view.ViewKind(args[0]), ""
			if len(args) == 2 {
				id = args[1]
			}
			return withGraph(g, func(_ string, gr *graph.Graph) error {
				if err := requireNodes(gr, viewIdentities(id, expand)...); err != nil {
					return err
				}
				v, err := view.Select(gr, kind, id, expand)
				var na view.NotApplicable
				switch {
				case errors.As(err, &na):
					return usageError("%v", err)
				case err != nil:
					return &exitError{code: exitUnexpected, err: err}
				}
				return writeResult(g.format, v, viewText(v, id), stdout)
			})
		},
	}
	c.Flags().StringArrayVar(&expand, "expand", nil, "expand this container, function or reaches edge (repeatable)")
	return c
}

// viewIdentities are the node identities that a view request names: its
// input, when given, and each expanded node or each end of an expanded reaches edge.
func viewIdentities(id string, expand []string) []string {
	var ids []string
	if id != "" {
		ids = append(ids, id)
	}
	for _, x := range expand {
		if rest, ok := strings.CutPrefix(x, "reaches "); ok {
			ids = append(ids, strings.Fields(rest)...)
		} else {
			ids = append(ids, x)
		}
	}
	return ids
}

func viewText(v view.View, id string) []byte {
	var b strings.Builder
	if id == "" {
		id = "the system"
	}
	fmt.Fprintf(&b, "%s view of %s\n", v.Kind, id)
	section(&b, "nodes", nodeLines(v.Nodes))
	section(&b, "edges", edgeLines(v.Edges))
	section(&b, "folded", v.Folded)
	if v.Hidden > 0 {
		fmt.Fprintf(&b, "%d more utility and infrastructure containers are not drawn\n", v.Hidden)
	}
	return []byte(b.String())
}
