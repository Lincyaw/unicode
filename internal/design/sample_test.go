package design

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/syntax"
)

// This file builds random valid designs: a model of the nodes the files
// declare, the syntax trees of the files, and the identities of the nodes and
// edges (`kind from to`) that docs/model.md §3–§4 and §7 give them. Every
// choice keeps the design valid: references are written so that they resolve
// by docs/format.md §2, dependencies follow `uses`, and cases fit their
// symbols' types and limits.

// Sample is a valid set of design and corpus files, with the nodes and edges
// they declare, and the attributes of the nodes it lists, by identity.
//
//uc:realizes design.Sample
type Sample struct {
	Files map[string]string
	Nodes []string
	Edges []string
	Attrs map[string]map[string][]string
}

// gNode is one declared node of the model.
type gNode struct {
	id, name, kind, form string
	parent               *gNode
	file                 string

	component *syntax.ComponentDecl
	uses      []*gNode

	typeDecl  *syntax.TypeDecl
	fields    []gField
	values    []string
	ops       []*gNode
	simple    bool // can be generated, without recursion
	recursive bool // a tree type, generable through its own list or optional

	opDecl  *syntax.OpDecl
	params  []gField
	raises  []*gNode
	handles *gNode

	flowDecl *syntax.FlowDecl
	propDecl *syntax.PropertyDecl
	symbols  []gSymbol
	limits   []gLimit
}

type gField struct {
	name string
	t    *gType
}

// gType is a type expression of the model.
type gType struct {
	builtin string
	named   *gNode
	kind    string // list, set, optional, map or call
	elems   []*gType
	targets []*gNode
}

type gSymbol struct {
	name      string
	t         *gType
	generated bool
	decl      *syntax.Symbol
}

// gLimit is a limit and the type in the symbol's type where it applies.
type gLimit struct {
	path      []string
	op        string // in, size or subset
	low, high int
	members   []string
	at        *gType
	decl      any
}

// designer draws one design.
type designer struct {
	t     *rapid.T
	rich  bool
	used  map[string]bool
	nodes []string
	edges []string
	files map[string]*syntax.File

	system     *gNode
	notations  []string
	roots      []string // each `language: dir`
	persons    []*gNode
	externals  []*gNode
	containers []*gNode
	types      []*gNode
	ops        []*gNode
	flows      []*gNode
	props      []*gNode

	skel skeleton
}

// skeleton holds the parts of a rich design that the broken designs change.
type skeleton struct {
	top, child           *gNode
	errorType, event     *gNode
	record, iface        *gNode
	service              *gNode // an object with the operations below
	raising, helper, sub *gNode
	flow, prop           *gNode
	call                 *syntax.Call // the flow's first call, with steps of its own
	inner                *syntax.Call // a call among them
	emit                 *syntax.Emit
	on                   *syntax.OnBlock
	async                *syntax.Async
	propCase             *syntax.CaseDecl
	number               *syntax.Assignment // the case's value of the limited int symbol
	recorded             *syntax.RecordedCase
	corpusPath           string
}

var (
	kebabNames = rapid.StringMatching(`[a-z][a-z0-9]{1,5}(-[a-z0-9]{1,4})?`)
	lowerNames = rapid.StringMatching(`[a-z][a-z0-9]{1,5}([A-Z][a-z0-9]{0,3})?`)
	upperNames = rapid.StringMatching(`[A-Z][a-z0-9]{1,5}([A-Z][a-z0-9]{0,3})?`)
	// keywordNames are drawn as names now and then: no keyword is reserved.
	keywordNames = []string{"type", "value", "on", "case", "field", "op", "flow", "in", "size", "call", "list",
		"string", "label", "uses", "focus", "core", "async", "alt", "opt", "loop", "else", "emits", "about",
		"forall", "where", "generated", "root", "actor", "person", "external", "component", "system", "notation",
		"extends", "raises", "handles", "property", "corpus", "coverage", "record", "enum", "map", "set", "bool"}
	summaries = rapid.StringMatching(`[a-z中文]{1,4}( [a-z中文"\\]{1,4}){0,2}`)
)

// unique draws a name that no other node, member or symbol of the design has,
// so that a single-segment reference never finds a second node first.
func (d *designer) unique(gen *rapid.Generator[string], label string) string {
	name := rapid.OneOf(gen, gen, gen, rapid.SampledFrom(keywordNames)).Draw(d.t, label)
	for n := 2; d.used[name]; n++ {
		name = strings.TrimRight(name, "0123456789") + strconv.Itoa(n)
	}
	d.used[name] = true
	return name
}

func (d *designer) kebab(label string) string { return d.unique(kebabNames, label) }
func (d *designer) lower(label string) string { return d.unique(lowerNames, label) }

func (d *designer) summary(label string) syntax.Summary {
	return syntax.Summary(summaries.Draw(d.t, label))
}

func (d *designer) optSummary(label string) *syntax.Summary {
	if !rapid.Bool().Draw(d.t, label+"?") {
		return nil
	}
	s := d.summary(label)
	return &s
}

func (d *designer) edge(kind string, from, to *gNode) {
	d.edges = append(d.edges, kind+" "+from.id+" "+to.id)
}

// declare gives n its identity under parent and records it.
func (d *designer) declare(parent, n *gNode) *gNode {
	n.parent = parent
	switch parent {
	case nil:
		n.id = n.name
	case d.system:
		n.id = n.name
		d.edge("contains", parent, n)
	default:
		n.id = parent.id + "." + n.name
		d.edge("contains", parent, n)
	}
	d.nodes = append(d.nodes, n.id)
	return n
}

// home is the component, external actor or system that holds n.
func gHome(n *gNode) *gNode {
	for x := n; x != nil; x = x.parent {
		switch x.kind {
		case "component", "external", "system":
			return x
		}
	}
	return nil
}

// allowed tells whether code in a may depend on b (docs/model.md §10 rule 13).
func gAllowed(a, b *gNode) bool {
	if a == b || b.kind == "component" && b.parent == a {
		return true
	}
	for x := a; x != nil; x = x.parent {
		if slices.Contains(x.uses, b) {
			return true
		}
	}
	return false
}

// refTo writes a reference from scope to n so that it resolves to n, now
// relative to scope and now from the root.
func (d *designer) refTo(n, scope *gNode) []string {
	segs := strings.Split(n.id, ".")
	if scope != nil && strings.HasPrefix(n.id, scope.id+".") && rapid.Bool().Draw(d.t, "relative") {
		rest := strings.Split(strings.TrimPrefix(n.id, scope.id+"."), ".")
		if isUpper(rest[0]) || len(rest) == 1 {
			return rest
		}
	}
	return segs
}

// gCodeName is the Go name of a type, an operation, or the predicate of a
// property or a flow (docs/design.md §4): each segment of the name with an
// upper-case first letter, after check for a property and checkFlow for a flow.
func gCodeName(kind, name string) string {
	var camel strings.Builder
	for _, seg := range strings.Split(name, "-") {
		camel.WriteString(strings.ToUpper(seg[:1]) + seg[1:])
	}
	switch kind {
	case "property":
		return "check" + camel.String()
	case "flow":
		return "checkFlow" + camel.String()
	}
	return camel.String()
}

// clashes tells whether a member of parent of the given kind and name would
// take the Go name of a type, an operation, a flow or a property already in
// parent (docs/model.md §10 rule 30).
func (d *designer) clashes(parent *gNode, kind, name string) bool {
	code := gCodeName(kind, name)
	for _, members := range [][]*gNode{d.types, d.ops, d.flows, d.props} {
		if slices.ContainsFunc(members, func(n *gNode) bool { return n.parent == parent && gCodeName(n.kind, n.name) == code }) {
			return true
		}
	}
	return false
}

// freeName draws names for a member of parent until one clashes with none there.
func (d *designer) freeName(parent *gNode, kind string, draw func() string) string {
	for {
		if name := draw(); !d.clashes(parent, kind, name) {
			return name
		}
	}
}

func pick[T any](d *designer, label string, from []T) T {
	return rapid.SampledFrom(from).Draw(d.t, label)
}

func count(d *designer, label string, low, high int) int {
	return rapid.IntRange(low, high).Draw(d.t, label)
}

// genSample draws a valid design: a system file, a tree of components with
// types, operations, flows and properties, and corpus files.
//
//uc:generates design.accepts-valid-design.sample
func genSample() *rapid.Generator[Sample] {
	return rapid.Custom(func(t *rapid.T) Sample {
		d := newDesign(t, rapid.Bool().Draw(t, "rich"))
		return d.sample()
	})
}

func newDesign(t *rapid.T, rich bool) *designer {
	// The reserved literals count as used, so no name is one of them; so does
	// system, since a top-level component system would be the system file.
	used := map[string]bool{"true": true, "false": true, "none": true, "system": true}
	d := &designer{t: t, rich: rich, used: used, files: map[string]*syntax.File{}}
	d.genSystem()
	d.genContainers()
	d.genTypes()
	d.genFields()
	d.genOps()
	d.genFlows()
	d.genProperties()
	d.genCorpus()
	return d
}

// sample prints every file in the canonical format. Of the attributes, it
// lists the system's: its notations and its roots (docs/design.md §5.2).
func (d *designer) sample() Sample {
	files := map[string]string{}
	for path, f := range d.files {
		files[path] = string(f.Print())
	}
	system := map[string][]string{}
	for key, values := range map[string][]string{"notations": d.notations, "roots": d.roots} {
		if len(values) > 0 {
			system[key] = slices.Clone(values)
		}
	}
	attrs := map[string]map[string][]string{d.system.id: system}
	return Sample{Files: files, Nodes: slices.Clone(d.nodes), Edges: slices.Clone(d.edges), Attrs: attrs}
}

func (d *designer) file(path string, kind syntax.FileKind) *syntax.File {
	f := &syntax.File{Path: path, Kind: kind}
	d.files[path] = f
	return f
}

var (
	containerLabels = map[string][]string{"ddd": {"bounded-context"}, "c4": {"software-system", "container", "component"}}
	typeLabels      = map[string][]string{"ddd": {"aggregate", "entity", "value", "domain-service", "repository", "factory"}}
)

// labels draws distinct labels of the declared notations from vocabulary.
func (d *designer) labels(vocabulary map[string][]string) []string {
	var all []string
	for _, n := range d.notations {
		all = append(all, vocabulary[n]...)
	}
	if len(all) == 0 {
		return nil
	}
	var out []string
	for range count(d, "labels", 0, 2) {
		if l := pick(d, "label", all); !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	return out
}

func (d *designer) genSystem() {
	f := d.file(systemPath, syntax.FileKindSystem)
	d.system = d.declare(nil, &gNode{name: d.kebab("system"), kind: "system", form: "system", file: systemPath})
	f.Decls = append(f.Decls, &syntax.SystemDecl{Name: syntax.Kebab(d.system.name), Summary: d.summary("summary")})
	for _, n := range []string{"ddd", "c4"} {
		if rapid.Bool().Draw(d.t, "notation "+n) {
			d.notations = append(d.notations, n)
			f.Decls = append(f.Decls, &syntax.NotationDecl{Name: n})
		}
	}
	// A rich design configures Go, whose name mapping the collisions follow.
	for _, language := range []string{"go", "python"} {
		if language == "go" && d.rich || rapid.Bool().Draw(d.t, "language "+language) {
			decl := &syntax.LanguageDecl{Name: language}
			for _, dir := range rapid.SliceOfNDistinct(rapid.SampledFrom(rootDirs), 1, 2, rapid.ID).Draw(d.t, "roots") {
				decl.Lines = append(decl.Lines, &syntax.Root{Path: syntax.Text(dir)})
				d.roots = append(d.roots, language+": "+dir)
			}
			f.Decls = append(f.Decls, decl)
		}
	}
	persons := count(d, "persons", 0, 2)
	if d.rich {
		persons = max(persons, 1)
	}
	for range persons {
		p := d.declare(d.system, &gNode{name: d.kebab("person"), kind: "person", form: "person", file: systemPath})
		d.persons = append(d.persons, p)
		f.Decls = append(f.Decls, &syntax.PersonDecl{Name: syntax.Kebab(p.name), Summary: d.summary("summary")})
	}
	externals := count(d, "externals", 0, 2)
	if d.rich {
		externals = max(externals, 1)
	}
	for range externals {
		x := d.declare(d.system, &gNode{name: d.kebab("external"), kind: "external", form: "external", file: systemPath})
		d.externals = append(d.externals, x)
		decl := &syntax.ExternalDecl{Name: syntax.Kebab(x.name), Summary: d.summary("summary")}
		f.Decls = append(f.Decls, decl)
		var errs []*gNode
		for i := range count(d, "externalTypes", 0, 2) {
			form := pick(d, "form", []string{"record", "error", "event", "enum"})
			if i == 0 && d.rich {
				form = "error"
			}
			t := d.newType(x, form)
			valueLines(t)
			decl.Lines = append(decl.Lines, t.typeDecl)
			for range count(d, "fields", 0, 2) {
				if form != "enum" {
					d.addField(t, &gType{builtin: pick(d, "builtin", builtins)})
				}
			}
			if form == "error" {
				errs = append(errs, t)
			}
		}
		for i := range count(d, "externalOps", 0, 2) {
			op := d.newOp(x, x)
			decl.Lines = append(decl.Lines, op.opDecl)
			for range count(d, "params", 0, 2) {
				d.addParam(op, x, &gType{builtin: pick(d, "builtin", builtins)})
			}
			if len(errs) > 0 && (i == 0 && d.rich || rapid.Bool().Draw(d.t, "raises?")) {
				d.addRaises(op, x, errs[0])
			}
		}
	}
}

var (
	builtins = []string{"bool", "int", "float", "string", "bytes", "time", "duration"}
	rootDirs = []string{".", "cmd", "internal", "py/src", "源码", `a "b"`}
)

// genContainers draws the tree of components and what each uses.
func (d *designer) genContainers() {
	tops := count(d, "components", 0, 3)
	if d.rich {
		tops = max(tops, 1)
	}
	for i := range tops {
		c := d.newComponent(d.system)
		if i == 0 && d.rich {
			d.skel.top = c
			d.skel.child = d.newComponent(c)
		}
		for range count(d, "children", 0, 1) {
			child := d.newComponent(c)
			if rapid.IntRange(0, 2).Draw(d.t, "grandchild") == 0 {
				d.newComponent(child)
			}
		}
	}
	targets := append(slices.Clone(d.containers), d.externals...)
	for _, c := range d.containers {
		var uses []*gNode
		for range count(d, "uses", 0, 3) {
			if u := pick(d, "use", targets); u != c && !slices.Contains(uses, u) {
				uses = append(uses, u)
			}
		}
		if c == d.skel.top {
			for _, x := range d.externals {
				if !slices.Contains(uses, x) {
					uses = append(uses, x)
				}
			}
		}
		if len(uses) == 0 {
			continue
		}
		c.uses = uses
		line := &syntax.Uses{}
		for _, u := range uses {
			line.Refs = append(line.Refs, syntax.Ref{Segments: d.refTo(u, c)})
			d.edge("uses", c, u)
		}
		c.component.Lines = append(c.component.Lines, line)
	}
}

func (d *designer) newComponent(parent *gNode) *gNode {
	c := &gNode{name: d.kebab("component"), kind: "component", form: "component"}
	d.declare(parent, c)
	c.file = componentPath(c.id)
	d.containers = append(d.containers, c)
	decl := &syntax.ComponentDecl{Path: syntax.Path{Segments: strings.Split(c.id, ".")}, Summary: d.summary("summary")}
	for _, l := range d.labels(containerLabels) {
		decl.Lines = append(decl.Lines, &syntax.Label{Name: syntax.Kebab(l)})
	}
	if parent == d.system || rapid.Bool().Draw(d.t, "focus?") {
		decl.Lines = append(decl.Lines, &syntax.Focus{Level: pick(d, "focus", []string{"core", "support", "utility"})})
	}
	paths := &syntax.Paths{Patterns: []syntax.Text{syntax.Text("internal/" + strings.ReplaceAll(c.id, ".", "/") + "/**")}}
	if rapid.Bool().Draw(d.t, "secondPath") {
		paths.Patterns = append(paths.Patterns, "cmd/"+syntax.Text(c.name)+"/*.go")
	}
	decl.Lines = append(decl.Lines, paths)
	c.component = decl
	d.file(c.file, syntax.FileKindComponent).Decls = []syntax.Decl{decl}
	return c
}

// newType declares a type of the given form in parent, a component or an
// external actor; its Go name is unique in parent.
func (d *designer) newType(parent *gNode, form string) *gNode {
	name := d.freeName(parent, "type", func() string { return upperNames.Draw(d.t, "type") })
	t := d.declare(parent, &gNode{name: name, kind: "type", form: form, file: parent.file})
	d.types = append(d.types, t)
	t.typeDecl = &syntax.TypeDecl{Name: syntax.Upper(name), Form: form, Summary: d.summary("summary")}
	if parent.kind == "component" {
		for _, l := range d.labels(typeLabels) {
			t.typeDecl.Lines = append(t.typeDecl.Lines, &syntax.Label{Name: syntax.Kebab(l)})
		}
	}
	if form == "enum" {
		for range count(d, "values", 1, 3) {
			t.values = append(t.values, d.lower("value"))
		}
	}
	return t
}

// valueLines writes an enum's values, which follow its extends and implements.
func valueLines(t *gNode) {
	for _, v := range t.values {
		t.typeDecl.Lines = append(t.typeDecl.Lines, &syntax.EnumValue{Name: syntax.Lower(v)})
	}
}

func (d *designer) componentFile(c *gNode) *syntax.File { return d.files[c.file] }

var typeForms = []string{"object", "record", "interface", "enum", "error", "event"}

// genTypes declares the types of every component.
func (d *designer) genTypes() {
	for _, c := range d.containers {
		f := d.componentFile(c)
		var forms []string
		for range count(d, "types", 0, 3) {
			forms = append(forms, pick(d, "form", typeForms))
		}
		if c == d.skel.top {
			forms = append(forms, "error", "event", "record", "interface", "enum", "object")
		}
		for i, form := range forms {
			t := d.newType(c, form)
			f.Decls = append(f.Decls, t.typeDecl)
			if c == d.skel.top && i >= len(forms)-6 {
				switch form {
				case "error":
					d.skel.errorType = t
				case "event":
					d.skel.event = t
				case "record":
					d.skel.record = t
				case "interface":
					d.skel.iface = t
				case "object":
					d.skel.service = t
				}
			}
		}
		if rapid.IntRange(0, 3).Draw(d.t, "tree?") == 0 {
			d.newTree(c)
		}
	}
}

// newTree declares a record that holds a list or an optional of itself: a
// recursive type that can still be generated.
func (d *designer) newTree(c *gNode) {
	t := d.newType(c, "record")
	t.recursive = true
	d.componentFile(c).Decls = append(d.componentFile(c).Decls, t.typeDecl)
	d.addField(t, &gType{builtin: "string"})
	d.addField(t, &gType{kind: pick(d, "treeKind", []string{"list", "optional"}), elems: []*gType{{named: t}}})
}

// typesFor lists the types that code in a may refer to, whose forms are among forms.
func (d *designer) typesFor(a *gNode, forms ...string) []*gNode {
	var out []*gNode
	for _, t := range d.types {
		if gAllowed(a, gHome(t)) && !t.recursive && (len(forms) == 0 || slices.Contains(forms, t.form)) {
			out = append(out, t)
		}
	}
	return out
}

// memberType draws the type of a field or a parameter of code in a.
func (d *designer) memberType(a *gNode, depth int) *gType {
	choices := d.typesFor(a)
	kind := count(d, "memberType", 0, 5)
	if depth <= 0 {
		kind %= 2
	}
	switch {
	case kind == 1 && len(choices) > 0:
		return &gType{named: pick(d, "named", choices)}
	case kind == 2 || kind == 3:
		return &gType{kind: pick(d, "elem", []string{"list", "set", "optional"}), elems: []*gType{d.memberType(a, depth-1)}}
	case kind == 4:
		return &gType{kind: "map", elems: []*gType{{builtin: pick(d, "key", []string{"string", "int"})}, d.memberType(a, depth-1)}}
	}
	return &gType{builtin: pick(d, "builtin", builtins)}
}

// expr writes t as a syntax tree, its references written from scope.
func (d *designer) expr(t *gType, scope *gNode) syntax.TypeExpr {
	switch {
	case t.builtin != "":
		return &syntax.Builtin{Name: t.builtin}
	case t.named != nil:
		return &syntax.NamedType{Ref: syntax.TypeRef{Segments: d.refTo(t.named, scope)}}
	case t.kind == "map":
		return &syntax.MapType{Key: d.expr(t.elems[0], scope), Value: d.expr(t.elems[1], scope)}
	case t.kind == "call":
		c := &syntax.CallType{}
		for _, target := range t.targets {
			c.Targets = append(c.Targets, syntax.Ref{Segments: d.refTo(target, scope)})
		}
		return c
	}
	return &syntax.ElemType{Kind: t.kind, Elem: d.expr(t.elems[0], scope)}
}

// refersTo records the refers edges from n to the types that t names.
func (d *designer) refersTo(n *gNode, t *gType) {
	seen := map[string]bool{}
	var walk func(t *gType)
	walk = func(t *gType) {
		if t.named != nil {
			if !seen[t.named.id] {
				seen[t.named.id] = true
				if !slices.Contains(d.edges, "refers "+n.id+" "+t.named.id) {
					d.edge("refers", n, t.named)
				}
			}
		}
		for _, target := range t.targets {
			if target.kind == "type" && !slices.Contains(d.edges, "refers "+n.id+" "+target.id) {
				d.edge("refers", n, target)
			}
		}
		for _, e := range t.elems {
			walk(e)
		}
	}
	walk(t)
}

func (d *designer) addField(t *gNode, ft *gType) {
	name := d.lower("field")
	t.fields = append(t.fields, gField{name, ft})
	t.typeDecl.Lines = append(t.typeDecl.Lines, &syntax.Field{Name: syntax.Lower(name), Type: d.expr(ft, gHome(t))})
	d.refersTo(t, ft)
}

// genFields draws the extends, implements and fields of every type, then
// marks the types that can be generated.
func (d *designer) genFields() {
	for _, t := range d.types {
		if t.parent.kind != "component" || t.recursive {
			continue
		}
		a := gHome(t)
		others := slices.DeleteFunc(d.typesFor(a), func(o *gNode) bool { return o == t })
		if len(others) > 0 && rapid.IntRange(0, 3).Draw(d.t, "extends?") == 0 {
			super := pick(d, "super", others)
			t.typeDecl.Lines = append(t.typeDecl.Lines, &syntax.Extends{Type: syntax.TypeRef{Segments: d.refTo(super, a)}})
			d.edge("extends", t, super)
		}
		ifaces := slices.DeleteFunc(d.typesFor(a, "interface"), func(o *gNode) bool { return o == t })
		if len(ifaces) > 0 && rapid.IntRange(0, 2).Draw(d.t, "implements?") == 0 {
			iface := pick(d, "iface", ifaces)
			t.typeDecl.Lines = append(t.typeDecl.Lines, &syntax.Implements{Type: syntax.TypeRef{Segments: d.refTo(iface, a)}})
			d.edge("implements", t, iface)
		}
		valueLines(t)
		if t.form == "enum" || t.form == "interface" {
			continue
		}
		n := count(d, "fields", 0, 3)
		if t == d.skel.event || t == d.skel.record {
			n = max(n, 1)
		}
		for range n {
			if t == d.skel.record || t == d.skel.event {
				d.addField(t, &gType{builtin: pick(d, "builtin", builtins)})
				continue
			}
			d.addField(t, d.memberType(a, 2))
		}
	}
	// A type is simple when its fields are; the fixpoint leaves out every cycle.
	for changed := true; changed; {
		changed = false
		for _, t := range d.types {
			if t.simple || t.recursive || t.form == "interface" || t.form == "error" {
				continue
			}
			if !slices.ContainsFunc(t.fields, func(f gField) bool { return !d.simpleType(f.t) }) {
				t.simple, changed = true, true
			}
		}
	}
}

// simpleType tells whether values of t can be drawn: builtins, simple named
// types, and lists, sets, optionals and maps with writable keys of them.
func (d *designer) simpleType(t *gType) bool {
	switch {
	case t.builtin != "":
		return true
	case t.named != nil:
		return t.named.simple && (t.named.form != "enum" || len(t.named.values) > 0)
	case t.kind == "map":
		k := t.elems[0]
		return (k.builtin == "string" || k.builtin == "int") && d.simpleType(t.elems[1])
	case t.kind == "call":
		return false
	}
	return d.simpleType(t.elems[0])
}

func (d *designer) newOp(parent, scope *gNode) *gNode {
	name := d.freeName(parent, "op", func() string { return d.lower("op") })
	op := d.declare(parent, &gNode{name: name, kind: "op", form: "op", file: parent.file})
	d.ops = append(d.ops, op)
	op.opDecl = &syntax.OpDecl{Name: syntax.Lower(op.name), Summary: d.optSummary("opSummary")}
	if parent.kind == "type" {
		parent.ops = append(parent.ops, op)
	}
	return op
}

func (d *designer) addParam(op, scope *gNode, t *gType) {
	name := d.lower("param")
	op.params = append(op.params, gField{name, t})
	op.opDecl.Params = append(op.opDecl.Params, syntax.Param{Name: syntax.Lower(name), Type: d.expr(t, scope)})
	d.refersTo(op, t)
}

func (d *designer) addRaises(op, scope, e *gNode) {
	op.raises = append(op.raises, e)
	op.opDecl.Raises = append(op.opDecl.Raises, syntax.TypeRef{Segments: d.refTo(e, scope)})
	if !slices.Contains(d.edges, "refers "+op.id+" "+e.id) {
		d.edge("refers", op, e)
	}
}

// genOps draws the operations of the types and components.
func (d *designer) genOps() {
	for _, c := range d.containers {
		for _, t := range slices.Clone(d.types) {
			if t.parent != c || t.recursive || t.form == "enum" {
				continue
			}
			n := count(d, "typeOps", 0, 2)
			if t == d.skel.service {
				d.skel.raising = d.newOp(t, c)
				d.addParam(d.skel.raising, c, &gType{builtin: "int"})
				d.addRaises(d.skel.raising, c, d.skel.errorType)
				d.skel.helper = d.newOp(t, c)
				d.skel.sub = d.newOp(t, c)
				d.subscribe(d.skel.sub, c, d.skel.event)
				for _, op := range []*gNode{d.skel.raising, d.skel.helper, d.skel.sub} {
					t.typeDecl.Lines = append(t.typeDecl.Lines, op.opDecl)
				}
			}
			for range n {
				op := d.randomOp(t, c)
				t.typeDecl.Lines = append(t.typeDecl.Lines, op.opDecl)
			}
		}
		f := d.componentFile(c)
		for range count(d, "componentOps", 0, 2) {
			f.Decls = append(f.Decls, d.randomOp(c, c).opDecl)
		}
	}
}

func (d *designer) subscribe(op, scope, event *gNode) {
	d.addParam(op, scope, &gType{named: event})
	op.handles = event
	op.opDecl.Handles = &syntax.TypeRef{Segments: d.refTo(event, scope)}
	d.edge("subscribes", op, event)
}

func (d *designer) randomOp(parent, c *gNode) *gNode {
	op := d.newOp(parent, c)
	if events := d.typesFor(c, "event"); len(events) > 0 && rapid.IntRange(0, 4).Draw(d.t, "subscriber?") == 0 {
		d.subscribe(op, c, pick(d, "event", events))
	} else {
		for range count(d, "params", 0, 3) {
			d.addParam(op, c, d.memberType(c, 2))
		}
	}
	if rapid.Bool().Draw(d.t, "result?") {
		t := d.memberType(c, 2)
		op.opDecl.Result = d.expr(t, c)
		d.refersTo(op, t)
	}
	if errs := d.typesFor(c, "error"); len(errs) > 0 {
		for range count(d, "raises", 0, 2) {
			if e := pick(d, "error", errs); !slices.Contains(op.raises, e) {
				d.addRaises(op, c, e)
			}
		}
	}
	return op
}

func (d *designer) sortedFiles() []string { return slices.Sorted(maps.Keys(d.files)) }

func (d *designer) String() string {
	var b strings.Builder
	for _, p := range d.sortedFiles() {
		fmt.Fprintf(&b, "--- %s\n%s", p, d.files[p].Print())
	}
	return b.String()
}
