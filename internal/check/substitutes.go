package check

import (
	"fmt"
	"slices"

	"github.com/lincyaw/unicode/internal/graph"
)

func (c *checker) substituteBinding(e graph.Edge) {
	if !c.isFunction(e.From) || c.isMethod(e.From) || !c.is(e.To, graph.LayerDesign, graph.NodeKindOp) || !c.is(c.parent[e.To], graph.LayerDesign, graph.NodeKindActor, "external") {
		c.wrongKind(e, "a substitute installer is a free function targeting an operation directly provided by an external system")
		return
	}
	c.placed(e.To, e.From)
	if clients := c.realizers(e.To); len(clients) == 1 {
		clientOwners := c.owners(clients[0])
		installerOwners := c.owners(e.From)
		if !slices.ContainsFunc(installerOwners, func(owner string) bool {
			return slices.Contains(clientOwners, owner) && c.usesFrom(owner, c.parent[e.To])
		}) {
			c.fail("check.substitute-outside-container", e.From, graph.StatusContradicted, e.Origin, "the substitute installer and its client are not owned by the same component that uses the external system", "place the installer in a file owned by the client's component")
		}
	}
	if edges := c.out[graph.EdgeKindSubstitutes][e.From]; len(edges) > 1 {
		c.fail("check.multiple-substitutes", e.From, graph.StatusContradicted, e.Origin, "a substitute installer targets more than one operation", "keep exactly one substitutes binding on each installer")
	}
	if edges := c.in[graph.EdgeKindSubstitutes][e.To]; len(edges) > 1 {
		c.fail("check.multiple-substitutes", e.To, graph.StatusContradicted, e.Origin, "the external operation has more than one substitute installer", "keep exactly one substitute installer for this operation")
	}
	if values := e.Attrs["nativeSignature"]; len(values) != 1 || values[0] != "true" {
		c.fail("check.substitute-signature", e.From, graph.StatusContradicted, e.Origin, fmt.Sprintf("the installer %s does not accept the exact native client function signature and return func()", e.From), "accept exactly one function with the client's full native signature and return one nonnil func() cleanup")
	} else if len(c.out[graph.EdgeKindSubstitutes][e.From]) == 1 && len(c.in[graph.EdgeKindSubstitutes][e.To]) == 1 && c.attr(e.From, "stub") == "true" {
		c.mark(e.To, graph.StatusStub)
	}
}
