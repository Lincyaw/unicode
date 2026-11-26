package graph

// AddFragment adds a fragment's nodes, then its edges, and adds nothing when
// it returns an error. Two kinds of edges wait for Derive instead of joining
// the graph at once (docs/design.md §4, docs/model.md §9):
//   - a binding (realizes, checks, generates) whose target is not a node of the
//     graph or the fragment; Derive adds it once its target is a node, and
//     reports it otherwise;
//   - a derived realizes, the candidate a backend gives for a method; Derive
//     adds it only where a method realizes an operation by §9.
//
// Any other edge must start and end at nodes of the graph or the fragment.
func (g *Graph) AddFragment(fragment Fragment) error {
	ids := map[string]bool{}
	for _, n := range fragment.Nodes {
		if _, ok := g.nodes[n.Id]; ok || ids[n.Id] {
			return DuplicateNode{Id: n.Id}
		}
		ids[n.Id] = true
	}
	isNode := func(id string) bool {
		_, ok := g.nodes[id]
		return ok || ids[id]
	}
	keys := map[edgeKey]bool{}
	for _, e := range g.pending {
		keys[edgeKey{e.Kind, e.From, e.To}] = true
	}
	var edges, pending []Edge
	for _, e := range fragment.Edges {
		key := edgeKey{e.Kind, e.From, e.To}
		if _, ok := g.edges[key]; ok || keys[key] {
			return DuplicateEdge{Kind: e.Kind, From: e.From, To: e.To}
		}
		keys[key] = true
		switch {
		case !isNode(e.From):
			return DanglingEdge{Kind: e.Kind, From: e.From, To: e.To, Missing: e.From}
		case isCandidate(e), isBinding(e.Kind) && !isNode(e.To):
			pending = append(pending, e)
		case !isNode(e.To):
			return DanglingEdge{Kind: e.Kind, From: e.From, To: e.To, Missing: e.To}
		default:
			edges = append(edges, e)
		}
	}
	for _, n := range fragment.Nodes {
		if err := g.AddNode(n); err != nil {
			panic("graph: AddFragment checked every identity, yet " + err.Error())
		}
	}
	for _, e := range edges {
		if err := g.AddEdge(e); err != nil {
			panic("graph: AddFragment checked every edge, yet " + err.Error())
		}
	}
	for _, e := range pending {
		e.Attrs = cloneAttrs(e.Attrs)
		g.pending = append(g.pending, e)
	}
	return nil
}

func isBinding(kind EdgeKind) bool {
	return kind == EdgeKindRealizes || kind == EdgeKindChecks || kind == EdgeKindGenerates
}

// isCandidate tells whether e is a backend's candidate for a method's realizes.
func isCandidate(e Edge) bool { return e.Kind == EdgeKindRealizes && e.Source == SourceDerived }
