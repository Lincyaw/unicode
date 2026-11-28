package graph

import (
	"cmp"
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

// EffectOrder is the order a set of effects is written in: the order of the
// enum (docs/design.md §5.2).
var EffectOrder = []Effect{EffectTime, EffectFs, EffectProcess, EffectNetwork}

// EffectWitness is where an operation's effect of one kind comes from
// (docs/model.md §9 D8): from its own direct effects, with rank 0 and no
// via, or from its successor via, whose rank for the kind is one less.
//
//uc:realizes graph.EffectWitness
type EffectWitness struct {
	Kind Effect
	Via  string
	Rank int
}

// OperationEffects is what D8 gives one operation: its direct effects, its
// successors, its effects, and a witness for each of them.
//
//uc:realizes graph.OperationEffects
type OperationEffects struct {
	Direct     []Effect
	Successors []string
	Effects    []Effect
	Witnesses  []EffectWitness
}

// Effects gives every code operation, and every design operation that a code
// operation realizes, its effects (docs/model.md §9 D8). A code operation's
// successors are the code operations it calls and, for an interface's
// abstract method, the same-named method of each code type that implements
// the interface, its own or the nearest one it gets from the types it
// embeds; a design operation's are the code operations that realize it. An
// operation has an effect of a kind when the kind is among its direct
// effects, or when one of its successors has it; its witness is the
// successor nearest to a direct effect of the kind, the first by identity
// among equals. A design operation that nothing realizes has none to give,
// and is left out.
func (g *Graph) Effects() map[string]OperationEffects {
	e := newEffectGraph(g, true)
	out := map[string]OperationEffects{}
	for _, id := range e.ops {
		r := OperationEffects{Direct: e.direct[id].list(), Successors: slices.Clone(e.next[id]), Effects: []Effect{}, Witnesses: []EffectWitness{}}
		for _, k := range EffectOrder {
			if w, ok := e.witness[k][id]; ok {
				r.Effects = append(r.Effects, k)
				r.Witnesses = append(r.Witnesses, w)
			}
		}
		out[id] = r
	}
	return out
}

// Dispatch lists the methods a call of the interface's abstract method id
// may run, by identity: the same-named method of each code type that
// implements the interface, its own or, failing those, the nearest that it
// gets from the types it embeds, as Go selects them. Any other operation
// has none.
func (g *Graph) Dispatch(id string) []string { return newEffectGraph(g, false).dispatch(id) }

// effectSet holds a set of effects, one bit for each in EffectOrder.
type effectSet uint8

func effectsOf(kinds []string) effectSet {
	var s effectSet
	for i, e := range EffectOrder {
		if slices.Contains(kinds, string(e)) {
			s |= 1 << i
		}
	}
	return s
}

func (s effectSet) list() []Effect {
	out := []Effect{}
	for i, e := range EffectOrder {
		if s&(1<<i) != 0 {
			out = append(out, e)
		}
	}
	return out
}

// effectGraph is the operations with their successors and direct effects,
// and, by kind, the witness of every operation that has the kind.
type effectGraph struct {
	g            *Graph
	ops          []string
	next         map[string][]string
	direct       map[string]effectSet
	children     map[string][]string
	parent       map[string]string
	implementers map[string][]string
	embeds       map[string][]string
	witness      map[Effect]map[string]EffectWitness
}

// newEffectGraph indexes g; it finds the operations' successors and
// witnesses only when full, since dispatch needs neither.
func newEffectGraph(g *Graph, full bool) *effectGraph {
	e := &effectGraph{g: g, next: map[string][]string{}, direct: map[string]effectSet{}, children: map[string][]string{},
		parent: map[string]string{}, implementers: map[string][]string{}, embeds: map[string][]string{}}
	realizers := map[string][]string{}
	for _, edge := range g.Edges() {
		switch {
		case edge.Kind == EdgeKindCalls && e.code(edge.From, NodeKindOp) && e.code(edge.To, NodeKindOp):
			e.next[edge.From] = append(e.next[edge.From], edge.To)
		case edge.Kind == EdgeKindContains:
			e.children[edge.From] = append(e.children[edge.From], edge.To)
			e.parent[edge.To] = edge.From
		case edge.Kind == EdgeKindImplements && e.code(edge.From, NodeKindType) && e.code(edge.To, NodeKindType):
			e.implementers[edge.To] = append(e.implementers[edge.To], edge.From)
		case edge.Kind == EdgeKindExtends && e.code(edge.From, NodeKindType) && e.code(edge.To, NodeKindType):
			e.embeds[edge.From] = append(e.embeds[edge.From], edge.To)
		case edge.Kind == EdgeKindRealizes && e.code(edge.From, NodeKindOp):
			realizers[edge.To] = append(realizers[edge.To], edge.From)
		}
	}
	if !full {
		return e
	}
	for id, n := range g.nodes {
		switch {
		case n.Layer == LayerCode && n.Kind == NodeKindOp:
			e.ops = append(e.ops, id)
			e.direct[id] = effectsOf(n.Attrs["directEffects"])
		case n.Layer == LayerDesign && n.Kind == NodeKindOp && len(realizers[id]) > 0:
			e.ops = append(e.ops, id)
			e.next[id] = realizers[id]
		}
	}
	slices.Sort(e.ops)
	for _, id := range e.ops {
		e.next[id] = append(e.next[id], e.dispatch(id)...)
		slices.Sort(e.next[id])
		e.next[id] = slices.Compact(e.next[id])
	}
	e.witnesses()
	return e
}

func (e *effectGraph) code(id string, kind NodeKind) bool {
	n, ok := e.g.nodes[id]
	return ok && n.Layer == LayerCode && n.Kind == kind
}

func (e *effectGraph) name(id string) string {
	if v := e.g.nodes[id].Attrs["name"]; len(v) > 0 {
		return v[0]
	}
	return ""
}

func (e *effectGraph) dispatch(id string) []string {
	n, ok := e.g.nodes[id]
	if !ok || n.Layer != LayerCode || n.Form != "abstract" {
		return nil
	}
	var out []string
	for _, t := range e.implementers[e.parent[id]] {
		out = append(out, e.methodNamed(t, e.name(id))...)
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// methodNamed lists the methods named m of a code type: its own or, failing
// those, the nearest it gets from the types it embeds.
func (e *effectGraph) methodNamed(t, m string) []string {
	seen := map[string]bool{t: true}
	for level := []string{t}; len(level) > 0; {
		var out, deeper []string
		for _, x := range level {
			for _, c := range e.children[x] {
				if e.code(c, NodeKindOp) && e.name(c) == m {
					out = append(out, c)
				}
			}
			for _, y := range e.embeds[x] {
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

// witnesses finds, for each kind, the operations that have it by a search
// backwards from those that have it directly: an operation reached in round
// r has rank r, and its witness is the successor of rank r-1 that reached
// it first, the first by identity among them.
func (e *effectGraph) witnesses() {
	before := map[string][]string{}
	for _, id := range e.ops {
		for _, s := range e.next[id] {
			before[s] = append(before[s], id)
		}
	}
	e.witness = map[Effect]map[string]EffectWitness{}
	for i, k := range EffectOrder {
		found := map[string]EffectWitness{}
		var frontier []string
		for _, id := range e.ops {
			if e.direct[id]&(1<<i) != 0 {
				found[id] = EffectWitness{Kind: k, Rank: 0}
				frontier = append(frontier, id)
			}
		}
		for rank := 1; len(frontier) > 0; rank++ {
			reached := map[string]string{}
			for _, s := range frontier {
				for _, p := range before[s] {
					if _, done := found[p]; done {
						continue
					}
					if via, ok := reached[p]; !ok || cmp.Less(s, via) {
						reached[p] = s
					}
				}
			}
			frontier = frontier[:0]
			for p, via := range reached {
				found[p] = EffectWitness{Kind: k, Via: via, Rank: rank}
				frontier = append(frontier, p)
			}
			slices.Sort(frontier)
		}
		e.witness[k] = found
	}
}
