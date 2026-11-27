package check

import "github.com/lincyaw/unicode/internal/graph"

// drivers uses the declaration's position because a method's containment
// parent is its receiver type, which can be declared in another file.
func (c *checker) drivers() {
	for _, n := range c.nodes {
		if !c.isFunction(n.Id) || c.attr(n.Id, "testKind") == "none" {
			continue
		}
		file := "file:" + n.Origin.Path
		if !c.isFile(file) || c.attr(file, "language") != "go" || c.attr(file, "generated") == "true" {
			continue
		}
		c.fail("check.handwritten-test", n.Id, graph.StatusContradicted, n.Origin,
			"the Go test entry point is declared outside a unicode-generated file",
			"keep its predicate and generators, declare their property bindings, and replace the handwritten entry point with unicode gen")
	}
}
