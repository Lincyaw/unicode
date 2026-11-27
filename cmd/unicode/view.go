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

// viewCommand is `unicode view <kind> [<id>] [--expand ID]... [--layer L] [--level L]`.
func viewCommand(g *globalOptions, stdout io.Writer) *cobra.Command {
	var expand []string
	layer, level := string(view.ClassLayerDesign), string(view.C4LevelContainer)
	c := &cobra.Command{
		Use:   "view <kind> [<id>]",
		Short: "Give the data of a view: the nodes and edges it draws, folded (docs/design.md §7.1-§7.2)",
		Long: "view selects the subgraph of the view kind for the node id and folds it. The kinds are architecture,\n" +
			"class, sequence, activity, events, calls, property, code, ddd and c4; architecture, events, ddd and c4\n" +
			"take the whole system when no id is given. --expand opens a folded container, a salient function, or a\n" +
			"reaches edge written \"reaches <from> <to>\". --layer picks the class view's layer, design or code;\n" +
			"--level picks the C4 view's level, context or container.",
		Args:        cobra.RangeArgs(1, 2),
		Annotations: queryAnnotations(),
		RunE: func(cmd *cobra.Command, args []string) error {
			kind, id := view.ViewKind(args[0]), ""
			if len(args) == 2 {
				id = args[1]
			}
			if err := viewOptions(cmd, kind, layer, level); err != nil {
				return err
			}
			return withGraph(g, func(_ string, gr *graph.Graph) error {
				if err := requireNodes(gr, viewIdentities(id, expand)...); err != nil {
					return err
				}
				v, err := view.Select(gr, kind, id, expand, view.ClassLayer(layer), view.C4Level(level))
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
	c.Flags().StringVar(&layer, "layer", layer, "the class view's layer: design or code")
	c.Flags().StringVar(&level, "level", level, "the C4 view's level: context or container")
	return c
}

// viewOptions refuses a --layer or a --level that the view kind does not
// read, or that names no layer or level.
func viewOptions(cmd *cobra.Command, kind view.ViewKind, layer, level string) error {
	switch {
	case cmd.Flags().Changed("layer") && kind != view.ViewKindClass:
		return usageError("--layer picks the layer of the class view, not of the %s view; leave it out", kind)
	case cmd.Flags().Changed("level") && kind != view.ViewKindC4:
		return usageError("--level picks the level of the C4 view, not of the %s view; leave it out", kind)
	case layer != string(view.ClassLayerDesign) && layer != string(view.ClassLayerCode):
		return usageError("--layer %s is not a layer; give design or code", layer)
	case level != string(view.C4LevelContext) && level != string(view.C4LevelContainer):
		return usageError("--level %s is not a level; give context or container", level)
	}
	return nil
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
		id = "the system, " + v.Input.Id
	}
	fmt.Fprintf(&b, "%s view of %s\n", v.Kind, id)
	if v.Layer != nil {
		fmt.Fprintf(&b, "layer: %s\n", *v.Layer)
	}
	if v.Level != nil {
		fmt.Fprintf(&b, "level: %s\n", *v.Level)
	}
	section(&b, "ancestors", nodeLines(v.Ancestors))
	section(&b, "nodes", viewNodeLines(v.Nodes, ""))
	edges := make([]string, len(v.Edges))
	for i, e := range v.Edges {
		edges[i] = edgeText(e.Edge)
		if e.Count > 0 {
			edges[i] += fmt.Sprintf(" (stands for %d edges)", e.Count)
		}
		if len(e.Labels) > 0 {
			edges[i] += ": " + strings.Join(e.Labels, ", ")
		}
	}
	section(&b, "edges", edges)
	section(&b, "folded", v.Folded)
	if v.Hidden > 0 {
		fmt.Fprintf(&b, "%d more utility and infrastructure containers are not drawn\n", v.Hidden)
	}
	return []byte(b.String())
}

// viewNodeLines writes each node on a line, then indented below it its
// members, the nodes of its contents and its count of undrawn detail functions.
func viewNodeLines(nodes []view.ViewNode, indent string) []string {
	var lines []string
	for _, n := range nodes {
		lines = append(lines, indent+nodeText(n.Node))
		for _, m := range n.Members {
			line := fmt.Sprintf("%s  %s %s", indent, m.Kind, m.Name)
			if m.Type != "" {
				line += ": " + m.Type
			}
			if m.CodeType != "" && m.CodeType != m.Type {
				line += ", in the code " + m.CodeType
			}
			if m.Status != nil {
				line += " (" + string(*m.Status) + ")"
			}
			lines = append(lines, line)
		}
		lines = append(lines, viewNodeLines(n.Contents, indent+"  ")...)
		if n.Details > 0 {
			lines = append(lines, fmt.Sprintf("%s  %d detail functions not drawn", indent, n.Details))
		}
	}
	return lines
}
