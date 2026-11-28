package check

import (
	"fmt"
	"slices"
	"strings"

	"github.com/lincyaw/unicode/internal/graph"
)

// purity reports each design operation declared pure whose realization
// reaches an effect (docs/model.md §10 rule 39), with the chain of calls
// that D8's witness gives from a realization to a function that has the
// first of those effects directly.
func (c *checker) purity() {
	for _, n := range c.nodes {
		result := c.effects[n.Id]
		if n.Layer != graph.LayerDesign || n.Kind != graph.NodeKindOp || !slices.Contains(n.Attrs["pure"], "true") || len(result.Effects) == 0 {
			continue
		}
		names := make([]string, len(result.Effects))
		for i, k := range result.Effects {
			names[i] = string(k)
		}
		kind := result.Effects[0]
		chain := c.witnessChain(n.Id, kind)
		c.fail("check.effect-in-pure-op", n.Id, graph.StatusContradicted, n.Origin,
			fmt.Sprintf("the operation %s is declared pure, but its realization reaches the effects %s: %s has the effect %s directly",
				n.Id, strings.Join(names, ", "), strings.Join(chain, " -> "), kind),
			fmt.Sprintf("take the effect out of the code that %s reaches, for example by passing in the value it reads, or remove pure from %s", chain[1], n.Id))
	}
}

// witnessChain follows the witnesses of kind from op to the function that
// has the kind directly: op, the realization, and the calls after it.
func (c *checker) witnessChain(op string, kind graph.Effect) []string {
	chain := []string{op}
	for current := op; ; {
		i := slices.IndexFunc(c.effects[current].Witnesses, func(w graph.EffectWitness) bool { return w.Kind == kind })
		if i < 0 || c.effects[current].Witnesses[i].Via == "" {
			return chain
		}
		current = c.effects[current].Witnesses[i].Via
		chain = append(chain, current)
	}
}
