package check

import (
	"fmt"
	"slices"
	"strings"

	"github.com/lincyaw/unicode/internal/graph"
)

// bindings checks rule 15: each binding names a design node of the kind its
// place allows. Bindings to no node are graph.Derive's to report.
func (c *checker) bindings() {
	checks := map[string][]graph.Edge{}
	observers := map[string][]graph.Edge{}
	for _, e := range c.edges {
		if e.Source != graph.SourceDeclared {
			continue
		}
		switch e.Kind {
		case graph.EdgeKindRealizes:
			c.realizesBinding(e)
		case graph.EdgeKindChecks:
			if !c.isFunction(e.From) || !c.isCheckable(e.To) {
				c.wrongKind(e, "a predicate checks a property or a flow")
			}
			checks[e.From] = append(checks[e.From], e)
		case graph.EdgeKindGenerates:
			c.generatesBinding(e)
		case graph.EdgeKindObserves:
			if !c.isFunction(e.From) || c.isMethod(e.From) || !c.is(e.To, graph.LayerDesign, graph.NodeKindObservation, "snapshot") {
				c.wrongKind(e, "an observation adapter is a free function that observes a named snapshot")
			}
			observers[e.From] = append(observers[e.From], e)
		}
	}
	for _, f := range c.nodes {
		if edges := observers[f.Id]; len(edges) > 1 {
			var targets []string
			for _, edge := range edges {
				targets = append(targets, edge.To)
				c.mark(edgeID(edge), graph.StatusContradicted)
				c.mark(edge.To, graph.StatusContradicted)
			}
			c.fail("check.multiple-observes", f.Id, graph.StatusContradicted, f.Origin,
				fmt.Sprintf("%s carries %d observes bindings (%s), but an adapter observes exactly one named snapshot", c.describe(f.Id), len(edges), strings.Join(targets, ", ")),
				"keep one observes binding on the adapter and give each other named snapshot its own adapter")
		}
		if edges := checks[f.Id]; len(edges) > 1 {
			var targets []string
			for _, e := range edges {
				targets = append(targets, e.To)
				c.mark(edgeID(e), graph.StatusContradicted)
			}
			c.fail("check.multiple-checks", f.Id, graph.StatusContradicted, f.Origin,
				fmt.Sprintf("%s carries %d checks bindings (%s), but a function is the predicate of one property or flow at most",
					c.describe(f.Id), len(edges), strings.Join(targets, ", ")),
				"keep one checks binding on the function, and give each other property or flow a predicate of its own")
		}
	}
}

func (c *checker) isCheckable(id string) bool {
	return c.is(id, graph.LayerDesign, graph.NodeKindProperty) || c.is(id, graph.LayerDesign, graph.NodeKindFlow)
}

func (c *checker) realizesBinding(e graph.Edge) {
	from := c.byID[e.From]
	switch {
	case c.isMethod(e.From):
		c.fail("check.method-binding", edgeID(e), graph.StatusContradicted, e.Origin,
			fmt.Sprintf("%s carries a realizes binding to %s, but a method carries no realizes binding", c.describe(e.From), e.To),
			"remove the binding: a method realizes the operation that its name maps to, of the design type that its type realizes")
	case from.Kind == graph.NodeKindType:
		if !c.is(e.To, graph.LayerDesign, graph.NodeKindType) {
			c.wrongKind(e, "a code type realizes a type")
		}
	case from.Kind == graph.NodeKindOp:
		if !c.is(e.To, graph.LayerDesign, graph.NodeKindOp) || !c.providedDirectly(e.To) {
			c.wrongKind(e, "a function outside any type realizes an operation that a component or an external system provides directly")
		}
	default:
		c.wrongKind(e, "only code types and functions carry realizes bindings")
	}
}

// providedDirectly tells whether an operation belongs to a container or an
// actor rather than to a type.
func (c *checker) providedDirectly(op string) bool {
	p := c.parent[op]
	return c.isDesignContainer(p) || c.is(p, graph.LayerDesign, graph.NodeKindActor)
}

func (c *checker) wrongKind(e graph.Edge, rule string) {
	c.fail("check.wrong-binding-kind", edgeID(e), graph.StatusContradicted, e.Origin,
		fmt.Sprintf("the %s binding of %s names %s, but %s", e.Kind, c.describe(e.From), c.describe(e.To), rule),
		fmt.Sprintf("correct the identity in the %s binding of %s", e.Kind, e.From))
}

func (c *checker) generatesBinding(e graph.Edge) {
	if !c.isFunction(e.From) || !c.isCheckable(e.To) {
		c.wrongKind(e, "a generator generates a symbol of a property or a flow")
		return
	}
	symbols := e.Attrs["symbols"]
	if len(symbols) == 0 {
		c.fail("check.not-generated-symbol", edgeID(e), graph.StatusContradicted, e.Origin,
			fmt.Sprintf("the generates binding of %s names no symbol of %s", e.From, e.To),
			fmt.Sprintf("write the binding as %s.<symbol>, naming a generated symbol", e.To))
	}
	for _, s := range symbols {
		if !slices.Contains(c.generatedSymbols(e.To), s) {
			c.fail("check.not-generated-symbol", edgeID(e), graph.StatusContradicted, e.Origin,
				fmt.Sprintf("the generates binding of %s names %s.%s, which is not a generated symbol of %s", e.From, e.To, s, e.To),
				fmt.Sprintf("name a symbol that %s marks generated, or remove the binding", e.To))
		}
	}
}

// symbol is a symbol of a flow or a property, from its `name: type` entry.
type symbol struct {
	name, typ string
	generated bool
}

func (c *checker) symbols(id string) []symbol {
	var out []symbol
	for _, s := range c.byID[id].Attrs["symbols"] {
		typ, generated := strings.CutSuffix(s, " generated")
		name, typ, _ := strings.Cut(typ, ": ")
		out = append(out, symbol{name, typ, generated})
	}
	return out
}

func (c *checker) generatedSymbols(id string) []string {
	var out []string
	for _, s := range c.symbols(id) {
		if s.generated {
			out = append(out, s.name)
		}
	}
	return out
}

// realizers lists the code that realizes a design type or operation (rule 16):
// a type through its binding; an operation of a type through a method that
// Derive matched to it, an interface's abstract methods included; any other
// operation through the binding of a function outside types. Bindings of the
// wrong kind count for nothing.
func (c *checker) realizers(id string) []string {
	var out []string
	for _, e := range c.in[graph.EdgeKindRealizes][id] {
		switch {
		case c.byID[id].Kind == graph.NodeKindType:
			if e.Source == graph.SourceDeclared && c.is(e.From, graph.LayerCode, graph.NodeKindType) {
				out = append(out, e.From)
			}
		case c.isMethod(e.From):
			if e.Source == graph.SourceDerived {
				out = append(out, e.From)
			}
		case c.isFunction(e.From) && c.providedDirectly(id):
			out = append(out, e.From)
		}
	}
	return out
}

// realizations checks rules 16 to 18 for every design type and operation:
// one code element realizes it, in a file of its container, with its name,
// signature and fields.
func (c *checker) realizations() {
	for _, n := range c.nodes {
		if n.Layer != graph.LayerDesign || n.Kind != graph.NodeKindType && n.Kind != graph.NodeKindOp {
			continue
		}
		rs := c.realizers(n.Id)
		switch {
		case len(rs) == 0:
			c.fail("check.unrealized", n.Id, graph.StatusMissing, n.Origin,
				fmt.Sprintf("no code realizes %s", c.describe(n.Id)), c.realizeFix(n.Id))
		case len(rs) > 1:
			c.fail("check.multiple-realizations", n.Id, graph.StatusContradicted, n.Origin,
				fmt.Sprintf("%s is realized %d times, by %s, but exactly one code element realizes it", c.describe(n.Id), len(rs), strings.Join(rs, ", ")),
				"keep one of them, and remove the bindings of the others or rename them")
		case c.attr(rs[0], "stub") == "true":
			c.mark(n.Id, graph.StatusStub)
		}
		for _, r := range rs {
			c.placed(n.Id, r)
			c.matches(n.Id, r)
		}
	}
}

func (c *checker) realizeFix(id string) string {
	home := c.home(id)
	switch {
	case c.byID[id].Kind == graph.NodeKindType:
		return fmt.Sprintf("write a code type with the binding realizes %s in code that %s owns", id, home)
	case !c.providedDirectly(id):
		return fmt.Sprintf("give the code type that realizes %s a method named after %s", c.parent[id], name(id))
	case c.is(home, graph.LayerDesign, graph.NodeKindActor):
		return fmt.Sprintf("write a client function with the binding realizes %s in a component that uses %s", id, home)
	}
	return fmt.Sprintf("write a function with the binding realizes %s in code that %s owns", id, home)
}

// placed checks rule 17: the code that realizes a design node lies in a file
// of the node's container; a client of an external system lies in a file of
// a component that may use the system.
func (c *checker) placed(id, r string) {
	home, owners := c.home(id), c.owners(r)
	switch {
	case c.isDesignContainer(home) && !slices.Contains(owners, home):
		c.fail("check.realization-outside-container", id, graph.StatusContradicted, c.byID[r].Origin,
			fmt.Sprintf("%s realizes %s of %s, but its file %s belongs to %s", r, id, home, c.attr(c.fileOf(r), "path"), ownerList(owners)),
			fmt.Sprintf("move %s into code that %s owns", r, home))
	case c.is(home, graph.LayerDesign, graph.NodeKindActor, "external") &&
		!slices.ContainsFunc(owners, func(o string) bool { return c.usesFrom(o, home) }):
		c.fail("check.realization-outside-container", id, graph.StatusContradicted, c.byID[r].Origin,
			fmt.Sprintf("the client %s realizes %s of the external system %s, but its file %s belongs to %s, which does not use %s",
				r, id, home, c.attr(c.fileOf(r), "path"), ownerList(owners), home),
			fmt.Sprintf("move %s into a component that uses %s", r, home))
	}
}

func ownerList(owners []string) string {
	if len(owners) == 0 {
		return "no component"
	}
	return strings.Join(owners, " and ")
}

// matches checks rule 18: the realizing code carries the design node's name,
// its operation's signature, and every field the design writes, of the same
// type; and an enum's code type has a constant for exactly the enum's values
// (decisions 40 and 43).
func (c *checker) matches(id, r string) {
	at := c.byID[r].Origin
	if got := c.attr(r, "name"); got != name(id) {
		c.fail("check.name-mismatch", id, graph.StatusContradicted, at,
			fmt.Sprintf("%s realizes %s, but its name maps back to %s, not to %s", r, id, got, name(id)),
			fmt.Sprintf("rename %s to the name that %s maps to", r, name(id)))
	}
	if c.byID[id].Kind == graph.NodeKindOp {
		if want, got := c.attr(id, "signature"), c.attr(r, "signature"); !c.sameSignature(want, got, r) {
			c.fail("check.signature-mismatch", id, graph.StatusContradicted, at,
				fmt.Sprintf("the signature of %s is %s, but %s, which realizes it, has %s", id, want, r, got),
				fmt.Sprintf("change the signature of %s to %s, or change the design", r, want))
		}
		return
	}
	code := map[string]string{}
	for _, f := range c.byID[r].Attrs["fields"] {
		n, typ, _ := strings.Cut(f, ": ")
		code[n] = typ
	}
	for _, f := range c.byID[id].Attrs["fields"] {
		n, typ, _ := strings.Cut(f, ": ")
		switch got, ok := code[n]; {
		case !ok:
			c.fail("check.missing-field", id, graph.StatusContradicted, at,
				fmt.Sprintf("the design gives %s the field %s, but %s, which realizes it, has no field %s", id, f, r, n),
				fmt.Sprintf("add the field %s to %s", f, r))
		case !c.sameType(typ, got):
			c.fail("check.field-type-mismatch", id, graph.StatusContradicted, at,
				fmt.Sprintf("the field %s of %s has the type %s, but in %s, which realizes it, the type %s", n, id, typ, r, got),
				fmt.Sprintf("give the field %s of %s the type %s, or change the design", n, r, typ))
		}
	}
	if c.byID[id].Form != "enum" {
		return
	}
	values, constants := c.byID[id].Attrs["values"], c.byID[r].Attrs["values"]
	for _, v := range values {
		if !slices.Contains(constants, v) {
			c.fail("check.missing-value", id, graph.StatusContradicted, at,
				fmt.Sprintf("the design gives the enum %s the value %s, but %s, which realizes it, has no constant for it", id, v, r),
				fmt.Sprintf("declare the constant of the value %s for %s", v, r))
		}
	}
	for _, v := range constants {
		if !slices.Contains(values, v) {
			c.fail("check.extra-value", id, graph.StatusContradicted, at,
				fmt.Sprintf("%s, which realizes the enum %s, has a constant for the value %s, which the design does not give the enum", r, id, v),
				fmt.Sprintf("remove the constant of the value %s from %s, or add the value %s to %s in the design", v, r, v, id))
		}
	}
}

// relationCodes are the relations that rule 19 checks, each with its code.
var relationCodes = map[graph.EdgeKind]string{
	graph.EdgeKindImplements: "check.missing-implements",
	graph.EdgeKindExtends:    "check.missing-extends",
}

// relations checks rule 19: an implements or extends between design types
// holds between the code types that realize them.
func (c *checker) relations() {
	for _, e := range c.edges {
		code, checked := relationCodes[e.Kind]
		if !checked || !c.is(e.From, graph.LayerDesign, graph.NodeKindType) || !c.is(e.To, graph.LayerDesign, graph.NodeKindType) {
			continue
		}
		from, to := c.realizers(e.From), c.realizers(e.To)
		if len(from) != 1 || len(to) != 1 || c.has(e.Kind, from[0], to[0]) {
			continue
		}
		c.fail(code, edgeID(e), graph.StatusContradicted, c.byID[from[0]].Origin,
			fmt.Sprintf("%s %s %s in the design, but %s, which realizes %s, does not %s %s, which realizes %s",
				e.From, e.Kind, e.To, from[0], e.From, strings.TrimSuffix(string(e.Kind), "s"), to[0], e.To),
			fmt.Sprintf("make %s %s %s in the code, or remove the %s from the design", from[0], strings.TrimSuffix(string(e.Kind), "s"), to[0], e.Kind))
	}
}
