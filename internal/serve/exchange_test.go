package serve

import (
	"fmt"
	"maps"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/review"
)

// Exchange is a repository and requests to its viewer: the graph of its
// snapshot, the files of its working tree, and the requests.
//
//uc:realizes serve.Exchange
type Exchange struct {
	Fragment graph.Fragment
	Files    map[string]string
	Requests []Request
}

// repoDraw draws a small repository's graph and files: a system with a person
// and an external system; components with types, operations, a flow and a
// property with cases; and a Go package for each, whose file declares the
// code types and functions that realize the design. Edges of every kind
// join them as docs/model.md §4 draws them, derived ones included, and
// salience is drawn, so the graph need not conform.
type repoDraw struct {
	t     *rapid.T
	f     graph.Fragment
	ids   map[string]graph.Node
	edges map[string]bool
	files map[string][]string
}

func (r *repoDraw) node(n graph.Node) string {
	if _, ok := r.ids[n.Id]; ok {
		return n.Id
	}
	if n.Source == "" {
		n.Source = graph.SourceDeclared
	}
	n.Salient = rapid.IntRange(0, 5).Draw(r.t, "detail?") > 0
	r.ids[n.Id] = n
	r.f.Nodes = append(r.f.Nodes, n)
	return n.Id
}

// design adds a design node written on the next line of its design file.
func (r *repoDraw) design(id string, kind graph.NodeKind, form, file string, attrs map[string][]string) string {
	r.files[file] = append(r.files[file], fmt.Sprintf("%s %s %s", form, kind, id))
	return r.node(graph.Node{Id: id, Kind: kind, Form: form, Layer: graph.LayerDesign, Summary: "the " + string(kind) + " " + id,
		Origin: diag.Position{Path: file, Line: len(r.files[file]), Column: 1}, Attrs: attrs})
}

// code adds a code type or function declared at the end of a Go file.
func (r *repoDraw) code(id string, kind graph.NodeKind, form, file string, decl []string, attrs map[string][]string) string {
	r.files[file] = append(r.files[file], decl...)
	line := len(r.files[file]) - len(decl) + 1
	return r.node(graph.Node{Id: id, Kind: kind, Form: form, Layer: graph.LayerCode, Source: graph.SourceExtracted,
		Origin: diag.Position{Path: file, Line: line, Column: 1}, Attrs: attrs})
}

func (r *repoDraw) edge(kind graph.EdgeKind, from, to string, attrs map[string][]string) {
	key := string(kind) + " " + from + " " + to
	if r.edges[key] || from == "" || to == "" {
		return
	}
	r.edges[key] = true
	source := graph.SourceDeclared
	if kind == graph.EdgeKindOwns || kind == graph.EdgeKindDepends || kind == graph.EdgeKindTraces || kind == graph.EdgeKindReaches || kind == graph.EdgeKindExercises {
		source = graph.SourceDerived
	}
	r.f.Edges = append(r.f.Edges, graph.Edge{Kind: kind, From: from, To: to, Source: source, Origin: r.ids[from].Origin, Attrs: attrs})
}

func (r *repoDraw) pick(label string, ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	return rapid.SampledFrom(ids).Draw(r.t, label)
}

func (r *repoDraw) of(kinds ...graph.NodeKind) []string {
	var ids []string
	for _, n := range r.f.Nodes {
		if slices.Contains(kinds, n.Kind) {
			ids = append(ids, n.Id)
		}
	}
	return ids
}

func genRepo(t *rapid.T) (graph.Fragment, map[string]string) {
	r := &repoDraw{t: t, ids: map[string]graph.Node{}, edges: map[string]bool{}, files: map[string][]string{}}
	notations := rapid.SliceOfNDistinct(rapid.SampledFrom([]string{"ddd", "c4"}), 0, 2, rapid.ID[string]).Draw(t, "notations")
	system := r.design("shop", graph.NodeKindContainer, "system", "design/system.uc", map[string][]string{"notations": notations, "roots": {"go: ."}})
	user := r.design("user", graph.NodeKindActor, "person", "design/system.uc", nil)
	gateway := r.design("gateway", graph.NodeKindActor, "external", "design/system.uc", nil)
	r.edge(graph.EdgeKindContains, system, user, nil)
	r.edge(graph.EdgeKindContains, system, gateway, nil)
	charge := r.design("gateway.charge", graph.NodeKindOp, "op", "design/system.uc", map[string][]string{"signature": {"(amount: int) -> string"}})
	r.edge(graph.EdgeKindContains, gateway, charge, nil)
	var components []string
	for i := range rapid.IntRange(1, 3).Draw(t, "components") {
		c := []string{"order", "stock", "audit"}[i]
		file := "design/" + c + ".uc"
		attrs := map[string][]string{"focus": {rapid.SampledFrom([]string{"core", "support", "utility"}).Draw(t, "focus")}, "paths": {"src/" + c + "/**"}}
		if labels := rapid.SliceOfNDistinct(rapid.SampledFrom([]string{"bounded-context", "container"}), 0, 2, rapid.ID[string]).Draw(t, "labels"); len(labels) > 0 {
			attrs["labels"] = labels
		}
		r.edge(graph.EdgeKindContains, system, r.design(c, graph.NodeKindContainer, "component", file, attrs), nil)
		components = append(components, c)
		r.designOf(c, file)
	}
	for _, c := range components {
		r.codeOf(c)
	}
	r.flows(components, user)
	r.links(components)
	return r.f, r.text()
}

// designOf adds a component's types, with fields, values and operations, its
// free operation and its property with cases.
func (r *repoDraw) designOf(c, file string) {
	t := r.t
	for k := range rapid.IntRange(0, 3).Draw(t, "types") {
		form := rapid.SampledFrom([]string{"record", "object", "enum", "error", "event"}).Draw(t, "typeForm")
		attrs := map[string][]string{}
		if form == "enum" {
			attrs["values"] = rapid.SliceOfNDistinct(rapid.SampledFrom([]string{"draft", "paid", "sent"}), 1, 3, rapid.ID[string]).Draw(t, "values")
		} else if form != "error" {
			attrs["fields"] = []string{"id: string"}
			if others := r.of(graph.NodeKindType); len(others) > 0 && rapid.Bool().Draw(t, "refField?") {
				attrs["fields"] = append(attrs["fields"], "of: list<"+r.pick("fieldType", others)+">")
			}
		}
		typ := r.design(fmt.Sprintf("%s.T%d", c, k), graph.NodeKindType, form, file, attrs)
		r.edge(graph.EdgeKindContains, c, typ, nil)
		for j := range rapid.IntRange(0, 2).Draw(t, "ops") {
			r.edge(graph.EdgeKindContains, typ, r.design(fmt.Sprintf("%s.op%d", typ, j), graph.NodeKindOp, "op", file, map[string][]string{"signature": {r.signature()}}), nil)
		}
	}
	if rapid.Bool().Draw(t, "freeOp?") {
		r.edge(graph.EdgeKindContains, c, r.design(c+".run", graph.NodeKindOp, "op", file, map[string][]string{"signature": {r.signature()}}), nil)
	}
	if rapid.Bool().Draw(t, "property?") {
		p := r.design(c+".holds", graph.NodeKindProperty, rapid.SampledFrom([]string{"invariant", "relation"}).Draw(t, "propertyForm"), file,
			map[string][]string{"symbols": {"x: int"}})
		r.edge(graph.EdgeKindContains, c, p, nil)
		for k := range rapid.IntRange(0, 2).Draw(t, "cases") {
			form := rapid.SampledFrom([]string{"declared", "recorded"}).Draw(t, "caseForm")
			where := file
			if form == "recorded" {
				where = "corpus/" + c + "/holds.uc"
			}
			r.edge(graph.EdgeKindContains, p, r.design(fmt.Sprintf("%s.c%d", p, k), graph.NodeKindCase, form, where, map[string][]string{"values": {"x = " + strconv.Itoa(k)}}), nil)
		}
	}
}

// signature draws an operation's signature over the declared types, which
// it refers to.
func (r *repoDraw) signature() string {
	types := r.of(graph.NodeKindType)
	if len(types) == 0 || !rapid.Bool().Draw(r.t, "typed?") {
		return "(x: int) -> int"
	}
	in, out := r.pick("in", types), r.pick("out", types)
	sig := "(x: " + in + ") -> " + out
	if errs := slices.DeleteFunc(slices.Clone(types), func(id string) bool { return r.ids[id].Form != "error" }); len(errs) > 0 && rapid.Bool().Draw(r.t, "raises?") {
		sig += " raises " + r.pick("raised", errs)
	}
	return sig
}

// codeOf adds a component's Go package and its files: always one, now and
// then a second and a _test.go file, named so that their paths sort in the
// opposite order to how the code tab ranks them. Each file gets some of the code: a code
// type for each of the component's types with a method for each operation,
// and a function for each free operation, each realizing its design node now
// and then, and the predicates of its properties.
func (r *repoDraw) codeOf(c string) {
	t := r.t
	pkg := "go:example.com/shop/src/" + c
	paths := []string{"src/" + c + "/" + c + ".go"}
	if rapid.Bool().Draw(t, "secondFile?") {
		paths = append(paths, "src/"+c+"/b.go")
	}
	if rapid.Bool().Draw(t, "testFile?") {
		paths = append(paths, "src/"+c+"/a_test.go")
	}
	r.node(graph.Node{Id: pkg, Kind: graph.NodeKindContainer, Form: "package", Layer: graph.LayerCode, Source: graph.SourceExtracted,
		Origin: diag.Position{Path: paths[0], Line: 1, Column: 1}, Attrs: map[string][]string{"language": {"go"}, "path": {"example.com/shop/src/" + c}}})
	for _, file := range paths {
		r.files[file] = []string{"package " + c, ""}
		fileID := r.node(graph.Node{Id: "file:" + file, Kind: graph.NodeKindContainer, Form: "file", Layer: graph.LayerCode, Source: graph.SourceExtracted,
			Origin: diag.Position{Path: file, Line: 1, Column: 1}, Attrs: map[string][]string{"language": {"go"}, "path": {file}}})
		r.edge(graph.EdgeKindContains, pkg, fileID, nil)
		r.edge(graph.EdgeKindOwns, c, fileID, nil)
	}
	for _, d := range r.children(c) {
		n := r.ids[d]
		name := d[strings.LastIndex(d, ".")+1:]
		file := rapid.SampledFrom(paths).Draw(t, "file")
		fileID := "file:" + file
		switch n.Kind {
		case graph.NodeKindType:
			typ := r.code(pkg+"."+name, graph.NodeKindType, "struct", file, []string{"// " + name + " is drawn.", "type " + name + " struct {", "\tID string", "}", ""},
				map[string][]string{"name": {name}, "fields": rapid.SampledFrom([][]string{{"id: string"}, {"id: int"}, {}}).Draw(t, "codeFields")})
			r.edge(graph.EdgeKindContains, fileID, typ, nil)
			if rapid.IntRange(0, 3).Draw(t, "realized?") > 0 {
				r.edge(graph.EdgeKindRealizes, typ, d, nil)
			}
			for _, op := range r.children(d) {
				method := "Op" + strings.TrimPrefix(op[strings.LastIndex(op, ".")+1:], "op")
				// A method may be written in another file than its type, as Go allows.
				m := r.code(typ+"."+method, graph.NodeKindOp, "method", rapid.SampledFrom(paths).Draw(t, "methodFile"), []string{"func (" + name + ") " + method + "() {", "}", ""},
					map[string][]string{"name": {strings.ToLower(method[:1]) + method[1:]}, "signature": {r.ids[op].Attrs["signature"][0]}, "stub": {"false"}, "testKind": {"none"}})
				r.edge(graph.EdgeKindContains, typ, m, nil)
				if rapid.Bool().Draw(t, "methodRealizes?") {
					r.edge(graph.EdgeKindRealizes, m, op, nil)
				}
			}
		case graph.NodeKindOp:
			f := r.code(pkg+".Run", graph.NodeKindOp, "func", file, []string{"func Run() {", "}", ""},
				map[string][]string{"name": {"run"}, "signature": {n.Attrs["signature"][0]}, "stub": {"false"}, "testKind": {"none"}})
			r.edge(graph.EdgeKindContains, fileID, f, nil)
			r.edge(graph.EdgeKindRealizes, f, d, nil)
		case graph.NodeKindProperty:
			f := r.code(pkg+".checkHolds", graph.NodeKindOp, "func", file, []string{"func checkHolds(x int) {", "}", ""},
				map[string][]string{"name": {"holds"}, "signature": {"(x: int)"}, "stub": {"false"}, "testKind": {"none"}})
			r.edge(graph.EdgeKindContains, fileID, f, nil)
			r.edge(graph.EdgeKindChecks, f, d, nil)
		}
	}
	for k := range rapid.IntRange(0, 2).Draw(t, "helpers") {
		file := rapid.SampledFrom(paths).Draw(t, "helperFile")
		h := r.code(fmt.Sprintf("%s.helper%d", pkg, k), graph.NodeKindOp, "func", file, []string{fmt.Sprintf("func helper%d() {", k), "}", ""},
			map[string][]string{"name": {fmt.Sprintf("helper%d", k)}, "signature": {"()"}, "stub": {"false"}, "testKind": {"none"}})
		r.edge(graph.EdgeKindContains, "file:"+file, h, nil)
	}
}

func (r *repoDraw) children(id string) []string {
	var ids []string
	for _, e := range r.f.Edges {
		if e.Kind == graph.EdgeKindContains && e.From == id {
			ids = append(ids, e.To)
		}
	}
	return ids
}

// flows adds a flow to the first component: steps that a person, a
// component or a type sends, calling operations, emitting events and
// handling errors, some nested, each call traced to a function now and then.
func (r *repoDraw) flows(components []string, user string) {
	t := r.t
	if !rapid.Bool().Draw(t, "flow?") {
		return
	}
	c := components[0]
	file := "design/" + c + ".uc"
	flow := r.design(c+".submit", graph.NodeKindFlow, "flow", file, map[string][]string{"symbols": {"x: int"}})
	r.edge(graph.EdgeKindContains, c, flow, nil)
	ops, types := r.of(graph.NodeKindOp), r.of(graph.NodeKindType)
	var steps []string
	for i := range rapid.IntRange(1, 5).Draw(t, "steps") {
		form := rapid.SampledFrom([]string{"call", "call", "emit", "on", "opt"}).Draw(t, "stepForm")
		step := r.design(fmt.Sprintf("%s#%d", flow, i+1), graph.NodeKindStep, form, file, map[string][]string{"index": {strconv.Itoa(i + 1)}, "args": {"x"}})
		parent := flow
		if len(steps) > 0 && rapid.Bool().Draw(t, "nested?") {
			parent = r.pick("parentStep", steps)
		}
		r.edge(graph.EdgeKindContains, parent, step, nil)
		steps = append(steps, step)
		r.edge(graph.EdgeKindFrom, step, r.pick("sender", append([]string{user}, append(slices.Clone(components), types...)...)), nil)
		switch form {
		case "call":
			op := r.pick("called", ops)
			r.edge(graph.EdgeKindCalls, step, op, nil)
			for _, f := range r.realizers(op) {
				r.edge(graph.EdgeKindTraces, step, f, nil)
			}
		case "emit":
			r.edge(graph.EdgeKindEmits, step, r.pick("emitted", types), nil)
		case "on":
			r.edge(graph.EdgeKindHandles, step, r.pick("handled", types), nil)
		}
	}
	if fs := r.of(graph.NodeKindOp); rapid.Bool().Draw(t, "flowPredicate?") {
		if f := r.pick("flowPredicate", slices.DeleteFunc(fs, func(id string) bool { return r.ids[id].Layer != graph.LayerCode })); f != "" {
			r.edge(graph.EdgeKindChecks, f, flow, nil)
		}
	}
}

func (r *repoDraw) realizers(id string) []string {
	var ids []string
	for _, e := range r.f.Edges {
		if e.Kind == graph.EdgeKindRealizes && e.To == id {
			ids = append(ids, e.From)
		}
	}
	return ids
}

// links adds the edges between what the components declare: uses and
// depends, refers from operations to the types of their signatures, about,
// subscribes, extends and implements, and calls, reaches and realizes in the code.
func (r *repoDraw) links(components []string) {
	t := r.t
	for _, a := range components {
		for _, b := range append(slices.Clone(components), "gateway") {
			if a != b && rapid.IntRange(0, 2).Draw(t, "uses?") == 0 {
				r.edge(graph.EdgeKindUses, a, b, nil)
			}
			if a != b && rapid.IntRange(0, 2).Draw(t, "depends?") == 0 {
				r.edge(graph.EdgeKindDepends, a, b, map[string][]string{"imports": {"file:src/" + a + "/" + a + ".go go:example.com/shop/src/" + b}})
			}
		}
	}
	types := r.of(graph.NodeKindType)
	for _, n := range slices.Clone(r.f.Nodes) {
		switch {
		case n.Layer == graph.LayerDesign && n.Kind == graph.NodeKindOp:
			for _, word := range strings.FieldsFunc(n.Attrs["signature"][0], func(c rune) bool { return strings.ContainsRune("(), <>:", c) }) {
				if slices.Contains(types, word) {
					r.edge(graph.EdgeKindRefers, n.Id, word, nil)
				}
			}
			if events := slices.DeleteFunc(slices.Clone(types), func(id string) bool { return r.ids[id].Form != "event" }); len(events) > 0 && rapid.IntRange(0, 3).Draw(t, "subscribes?") == 0 {
				r.edge(graph.EdgeKindSubscribes, n.Id, r.pick("event", events), nil)
			}
		case n.Kind == graph.NodeKindType && n.Layer == graph.LayerDesign:
			for _, f := range n.Attrs["fields"] {
				for _, word := range strings.FieldsFunc(f, func(c rune) bool { return strings.ContainsRune(", <>:", c) }) {
					if slices.Contains(types, word) {
						r.edge(graph.EdgeKindRefers, n.Id, word, nil)
					}
				}
			}
			if len(types) > 1 && rapid.IntRange(0, 4).Draw(t, "extends?") == 0 {
				r.edge(rapid.SampledFrom([]graph.EdgeKind{graph.EdgeKindExtends, graph.EdgeKindImplements}).Draw(t, "super"), n.Id, r.pick("supertype", types), nil)
			}
		case n.Kind == graph.NodeKindProperty:
			r.edge(graph.EdgeKindAbout, n.Id, r.pick("about", append(r.of(graph.NodeKindOp), types...)), nil)
			r.edge(graph.EdgeKindRefers, n.Id, r.pick("symbolType", types), nil)
		}
	}
	var functions []string
	for _, n := range r.f.Nodes {
		if n.Layer == graph.LayerCode && n.Kind == graph.NodeKindOp {
			functions = append(functions, n.Id)
		}
	}
	for range rapid.IntRange(0, 6).Draw(t, "calls") {
		from, to := r.pick("caller", functions), r.pick("callee", functions)
		kind := rapid.SampledFrom([]graph.EdgeKind{graph.EdgeKindCalls, graph.EdgeKindCalls, graph.EdgeKindReaches}).Draw(t, "callKind")
		attrs := map[string][]string{"sites": {r.ids[from].Origin.Path + ":" + strconv.Itoa(r.ids[from].Origin.Line) + ":2"}}
		if kind == graph.EdgeKindReaches {
			attrs = map[string][]string{"via": {r.pick("via", functions)}}
		}
		r.edge(kind, from, to, attrs)
	}
}

// text writes each drawn file as its lines.
func (r *repoDraw) text() map[string]string {
	out := map[string]string{}
	for path, lines := range r.files {
		out[path] = strings.Join(lines, "\n") + "\n"
	}
	return out
}

// endpoints are the paths the requests go to, each with how to draw its
// query parameters.
var endpoints = map[string]func(r *requests) url.Values{
	"/api/snapshot":    func(*requests) url.Values { return nil },
	"/api/tree":        func(*requests) url.Values { return nil },
	"/api/diagnostics": func(*requests) url.Values { return nil },
	"/api/revisions":   func(r *requests) url.Values { return r.optional("limit", r.number) },
	"/api/entity": func(r *requests) url.Values {
		q := url.Values{"id": {r.id("entity")}}
		if rapid.IntRange(0, 5).Draw(r.t, "base?") == 0 {
			q["base"] = []string{"HEAD"}
		}
		return q
	},
	"/api/edge": func(r *requests) url.Values {
		e := rapid.SampledFrom(r.f.Edges).Draw(r.t, "edge")
		q := url.Values{"kind": {string(e.Kind)}, "from": {e.From}, "to": {e.To}}
		if rapid.IntRange(0, 5).Draw(r.t, "otherEdge?") == 0 {
			q["to"] = []string{r.id("edgeTo")}
		}
		return q
	},
	"/api/nodes":  func(r *requests) url.Values { return url.Values{"id": r.ids("nodes", 1)} },
	"/api/node":   func(r *requests) url.Values { return url.Values{"id": {r.id("node")}} },
	"/api/search": func(r *requests) url.Values { return r.search() },
	"/api/slice":  func(r *requests) url.Values { return r.slice() },
	"/api/impact": func(r *requests) url.Values { return url.Values{"id": r.ids("impact", 1)} },
	"/api/code":   func(r *requests) url.Values { return r.code() },
	"/api/compare": func(r *requests) url.Values {
		return url.Values{"base": {rapid.SampledFrom([]string{"HEAD", "HEAD", "-x", "nope", ""}).Draw(r.t, "base")}}
	},
	"/api/views":   func(r *requests) url.Values { return r.view() },
	"/api/reviews": func(r *requests) url.Values { return r.reviews() },
	"/api/nowhere": func(*requests) url.Values { return nil },
}

// queryEndpoints are the endpoints that answer as query, view, graph.Compare
// and review.Evaluate do, and the entity page, whose review is review.Evaluate's.
var queryEndpoints = []string{"/api/node", "/api/search", "/api/slice", "/api/impact", "/api/views", "/api/compare", "/api/reviews", "/api/entity"}

// requests draws requests about one graph: identities are mostly its nodes'.
type requests struct {
	t     *rapid.T
	f     graph.Fragment
	files map[string]string
	path  string
}

func (r *requests) id(label string) string {
	if rapid.IntRange(0, 6).Draw(r.t, label+"Unknown?") == 0 {
		return rapid.SampledFrom([]string{"nowhere", "order.Nothing", "go:example.com/none"}).Draw(r.t, label+"Unknown")
	}
	return rapid.SampledFrom(r.f.Nodes).Draw(r.t, label).Id
}

func (r *requests) ids(label string, least int) []string {
	var out []string
	for range rapid.IntRange(least, 3).Draw(r.t, label+"Count") {
		out = append(out, r.id(label))
	}
	return out
}

func (r *requests) number(label string) string {
	return strconv.Itoa(rapid.IntRange(-1, 8).Draw(r.t, label))
}

func (r *requests) optional(key string, value func(string) string) url.Values {
	if rapid.Bool().Draw(r.t, key+"?") {
		return url.Values{key: {value(key)}}
	}
	return nil
}

func (r *requests) search() url.Values {
	text := r.id("searched")
	runes := []rune(text)
	start := rapid.IntRange(0, len(runes)).Draw(r.t, "start")
	q := url.Values{"text": {string(runes[start:rapid.IntRange(start, len(runes)).Draw(r.t, "end")])}}
	if rapid.Bool().Draw(r.t, "kind?") {
		q["kind"] = []string{rapid.SampledFrom([]string{"type", "op", "container", "step", "flow", "bogus"}).Draw(r.t, "kind")}
	}
	maps.Copy(q, r.optional("limit", r.number))
	return q
}

func (r *requests) slice() url.Values {
	q := url.Values{"id": r.ids("focus", 1)}
	if expand := r.ids("expand", 0); len(expand) > 0 {
		q["expand"] = expand
	}
	if rapid.Bool().Draw(r.t, "withCode?") {
		q["withCode"] = []string{"true"}
	}
	return q
}

func (r *requests) code() url.Values {
	paths := slices.Sorted(maps.Keys(r.files))
	q := url.Values{"path": {rapid.SampledFrom(append(paths, "/etc/passwd", "../x", "design/none.uc")).Draw(r.t, "codePath")}}
	maps.Copy(q, r.optional("from", r.number))
	maps.Copy(q, r.optional("to", r.number))
	return q
}

// reviews draws the parameters of a review listing: zero to three identities,
// pending true, false or not given, and about a third of the time the base
// commit HEAD.
func (r *requests) reviews() url.Values {
	q := url.Values{}
	if ids := r.ids("reviewed", 0); len(ids) > 0 {
		q["id"] = ids
	}
	if pending := rapid.SampledFrom([]string{"", "true", "false"}).Draw(r.t, "pending"); pending != "" {
		q["pending"] = []string{pending}
	}
	if rapid.IntRange(0, 2).Draw(r.t, "reviewBase?") == 0 {
		q["base"] = []string{"HEAD"}
	}
	return q
}

// view draws a view kind, now and then one that does not exist, and its
// input and expansions; now and then a layer or a level, given to any view
// and now and then naming no value.
func (r *requests) view() url.Values {
	kind := rapid.SampledFrom([]string{"architecture", "ddd", "class", "sequence", "activity", "events", "calls", "property", "code", "c4", "bogus"}).Draw(r.t, "view")
	r.path = "/api/views/" + kind
	q := url.Values{}
	if rapid.IntRange(0, 3).Draw(r.t, "input?") > 0 {
		q["input"] = []string{r.id("input")}
	}
	for range rapid.IntRange(0, 2).Draw(r.t, "expandCount") {
		x := r.id("expand")
		if rapid.IntRange(0, 3).Draw(r.t, "reaches?") == 0 {
			x = "reaches " + x + " " + r.id("reachesTo")
		}
		q["expand"] = append(q["expand"], x)
	}
	if rapid.IntRange(0, 3).Draw(r.t, "layer?") == 0 {
		q["layer"] = []string{rapid.SampledFrom([]string{"design", "code", "code", "bogus"}).Draw(r.t, "layer")}
	}
	if rapid.IntRange(0, 3).Draw(r.t, "level?") == 0 {
		q["level"] = []string{rapid.SampledFrom([]string{"context", "context", "container", "bogus"}).Draw(r.t, "level")}
	}
	return q
}

// genExchangeOf draws a repository and one to six requests to the
// endpoints, a request now and then twice.
func genExchangeOf(paths []string) *rapid.Generator[Exchange] {
	return rapid.Custom(func(t *rapid.T) Exchange {
		f, files := genRepo(t)
		f = withReviews(t, f)
		x := Exchange{Fragment: f, Files: files}
		for range rapid.IntRange(1, 6).Draw(t, "requests") {
			if len(x.Requests) > 0 && rapid.IntRange(0, 4).Draw(t, "again?") == 0 {
				x.Requests = append(x.Requests, rapid.SampledFrom(x.Requests).Draw(t, "repeated"))
				continue
			}
			r := &requests{t: t, f: f, files: files, path: rapid.SampledFrom(paths).Draw(t, "endpoint")}
			q := endpoints[r.path](r)
			x.Requests = append(x.Requests, Request{Path: r.path, Query: q})
		}
		return x
	})
}

// withReviews adds to f the review layer of zero to three records that
// review.Write gives for candidate targets of f, by one of two reviewers,
// with either decision and with or without a comment; after about a third
// of the records about a node, the node's summary changes, so that the
// record no longer matches its content.
func withReviews(t *rapid.T, f graph.Fragment) graph.Fragment {
	g := graphOf(t, f)
	candidates := review.Evaluate(&g, nil, nil).Items
	count := rapid.IntRange(0, 3).Draw(t, "reviews")
	if len(candidates) == 0 || count == 0 {
		return f
	}
	dir, err := os.MkdirTemp("", "unicode-reviews-")
	if err != nil {
		t.Fatalf("setup failed: cannot make a directory for the review store: %v", err)
	}
	defer removeAll(t, dir)
	var records []review.Record
	changed := map[string]bool{}
	for range count {
		target := rapid.SampledFrom(candidates).Draw(t, "reviewed").Target
		written := review.Write(dir, &g, nil, review.Request{Target: target, Reviewer: rapid.SampledFrom([]string{"reviewer", "张三"}).Draw(t, "reviewer"),
			Decision: rapid.SampledFrom([]string{"approved", "changes-requested"}).Draw(t, "decision"), Comment: rapid.SampledFrom([]string{"", "字段与代码一致"}).Draw(t, "comment")})
		if written.Record == nil {
			t.Fatalf("setup failed: review.Write refused a review of the candidate %+v: %v", target, written.Diagnostics)
		}
		records = append(records, *written.Record)
		if target.Node != "" && rapid.IntRange(0, 2).Draw(t, "changed?") == 0 {
			changed[target.Node] = true
		}
	}
	out := graph.Fragment{Nodes: slices.Clone(f.Nodes), Edges: slices.Clone(f.Edges)}
	for i, n := range out.Nodes {
		if changed[n.Id] {
			out.Nodes[i].Summary = n.Summary + " (changed after its review)"
		}
	}
	changedGraph := graphOf(t, out)
	attached := review.Attach(&changedGraph, records)
	if len(attached.Diagnostics) > 0 {
		t.Fatalf("setup failed: review.Attach refused the records review.Write gave: %v", attached.Diagnostics)
	}
	out.Nodes = append(out.Nodes, attached.Fragment.Nodes...)
	out.Edges = append(out.Edges, attached.Fragment.Edges...)
	return out
}

// genExchange draws requests to every endpoint, and to one that does not exist.
//
//uc:generates serve.answer-request.exchange
//uc:generates serve.answers-follow-contract.exchange
//uc:generates serve.answers-do-not-escape-html.exchange
//uc:generates serve.identities-resolve.exchange
//uc:generates serve.answers-are-deterministic.exchange
//uc:generates serve.snapshot-head-follows-record.exchange
//uc:generates serve.open-refuses-malformed-record.exchange
func genExchange() *rapid.Generator[Exchange] {
	return genExchangeOf(slices.Sorted(maps.Keys(endpoints)))
}

// genQueryExchange draws requests to the endpoints that query, view,
// graph.Compare and review.Evaluate answer, and to entity pages.
//
//uc:generates serve.queries-agree.exchange
func genQueryExchange() *rapid.Generator[Exchange] {
	return genExchangeOf(queryEndpoints)
}

// genCodeViewExchange draws code views whose input is one of the repository's
// code files or code types.
//
//uc:generates serve.views-name-code-files.exchange
func genCodeViewExchange() *rapid.Generator[Exchange] {
	return rapid.Custom(func(t *rapid.T) Exchange {
		f, files := genRepo(t)
		var inputs []string
		for _, n := range f.Nodes {
			if n.Layer == graph.LayerCode && (n.Kind == graph.NodeKindType || n.Kind == graph.NodeKindContainer && n.Form == "file") {
				inputs = append(inputs, n.Id)
			}
		}
		x := Exchange{Fragment: f, Files: files}
		for range rapid.IntRange(1, 4).Draw(t, "requests") {
			x.Requests = append(x.Requests, Request{Path: "/api/views/code", Query: map[string][]string{"input": {rapid.SampledFrom(inputs).Draw(t, "input")}}})
		}
		return x
	})
}

// genEntityExchange draws requests for entity pages.
//
//uc:generates serve.code-tab-ranks-files.exchange
func genEntityExchange() *rapid.Generator[Exchange] {
	return genExchangeOf([]string{"/api/entity"})
}

// genTabsExchange draws requests for the answers that give tabs: entity pages and views.
//
//uc:generates serve.tabs-open-views.exchange
func genTabsExchange() *rapid.Generator[Exchange] {
	return genExchangeOf([]string{"/api/entity", "/api/views"})
}

// served writes an exchange's repository to a new directory, commits its
// files when a request reads a revision, writes its snapshot at
// .unicode/graph.json, and opens a server on it whose base commits all have
// the graph baseOf gives.
func served(t rapid.TB, x Exchange) (*Server, string, graph.Graph) {
	root, g := written(t, x)
	s, err := Open(root, filepath.Join(root, ".unicode", "graph.json"), baseBuilder{graphOf(t, baseOf(x.Fragment))})
	if err != nil {
		t.Fatalf("Open refused the snapshot that Encode wrote of a graph with a system container: %v", err)
	}
	return s, root, g
}

// written writes an exchange's repository to a new directory, commits its
// files when a request reads a revision, and writes its snapshot at
// .unicode/graph.json.
func written(t rapid.TB, x Exchange) (string, graph.Graph) {
	root, err := os.MkdirTemp("", "unicode-serve-")
	if err != nil {
		t.Fatalf("setup failed: cannot make a directory for the repository: %v", err)
	}
	for path, text := range x.Files {
		writeFile(t, filepath.Join(root, filepath.FromSlash(path)), []byte(text))
	}
	if slices.ContainsFunc(x.Requests, readsRevision) {
		runGit(t, root, "init", "-q")
		runGit(t, root, "add", "-A")
		runGit(t, root, "-c", "user.name=unicode", "-c", "user.email=unicode@example.com", "-c", "commit.gpgsign=false",
			"-c", "core.hooksPath=/dev/null", "commit", "-q", "--allow-empty", "-m", "the drawn repository")
	}
	g := graphOf(t, x.Fragment)
	writeFile(t, filepath.Join(root, ".unicode", "graph.json"), g.Encode())
	return root, g
}

// readsRevision tells whether a request reads a commit of the repository.
func readsRevision(r Request) bool {
	_, base := r.Query["base"]
	return base || r.Path == "/api/compare" || r.Path == "/api/revisions"
}

// runGit runs git in root without the user's and the system's configuration.
func runGit(t rapid.TB, root string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("setup failed: git %s: %v\n%s", strings.Join(args, " "), err, stderr.String())
	}
	return strings.TrimSpace(string(out))
}

// baseBuilder gives the same graph for every commit.
type baseBuilder struct{ base graph.Graph }

func (b baseBuilder) Build(string, string) (graph.Graph, error) { return b.base, nil }

// baseOf is the graph a commit had before the snapshot: without the
// snapshot's last node other than the system and the edges at it, with
// another summary and another attribute value for its first node other
// than the system, so that its comparison adds, modifies and leaves alone.
func baseOf(f graph.Fragment) graph.Fragment {
	base := shrunk(f)
	for i, n := range base.Nodes {
		if n.Form != "system" {
			n.Summary += " before"
			n.Attrs = maps.Clone(n.Attrs)
			if n.Attrs == nil {
				n.Attrs = map[string][]string{}
			}
			n.Attrs["labels"] = append(slices.Clone(n.Attrs["labels"]), "older")
			base.Nodes[i] = n
			break
		}
	}
	return base
}

func writeFile(t rapid.TB, path string, data []byte) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("setup failed: cannot make the directory of %s: %v", path, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("setup failed: cannot write %s: %v", path, err)
	}
}

// rewrite writes another graph's snapshot over the served one and marks the
// file as written a second later, so that its state tells it apart.
func rewrite(t rapid.TB, root string, g graph.Graph, at time.Time) {
	path := filepath.Join(root, ".unicode", "graph.json")
	writeFile(t, path, g.Encode())
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatalf("setup failed: cannot set the time of the snapshot: %v", err)
	}
}

func graphOf(t rapid.TB, f graph.Fragment) graph.Graph {
	g := graph.NewGraph()
	for _, n := range f.Nodes {
		if err := g.AddNode(n); err != nil {
			t.Fatalf("setup failed: AddNode refused the node %q, whose identity is new: %v", n.Id, err)
		}
	}
	for _, e := range f.Edges {
		if err := g.AddEdge(e); err != nil {
			t.Fatalf("setup failed: AddEdge refused the new edge %s %q %q between nodes of the graph: %v", e.Kind, e.From, e.To, err)
		}
	}
	return g
}

func removeAll(t rapid.TB, root string) {
	if err := os.RemoveAll(root); err != nil {
		t.Fatalf("cannot remove the repository %s: %v", root, err)
	}
}
