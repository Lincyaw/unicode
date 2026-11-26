package check

import (
	"fmt"
	"slices"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/lincyaw/unicode/internal/graph"
)

// ownership checks rule 12: every source file belongs to a component, and no
// two components that are not nested match the same file. Patterns that do
// not parse are graph.Derive's to report.
func (c *checker) ownership() {
	reported := map[[2]string]bool{}
	for _, f := range c.nodes {
		if !c.isFile(f.Id) {
			continue
		}
		path := c.attr(f.Id, "path")
		if len(c.in[graph.EdgeKindOwns][f.Id]) == 0 {
			c.fail("check.unowned-file", f.Id, graph.StatusContradicted, f.Origin,
				fmt.Sprintf("the file %s belongs to no component: no paths pattern of the design matches it", path),
				fmt.Sprintf("add a pattern that matches %s to the paths of the component that the file belongs to", path))
		}
		var matching []string
		for _, n := range c.nodes {
			if c.isDesignContainer(n.Id) && slices.ContainsFunc(n.Attrs["paths"], func(p string) bool {
				return doublestar.ValidatePattern(p) && doublestar.MatchUnvalidated(p, path)
			}) {
				matching = append(matching, n.Id)
			}
		}
		for _, a := range matching {
			for _, b := range matching {
				if a == b || slices.Contains(c.ancestors(a), b) || slices.Contains(c.ancestors(b), a) || reported[[2]string{a, b}] {
					continue
				}
				reported[[2]string{a, b}] = true
				c.fail("check.overlapping-paths", a, graph.StatusContradicted, c.byID[a].Origin,
					fmt.Sprintf("the paths of %s match the file %s, and so do the paths of %s, which is neither inside %s nor around it", a, path, b, a),
					fmt.Sprintf("narrow the paths of %s or of %s so that only one of them matches %s, or nest one component in the other", a, b, path))
			}
		}
	}
}

// dependencies checks rule 14: every depends is allowed by rule 13.
func (c *checker) dependencies() {
	for _, e := range c.edges {
		if e.Kind != graph.EdgeKindDepends || c.allowed(e.From, e.To) {
			continue
		}
		c.fail("check.undeclared-dependency", edgeID(e), graph.StatusContradicted, e.Origin,
			fmt.Sprintf("the code of %s uses %s (%s), but %s may not depend on %s: it is not a child of %s, and neither %s nor a component around it uses it",
				e.From, e.To, basis(e), e.From, e.To, e.From, e.From),
			fmt.Sprintf("add %s to the uses of %s in the design, or remove the dependency from the code", e.To, e.From))
	}
}

// allowed is rule 13: a may depend on b when b is a child of a, or a or one
// of its ancestors uses b.
func (c *checker) allowed(a, b string) bool {
	return c.has(graph.EdgeKindContains, a, b) && c.isDesignContainer(b) || c.usesFrom(a, b)
}

// basis writes the first import or call that a depends rests on.
func basis(e graph.Edge) string {
	for _, kind := range []string{"imports", "calls"} {
		if v := e.Attrs[kind]; len(v) > 0 {
			from, to, _ := strings.Cut(v[0], " ")
			return from + " " + kind + " " + to
		}
	}
	return "derived"
}
