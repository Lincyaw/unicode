package check

import (
	"fmt"
	"strings"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
)

func list(diags []diag.Diagnostic) string {
	var b strings.Builder
	for _, d := range diags {
		fmt.Fprintf(&b, "  %s %s at %s:%d: %s (fix: %s)\n", d.Code, d.Subject, d.Position.Path, d.Position.Line, d.Message, d.Fix)
	}
	return b.String()
}

// declaredItems lists the keys of every declared node and edge and every depends.
func declaredItems(g *graph.Graph) []string {
	var out []string
	for _, n := range g.Nodes() {
		if n.Source == graph.SourceDeclared {
			out = append(out, n.Id)
		}
	}
	for _, e := range g.Edges() {
		if e.Source == graph.SourceDeclared || e.Kind == graph.EdgeKindDepends {
			out = append(out, edgeID(e))
		}
	}
	return out
}
