package view

import (
	"cmp"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/lincyaw/unicode/internal/graph"
)

// selectors pick each view's candidate nodes and edge kinds (docs/design.md §7.1).
var selectors = map[ViewKind]func(index, request) selection{
	ViewKindArchitecture: architecture,
	ViewKindClass:        class,
	ViewKindSequence:     flow,
	ViewKindActivity:     flow,
	ViewKindEvents:       events,
	ViewKindCalls:        calls,
	ViewKindProperty:     property,
	ViewKindCode:         code,
	ViewKindDdd:          ddd,
	ViewKindC4:           c4,
}

var (
	dddTypeLabels = []string{"aggregate", "entity", "value", "domain-service", "repository", "factory"}
)

func isExternal(n graph.Node) bool { return n.Kind == graph.NodeKindActor && n.Form == "external" }

func hasLabel(n graph.Node, labels []string) bool {
	return slices.ContainsFunc(n.Attrs["labels"], func(l string) bool { return slices.Contains(labels, l) })
}

// architecture: the input, the design containers and external systems in
// it, and those its containers use or depend on, or that use or depend on them.
func architecture(ix index, r request) selection {
	input := r.input
	s := selection{kinds: []graph.EdgeKind{graph.EdgeKindUses, graph.EdgeKindDepends, graph.EdgeKindContains}}
	s.add(input)
	for _, d := range ix.descendants(input) {
		if n := ix.nodes[d]; isDesignContainer(n) || isExternal(n) {
			s.add(d)
		}
	}
	for _, id := range slices.Clone(s.nodes) {
		for _, o := range ix.neighbors(id, graph.EdgeKindUses, graph.EdgeKindDepends) {
			if n := ix.nodes[o]; isDesignContainer(n) || isExternal(n) {
				s.add(o)
			}
		}
	}
	return s
}

// class: for a container, the containers, external systems and types in
// it, and the design types in other containers that its types refer to,
// extend or implement; for a type, it and the design types one refers,
// extends or implements edge away. Each type comes with its operations.
// Containers outside the input's home, the input or the type's container,
// fold whatever their focus. The code layer draws, instead of each design
// type and operation, the salient code that realizes it.
func class(ix index, r request) selection {
	input := r.input
	s := selection{kinds: []graph.EdgeKind{graph.EdgeKindRefers, graph.EdgeKindExtends, graph.EdgeKindImplements, graph.EdgeKindContains},
		home: input}
	s.add(input)
	isDesignType := func(id string) bool {
		n := ix.nodes[id]
		return n.Kind == graph.NodeKindType && n.Layer == graph.LayerDesign
	}
	if ix.nodes[input].Kind == graph.NodeKindContainer {
		for _, d := range ix.descendants(input) {
			if n := ix.nodes[d]; n.Kind == graph.NodeKindContainer || n.Kind == graph.NodeKindType || isExternal(n) {
				s.add(d)
			}
		}
		for _, id := range slices.Clone(s.nodes) {
			if isDesignType(id) {
				s.add(slices.DeleteFunc(ix.targetsOf(id, typeRelations...), func(o string) bool { return !isDesignType(o) })...)
			}
		}
	} else {
		s.home = ix.parent(input)
		s.add(slices.DeleteFunc(ix.neighbors(input, typeRelations...), func(o string) bool { return !isDesignType(o) })...)
	}
	for _, id := range slices.Clone(s.nodes) {
		if ix.nodes[id].Kind == graph.NodeKindType {
			for _, c := range ix.targets(id, graph.EdgeKindContains) {
				if ix.nodes[c].Kind == graph.NodeKindOp {
					s.add(c)
				}
			}
		}
	}
	if r.layer == ClassLayerCode {
		s = s.realized(ix)
	}
	return s
}

// typeRelations are the edges between types that the class view draws.
var typeRelations = []graph.EdgeKind{graph.EdgeKindRefers, graph.EdgeKindExtends, graph.EdgeKindImplements}

// realized is the selection with each design type and operation replaced
// by the salient code types and functions that realize it.
func (s selection) realized(ix index) selection {
	code := selection{kinds: s.kinds, home: s.home}
	for _, id := range s.nodes {
		n := ix.nodes[id]
		if n.Layer != graph.LayerDesign || n.Kind != graph.NodeKindType && n.Kind != graph.NodeKindOp {
			code.add(id)
			continue
		}
		for _, c := range ix.sources(id, graph.EdgeKindRealizes) {
			if m := ix.nodes[c]; m.Layer == graph.LayerCode && m.Kind == n.Kind && m.Salient {
				code.add(c)
			}
		}
	}
	return code
}

// steps returns the steps of a flow sorted by their index in the flow.
func steps(ix index, flow string) []string {
	var found []string
	for _, d := range ix.descendants(flow) {
		if ix.nodes[d].Kind == graph.NodeKindStep {
			found = append(found, d)
		}
	}
	slices.SortFunc(found, func(a, b string) int {
		return cmp.Or(cmp.Compare(stepIndex(ix, a), stepIndex(ix, b)), cmp.Compare(a, b))
	})
	return found
}

// stepIndex is the index attribute of a step; a step without one sorts last.
func stepIndex(ix index, id string) int {
	if i := ix.nodes[id].Attrs["index"]; len(i) > 0 {
		if n, err := strconv.Atoi(i[0]); err == nil {
			return n
		}
	}
	return math.MaxInt
}

// lifelines returns the participants of the steps in the order they first
// appear: each step's sender, then its receiver, which is the owner of the
// operation it calls or the event it emits.
func lifelines(ix index, steps []string) []string {
	var found []string
	for _, s := range steps {
		found = append(found, ix.targets(s, graph.EdgeKindFrom)...)
		for _, op := range ix.targets(s, graph.EdgeKindCalls) {
			if owner := ix.parent(op); owner != "" {
				found = append(found, owner)
			}
		}
		found = append(found, ix.targets(s, graph.EdgeKindEmits)...)
	}
	return found
}

// flow, for the sequence and activity views: the flow, its lifelines in the
// order they first appear, its steps in their order, then the operations
// the steps call, the events and errors they emit and handle, the events
// that the operations of async steps subscribe to, the functions they
// trace to and the cases that cover them.
func flow(ix index, r request) selection {
	input := r.input
	s := selection{kinds: []graph.EdgeKind{graph.EdgeKindContains, graph.EdgeKindFrom, graph.EdgeKindCalls,
		graph.EdgeKindEmits, graph.EdgeKindHandles, graph.EdgeKindSubscribes, graph.EdgeKindTraces, graph.EdgeKindCovers}}
	s.add(input)
	ordered := steps(ix, input)
	s.add(lifelines(ix, ordered)...)
	s.add(ordered...)
	var rest []string
	for _, st := range ordered {
		for _, k := range []graph.EdgeKind{graph.EdgeKindCalls, graph.EdgeKindHandles, graph.EdgeKindTraces} {
			rest = append(rest, ix.targets(st, k)...)
		}
		if ix.nodes[st].Form == "async" {
			for _, op := range ix.targets(st, graph.EdgeKindCalls) {
				rest = append(rest, ix.targets(op, graph.EdgeKindSubscribes)...)
			}
		}
		rest = append(rest, ix.sources(st, graph.EdgeKindCovers)...)
	}
	slices.Sort(rest)
	s.add(rest...)
	return s
}

// events: the events in the container, the steps that emit them and the
// operations that subscribe to them; the errors in it, the call steps whose
// operation raises them and the on steps that handle them.
func events(ix index, r request) selection {
	input := r.input
	s := selection{kinds: []graph.EdgeKind{graph.EdgeKindEmits, graph.EdgeKindSubscribes, graph.EdgeKindHandles}}
	raisers := map[string][]string{}
	for _, id := range slices.Sorted(maps.Keys(ix.nodes)) {
		if n := ix.nodes[id]; n.Kind == graph.NodeKindOp && n.Layer == graph.LayerDesign {
			for _, e := range raises(n) {
				raisers[e] = append(raisers[e], id)
			}
		}
	}
	for _, d := range ix.descendants(input) {
		switch n := ix.nodes[d]; {
		case n.Kind == graph.NodeKindType && n.Form == "event":
			s.add(d)
			s.add(ix.sources(d, graph.EdgeKindEmits)...)
			s.add(ix.sources(d, graph.EdgeKindSubscribes)...)
		case n.Kind == graph.NodeKindType && n.Form == "error":
			s.add(d)
			for _, op := range raisers[d] {
				for _, step := range ix.sources(op, graph.EdgeKindCalls) {
					if ix.nodes[step].Kind == graph.NodeKindStep && ix.nodes[step].Form == "call" {
						s.add(step)
					}
				}
			}
			s.add(ix.sources(d, graph.EdgeKindHandles)...)
		}
	}
	return s
}

// raises returns the errors that the signature of the design operation op
// raises, as identities.
func raises(op graph.Node) []string {
	sig := op.Attrs["signature"]
	if len(sig) == 0 {
		return nil
	}
	at := strings.LastIndex(sig[0], " raises ")
	if at < 0 {
		return nil
	}
	return strings.Split(sig[0][at+len(" raises "):], ", ")
}

// calls: the input with the functions that realize it, and every salient
// function they reach along calls and reaches edges between salient functions.
func calls(ix index, r request) selection {
	input := r.input
	s := selection{kinds: []graph.EdgeKind{graph.EdgeKindCalls, graph.EdgeKindReaches, graph.EdgeKindRealizes}}
	s.add(input)
	for _, f := range ix.sources(input, graph.EdgeKindRealizes) {
		if ix.nodes[f].Kind == graph.NodeKindOp {
			s.add(f)
		}
	}
	for i := 0; i < len(s.nodes); i++ {
		for _, e := range ix.out[s.nodes[i]] {
			if n := ix.nodes[e.To]; (e.Kind == graph.EdgeKindCalls || e.Kind == graph.EdgeKindReaches) && n.Kind == graph.NodeKindOp && n.Salient {
				s.add(e.To)
			}
		}
	}
	return s
}

// property: the property, or every property in the container, each with
// what it is about, its cases and its predicate.
func property(ix index, r request) selection {
	input := r.input
	s := selection{kinds: []graph.EdgeKind{graph.EdgeKindAbout, graph.EdgeKindContains, graph.EdgeKindChecks}}
	props := []string{input}
	if ix.nodes[input].Kind != graph.NodeKindProperty {
		props = nil
		for _, d := range ix.descendants(input) {
			if ix.nodes[d].Kind == graph.NodeKindProperty {
				props = append(props, d)
			}
		}
	}
	for _, p := range props {
		s.add(p)
		s.add(ix.targets(p, graph.EdgeKindAbout)...)
		for _, c := range ix.targets(p, graph.EdgeKindContains) {
			if ix.nodes[c].Kind == graph.NodeKindCase {
				s.add(c)
			}
		}
		s.add(ix.sources(p, graph.EdgeKindChecks)...)
	}
	return s
}

// code: the file or type and what it contains, the functions at the other
// end of the calls and reaches edges of its functions, and the cases that
// cover any of them.
func code(ix index, r request) selection {
	input := r.input
	s := selection{kinds: []graph.EdgeKind{graph.EdgeKindContains, graph.EdgeKindCalls, graph.EdgeKindReaches, graph.EdgeKindCovers}}
	contents := append([]string{input}, ix.descendants(input)...)
	s.add(contents...)
	for _, id := range contents {
		s.add(ix.neighbors(id, graph.EdgeKindCalls, graph.EdgeKindReaches)...)
		s.add(ix.sources(id, graph.EdgeKindCovers)...)
	}
	return s
}

// ddd: the bounded contexts among the container and the containers in it,
// and the types in them that carry a DDD label or are events.
func ddd(ix index, r request) selection {
	input := r.input
	s := selection{kinds: []graph.EdgeKind{graph.EdgeKindRefers, graph.EdgeKindExtends, graph.EdgeKindImplements, graph.EdgeKindContains}}
	for _, c := range append([]string{input}, ix.descendants(input)...) {
		if n := ix.nodes[c]; !isDesignContainer(n) || !slices.Contains(n.Attrs["labels"], "bounded-context") {
			continue
		}
		s.add(c)
		for _, d := range ix.descendants(c) {
			if n := ix.nodes[d]; n.Kind == graph.NodeKindType && (hasLabel(n, dddTypeLabels) || n.Form == "event") {
				s.add(d)
			}
		}
	}
	return s
}

// c4: at the context level, the input standing for everything in it; at
// the container level, the input and the design containers in it labelled
// container, each standing for what it contains. Either level draws the
// actors in the input, the external systems that the input or a container
// in it uses or depends on, and the persons whose steps call an operation
// in it, each step standing in for its person. The edges are the uses and
// depends between what stands for their ends, and the calls of the
// persons' steps.
func c4(ix index, r request) selection {
	input := r.input
	s := selection{kinds: []graph.EdgeKind{graph.EdgeKindUses, graph.EdgeKindDepends, graph.EdgeKindCalls}}
	s.add(input)
	inside := ix.descendants(input)
	boxes := map[string]bool{}
	if r.level == C4LevelContext {
		boxes[input] = true
	}
	for _, d := range inside {
		if n := ix.nodes[d]; r.level == C4LevelContainer && isDesignContainer(n) && slices.Contains(n.Attrs["labels"], "container") {
			boxes[d] = true
			s.add(d)
		}
	}
	within := map[string]bool{input: true}
	for _, d := range inside {
		within[d] = true
		if ix.nodes[d].Kind == graph.NodeKindActor {
			s.add(d)
		}
	}
	for _, c := range append([]string{input}, inside...) {
		if isDesignContainer(ix.nodes[c]) {
			for _, o := range ix.neighbors(c, graph.EdgeKindUses, graph.EdgeKindDepends) {
				if isExternal(ix.nodes[o]) {
					s.add(o)
				}
			}
		}
	}
	for _, id := range slices.Sorted(maps.Keys(ix.nodes)) {
		if isPerson(ix.nodes[id]) && slices.ContainsFunc(ix.sources(id, graph.EdgeKindFrom), func(step string) bool {
			return slices.ContainsFunc(ix.targets(step, graph.EdgeKindCalls), func(op string) bool { return within[op] })
		}) {
			s.add(id)
		}
	}
	s.boxOf = func(id string) string {
		if ix.nodes[id].Kind == graph.NodeKindStep {
			for _, p := range ix.targets(id, graph.EdgeKindFrom) {
				if isPerson(ix.nodes[p]) {
					return p
				}
			}
			return ""
		}
		for _, c := range append([]string{id}, ix.containers(id)...) {
			if boxes[c] || ix.nodes[c].Kind == graph.NodeKindActor {
				return c
			}
		}
		return ""
	}
	return s
}
