package golang

import (
	"cmp"
	"fmt"
	"go/token"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
)

// This file computes from the model of a module the nodes and edges that
// extraction should give.

// expectation collects the nodes and edges the model should extract to.
type expectation struct {
	w        *world
	nodes    map[string]graph.Node
	edges    map[edgeKey]graph.Edge
	sites    map[edgeKey][]diag.Position
	realized map[*gtype][]string
}

func (w *world) module() Module {
	files := maps.Clone(rapidFiles)
	files["go.mod"] = goMod
	x := &expectation{
		w: w, nodes: map[string]graph.Node{}, edges: map[edgeKey]graph.Edge{}, sites: map[edgeKey][]diag.Position{},
		realized: map[*gtype][]string{},
	}
	for _, ty := range w.types {
		for _, b := range bindingsOf(ty.doc, true) {
			x.realized[ty] = append(x.realized[ty], b.target)
		}
	}
	for _, p := range w.packages {
		written := map[*gfile]*writer{}
		for _, f := range p.files {
			written[f] = write(f)
			files[f.path] = written[f].b.String()
		}
		x.addPackage(p, written)
	}
	x.addImplements()
	m := Module{Files: files}
	for _, n := range x.nodes {
		m.Nodes = append(m.Nodes, describeNode(n))
	}
	for key, e := range x.edges {
		if sites := x.sites[key]; len(sites) > 0 {
			slices.SortFunc(sites, func(a, b diag.Position) int {
				return cmp.Or(cmp.Compare(a.Path, b.Path), cmp.Compare(a.Line, b.Line), cmp.Compare(a.Column, b.Column))
			})
			e.Attrs = map[string][]string{"sites": {}}
			for _, s := range sites {
				e.Attrs["sites"] = append(e.Attrs["sites"], fmt.Sprintf("%s:%d:%d", s.Path, s.Line, s.Column))
			}
		}
		m.Edges = append(m.Edges, describeEdge(e))
	}
	slices.Sort(m.Nodes)
	slices.Sort(m.Edges)
	return m
}

func (x *expectation) addPackage(p *gpkg, positions map[*gfile]*writer) {
	files := slices.SortedFunc(slices.Values(p.files), func(a, b *gfile) int { return cmp.Compare(a.path, b.path) })
	written := files[0]
	for _, f := range files {
		if len(f.doc) > 0 {
			written = f
			break
		}
	}
	pkgID := "go:" + p.path
	x.node(pkgID, graph.NodeKindContainer, "package", positions[written].at[written], summaryOf(written.doc),
		map[string][]string{"language": {"go"}, "path": {p.path}})
	for _, f := range files {
		at := positions[f].at
		fileID := "file:" + f.path
		x.node(fileID, graph.NodeKindContainer, "file", at[f], "", map[string][]string{"language": {"go"}, "path": {f.path}})
		x.edge(graph.EdgeKindContains, pkgID, fileID, graph.SourceExtracted)
		for _, imported := range x.w.packages {
			if positions[f].imports[imported.path] {
				x.edge(graph.EdgeKindImports, fileID, "go:"+imported.path, graph.SourceExtracted)
			}
		}
		for _, decl := range f.decls {
			switch decl := decl.(type) {
			case *gtype:
				x.addType(decl, fileID, at)
			case *ggroup:
				for _, ty := range decl.types {
					x.addType(ty, fileID, at)
				}
			case *gfunc:
				x.addFunc(decl, fileID, at)
			}
		}
	}
}

func typeIdentity(ty *gtype) string { return "go:" + ty.file.pkg.path + "." + ty.name }

func funcIdentity(fn *gfunc) string {
	if fn.recv != nil {
		return typeIdentity(fn.recv) + "." + fn.sig.name
	}
	return "go:" + fn.file.pkg.path + "." + fn.sig.name
}

// summaryOf is the sentence a doc comment starts with, when it has one.
func summaryOf(doc []string) string {
	if len(doc) == 0 {
		return ""
	}
	return strings.TrimPrefix(doc[0], "// ")
}

func (x *expectation) addType(ty *gtype, fileID string, at map[any]diag.Position) {
	id := typeIdentity(ty)
	attrs := map[string][]string{"name": {ty.name}}
	switch ty.form {
	case "struct":
		attrs["fields"] = []string{}
		for _, field := range ty.fields {
			if token.IsExported(field.name) {
				attrs["fields"] = append(attrs["fields"], lowerFirst(field.name)+": "+x.design(field.typ))
			}
			x.refers(id, field.typ)
		}
		if ty.embed != nil {
			x.edge(graph.EdgeKindExtends, id, typeIdentity(ty.embed), graph.SourceExtracted)
		}
	case "interface":
		attrs["methods"] = []string{}
		for _, sig := range slices.SortedFunc(maps.Values(ifaceMethods(ty)), func(a, b *gsig) int { return cmp.Compare(a.name, b.name) }) {
			if token.IsExported(sig.name) {
				attrs["methods"] = append(attrs["methods"], lowerFirst(sig.name)+" "+x.signature(sig))
			}
		}
		for _, sig := range ty.sigs {
			x.refersSig(id, sig)
		}
		for _, e := range ty.embeds {
			x.edge(graph.EdgeKindExtends, id, typeIdentity(e), graph.SourceExtracted)
		}
	default:
		x.refers(id, ty.under)
	}
	x.node(id, graph.NodeKindType, ty.form, at[ty], summaryOf(docLines(ty.doc)), attrs)
	x.edge(graph.EdgeKindContains, fileID, id, graph.SourceExtracted)
	x.bindings(id, ty.doc, true)
}

func (x *expectation) addFunc(fn *gfunc, fileID string, at map[any]diag.Position) {
	id := funcIdentity(fn)
	form, parent := "func", fileID
	if fn.recv != nil {
		form, parent = "method", typeIdentity(fn.recv)
	}
	testKind := "none"
	if fn.file.test && fn.recv == nil {
		switch {
		case fn.testName == "Fuzz":
			testKind = "fuzz"
		case fn.testName == "Test" && callsRapid(fn.body):
			testKind = "property"
		case fn.testName == "Test":
			testKind = "example"
		}
	}
	x.node(id, graph.NodeKindOp, form, at[fn], summaryOf(docLines(fn.doc)), map[string][]string{
		"name":      {lowerFirst(fn.sig.name)},
		"signature": {x.signature(fn.sig)},
		"testKind":  {testKind},
		"stub":      {strconv.FormatBool(fn.terminal == marker && !slices.ContainsFunc(fn.body, isStatement))},
	})
	x.edge(graph.EdgeKindContains, parent, id, graph.SourceExtracted)
	x.refersSig(id, fn.sig)
	x.bindings(id, fn.doc, false)
	for _, site := range calls(fn, at) {
		key := x.edge(graph.EdgeKindCalls, id, funcIdentity(site.target), graph.SourceExtracted)
		x.sites[key] = append(x.sites[key], site.at)
	}
	if fn.recv != nil && token.IsExported(fn.sig.name) {
		for _, target := range x.realized[fn.recv] {
			x.edge(graph.EdgeKindRealizes, id, target+"."+lowerFirst(fn.sig.name), graph.SourceDerived)
		}
	}
}

// isStatement tells whether a line of a body is a statement; a comment is not,
// so a stub may carry one.
func isStatement(s gstmt) bool { return s.kind != "comment" }

func callsRapid(body []gstmt) bool {
	return slices.ContainsFunc(body, func(s gstmt) bool { return s.kind == "rapid" || s.inner != nil && s.inner.kind == "rapid" })
}

type site struct {
	target *gfunc
	at     diag.Position
}

// calls lists the static calls of a body to functions and methods of the module.
func calls(fn *gfunc, at map[any]diag.Position) []site {
	var out []site
	for i := range fn.body {
		s := &fn.body[i]
		if s.inner != nil {
			s = s.inner
		}
		if s.kind == "call" {
			out = append(out, site{target: s.target, at: at[s]})
		}
	}
	return out
}

func (x *expectation) bindings(id string, doc []binding, isType bool) {
	for _, b := range bindingsOf(doc, isType) {
		switch b.verb {
		case "generates":
			dot := strings.LastIndex(b.target, ".")
			property, symbol := b.target[:dot], b.target[dot+1:]
			key := edgeKey{graph.EdgeKindGenerates, id, property}
			e, ok := x.edges[key]
			if !ok {
				e = graph.Edge{Kind: key.kind, From: id, To: property, Source: graph.SourceDeclared, Attrs: map[string][]string{}}
			}
			e.Attrs["symbols"] = append(e.Attrs["symbols"], symbol)
			x.edges[key] = e
		default:
			x.edge(map[string]graph.EdgeKind{"realizes": graph.EdgeKindRealizes, "checks": graph.EdgeKindChecks}[b.verb],
				id, b.target, graph.SourceDeclared)
		}
	}
}

func (x *expectation) refers(from string, e *gexpr) {
	switch e.kind {
	case "named":
		x.edge(graph.EdgeKindRefers, from, typeIdentity(e.named), graph.SourceExtracted)
	case "pointer", "slice", "map":
		x.refers(from, e.elem)
	}
}

func (x *expectation) refersSig(from string, sig *gsig) {
	for _, p := range sig.params {
		x.refers(from, p.typ)
	}
	for _, r := range sig.results {
		x.refers(from, r)
	}
}

// addImplements expects an implements edge from every type that is no
// interface to every interface with methods whose method set its own holds.
func (x *expectation) addImplements() {
	for _, ty := range x.w.types {
		if ty.form == "interface" {
			continue
		}
		have := methodSet(ty)
		for _, iface := range x.w.types {
			want := ifaceMethods(iface)
			if iface.form != "interface" || len(want) == 0 {
				continue
			}
			holds := true
			for id, sig := range want {
				holds = holds && have[id] != nil && canonSig(have[id]) == canonSig(sig)
			}
			if holds {
				x.edge(graph.EdgeKindImplements, typeIdentity(ty), typeIdentity(iface), graph.SourceExtracted)
			}
		}
	}
}

func (x *expectation) node(id string, kind graph.NodeKind, form string, at diag.Position, summary string, attrs map[string][]string) {
	x.nodes[id] = graph.Node{
		Id: id, Kind: kind, Form: form, Layer: graph.LayerCode, Source: graph.SourceExtracted, Origin: at,
		Summary: summary, Attrs: attrs,
	}
}

// edge expects an edge; a declared one stays when a derived one would replace it.
func (x *expectation) edge(kind graph.EdgeKind, from, to string, source graph.Source) edgeKey {
	key := edgeKey{kind, from, to}
	if _, ok := x.edges[key]; !ok {
		x.edges[key] = graph.Edge{Kind: kind, From: from, To: to, Source: source}
	}
	return key
}

func lowerFirst(name string) string {
	if !token.IsExported(name) {
		return name
	}
	return strings.ToLower(name[:1]) + name[1:]
}

// design writes a type expression in the design's notation (docs/design.md §4).
func (x *expectation) design(e *gexpr) string {
	switch e.kind {
	case "basic":
		return map[string]string{"bool": "bool", "int": "int", "float64": "float", "string": "string", "int64": "int64"}[e.basic]
	case "raw", "error":
		return cmp.Or(e.basic, "error")
	case "named":
		if targets := x.realized[e.named]; len(targets) > 0 {
			return targets[0]
		}
		return typeIdentity(e.named)
	case "ext":
		return "go:" + e.ext[0] + "." + e.ext[2]
	case "time":
		return "time"
	case "duration":
		return "duration"
	case "pointer":
		return "optional<" + x.design(e.elem) + ">"
	case "slice":
		return "list<" + x.design(e.elem) + ">"
	case "bytes":
		return "bytes"
	case "map":
		return "map<" + x.design(&gexpr{kind: "basic", basic: e.basic}) + ", " + x.design(e.elem) + ">"
	default:
		return "set<" + x.design(&gexpr{kind: "basic", basic: e.basic}) + ">"
	}
}

func (x *expectation) signature(sig *gsig) string {
	var params []string
	for _, p := range sig.params {
		params = append(params, cmp.Or(p.name, "_")+": "+x.design(p.typ))
	}
	out := "(" + strings.Join(params, ", ") + ")"
	results := sig.results
	raises := len(results) > 0 && results[len(results)-1].kind == "error"
	if raises {
		results = results[:len(results)-1]
	}
	var written []string
	for _, r := range results {
		written = append(written, x.design(r))
	}
	switch len(written) {
	case 0:
	case 1:
		out += " -> " + written[0]
	default:
		out += " -> (" + strings.Join(written, ", ") + ")"
	}
	if raises {
		out += " raises error"
	}
	return out
}

// canonSig writes a signature's Go types with package paths, so that two
// signatures are identical exactly when they write the same.
func canonSig(sig *gsig) string {
	var params, results []string
	for _, p := range sig.params {
		params = append(params, canon(p.typ))
	}
	for _, r := range sig.results {
		results = append(results, canon(r))
	}
	return "(" + strings.Join(params, ",") + ")(" + strings.Join(results, ",") + ")"
}

func canon(e *gexpr) string {
	switch e.kind {
	case "named":
		return typeIdentity(e.named)
	case "ext":
		return e.ext[0] + "." + e.ext[2]
	case "pointer", "slice", "map":
		return e.kind + "[" + e.basic + "]" + canon(e.elem)
	}
	return e.kind + "[" + e.basic + "]"
}
