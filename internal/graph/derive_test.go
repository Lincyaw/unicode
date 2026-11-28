package graph

import (
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/diag"
)

// sampler builds a fragment node by node, giving every node and edge its own
// line, and every edge a new identity.
type sampler struct {
	t    *rapid.T
	f    Fragment
	keys map[edgeKey]bool
	line int
	// What the sample is built to be: each component's directory, the
	// forms of its paths patterns and its depth, the components with an
	// invalid pattern, the components each file was drawn to match, each
	// code element's container, the external system of each operation
	// declared under it, and the chains of detail functions drawn.
	dirs        map[string]string
	up          map[string]string
	forms       map[string][]string
	depth       map[string]int
	bad         []string
	matched     map[string][]string
	parent      map[string]string
	externalOps map[string]string
	chains      []Chain
}

func newSampler(t *rapid.T) *sampler {
	return &sampler{t: t, keys: map[edgeKey]bool{}, dirs: map[string]string{}, up: map[string]string{}, forms: map[string][]string{},
		depth: map[string]int{"sys": 0}, matched: map[string][]string{}, parent: map[string]string{}, externalOps: map[string]string{}}
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

// sampleDesign is what the code part of a sample binds to.
type sampleDesign struct {
	types, ops, others, properties, observations []string
}

func (s *sampler) design() sampleDesign {
	var d sampleDesign
	containers := []string{s.node("sys", LayerDesign, NodeKindContainer, "system", nil)}
	for i := range rapid.IntRange(0, 4).Draw(s.t, "components") {
		parent := s.pick("parent", containers)
		id, dir := fmt.Sprintf("k%d", i), fmt.Sprintf("d%d", i)
		if parent != "sys" {
			id, dir = parent+"."+id, s.dirs[parent]+"/"+dir
		}
		// Each pattern is drawn as a form whose files are known: every
		// file, the files under the component's directory, the .go files
		// directly in it, its file z.go, or none, for an invalid pattern.
		var paths []string
		for range rapid.IntRange(0, 2).Draw(s.t, "patterns") {
			form := rapid.SampledFrom([]string{"all", "under", "under", "in", "exact", "invalid"}).Draw(s.t, "pattern")
			s.forms[id] = append(s.forms[id], form)
			paths = append(paths, map[string]string{"all": "**", "under": dir + "/**", "in": dir + "/*.go", "exact": dir + "/z.go", "invalid": dir + "/["}[form])
			if form == "invalid" {
				s.bad = append(s.bad, id)
			}
		}
		s.node(id, LayerDesign, NodeKindContainer, "component", map[string][]string{"paths": paths})
		s.contains(parent, id)
		s.dirs[id], s.up[id], s.depth[id] = dir, parent, s.depth[parent]+1
		containers = append(containers, id)
	}
	owners := containers[1:]
	if rapid.Bool().Draw(s.t, "external") {
		s.node("x", LayerDesign, NodeKindActor, "external", nil)
		s.contains("sys", "x")
		s.depth["x"] = 1
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
				if c == "x" {
					s.externalOps[id+"."+name] = c
				}
			}
		}
		for _, name := range s.subset("op", []string{"run", "load"}) {
			d.ops = append(d.ops, s.node(c+"."+name, LayerDesign, NodeKindOp, "op", nil))
			s.contains(c, c+"."+name)
			if c == "x" {
				s.externalOps[c+"."+name] = c
			}
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

// plantFiles draws the files: x.go and y.go in each component's directory,
// z.go where a component names it exactly, and e/x.go outside them all. It
// records, for each file, the components whose pattern forms match it.
func (s *sampler) plantFiles() []string {
	var files []string
	for _, c := range slices.Sorted(maps.Keys(s.dirs)) {
		names := []string{"x.go", "y.go"}
		if slices.Contains(s.forms[c], "exact") {
			names = append(names, "z.go")
		}
		for _, name := range s.subset("file", names) {
			files = append(files, s.dirs[c]+"/"+name)
			s.matched["file:"+s.dirs[c]+"/"+name] = s.matching(c, name)
		}
	}
	if rapid.Bool().Draw(s.t, "outsideFile") {
		files = append(files, "e/x.go")
		s.matched["file:e/x.go"] = s.matching("", "x.go")
	}
	return files
}

// matching lists the components whose drawn forms match the file name
// drawn in the directory of the component owner ("" for none).
func (s *sampler) matching(owner, name string) []string {
	out := []string{}
	for _, c := range slices.Sorted(maps.Keys(s.forms)) {
		if slices.ContainsFunc(s.forms[c], func(form string) bool {
			switch form {
			case "all":
				return true
			case "under":
				return s.within(owner, c)
			case "in":
				return owner == c
			case "exact":
				return owner == c && name == "z.go"
			}
			return false
		}) {
			out = append(out, c)
		}
	}
	return out
}

// within tells whether the component owner is c or nested in it.
func (s *sampler) within(owner, c string) bool {
	for at := owner; at != "" && at != "sys"; at = s.up[at] {
		if at == c {
			return true
		}
	}
	return false
}

func (s *sampler) code(d sampleDesign) {
	files := s.plantFiles()
	packages := map[string]bool{}
	for _, f := range files {
		dir := path.Dir(f)
		if !packages[dir] {
			packages[dir] = true
			s.node("go:m/"+dir, LayerCode, NodeKindContainer, "package", map[string][]string{"path": {"m/" + dir}, "language": {"go"}})
		}
		attrs := map[string][]string{"path": {f}, "language": {"go"}}
		if rapid.IntRange(0, 3).Draw(s.t, "generatedFile") == 0 {
			attrs["generated"] = []string{"true"}
		}
		s.node("file:"+f, LayerCode, NodeKindContainer, "file", attrs)
		s.contains("go:m/"+dir, "file:"+f)
		s.parent["file:"+f] = "go:m/" + dir
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
		s.parent[id] = "file:" + f
		for _, name := range s.subset("method", []string{"run", "stop", "other"}) {
			m := s.node(id+"."+strings.ToUpper(name[:1])+name[1:], LayerCode, NodeKindOp, methodForm, map[string][]string{"name": {name}})
			s.contains(id, m)
			s.parent[m] = id
			methods[id] = append(methods[id], m)
			functions = append(functions, m)
			if rapid.IntRange(0, 9).Draw(s.t, "methodBinding") == 0 {
				s.edge(EdgeKindRealizes, m, s.pick("methodTarget", d.ops), SourceDeclared, nil)
			}
		}
		switch {
		case rapid.IntRange(0, 3).Draw(s.t, "typeBinding") > 0:
			target := s.pick("typeTarget", slices.Concat(d.types, d.types, d.others, missing))
			s.edge(EdgeKindRealizes, id, target, SourceDeclared, nil)
			s.candidates(methods[id], target)
		case len(d.types) > 0 && rapid.Bool().Draw(s.t, "strayCandidates"):
			// Candidates of a type that realizes nothing: none of them holds.
			s.candidates(methods[id], s.pick("strayTarget", d.types))
		}
	}
	for i := range rapid.IntRange(0, 6).Draw(s.t, "funcs") {
		f := s.pick("funcFile", files)
		id := s.node(fmt.Sprintf("go:m/%s.F%d", path.Dir(f), i), LayerCode, NodeKindOp, "func",
			map[string][]string{"name": {rapid.SampledFrom([]string{"run", "load", "check"}).Draw(s.t, "funcName")}})
		s.contains("file:"+f, id)
		s.parent[id] = "file:" + f
		functions = append(functions, id)
		switch rapid.IntRange(0, 5).Draw(s.t, "funcBinding") {
		case 0:
			s.edge(EdgeKindRealizes, id, s.pick("funcTarget", slices.Concat(d.ops, d.ops, d.types, missing)), SourceDeclared, nil)
		case 1:
			s.edge(EdgeKindChecks, id, s.pick("checksTarget", slices.Concat(d.properties, d.properties, d.types, missing)), SourceDeclared, nil)
		case 2:
			s.edge(EdgeKindGenerates, id, s.pick("generatesTarget", slices.Concat(d.properties, missing)), SourceDeclared, map[string][]string{"symbols": {"v"}})
		case 3:
			s.edge(EdgeKindObserves, id, s.pick("observesTarget", slices.Concat(d.observations, missing)), SourceDeclared, nil)
		case 4:
			s.edge(EdgeKindPublishes, id, s.pick("publishesTarget", slices.Concat(d.types, missing)), SourceDeclared, nil)
		}
	}
	for range rapid.IntRange(0, 16).Draw(s.t, "calls") {
		s.call(s.pick("caller", functions), s.pick("callee", functions))
	}
	if ops := slices.Sorted(maps.Keys(s.externalOps)); len(ops) > 0 && len(functions) > 0 && rapid.Bool().Draw(s.t, "client") {
		// A client of the external system, and a call to it.
		client := s.function(files, "X")
		s.edge(EdgeKindRealizes, client, s.pick("clientOf", ops), SourceDeclared, nil)
		s.call(s.pick("clientCaller", functions), client)
		functions = append(functions, client)
	}
	if len(functions) > 1 && rapid.IntRange(0, 3).Draw(s.t, "chain") > 0 {
		// A chain from one function to another through one or two new
		// functions without bindings, and now and then a direct call too.
		ends := functions
		if bound := s.bound(functions); len(bound) > 1 {
			ends = bound
		}
		from, to := s.pick("chainFrom", ends), s.pick("chainTo", ends)
		chain := []string{from, s.function(files, "D1")}
		if rapid.Bool().Draw(s.t, "longChain") {
			chain = append(chain, s.function(files, "D2"))
		}
		chain = append(chain, to)
		for i := 1; i < len(chain); i++ {
			s.call(chain[i-1], chain[i])
		}
		s.chains = append(s.chains, Chain{From: from, To: to, Via: slices.Clone(chain[1 : len(chain)-1])})
		if rapid.Bool().Draw(s.t, "shortcut") {
			s.call(from, to)
		}
	}
	if len(d.properties) > 0 && rapid.Bool().Draw(s.t, "witness") {
		s.witnessGadget(files, s.pick("witnessCheck", d.properties))
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

// witnessGadget declares, in one file, a predicate W of check and functions
// it reaches by paths of known length: We by a path of two through Wa and Wb
// and by a shorter one through Wd (Wa sorts before Wd), Wf through Wa or
// through Wc at the same distance (a tie Wa breaks), and Wg only through Wa
// and Wb.
func (s *sampler) witnessGadget(files []string, check string) {
	f := s.pick("witnessFile", files)
	fn := func(name string) string {
		id := s.node(fmt.Sprintf("go:m/%s.%s", path.Dir(f), name), LayerCode, NodeKindOp, "func", map[string][]string{"name": {strings.ToLower(name)}})
		s.contains("file:"+f, id)
		s.parent[id] = "file:" + f
		return id
	}
	w := fn("W")
	s.edge(EdgeKindChecks, w, check, SourceDeclared, nil)
	ids := map[string]string{}
	for _, name := range []string{"Wa", "Wb", "Wc", "Wd", "We", "Wf", "Wg"} {
		ids[name] = fn(name)
	}
	ids["W"] = w
	for _, c := range [][2]string{{"W", "Wa"}, {"W", "Wc"}, {"W", "Wd"}, {"Wa", "Wb"}, {"Wb", "Wg"}, {"Wb", "We"}, {"Wd", "We"}, {"Wa", "Wf"}, {"Wc", "Wf"}} {
		s.call(ids[c[0]], ids[c[1]])
	}
}

// candidates gives each method the candidate edge of a backend to the
// operation of target that has its name.
func (s *sampler) candidates(methods []string, target string) {
	for _, m := range methods {
		s.edge(EdgeKindRealizes, m, target+"."+s.nodeAttr(m, "name"), SourceDerived, nil)
	}
}

// function declares a function without bindings named name in one of the files.
func (s *sampler) function(files []string, name string) string {
	f := s.pick(name+"File", files)
	id := s.node(fmt.Sprintf("go:m/%s.%s", path.Dir(f), name), LayerCode, NodeKindOp, "func", map[string][]string{"name": {strings.ToLower(name)}})
	s.contains("file:"+f, id)
	s.parent[id] = "file:" + f
	return id
}

// bound lists the functions that carry a binding to a node of the sample.
func (s *sampler) bound(functions []string) []string {
	ids := map[string]bool{}
	for _, n := range s.f.Nodes {
		ids[n.Id] = true
	}
	var out []string
	for _, e := range s.f.Edges {
		if e.Kind != EdgeKindCalls && e.Kind != EdgeKindContains && e.Source != SourceDerived && ids[e.To] && slices.Contains(functions, e.From) && !slices.Contains(out, e.From) {
			out = append(out, e.From)
		}
	}
	return out
}

func (s *sampler) call(from, to string) {
	if from != "" {
		s.edge(EdgeKindCalls, from, to, SourceExtracted, map[string][]string{"sites": {"code.go:1:1"}})
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

// Usage is a call between two code functions, or an import, of a
// derivation sample, with the value that lists it in a depends edge's attrs.
//
//uc:realizes graph.Usage
type Usage struct {
	Kind     EdgeKind
	From, To string
	Label    string
}

// DeriveSample is a fragment and the facts it was drawn to have.
//
//uc:realizes graph.DeriveSample
type DeriveSample struct {
	Fragment    Fragment
	Matches     map[string][]string
	Depth       map[string]int
	BadPatterns []string
	Dangling    map[string]diag.Position
	Files       map[string]string
	SourceFiles map[string]string
	Ancestors   map[string][]string
	ExternalOps map[string]string
	Usages      []Usage
	Chains      []Chain
}

// Chain is a chain of calls drawn into a derivation sample: from a function
// through new functions without bindings to another function.
//
//uc:realizes graph.Chain
type Chain struct {
	From, To string
	Via      []string
}

// genDeriveSample draws a small design and code layer: nested components
// with path patterns (some invalid), an external system, types, operations,
// flows with steps and properties; files, types, methods, functions, calls,
// imports, and bindings, some to nodes that do not exist or are of the wrong
// kind, with the candidates a backend gives for methods. With it come the
// facts the drawing fixed: which containers' patterns match each file by the
// form drawn, the depths, the invalid patterns, the bindings to identities
// of no node, each code element's file, source file and containers, the
// external system's operations, and the calls and imports.
//
//uc:generates graph.derive-keeps-resolved-edges.sample
//uc:generates graph.derive-reports-unresolved-bindings.sample
//uc:generates graph.derive-reports-invalid-patterns.sample
//uc:generates graph.derive-diagnostics-are-described.sample
//uc:generates graph.owns-follows-deepest-pattern.sample
//uc:generates graph.method-realizes-follows-candidates.sample
//uc:generates graph.depends-lists-calls.sample
//uc:generates graph.depends-lists-imports.sample
//uc:generates graph.depends-attrs-are-distinct-usages.sample
//uc:generates graph.depends-values-are-usages.sample
//uc:generates graph.traces-follow-realizations.sample
//uc:generates graph.exercises-close-over-calls.sample
//uc:generates graph.exercises-paths-lead-to-predicates.sample
//uc:generates graph.design-nodes-are-salient.sample
//uc:generates graph.code-salience-follows-bindings.sample
//uc:generates graph.container-salience-follows-elements.sample
//uc:generates graph.reaches-joins-salient-functions.sample
//uc:generates graph.reaches-covers-detail-chains.sample
//uc:generates graph.derive-ignores-order.sample
//uc:generates graph.derive-adds-only-defined-edges.sample
//uc:generates graph.exercises-paths-are-shortest.sample
//uc:generates graph.exercises-paths-are-ordered.sample
//uc:generates graph.reaches-covers-planted-chains.sample
func genDeriveSample() *rapid.Generator[DeriveSample] {
	return rapid.Custom(func(t *rapid.T) DeriveSample {
		s := newSampler(t)
		s.code(s.design())
		return s.sample()
	})
}

// sample gives the fragment with the facts it was drawn to have.
func (s *sampler) sample() DeriveSample {
	out := DeriveSample{Fragment: s.f, Matches: s.matched, Depth: s.depth, BadPatterns: s.bad, Dangling: map[string]diag.Position{},
		Files: map[string]string{}, SourceFiles: map[string]string{}, Ancestors: map[string][]string{}, ExternalOps: s.externalOps, Chains: s.chains}
	nodes := map[string]Node{}
	for _, n := range s.f.Nodes {
		nodes[n.Id] = n
	}
	for _, n := range s.f.Nodes {
		if n.Layer == LayerCode && (n.Kind == NodeKindType || n.Kind == NodeKindOp) {
			for a := s.parent[n.Id]; a != ""; a = s.parent[a] {
				out.Ancestors[n.Id] = append(out.Ancestors[n.Id], a)
				if nodes[a].Form == "file" && out.Files[n.Id] == "" {
					out.Files[n.Id] = a
				}
			}
			out.SourceFiles[n.Id] = out.Files[n.Id]
			if _, ok := nodes["file:"+n.Origin.Path]; ok {
				out.SourceFiles[n.Id] = "file:" + n.Origin.Path
			}
		}
	}
	for _, e := range s.f.Edges {
		_, resolved := nodes[e.To]
		switch {
		case e.Kind == EdgeKindRealizes && e.Source == SourceDerived:
		case !resolved:
			out.Dangling[string(e.Kind)+" "+e.From+" "+e.To] = e.Origin
		case e.Kind == EdgeKindImports || e.Kind == EdgeKindCalls && nodes[e.From].Layer == LayerCode && nodes[e.To].Layer == LayerCode:
			out.Usages = append(out.Usages, Usage{Kind: e.Kind, From: e.From, To: e.To, Label: e.From + " " + e.To})
		}
	}
	return out
}

// graphOf adds the nodes, then the edges, in the order given, to a new
// graph, each by AddNode and AddEdge.
func graphOf(nodes []Node, edges []Edge) (*Graph, error) {
	g := NewGraph()
	for _, n := range nodes {
		if err := g.AddNode(n); err != nil {
			return nil, err
		}
	}
	for _, e := range edges {
		if err := g.AddEdge(e); err != nil {
			return nil, err
		}
	}
	return &g, nil
}

// fragmentGraph adds the nodes and edges, in the order given, to a new
// graph by AddFragment, which keeps bindings and candidates for Derive.
func fragmentGraph(nodes []Node, edges []Edge) (*Graph, error) {
	g := NewGraph()
	if err := g.AddFragment(Fragment{Nodes: nodes, Edges: edges}); err != nil {
		return nil, err
	}
	return &g, nil
}

//uc:generates graph.derive-keeps-resolved-edges.snapshot
//uc:generates graph.derive-reports-unresolved-bindings.snapshot
//uc:generates graph.derive-reports-invalid-patterns.snapshot
//uc:generates graph.derive-diagnostics-are-described.snapshot
//uc:generates graph.owns-follows-deepest-pattern.snapshot
//uc:generates graph.method-realizes-follows-candidates.snapshot
//uc:generates graph.depends-lists-calls.snapshot
//uc:generates graph.depends-lists-imports.snapshot
//uc:generates graph.depends-attrs-are-distinct-usages.snapshot
//uc:generates graph.depends-values-are-usages.snapshot
//uc:generates graph.traces-follow-realizations.snapshot
//uc:generates graph.exercises-close-over-calls.snapshot
//uc:generates graph.exercises-paths-lead-to-predicates.snapshot
//uc:generates graph.design-nodes-are-salient.snapshot
//uc:generates graph.code-salience-follows-bindings.snapshot
//uc:generates graph.container-salience-follows-elements.snapshot
//uc:generates graph.reaches-joins-salient-functions.snapshot
//uc:generates graph.derive-ignores-order.snapshot
//uc:generates graph.derive-adds-only-defined-edges.snapshot
//uc:generates graph.exercises-paths-are-shortest.snapshot
//uc:generates graph.reaches-covers-planted-chains.snapshot
func deriveUnderived(sample DeriveSample) (*Graph, error) {
	return fragmentGraph(sample.Fragment.Nodes, sample.Fragment.Edges)
}

//uc:generates graph.derive-ignores-order.reordered
func deriveReordered(sample DeriveSample, rotation int) (*Graph, error) {
	g, err := fragmentGraph(rotate(sample.Fragment.Nodes, rotation), rotate(sample.Fragment.Edges, rotation))
	if err != nil {
		return nil, err
	}
	g.Derive()
	return g, nil
}

// reachesByDefinition is what docs/model.md §9 D7 derives from the calls
// between the code functions of g and their salience: for salient f and g
// that are distinct and do not call each other directly, the detail
// functions on some chain f -> d1 -> ... -> dk -> g of details, sorted.
func reachesByDefinition(g *Graph) map[edgeKey][]string {
	var functions, details []string
	for _, n := range g.Nodes() {
		if n.Layer == LayerCode && n.Kind == NodeKindOp {
			functions = append(functions, n.Id)
			if !n.Salient {
				details = append(details, n.Id)
			}
		}
	}
	calls := func(a, b string) bool { return g.Edge(EdgeKindCalls, a, b) != nil }
	// within is the transitive closure of calls among the details.
	within := map[[2]string]bool{}
	for _, a := range details {
		for _, b := range details {
			within[[2]string{a, b}] = a == b || calls(a, b)
		}
	}
	for _, k := range details {
		for _, a := range details {
			for _, b := range details {
				within[[2]string{a, b}] = within[[2]string{a, b}] || within[[2]string{a, k}] && within[[2]string{k, b}]
			}
		}
	}
	out := map[edgeKey][]string{}
	for _, f := range functions {
		for _, to := range functions {
			if !g.Node(f).Salient || !g.Node(to).Salient || f == to || calls(f, to) {
				continue
			}
			var via []string
			for _, d := range details {
				from := slices.ContainsFunc(details, func(d1 string) bool { return calls(f, d1) && within[[2]string{d1, d}] })
				onward := slices.ContainsFunc(details, func(dk string) bool { return within[[2]string{d, dk}] && calls(dk, to) })
				if from && onward {
					via = append(via, d)
				}
			}
			if len(via) > 0 {
				out[edgeKey{EdgeKindReaches, f, to}] = via
			}
		}
	}
	return out
}

//uc:checks graph.reaches-covers-detail-chains
func checkReachesCoversDetailChains(t rapid.TB, sample DeriveSample) {
	g, err := fragmentGraph(sample.Fragment.Nodes, sample.Fragment.Edges)
	if err != nil {
		t.Fatalf("AddFragment refused a fragment whose nodes are distinct and whose edges start at its nodes: %v", err)
	}
	g.Derive()
	want := reachesByDefinition(g)
	for _, e := range g.Edges() {
		if e.Kind != EdgeKindReaches {
			continue
		}
		key := edgeKey{e.Kind, e.From, e.To}
		via, ok := want[key]
		if !ok {
			t.Fatalf("Derive joined %s to %s by reaches, but no chain of detail functions leads from one to the other", e.From, e.To)
		}
		if got := e.Attrs["via"]; !slices.Equal(got, via) {
			t.Fatalf("the reaches edge from %s to %s lists via %q, but the detail functions on its chains, in identity order, are %q", e.From, e.To, got, via)
		}
		delete(want, key)
	}
	for key, via := range want {
		t.Fatalf("Derive did not join %s to %s by reaches, though the chains through the detail functions %q lead from one to the other", key.from, key.to, via)
	}
}

//uc:checks graph.exercises-paths-are-ordered
func checkExercisesPathsAreOrdered(t rapid.TB, sample DeriveSample) {
	g, err := fragmentGraph(sample.Fragment.Nodes, sample.Fragment.Edges)
	if err != nil {
		t.Fatalf("AddFragment refused a fragment whose nodes are distinct and whose edges start at its nodes: %v", err)
	}
	g.Derive()
	for _, x := range g.Edges() {
		path := x.Attrs["path"]
		if x.Kind != EdgeKindExercises || len(path) == 0 {
			continue
		}
		last := path[len(path)-1]
		if g.Edge(EdgeKindCalls, last, x.To) == nil {
			t.Fatalf("the path %q of exercises(%s, %s) ends in %s, which does not call %s", path, x.From, x.To, last, x.To)
		}
		before := g.Edge(EdgeKindExercises, x.From, last)
		if before == nil || !slices.Equal(before.Attrs["path"], path[:len(path)-1]) {
			t.Fatalf("the path %q of exercises(%s, %s) does not continue the path of exercises(%s, %s): %+v", path, x.From, x.To, x.From, last, before)
		}
		for _, c := range g.Edges() {
			caller := g.Edge(EdgeKindExercises, x.From, c.From)
			if c.Kind == EdgeKindCalls && c.To == x.To && c.From < last && caller != nil && len(caller.Attrs["path"]) == len(path)-1 {
				t.Fatalf("the path %q of exercises(%s, %s) ends in %s, but %s, as near to %s and with a smaller identity, also calls %s", path, x.From, x.To, last, c.From, x.From, x.To)
			}
		}
	}
}
