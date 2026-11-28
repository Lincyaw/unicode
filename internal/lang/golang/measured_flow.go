package golang

import (
	"strings"

	"github.com/lincyaw/unicode/internal/graph"
)

func (c *contractCompiler) measuredFlow(unit *contractUnit) bool {
	if unit.node.Kind != graph.NodeKindFlow || unit.predicate != nil {
		return true
	}
	valid := true
	for _, step := range c.nodes {
		if step.Kind != graph.NodeKindStep || !strings.HasPrefix(step.Id, unit.node.Id+"#") || step.Form != "emit" {
			continue
		}
		for _, emission := range c.graphEdges() {
			if emission.Kind != graph.EdgeKindEmits || emission.From != step.Id {
				continue
			}
			found := false
			for _, publisher := range c.graphEdges() {
				if publisher.Kind == graph.EdgeKindPublishes && publisher.To == emission.To {
					found = true
					break
				}
			}
			if !found {
				c.diagnostic(unit.node, "lang.golang.missing-publisher", "the automatic flow has no observed publication boundary for "+emission.To, "bind a concrete void publisher with //uc:publishes "+emission.To)
				valid = false
			}
		}
	}
	return valid
}
