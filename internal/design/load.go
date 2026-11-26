package design

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/alecthomas/participle/v2/lexer"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/syntax"
)

// Loaded is the design layer of the graph and the diagnostics of the files
// it was loaded from; with diagnostics, the graph may be incomplete.
//
//uc:realizes design.Loaded
type Loaded struct {
	Graph       graph.Graph
	Diagnostics []diag.Diagnostic
}

const systemPath = "design/system.uc"

var recordedName = regexp.MustCompile(`^r-[0-9a-f]{8}$`)

// Load resolves the references of the design and corpus files, builds the
// nodes and edges of the design layer (docs/model.md §3–§4, §7) and checks
// the constraints that need only the design (docs/design.md §5.1). The files
// are syntax trees from syntax.Parse, each of the kind syntax.KindOf gives its
// path; their order does not matter.
//
//uc:realizes design.load
func Load(files []syntax.File) Loaded {
	l := &loader{byID: map[string]*entity{}, notations: map[string]bool{}, seen: map[edgeKey]bool{}}
	byKind := map[syntax.FileKind][]*syntax.File{}
	sorted := slices.Clone(files)
	slices.SortStableFunc(sorted, func(a, b syntax.File) int { return cmp.Compare(a.Path, b.Path) })
	for i := range sorted {
		byKind[sorted[i].Kind] = append(byKind[sorted[i].Kind], &sorted[i])
	}
	l.system(byKind[syntax.FileKindSystem])
	for _, c := range l.components(byKind[syntax.FileKindComponent]) {
		l.members(c)
	}
	for _, f := range byKind[syntax.FileKindCorpus] {
		l.corpus(f)
	}
	// Each stage may queue work for the next ones.
	for _, queue := range []*[]func(){&l.uses, &l.signatures, &l.domains, &l.checks} {
		for i := 0; i < len(*queue); i++ {
			(*queue)[i]()
		}
	}
	slices.SortFunc(l.diags, func(a, b diag.Diagnostic) int {
		return cmp.Or(cmp.Compare(a.Position.Path, b.Position.Path), cmp.Compare(a.Position.Line, b.Position.Line),
			cmp.Compare(a.Position.Column, b.Position.Column), cmp.Compare(a.Code, b.Code), cmp.Compare(a.Message, b.Message))
	})
	return Loaded{Graph: l.build(), Diagnostics: l.diags}
}

// loader holds the nodes and edges declared so far. Declarations are read in
// a fixed order (the system file, the components from the shallowest, their
// members, the corpus files, each group by path), so that which of two clashing
// declarations is the duplicate does not depend on the order of the files.
// References are resolved once every node is declared, in stages: the uses of
// components, which dependencies are checked against; the signatures of types
// and operations and the symbols' types; the domains of symbols, which read the
// fields of types; and the checks of steps and cases, which read all of it.
type loader struct {
	root       *entity
	byID       map[string]*entity
	entities   []*entity
	notations  map[string]bool
	edges      []graph.Edge
	seen       map[edgeKey]bool
	uses       []func()
	signatures []func()
	domains    []func()
	checks     []func()
	diags      []diag.Diagnostic
}

type edgeKey struct {
	kind     graph.EdgeKind
	from, to string
}

func position(f *syntax.File, pos lexer.Position) diag.Position {
	return diag.Position{Path: f.Path, Line: pos.Line, Column: pos.Column}
}

func (l *loader) report(f *syntax.File, pos lexer.Position, code, subject, message, fix string) {
	l.diags = append(l.diags, diag.Diagnostic{Code: code, Subject: subject, Position: position(f, pos), Message: message, Fix: fix})
}

// declare adds e under parent, which is nil for a component without one.
// Children of the root take their name as identity, others their parent's
// identity, a dot and their name, unless e has its identity already. A second
// node with an identity already declared is a duplicate (docs/model.md §10
// rule 2): it is reported and left out of the graph, and declare returns false.
func (l *loader) declare(parent, e *entity, containsAttrs map[string][]string) bool {
	switch {
	case e.id != "":
	case parent == l.root:
		e.id = e.name
	default:
		e.id = parent.id + "." + e.name
	}
	if prior, ok := l.byID[e.id]; ok {
		where := fmt.Sprintf("%s:%d", prior.file.Path, prior.pos.Line)
		l.report(e.file, e.pos, "design.duplicate-name", e.id,
			fmt.Sprintf("%s is declared twice: %s is declared at %s", e.id, describe(prior), where),
			"rename one of the two, or remove one")
		return false
	}
	if e.source == "" {
		e.source = graph.SourceDeclared
	}
	e.children = map[string]*entity{}
	l.byID[e.id] = e
	l.entities = append(l.entities, e)
	if parent != nil {
		e.parent = parent
		parent.children[e.name] = e
		l.edge(graph.EdgeKindContains, parent, e, e.file, e.pos, e.source, containsAttrs)
	}
	return true
}

// edge records an edge between two nodes, once; an edge to an unresolved
// reference or to a missing system has no end and is left out.
func (l *loader) edge(kind graph.EdgeKind, from, to *entity, f *syntax.File, pos lexer.Position, source graph.Source, attrs map[string][]string) {
	if from == nil || to == nil || from.virtual || to.virtual {
		return
	}
	key := edgeKey{kind, from.id, to.id}
	if l.seen[key] {
		return
	}
	l.seen[key] = true
	l.edges = append(l.edges, graph.Edge{Kind: kind, From: from.id, To: to.id, Source: source, Origin: position(f, pos), Attrs: attrs})
}

func (l *loader) build() graph.Graph {
	g := graph.NewGraph()
	for _, e := range l.entities {
		if e.virtual {
			continue
		}
		node := graph.Node{
			Id: e.id, Kind: e.kind, Form: e.form, Layer: graph.LayerDesign, Source: e.source,
			Origin: position(e.file, e.pos), Summary: e.summary, Attrs: e.attrs,
		}
		if err := g.AddNode(node); err != nil {
			panic("design: declare admits each identity once, yet " + err.Error())
		}
	}
	for _, e := range l.edges {
		if err := g.AddEdge(e); err != nil {
			panic("design: edges join declared nodes once each, yet " + err.Error())
		}
	}
	return g
}

// system declares the system, its notations and its actors. Only design/system.uc
// is of the system kind, so there is at most one system file.
func (l *loader) system(files []*syntax.File) {
	if len(files) == 0 {
		l.root = &entity{virtual: true, children: map[string]*entity{}}
		l.diags = append(l.diags, diag.Diagnostic{
			Code: "design.missing-system-file", Subject: systemPath, Position: diag.Position{Path: systemPath, Line: 1, Column: 1},
			Message: "the design has no system file design/system.uc among the files that parse",
			Fix:     `write design/system.uc with the system declaration, like system name "summary", or fix its syntax errors`,
		})
		return
	}
	f := files[0]
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *syntax.SystemDecl:
			l.root = &entity{id: string(d.Name), name: string(d.Name), kind: graph.NodeKindContainer, form: "system", file: f, pos: d.Pos, summary: string(d.Summary)}
			l.declare(nil, l.root, nil)
		case *syntax.NotationDecl:
			l.notations[d.Name] = true
		case *syntax.PersonDecl:
			l.declare(l.root, &entity{name: string(d.Name), kind: graph.NodeKindActor, form: "person", file: f, pos: d.Pos, summary: string(d.Summary)}, nil)
		case *syntax.ExternalDecl:
			actor := &entity{name: string(d.Name), kind: graph.NodeKindActor, form: "external", file: f, pos: d.Pos, summary: string(d.Summary)}
			if !l.declare(l.root, actor, nil) {
				continue
			}
			for _, line := range d.Lines {
				switch line := line.(type) {
				case *syntax.TypeDecl:
					l.typeDecl(f, actor, actor, line)
				case *syntax.OpDecl:
					l.opDecl(f, actor, actor, line)
				}
			}
		}
	}
}

// component is a component file and the component it declares.
type component struct {
	file   *syntax.File
	decl   *syntax.ComponentDecl
	entity *entity
}

// components declares the component of every component file, parents before
// children, and returns those declared.
func (l *loader) components(files []*syntax.File) []component {
	var all []component
	for _, f := range files {
		for _, d := range f.Decls {
			if d, ok := d.(*syntax.ComponentDecl); ok {
				all = append(all, component{file: f, decl: d})
				break
			}
		}
	}
	slices.SortStableFunc(all, func(a, b component) int {
		return cmp.Compare(len(a.decl.Path.Segments), len(b.decl.Path.Segments))
	})
	var declared []component
	for _, c := range all {
		segs := c.decl.Path.Segments
		id := strings.Join(segs, ".")
		e := &entity{id: id, name: segs[len(segs)-1], kind: graph.NodeKindContainer, form: "component", file: c.file, pos: c.decl.Pos, summary: string(c.decl.Summary)}
		if want := componentID(c.file.Path); want != id {
			l.report(c.file, c.decl.Pos, "design.component-path-mismatch", id,
				fmt.Sprintf("the file %s must declare the component %s, yet it declares %s", c.file.Path, want, id),
				fmt.Sprintf("move the file to %s, or declare the component %s in it", componentPath(id), want))
		}
		parent := l.root
		if len(segs) > 1 {
			parentID := strings.Join(segs[:len(segs)-1], ".")
			parent = l.byID[parentID]
			if !parent.is(graph.NodeKindContainer, "component") {
				parent = nil
				l.report(c.file, c.decl.Pos, "design.missing-parent", id,
					fmt.Sprintf("the component %s is inside %s, which is not a declared component", id, parentID),
					fmt.Sprintf("declare the component %s in %s, or move %s to an existing component", parentID, componentPath(parentID), id))
			}
		}
		if !l.declare(parent, e, nil) {
			continue
		}
		c.entity = e
		l.componentLines(c, parent)
		if len(segs) == 1 && !slices.ContainsFunc(c.decl.Lines, func(line syntax.ComponentLine) bool { _, ok := line.(*syntax.Focus); return ok }) {
			l.report(c.file, c.decl.Pos, "design.missing-focus", id,
				fmt.Sprintf("the top-level component %s has no focus", id),
				"add focus core, focus support or focus utility to its block, after its labels")
		}
		declared = append(declared, c)
	}
	return declared
}

// componentID is the identity of the component that the file at path must declare.
func componentID(path string) string {
	return strings.ReplaceAll(strings.TrimSuffix(strings.TrimPrefix(path, "design/"), ".uc"), "/", ".")
}

func componentPath(id string) string { return "design/" + strings.ReplaceAll(id, ".", "/") + ".uc" }

func corpusPath(id string) string { return "corpus/" + strings.ReplaceAll(id, ".", "/") + ".uc" }

// componentLines reads a component's labels, focus, paths and uses.
func (l *loader) componentLines(c component, parent *entity) {
	e := c.entity
	var labels, paths []string
	var uses []syntax.Ref
	for _, line := range c.decl.Lines {
		switch line := line.(type) {
		case *syntax.Label:
			labels = append(labels, string(line.Name))
			l.label(e, line)
		case *syntax.Focus:
			e.focus = line.Level
		case *syntax.Paths:
			for _, p := range line.Patterns {
				paths = append(paths, string(p))
			}
		case *syntax.Uses:
			uses = line.Refs
		}
	}
	if e.focus == "" && parent != nil {
		e.focus = parent.focus
	}
	if e.focus != "" {
		e.setAttr("focus", []string{e.focus})
	}
	e.setAttr("labels", labels)
	e.setAttr("paths", paths)
	l.uses = append(l.uses, func() {
		for _, r := range uses {
			if target := l.refer(e, e, r.Segments, r.Pos, acceptUses); target != nil {
				e.uses = append(e.uses, target)
				l.edge(graph.EdgeKindUses, e, target, c.file, r.Pos, graph.SourceDeclared, nil)
			}
		}
	})
}

// members declares a component's types, operations, flows and properties.
func (l *loader) members(c component) {
	for _, d := range c.file.Decls {
		switch d := d.(type) {
		case *syntax.TypeDecl:
			l.typeDecl(c.file, c.entity, c.entity, d)
		case *syntax.OpDecl:
			l.opDecl(c.file, c.entity, c.entity, d)
		case *syntax.FlowDecl:
			l.flowDecl(c.file, c.entity, d)
		case *syntax.PropertyDecl:
			l.propertyDecl(c.file, c.entity, d)
		}
	}
}

// typeDecl declares a type and its operations under parent, a component or
// an external actor, whose scope its references resolve in.
func (l *loader) typeDecl(f *syntax.File, parent, scope *entity, d *syntax.TypeDecl) {
	e := &entity{name: string(d.Name), kind: graph.NodeKindType, form: d.Form, file: f, pos: d.Pos, summary: string(d.Summary)}
	if !l.declare(parent, e, nil) {
		return
	}
	var labels []string
	taken := map[string]bool{} // the names of the fields and values, which the operations may not take
	for _, line := range d.Lines {
		switch line := line.(type) {
		case *syntax.Label:
			labels = append(labels, string(line.Name))
			l.label(e, line)
		case *syntax.EnumValue:
			e.values = append(e.values, string(line.Name))
			l.unique(e, taken, string(line.Name), line.Pos)
		case *syntax.Field:
			l.unique(e, taken, string(line.Name), line.Pos)
		case *syntax.OpDecl:
			if taken[string(line.Name)] {
				l.unique(e, taken, string(line.Name), line.Pos)
			}
			l.opDecl(f, e, scope, line)
		}
	}
	e.setAttr("labels", labels)
	e.setAttr("values", e.values)
	l.signatures = append(l.signatures, func() {
		for _, line := range d.Lines {
			switch line := line.(type) {
			case *syntax.Field:
				t := l.typeOf(e, scope, line.Type)
				e.fields = append(e.fields, member{string(line.Name), t})
				l.refers(e, t)
				l.depends(e, t.allRefs())
			case *syntax.Extends:
				l.supertype(e, scope, graph.EdgeKindExtends, line.Type)
			case *syntax.Implements:
				l.supertype(e, scope, graph.EdgeKindImplements, line.Type)
			}
		}
		e.setAttr("fields", members(e.fields))
	})
}

func (l *loader) supertype(e, scope *entity, kind graph.EdgeKind, r syntax.TypeRef) {
	target := l.refer(e, scope, r.Segments, r.Pos, acceptType)
	l.edge(kind, e, target, e.file, r.Pos, graph.SourceDeclared, nil)
	l.depends(e, []ref{{strings.Join(r.Segments, "."), r.Pos, target}})
}

// unique reports a member of e whose name an earlier one has (docs/model.md
// §10 rule 2, read for the fields, values and operations of a type, the
// parameters of an operation, and the symbols and cases of a flow or a property).
func (l *loader) unique(e *entity, names map[string]bool, name string, pos lexer.Position) {
	if names[name] {
		l.report(e.file, pos, "design.duplicate-name", e.id,
			fmt.Sprintf("%s has two members named %s", describe(e), name), "rename one of the two, or remove one")
	}
	names[name] = true
}

// refers records a refers edge from e to every type that t names.
func (l *loader) refers(e *entity, t *typ) {
	for _, r := range t.allRefs() {
		if r.target.is(graph.NodeKindType) {
			l.edge(graph.EdgeKindRefers, e, r.target, e.file, r.pos, graph.SourceDeclared, nil)
		}
	}
}

// opDecl declares an operation under parent: a component, an external actor or a type.
func (l *loader) opDecl(f *syntax.File, parent, scope *entity, d *syntax.OpDecl) {
	e := &entity{name: string(d.Name), kind: graph.NodeKindOp, form: "op", file: f, pos: d.Pos}
	if d.Summary != nil {
		e.summary = string(*d.Summary)
	}
	if !l.declare(parent, e, nil) {
		return
	}
	if parent.kind == graph.NodeKindType {
		parent.ops = append(parent.ops, e)
	}
	params := map[string]bool{}
	for _, p := range d.Params {
		l.unique(e, params, string(p.Name), p.Pos)
	}
	l.signatures = append(l.signatures, func() {
		for _, p := range d.Params {
			t := l.typeOf(e, scope, p.Type)
			e.params = append(e.params, member{string(p.Name), t})
			l.refers(e, t)
			l.depends(e, t.allRefs())
		}
		signature := "(" + strings.Join(members(e.params), ", ") + ")"
		if d.Result != nil {
			t := l.typeOf(e, scope, d.Result)
			l.refers(e, t)
			l.depends(e, t.allRefs())
			signature += " -> " + t.text()
		}
		var raises []string
		for _, r := range d.Raises {
			target := l.refer(e, scope, r.Segments, r.Pos, acceptError)
			written := ref{strings.Join(r.Segments, "."), r.Pos, target}
			raises = append(raises, written.text())
			l.depends(e, []ref{written})
			if target != nil {
				e.raises = append(e.raises, target)
				l.edge(graph.EdgeKindRefers, e, target, f, r.Pos, graph.SourceDeclared, nil)
			}
		}
		if len(raises) > 0 {
			signature += " raises " + strings.Join(raises, ", ")
		}
		e.setAttr("signature", []string{signature})
		if d.Handles != nil {
			e.handles = l.refer(e, scope, d.Handles.Segments, d.Handles.Pos, acceptType)
			l.edge(graph.EdgeKindSubscribes, e, e.handles, f, d.Handles.Pos, graph.SourceDeclared, nil)
			l.depends(e, []ref{{strings.Join(d.Handles.Segments, "."), d.Handles.Pos, e.handles}})
			l.subscription(e, d.Handles.Pos)
		}
	})
}

func (l *loader) flowDecl(f *syntax.File, parent *entity, d *syntax.FlowDecl) {
	e := &entity{name: string(d.Name), kind: graph.NodeKindFlow, form: "flow", file: f, pos: d.Pos, summary: string(d.Summary)}
	if !l.declare(parent, e, nil) {
		return
	}
	var symbols []*syntax.Symbol
	var limits []any
	var cases []*syntax.CaseDecl
	var steps []syntax.Step
	for _, line := range d.Lines {
		switch line := line.(type) {
		case *syntax.Symbol:
			symbols = append(symbols, line)
		case *syntax.IntervalLimit, *syntax.SubsetLimit:
			limits = append(limits, line)
		case *syntax.CaseDecl:
			cases = append(cases, line)
			l.caseDecl(f, e, line)
		case syntax.Step:
			steps = append(steps, line)
		}
	}
	l.inputs(e, parent, symbols, limits, cases)
	l.checks = append(l.checks, func() {
		s := &stepper{l: l, flow: e, scope: parent, emitted: map[*entity]bool{}, sent: map[*entity]bool{}}
		s.list(e, steps, within{}, "")
	})
}

func (l *loader) propertyDecl(f *syntax.File, parent *entity, d *syntax.PropertyDecl) {
	e := &entity{name: string(d.Name), kind: graph.NodeKindProperty, form: d.Form, file: f, pos: d.Pos, summary: string(d.Summary)}
	if !l.declare(parent, e, nil) {
		return
	}
	var symbols []*syntax.Symbol
	var limits []any
	var cases []*syntax.CaseDecl
	for _, line := range d.Lines {
		switch line := line.(type) {
		case *syntax.About:
			l.signatures = append(l.signatures, func() {
				for _, r := range line.Refs {
					l.edge(graph.EdgeKindAbout, e, l.refer(e, parent, r.Segments, r.Pos, acceptAbout), f, r.Pos, graph.SourceDeclared, nil)
				}
			})
		case *syntax.Symbol:
			symbols = append(symbols, line)
		case *syntax.IntervalLimit, *syntax.SubsetLimit:
			limits = append(limits, line)
		case *syntax.CaseDecl:
			cases = append(cases, line)
			l.caseDecl(f, e, line)
		}
	}
	l.inputs(e, parent, symbols, limits, cases)
}

// caseDecl declares a declared case of a flow or a property.
func (l *loader) caseDecl(f *syntax.File, owner *entity, d *syntax.CaseDecl) {
	e := &entity{name: string(d.Name), kind: graph.NodeKindCase, form: "declared", file: f, pos: d.Pos}
	if d.Summary != nil {
		e.summary = string(*d.Summary)
	}
	if !l.declare(owner, e, nil) {
		return
	}
	if recordedName.MatchString(e.name) {
		l.report(f, d.Pos, "design.recorded-case-name", e.id,
			fmt.Sprintf("the declared case %s has the name of a recorded case, r- and eight hex digits", e.name),
			"rename the case; names of the form r-0123abcd are left to recorded cases")
	}
	e.setAttr("values", assignments(d.Lines))
	l.checks = append(l.checks, func() { l.checkCase(e, owner, d.Lines) })
}

// assignments writes the values of a case as `symbol = value`.
func assignments(lines []syntax.CaseLine) []string {
	var out []string
	for _, line := range lines {
		if a, ok := line.(*syntax.Assignment); ok {
			out = append(out, string(a.Symbol)+" = "+syntax.ValueText(a.Value))
		}
	}
	return out
}

// corpus declares the recorded cases of a corpus file under the property or
// flow it names (docs/model.md §10 rule 29).
func (l *loader) corpus(f *syntax.File) {
	var decl *syntax.CorpusDecl
	for _, d := range f.Decls {
		if d, ok := d.(*syntax.CorpusDecl); ok {
			decl = d
			break
		}
	}
	file := &entity{id: f.Path, file: f}
	target := l.refer(file, nil, decl.Target.Segments, decl.Target.Pos, acceptCorpus)
	if target == nil {
		return
	}
	if want := corpusPath(target.id); want != f.Path {
		l.report(f, decl.Pos, "design.corpus-path-mismatch", f.Path,
			fmt.Sprintf("the recorded cases of %s belong in %s, not in %s", target.id, want, f.Path),
			"move the file to "+want)
	}
	for _, d := range f.Decls {
		rc, ok := d.(*syntax.RecordedCase)
		if !ok {
			continue
		}
		e := &entity{name: string(rc.Name), kind: graph.NodeKindCase, form: "recorded", source: graph.SourceProduced, file: f, pos: rc.Pos}
		if !l.declare(target, e, nil) {
			continue
		}
		e.setAttr("values", assignments(rc.Lines))
		e.setAttr("reason", []string{rc.Reason})
		l.checks = append(l.checks, func() { l.checkCase(e, target, rc.Lines) })
	}
}
