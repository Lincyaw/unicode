package check

import (
	"fmt"
	"slices"
	"strings"

	"github.com/lincyaw/unicode/internal/graph"
)

// predicates checks rule 20: each property and flow has one predicate, whose
// parameters, after the language's test handle, are its symbols.
func (c *checker) predicates() {
	for _, p := range c.nodes {
		if !c.isCheckable(p.Id) {
			continue
		}
		var preds []string
		for _, e := range c.in[graph.EdgeKindChecks][p.Id] {
			if c.isFunction(e.From) {
				preds = append(preds, e.From)
			}
		}
		switch len(preds) {
		case 0:
			c.fail("check.missing-predicate", p.Id, graph.StatusMissing, p.Origin,
				fmt.Sprintf("no predicate checks %s", c.describe(p.Id)),
				fmt.Sprintf("write a predicate with the binding checks %s whose parameters are its symbols", p.Id))
		case 1:
			c.predicateSignature(p.Id, preds[0])
		default:
			c.fail("check.multiple-predicates", p.Id, graph.StatusContradicted, p.Origin,
				fmt.Sprintf("%s has %d predicates, %s, but exactly one checks it", c.describe(p.Id), len(preds), strings.Join(preds, ", ")),
				"keep one predicate, and remove the checks binding of the others")
		}
	}
}

func (c *checker) predicateSignature(p, pred string) {
	params := parameters(c.attr(pred, "signature"))
	var want []string
	for _, s := range c.symbols(p) {
		want = append(want, s.name+": "+s.typ)
	}
	prefix, l := languageOf(pred)
	handle := l.handle
	rest, ok := params, true
	if handle != "" {
		ok = len(params) > 0 && strings.HasSuffix(params[0], ": "+handle)
		if ok {
			rest = params[1:]
		}
	}
	if !ok || !c.sameParams(want, rest) {
		lead := ""
		if handle != "" {
			lead = fmt.Sprintf(", after a first parameter of the type %s that %s predicates take,", strings.TrimPrefix(handle, prefix), strings.TrimSuffix(prefix, ":"))
		}
		c.fail("check.predicate-signature", p, graph.StatusContradicted, c.byID[pred].Origin,
			fmt.Sprintf("the predicate %s of %s takes (%s), but its parameters%s must be the symbols (%s)",
				pred, p, strings.Join(params, ", "), lead, strings.Join(want, ", ")),
			fmt.Sprintf("give %s exactly the parameters (%s)%s", pred, strings.Join(want, ", "), strings.TrimSuffix(lead, ",")))
		return
	}
	if c.attr(pred, "stub") == "true" {
		c.mark(p, graph.StatusStub)
	}
}

// generators checks the half of rule 24 that needs the code: each generated
// symbol has exactly one generates binding.
func (c *checker) generators() {
	for _, p := range c.nodes {
		if !c.isCheckable(p.Id) {
			continue
		}
		for _, s := range c.generatedSymbols(p.Id) {
			var gens []string
			for _, e := range c.in[graph.EdgeKindGenerates][p.Id] {
				if c.isFunction(e.From) && slices.Contains(e.Attrs["symbols"], s) {
					gens = append(gens, e.From)
				}
			}
			switch {
			case len(gens) == 0:
				c.fail("check.missing-generator", p.Id, graph.StatusMissing, p.Origin,
					fmt.Sprintf("the symbol %s of %s is generated, but no function has the binding generates %s.%s", s, p.Id, p.Id, s),
					fmt.Sprintf("write a generator with the binding generates %s.%s that returns a generator of the symbol's type", p.Id, s))
			case len(gens) == 1 && c.attr(gens[0], "stub") == "true":
				c.mark(p.Id, graph.StatusStub)
			case len(gens) > 1:
				c.fail("check.multiple-generators", p.Id, graph.StatusContradicted, p.Origin,
					fmt.Sprintf("the symbol %s of %s has %d generators, %s, but exactly one generates it", s, p.Id, len(gens), strings.Join(gens, ", ")),
					fmt.Sprintf("keep one generates %s.%s binding, and remove the others", p.Id, s))
			}
		}
	}
}

// steps checks rule 22: each call and send step is traced to a function that
// the code reaches from the step's context, and the code of each emit
// step's sender refers to the event's code type.
func (c *checker) steps() {
	for _, s := range c.nodes {
		switch {
		case c.is(s.Id, graph.LayerDesign, graph.NodeKindStep, "call", "send"):
			c.callStep(s)
		case c.is(s.Id, graph.LayerDesign, graph.NodeKindStep, "emit"):
			c.emitStep(s)
		}
	}
}

// calledOp is the design operation that a step calls, or "".
func (c *checker) calledOp(step string) string {
	for _, e := range c.out[graph.EdgeKindCalls][step] {
		if c.is(e.To, graph.LayerDesign, graph.NodeKindOp) {
			return e.To
		}
	}
	return ""
}

func (c *checker) callStep(s graph.Node) {
	op := c.calledOp(s.Id)
	if op == "" {
		return
	}
	var traced []string
	for _, e := range c.out[graph.EdgeKindTraces][s.Id] {
		traced = append(traced, e.To)
	}
	if len(traced) == 0 {
		c.fail("check.untraced-step", s.Id, graph.StatusMissing, s.Origin,
			fmt.Sprintf("the step %s calls %s, which no function of the code realizes", s.Id, op),
			fmt.Sprintf("realize %s in the code", op))
		return
	}
	context, what, checked := c.context(s.Id)
	if !checked {
		return
	}
	for _, from := range context {
		for _, f := range traced {
			if c.reaches(from, f) {
				return
			}
		}
	}
	c.fail("check.unreachable-step", s.Id, graph.StatusMissing, s.Origin,
		fmt.Sprintf("the step %s calls %s, realized by %s, but no %s reaches it along calls", s.Id, op, strings.Join(traced, ", "), what),
		fmt.Sprintf("call %s from a %s, or correct the flow", strings.Join(traced, " or "), what))
}

// context lists the functions from which the code must reach what a step
// calls, and describes them: those that realize the operation of the
// enclosing call or async; for an outermost step, the methods of its sender
// type's code or the functions of its sender container's. Interfaces go by
// dispatch: an abstract method stands for the same-named method of each
// code type that implements its interface, and a code interface's methods
// for those of each type that implements it. Checked is false where rule
// 22 checks nothing, or where the code to start from is missing, which
// other rules report.
func (c *checker) context(step string) (functions []string, what string, checked bool) {
	for _, a := range c.ancestors(step) {
		if !c.is(a, graph.LayerDesign, graph.NodeKindStep) {
			break
		}
		if c.is(a, graph.LayerDesign, graph.NodeKindStep, "call", "async") {
			op := c.calledOp(a)
			var realizers, out []string
			for _, e := range c.in[graph.EdgeKindRealizes][op] {
				if c.isFunction(e.From) {
					realizers = append(realizers, e.From)
					out = append(out, c.dispatch(e.From)...)
				}
			}
			return out, "function that realizes or, through an interface, implements " + op + " (the operation of the enclosing step " + a + ")", len(realizers) > 0
		}
	}
	sender := c.sender(step)
	switch {
	case c.is(sender, graph.LayerDesign, graph.NodeKindType):
		realizers := c.realizers(sender)
		var out []string
		for _, r := range realizers {
			types := []string{r}
			if c.is(r, graph.LayerCode, graph.NodeKindType, "interface") {
				types = append(types, c.implementers(r)...)
			}
			for _, t := range types {
				out = append(out, c.methodsOf(t)...)
			}
		}
		return out, "method of the code type that realizes the sender " + sender + ", or of a type that implements it", len(realizers) > 0
	case c.isDesignContainer(sender):
		return c.functionsOf(sender), "function in the code of the sender " + sender, true
	}
	return nil, "", false
}

// dispatch lists the functions that a call of fn may run: fn itself, or for
// an interface's abstract method, the same-named method of each code type
// that implements the interface.
func (c *checker) dispatch(fn string) []string {
	if !c.is(fn, graph.LayerCode, graph.NodeKindOp, "abstract") {
		return []string{fn}
	}
	var out []string
	for _, t := range c.implementers(c.parent[fn]) {
		out = append(out, c.methodNamed(t, c.attr(fn, "name"))...)
	}
	return out
}

// implementers lists the code types that implement the code interface i.
func (c *checker) implementers(i string) []string {
	var out []string
	for _, e := range c.in[graph.EdgeKindImplements][i] {
		if c.is(e.From, graph.LayerCode, graph.NodeKindType) {
			out = append(out, e.From)
		}
	}
	return out
}

// embedded lists a code type and the types it embeds (extends), nearest
// first, each embedding level after the one that embeds it.
func (c *checker) embedded(t string) [][]string {
	var levels [][]string
	seen := map[string]bool{t: true}
	for level := []string{t}; len(level) > 0; {
		levels = append(levels, level)
		var next []string
		for _, x := range level {
			for _, e := range c.out[graph.EdgeKindExtends][x] {
				if !seen[e.To] && c.is(e.To, graph.LayerCode, graph.NodeKindType) {
					seen[e.To] = true
					next = append(next, e.To)
				}
			}
		}
		level = next
	}
	return levels
}

// methodsOf lists the methods of a code type, those it gets from the types
// it embeds included.
func (c *checker) methodsOf(t string) []string {
	var out []string
	for _, level := range c.embedded(t) {
		for _, x := range level {
			for _, m := range c.children[x] {
				if c.isFunction(m) {
					out = append(out, m)
				}
			}
		}
	}
	return out
}

// methodNamed lists the methods named name that a code type has, its own or,
// failing those, the nearest that it gets from the types it embeds, as Go
// selects them.
func (c *checker) methodNamed(t, name string) []string {
	for _, level := range c.embedded(t) {
		var out []string
		for _, x := range level {
			for _, m := range c.children[x] {
				if c.isFunction(m) && c.attr(m, "name") == name {
					out = append(out, m)
				}
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

func (c *checker) sender(step string) string {
	if from := c.out[graph.EdgeKindFrom][step]; len(from) > 0 {
		return from[0].To
	}
	return ""
}

// emitStep checks that the code of an emit step's sender refers to the code
// type that realizes the event.
func (c *checker) emitStep(s graph.Node) {
	sender := c.sender(s.Id)
	var event string
	if emits := c.out[graph.EdgeKindEmits][s.Id]; len(emits) > 0 {
		event = emits[0].To
	}
	events := c.realizers(event)
	var code []string
	switch {
	case c.is(sender, graph.LayerDesign, graph.NodeKindType):
		for _, r := range c.realizers(sender) {
			code = append(append(code, r), c.children[r]...)
		}
	case c.isDesignContainer(sender):
		for _, n := range c.nodes {
			if n.Layer == graph.LayerCode && (n.Kind == graph.NodeKindType || n.Kind == graph.NodeKindOp) && slices.Contains(c.owners(n.Id), sender) {
				code = append(code, n.Id)
			}
		}
	}
	if event == "" || len(events) == 0 || len(code) == 0 {
		return
	}
	for _, from := range code {
		for _, e := range events {
			if c.has(graph.EdgeKindRefers, from, e) {
				return
			}
		}
	}
	c.fail("check.unreferenced-event", s.Id, graph.StatusMissing, s.Origin,
		fmt.Sprintf("the step %s has %s emit %s, but no code of %s refers to %s, which realizes the event", s.Id, sender, event, sender, strings.Join(events, " or ")),
		fmt.Sprintf("build and publish a %s in the code that realizes %s, or correct the flow", strings.Join(events, " or "), sender))
}
