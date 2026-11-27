package graph

import (
	"cmp"
	"fmt"
	"maps"
	"slices"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/lincyaw/unicode/internal/diag"
)

// Derive computes what docs/model.md §9 derives from the other nodes and
// edges: owns, the realizes of methods, depends, traces, exercises, salience
// and reaches. It first drops every derived edge and salience flag, so that
// deriving again gives the graph that deriving once does. It returns a
// diagnostic for each binding whose target is not a node and for each path
// pattern that does not parse.
func (g *Graph) Derive() []diag.Diagnostic {
	for key, e := range g.edges {
		if e.Source == SourceDerived {
			delete(g.edges, key)
		}
	}
	for id, n := range g.nodes {
		n.Salient = false
		g.nodes[id] = n
	}
	diags := g.resolveBindings()
	d := newDerivation(g)
	diags = append(diags, d.owns()...)
	d.methodRealizes()
	d.salience()
	d.depends()
	d.traces()
	d.exercises()
	d.reaches()
	slices.SortFunc(diags, func(a, b diag.Diagnostic) int {
		return cmp.Or(cmp.Compare(a.Position.Path, b.Position.Path), cmp.Compare(a.Position.Line, b.Position.Line),
			cmp.Compare(a.Position.Column, b.Position.Column), cmp.Compare(a.Code, b.Code),
			cmp.Compare(a.Subject, b.Subject), cmp.Compare(a.Message, b.Message))
	})
	return diags
}

// resolveBindings adds every waiting binding whose target is now a node, and
// reports the others; they keep waiting, as do the candidates of methods.
func (g *Graph) resolveBindings() []diag.Diagnostic {
	var waiting []Edge
	var diags []diag.Diagnostic
	for _, e := range g.pending {
		_, resolved := g.nodes[e.To]
		key := edgeKey{e.Kind, e.From, e.To}
		switch {
		case isCandidate(e):
			waiting = append(waiting, e)
		case resolved:
			if _, ok := g.edges[key]; !ok {
				g.edges[key] = e
			}
		default:
			waiting = append(waiting, e)
			diags = append(diags, diag.Diagnostic{
				Code: "graph.unresolved-binding", Subject: edgeID(e), Position: e.Origin,
				Message: fmt.Sprintf("the %s binding of %s names %s, which is not a node of the design", e.Kind, e.From, e.To),
				Fix:     fmt.Sprintf("correct the identity in the binding, or declare %s in the design", e.To),
			})
		}
	}
	g.pending = waiting
	return diags
}

func edgeID(e Edge) string { return string(e.Kind) + " " + e.From + " " + e.To }

// derivation indexes the graph's edges by kind and end, and adds the derived
// edges to the graph and to the index as each rule computes them.
type derivation struct {
	g        *Graph
	parent   map[string]string
	children map[string][]string
	out      map[EdgeKind]map[string][]Edge
	in       map[EdgeKind]map[string][]Edge
}

func newDerivation(g *Graph) *derivation {
	d := &derivation{g: g, parent: map[string]string{}, children: map[string][]string{},
		out: map[EdgeKind]map[string][]Edge{}, in: map[EdgeKind]map[string][]Edge{}}
	for _, e := range g.Edges() {
		d.index(e)
	}
	return d
}

func (d *derivation) index(e Edge) {
	if e.Kind == EdgeKindContains {
		d.parent[e.To] = e.From
		d.children[e.From] = append(d.children[e.From], e.To)
	}
	if d.out[e.Kind] == nil {
		d.out[e.Kind], d.in[e.Kind] = map[string][]Edge{}, map[string][]Edge{}
	}
	d.out[e.Kind][e.From] = append(d.out[e.Kind][e.From], e)
	d.in[e.Kind][e.To] = append(d.in[e.Kind][e.To], e)
}

func (d *derivation) add(e Edge) {
	e.Source = SourceDerived
	d.g.edges[edgeKey{e.Kind, e.From, e.To}] = e
	d.index(e)
}

func (d *derivation) is(id string, layer Layer, kind NodeKind, forms ...string) bool {
	n, ok := d.g.nodes[id]
	return ok && n.Layer == layer && n.Kind == kind && (len(forms) == 0 || slices.Contains(forms, n.Form))
}

func (d *derivation) has(kind EdgeKind, from, to string) bool {
	_, ok := d.g.edges[edgeKey{kind, from, to}]
	return ok
}

// ancestors lists the containment ancestors of id, nearest first; a broken
// graph whose contains edges make a cycle stops at the first repeat.
func (d *derivation) ancestors(id string) []string {
	var out []string
	seen := map[string]bool{id: true}
	for p, ok := d.parent[id]; ok && !seen[p]; p, ok = d.parent[p] {
		seen[p] = true
		out = append(out, p)
	}
	return out
}

func (d *derivation) isFile(id string) bool { return d.is(id, LayerCode, NodeKindContainer, "file") }

func (d *derivation) isFunction(id string) bool { return d.is(id, LayerCode, NodeKindOp) }

// fileOf is the code file that holds id, or id itself when it is a file.
func (d *derivation) fileOf(id string) string {
	if d.isFile(id) {
		return id
	}
	for _, a := range d.ancestors(id) {
		if d.isFile(a) {
			return a
		}
	}
	return ""
}

// owners are the design containers that own the file of id.
func (d *derivation) owners(id string) []string {
	var out []string
	for _, e := range d.in[EdgeKindOwns][d.fileOf(id)] {
		out = append(out, e.From)
	}
	return out
}

// owns gives each code file to the deepest design containers whose paths
// patterns match its path; several tie only in a graph that breaks rule 12.
func (d *derivation) owns() []diag.Diagnostic {
	type owner struct {
		id       string
		depth    int
		patterns []string
	}
	var owners []owner
	var diags []diag.Diagnostic
	nodes := d.g.Nodes()
	for _, n := range nodes {
		if n.Layer != LayerDesign || n.Kind != NodeKindContainer {
			continue
		}
		o := owner{id: n.Id, depth: len(d.ancestors(n.Id))}
		for _, p := range n.Attrs["paths"] {
			if doublestar.ValidatePattern(p) {
				o.patterns = append(o.patterns, p)
				continue
			}
			diags = append(diags, diag.Diagnostic{
				Code: "graph.bad-path-pattern", Subject: n.Id, Position: n.Origin,
				Message: fmt.Sprintf("the paths pattern %q of %s is not a valid pattern, so it owns no file", p, n.Id),
				Fix:     "correct the pattern: / separates directories, * and ? match within one, ** matches any number of them, and every [ and { closes",
			})
		}
		owners = append(owners, o)
	}
	for _, f := range nodes {
		if !d.isFile(f.Id) || len(f.Attrs["path"]) == 0 {
			continue
		}
		path, best := f.Attrs["path"][0], -1
		var deepest []owner
		for _, o := range owners {
			matched := slices.ContainsFunc(o.patterns, func(p string) bool { return doublestar.MatchUnvalidated(p, path) })
			switch {
			case !matched || o.depth < best:
			case o.depth > best:
				best, deepest = o.depth, []owner{o}
			default:
				deepest = append(deepest, o)
			}
		}
		for _, o := range deepest {
			d.add(Edge{Kind: EdgeKindOwns, From: o.id, To: f.Id, Origin: d.g.nodes[o.id].Origin})
		}
	}
	return diags
}

// methodRealizes keeps the candidates by which a method m of a code type c,
// an interface's abstract method included, realizes an operation o of a
// design type t that c realizes.
func (d *derivation) methodRealizes() {
	candidates := slices.Clone(d.g.pending)
	slices.SortFunc(candidates, func(a, b Edge) int { return cmp.Or(cmp.Compare(a.From, b.From), cmp.Compare(a.To, b.To)) })
	for _, e := range candidates {
		t := d.parent[e.To]
		if isCandidate(e) && !d.has(EdgeKindRealizes, e.From, e.To) && d.isFunction(e.From) && d.is(e.To, LayerDesign, NodeKindOp) &&
			d.is(t, LayerDesign, NodeKindType) && d.has(EdgeKindRealizes, d.parent[e.From], t) {
			d.add(e)
		}
	}
}

// salience marks every design node, every code type and function that
// realizes a design node or carries a checks, generates or observes binding,
// and the files and packages that hold them. Unicode-generated declarations
// and files remain details.
func (d *derivation) salience() {
	salient := map[string]bool{}
	for id, n := range d.g.nodes {
		switch {
		case n.Layer == LayerDesign:
			salient[id] = true
		case n.Layer == LayerCode && !d.inGeneratedFile(id) && (n.Kind == NodeKindType || n.Kind == NodeKindOp) &&
			len(d.out[EdgeKindRealizes][id])+len(d.out[EdgeKindChecks][id])+len(d.out[EdgeKindGenerates][id])+len(d.out[EdgeKindObserves][id])+len(d.out[EdgeKindSubstitutes][id]) > 0:
			salient[id] = true
			for _, a := range append(d.ancestors(id), d.definingFile(id)) {
				if d.is(a, LayerCode, NodeKindContainer) && (!d.isFile(a) || !d.inGeneratedFile(a)) {
					salient[a] = true
				}
			}
		}
	}
	for id := range salient {
		n := d.g.nodes[id]
		n.Salient = true
		d.g.nodes[id] = n
	}
}

func (d *derivation) inGeneratedFile(id string) bool {
	return slices.Contains(d.g.nodes[d.definingFile(id)].Attrs["generated"], "true")
}

func (d *derivation) definingFile(id string) string {
	file := "file:" + d.g.nodes[id].Origin.Path
	if !d.isFile(file) {
		file = d.fileOf(id)
	}
	return file
}

// depends finds, for each pair of distinct design containers A and B, the
// imports and calls by which code that A owns uses code that B owns, and the
// calls by which it uses a client of an external system B.
func (d *derivation) depends() {
	type pair struct{ from, to string }
	basis := map[pair]map[EdgeKind][]Edge{}
	note := func(froms, tos []string, e Edge) {
		for _, a := range froms {
			for _, b := range tos {
				if a == b {
					continue
				}
				p := pair{a, b}
				if basis[p] == nil {
					basis[p] = map[EdgeKind][]Edge{}
				}
				basis[p][e.Kind] = append(basis[p][e.Kind], e)
			}
		}
	}
	for _, e := range d.g.Edges() {
		switch {
		case e.Kind == EdgeKindImports && d.is(e.From, LayerCode, NodeKindContainer):
			var tos []string
			for _, f := range d.filesOf(e.To) {
				tos = append(tos, d.owners(f)...)
			}
			note(d.owners(e.From), tos, e)
		case e.Kind == EdgeKindCalls && d.isFunction(e.From) && d.isFunction(e.To):
			note(d.owners(e.From), append(d.owners(e.To), d.externalsOf(e.To)...), e)
		}
	}
	for _, p := range slices.SortedFunc(maps.Keys(basis), func(a, b pair) int {
		return cmp.Or(cmp.Compare(a.from, b.from), cmp.Compare(a.to, b.to))
	}) {
		attrs := map[string][]string{}
		var first *Edge
		for _, kind := range []EdgeKind{EdgeKindImports, EdgeKindCalls} {
			for _, e := range basis[p][kind] {
				if first == nil {
					first = &e
				}
				if v := e.From + " " + e.To; !slices.Contains(attrs[string(kind)], v) {
					attrs[string(kind)] = append(attrs[string(kind)], v)
				}
			}
		}
		d.add(Edge{Kind: EdgeKindDepends, From: p.from, To: p.to, Origin: first.Origin, Attrs: attrs})
	}
}

// filesOf lists the code files of a package, or the file itself.
func (d *derivation) filesOf(id string) []string {
	if d.isFile(id) {
		return []string{id}
	}
	var out []string
	for _, c := range d.children[id] {
		if d.isFile(c) {
			out = append(out, c)
		}
	}
	return out
}

// externalsOf lists the external systems one of whose operations the code
// function id realizes: it is their client.
func (d *derivation) externalsOf(id string) []string {
	var out []string
	for _, e := range d.out[EdgeKindRealizes][id] {
		if !d.is(e.To, LayerDesign, NodeKindOp) {
			continue
		}
		for _, a := range d.ancestors(e.To) {
			if d.is(a, LayerDesign, NodeKindActor, "external") && !slices.Contains(out, a) {
				out = append(out, a)
			}
			if d.is(a, LayerDesign, NodeKindContainer) {
				break
			}
		}
	}
	return out
}

// traces joins each call or send step to the code functions that realize
// the operation it calls.
func (d *derivation) traces() {
	for _, s := range d.g.Nodes() {
		if !d.is(s.Id, LayerDesign, NodeKindStep, "call", "send") {
			continue
		}
		for _, c := range d.out[EdgeKindCalls][s.Id] {
			for _, r := range d.in[EdgeKindRealizes][c.To] {
				if d.isFunction(r.From) && !d.has(EdgeKindTraces, s.Id, r.From) {
					d.add(Edge{Kind: EdgeKindTraces, From: s.Id, To: r.From, Origin: r.Origin})
				}
			}
		}
	}
}

// callees lists the code functions that the code function id calls.
func (d *derivation) callees(id string) []string {
	var out []string
	for _, e := range d.out[EdgeKindCalls][id] {
		if d.isFunction(e.To) {
			out = append(out, e.To)
		}
	}
	return out
}

// reachable lists, sorted, the starts and the code functions reached from
// them along calls, moving on only from functions that pass through.
func (d *derivation) reachable(starts []string, through func(string) bool) []string {
	seen := map[string]bool{}
	queue := slices.Clone(starts)
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if seen[id] {
			continue
		}
		seen[id] = true
		if through(id) {
			queue = append(queue, d.callees(id)...)
		}
	}
	return slices.Sorted(maps.Keys(seen))
}

// exercises joins each predicate to every function it reaches along calls.
func (d *derivation) exercises() {
	for _, p := range d.g.Nodes() {
		checks := d.out[EdgeKindChecks][p.Id]
		if !d.isFunction(p.Id) || len(checks) == 0 {
			continue
		}
		for _, f := range d.reachable(d.callees(p.Id), func(string) bool { return true }) {
			d.add(Edge{Kind: EdgeKindExercises, From: p.Id, To: f, Origin: checks[0].Origin})
		}
	}
}

// reaches joins two distinct salient functions f and g when f calls g through
// one or more detail functions and not directly; attr via lists the detail
// functions on such call chains.
func (d *derivation) reaches() {
	salient := func(id string) bool { return d.g.nodes[id].Salient }
	detail := func(id string) bool { return !salient(id) }
	callers := map[string][]string{}
	for _, e := range d.g.Edges() {
		if e.Kind == EdgeKindCalls && d.isFunction(e.From) && d.isFunction(e.To) {
			callers[e.To] = append(callers[e.To], e.From)
		}
	}
	// reachingDetails lists the detail functions that call g through detail functions only.
	reachingDetails := func(g string) map[string]bool {
		seen := map[string]bool{}
		queue := slices.Clone(callers[g])
		for len(queue) > 0 {
			id := queue[0]
			queue = queue[1:]
			if seen[id] || !detail(id) {
				continue
			}
			seen[id] = true
			queue = append(queue, callers[id]...)
		}
		return seen
	}
	for _, f := range d.g.Nodes() {
		if !d.isFunction(f.Id) || !f.Salient {
			continue
		}
		direct := d.callees(f.Id)
		var details []string
		for _, id := range d.reachable(direct, detail) {
			if detail(id) {
				details = append(details, id)
			}
		}
		targets := map[string]bool{}
		for _, id := range details {
			for _, g := range d.callees(id) {
				if salient(g) && g != f.Id && !slices.Contains(direct, g) {
					targets[g] = true
				}
			}
		}
		for _, g := range slices.Sorted(maps.Keys(targets)) {
			toG := reachingDetails(g)
			var via []string
			for _, id := range details {
				if toG[id] {
					via = append(via, id)
				}
			}
			first := slices.IndexFunc(direct, func(id string) bool { return toG[id] })
			origin := d.g.edges[edgeKey{EdgeKindCalls, f.Id, direct[first]}].Origin
			d.add(Edge{Kind: EdgeKindReaches, From: f.Id, To: g, Origin: origin, Attrs: map[string][]string{"via": via}})
		}
	}
}
