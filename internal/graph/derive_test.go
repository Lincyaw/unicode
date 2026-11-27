package graph

import (
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/diag"
)

// The directories and files that a derivation sample's paths and patterns
// are drawn from: nested directories, so that the deepest owner matters, and
// one directory that no pattern names but **.
var (
	sampleDirs  = []string{"a", "a/b", "c", "c/d"}
	sampleFiles = []string{"a/x.go", "a/y.go", "a/b/x.go", "c/x.go", "c/d/x.go", "e/x.go"}
)

// sampler builds a fragment node by node, giving every node and edge its own
// line, and every edge a new identity.
type sampler struct {
	t    *rapid.T
	f    Fragment
	keys map[edgeKey]bool
	line int
}

func (s *sampler) pos(path string) diag.Position {
	s.line++
	return diag.Position{Path: path, Line: s.line, Column: 1}
}

func (s *sampler) node(id string, layer Layer, kind NodeKind, form string, attrs map[string][]string) string {
	source, file := SourceDeclared, "design/sample.uc"
	if layer == LayerCode {
		source, file = SourceExtracted, "code.go"
	}
	s.f.Nodes = append(s.f.Nodes, Node{Id: id, Kind: kind, Form: form, Layer: layer, Source: source, Origin: s.pos(file), Attrs: attrs})
	return id
}

func (s *sampler) edge(kind EdgeKind, from, to string, source Source, attrs map[string][]string) {
	key := edgeKey{kind, from, to}
	if s.keys[key] || from == "" || to == "" {
		return
	}
	s.keys[key] = true
	s.f.Edges = append(s.f.Edges, Edge{Kind: kind, From: from, To: to, Source: source, Origin: s.pos("code.go"), Attrs: attrs})
}

func (s *sampler) contains(parent, child string) {
	s.edge(EdgeKindContains, parent, child, SourceDeclared, nil)
}

// pick draws one of the options, or "" when there are none.
func (s *sampler) pick(label string, options []string) string {
	if len(options) == 0 {
		return ""
	}
	return rapid.SampledFrom(options).Draw(s.t, label)
}

// subset draws some of the options, keeping their order.
func (s *sampler) subset(label string, options []string) []string {
	var out []string
	for _, o := range options {
		if rapid.Bool().Draw(s.t, label) {
			out = append(out, o)
		}
	}
	return out
}

func (s *sampler) pattern() string {
	dir := s.pick("dir", sampleDirs)
	return rapid.SampledFrom([]string{"**", dir + "/**", dir + "/**", dir + "/*.go", s.pick("file", sampleFiles), dir + "/["}).Draw(s.t, "pattern")
}

// sampleDesign is what the code part of a sample binds to.
type sampleDesign struct {
	types, ops, others, properties, observations []string
}

func (s *sampler) design() sampleDesign {
	var d sampleDesign
	containers := []string{s.node("sys", LayerDesign, NodeKindContainer, "system", nil)}
	for i := range rapid.IntRange(0, 4).Draw(s.t, "components") {
		parent := s.pick("parent", containers)
		id := fmt.Sprintf("k%d", i)
		if parent != "sys" {
			id = parent + "." + id
		}
		var paths []string
		for range rapid.IntRange(0, 2).Draw(s.t, "patterns") {
			paths = append(paths, s.pattern())
		}
		s.node(id, LayerDesign, NodeKindContainer, "component", map[string][]string{"paths": paths})
		s.contains(parent, id)
		containers = append(containers, id)
	}
	owners := containers[1:]
	if rapid.Bool().Draw(s.t, "external") {
		s.node("x", LayerDesign, NodeKindActor, "external", nil)
		s.contains("sys", "x")
		owners = append(owners, "x")
	}
	for _, c := range owners {
		for j := range rapid.IntRange(0, 2).Draw(s.t, "types") {
			id := s.node(fmt.Sprintf("%s.T%d", c, j), LayerDesign, NodeKindType, rapid.SampledFrom([]string{"object", "record", "interface"}).Draw(s.t, "form"), nil)
			s.contains(c, id)
			d.types = append(d.types, id)
			for _, name := range s.subset("typeOp", []string{"run", "stop"}) {
				d.ops = append(d.ops, s.node(id+"."+name, LayerDesign, NodeKindOp, "op", nil))
				s.contains(id, id+"."+name)
			}
		}
		for _, name := range s.subset("op", []string{"run", "load"}) {
			d.ops = append(d.ops, s.node(c+"."+name, LayerDesign, NodeKindOp, "op", nil))
			s.contains(c, c+"."+name)
		}
	}
	d.others = slices.Clone(containers)
	for _, c := range containers[1:] {
		if rapid.Bool().Draw(s.t, "flow") {
			flow := s.node(c+".f", LayerDesign, NodeKindFlow, "flow", nil)
			s.contains(c, flow)
			d.properties = append(d.properties, flow)
			n := 0
			s.steps(flow, flow, &n, d.ops, 2)
		}
		if rapid.Bool().Draw(s.t, "property") {
			p := s.node(c+".p", LayerDesign, NodeKindProperty, "invariant", map[string][]string{"symbols": {"v: int generated"}})
			s.contains(c, p)
			d.properties = append(d.properties, p)
			if rapid.Bool().Draw(s.t, "observation") {
				o := s.node(p+".state", LayerDesign, NodeKindObservation, "snapshot", map[string][]string{"type": {"int"}, "inputs": {"v"}})
				s.contains(p, o)
				d.observations = append(d.observations, o)
			}
		}
	}
	return d
}

// steps declares 1 to 3 steps under parent, numbered from n+1, each nesting
// up to depth more levels.
func (s *sampler) steps(flow, parent string, n *int, ops []string, depth int) {
	for range rapid.IntRange(1, 3).Draw(s.t, "steps") {
		*n++
		form := rapid.SampledFrom([]string{"call", "send", "emit", "async", "alt"}).Draw(s.t, "stepForm")
		id := s.node(fmt.Sprintf("%s#%d", flow, *n), LayerDesign, NodeKindStep, form, map[string][]string{"index": {fmt.Sprint(*n)}})
		s.contains(parent, id)
		if op := s.pick("calledOp", ops); op != "" && form != "emit" && form != "alt" {
			s.edge(EdgeKindCalls, id, op, SourceDeclared, nil)
		}
		if (form == "call" || form == "alt") && depth > 0 && rapid.Bool().Draw(s.t, "nested") {
			s.steps(flow, id, n, ops, depth-1)
		}
	}
}

func (s *sampler) code(d sampleDesign) {
	files := s.subset("file", sampleFiles)
	packages := map[string]bool{}
	for _, f := range files {
		dir := path.Dir(f)
		if !packages[dir] {
			packages[dir] = true
			s.node("go:m/"+dir, LayerCode, NodeKindContainer, "package", map[string][]string{"path": {"m/" + dir}, "language": {"go"}})
		}
		attrs := map[string][]string{"path": {f}, "language": {"go"}}
		if rapid.Bool().Draw(s.t, "generatedFile") {
			attrs["generated"] = []string{"true"}
		}
		s.node("file:"+f, LayerCode, NodeKindContainer, "file", attrs)
		s.contains("go:m/"+dir, "file:"+f)
	}
	if len(files) == 0 {
		return
	}
	missing := []string{"k9.Gone", "k0.gone"}
	var functions []string
	methods := map[string][]string{}
	for i := range rapid.IntRange(0, 4).Draw(s.t, "codeTypes") {
		f := s.pick("typeFile", files)
		// An interface's methods are op nodes too, of the form abstract.
		form, methodForm := rapid.SampledFrom([]string{"struct", "interface"}).Draw(s.t, "codeForm"), "method"
		if form == "interface" {
			methodForm = "abstract"
		}
		id := s.node(fmt.Sprintf("go:m/%s.C%d", path.Dir(f), i), LayerCode, NodeKindType, form, map[string][]string{"name": {fmt.Sprintf("C%d", i)}})
		s.contains("file:"+f, id)
		for _, name := range s.subset("method", []string{"run", "stop", "other"}) {
			m := s.node(id+"."+strings.ToUpper(name[:1])+name[1:], LayerCode, NodeKindOp, methodForm, map[string][]string{"name": {name}})
			s.contains(id, m)
			methods[id] = append(methods[id], m)
			functions = append(functions, m)
			if rapid.IntRange(0, 9).Draw(s.t, "methodBinding") == 0 {
				s.edge(EdgeKindRealizes, m, s.pick("methodTarget", d.ops), SourceDeclared, nil)
			}
		}
		if rapid.IntRange(0, 3).Draw(s.t, "typeBinding") > 0 {
			target := s.pick("typeTarget", slices.Concat(d.types, d.types, d.others, missing))
			s.edge(EdgeKindRealizes, id, target, SourceDeclared, nil)
			for _, m := range methods[id] {
				name := s.nodeAttr(m, "name")
				s.edge(EdgeKindRealizes, m, target+"."+name, SourceDerived, nil)
			}
		}
	}
	for i := range rapid.IntRange(0, 6).Draw(s.t, "funcs") {
		f := s.pick("funcFile", files)
		id := s.node(fmt.Sprintf("go:m/%s.F%d", path.Dir(f), i), LayerCode, NodeKindOp, "func",
			map[string][]string{"name": {rapid.SampledFrom([]string{"run", "load", "check"}).Draw(s.t, "funcName")}})
		s.contains("file:"+f, id)
		functions = append(functions, id)
		switch rapid.IntRange(0, 4).Draw(s.t, "funcBinding") {
		case 0:
			s.edge(EdgeKindRealizes, id, s.pick("funcTarget", slices.Concat(d.ops, d.ops, d.types, missing)), SourceDeclared, nil)
		case 1:
			s.edge(EdgeKindChecks, id, s.pick("checksTarget", slices.Concat(d.properties, d.properties, d.types, missing)), SourceDeclared, nil)
		case 2:
			s.edge(EdgeKindGenerates, id, s.pick("generatesTarget", slices.Concat(d.properties, missing)), SourceDeclared, map[string][]string{"symbols": {"v"}})
		case 3:
			s.edge(EdgeKindObserves, id, s.pick("observesTarget", slices.Concat(d.observations, missing)), SourceDeclared, nil)
		}
	}
	for range rapid.IntRange(0, 16).Draw(s.t, "calls") {
		from, to := s.pick("caller", functions), s.pick("callee", functions)
		if from != "" {
			s.edge(EdgeKindCalls, from, to, SourceExtracted, map[string][]string{"sites": {"code.go:1:1"}})
		}
	}
	for i := range s.f.Nodes {
		node := &s.f.Nodes[i]
		if node.Layer == LayerCode && (node.Kind == NodeKindType || node.Kind == NodeKindOp) && rapid.Bool().Draw(s.t, "explicitSourceFile") {
			node.Origin.Path = s.pick("sourceFile", files)
		}
	}
	pkgs := slices.Sorted(maps.Keys(packages))
	for range rapid.IntRange(0, 5).Draw(s.t, "imports") {
		s.edge(EdgeKindImports, "file:"+s.pick("importer", files), "go:m/"+s.pick("imported", pkgs), SourceExtracted, nil)
	}
}

func (s *sampler) nodeAttr(id, key string) string {
	for _, n := range s.f.Nodes {
		if n.Id == id {
			return n.Attrs[key][0]
		}
	}
	return ""
}

// genDeriveFragment draws a small design and code layer: nested components
// with path patterns (some invalid), an external system, types, operations,
// flows with steps and properties; files, types, methods, functions, calls,
// imports, and bindings, some to nodes that do not exist or are of the wrong
// kind, with the candidates a backend gives for methods.
//
//uc:generates graph.derive-matches-definition.fragment
func genDeriveFragment() *rapid.Generator[Fragment] {
	return rapid.Custom(func(t *rapid.T) Fragment {
		s := &sampler{t: t, keys: map[edgeKey]bool{}}
		s.code(s.design())
		return s.f
	})
}

// expectation is what docs/model.md §9 derives from a fragment, computed
// straight from the definitions without the graph: the derived edges with
// their attrs, the salient nodes, the edges that join the graph as they are,
// the diagnostics as `code subject`, and the origin of every node and edge.
type expectation struct {
	derived map[edgeKey]map[string][]string
	salient map[string]bool
	kept    map[edgeKey]bool
	diags   []string
	origins map[string]diag.Position

	nodes     []Node
	byID      map[string]Node
	edges     []Edge
	parent    map[string]string
	realizes  map[edgeKey]bool
	functions []string
}

func definition(f Fragment) expectation {
	x := expectation{derived: map[edgeKey]map[string][]string{}, salient: map[string]bool{}, kept: map[edgeKey]bool{},
		origins: map[string]diag.Position{}, nodes: f.Nodes, byID: map[string]Node{}, parent: map[string]string{},
		realizes: map[edgeKey]bool{}}
	for _, n := range f.Nodes {
		x.byID[n.Id] = n
		x.origins[n.Id] = n.Origin
		if x.is(n.Id, LayerCode, NodeKindOp) {
			x.functions = append(x.functions, n.Id)
		}
	}
	for _, e := range f.Edges {
		key := edgeKey{e.Kind, e.From, e.To}
		x.origins[string(e.Kind)+" "+e.From+" "+e.To] = e.Origin
		_, resolved := x.byID[e.To]
		switch {
		case e.Kind == EdgeKindRealizes && e.Source == SourceDerived:
		case !resolved:
			x.diags = append(x.diags, "graph.unresolved-binding "+string(e.Kind)+" "+e.From+" "+e.To)
		default:
			x.kept[key] = true
			x.edges = append(x.edges, e)
			if e.Kind == EdgeKindContains {
				x.parent[e.To] = e.From
			}
			if e.Kind == EdgeKindRealizes {
				x.realizes[key] = true
			}
		}
	}
	x.owns()
	x.methodRealizes()
	x.salience()
	x.depends()
	x.traces()
	x.exercises()
	x.reaches()
	slices.Sort(x.diags)
	return x
}

func (x *expectation) is(id string, layer Layer, kind NodeKind, forms ...string) bool {
	n, ok := x.byID[id]
	return ok && n.Layer == layer && n.Kind == kind && (len(forms) == 0 || slices.Contains(forms, n.Form))
}

func (x *expectation) has(kind EdgeKind, from, to string) bool {
	return x.kept[edgeKey{kind, from, to}]
}

func (x *expectation) derive(kind EdgeKind, from, to string, attrs map[string][]string) {
	x.derived[edgeKey{kind, from, to}] = attrs
}

func (x *expectation) ancestors(id string) []string {
	var out []string
	for p, ok := x.parent[id]; ok; p, ok = x.parent[p] {
		out = append(out, p)
	}
	return out
}

func (x *expectation) fileOf(id string) string {
	for _, a := range append([]string{id}, x.ancestors(id)...) {
		if x.is(a, LayerCode, NodeKindContainer, "file") {
			return a
		}
	}
	return ""
}

func (x *expectation) owners(id string) []string {
	var out []string
	for k := range x.derived {
		if k.kind == EdgeKindOwns && k.to == x.fileOf(id) {
			out = append(out, k.from)
		}
	}
	return out
}

func (x *expectation) calls(a, b string) bool {
	return x.has(EdgeKindCalls, a, b) && x.is(a, LayerCode, NodeKindOp) && x.is(b, LayerCode, NodeKindOp)
}

// owns: a file belongs to the deepest containers whose patterns match it.
func (x *expectation) owns() {
	for _, file := range x.nodes {
		if !x.is(file.Id, LayerCode, NodeKindContainer, "file") {
			continue
		}
		depth := -1
		var deepest []string
		for _, c := range x.nodes {
			if !x.is(c.Id, LayerDesign, NodeKindContainer) ||
				!slices.ContainsFunc(c.Attrs["paths"], func(p string) bool { return matches(p, file.Attrs["path"][0]) }) {
				continue
			}
			switch d := len(x.ancestors(c.Id)); {
			case d > depth:
				depth, deepest = d, []string{c.Id}
			case d == depth:
				deepest = append(deepest, c.Id)
			}
		}
		for _, c := range deepest {
			x.derive(EdgeKindOwns, c, file.Id, nil)
		}
	}
	for _, c := range x.nodes {
		for _, p := range c.Attrs["paths"] {
			if strings.Contains(p, "[") {
				x.diags = append(x.diags, "graph.bad-path-pattern "+c.Id)
			}
		}
	}
}

// methodRealizes: c realizes t, o is an operation of t, and m a method of c named o.
func (x *expectation) methodRealizes() {
	for _, r := range x.edges {
		if r.Kind != EdgeKindRealizes || !x.is(r.From, LayerCode, NodeKindType) || !x.is(r.To, LayerDesign, NodeKindType) {
			continue
		}
		for _, o := range x.nodes {
			for _, m := range x.nodes {
				if x.is(o.Id, LayerDesign, NodeKindOp) && x.parent[o.Id] == r.To && x.is(m.Id, LayerCode, NodeKindOp) &&
					x.parent[m.Id] == r.From && o.Id == r.To+"."+m.Attrs["name"][0] && !x.has(EdgeKindRealizes, m.Id, o.Id) {
					x.derive(EdgeKindRealizes, m.Id, o.Id, nil)
					x.realizes[edgeKey{EdgeKindRealizes, m.Id, o.Id}] = true
				}
			}
		}
	}
}

// salience: design nodes; code types and functions that realize, check or
// generate; the files and packages that hold those.
func (x *expectation) salience() {
	for _, n := range x.nodes {
		binds := false
		for k := range x.realizes {
			binds = binds || k.from == n.Id
		}
		for k := range x.kept {
			binds = binds || (k.kind == EdgeKindChecks || k.kind == EdgeKindGenerates || k.kind == EdgeKindObserves) && k.from == n.Id
		}
		switch {
		case n.Layer == LayerDesign:
			x.salient[n.Id] = true
		case n.Layer == LayerCode && (n.Kind == NodeKindType || n.Kind == NodeKindOp) && binds:
			file := "file:" + n.Origin.Path
			if !x.is(file, LayerCode, NodeKindContainer, "file") {
				file = x.fileOf(n.Id)
			}
			if slices.Contains(x.byID[file].Attrs["generated"], "true") {
				continue
			}
			x.salient[n.Id] = true
			for _, a := range append(x.ancestors(n.Id), file) {
				if x.is(a, LayerCode, NodeKindContainer) && (!x.is(a, LayerCode, NodeKindContainer, "file") || !slices.Contains(x.byID[a].Attrs["generated"], "true")) {
					x.salient[a] = true
				}
			}
		}
	}
}

// clientOf tells whether the function h realizes an operation of the external system ext.
func (x *expectation) clientOf(h, ext string) bool {
	for k := range x.realizes {
		if k.from != h || !x.is(k.to, LayerDesign, NodeKindOp) {
			continue
		}
		for _, a := range x.ancestors(k.to) {
			if x.is(a, LayerDesign, NodeKindContainer) {
				break
			}
			if a == ext {
				return true
			}
		}
	}
	return false
}

// depends: for every container A and every other container or external
// system B, the imports and calls by which A's code uses B's.
func (x *expectation) depends() {
	for _, a := range x.nodes {
		for _, b := range x.nodes {
			external := x.is(b.Id, LayerDesign, NodeKindActor, "external")
			if !x.is(a.Id, LayerDesign, NodeKindContainer) || a.Id == b.Id || !x.is(b.Id, LayerDesign, NodeKindContainer) && !external {
				continue
			}
			attrs := map[string][]string{}
			for _, e := range x.edges {
				switch {
				case e.Kind == EdgeKindImports && slices.Contains(x.owners(e.From), a.Id):
					for _, file := range x.nodes {
						if (file.Id == e.To || x.parent[file.Id] == e.To) && x.is(file.Id, LayerCode, NodeKindContainer, "file") &&
							slices.Contains(x.owners(file.Id), b.Id) && !slices.Contains(attrs["imports"], e.From+" "+e.To) {
							attrs["imports"] = append(attrs["imports"], e.From+" "+e.To)
						}
					}
				case e.Kind == EdgeKindCalls && x.calls(e.From, e.To) && slices.Contains(x.owners(e.From), a.Id) &&
					(slices.Contains(x.owners(e.To), b.Id) || external && x.clientOf(e.To, b.Id)):
					attrs["calls"] = append(attrs["calls"], e.From+" "+e.To)
				}
			}
			if len(attrs) > 0 {
				for k := range attrs {
					slices.Sort(attrs[k])
				}
				x.derive(EdgeKindDepends, a.Id, b.Id, attrs)
			}
		}
	}
}

// traces: a call or send step to each function that realizes its operation.
func (x *expectation) traces() {
	for _, s := range x.nodes {
		for _, fn := range x.functions {
			for k := range x.realizes {
				if x.is(s.Id, LayerDesign, NodeKindStep, "call", "send") && k.from == fn && x.has(EdgeKindCalls, s.Id, k.to) {
					x.derive(EdgeKindTraces, s.Id, fn, nil)
				}
			}
		}
	}
}

// closure is the transitive closure of calls among the given functions.
func (x *expectation) closure(among []string) map[[2]string]bool {
	c := map[[2]string]bool{}
	for _, a := range among {
		for _, b := range among {
			c[[2]string{a, b}] = x.calls(a, b)
		}
	}
	for _, k := range among {
		for _, a := range among {
			for _, b := range among {
				c[[2]string{a, b}] = c[[2]string{a, b}] || c[[2]string{a, k}] && c[[2]string{k, b}]
			}
		}
	}
	return c
}

// exercises: a predicate to every function in the closure of its calls.
func (x *expectation) exercises() {
	all := x.closure(x.functions)
	for _, p := range x.functions {
		checks := false
		for k := range x.kept {
			checks = checks || k.kind == EdgeKindChecks && k.from == p
		}
		for _, g := range x.functions {
			if checks && all[[2]string{p, g}] {
				x.derive(EdgeKindExercises, p, g, nil)
			}
		}
	}
}

// reaches: d is on a chain f -> d1 -> ... -> d -> ... -> dk -> g of details
// between two salient functions that do not call each other directly.
func (x *expectation) reaches() {
	var details []string
	for _, fn := range x.functions {
		if !x.salient[fn] {
			details = append(details, fn)
		}
	}
	within := x.closure(details)
	along := func(a, b string) bool { return a == b || within[[2]string{a, b}] }
	for _, fn := range x.functions {
		for _, g := range x.functions {
			if !x.salient[fn] || !x.salient[g] || fn == g || x.calls(fn, g) {
				continue
			}
			var via []string
			for _, d := range details {
				from := slices.ContainsFunc(details, func(d1 string) bool { return x.calls(fn, d1) && along(d1, d) })
				to := slices.ContainsFunc(details, func(dk string) bool { return along(d, dk) && x.calls(dk, g) })
				if from && to {
					via = append(via, d)
				}
			}
			if len(via) > 0 {
				slices.Sort(via)
				x.derive(EdgeKindReaches, fn, g, map[string][]string{"via": via})
			}
		}
	}
}

// matches is a path pattern's meaning for the forms a derivation sample uses.
func matches(pattern, p string) bool {
	switch dir, _, _ := strings.Cut(pattern, "/*"); {
	case pattern == "**":
		return true
	case strings.HasSuffix(pattern, "/**"):
		return p == dir || strings.HasPrefix(p, dir+"/")
	case strings.HasSuffix(pattern, "/*.go"):
		return path.Dir(p) == dir && strings.HasSuffix(p, ".go")
	}
	return pattern == p
}

func describeAttrs(attrs map[string][]string) string {
	var parts []string
	for _, k := range slices.Sorted(maps.Keys(attrs)) {
		parts = append(parts, k+"="+strings.Join(attrs[k], ","))
	}
	return "{" + strings.Join(parts, " ") + "}"
}

//uc:checks graph.derive-matches-definition
func checkDeriveMatchesDefinition(t rapid.TB, fragment Fragment) {
	g := NewGraph()
	if err := g.AddFragment(fragment); err != nil {
		t.Fatalf("AddFragment refused a fragment whose nodes are distinct and whose edges start at its nodes: %v", err)
	}
	diags := g.Derive()
	want := definition(fragment)
	got := map[edgeKey]map[string][]string{}
	for _, e := range g.Edges() {
		key := edgeKey{e.Kind, e.From, e.To}
		switch {
		case e.Source == SourceDerived:
			got[key] = e.Attrs
		case !want.kept[key]:
			t.Fatalf("the graph holds the edge %s %s %s, which the fragment does not have or whose target is not a node", e.Kind, e.From, e.To)
		}
	}
	for key := range want.kept {
		if _, ok := g.edges[key]; !ok {
			t.Fatalf("the edge %s %s %s of the fragment, whose ends are nodes, is not in the graph", key.kind, key.from, key.to)
		}
	}
	for key, attrs := range want.derived {
		has, ok := got[key]
		switch {
		case !ok:
			t.Fatalf("Derive did not derive %s %s %s, which model.md §9 derives with attrs %s", key.kind, key.from, key.to, describeAttrs(attrs))
		case !sameAttrs(has, attrs):
			t.Fatalf("Derive gave %s %s %s the attrs %s; model.md §9 gives %s", key.kind, key.from, key.to, describeAttrs(has), describeAttrs(attrs))
		}
	}
	for key := range got {
		if _, ok := want.derived[key]; !ok {
			t.Fatalf("Derive derived %s %s %s, which model.md §9 does not derive", key.kind, key.from, key.to)
		}
	}
	for _, n := range g.Nodes() {
		if n.Salient != want.salient[n.Id] {
			t.Fatalf("Derive made %s salient=%v; model.md §9 makes it salient=%v", n.Id, n.Salient, want.salient[n.Id])
		}
	}
	var codes []string
	for _, d := range diags {
		codes = append(codes, d.Code+" "+d.Subject)
		if d.Message == "" || d.Fix == "" || d.Position != want.origins[d.Subject] {
			t.Fatalf("the diagnostic %+v lacks its message or fix, or is not at the origin %v of its subject", d, want.origins[d.Subject])
		}
	}
	slices.Sort(codes)
	if !slices.Equal(codes, want.diags) {
		t.Fatalf("Derive reported %q; a binding to no node and an invalid path pattern each give one diagnostic: %q", codes, want.diags)
	}
}

func TestDeriveMatchesDefinition(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) { checkDeriveMatchesDefinition(t, genDeriveFragment().Draw(t, "fragment")) })
}

func FuzzDeriveMatchesDefinition(f *testing.F) {
	f.Fuzz(rapid.MakeFuzz(func(t *rapid.T) { checkDeriveMatchesDefinition(t, genDeriveFragment().Draw(t, "fragment")) }))
}
