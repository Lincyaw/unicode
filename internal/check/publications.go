package check

import (
	"fmt"
	"slices"

	"github.com/lincyaw/unicode/internal/graph"
)

func (c *checker) publisherBinding(e graph.Edge) {
	if !c.isFunction(e.From) || !c.is(e.To, graph.LayerDesign, graph.NodeKindType, "event") {
		c.wrongKind(e, "a publisher is a function or method targeting a design event")
		return
	}
	if len(c.out[graph.EdgeKindPublishes][e.From]) > 1 {
		c.fail("check.multiple-publications", e.From, graph.StatusContradicted, e.Origin,
			"a function carries more than one event publication binding", "keep exactly one publishes binding on each publication boundary")
		c.mark(edgeID(e), graph.StatusContradicted)
	}
	signature, parsed := parseSignature(c.attr(e.From, "signature"))
	if !parsed || len(signature.params) != 1 || !c.sameType(e.To, typesOf(signature.params)[0]) || signature.result != "" || signature.raises != "" {
		c.fail("check.publisher-signature", edgeID(e), graph.StatusContradicted, e.Origin,
			fmt.Sprintf("the publisher %s has signature %s, but publishing %s requires exactly that event parameter and no result", e.From, c.attr(e.From, "signature"), e.To),
			"accept exactly one parameter of the published event type and return no value")
	}
	home := c.home(e.To)
	if !slices.ContainsFunc(c.owners(e.From), func(owner string) bool { return owner == home || c.allowed(owner, home) }) {
		c.fail("check.publisher-outside-container", edgeID(e), graph.StatusContradicted, e.Origin,
			fmt.Sprintf("the publisher %s is not owned by a component allowed to depend on %s", e.From, home),
			"place the publisher in the event's component or a component allowed to use it")
	}
}
