package check

import (
	"fmt"
	"slices"
	"strings"

	"github.com/lincyaw/unicode/internal/graph"
)

// purity reports each design operation declared pure whose realization
// reaches an effect (docs/model.md §10 rule 39), with the shortest chain of
// calls from a realization to a function that has one of them directly.
func (c *checker) purity(effects map[string][]graph.Effect) {
	for _, n := range c.nodes {
		kinds := effects[n.Id]
		if n.Layer != graph.LayerDesign || n.Kind != graph.NodeKindOp || !slices.Contains(n.Attrs["pure"], "true") || len(kinds) == 0 {
			continue
		}
		names := make([]string, len(kinds))
		for i, k := range kinds {
			names[i] = string(k)
		}
		chain := c.effectChain(n.Id)
		c.fail("check.effect-in-pure-op", n.Id, graph.StatusContradicted, n.Origin,
			fmt.Sprintf("the operation %s is declared pure, but its realization reaches the effects %s: %s has the effects %s directly",
				n.Id, strings.Join(names, ", "), strings.Join(chain, " -> "), strings.Join(c.byID[chain[len(chain)-1]].Attrs["directEffects"], ", ")),
			fmt.Sprintf("take the effect out of the code that %s reaches, for example by passing in the value it reads, or remove pure from %s", chain[0], n.Id))
	}
}

// effectChain is the shortest chain of calls, interface dispatch included,
// from a code operation that realizes the design operation op to a function
// with a direct effect.
func (c *checker) effectChain(op string) []string {
	var starts []string
	for _, e := range c.in[graph.EdgeKindRealizes][op] {
		if c.isFunction(e.From) {
			starts = append(starts, e.From)
		}
	}
	if len(starts) == 0 {
		return []string{op}
	}
	slices.Sort(starts)
	from := map[string]string{}
	seen := map[string]bool{}
	for _, s := range starts {
		seen[s] = true
	}
	for queue := slices.Clone(starts); len(queue) > 0; queue = queue[1:] {
		f := queue[0]
		if len(c.byID[f].Attrs["directEffects"]) > 0 {
			chain := []string{f}
			for p, ok := from[f]; ok; p, ok = from[p] {
				chain = append([]string{p}, chain...)
			}
			return chain
		}
		next := c.callees(f)
		if c.is(f, graph.LayerCode, graph.NodeKindOp, "abstract") {
			next = append(next, c.dispatch(f)...)
		}
		for _, g := range next {
			if !seen[g] {
				seen[g] = true
				from[g] = f
				queue = append(queue, g)
			}
		}
	}
	return starts[:1]
}
