package serve

import (
	"cmp"
	"maps"
	"net/http"
	"slices"
	"strings"

	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/review"
	"github.com/lincyaw/unicode/internal/view"
)

// page writes the nodes an entity page lists and remembers them, so that the
// page's diagnostics are those of what it shows.
type page struct {
	st     *state
	listed map[string]bool
}

func (p *page) node(id string) Node {
	p.listed[id] = true
	return p.st.node(id)
}

func (p *page) nodes(ids []string) []Node {
	out := make([]Node, 0, len(ids))
	for _, id := range ids {
		out = append(out, p.node(id))
	}
	return out
}

// flowSteps groups steps that are in flow order by their flow: each flow's
// node with its steps, as the entity page's 参与的流程 lists them.
func (p *page) flowSteps(steps []string) []FlowSteps {
	out := []FlowSteps{}
	for _, s := range steps {
		flow := p.st.stepInfo(s).Flow
		if len(out) == 0 || out[len(out)-1].Flow.Id != flow {
			out = append(out, FlowSteps{Flow: p.node(flow), Steps: []Node{}})
		}
		last := &out[len(out)-1]
		last.Steps = append(last.Steps, p.node(s))
	}
	return out
}

func (p *page) neighbors(focus string, edges []graph.Edge) []Neighbor {
	for _, e := range edges {
		p.listed[e.From], p.listed[e.To] = true, true
	}
	return p.st.neighbors(focus, edges)
}

// viewKinds are the views in the order of their tabs.
var viewKinds = []view.ViewKind{view.ViewKindArchitecture, view.ViewKindDdd, view.ViewKindClass, view.ViewKindSequence,
	view.ViewKindActivity, view.ViewKindEvents, view.ViewKindCalls, view.ViewKindProperty, view.ViewKindCode, view.ViewKindC4}

// entity is the page of the node id: the common sections, then the one of its kind.
func (st *state) entity(id string) (Entity, error) {
	n, ok := st.nodes[id]
	if !ok {
		return Entity{}, st.unknown(id)
	}
	p := &page{st: st, listed: map[string]bool{}}
	views := st.relatedViews(id)
	e := Entity{
		Node:       st.node(id),
		Ancestors:  st.nodeList(st.ancestors(id)),
		Views:      views,
		Tabs:       st.tabsWith(id, views),
		RealizedBy: p.neighborsOf(st.realizations(id)),
		Properties: p.nodes(st.relatedProperties(id)),
		Flows:      p.flowSteps(st.participations(id)),
		Sources:    st.sourcesOf(id),
		Evidence:   st.evidenceOf(id), Review: st.reviewOf(review.Target{Node: id}),
	}
	covered := st.sources(id, graph.EdgeKindCovers)
	if len(covered) > 0 {
		nodes := p.nodes(sorted(covered))
		e.CoveredBy = &nodes
	}
	switch {
	case n.Layer == graph.LayerCode:
		e.Code = p.codeDetail(id)
	case n.Kind == graph.NodeKindContainer || n.Kind == graph.NodeKindActor:
		e.Container = p.containerDetail(id)
	case n.Kind == graph.NodeKindType:
		e.Type = p.typeDetail(id)
	case n.Kind == graph.NodeKindOp:
		e.Op = p.opDetail(id)
	case n.Kind == graph.NodeKindFlow:
		e.Flow = &FlowDetail{Steps: p.nodes(st.flowSteps(id))}
		e.Flow.Predicate, e.Flow.Generators, e.Flow.Cases = p.checked(id)
	case n.Kind == graph.NodeKindProperty:
		e.Property = &PropertyDetail{About: p.nodes(st.about(id))}
		e.Property.Predicate, e.Property.Generators, e.Property.Cases = p.checked(id)
	case n.Kind == graph.NodeKindStep:
		e.Step = p.stepDetail(id)
	case n.Kind == graph.NodeKindCase:
		e.Case = &CaseDetail{Covers: p.neighbors(id, st.outgoing(id, graph.EdgeKindCovers))}
	}
	at := map[string]bool{}
	for _, edge := range append(slices.Clone(st.out[id]), st.in[id]...) {
		at[edgeID(edge.Kind, edge.From, edge.To)] = true
	}
	delete(p.listed, "")
	e.Diagnostics = st.diagnostics(func(subject string) bool { return subject == id || at[subject] || p.listed[subject] })
	return e, nil
}

func (st *state) unknown(id string) problem {
	return refuse(http.StatusNotFound, "serve.not-found", "give the identity of a node of the snapshot, as /api/search finds it",
		"the snapshot has no node %q", id)
}

// focusedEdge is an edge with the node of the page it belongs to.
type focusedEdge struct {
	focus string
	edge  graph.Edge
}

func (p *page) neighborsOf(edges []focusedEdge) []Neighbor {
	out := []Neighbor{}
	for _, f := range edges {
		out = append(out, p.neighbors(f.focus, []graph.Edge{f.edge})...)
	}
	return out
}

// realizations are the realizes edges into a design type or operation; for
// an external system, those into its types and operations.
func (st *state) realizations(id string) []focusedEdge {
	if st.nodes[id].Layer != graph.LayerDesign {
		return nil
	}
	var out []focusedEdge
	targets := []string{id}
	if n := st.nodes[id]; n.Kind == graph.NodeKindActor && n.Form == "external" {
		targets = st.children(id, graph.NodeKindType, graph.NodeKindOp)
	}
	for _, t := range targets {
		for _, e := range st.incoming(t, graph.EdgeKindRealizes) {
			out = append(out, focusedEdge{t, e})
		}
	}
	return out
}

// relatedProperties are the properties about the node or one of its
// operations; for a step, about its flow or what it calls.
func (st *state) relatedProperties(id string) []string {
	about := []string{id}
	if st.nodes[id].Kind == graph.NodeKindStep {
		about = append(st.targets(id, graph.EdgeKindCalls), st.stepInfo(id).Flow)
	} else {
		about = append(about, st.children(id, graph.NodeKindOp)...)
	}
	found := map[string]bool{}
	for _, a := range about {
		for _, p := range st.sources(a, graph.EdgeKindAbout) {
			if st.nodes[p].Kind == graph.NodeKindProperty {
				found[p] = true
			}
		}
	}
	return slices.Sorted(maps.Keys(found))
}

// participations are the steps a design node takes part in: those that call
// it or one of its operations, call an operation whose signature refers to
// it, emit it or handle it, or that it sends as a person; by flow and index.
func (st *state) participations(id string) []string {
	n := st.nodes[id]
	if n.Layer != graph.LayerDesign || n.Kind == graph.NodeKindStep {
		return []string{}
	}
	found := map[string]bool{}
	add := func(ids []string) {
		for _, s := range ids {
			if st.nodes[s].Kind == graph.NodeKindStep {
				found[s] = true
			}
		}
	}
	for _, op := range append([]string{id}, st.children(id, graph.NodeKindOp)...) {
		add(st.sources(op, graph.EdgeKindCalls))
	}
	for _, op := range st.sources(id, graph.EdgeKindRefers) {
		if st.is(op, graph.LayerDesign, graph.NodeKindOp) {
			add(st.sources(op, graph.EdgeKindCalls))
		}
	}
	add(st.sources(id, graph.EdgeKindEmits, graph.EdgeKindHandles))
	if n.Kind == graph.NodeKindActor && n.Form == "person" {
		add(st.sources(id, graph.EdgeKindFrom))
	}
	return st.inFlowOrder(slices.Collect(maps.Keys(found)))
}

// inFlowOrder sorts steps by their flow, then by index.
func (st *state) inFlowOrder(steps []string) []string {
	flows := map[string]string{}
	for _, s := range steps {
		flows[s] = st.stepInfo(s).Flow
	}
	slices.SortFunc(steps, func(a, b string) int {
		return cmp.Or(cmp.Compare(flows[a], flows[b]), cmp.Compare(st.index(a), st.index(b)), cmp.Compare(a, b))
	})
	return steps
}

// sourcesOf is the source of the code elements that realize the node, or a
// code type's or function's own.
func (st *state) sourcesOf(id string) []CodeExcerpt {
	var elements []string
	if st.nodes[id].Layer == graph.LayerCode {
		elements = []string{id}
	}
	for _, r := range st.realizations(id) {
		elements = append(elements, r.edge.From)
	}
	out := []CodeExcerpt{}
	for _, c := range elements {
		if excerpt, ok := st.declaration(c); ok {
			out = append(out, excerpt)
		}
	}
	return out
}

// relatedViews are the views to open from a node's page: those that take it
// as input; for a design node, the class, DDD and architecture views of its
// container; the sequence and activity views of the flows it takes part in;
// the calls views of its operations and of the functions that realize it;
// the property views of its properties. Each is listed once, when it applies.
func (st *state) relatedViews(id string) []ViewRequest {
	type candidate struct {
		kind       view.ViewKind
		input, sel string
	}
	var candidates []candidate
	add := func(kind view.ViewKind, input, sel string) {
		candidates = append(candidates, candidate{kind, input, sel})
	}
	for _, k := range viewKinds {
		add(k, id, "")
	}
	n := st.nodes[id]
	if c := st.container(id); c != "" && n.Layer == graph.LayerDesign {
		for _, k := range []view.ViewKind{view.ViewKindClass, view.ViewKindDdd, view.ViewKindArchitecture} {
			add(k, c, id)
		}
	}
	steps := st.participations(id)
	if n.Kind == graph.NodeKindStep {
		steps = []string{id}
	}
	for _, s := range steps {
		flow := st.stepInfo(s).Flow
		add(view.ViewKindSequence, flow, s)
		add(view.ViewKindActivity, flow, s)
	}
	for _, op := range st.children(id, graph.NodeKindOp) {
		add(view.ViewKindCalls, op, "")
	}
	for _, r := range st.realizations(id) {
		if st.nodes[r.edge.From].Kind == graph.NodeKindOp {
			add(view.ViewKindCalls, r.edge.From, "")
		}
	}
	for _, p := range st.relatedProperties(id) {
		add(view.ViewKindProperty, p, "")
	}
	out := []ViewRequest{}
	seen := map[candidate]bool{}
	for _, c := range candidates {
		if seen[c] || !st.viewApplies(c.kind, c.input) {
			continue
		}
		seen[c] = true
		r := ViewRequest{Kind: ViewKind(c.kind), Input: st.brief(c.input), Expand: []string{}}
		if c.sel != "" {
			r.Select = &c.sel
		}
		out = append(out, r)
	}
	return out
}

// container is the nearest design container that holds a design node, "" when none.
func (st *state) container(id string) string {
	for _, a := range slices.Backward(st.ancestors(id)) {
		if st.is(a, graph.LayerDesign, graph.NodeKindContainer) {
			return a
		}
	}
	return ""
}

func (p *page) containerDetail(id string) *ContainerDetail {
	st := p.st
	files := st.targets(id, graph.EdgeKindOwns)
	slices.Sort(files)
	return &ContainerDetail{
		Contents:     p.nodes(st.children(id)),
		DependsOn:    p.neighbors(id, st.outgoing(id, graph.EdgeKindUses, graph.EdgeKindDepends)),
		DependedOnBy: p.neighbors(id, st.incoming(id, graph.EdgeKindUses, graph.EdgeKindDepends)),
		Files:        p.nodes(files),
	}
}

func (p *page) typeDetail(id string) *TypeDetail {
	st := p.st
	ops := st.children(id, graph.NodeKindOp)
	rows := []OpRow{}
	refers := map[string]bool{}
	for _, t := range st.targets(id, graph.EdgeKindRefers) {
		refers[t] = true
	}
	for _, op := range ops {
		rows = append(rows, OpRow{Op: p.node(op), RealizedBy: p.neighbors(op, st.incoming(op, graph.EdgeKindRealizes)),
			Properties: p.nodes(st.relatedProperties(op))})
		for _, t := range st.targets(op, graph.EdgeKindRefers) {
			refers[t] = true
		}
	}
	delete(refers, id)
	var referredBy, raisedBy []string
	for _, r := range st.sources(id, graph.EdgeKindRefers) {
		if st.is(r, graph.LayerDesign, graph.NodeKindType, graph.NodeKindOp) {
			referredBy = append(referredBy, r)
		}
		if st.is(r, graph.LayerDesign, graph.NodeKindOp) && slices.Contains(raises(st.attr(r, "signature")), id) {
			raisedBy = append(raisedBy, r)
		}
	}
	return &TypeDetail{
		Members:     p.members(id),
		Ops:         rows,
		Refers:      p.nodes(slices.DeleteFunc(slices.Sorted(maps.Keys(refers)), func(t string) bool { return st.nodes[t].Kind != graph.NodeKindType })),
		ReferredBy:  p.nodes(sorted(referredBy)),
		Supertypes:  p.neighbors(id, st.outgoing(id, graph.EdgeKindExtends, graph.EdgeKindImplements)),
		Subtypes:    p.neighbors(id, st.incoming(id, graph.EdgeKindExtends, graph.EdgeKindImplements)),
		EmittedBy:   p.nodes(st.inFlowOrder(st.sources(id, graph.EdgeKindEmits))),
		Subscribers: p.nodes(sorted(st.sources(id, graph.EdgeKindSubscribes))),
		RaisedBy:    p.nodes(sorted(raisedBy)),
		HandledBy:   p.nodes(st.inFlowOrder(st.sources(id, graph.EdgeKindHandles))),
	}
}

// members are check's comparison of a design type's fields and enum values
// with the code type that realizes it, which a type's card shows too.
func (p *page) members(id string) []Member {
	rows := []Member{}
	for _, m := range p.st.members(id) {
		rows = append(rows, member(m))
		for _, r := range m.Refers {
			p.listed[r] = true
		}
	}
	return rows
}

// typeRefs are the identities of the declared types that a type written in
// the canonical format names, in the order it names them.
func (st *state) typeRefs(typ string) []string {
	out := []string{}
	for _, word := range strings.FieldsFunc(typ, func(r rune) bool { return strings.ContainsRune("<>, ()", r) }) {
		if st.is(word, graph.LayerDesign, graph.NodeKindType) && !slices.Contains(out, word) {
			out = append(out, word)
		}
	}
	return out
}

// raises are the errors a signature written `(…) -> T raises E1, E2` names.
func raises(signature string) []string {
	_, errs, ok := strings.Cut(signature, " raises ")
	if !ok {
		return nil
	}
	return strings.Split(errs, ", ")
}

func (p *page) opDetail(id string) *OpDetail {
	st := p.st
	detail := &OpDetail{SignatureTypes: p.nodes(st.typeRefs(st.attr(id, "signature"))), Callers: []Neighbor{}, Callees: []Neighbor{}}
	for _, f := range st.sources(id, graph.EdgeKindRealizes) {
		if !st.is(f, graph.LayerCode, graph.NodeKindOp) {
			continue
		}
		detail.Callers = append(detail.Callers, p.neighbors(f, st.incoming(f, graph.EdgeKindCalls, graph.EdgeKindReaches))...)
		detail.Callees = append(detail.Callees, p.neighbors(f, st.salientCallees(f))...)
	}
	if s := st.targets(id, graph.EdgeKindSubscribes); len(s) > 0 {
		event := p.node(s[0])
		detail.Subscribes = &event
	}
	return detail
}

// salientCallees are the calls and reaches edges from f to salient functions.
func (st *state) salientCallees(f string) []graph.Edge {
	return slices.DeleteFunc(st.outgoing(f, graph.EdgeKindCalls, graph.EdgeKindReaches), func(e graph.Edge) bool {
		return !st.nodes[e.To].Salient || st.nodes[e.To].Kind != graph.NodeKindOp
	})
}

// checked is what checks a property or a flow: its predicate, its
// generators, and its cases, the declared ones in the design's order, then
// the recorded ones in the corpus's.
func (p *page) checked(id string) (*Neighbor, []Neighbor, []CaseRow) {
	st := p.st
	var predicate *Neighbor
	if checks := slices.DeleteFunc(st.incoming(id, graph.EdgeKindChecks), func(e graph.Edge) bool {
		return !st.is(e.From, graph.LayerCode, graph.NodeKindOp)
	}); len(checks) > 0 {
		predicate = &p.neighbors(id, checks[:1])[0]
	}
	cases := st.children(id, graph.NodeKindCase)
	recorded := func(c string) int {
		if st.nodes[c].Form == "declared" {
			return 0
		}
		return 1
	}
	slices.SortStableFunc(cases, func(a, b string) int { return cmp.Compare(recorded(a), recorded(b)) })
	rows := []CaseRow{}
	for _, c := range cases {
		rows = append(rows, CaseRow{Node: p.node(c), Evidence: st.evidenceOf(c)})
	}
	return predicate, p.neighbors(id, st.incoming(id, graph.EdgeKindGenerates)), rows
}

// flowSteps are every step of a flow's step tree, by index.
func (st *state) flowSteps(flow string) []string {
	var steps []string
	queue := st.children(flow, graph.NodeKindStep)
	seen := map[string]bool{}
	for len(queue) > 0 {
		s := queue[0]
		queue = queue[1:]
		if seen[s] {
			continue
		}
		seen[s] = true
		steps = append(steps, s)
		queue = append(queue, st.subSteps(s)...)
	}
	st.byIndex(steps)
	return steps
}

// about is what a property is about, in the order the about clause writes it.
func (st *state) about(property string) []string {
	edges := st.outgoing(property, graph.EdgeKindAbout)
	slices.SortStableFunc(edges, func(a, b graph.Edge) int {
		return cmp.Or(cmp.Compare(a.Origin.Line, b.Origin.Line), cmp.Compare(a.Origin.Column, b.Origin.Column))
	})
	ids := []string{}
	for _, e := range edges {
		ids = append(ids, e.To)
	}
	return ids
}

func (p *page) stepDetail(id string) *StepDetail {
	st := p.st
	detail := &StepDetail{Children: p.nodes(st.subSteps(id)), Traces: p.neighbors(id, st.outgoing(id, graph.EdgeKindTraces)), CallSites: []Neighbor{}}
	parent := st.parent(id)
	if st.nodes[id].Form == "on" {
		siblings := st.children(parent, graph.NodeKindStep)
		st.byIndex(siblings)
		for _, s := range slices.Backward(siblings) {
			if st.index(s) < st.index(id) && st.nodes[s].Form != "on" {
				if st.nodes[s].Form == "call" {
					call := p.node(s)
					detail.Call = &call
				}
				break
			}
		}
	}
	callers := map[string]bool{}
	for _, sender := range st.targets(id, graph.EdgeKindFrom) {
		for _, f := range st.ownedCode(sender) {
			callers[f] = true
		}
	}
	for _, f := range st.targets(id, graph.EdgeKindTraces) {
		sites := slices.DeleteFunc(st.incoming(f, graph.EdgeKindCalls), func(e graph.Edge) bool { return !callers[e.From] })
		detail.CallSites = append(detail.CallSites, p.neighbors(f, sites)...)
	}
	return detail
}

// ownedCode are the functions and methods in the files that the container
// of a step's sender owns: the sender itself when it is a container, else
// the design container that declares it. A sub-step's sender is the
// receiver of its enclosing call (docs/model.md §10, rule 8), so this is the
// code of the enclosing call's operation as well.
func (st *state) ownedCode(sender string) []string {
	owner := sender
	if !st.is(owner, graph.LayerDesign, graph.NodeKindContainer) {
		owner = st.container(sender)
	}
	var found []string
	for _, file := range st.targets(owner, graph.EdgeKindOwns) {
		for _, c := range st.children(file) {
			if st.nodes[c].Kind == graph.NodeKindOp {
				found = append(found, c)
			}
			found = append(found, st.children(c, graph.NodeKindOp)...)
		}
	}
	return found
}

func (p *page) codeDetail(id string) *CodeDetail {
	st := p.st
	detail := &CodeDetail{Contents: p.nodes(st.children(id)), Callers: p.neighbors(id, st.incoming(id, graph.EdgeKindCalls, graph.EdgeKindReaches)),
		Callees: p.neighbors(id, st.salientCallees(id)), Details: []Node{}}
	if owners := st.sources(id, graph.EdgeKindOwns); len(owners) > 0 {
		owner := p.node(owners[0])
		detail.Owner = &owner
	}
	for _, c := range st.targets(id, graph.EdgeKindCalls) {
		if st.nodes[c].Kind == graph.NodeKindOp && !st.nodes[c].Salient {
			detail.Details = append(detail.Details, p.node(c))
		}
	}
	if n := st.nodes[id]; n.Kind == graph.NodeKindType || n.Kind == graph.NodeKindOp {
		if line, ok := st.line(n.Origin.Path, n.Origin.Line); ok {
			declaration := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line), "{"))
			detail.Declaration = &declaration
		}
	}
	return detail
}

// edgeEntity is the preview of the edge kind from to.
func (st *state) edgeEntity(kind graph.EdgeKind, from, to string) (EdgeEntity, error) {
	id := edgeID(kind, from, to)
	e, ok := st.edges[id]
	if !ok {
		return EdgeEntity{}, refuse(http.StatusNotFound, "serve.not-found", "give the kind, start and end of an edge of the snapshot, as /api/node lists them",
			"the snapshot has no edge %s", id)
	}
	var named []string
	for _, key := range []string{"via", "imports", "calls"} {
		for _, value := range e.Attrs[key] {
			for _, word := range strings.Fields(value) {
				if _, ok := st.nodes[word]; ok && !slices.Contains(named, word) {
					named = append(named, word)
				}
			}
		}
	}
	steps := st.inFlowOrder(slices.DeleteFunc(st.sources(to, graph.EdgeKindTraces), func(s string) bool {
		return st.nodes[s].Kind != graph.NodeKindStep
	}))
	views := []ViewRequest{}
	if kind == graph.EdgeKindReaches && st.viewApplies(view.ViewKindCalls, from) {
		views = append(views, ViewRequest{Kind: ViewKindCalls, Input: st.brief(from), Expand: []string{"reaches " + from + " " + to}})
	}
	for _, s := range steps {
		if flow := st.stepInfo(s).Flow; st.viewApplies(view.ViewKindSequence, flow) {
			sel := s
			views = append(views, ViewRequest{Kind: ViewKindSequence, Input: st.brief(flow), Expand: []string{}, Select: &sel})
		}
	}
	return EdgeEntity{Edge: st.edge(e), FromNode: st.node(from), ToNode: st.node(to), Nodes: st.nodeList(named), Steps: st.nodeList(steps),
		Diagnostics: st.diagnostics(func(subject string) bool { return subject == id }), Views: views, Review: st.reviewOf(review.Target{EdgeKind: string(kind), From: from, To: to})}, nil
}

func sorted(ids []string) []string {
	slices.Sort(ids)
	return slices.Compact(ids)
}
