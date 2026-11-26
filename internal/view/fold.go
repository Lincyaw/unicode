package view

import (
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
}

type edgeKey struct {
	kind     graph.EdgeKind
	from, to string
}

// unfolded are the views of a flow's participants, which the designer wrote
// into the flow and which therefore never fold by focus.
var unfolded = []ViewKind{ViewKindSequence, ViewKindActivity, ViewKindEvents}

// fold draws the salient nodes of a selection, folding support containers
// and leaving out utility ones unless the view is one of unfolded; then it
// draws what the expanded salient functions and reaches edges reveal; then
// every edge between drawn nodes that the selection or an expansion picks.
func (ix index) fold(kind ViewKind, input string, sel selection, expand []string) View {
	f := folding{ix: ix, byFocus: !slices.Contains(unfolded, kind), exempt: map[string]bool{input: true},
		drawn: map[string]bool{}, folded: map[string]bool{}, hidden: map[string]bool{}}
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
	v := View{Kind: kind, Nodes: []graph.Node{}, Edges: []graph.Edge{}, Folded: slices.Sorted(maps.Keys(f.folded)), Hidden: len(f.hidden)}
	if kind != ViewKindSequence && kind != ViewKindActivity {
		slices.Sort(f.order)
	}
	for _, id := range f.order {
		v.Nodes = append(v.Nodes, ix.nodes[id])
	}
	for _, e := range ix.edges {
		if f.drawn[e.From] && f.drawn[e.To] && (slices.Contains(sel.kinds, e.Kind) || revealed[edgeKey{e.Kind, e.From, e.To}]) {
			v.Edges = append(v.Edges, e)
		}
	}
	return v
}

// place draws id, or the support container it folds into, or records the
// utility container that keeps it out of the view.
func (f *folding) place(id string) {
	switch c, focus := f.cover(id); focus {
	case "support":
		f.draw(c)
		f.folded[c] = true
	case "utility":
		f.hidden[c] = true
	default:
		f.draw(id)
	}
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
		if focus := f.ix.focus(c); !f.exempt[c] && (focus == "support" || focus == "utility") {
			return c, focus
		}
	}
	return "", ""
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
