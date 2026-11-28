package graph

import (
	"slices"

	"pgregory.net/rapid"
)

// EffectSample is a derived graph and the effects docs/model.md §9 D8 gives
// its operations, computed straight from the definition.
//
//uc:realizes graph.EffectSample
type EffectSample struct {
	Snapshot *Graph
	Expected map[string][]Effect
}

// genEffectSample draws the graph of genDeriveFragment with up to six more
// edges, each by even chance an implements edge from a struct to an
// interface or an extends edge between structs, and a random non-empty set
// of direct effects on about half of the concrete functions and methods; it
// derives the graph and computes the effects D8 defines.
//
//uc:generates graph.effects-follow-calls.sample
func genEffectSample() *rapid.Generator[EffectSample] {
	return rapid.Custom(func(t *rapid.T) EffectSample {
		s := &sampler{t: t, keys: map[edgeKey]bool{}}
		s.code(s.design())
		var structs, interfaces []string
		for _, n := range s.f.Nodes {
			switch {
			case n.Layer == LayerCode && n.Kind == NodeKindType && n.Form == "interface":
				interfaces = append(interfaces, n.Id)
			case n.Layer == LayerCode && n.Kind == NodeKindType:
				structs = append(structs, n.Id)
			}
		}
		for range rapid.IntRange(0, 6).Draw(t, "typeEdges") {
			if rapid.Bool().Draw(t, "implements") {
				s.edge(EdgeKindImplements, s.pick("implementer", structs), s.pick("interface", interfaces), SourceExtracted, nil)
			} else {
				s.edge(EdgeKindExtends, s.pick("embedder", structs), s.pick("embedded", structs), SourceExtracted, nil)
			}
		}
		for i := range s.f.Nodes {
			n := &s.f.Nodes[i]
			if n.Layer != LayerCode || n.Kind != NodeKindOp || n.Form == "abstract" || !rapid.Bool().Draw(t, "effectful") {
				continue
			}
			kinds := rapid.SliceOfNDistinct(rapid.SampledFrom(effectOrder), 1, len(effectOrder), rapid.ID).Draw(t, "directEffects")
			var written []string
			for _, e := range effectOrder {
				if slices.Contains(kinds, e) {
					written = append(written, string(e))
				}
			}
			n.Attrs["directEffects"] = written
		}
		g := NewGraph()
		if err := g.AddFragment(s.f); err != nil {
			panic(err)
		}
		g.Derive()
		return EffectSample{Snapshot: &g, Expected: definedEffects(&g)}
	})
}

// definedEffects computes D8 as docs/model.md §9 states it, function by
// function: the union of the direct effects of f and of every function
// reachable from f along calls, where a reached abstract method also reaches
// the same-named method of each code type that implements its interface,
// the type's own or the nearest one it embeds; a design operation has the
// union over the code operations that realize it.
func definedEffects(g *Graph) map[string][]Effect {
	nodes := map[string]Node{}
	for _, n := range g.Nodes() {
		nodes[n.Id] = n
	}
	code := func(id string, kind NodeKind) bool {
		n, ok := nodes[id]
		return ok && n.Layer == LayerCode && n.Kind == kind
	}
	edges := g.Edges()
	targets := func(kind EdgeKind, from string) []string {
		var out []string
		for _, e := range edges {
			if e.Kind == kind && e.From == from {
				out = append(out, e.To)
			}
		}
		return out
	}
	sources := func(kind EdgeKind, to string) []string {
		var out []string
		for _, e := range edges {
			if e.Kind == kind && e.To == to {
				out = append(out, e.From)
			}
		}
		return out
	}
	name := func(id string) string { return first(nodes[id].Attrs["name"]) }
	methodNamed := func(t, m string) []string {
		level, seen := []string{t}, map[string]bool{t: true}
		for len(level) > 0 {
			var found, next []string
			for _, x := range level {
				for _, c := range targets(EdgeKindContains, x) {
					if code(c, NodeKindOp) && name(c) == m {
						found = append(found, c)
					}
				}
				for _, y := range targets(EdgeKindExtends, x) {
					if code(y, NodeKindType) && !seen[y] {
						seen[y] = true
						next = append(next, y)
					}
				}
			}
			if len(found) > 0 {
				return found
			}
			level = next
		}
		return nil
	}
	runs := func(f string) []string {
		var out []string
		for _, g := range targets(EdgeKindCalls, f) {
			if code(g, NodeKindOp) {
				out = append(out, g)
			}
		}
		if nodes[f].Form == "abstract" {
			for _, i := range sources(EdgeKindContains, f) {
				for _, t := range sources(EdgeKindImplements, i) {
					if code(t, NodeKindType) && code(i, NodeKindType) {
						out = append(out, methodNamed(t, name(f))...)
					}
				}
			}
		}
		return out
	}
	effectsOf := func(f string) map[Effect]bool {
		has := map[Effect]bool{}
		seen := map[string]bool{f: true}
		for queue := []string{f}; len(queue) > 0; queue = queue[1:] {
			for _, e := range nodes[queue[0]].Attrs["directEffects"] {
				has[Effect(e)] = true
			}
			for _, g := range runs(queue[0]) {
				if !seen[g] {
					seen[g] = true
					queue = append(queue, g)
				}
			}
		}
		return has
	}
	ordered := func(has map[Effect]bool) []Effect {
		out := []Effect{}
		for _, e := range effectOrder {
			if has[e] {
				out = append(out, e)
			}
		}
		return out
	}
	out := map[string][]Effect{}
	for id, n := range nodes {
		switch {
		case n.Layer == LayerCode && n.Kind == NodeKindOp:
			out[id] = ordered(effectsOf(id))
		case n.Layer == LayerDesign && n.Kind == NodeKindOp:
			has, realized := map[Effect]bool{}, false
			for _, r := range sources(EdgeKindRealizes, id) {
				if code(r, NodeKindOp) {
					realized = true
					for e := range effectsOf(r) {
						has[e] = true
					}
				}
			}
			if realized {
				out[id] = ordered(has)
			}
		}
	}
	return out
}

func first(values []string) string {
	if len(values) > 0 {
		return values[0]
	}
	return ""
}
