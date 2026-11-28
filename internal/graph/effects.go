package graph

import (
	"slices"
)

// Effect is a kind of effect (docs/model.md §3.1).
//
//uc:realizes graph.Effect
type Effect string

const (
	EffectTime    Effect = "time"
	EffectFs      Effect = "fs"
	EffectProcess Effect = "process"
	EffectNetwork Effect = "network"
)

// effectOrder is the order a set of effects is written in: the order of the
// enum (docs/design.md §5.2).
var effectOrder = []Effect{EffectTime, EffectFs, EffectProcess, EffectNetwork}

// effectSet holds a set of effects, one bit for each in effectOrder.
type effectSet uint8

func effectsOf(kinds []string) effectSet {
	var s effectSet
	for i, e := range effectOrder {
		if slices.Contains(kinds, string(e)) {
			s |= 1 << i
		}
	}
	return s
}

func (s effectSet) list() []Effect {
	out := []Effect{}
	for i, e := range effectOrder {
		if s&(1<<i) != 0 {
			out = append(out, e)
		}
	}
	return out
}

// Effects gives every code operation, and every design operation that a code
// operation realizes, its effects (docs/model.md §9 D8). A code operation's
// are the direct effects of the functions it reaches along calls, itself
// included; a call of an interface's abstract method reaches the same-named
// method of each code type that implements the interface, its own or the
// nearest one it gets from the types it embeds. A design operation's are
// those of the code operations that realize it. A design operation that
// nothing realizes has none to give, and is left out.
func (g *Graph) Effects() map[string][]Effect {
	e := newEffectGraph(g)
	out := map[string][]Effect{}
	for _, f := range e.functions {
		out[f] = e.of(f).list()
	}
	for _, n := range g.nodes {
		if n.Layer != LayerDesign || n.Kind != NodeKindOp {
			continue
		}
		var s effectSet
		realized := false
		for _, r := range e.realizers[n.Id] {
			realized = true
			s |= e.of(r)
		}
		if realized {
			out[n.Id] = s.list()
		}
	}
	return out
}

// effectGraph is the code operations with the functions a call of each may
// run: what it calls and, for an abstract method, its dispatch.
type effectGraph struct {
	functions []string
	next      map[string][]string
	direct    map[string]effectSet
	realizers map[string][]string
	// The strongly connected components share their effects: comp names each
	// function's, reached holds each component's, once computed.
	comp    map[string]int
	reached []effectSet
}

func newEffectGraph(g *Graph) *effectGraph {
	e := &effectGraph{next: map[string][]string{}, direct: map[string]effectSet{}, realizers: map[string][]string{}}
	isCode := func(id string, kind NodeKind) bool {
		n, ok := g.nodes[id]
		return ok && n.Layer == LayerCode && n.Kind == kind
	}
	children := map[string][]string{}
	parent := map[string]string{}
	implementers := map[string][]string{}
	embeds := map[string][]string{}
	for _, edge := range g.Edges() {
		switch {
		case edge.Kind == EdgeKindCalls && isCode(edge.From, NodeKindOp) && isCode(edge.To, NodeKindOp):
			e.next[edge.From] = append(e.next[edge.From], edge.To)
		case edge.Kind == EdgeKindContains:
			children[edge.From] = append(children[edge.From], edge.To)
			parent[edge.To] = edge.From
		case edge.Kind == EdgeKindImplements && isCode(edge.From, NodeKindType) && isCode(edge.To, NodeKindType):
			implementers[edge.To] = append(implementers[edge.To], edge.From)
		case edge.Kind == EdgeKindExtends && isCode(edge.From, NodeKindType) && isCode(edge.To, NodeKindType):
			embeds[edge.From] = append(embeds[edge.From], edge.To)
		case edge.Kind == EdgeKindRealizes && isCode(edge.From, NodeKindOp):
			e.realizers[edge.To] = append(e.realizers[edge.To], edge.From)
		}
	}
	name := func(id string) string {
		if v := g.nodes[id].Attrs["name"]; len(v) > 0 {
			return v[0]
		}
		return ""
	}
	// methodNamed lists the methods named m of a code type: its own or,
	// failing those, the nearest it gets from the types it embeds.
	methodNamed := func(t, m string) []string {
		seen := map[string]bool{t: true}
		for level := []string{t}; len(level) > 0; {
			var out, deeper []string
			for _, x := range level {
				for _, c := range children[x] {
					if isCode(c, NodeKindOp) && name(c) == m {
						out = append(out, c)
					}
				}
				for _, y := range embeds[x] {
					if !seen[y] {
						seen[y] = true
						deeper = append(deeper, y)
					}
				}
			}
			if len(out) > 0 {
				return out
			}
			level = deeper
		}
		return nil
	}
	for id, n := range g.nodes {
		if n.Layer != LayerCode || n.Kind != NodeKindOp {
			continue
		}
		e.functions = append(e.functions, id)
		e.direct[id] = effectsOf(n.Attrs["directEffects"])
		if n.Form == "abstract" {
			for _, t := range implementers[parent[id]] {
				e.next[id] = append(e.next[id], methodNamed(t, name(id))...)
			}
		}
	}
	slices.Sort(e.functions)
	e.components()
	return e
}

// components finds the strongly connected components of the functions along
// next (Tarjan), and gives each the effects of its members and of every
// component it reaches; Tarjan closes a component only after those it
// reaches.
func (e *effectGraph) components() {
	e.comp = map[string]int{}
	index := map[string]int{}
	low := map[string]int{}
	onStack := map[string]bool{}
	var stack []string
	var visit func(f string)
	visit = func(f string) {
		index[f], low[f] = len(index), len(index)
		stack = append(stack, f)
		onStack[f] = true
		for _, g := range e.next[f] {
			if _, seen := index[g]; !seen {
				visit(g)
				low[f] = min(low[f], low[g])
			} else if onStack[g] {
				low[f] = min(low[f], index[g])
			}
		}
		if low[f] != index[f] {
			return
		}
		c := len(e.reached)
		var s effectSet
		var members []string
		for {
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[top] = false
			e.comp[top] = c
			members = append(members, top)
			s |= e.direct[top]
			if top == f {
				break
			}
		}
		e.reached = append(e.reached, 0)
		for _, m := range members {
			for _, g := range e.next[m] {
				if d, ok := e.comp[g]; ok && d != c {
					s |= e.reached[d]
				}
			}
		}
		e.reached[c] = s
	}
	for _, f := range e.functions {
		if _, seen := index[f]; !seen {
			visit(f)
		}
	}
}

func (e *effectGraph) of(f string) effectSet { return e.reached[e.comp[f]] }
