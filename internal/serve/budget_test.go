package serve

import (
	"fmt"
	"math/rand/v2"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/view"
)

// The size of the budget repository, as unicode's own graph has it: 18
// design components, 234 code containers, 4104 functions, 1080 code types,
// 5771 nodes in all, and 36000 edges, most of them exercises, refers and
// calls.
const (
	budgetComponents = 18
	budgetFiles      = 12 // per component
	budgetTypes      = 5  // code types per file
	budgetFunctions  = 19 // functions per file
	budgetExercised  = 170
)

// budgetRepo is a repository the answering budgets are measured on, its
// snapshot written and a server opened on it.
type budgetRepo struct {
	fragment graph.Fragment
	server   *Server
	// inputs are the inputs each view kind takes in the repository's graph.
	inputs map[view.ViewKind][]string
}

// budgetRepos are the budget repositories by seed, each built once and shared
// by the cases of that seed; budgetDirs are their directories.
var (
	budgetRepos sync.Map
	budgetMu    sync.Mutex
	budgetDirs  []string
)

// budgetOf is the budget repository of seed, built when first asked for; it
// panics when the repository cannot be written or opened, since generators
// have no test to fail.
func budgetOf(seed int) budgetRepo {
	built, _ := budgetRepos.LoadOrStore(seed, sync.OnceValues(func() (budgetRepo, error) { return writeBudgetRepo(seed) }))
	repo, err := built.(func() (budgetRepo, error))()
	if err != nil {
		panic(fmt.Sprintf("setup failed: cannot write and open the budget repository of seed %d: %v", seed, err))
	}
	return repo
}

func writeBudgetRepo(seed int) (budgetRepo, error) {
	f, files := bulkRepo(seed)
	root, err := os.MkdirTemp("", "unicode-serve-budget-")
	if err != nil {
		return budgetRepo{}, err
	}
	budgetMu.Lock()
	budgetDirs = append(budgetDirs, root)
	budgetMu.Unlock()
	g := graph.NewGraph()
	for _, n := range f.Nodes {
		if err := g.AddNode(n); err != nil {
			return budgetRepo{}, err
		}
	}
	for _, e := range f.Edges {
		if err := g.AddEdge(e); err != nil {
			return budgetRepo{}, err
		}
	}
	files[".unicode/graph.json"] = string(g.Encode())
	for path, text := range files {
		target := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return budgetRepo{}, err
		}
		if err := os.WriteFile(target, []byte(text), 0o644); err != nil {
			return budgetRepo{}, err
		}
	}
	s, err := Open(root, filepath.Join(root, ".unicode", "graph.json"), nil)
	if err != nil {
		return budgetRepo{}, err
	}
	inputs := map[view.ViewKind][]string{}
	for _, kind := range viewKinds {
		inputs[kind] = view.Inputs(&g, kind)
	}
	return budgetRepo{fragment: f, server: s, inputs: inputs}, nil
}

// TestMain removes the budget repositories that cases made, after the tests.
func TestMain(m *testing.M) {
	code := m.Run()
	for _, dir := range budgetDirs {
		_ = os.RemoveAll(dir)
	}
	os.Exit(code)
}

// genBudgetServer is the server opened on the budget repository of seed.
//
//uc:generates serve.pages-answer-within-budget.server
//uc:generates serve.queries-answer-within-budget.server
func genBudgetServer(seed int) *rapid.Generator[*Server] {
	return rapid.Just(budgetOf(seed).server)
}

// genPageRequest draws a request of the viewer's pages to the budget
// repository of seed: the snapshot, the design tree, the diagnostics, an
// entity page, a group of nodes, an edge, or a search for part of an
// identity.
//
//uc:generates serve.pages-answer-within-budget.request
func genPageRequest(seed int) *rapid.Generator[Request] {
	f := budgetOf(seed).fragment
	return rapid.Custom(func(t *rapid.T) Request {
		node := func(label string) string { return rapid.SampledFrom(f.Nodes).Draw(t, label).Id }
		switch path := rapid.SampledFrom([]string{"/api/snapshot", "/api/tree", "/api/diagnostics", "/api/entity", "/api/nodes", "/api/edge", "/api/search"}).Draw(t, "endpoint"); path {
		case "/api/entity":
			return Request{Path: path, Query: url.Values{"id": {node("entity")}}}
		case "/api/nodes":
			ids := []string{}
			for range rapid.IntRange(1, 3).Draw(t, "count") {
				ids = append(ids, node("node"))
			}
			return Request{Path: path, Query: url.Values{"id": ids}}
		case "/api/edge":
			e := rapid.SampledFrom(f.Edges).Draw(t, "edge")
			return Request{Path: path, Query: url.Values{"kind": {string(e.Kind)}, "from": {e.From}, "to": {e.To}}}
		case "/api/search":
			id := node("searched")
			start := rapid.IntRange(0, len(id)-1).Draw(t, "start")
			return Request{Path: path, Query: url.Values{"text": {id[start:rapid.IntRange(start+1, len(id)).Draw(t, "end")]}}}
		default:
			return Request{Path: path, Query: url.Values{}}
		}
	})
}

// genQueryRequest draws a query or view request to the budget repository of
// seed: a node with its edges, a slice, an impact, the reviews of a node,
// pending or all, or a view of a kind and one of its inputs.
//
//uc:generates serve.queries-answer-within-budget.request
func genQueryRequest(seed int) *rapid.Generator[Request] {
	repo := budgetOf(seed)
	return rapid.Custom(func(t *rapid.T) Request {
		node := func(label string) string { return rapid.SampledFrom(repo.fragment.Nodes).Draw(t, label).Id }
		switch path := rapid.SampledFrom([]string{"/api/node", "/api/slice", "/api/impact", "/api/reviews", "/api/views"}).Draw(t, "endpoint"); path {
		case "/api/reviews":
			return Request{Path: path, Query: url.Values{"id": {node("reviewed")}, "pending": {fmt.Sprint(rapid.Bool().Draw(t, "pending"))}}}
		case "/api/views":
			kind := rapid.SampledFrom(viewKinds).Draw(t, "kind")
			return Request{Path: path + "/" + string(kind), Query: url.Values{"input": {rapid.SampledFrom(repo.inputs[kind]).Draw(t, "input")}}}
		default:
			return Request{Path: path, Query: url.Values{"id": {node("id")}}}
		}
	})
}

// bulk builds a budget repository from a seed.
type bulk struct {
	rng     *rand.Rand
	f       graph.Fragment
	lines   map[string][]string
	origins map[string]diag.Position
	edges   map[string]bool
	// shown are the code containers to mark salient.
	shown map[string]bool
}

func (b *bulk) node(n graph.Node, path string, text ...string) string {
	b.lines[path] = append(b.lines[path], text...)
	n.Origin = diag.Position{Path: path, Line: len(b.lines[path]) - len(text) + 1, Column: 1}
	if n.Source == "" {
		n.Source = graph.SourceDeclared
	}
	n.Salient = n.Layer == graph.LayerDesign
	b.origins[n.Id] = n.Origin
	b.f.Nodes = append(b.f.Nodes, n)
	return n.Id
}

func (b *bulk) design(id string, kind graph.NodeKind, form, path string, attrs map[string][]string) string {
	return b.node(graph.Node{Id: id, Kind: kind, Form: form, Layer: graph.LayerDesign, Summary: "the " + form + " " + id, Attrs: attrs},
		path, fmt.Sprintf("%s %s", form, id))
}

func (b *bulk) code(id string, kind graph.NodeKind, form, path string, attrs map[string][]string, text ...string) string {
	return b.node(graph.Node{Id: id, Kind: kind, Form: form, Layer: graph.LayerCode, Source: graph.SourceExtracted, Attrs: attrs}, path, text...)
}

func (b *bulk) edge(kind graph.EdgeKind, from, to string, attrs map[string][]string) {
	key := string(kind) + " " + from + " " + to
	if b.edges[key] || from == to {
		return
	}
	b.edges[key] = true
	source := graph.SourceDeclared
	switch {
	case kind == graph.EdgeKindOwns || kind == graph.EdgeKindDepends || kind == graph.EdgeKindTraces || kind == graph.EdgeKindReaches || kind == graph.EdgeKindExercises:
		source = graph.SourceDerived
	case strings.HasPrefix(from, "go:") || strings.HasPrefix(from, "file:"):
		if kind != graph.EdgeKindRealizes && kind != graph.EdgeKindChecks {
			source = graph.SourceExtracted
		}
	}
	b.f.Edges = append(b.f.Edges, graph.Edge{Kind: kind, From: from, To: to, Source: source, Origin: b.origins[from], Attrs: attrs})
}

func (b *bulk) pick(ids []string) string { return ids[b.rng.IntN(len(ids))] }

// bulkRepo builds a repository of unicode's size from seed: a
// system with a person; components with types, operations and properties,
// one of them manual and one with a case; a flow of steps in half of the
// components; for each component a Go package of files of types and
// functions that realize the design, check its properties, and call, refer
// to and exercise each other.
func bulkRepo(seed int) (graph.Fragment, map[string]string) {
	b := &bulk{rng: rand.New(rand.NewPCG(uint64(seed), 11)), lines: map[string][]string{}, origins: map[string]diag.Position{}, edges: map[string]bool{}, shown: map[string]bool{}}
	system := b.design("shop", graph.NodeKindContainer, "system", "design/system.uc", map[string][]string{"notations": {"ddd", "c4"}, "roots": {"go: ."}})
	user := b.design("user", graph.NodeKindActor, "person", "design/system.uc", nil)
	b.edge(graph.EdgeKindContains, system, user, nil)
	var components, types, ops []string
	for c := range budgetComponents {
		name := fmt.Sprintf("c%02d", c)
		file := "design/" + name + ".uc"
		component := b.design(name, graph.NodeKindContainer, "component", file, map[string][]string{"focus": {[]string{"core", "support", "utility"}[c%3]}, "paths": {name + "/**"}})
		b.edge(graph.EdgeKindContains, system, component, nil)
		components = append(components, component)
		for k := range 7 {
			typ := b.design(fmt.Sprintf("%s.T%d", name, k), graph.NodeKindType, []string{"record", "object", "enum", "error", "event", "record", "object"}[k], file, map[string][]string{"fields": {"id: string"}})
			b.edge(graph.EdgeKindContains, component, typ, nil)
			types = append(types, typ)
			if k < 3 {
				op := b.design(typ+".op", graph.NodeKindOp, "op", file, map[string][]string{"signature": {"(x: " + typ + ") -> " + typ}})
				b.edge(graph.EdgeKindContains, typ, op, nil)
				b.edge(graph.EdgeKindRefers, op, typ, nil)
				ops = append(ops, op)
			}
		}
		run := b.design(name+".run", graph.NodeKindOp, "op", file, map[string][]string{"signature": {"(x: int) -> int"}})
		b.edge(graph.EdgeKindContains, component, run, nil)
		ops = append(ops, run)
		for p := range 4 {
			attrs := map[string][]string{"symbols": {"x: " + name + ".T0"}}
			if p == 0 {
				attrs["manual"] = []string{"needs an environment"}
			}
			property := b.design(fmt.Sprintf("%s.holds%d", name, p), graph.NodeKindProperty, "invariant", file, attrs)
			b.edge(graph.EdgeKindContains, component, property, nil)
			b.edge(graph.EdgeKindAbout, property, b.pick(ops), nil)
			b.edge(graph.EdgeKindRefers, property, name+".T0", nil)
			if p == 1 {
				b.edge(graph.EdgeKindContains, property, b.design(property+".c0", graph.NodeKindCase, "declared", file, map[string][]string{"values": {"x = {id: \"a\"}"}}), nil)
			}
		}
	}
	for i, typ := range types {
		b.edge(graph.EdgeKindRefers, typ, b.pick(types), nil)
		if i%7 == 6 {
			b.edge(graph.EdgeKindExtends, typ, b.pick(types), nil)
		}
	}
	for _, c := range components {
		for range 3 {
			b.edge(graph.EdgeKindUses, c, b.pick(components), nil)
		}
	}
	for c, component := range components[:budgetComponents/2] {
		file := "design/" + component + ".uc"
		flow := b.design(component+".submit", graph.NodeKindFlow, "flow", file, map[string][]string{"symbols": {"x: int"}})
		b.edge(graph.EdgeKindContains, component, flow, nil)
		for s := range 4 {
			step := b.design(fmt.Sprintf("%s#%d", flow, s+1), graph.NodeKindStep, "call", file, map[string][]string{"index": {fmt.Sprint(s + 1)}, "args": {"x"}})
			b.edge(graph.EdgeKindContains, flow, step, nil)
			b.edge(graph.EdgeKindFrom, step, []string{user, component}[min(s, 1)], nil)
			b.edge(graph.EdgeKindCalls, step, ops[(c*8+s*3)%len(ops)], nil)
		}
	}
	b.codeOf(components)
	b.salience()
	files := map[string]string{}
	for path, lines := range b.lines {
		files[path] = strings.Join(lines, "\n") + "\n"
	}
	return b.f, files
}

// codeOf adds each component's Go package: files of types and functions,
// the bindings to the design, and the calls, refers, reaches, imports,
// implements and exercises among them. The packages are layered as the
// components are ordered: a function calls an earlier function of its
// package, or now and then one of an earlier package, and refers to a type
// of the package it calls into.
func (b *bulk) codeOf(components []string) {
	packageFunctions, packageTypes := map[string][]string{}, map[string][]string{}
	for _, c := range components {
		pkg := b.code("go:example.com/shop/"+c, graph.NodeKindContainer, "package", c+"/doc.go",
			map[string][]string{"language": {"go"}, "path": {"example.com/shop/" + c}}, "package "+c)
		b.shown[pkg] = true
		for k := range budgetFiles {
			path := fmt.Sprintf("%s/f%02d.go", c, k)
			file := b.code("file:"+path, graph.NodeKindContainer, "file", path, map[string][]string{"language": {"go"}, "path": {path}}, "package "+c, "")
			b.shown[file] = k%2 == 0
			b.edge(graph.EdgeKindContains, pkg, file, nil)
			b.edge(graph.EdgeKindOwns, c, file, nil)
			for j := range budgetTypes {
				name := fmt.Sprintf("Type%02d%d", k, j)
				typ := b.code(pkg+"."+name, graph.NodeKindType, "struct", path, map[string][]string{"name": {name}, "fields": {"id: string"}},
					"type "+name+" struct {", "\tID string", "}", "")
				b.edge(graph.EdgeKindContains, file, typ, nil)
				packageTypes[c] = append(packageTypes[c], typ)
				if k == 0 {
					b.edge(graph.EdgeKindRealizes, typ, fmt.Sprintf("%s.T%d", c, j), nil)
				}
			}
			for j := range budgetFunctions {
				name := fmt.Sprintf("func%02d%d", k, j)
				fn := b.code(pkg+"."+name, graph.NodeKindOp, "func", path,
					map[string][]string{"name": {name}, "signature": {"(x: int) -> int"}, "stub": {"false"}, "testKind": {"none"}},
					"func "+name+"(x int) int {", "\treturn x", "}", "")
				b.edge(graph.EdgeKindContains, file, fn, nil)
				packageFunctions[c] = append(packageFunctions[c], fn)
			}
			b.edge(graph.EdgeKindImports, "file:"+path, "go:example.com/shop/"+b.pick(components), nil)
		}
	}
	for _, c := range components {
		own := packageFunctions[c]
		for i, op := range []string{c + ".T0.op", c + ".T1.op", c + ".T2.op", c + ".run"} {
			b.edge(graph.EdgeKindRealizes, own[i], op, nil)
		}
		for p := range 4 {
			predicate := own[len(own)-1-p]
			b.edge(graph.EdgeKindChecks, predicate, fmt.Sprintf("%s.holds%d", c, p), nil)
			for range budgetExercised {
				b.edge(graph.EdgeKindExercises, predicate, b.pick(own), nil)
			}
		}
		for range 3 {
			other := b.pick(components)
			b.edge(graph.EdgeKindDepends, c, other, map[string][]string{"imports": {"file:" + c + "/f00.go go:example.com/shop/" + other}})
		}
	}
	for c, component := range components[:budgetComponents/2] {
		for s := range 4 {
			b.edge(graph.EdgeKindTraces, fmt.Sprintf("%s.submit#%d", component, s+1), packageFunctions[components[(c*8+s*3)%len(components)]][0], nil)
		}
	}
	for ci, c := range components {
		own, lower := packageFunctions[c], components[:ci]
		for i, fn := range own {
			for range 2 + b.rng.IntN(2) {
				switch {
				case len(lower) > 0 && b.rng.IntN(7) == 0:
					other := b.pick(lower)
					b.edge(graph.EdgeKindCalls, fn, b.pick(packageFunctions[other]), map[string][]string{"sites": {"1:1"}})
					b.edge(graph.EdgeKindRefers, fn, b.pick(packageTypes[other]), nil)
				case i > 0:
					b.edge(graph.EdgeKindCalls, fn, own[b.rng.IntN(i)], map[string][]string{"sites": {"1:1"}})
					b.edge(graph.EdgeKindRefers, fn, b.pick(packageTypes[c]), nil)
				}
			}
			if i > 1 && b.rng.IntN(12) == 0 {
				b.edge(graph.EdgeKindReaches, fn, own[b.rng.IntN(i-1)], map[string][]string{"via": {own[i-1]}})
			}
		}
		for _, typ := range packageTypes[c] {
			if b.rng.IntN(6) == 0 {
				b.edge(graph.EdgeKindImplements, typ, b.pick(packageTypes[c]), nil)
			}
		}
	}
}

// salience marks the code nodes salient as unicode's own graph has them:
// the packages, every other file, and the code elements that realize or
// check a design node; the other code elements are details.
func (b *bulk) salience() {
	for _, e := range b.f.Edges {
		if e.Kind == graph.EdgeKindRealizes || e.Kind == graph.EdgeKindChecks {
			b.shown[e.From] = true
		}
	}
	for i, n := range b.f.Nodes {
		if n.Layer == graph.LayerCode {
			b.f.Nodes[i].Salient = b.shown[n.Id]
		}
	}
}
