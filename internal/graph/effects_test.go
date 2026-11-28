package graph

import (
	"slices"

	"pgregory.net/rapid"
)

// EffectSample is a derived graph, the direct effects written on each of
// its code operations when it was drawn, and one of its abstract methods.
//
//uc:realizes graph.EffectSample
type EffectSample struct {
	Snapshot *Graph
	Direct   map[string][]Effect
	Abstract string
}

// genEffectSample draws the graph of genDeriveFragment with up to six more
// edges, each by even chance an implements edge from a struct to an
// interface or an extends edge between structs, and a random non-empty set
// of direct effects on about half of the concrete functions and methods,
// derives it, and picks one of its abstract methods, or none when it has
// none.
//
//uc:generates graph.effects-follow-calls.sample
//uc:generates graph.effects-read-the-graph.sample
//uc:generates graph.dispatch-finds-implementations.sample
func genEffectSample() *rapid.Generator[EffectSample] {
	return rapid.Custom(func(t *rapid.T) EffectSample {
		s := &sampler{t: t, keys: map[edgeKey]bool{}}
		s.code(s.design())
		var structs, interfaces, abstract []string
		for _, n := range s.f.Nodes {
			switch {
			case n.Layer == LayerCode && n.Kind == NodeKindType && n.Form == "interface":
				interfaces = append(interfaces, n.Id)
			case n.Layer == LayerCode && n.Kind == NodeKindType:
				structs = append(structs, n.Id)
			case n.Layer == LayerCode && n.Form == "abstract":
				abstract = append(abstract, n.Id)
			}
		}
		for range rapid.IntRange(0, 6).Draw(t, "typeEdges") {
			if rapid.Bool().Draw(t, "implements") {
				s.edge(EdgeKindImplements, s.pick("implementer", structs), s.pick("interface", interfaces), SourceExtracted, nil)
			} else {
				s.edge(EdgeKindExtends, s.pick("embedder", structs), s.pick("embedded", structs), SourceExtracted, nil)
			}
		}
		direct := map[string][]Effect{}
		for i := range s.f.Nodes {
			n := &s.f.Nodes[i]
			if n.Layer != LayerCode || n.Kind != NodeKindOp {
				continue
			}
			direct[n.Id] = []Effect{}
			if n.Form == "abstract" || !rapid.Bool().Draw(t, "effectful") {
				continue
			}
			kinds := rapid.SliceOfNDistinct(rapid.SampledFrom(EffectOrder), 1, len(EffectOrder), rapid.ID).Draw(t, "directEffects")
			for _, e := range EffectOrder {
				if slices.Contains(kinds, e) {
					n.Attrs["directEffects"] = append(n.Attrs["directEffects"], string(e))
					direct[n.Id] = append(direct[n.Id], e)
				}
			}
		}
		g := NewGraph()
		if err := g.AddFragment(s.f); err != nil {
			panic(err)
		}
		g.Derive()
		return EffectSample{Snapshot: &g, Direct: direct, Abstract: s.pick("abstract", abstract)}
	})
}
