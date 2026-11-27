package view

import (
	"maps"
	"slices"
	"strings"

	"github.com/lincyaw/unicode/internal/graph"
)

// world answers, straight from the definitions of docs/design.md §7.1–§7.2,
// what a view should draw; it reads the fragment's lists and keeps no index.
type world struct {
	nodes  map[string]graph.Node
	edges  []graph.Edge
	system string
}

func newWorld(f graph.Fragment) world {
	w := world{nodes: map[string]graph.Node{}, edges: f.Edges}
	for _, n := range f.Nodes {
		w.nodes[n.Id] = n
		if n.Kind == graph.NodeKindContainer && n.Layer == graph.LayerDesign && n.Form == "system" && (w.system == "" || n.Id < w.system) {
			w.system = n.Id
		}
	}
	return w
}

// to returns the ends of the edges of the kind from id, sorted.
func (w world) to(id string, kind graph.EdgeKind) []string {
	var ids []string
	for _, e := range w.edges {
		if e.Kind == kind && e.From == id {
			ids = append(ids, e.To)
		}
	}
	slices.Sort(ids)
	return ids
}

// from returns the starts of the edges of the kind to id, sorted.
func (w world) from(id string, kind graph.EdgeKind) []string {
	var ids []string
	for _, e := range w.edges {
		if e.Kind == kind && e.To == id {
			ids = append(ids, e.From)
		}
	}
	slices.Sort(ids)
	return ids
}

// adjacent returns the nodes one edge of the kinds away from id, either way.
func (w world) adjacent(id string, kinds ...graph.EdgeKind) []string {
	var ids []string
	for _, k := range kinds {
		ids = append(append(ids, w.to(id, k)...), w.from(id, k)...)
	}
	return ids
}

// below returns everything id contains along chains of contains edges.
func (w world) below(id string) map[string]bool {
	found := map[string]bool{}
	for grown := true; grown; {
		grown = false
		for _, e := range w.edges {
			if e.Kind == graph.EdgeKindContains && (e.From == id || found[e.From]) && e.To != id && !found[e.To] {
				found[e.To], grown = true, true
			}
		}
	}
	return found
}

// up returns what encloses id, innermost first: a file's owner, else the
// container along contains, and so on.
func (w world) up(id string) []string {
	var chain []string
	for at := id; ; {
		next := ""
		if owners := w.from(at, graph.EdgeKindOwns); len(owners) > 0 {
			next = owners[0]
		} else if parents := w.from(at, graph.EdgeKindContains); len(parents) > 0 {
			next = parents[0]
		}
		if next == "" || next == id || slices.Contains(chain, next) {
			return chain
		}
		chain = append(chain, next)
		at = next
	}
}

func (w world) designContainer(id string) bool {
	n, ok := w.nodes[id]
	return ok && n.Kind == graph.NodeKindContainer && n.Layer == graph.LayerDesign
}

func (w world) external(id string) bool {
	n := w.nodes[id]
	return n.Kind == graph.NodeKindActor && n.Form == "external"
}

// focus is a design container's own focus, else its nearest enclosing design container's.
func (w world) focus(id string) string {
	if !w.designContainer(id) {
		return ""
	}
	if f := w.nodes[id].Attrs["focus"]; len(f) > 0 {
		return f[0]
	}
	for _, p := range w.from(id, graph.EdgeKindContains) {
		if p != id && w.designContainer(p) {
			return w.focus(p)
		}
	}
	return ""
}

// applicable resolves the input of a view and tells whether the view applies to it.
func (w world) applicable(kind ViewKind, id string) (string, bool) {
	if id == "" && (kind == ViewKindArchitecture || kind == ViewKindEvents || kind == ViewKindDdd || kind == ViewKindC4) {
		id = w.system
	}
	n, ok := w.nodes[id]
	if !ok || !n.Salient {
		return id, false
	}
	switch kind {
	case ViewKindArchitecture, ViewKindEvents:
		return id, w.designContainer(id)
	case ViewKindDdd, ViewKindC4:
		return id, w.designContainer(id) && slices.Contains(w.nodes[w.system].Attrs["notations"], string(kind))
	case ViewKindClass:
		return id, n.Layer == graph.LayerDesign && (n.Kind == graph.NodeKindContainer || n.Kind == graph.NodeKindType)
	case ViewKindSequence, ViewKindActivity:
		return id, n.Kind == graph.NodeKindFlow
	case ViewKindCalls:
		return id, n.Kind == graph.NodeKindOp
	case ViewKindProperty:
		return id, n.Kind == graph.NodeKindProperty || w.designContainer(id)
	case ViewKindCode:
		return id, n.Layer == graph.LayerCode && (n.Kind == graph.NodeKindType || n.Kind == graph.NodeKindContainer && n.Form == "file")
	}
	return id, false
}

// drawnKinds are the kinds of the edges each view draws between its nodes.
var drawnKinds = map[ViewKind][]graph.EdgeKind{
	ViewKindArchitecture: {graph.EdgeKindUses, graph.EdgeKindDepends, graph.EdgeKindContains},
	ViewKindClass:        {graph.EdgeKindRefers, graph.EdgeKindExtends, graph.EdgeKindImplements, graph.EdgeKindContains},
	ViewKindSequence: {graph.EdgeKindContains, graph.EdgeKindFrom, graph.EdgeKindCalls, graph.EdgeKindEmits,
		graph.EdgeKindHandles, graph.EdgeKindSubscribes, graph.EdgeKindTraces, graph.EdgeKindCovers},
	ViewKindActivity: {graph.EdgeKindContains, graph.EdgeKindFrom, graph.EdgeKindCalls, graph.EdgeKindEmits,
		graph.EdgeKindHandles, graph.EdgeKindSubscribes, graph.EdgeKindTraces, graph.EdgeKindCovers},
	ViewKindEvents:   {graph.EdgeKindEmits, graph.EdgeKindSubscribes, graph.EdgeKindHandles},
	ViewKindCalls:    {graph.EdgeKindCalls, graph.EdgeKindReaches, graph.EdgeKindRealizes},
	ViewKindProperty: {graph.EdgeKindAbout, graph.EdgeKindContains, graph.EdgeKindChecks},
	ViewKindCode:     {graph.EdgeKindContains, graph.EdgeKindCalls, graph.EdgeKindReaches, graph.EdgeKindCovers},
	ViewKindDdd:      {graph.EdgeKindRefers, graph.EdgeKindExtends, graph.EdgeKindImplements, graph.EdgeKindContains},
	ViewKindC4:       {graph.EdgeKindUses, graph.EdgeKindDepends, graph.EdgeKindCalls},
}

// stepsOf returns the steps of a flow in the order of their index.
func (w world) stepsOf(flow string) []string {
	var found []string
	for id := range w.below(flow) {
		if w.nodes[id].Kind == graph.NodeKindStep {
			found = append(found, id)
		}
	}
	slices.SortFunc(found, func(a, b string) int {
		ia, ib := w.nodes[a].Attrs["index"][0], w.nodes[b].Attrs["index"][0]
		if d := len(ia) - len(ib); d != 0 {
			return d
		}
		return strings.Compare(ia, ib)
	})
	return found
}

// participants returns, step by step in order, each step's senders and then
// its receivers: the owners of the operations it calls and the events it emits.
func (w world) participants(flow string) []string {
	var found []string
	for _, s := range w.stepsOf(flow) {
		found = append(found, w.to(s, graph.EdgeKindFrom)...)
		for _, op := range w.to(s, graph.EdgeKindCalls) {
			found = append(found, w.from(op, graph.EdgeKindContains)...)
		}
		found = append(found, w.to(s, graph.EdgeKindEmits)...)
	}
	return found
}

// asked is a view request as the oracle reads it: the kind, the resolved
// input, the class view's layer and the C4 view's level.
type asked struct {
	kind  ViewKind
	input string
	layer ClassLayer
	level C4Level
}

// candidates returns what the view selects for the input before folding (§7.1).
func (w world) candidates(a asked) map[string]bool {
	kind, input := a.kind, a.input
	c := map[string]bool{input: true}
	below := w.below(input)
	add := func(ids ...string) {
		for _, id := range ids {
			c[id] = true
		}
	}
	switch kind {
	case ViewKindArchitecture:
		for id := range below {
			if w.designContainer(id) || w.external(id) {
				add(id)
			}
		}
		for _, id := range keys(c) {
			for _, o := range w.adjacent(id, graph.EdgeKindUses, graph.EdgeKindDepends) {
				if w.designContainer(o) || w.external(o) {
					add(o)
				}
			}
		}
	case ViewKindC4:
		for id := range w.boxes(a) {
			add(id)
		}
		for id := range below {
			if w.nodes[id].Kind == graph.NodeKindActor {
				add(id)
			}
		}
		for _, id := range append(keys(below), input) {
			for _, o := range w.adjacent(id, graph.EdgeKindUses, graph.EdgeKindDepends) {
				if w.designContainer(id) && w.external(o) {
					add(o)
				}
			}
		}
		for _, e := range w.edges {
			if p := w.nodes[e.To]; e.Kind == graph.EdgeKindFrom && p.Kind == graph.NodeKindActor && p.Form == "person" {
				for _, op := range w.to(e.From, graph.EdgeKindCalls) {
					if below[op] {
						add(e.To)
					}
				}
			}
		}
	case ViewKindClass:
		designType := func(id string) bool {
			return w.nodes[id].Kind == graph.NodeKindType && w.nodes[id].Layer == graph.LayerDesign
		}
		if w.nodes[input].Kind == graph.NodeKindType {
			for _, o := range w.adjacent(input, graph.EdgeKindRefers, graph.EdgeKindExtends, graph.EdgeKindImplements) {
				if designType(o) {
					add(o)
				}
			}
		} else {
			for id := range below {
				if k := w.nodes[id].Kind; k == graph.NodeKindContainer || k == graph.NodeKindType || w.external(id) {
					add(id)
				}
			}
			for id := range below {
				for _, k := range []graph.EdgeKind{graph.EdgeKindRefers, graph.EdgeKindExtends, graph.EdgeKindImplements} {
					for _, o := range w.to(id, k) {
						if designType(id) && designType(o) {
							add(o)
						}
					}
				}
			}
		}
		for _, id := range keys(c) {
			for _, o := range w.to(id, graph.EdgeKindContains) {
				if w.nodes[id].Kind == graph.NodeKindType && w.nodes[o].Kind == graph.NodeKindOp {
					add(o)
				}
			}
		}
		if a.layer == ClassLayerCode {
			design := keys(c)
			clear(c)
			for _, id := range design {
				n := w.nodes[id]
				if n.Layer != graph.LayerDesign || n.Kind != graph.NodeKindType && n.Kind != graph.NodeKindOp {
					add(id)
					continue
				}
				for _, r := range w.from(id, graph.EdgeKindRealizes) {
					if m := w.nodes[r]; m.Layer == graph.LayerCode && m.Kind == n.Kind {
						add(r)
					}
				}
			}
		}
	case ViewKindSequence, ViewKindActivity:
		add(w.participants(input)...)
		for _, s := range w.stepsOf(input) {
			add(s)
			add(w.to(s, graph.EdgeKindCalls)...)
			for _, op := range w.to(s, graph.EdgeKindCalls) {
				if w.nodes[s].Form == "async" {
					add(w.to(op, graph.EdgeKindSubscribes)...)
				}
			}
			add(w.to(s, graph.EdgeKindHandles)...)
			add(w.to(s, graph.EdgeKindTraces)...)
			add(w.from(s, graph.EdgeKindCovers)...)
		}
	case ViewKindEvents:
		delete(c, input)
		for id := range below {
			if n := w.nodes[id]; n.Kind == graph.NodeKindType && n.Form == "event" {
				add(id)
				add(w.from(id, graph.EdgeKindEmits)...)
				add(w.from(id, graph.EdgeKindSubscribes)...)
			}
			if n := w.nodes[id]; n.Kind == graph.NodeKindType && n.Form == "error" {
				add(id)
				add(w.from(id, graph.EdgeKindHandles)...)
				for _, op := range w.nodes {
					sig := op.Attrs["signature"]
					if op.Kind != graph.NodeKindOp || len(sig) == 0 || !strings.Contains(sig[0], " raises ") {
						continue
					}
					_, raised, _ := strings.Cut(sig[0], " raises ")
					if slices.Contains(strings.Split(raised, ", "), id) {
						for _, s := range w.from(op.Id, graph.EdgeKindCalls) {
							if w.nodes[s].Kind == graph.NodeKindStep && w.nodes[s].Form == "call" {
								add(s)
							}
						}
					}
				}
			}
		}
	case ViewKindCalls:
		for _, f := range w.from(input, graph.EdgeKindRealizes) {
			if w.nodes[f].Kind == graph.NodeKindOp {
				add(f)
			}
		}
		for grown := true; grown; {
			grown = false
			for _, e := range w.edges {
				if n := w.nodes[e.To]; c[e.From] && !c[e.To] && (e.Kind == graph.EdgeKindCalls || e.Kind == graph.EdgeKindReaches) && n.Kind == graph.NodeKindOp && n.Salient {
					add(e.To)
					grown = true
				}
			}
		}
	case ViewKindProperty:
		if w.nodes[input].Kind != graph.NodeKindProperty {
			clear(c)
			for id := range below {
				if w.nodes[id].Kind == graph.NodeKindProperty {
					add(id)
				}
			}
		}
		for _, p := range keys(c) {
			add(w.to(p, graph.EdgeKindAbout)...)
			add(w.from(p, graph.EdgeKindChecks)...)
			for _, k := range w.to(p, graph.EdgeKindContains) {
				if w.nodes[k].Kind == graph.NodeKindCase {
					add(k)
				}
			}
		}
	case ViewKindCode:
		for id := range below {
			add(id)
		}
		for _, id := range keys(c) {
			add(w.adjacent(id, graph.EdgeKindCalls, graph.EdgeKindReaches)...)
			add(w.from(id, graph.EdgeKindCovers)...)
		}
	case ViewKindDdd:
		clear(c)
		below[input] = true
		for b := range below {
			if !w.designContainer(b) || !slices.Contains(w.nodes[b].Attrs["labels"], "bounded-context") {
				continue
			}
			add(b)
			for id := range w.below(b) {
				if n := w.nodes[id]; n.Kind == graph.NodeKindType && (n.Form == "event" || hasAny(n.Attrs["labels"], "aggregate", "entity", "value", "domain-service", "repository", "factory")) {
					add(id)
				}
			}
		}
	}
	return c
}

func hasAny(values []string, wanted ...string) bool {
	for _, v := range values {
		if slices.Contains(wanted, v) {
			return true
		}
	}
	return false
}

func keys(s map[string]bool) []string {
	return slices.Sorted(maps.Keys(s))
}

// foldingKinds are the views that fold by focus (§7.2); the views of a flow
// and of events draw every participant.
var foldingKinds = []ViewKind{ViewKindArchitecture, ViewKindClass, ViewKindCalls, ViewKindCode, ViewKindDdd, ViewKindC4}

// exempt returns the containers that do not fold in a view of kind on input:
// the input, what encloses it, and the expanded identities; every container
// when the view does not fold by focus.
func (w world) exempt(kind ViewKind, input string, expand []string) map[string]bool {
	e := map[string]bool{input: true}
	if !slices.Contains(foldingKinds, kind) {
		for id := range w.nodes {
			e[id] = true
		}
	}
	for _, id := range append(w.up(input), expand...) {
		e[id] = true
	}
	return e
}

// folder returns the container that decides how id is drawn, with its
// focus: of the containers that enclose or are id, not exempt, with focus
// support or utility, the one no other of them encloses.
func (w world) folder(id string, exempt map[string]bool, home string) (string, string) {
	var folding []string
	for _, c := range append(w.up(id), id) {
		if f := w.foldFocus(c, home); !exempt[c] && (f == "support" || f == "utility") {
			folding = append(folding, c)
		}
	}
	for _, c := range folding {
		enclosed := false
		for _, other := range folding {
			enclosed = enclosed || other != c && slices.Contains(w.up(c), other)
		}
		if !enclosed {
			return c, w.foldFocus(c, home)
		}
	}
	return "", ""
}

// home is the container outside which the class view folds every design
// container (§7.1): its input container, or the container of its input
// type; "" for the other views.
func (w world) home(a asked) string {
	if a.kind != ViewKindClass {
		return ""
	}
	if w.nodes[a.input].Kind == graph.NodeKindType {
		if parents := w.from(a.input, graph.EdgeKindContains); len(parents) > 0 {
			return parents[0]
		}
		return ""
	}
	return a.input
}

// foldFocus is the focus by which the container c folds in a view with the
// home: its focus, except support for a design container outside the home
// that is not a utility.
func (w world) foldFocus(c, home string) string {
	f := w.focus(c)
	if home != "" && f != "utility" && w.designContainer(c) && c != home && !slices.Contains(w.up(c), home) {
		return "support"
	}
	return f
}

// expectation is what a view must draw: its nodes, the folded containers,
// the utility containers left out, and its edges, each with the edges of
// the graph it stands for when an end is folded, or none when it is one.
type expectation struct {
	nodes, folded, hiders map[string]bool
	edges                 map[edgeKey][]graph.Edge
	// home maps a node folded into a container, or left out with one, to that container.
	home map[string]string
	// box maps, in a C4 view, a node to the node that stands for it.
	box func(string) string
}

// placeInto draws id, or the support container it folds into, or records the
// utility container that keeps it out.
func (w world) placeInto(x *expectation, id string, exempt map[string]bool, home string) {
	switch c, f := w.folder(id, exempt, home); f {
	case "support":
		x.nodes[c], x.folded[c] = true, true
		x.home[id] = c
	case "utility":
		x.hiders[c] = true
		x.home[id] = c
	default:
		x.nodes[id] = true
	}
}

// standIn is the drawn node that stands for id: id, or the folded container
// it is in; "" when neither is drawn.
func (x expectation) standIn(id string) string {
	c, placed := x.home[id]
	switch {
	case placed && x.folded[c]:
		return c
	case x.nodes[id]:
		return id
	case !placed && x.box != nil && x.box(id) != "" && x.box(id) != id:
		return x.standIn(x.box(id))
	}
	return ""
}

// boxes are the nodes of a C4 view that stand for what they contain: the
// input at the context level, the containers in it labelled container at
// the container level (§7.1).
func (w world) boxes(a asked) map[string]bool {
	boxes := map[string]bool{}
	if a.level == C4LevelContext {
		boxes[a.input] = true
		return boxes
	}
	for id := range w.below(a.input) {
		if w.designContainer(id) && slices.Contains(w.nodes[id].Attrs["labels"], "container") {
			boxes[id] = true
		}
	}
	return boxes
}

// box maps, in a C4 view, a step to its person, and any other node to the
// innermost actor or box among it and the nodes that contain it.
func (w world) box(a asked) func(string) string {
	if a.kind != ViewKindC4 {
		return nil
	}
	boxes := w.boxes(a)
	return func(id string) string {
		if w.nodes[id].Kind == graph.NodeKindStep {
			for _, p := range w.to(id, graph.EdgeKindFrom) {
				if w.nodes[p].Kind == graph.NodeKindActor && w.nodes[p].Form == "person" {
					return p
				}
			}
			return ""
		}
		for _, c := range append([]string{id}, w.containers(id)...) {
			if boxes[c] || w.nodes[c].Kind == graph.NodeKindActor {
				return c
			}
		}
		return ""
	}
}

// containers returns the nodes that contain id along contains, innermost first.
func (w world) containers(id string) []string {
	var chain []string
	for at := id; ; {
		parents := w.from(at, graph.EdgeKindContains)
		if len(parents) == 0 || parents[0] == id || slices.Contains(chain, parents[0]) {
			return chain
		}
		chain = append(chain, parents[0])
		at = parents[0]
	}
}

func (w world) detailFunction(id string) bool {
	n, ok := w.nodes[id]
	return ok && n.Kind == graph.NodeKindOp && !n.Salient
}

// expected computes the whole view: the candidates' salient nodes placed,
// then what each expansion reveals placed, then the edges between drawn nodes.
func (w world) expected(a asked, expand []string) expectation {
	kind, input := a.kind, a.input
	x := expectation{nodes: map[string]bool{}, folded: map[string]bool{}, hiders: map[string]bool{}, edges: map[edgeKey][]graph.Edge{},
		home: map[string]string{}, box: w.box(a)}
	exempt, home := w.exempt(kind, input, expand), w.home(a)
	for _, id := range keys(w.candidates(a)) {
		if w.nodes[id].Salient {
			w.placeInto(&x, id, exempt, home)
		}
	}
	firstPass := maps.Clone(x.nodes)
	revealed := map[edgeKey]bool{}
	for _, e := range expand {
		if words := strings.Split(e, " "); len(words) == 3 && words[0] == "reaches" {
			start, end := words[1], words[2]
			if firstPass[start] && firstPass[end] && slices.Contains(drawnKinds[kind], graph.EdgeKindReaches) && slices.Contains(w.to(start, graph.EdgeKindReaches), end) {
				for _, k := range w.detailPath(start, end) {
					revealed[k] = true
				}
			}
			continue
		}
		if n := w.nodes[e]; firstPass[e] && n.Kind == graph.NodeKindOp && n.Salient {
			for _, c := range w.to(e, graph.EdgeKindCalls) {
				if w.detailFunction(c) {
					revealed[edgeKey{graph.EdgeKindCalls, e, c}] = true
				}
			}
		}
	}
	for k := range revealed {
		for _, id := range []string{k.from, k.to} {
			if w.detailFunction(id) {
				w.placeInto(&x, id, exempt, home)
			}
		}
	}
	// The code view draws a call only into or out of its input.
	inside := w.below(input)
	inside[input] = true
	outside := func(e graph.Edge) bool {
		return kind == ViewKindCode && (e.Kind == graph.EdgeKindCalls || e.Kind == graph.EdgeKindReaches) && !inside[e.From] && !inside[e.To]
	}
	plain := map[edgeKey]bool{}
	for _, e := range w.edges {
		k := edgeKey{e.Kind, e.From, e.To}
		from, to := x.standIn(e.From), x.standIn(e.To)
		if from == "" || to == "" || (!slices.Contains(drawnKinds[kind], e.Kind) || outside(e)) && !revealed[k] {
			continue
		}
		if from == e.From && to == e.To {
			plain[k] = true
		} else if from != to {
			at := edgeKey{e.Kind, from, to}
			x.edges[at] = append(x.edges[at], e)
		}
	}
	for k := range plain {
		if merged, ok := x.edges[k]; ok {
			x.edges[k] = append(merged, w.edge(k))
		} else {
			x.edges[k] = nil
		}
	}
	return x
}

// edge returns the edge of the graph with the key k.
func (w world) edge(k edgeKey) graph.Edge {
	for _, e := range w.edges {
		if e.Kind == k.kind && e.From == k.from && e.To == k.to {
			return e
		}
	}
	return graph.Edge{}
}

// detailPath returns the calls edges of the paths from start to end whose
// inner functions, at least one, are all details.
func (w world) detailPath(start, end string) []edgeKey {
	reach := func(origin string, forward bool) map[string]bool {
		found := map[string]bool{}
		for grown := true; grown; {
			grown = false
			for _, e := range w.edges {
				a, b := e.From, e.To
				if !forward {
					a, b = b, a
				}
				if e.Kind == graph.EdgeKindCalls && (a == origin || found[a]) && w.detailFunction(b) && !found[b] {
					found[b], grown = true, true
				}
			}
		}
		return found
	}
	after, before := reach(start, true), reach(end, false)
	inner := func(id string) bool { return after[id] && before[id] }
	var path []edgeKey
	for _, e := range w.edges {
		if e.Kind == graph.EdgeKindCalls && (inner(e.From) && (inner(e.To) || e.To == end) || e.From == start && inner(e.To)) {
			path = append(path, edgeKey{e.Kind, e.From, e.To})
		}
	}
	return path
}
