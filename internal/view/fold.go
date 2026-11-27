package view

import (
	"cmp"
	"maps"
	"slices"
	"strings"

	"github.com/lincyaw/unicode/internal/graph"
)

// folding is the state of drawing one selection (docs/design.md §7.2).
// Containers fold by focus only when byFocus is set, and those in exempt
// never fold: the input, what encloses it, and the expanded identities.
type folding struct {
	ix      index
	byFocus bool
	exempt  map[string]bool
	order   []string
	drawn   map[string]bool
	folded  map[string]bool
	hidden  map[string]bool
	// into maps a node placed into a folded container to that container,
	// and a node that a utility container keeps out of the view to "".
	into map[string]string
	// home is the selection's home (see selection).
	home string
}

type edgeKey struct {
	kind     graph.EdgeKind
	from, to string
}

// focusFolding are the views that fold by focus (docs/design.md §7.2). The views of
// a flow and the property view do not: what they draw was written by the
// designer into the flow or the property.
var focusFolding = []ViewKind{ViewKindArchitecture, ViewKindClass, ViewKindCalls, ViewKindCode, ViewKindDdd, ViewKindC4}

// fold draws the salient nodes of a selection, folding support containers
// and leaving out utility ones when the view is one of focusFolding; then it
// draws what the expanded salient functions and reaches edges reveal; then
// the edges that the selection or an expansion picks, between the nodes
// that stand for their ends.
func (ix index) fold(r request, sel selection, expand []string) View {
	kind, input := r.kind, r.input
	f := folding{ix: ix, byFocus: slices.Contains(focusFolding, kind), exempt: map[string]bool{input: true},
		drawn: map[string]bool{}, folded: map[string]bool{}, hidden: map[string]bool{}, into: map[string]string{}, home: sel.home}
	for _, id := range append(ix.enclosing(input), expand...) {
		f.exempt[id] = true
	}
	for _, id := range sel.nodes {
		if ix.nodes[id].Salient {
			f.place(id)
		}
	}
	revealed := map[edgeKey]bool{}
	for _, x := range expand {
		nodes, edges := f.reveal(x, sel)
		for _, id := range nodes {
			f.place(id)
		}
		for _, e := range edges {
			revealed[e] = true
		}
	}
	v := View{Kind: kind, Nodes: []ViewNode{}, Edges: []ViewEdge{}, Folded: append([]string{}, slices.Sorted(maps.Keys(f.folded))...), Hidden: len(f.hidden)}
	if kind != ViewKindSequence && kind != ViewKindActivity {
		slices.Sort(f.order)
	}
	for _, id := range f.order {
		v.Nodes = append(v.Nodes, ViewNode{Node: ix.nodes[id]})
	}
	v.Edges = f.edges(sel, revealed)
	return v
}

// place draws id, or the support container it folds into, or records the
// utility container that keeps it out of the view.
func (f *folding) place(id string) {
	switch c, focus := f.cover(id); focus {
	case "support":
		f.draw(c)
		f.folded[c] = true
		if c != id {
			f.into[id] = c
		}
	case "utility":
		f.hidden[c] = true
		f.into[id] = ""
	default:
		f.draw(id)
	}
}

// stand returns the drawn node that stands for id: id itself, the folded
// container it was placed into, or what stands for the node the selection
// maps it to; "" when the view draws none of them.
func (f *folding) stand(id string, sel selection) string {
	if f.drawn[id] {
		return id
	}
	if c, placed := f.into[id]; placed {
		return c
	}
	if sel.boxOf != nil {
		if b := sel.boxOf(id); b != "" && b != id {
			return f.stand(b, sel)
		}
	}
	return ""
}

// edges draws each edge of the kinds the selection picks, and each edge an
// expansion reveals, between the nodes that stand for its ends. The edges
// that come to join the same two nodes because an end was folded merge
// into one edge, which counts them; an edge folded into a single node is
// not drawn.
func (f *folding) edges(sel selection, revealed map[edgeKey]bool) []ViewEdge {
	groups := map[edgeKey][]graph.Edge{}
	moved := map[edgeKey]bool{}
	for _, e := range f.ix.edges {
		picked := slices.Contains(sel.kinds, e.Kind) && (sel.keeps == nil || sel.keeps(e))
		if !picked && !revealed[edgeKey{e.Kind, e.From, e.To}] {
			continue
		}
		from, to := f.stand(e.From, sel), f.stand(e.To, sel)
		shifted := from != e.From || to != e.To
		if from == "" || to == "" || shifted && from == to {
			continue
		}
		k := edgeKey{e.Kind, from, to}
		groups[k] = append(groups[k], e)
		moved[k] = moved[k] || shifted
	}
	drawn := []ViewEdge{}
	for _, k := range slices.SortedFunc(maps.Keys(groups), edgeKey.compare) {
		e := ViewEdge{Edge: groups[k][0], Labels: f.ix.labels(k.from, groups[k])}
		if moved[k] {
			e.Edge, e.Count = merged(k, groups[k]), len(groups[k])
		}
		drawn = append(drawn, e)
	}
	return drawn
}

func (k edgeKey) compare(o edgeKey) int {
	return cmp.Or(cmp.Compare(k.kind, o.kind), cmp.Compare(k.from, o.from), cmp.Compare(k.to, o.to))
}

// merged is the edge k that stands for the edges of the graph es: derived
// from them, at the origin of the first, with the values of their
// attributes in the order they first appear.
func merged(k edgeKey, es []graph.Edge) graph.Edge {
	e := graph.Edge{Kind: k.kind, From: k.from, To: k.to, Source: graph.SourceDerived, Origin: es[0].Origin}
	for _, x := range es {
		for _, key := range slices.Sorted(maps.Keys(x.Attrs)) {
			if e.Attrs == nil {
				e.Attrs = map[string][]string{}
			}
			for _, v := range x.Attrs[key] {
				if !slices.Contains(e.Attrs[key], v) {
					e.Attrs[key] = append(e.Attrs[key], v)
				}
			}
		}
	}
	return e
}

func (f *folding) draw(id string) {
	if !f.drawn[id] {
		f.drawn[id] = true
		f.order = append(f.order, id)
	}
}

// cover returns the outermost container, among those enclosing id and id
// itself, that is not exempt and whose focus is support or utility, with that
// focus; "", "" when there is none or the view does not fold by focus.
func (f *folding) cover(id string) (string, string) {
	if !f.byFocus {
		return "", ""
	}
	chain := f.ix.enclosing(id)
	slices.Reverse(chain)
	for _, c := range append(chain, id) {
		if focus := f.focus(c); !f.exempt[c] && (focus == "support" || focus == "utility") {
			return c, focus
		}
	}
	return "", ""
}

// focus is the focus by which a container folds: its own, except that a
// design container outside the selection's home folds as support unless it
// is a utility.
func (f *folding) focus(c string) string {
	focus := f.ix.focus(c)
	if f.home != "" && focus != "utility" && isDesignContainer(f.ix.nodes[c]) && c != f.home && !slices.Contains(f.ix.containers(c), f.home) {
		return "support"
	}
	return focus
}

// reveal returns the detail functions, and the calls edges to them, that
// expanding x shows: the direct detail callees of a drawn salient function,
// or the detail functions on the paths of a drawn reaches edge.
func (f *folding) reveal(x string, sel selection) ([]string, []edgeKey) {
	ix := f.ix
	if from, to, ok := reachesEdge(x); ok {
		if !f.drawn[from] || !f.drawn[to] || !slices.Contains(sel.kinds, graph.EdgeKindReaches) || !slices.Contains(ix.targets(from, graph.EdgeKindReaches), to) {
			return nil, nil
		}
		return ix.between(from, to)
	}
	if n := ix.nodes[x]; !f.drawn[x] || n.Kind != graph.NodeKindOp || !n.Salient {
		return nil, nil
	}
	var nodes []string
	var edges []edgeKey
	for _, c := range ix.targets(x, graph.EdgeKindCalls) {
		if isDetailFunction(ix.nodes[c]) {
			nodes = append(nodes, c)
			edges = append(edges, edgeKey{graph.EdgeKindCalls, x, c})
		}
	}
	return nodes, edges
}

// reachesEdge reads the identity of an expanded reaches edge, "reaches <from> <to>".
func reachesEdge(x string) (string, string, bool) {
	rest, ok := strings.CutPrefix(x, "reaches ")
	if !ok {
		return "", "", false
	}
	return strings.Cut(rest, " ")
}

func isDetailFunction(n graph.Node) bool { return n.Kind == graph.NodeKindOp && !n.Salient }

// between returns the detail functions on the paths of calls from the
// function from to the function to that pass only through detail functions,
// and the calls edges of those paths.
func (ix index) between(from, to string) ([]string, []edgeKey) {
	after := ix.detailClosure(from, ix.targets)
	before := ix.detailClosure(to, ix.sources)
	var nodes []string
	for _, id := range slices.Sorted(maps.Keys(after)) {
		if before[id] {
			nodes = append(nodes, id)
		}
	}
	on := func(id string) bool { return after[id] && before[id] }
	var edges []edgeKey
	for _, e := range ix.edges {
		if e.Kind == graph.EdgeKindCalls && (e.From == from || on(e.From)) && (e.To == to || on(e.To)) && (on(e.From) || on(e.To)) {
			edges = append(edges, edgeKey{e.Kind, e.From, e.To})
		}
	}
	return nodes, edges
}

// detailClosure returns the detail functions reached from start by calls
// edges, followed by step, through detail functions only.
func (ix index) detailClosure(start string, step func(string, graph.EdgeKind) []string) map[string]bool {
	seen := map[string]bool{}
	for queue := []string{start}; len(queue) > 0; queue = queue[1:] {
		for _, next := range step(queue[0], graph.EdgeKindCalls) {
			if isDetailFunction(ix.nodes[next]) && !seen[next] {
				seen[next] = true
				queue = append(queue, next)
			}
		}
	}
	return seen
}
