package check

import (
	"fmt"
	"slices"
	"strings"

	"github.com/lincyaw/unicode/internal/graph"
)

// observations checks rule 34. Observation values come from one free
// adapter whose explicit inputs follow the observation's from clause.
func (c *checker) observations() {
	for _, observation := range c.nodes {
		if !c.is(observation.Id, graph.LayerDesign, graph.NodeKindObservation, "snapshot") {
			continue
		}
		var adapters []string
		for _, binding := range c.in[graph.EdgeKindObserves][observation.Id] {
			if c.isFunction(binding.From) && !c.isMethod(binding.From) {
				adapters = append(adapters, binding.From)
			}
		}
		switch len(adapters) {
		case 0:
			c.fail("check.missing-observer", observation.Id, graph.StatusMissing, observation.Origin,
				fmt.Sprintf("no free function observes the named snapshot %s", observation.Id),
				fmt.Sprintf("write an adapter with the binding observes %s and the snapshot's explicit inputs", observation.Id))
		case 1:
			c.observerSignature(observation, adapters[0])
		default:
			c.fail("check.multiple-observers", observation.Id, graph.StatusContradicted, observation.Origin,
				fmt.Sprintf("the named snapshot %s has %d adapters, %s, but exactly one must observe it", observation.Id, len(adapters), strings.Join(adapters, ", ")),
				fmt.Sprintf("keep one observes %s binding and remove the others", observation.Id))
		}
		for _, adapter := range adapters {
			c.observerPlacement(observation, adapter)
		}
	}
}

func (c *checker) observerSignature(observation graph.Node, adapter string) {
	symbols := map[string]string{}
	for _, symbol := range c.symbols(c.parent[observation.Id]) {
		symbols[symbol.name] = symbol.typ
	}
	var expected []string
	valid := c.is(c.parent[observation.Id], graph.LayerDesign, graph.NodeKindProperty)
	for _, input := range observation.Attrs["inputs"] {
		typ, ok := symbols[input]
		valid = valid && ok
		expected = append(expected, typ)
	}
	signature, parsed := parseSignature(c.attr(adapter, "signature"))
	_, language := languageOf(adapter)
	valid = valid && parsed && c.sameTypes(expected, typesOf(signature.params)) &&
		signature.result != "" && c.sameType(c.attr(observation.Id, "type"), signature.result) &&
		(signature.raises == "" || !language.noRaises && signature.raises == "error")
	if !valid {
		c.fail("check.observer-signature", observation.Id, graph.StatusContradicted, c.byID[adapter].Origin,
			fmt.Sprintf("the adapter %s has signature %s, but %s requires inputs (%s) and result %s without a test handle",
				adapter, c.attr(adapter, "signature"), observation.Id, strings.Join(expected, ", "), c.attr(observation.Id, "type")),
			"match the from inputs in order and return the snapshot type, optionally followed by Go's infrastructure error")
	} else if c.attr(adapter, "stub") == "true" {
		c.mark(observation.Id, graph.StatusStub)
	}
}

func (c *checker) observerPlacement(observation graph.Node, adapter string) {
	home, owners := c.home(observation.Id), c.owners(adapter)
	if c.isDesignContainer(home) && slices.Contains(owners, home) {
		return
	}
	c.fail("check.observer-outside-container", observation.Id, graph.StatusContradicted, c.byID[adapter].Origin,
		fmt.Sprintf("the adapter %s of %s belongs to %s, but the property's container is %s", adapter, observation.Id, ownerList(owners), home),
		fmt.Sprintf("move the adapter into code owned by %s", home))
}
