package view

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/graph"
)

// Request is a graph and one view request on it: the view kind, the input
// node, the identities to expand, the class view's layer and the C4 view's level.
//
//uc:realizes view.Request
type Request struct {
	Fragment graph.Fragment
	Kind     ViewKind
	Id       string
	Expand   []string
	Layer    ClassLayer
	Level    C4Level
}

// genLayers draws the layer and the level of a request.
func genLayers(t *rapid.T) (ClassLayer, C4Level) {
	return rapid.SampledFrom([]ClassLayer{ClassLayerDesign, ClassLayerCode}).Draw(t, "layer"),
		rapid.SampledFrom([]C4Level{C4LevelContext, C4LevelContainer}).Draw(t, "level")
}

var viewKinds = []ViewKind{ViewKindArchitecture, ViewKindClass, ViewKindSequence, ViewKindActivity, ViewKindEvents,
	ViewKindCalls, ViewKindProperty, ViewKindCode, ViewKindDdd, ViewKindC4}

// builder accumulates a fragment with distinct nodes and distinct edges.
type builder struct {
	t    *rapid.T
	frag graph.Fragment
	at   map[string]int
	keys map[edgeKey]bool
}

func (b *builder) node(id string, kind graph.NodeKind, form string, layer graph.Layer, salient bool, attrs map[string][]string) string {
	b.at[id] = len(b.frag.Nodes)
	b.frag.Nodes = append(b.frag.Nodes, graph.Node{Id: id, Kind: kind, Form: form, Layer: layer,
		Source: graph.SourceDeclared, Summary: "summary of " + id, Salient: salient, Attrs: attrs})
	return id
}

func (b *builder) edge(kind graph.EdgeKind, from, to string) {
	if from == "" || to == "" || b.keys[edgeKey{kind, from, to}] {
		return
	}
	b.keys[edgeKey{kind, from, to}] = true
	b.frag.Edges = append(b.frag.Edges, graph.Edge{Kind: kind, From: from, To: to, Source: graph.SourceDeclared})
}

// pick draws one of ids, or "" when there is none.
func (b *builder) pick(label string, ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	return rapid.SampledFrom(ids).Draw(b.t, label)
}

// subset draws some of the values, each at most once, in their order.
func (b *builder) subset(label string, values []string) []string {
	var chosen []string
	for _, v := range values {
		if rapid.IntRange(0, 2).Draw(b.t, label) == 0 {
			chosen = append(chosen, v)
		}
	}
	return chosen
}

// genGraph draws a graph shaped like a real one: a system with components
// of every focus, some inheriting it, labels of both notations, external
// systems and persons, types, operations, at least minFlows flows with
// nested steps, properties with cases; and a code layer of packages and
// files owned by components, with salient and detail functions, calls
// between them, the reaches edges those calls imply, and bindings.
func genGraph(t *rapid.T, minFlows int) graph.Fragment {
	b := &builder{t: t, at: map[string]int{}, keys: map[edgeKey]bool{}}
	var notations []string
	for _, n := range []string{"ddd", "c4"} {
		if rapid.IntRange(0, 3).Draw(t, "notation") > 0 {
			notations = append(notations, n)
		}
	}
	sys := b.node("sys", graph.NodeKindContainer, "system", graph.LayerDesign, true, map[string][]string{"notations": notations})
	focuses := []string{"core", "core", "support", "support", "utility"}
	var components []string
	for i := range rapid.IntRange(1, 6).Draw(t, "components") {
		parent := b.pick("componentParent", append([]string{sys}, components...))
		attrs := map[string][]string{}
		if parent == sys || rapid.Bool().Draw(t, "ownFocus") {
			attrs["focus"] = []string{rapid.SampledFrom(focuses).Draw(t, "focus")}
		}
		if labels := b.subset("containerLabel", []string{"bounded-context", "software-system", "container", "component"}); labels != nil {
			attrs["labels"] = labels
		}
		id := fmt.Sprintf("c%d", i)
		if parent != sys {
			id = parent + "." + id
		}
		components = append(components, b.node(id, graph.NodeKindContainer, "component", graph.LayerDesign, true, attrs))
		b.edge(graph.EdgeKindContains, parent, id)
	}
	var actors, externals []string
	for i := range rapid.IntRange(0, 3).Draw(t, "actors") {
		form := rapid.SampledFrom([]string{"external", "person"}).Draw(t, "actorForm")
		id := b.node(fmt.Sprintf("a%d", i), graph.NodeKindActor, form, graph.LayerDesign, true, nil)
		b.edge(graph.EdgeKindContains, sys, id)
		actors = append(actors, id)
		if form == "external" {
			externals = append(externals, id)
		}
	}
	var types, events, errors []string
	for i := range rapid.IntRange(1, 8).Draw(t, "types") {
		parent := b.pick("typeParent", append(append([]string{}, components...), externals...))
		form := rapid.SampledFrom([]string{"object", "record", "event", "error", "interface", "enum"}).Draw(t, "typeForm")
		id := fmt.Sprintf("%s.T%d", parent, i)
		attrs := map[string][]string{}
		if labels := b.subset("typeLabel", []string{"aggregate", "value", "repository"}); labels != nil {
			attrs["labels"] = labels
		}
		for j := range rapid.IntRange(0, 3).Draw(t, "members") {
			if form == "enum" {
				attrs["values"] = append(attrs["values"], fmt.Sprintf("v%d", j))
				continue
			}
			named := rapid.SampledFrom(append(slices.Clone(types), id)).Draw(t, "fieldType")
			typ := rapid.SampledFrom([]string{"int", "string", named, "list<" + named + ">", "optional<" + named + ">"}).Draw(t, "fieldTypeForm")
			attrs["fields"] = append(attrs["fields"], fmt.Sprintf("f%d: %s", j, typ))
		}
		b.node(id, graph.NodeKindType, form, graph.LayerDesign, true, attrs)
		b.edge(graph.EdgeKindContains, parent, id)
		types = append(types, id)
		switch form {
		case "event":
			events = append(events, id)
		case "error":
			errors = append(errors, id)
		}
	}
	var ops []string
	for i := range rapid.IntRange(0, 6).Draw(t, "ops") {
		parent := b.pick("opParent", append(append(append([]string{}, components...), externals...), types...))
		signature := "(n: int)"
		if raised := b.subset("raised", errors); raised != nil {
			signature += " raises " + strings.Join(raised, ", ")
		}
		id := b.node(fmt.Sprintf("%s.o%d", parent, i), graph.NodeKindOp, "op", graph.LayerDesign, true, map[string][]string{"signature": {signature}})
		b.edge(graph.EdgeKindContains, parent, id)
		ops = append(ops, id)
	}
	participants := append(append(append([]string{}, components...), actors...), types...)
	var flows, steps []string
	for i := range rapid.IntRange(minFlows, 2).Draw(t, "flows") {
		id := b.node(fmt.Sprintf("%s.f%d", b.pick("flowParent", components), i), graph.NodeKindFlow, "flow", graph.LayerDesign, true, nil)
		b.edge(graph.EdgeKindContains, parentOf(id), id)
		flows = append(flows, id)
		steps = append(steps, b.steps(id, participants, ops, events, errors)...)
	}
	var cases []string
	for i := range rapid.IntRange(0, 2).Draw(t, "properties") {
		id := b.node(fmt.Sprintf("%s.p%d", b.pick("propertyParent", components), i), graph.NodeKindProperty, "invariant", graph.LayerDesign, true, nil)
		b.edge(graph.EdgeKindContains, parentOf(id), id)
		for range rapid.IntRange(1, 2).Draw(t, "abouts") {
			b.edge(graph.EdgeKindAbout, id, b.pick("about", append(append(append(append([]string{}, components...), types...), ops...), flows...)))
		}
		for j := range rapid.IntRange(0, 2).Draw(t, "cases") {
			c := b.node(fmt.Sprintf("%s.k%d", id, j), graph.NodeKindCase, "declared", graph.LayerDesign, true, nil)
			b.edge(graph.EdgeKindContains, id, c)
			cases = append(cases, c)
		}
	}
	for range rapid.IntRange(0, 4).Draw(t, "uses") {
		b.edge(graph.EdgeKindUses, b.pick("user", components), b.pick("used", append(append([]string{}, components...), actors...)))
	}
	for range rapid.IntRange(0, 6).Draw(t, "depends") {
		b.edge(graph.EdgeKindDepends, b.pick("dependent", components), b.pick("dependency", append(append([]string{}, components...), externals...)))
	}
	for range rapid.IntRange(0, 5).Draw(t, "typeEdges") {
		kind := rapid.SampledFrom([]graph.EdgeKind{graph.EdgeKindRefers, graph.EdgeKindExtends, graph.EdgeKindImplements}).Draw(t, "typeEdge")
		b.edge(kind, b.pick("typeFrom", types), b.pick("typeTo", types))
	}
	for range rapid.IntRange(0, 3).Draw(t, "subscribes") {
		b.edge(graph.EdgeKindSubscribes, b.pick("subscriber", ops), b.pick("subscribed", events))
	}
	b.code(components, types, ops, steps, cases)
	return b.frag
}

// parentOf returns the identity of the node that a generated identity names as its parent.
func parentOf(id string) string {
	for i := len(id) - 1; i >= 0; i-- {
		if id[i] == '.' {
			return id[:i]
		}
	}
	return ""
}

// steps draws a step tree under the flow, the second step always nested in
// the first, and numbers the steps in text order from 1, as their index.
func (b *builder) steps(flow string, participants, ops, events, errors []string) []string {
	n := rapid.IntRange(1, 12).Draw(b.t, "steps")
	children := make([][]int, n+1) // children[0] are the flow's; children[i+1] are step i's
	for i := range n {
		parent := 0
		if i == 1 {
			parent = 1
		} else if i > 1 {
			parent = rapid.IntRange(0, i).Draw(b.t, "stepParent")
		}
		children[parent] = append(children[parent], i+1)
	}
	var ids []string
	var walk func(parent string, at int)
	walk = func(parent string, at int) {
		for _, c := range children[at] {
			index := strconv.Itoa(len(ids) + 1)
			form := rapid.SampledFrom([]string{"call", "send", "emit", "async", "alt", "opt", "loop", "on"}).Draw(b.t, "stepForm")
			id := b.node(flow+"#"+index, graph.NodeKindStep, form, graph.LayerDesign, true, map[string][]string{"index": {index}})
			ids = append(ids, id)
			b.edge(graph.EdgeKindContains, parent, id)
			switch form {
			case "call", "send":
				b.edge(graph.EdgeKindFrom, id, b.pick("sender", participants))
				b.edge(graph.EdgeKindCalls, id, b.pick("called", ops))
			case "emit":
				b.edge(graph.EdgeKindFrom, id, b.pick("emitter", participants))
				b.edge(graph.EdgeKindEmits, id, b.pick("emitted", events))
			case "async":
				b.edge(graph.EdgeKindCalls, id, b.pick("subscriber", ops))
			case "on":
				b.edge(graph.EdgeKindHandles, id, b.pick("handled", errors))
			}
			walk(id, c)
		}
	}
	walk(flow, 0)
	return ids
}

// code draws the code layer: packages, files with an owner now and then,
// code types and functions, salient or not, with their bindings, calls,
// the reaches edges those calls imply, traces and covers.
func (b *builder) code(components, types, ops, steps, cases []string) {
	t := b.t
	var packages, files, codeTypes, functions []string
	for i := range rapid.IntRange(1, 2).Draw(t, "packages") {
		packages = append(packages, b.node(fmt.Sprintf("go:p%d", i), graph.NodeKindContainer, "package", graph.LayerCode, false, nil))
	}
	for i := range rapid.IntRange(1, 4).Draw(t, "files") {
		parent := b.pick("filePackage", packages)
		id := b.node(fmt.Sprintf("file:%s/f%d.go", parent[3:], i), graph.NodeKindContainer, "file", graph.LayerCode, false, nil)
		b.edge(graph.EdgeKindContains, parent, id)
		if rapid.IntRange(0, 3).Draw(t, "owned") > 0 {
			b.edge(graph.EdgeKindOwns, b.pick("owner", components), id)
		}
		files = append(files, id)
	}
	for i := range rapid.IntRange(0, 4).Draw(t, "codeTypes") {
		parent := b.pick("codeTypeFile", files)
		salient := rapid.Bool().Draw(t, "codeTypeSalient")
		id := b.node(fmt.Sprintf("%s.CT%d", parent, i), graph.NodeKindType, "struct", graph.LayerCode, salient, nil)
		b.edge(graph.EdgeKindContains, parent, id)
		if realized := b.pick("realizedType", types); salient && realized != "" {
			b.edge(graph.EdgeKindRealizes, id, realized)
			b.frag.Nodes[b.at[id]].Attrs = b.codeMembers(b.frag.Nodes[b.at[realized]])
		}
		codeTypes = append(codeTypes, id)
	}
	for range rapid.IntRange(0, 3).Draw(t, "codeTypeEdges") {
		kind := rapid.SampledFrom([]graph.EdgeKind{graph.EdgeKindRefers, graph.EdgeKindExtends, graph.EdgeKindImplements}).Draw(t, "codeTypeEdge")
		b.edge(kind, b.pick("codeTypeFrom", codeTypes), b.pick("codeTypeTo", codeTypes))
	}
	properties := b.ofKind(graph.NodeKindProperty)
	for i := range rapid.IntRange(2, 10).Draw(t, "functions") {
		parent := b.pick("functionParent", append(append([]string{}, files...), codeTypes...))
		salient := rapid.Bool().Draw(t, "functionSalient")
		id := b.node(fmt.Sprintf("%s.fn%d", parent, i), graph.NodeKindOp, "func", graph.LayerCode, salient, nil)
		b.edge(graph.EdgeKindContains, parent, id)
		if salient {
			switch rapid.IntRange(0, 2).Draw(t, "binding") {
			case 0:
				b.edge(graph.EdgeKindRealizes, id, b.pick("realizedOp", ops))
			case 1:
				b.edge(graph.EdgeKindChecks, id, b.pick("checked", properties))
			}
		}
		functions = append(functions, id)
	}
	for range rapid.IntRange(2, 20).Draw(t, "calls") {
		b.edge(graph.EdgeKindCalls, b.pick("caller", functions), b.pick("callee", functions))
	}
	b.reaches(functions)
	for i, e := range b.frag.Edges {
		if e.Kind == graph.EdgeKindDepends {
			for range rapid.IntRange(0, 2).Draw(t, "dependsCalls") {
				b.frag.Edges[i].Attrs = map[string][]string{"calls": append(b.frag.Edges[i].Attrs["calls"],
					b.pick("dependsCaller", functions)+" "+b.pick("dependsCallee", functions))}
			}
		}
	}
	salientFunctions := b.salient(functions)
	for _, s := range steps {
		if f := b.frag.Nodes[b.at[s]].Form; (f == "call" || f == "send") && rapid.Bool().Draw(t, "traced") {
			b.edge(graph.EdgeKindTraces, s, b.pick("trace", salientFunctions))
		}
	}
	for range rapid.IntRange(0, 3).Draw(t, "covers") {
		b.edge(graph.EdgeKindCovers, b.pick("case", cases), b.pick("covered", append(append([]string{}, steps...), functions...)))
	}
	// A file or package is salient when it holds a salient element.
	for i := len(b.frag.Nodes) - 1; i >= 0; i-- {
		if n := b.frag.Nodes[i]; n.Layer == graph.LayerCode && n.Salient {
			for p := parentOfCode(b, n.Id); p != ""; p = parentOfCode(b, p) {
				if b.frag.Nodes[b.at[p]].Kind == graph.NodeKindContainer {
					b.frag.Nodes[b.at[p]].Salient = true
				}
			}
		}
	}
}

// codeMembers draws the fields or enum values of a code type that realizes
// the design type t: each of t's kept alike, left out, given another type
// or, when its type is a name, made optional as Go writes a pointer; now
// and then one only the code has.
func (b *builder) codeMembers(t graph.Node) map[string][]string {
	attrs := map[string][]string{}
	if t.Form == "enum" {
		attrs["values"] = b.subset("codeValue", t.Attrs["values"])
		if rapid.IntRange(0, 3).Draw(b.t, "extraValue") == 0 {
			attrs["values"] = append(attrs["values"], "extra")
		}
		return attrs
	}
	for _, f := range t.Attrs["fields"] {
		name, typ, _ := strings.Cut(f, ": ")
		switch rapid.IntRange(0, 4).Draw(b.t, "codeField") {
		case 0:
		case 1:
			attrs["fields"] = append(attrs["fields"], name+": bool")
		case 2:
			if !strings.Contains(typ, "<") {
				typ = "optional<" + typ + ">"
			}
			attrs["fields"] = append(attrs["fields"], name+": "+typ)
		default:
			attrs["fields"] = append(attrs["fields"], f)
		}
	}
	if rapid.IntRange(0, 3).Draw(b.t, "extraField") == 0 {
		attrs["fields"] = append(attrs["fields"], "extra: int")
	}
	return attrs
}

// parentOfCode returns the container of a code node along the generated contains edges.
func parentOfCode(b *builder, id string) string {
	for _, e := range b.frag.Edges {
		if e.Kind == graph.EdgeKindContains && e.To == id {
			return e.From
		}
	}
	return ""
}

func (b *builder) ofKind(kind graph.NodeKind) []string {
	var ids []string
	for _, n := range b.frag.Nodes {
		if n.Kind == kind {
			ids = append(ids, n.Id)
		}
	}
	return ids
}

func (b *builder) salient(ids []string) []string {
	var found []string
	for _, id := range ids {
		if b.frag.Nodes[b.at[id]].Salient {
			found = append(found, id)
		}
	}
	return found
}

// reaches adds a reaches edge from each salient function to each salient
// function it calls through one or more detail functions (docs/model.md §9).
func (b *builder) reaches(functions []string) {
	callees := map[string][]string{}
	for _, e := range b.frag.Edges {
		if e.Kind == graph.EdgeKindCalls {
			callees[e.From] = append(callees[e.From], e.To)
		}
	}
	isSalient := func(id string) bool { return b.frag.Nodes[b.at[id]].Salient }
	for _, f := range b.salient(functions) {
		seen := map[string]bool{}
		var queue []string
		for _, c := range callees[f] {
			if !isSalient(c) && !seen[c] {
				seen[c] = true
				queue = append(queue, c)
			}
		}
		for ; len(queue) > 0; queue = queue[1:] {
			for _, c := range callees[queue[0]] {
				switch {
				case isSalient(c):
					b.edge(graph.EdgeKindReaches, f, c)
				case !seen[c]:
					seen[c] = true
					queue = append(queue, c)
				}
			}
		}
	}
}

// genExpand draws up to four identities to expand. Most are what a reader
// would click in the unexpanded view of kind on input: a drawn function, a
// drawn reaches edge written "reaches <from> <to>", a folded container or one
// left out. The others are any container or function of the graph, or
// names of nothing.
func genExpand(t *rapid.T, f graph.Fragment, kind ViewKind, input string, layer ClassLayer, level C4Level) []string {
	w := newWorld(f)
	var seen []string
	if input, ok := w.applicable(kind, input); ok {
		x := w.expected(asked{kind, input, layer, level}, nil)
		for _, id := range keys(x.nodes) {
			if w.nodes[id].Kind == graph.NodeKindOp || x.folded[id] {
				seen = append(seen, id)
			}
		}
		seen = append(seen, keys(x.hiders)...)
		for k := range x.edges {
			if k.kind == graph.EdgeKindReaches {
				seen = append(seen, "reaches "+k.from+" "+k.to)
			}
		}
		slices.Sort(seen)
	}
	others := []string{"nothing", "reaches nothing"}
	for _, n := range f.Nodes {
		if n.Kind == graph.NodeKindContainer || n.Kind == graph.NodeKindOp {
			others = append(others, n.Id)
		}
	}
	var expand []string
	for range rapid.IntRange(0, 4).Draw(t, "expansions") {
		if len(seen) > 0 && rapid.IntRange(0, 3).Draw(t, "clicked") > 0 {
			expand = append(expand, rapid.SampledFrom(seen).Draw(t, "seen"))
		} else {
			expand = append(expand, rapid.SampledFrom(others).Draw(t, "other"))
		}
	}
	return expand
}

// genRequest draws a graph and a request of any view kind, whose input is
// mostly a node the view applies to, and otherwise any node or "".
//
//uc:generates view.view-is-folded-subgraph.request
//uc:generates view.view-names-input-ancestors.request
//uc:generates view.view-cards-show-contents.request
func genRequest() *rapid.Generator[Request] {
	return rapid.Custom(func(t *rapid.T) Request {
		f := genGraph(t, 0)
		kind := rapid.SampledFrom(viewKinds).Draw(t, "kind")
		w := newWorld(f)
		var fitting, all []string
		for _, n := range f.Nodes {
			all = append(all, n.Id)
			if _, ok := w.applicable(kind, n.Id); ok {
				fitting = append(fitting, n.Id)
			}
		}
		id := rapid.SampledFrom(append(all, "")).Draw(t, "anyInput")
		if len(fitting) > 0 && rapid.IntRange(0, 5).Draw(t, "fits") > 0 {
			id = rapid.SampledFrom(fitting).Draw(t, "input")
		}
		layer, level := genLayers(t)
		return Request{Fragment: f, Kind: kind, Id: id, Expand: genExpand(t, f, kind, id, layer, level), Layer: layer, Level: level}
	})
}

// genFlowRequest draws a graph with at least one flow of nested steps and a
// sequence or activity view request for one of its flows.
//
//uc:generates view.sequence-follows-steps.request
func genFlowRequest() *rapid.Generator[Request] {
	return rapid.Custom(func(t *rapid.T) Request {
		f := genGraph(t, 1)
		var flows []string
		for _, n := range f.Nodes {
			if n.Kind == graph.NodeKindFlow {
				flows = append(flows, n.Id)
			}
		}
		kind := rapid.SampledFrom([]ViewKind{ViewKindSequence, ViewKindActivity}).Draw(t, "kind")
		flow := rapid.SampledFrom(flows).Draw(t, "flow")
		layer, level := genLayers(t)
		return Request{Fragment: f, Kind: kind, Id: flow, Expand: genExpand(t, f, kind, flow, layer, level), Layer: layer, Level: level}
	})
}

// genSelectGraph draws a graph shaped like a real one, as genGraph does, for
// a contract that calls Select on it.
//
//uc:generates view.view-echoes-its-request.g
func genSelectGraph() *rapid.Generator[*graph.Graph] {
	return rapid.Custom(func(t *rapid.T) *graph.Graph {
		g := graphOf(t, genGraph(t, 0))
		return &g
	})
}

// genSelectInput draws the input of a contract's view request: "", an
// identity that genGraph always or often draws, or one it never draws.
//
//uc:generates view.view-echoes-its-request.id
func genSelectInput() *rapid.Generator[string] {
	return rapid.SampledFrom([]string{"", "sys", "c0", "c1", "c0.c1", "c0.T0", "c1.T1", "c0.o0", "a0", "go:p0", "file:p0/f0.go", "nothing"})
}
