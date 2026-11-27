package check

import (
	"math/bits"
	"slices"
	"strings"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/graph"
)

// Broken is a conforming graph with one change that breaks a constraint of
// docs/design.md §5.3: the graph, the node or edge that the constraint's
// diagnostic is about, and the diagnostic's code.
//
//uc:realizes check.Broken
type Broken struct {
	Fragment graph.Fragment
	Subject  string
	Code     string
}

// genBroken draws a conforming graph and breaks one constraint at a random site of it.
//
//uc:generates check.rejects-broken-graph.broken
func genBroken() *rapid.Generator[Broken] {
	return rapid.Custom(func(t *rapid.T) Broken {
		w := newWorld(t)
		b := breakages[uniform(t, len(breakages))]
		return Broken{Subject: b.apply(w), Code: b.code, Fragment: w.f}
	})
}

// uniform draws an index below n with equal chances, from unbiased bits:
// rapid's integers favour small values and the largest one.
func uniform(t *rapid.T, n int) int {
	for {
		i := 0
		for range bits.Len(uint(n - 1)) {
			i <<= 1
			if rapid.Bool().Draw(t, "bit") {
				i++
			}
		}
		if i < n {
			return i
		}
	}
}

func choose[T any](w *world, options []T) T { return options[uniform(w.t, len(options))] }

func where[T any](items []T, keep func(T) bool) []T {
	var out []T
	for _, x := range items {
		if keep(x) {
			out = append(out, x)
		}
	}
	return out
}

// A breakage changes a conforming world so that it breaks the constraint of
// code, and returns the subject of the diagnostic it must give.
type breakage struct {
	code  string
	apply func(w *world) string
}

var breakages = []breakage{
	{"check.handwritten-test", func(w *world) string {
		entry := choose(w, w.drivers)
		w.setAttr(entry, "testKind", []string{rapid.SampledFrom([]string{"example", "property", "fuzz"}).Draw(w.t, "handwrittenTestKind")})
		return entry
	}},
	{"check.multiple-observes", func(w *world) string {
		o := choose(w, w.observations)
		twin := w.node(o.id+"Other", graph.LayerDesign, graph.NodeKindObservation, "snapshot", w.designFile(o.home), map[string][]string{"type": w.attr(o.id, "type"), "inputs": w.attr(o.id, "inputs")})
		w.declared(graph.EdgeKindContains, w.parentOf(o.id), twin)
		w.declared(graph.EdgeKindObserves, o.adapter, twin)
		return o.adapter
	}},
	{"check.missing-observer", func(w *world) string {
		o := choose(w, w.observations)
		w.removeEdges(func(e graph.Edge) bool { return e.Kind == graph.EdgeKindObserves && e.To == o.id })
		return o.id
	}},
	{"check.multiple-observers", func(w *world) string {
		o := choose(w, w.observations)
		twin := w.function(o.home.file, o.adapter+"Twin", "observe", w.attr(o.adapter, "signature")[0], "func")
		w.declared(graph.EdgeKindObserves, twin, o.id)
		return o.id
	}},
	{"check.observer-outside-container", func(w *world) string {
		o := choose(w, w.observations)
		outside := choose(w, where(w.comps, func(c *comp) bool { return c != o.home }))
		w.reparent(o.adapter, outside.file)
		return o.id
	}},
	{"check.observer-signature", func(w *world) string {
		o := choose(w, w.observations)
		signature := w.attr(o.adapter, "signature")[0]
		params, rest, _ := splitParameters(signature)
		switch uniform(w.t, 4) {
		case 0:
			params = append(params, handle)
			signature = signatureOf(params, rest)
		case 1:
			signature = signatureOf(params, " -> (int, string)")
		case 2:
			params[0] = "changed: bytes"
			signature = signatureOf(params, rest)
		default:
			rest, _, _ = strings.Cut(rest, " raises ")
			signature = signatureOf(params, rest+" raises custom")
		}
		w.setAttr(o.adapter, "signature", []string{signature})
		return o.id
	}},
	{"check.wrong-binding-kind", func(w *world) string {
		o := choose(w, w.observations)
		target := choose(w, w.types).id
		w.retarget(graph.EdgeKindObserves, o.adapter, o.id, target)
		return "observes " + o.adapter + " " + target
	}},
	{"check.wrong-binding-kind", func(w *world) string {
		o := choose(w, w.observations)
		owner := w.typeNamed(objectOf(o.home))
		w.reparent(o.adapter, owner.code)
		return "observes " + o.adapter + " " + o.id
	}},
	{"check.unowned-file", func(w *world) string {
		w.node("go:m/other", graph.LayerCode, graph.NodeKindContainer, "package", "other", map[string][]string{"path": {"m/other"}})
		file := w.node("file:other/code.go", graph.LayerCode, graph.NodeKindContainer, "file", "other/code.go", map[string][]string{"path": {"other/code.go"}})
		w.extracted(graph.EdgeKindContains, "go:m/other", file)
		return file
	}},
	{"check.overlapping-paths", func(w *world) string {
		var pairs [][2]*comp
		for _, a := range w.comps {
			for _, b := range w.comps {
				if a != b && !w.nested(a, b) {
					pairs = append(pairs, [2]*comp{a, b})
				}
			}
		}
		p := choose(w, pairs)
		w.setAttr(p[0].id, "paths", append(w.attr(p[0].id, "paths"), strings.TrimPrefix(p[1].file, "file:")))
		return p[0].id
	}},
	{"check.undeclared-dependency", func(w *world) string {
		var pairs [][2]*comp
		for _, a := range w.comps {
			for _, b := range w.comps {
				if a != b && !slices.Contains(w.allowed(a), b.id) {
					pairs = append(pairs, [2]*comp{a, b})
				}
			}
		}
		p := choose(w, pairs)
		w.call(p[0].helper, p[1].helper)
		return "depends " + p[0].id + " " + p[1].id
	}},
	{"check.wrong-binding-kind", func(w *world) string {
		switch uniform(w.t, 3) {
		case 0:
			t, c := choose(w, w.types), choose(w, w.comps)
			w.retarget(graph.EdgeKindRealizes, t.code, t.id, c.id)
			return "realizes " + t.code + " " + c.id
		case 1:
			k, t := choose(w, w.checkables), choose(w, w.types)
			w.retarget(graph.EdgeKindChecks, k.pred, k.id, t.id)
			return "checks " + k.pred + " " + t.id
		}
		c, o := choose(w, w.comps), choose(w, where(w.ops, func(o *dop) bool { return o.owner != nil }))
		w.declared(graph.EdgeKindRealizes, c.helper, o.id)
		return "realizes " + c.helper + " " + o.id
	}},
	{"check.method-binding", func(w *world) string {
		o := choose(w, where(w.ops, func(o *dop) bool { return o.kind != "func" }))
		var targets []string
		for _, c := range w.comps {
			targets = append(targets, c.id)
		}
		for _, k := range w.checkables {
			targets = append(targets, k.id)
		}
		target := choose(w, targets)
		w.declared(graph.EdgeKindRealizes, o.code, target)
		return "realizes " + o.code + " " + target
	}},
	{"check.multiple-checks", func(w *world) string {
		a := choose(w, w.checkables)
		b := choose(w, where(w.checkables, func(k *checkable) bool { return k != a }))
		w.declared(graph.EdgeKindChecks, a.pred, b.id)
		return a.pred
	}},
	{"check.not-generated-symbol", func(w *world) string {
		k := choose(w, w.checkables)
		symbols := []string{"nosuch"}
		for _, s := range k.symbols {
			if n, _, _ := strings.Cut(s, ": "); !strings.HasSuffix(s, " generated") {
				symbols = append(symbols, n)
			}
		}
		w.edge(graph.EdgeKindGenerates, k.home.helper, k.id, graph.SourceDeclared, map[string][]string{"symbols": {choose(w, symbols)}})
		return "generates " + k.home.helper + " " + k.id
	}},
	{"check.unrealized", func(w *world) string {
		if uniform(w.t, 3) == 0 {
			t := choose(w, w.types)
			w.removeEdges(func(e graph.Edge) bool { return e.Kind == graph.EdgeKindRealizes && e.From == t.code && e.To == t.id })
			return t.id
		}
		o := choose(w, w.ops)
		switch o.kind {
		case "func":
			w.removeEdges(func(e graph.Edge) bool { return e.Kind == graph.EdgeKindRealizes && e.From == o.code })
		default:
			w.removeNode(o.code)
		}
		return o.id
	}},
	{"check.multiple-realizations", func(w *world) string {
		if uniform(w.t, 3) == 0 {
			t := choose(w, w.types)
			twin := t.code + "Twin"
			w.node(twin, graph.LayerCode, graph.NodeKindType, "struct", "code", map[string][]string{"name": {name(t.id)}, "fields": t.codeFields})
			w.extracted(graph.EdgeKindContains, w.parentOf(t.code), twin)
			w.declared(graph.EdgeKindRealizes, twin, t.id)
			return t.id
		}
		o := choose(w, w.ops)
		twin := w.function(w.parentOf(o.code), o.code+"Twin", name(o.id), o.codeSignature, o.kind)
		if o.kind != "func" {
			w.edge(graph.EdgeKindRealizes, twin, o.id, graph.SourceDerived, nil)
		} else {
			w.declared(graph.EdgeKindRealizes, twin, o.id)
		}
		return o.id
	}},
	{"check.realization-outside-container", func(w *world) string {
		type site struct {
			id, code string
			others   []*comp
		}
		var sites []site
		for _, t := range w.types {
			sites = append(sites, site{t.id, t.code, w.outside(t.home)})
		}
		for _, o := range where(w.ops, func(o *dop) bool { return o.kind == "func" }) {
			sites = append(sites, site{o.id, o.code, w.outside(o.home)})
		}
		x := choose(w, where(sites, func(x site) bool { return len(x.others) > 0 }))
		w.reparent(x.code, choose(w, x.others).file)
		return x.id
	}},
	{"check.name-mismatch", func(w *world) string {
		var sites []string
		for _, t := range w.types {
			sites = append(sites, t.id)
		}
		for _, o := range where(w.ops, func(o *dop) bool { return o.kind == "func" }) {
			sites = append(sites, o.id)
		}
		id := choose(w, sites)
		w.setAttr(w.codeOf(id), "name", []string{"renamed"})
		return id
	}},
	{"check.signature-mismatch", func(w *world) string {
		o := choose(w, w.ops)
		params, rest, _ := splitParameters(o.codeSignature)
		var code string
		switch uniform(w.t, 5) {
		case 0:
			// No type of the sample is bytes.
			code = "(z: bytes)"
		case 1:
			// One parameter retyped, its name kept. An operation without
			// parameters first gets one, in the design and in the code.
			if len(params) == 0 {
				_, designRest, _ := splitParameters(o.signature)
				params = []string{"a: int"}
				w.setAttr(o.id, "signature", []string{signatureOf(params, designRest)})
			}
			i := uniform(w.t, len(params))
			n, _, _ := strings.Cut(params[i], ": ")
			params[i] = n + ": bytes"
			code = signatureOf(params, rest)
		case 2:
			// Two parameters reordered, each keeping its name: the design
			// takes (a: int, b: string, …) and the code (b: string, a: int, …).
			designParams, designRest, _ := splitParameters(o.signature)
			w.setAttr(o.id, "signature", []string{signatureOf(append([]string{"a: int", "b: string"}, designParams...), designRest)})
			code = signatureOf(append([]string{"b: string", "a: int"}, params...), rest)
		case 3:
			// The code raises where the design does not, or the other way round.
			code = o.codeSignature
			if trimmed, ok := strings.CutSuffix(code, " raises error"); ok {
				code = trimmed
			} else {
				code += " raises error"
			}
		default:
			// A Go result list, which no operation's signature matches.
			code = signatureOf(params, " -> (int, string)")
		}
		w.setAttr(o.code, "signature", []string{code})
		return o.id
	}},
	{"check.missing-field", func(w *world) string {
		t := choose(w, w.types)
		w.setAttr(t.id, "fields", append(slices.Clone(t.fields), "extra: int"))
		return t.id
	}},
	{"check.field-type-mismatch", func(w *world) string {
		t := choose(w, w.types)
		// A field of another type; an event passed by pointer, which only an
		// object may be; or a value where the design wants a pointer.
		pair := choose(w, [][2]string{{"int", "string"}, {"k0.T1", "optional<k0.T1>"}, {"optional<k0.T0>", "k0.T0"}})
		w.setAttr(t.id, "fields", append(slices.Clone(t.fields), "extra: "+pair[0]))
		w.setAttr(t.code, "fields", append(slices.Clone(t.codeFields), "extra: "+pair[1]))
		return t.id
	}},
	{"check.missing-value", func(w *world) string {
		t := choose(w, where(w.types, func(t *dtype) bool { return t.form == "enum" }))
		if uniform(w.t, 2) == 0 {
			w.setAttr(t.id, "values", append(w.attr(t.id, "values"), "frozen"))
		} else {
			values := w.attr(t.code, "values")
			w.setAttr(t.code, "values", slices.Delete(values, 0, 1))
		}
		return t.id
	}},
	{"check.extra-value", func(w *world) string {
		t := choose(w, where(w.types, func(t *dtype) bool { return t.form == "enum" }))
		if uniform(w.t, 2) == 0 {
			w.setAttr(t.code, "values", append(w.attr(t.code, "values"), "archived"))
		} else {
			values := w.attr(t.id, "values")
			i := uniform(w.t, len(values))
			w.setAttr(t.id, "values", slices.Delete(values, i, i+1))
		}
		return t.id
	}},
	{"check.missing-implements", func(w *world) string { return w.relation(graph.EdgeKindImplements) }},
	{"check.missing-extends", func(w *world) string { return w.relation(graph.EdgeKindExtends) }},
	{"check.missing-predicate", func(w *world) string {
		k := choose(w, w.checkables)
		w.removeEdges(func(e graph.Edge) bool { return e.Kind == graph.EdgeKindChecks && e.From == k.pred })
		return k.id
	}},
	{"check.multiple-predicates", func(w *world) string {
		k := choose(w, w.checkables)
		twin := w.function(k.home.file, k.pred+"Twin", "check", w.attr(k.pred, "signature")[0], "func")
		w.declared(graph.EdgeKindChecks, twin, k.id)
		return k.id
	}},
	{"check.predicate-signature", func(w *world) string {
		breaking := uniform(w.t, 4)
		k := choose(w, w.checkables)
		if breaking == 3 {
			k = choose(w, where(w.checkables, func(k *checkable) bool { return len(k.symbols) > 0 }))
		}
		params := parameters(w.attr(k.pred, "signature")[0])
		switch breaking {
		case 0:
			params = params[1:]
		case 1:
			params = append(params, "extra: int")
		case 2:
			params[0] = "t: go:testing.T"
		default:
			// A symbol's parameter renamed: unlike an operation's
			// parameters, a predicate's are its symbols by name (rule 20).
			_, typ, _ := strings.Cut(params[1], ": ")
			params[1] = "renamed: " + typ
		}
		w.setAttr(k.pred, "signature", []string{"(" + strings.Join(params, ", ") + ")"})
		return k.id
	}},
	{"check.untraced-step", func(w *world) string {
		s := choose(w, where(w.steps, func(s *step) bool { return s.form != "emit" }))
		orphan := w.node(s.home.id+".orphan", graph.LayerDesign, graph.NodeKindOp, "op", "design", map[string][]string{"signature": {"()"}})
		w.declared(graph.EdgeKindContains, s.home.id, orphan)
		w.retarget(graph.EdgeKindCalls, s.id, s.op, orphan)
		return s.id
	}},
	{"check.unreachable-step", unreachable},
	{"check.unreachable-step", func(w *world) string {
		// Through an interface: no implementation reaches the step, or no
		// code type implements the interface at all.
		dispatched := where(w.steps, func(s *step) bool { return s.dispatch != "" })
		if len(dispatched) == 0 {
			return unreachable(w)
		}
		s := choose(w, dispatched)
		if uniform(w.t, 2) == 0 {
			w.removeEdges(func(e graph.Edge) bool { return e.Kind == graph.EdgeKindCalls && e.To == s.traced })
		} else {
			w.removeEdges(func(e graph.Edge) bool { return e.Kind == graph.EdgeKindImplements && e.To == s.dispatch })
		}
		return s.id
	}},
	{"check.unreferenced-event", func(w *world) string {
		s := choose(w, where(w.steps, func(s *step) bool { return s.form == "emit" }))
		code := w.codeUnder(s.senderCode[0])
		if s.sender == s.home.id {
			code = w.codeUnder(s.home.file)
		}
		w.removeEdges(func(e graph.Edge) bool {
			return e.Kind == graph.EdgeKindRefers && e.To == s.event && slices.Contains(code, e.From)
		})
		return s.id
	}},
	{"check.missing-generator", func(w *world) string {
		k := choose(w, where(w.checkables, func(k *checkable) bool { return len(k.generators) > 0 }))
		w.removeEdges(func(e graph.Edge) bool { return e.Kind == graph.EdgeKindGenerates && e.To == k.id })
		return k.id
	}},
	{"check.multiple-generators", func(w *world) string {
		k := choose(w, where(w.checkables, func(k *checkable) bool { return len(k.generators) > 0 }))
		twin := w.function(k.home.file, k.generators["s"]+"Twin", "gen", "()", "func")
		w.edge(graph.EdgeKindGenerates, twin, k.id, graph.SourceDeclared, map[string][]string{"symbols": {"s"}})
		return k.id
	}},
}

// signatureOf writes a signature from its parameters and what follows them.
func signatureOf(params []string, rest string) string {
	return "(" + strings.Join(params, ", ") + ")" + rest
}

// unreachable cuts every call to the function that a step with a context traces to.
func unreachable(w *world) string {
	s := choose(w, where(w.steps, func(s *step) bool { return len(s.context) > 0 }))
	w.removeEdges(func(e graph.Edge) bool { return e.Kind == graph.EdgeKindCalls && e.To == s.traced })
	return s.id
}

// relation declares an implements or extends between two types that the
// code does not repeat between the types that realize them.
func (w *world) relation(kind graph.EdgeKind) string {
	var pairs [][2]*dtype
	for _, a := range w.types {
		for _, b := range w.types {
			if a != b && !w.hasEdge(kind, a.code, b.code) {
				pairs = append(pairs, [2]*dtype{a, b})
			}
		}
	}
	p := choose(w, pairs)
	w.declared(kind, p[0].id, p[1].id)
	return string(kind) + " " + p[0].id + " " + p[1].id
}

// codeOf is the code that realizes a design type or a function's operation.
func (w *world) codeOf(id string) string {
	for _, t := range w.types {
		if t.id == id {
			return t.code
		}
	}
	for _, o := range w.ops {
		if o.id == id {
			return o.code
		}
	}
	return ""
}

func (w *world) nested(a, b *comp) bool { return w.within(a, b) || w.within(b, a) }

// outside lists the components whose files do not own the code of what home
// holds: those other than home, or for the external system's code (home
// nil), those that do not lie in its client and so may not use it.
func (w *world) outside(home *comp) []*comp {
	return where(w.comps, func(c *comp) bool {
		if home == nil {
			return !w.within(c, w.client)
		}
		return c != home
	})
}

// within tells whether c is inside around, or is it.
func (w *world) within(c, around *comp) bool {
	for x := c; x != nil; x = x.parent {
		if x == around {
			return true
		}
	}
	return false
}

func (w *world) hasEdge(kind graph.EdgeKind, from, to string) bool {
	return slices.ContainsFunc(w.f.Edges, func(e graph.Edge) bool { return e.Kind == kind && e.From == from && e.To == to })
}

func (w *world) attr(id, key string) []string {
	for _, n := range w.f.Nodes {
		if n.Id == id {
			return slices.Clone(n.Attrs[key])
		}
	}
	return nil
}

func (w *world) setAttr(id, key string, values []string) {
	for i, n := range w.f.Nodes {
		if n.Id == id {
			attrs := map[string][]string{}
			for k, v := range n.Attrs {
				attrs[k] = v
			}
			attrs[key] = values
			w.f.Nodes[i].Attrs = attrs
		}
	}
}

func (w *world) removeEdges(match func(graph.Edge) bool) {
	w.f.Edges = slices.DeleteFunc(w.f.Edges, match)
}

func (w *world) removeNode(id string) {
	w.f.Nodes = slices.DeleteFunc(w.f.Nodes, func(n graph.Node) bool { return n.Id == id })
	w.removeEdges(func(e graph.Edge) bool { return e.From == id || e.To == id })
}

// retarget points the edge kind from → to at another end.
func (w *world) retarget(kind graph.EdgeKind, from, to, other string) {
	for i, e := range w.f.Edges {
		if e.Kind == kind && e.From == from && e.To == to {
			w.f.Edges[i].To = other
		}
	}
}

func (w *world) parentOf(id string) string {
	for _, e := range w.f.Edges {
		if e.Kind == graph.EdgeKindContains && e.To == id {
			return e.From
		}
	}
	return ""
}

// reparent moves a code element into another file.
func (w *world) reparent(id, file string) {
	for i, e := range w.f.Edges {
		if e.Kind == graph.EdgeKindContains && e.To == id {
			w.f.Edges[i].From = file
		}
	}
}

// codeUnder lists a code element and everything it contains.
func (w *world) codeUnder(id string) []string {
	out := []string{id}
	for i := 0; i < len(out); i++ {
		for _, e := range w.f.Edges {
			if e.Kind == graph.EdgeKindContains && e.From == out[i] {
				out = append(out, e.To)
			}
		}
	}
	return out
}
