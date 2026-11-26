package check

import (
	"fmt"
	"slices"
	"strings"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
)

// Sample is a graph that meets every constraint of docs/design.md §5.3: the
// design layer and the code layer, not yet derived.
//
//uc:realizes check.Sample
type Sample struct {
	Fragment graph.Fragment
}

// The test handle of a Go predicate, as the backend writes its type.
const handle = "t: go:pgregory.net/rapid.TB"

// world builds a conforming graph and keeps what each part of it is, so that
// a breakage can find its sites. Components are k0, k1, … with k0 and k1 at
// the top; each owns the directory src/<its path> and one file there, with a
// detail function helper. A component uses only components declared before
// it, so k0 uses none and may not depend on k1.
type world struct {
	t          *rapid.T
	f          graph.Fragment
	line       int
	comps      []*comp
	types      []*dtype
	ops        []*dop
	checkables []*checkable
	steps      []*step
	external   bool
	client     *comp // the component that holds the external system's client
}

type comp struct {
	id, dir, pkg, file, helper string
	parent                     *comp
	uses                       []string
}

type dtype struct {
	id, form, code     string
	home               *comp
	fields, codeFields []string
	ops                []*dop
}

// dop is a design operation and its realizer: a method, an interface's
// abstract method, or a function.
type dop struct {
	id, signature, code, codeSignature, kind string
	home                                     *comp // nil for the external system's
	owner                                    *dtype
}

type checkable struct {
	id, pred   string
	home       *comp
	symbols    []string
	generators map[string]string
}

// step is a flow step: what it calls or emits, who sends it, the function
// that realizes its operation, and the functions of its context that reach it.
type step struct {
	id, form, op, sender, traced string
	context                      []string
	event                        string
	senderCode                   []string
	home                         *comp
}

func (w *world) pos(path string) diag.Position {
	w.line++
	return diag.Position{Path: path, Line: w.line, Column: 1}
}

func (w *world) node(id string, layer graph.Layer, kind graph.NodeKind, form, at string, attrs map[string][]string) string {
	source := graph.SourceDeclared
	if layer == graph.LayerCode {
		source = graph.SourceExtracted
	}
	w.f.Nodes = append(w.f.Nodes, graph.Node{Id: id, Kind: kind, Form: form, Layer: layer, Source: source, Origin: w.pos(at), Attrs: attrs})
	return id
}

func (w *world) edge(kind graph.EdgeKind, from, to string, source graph.Source, attrs map[string][]string) {
	for _, e := range w.f.Edges {
		if e.Kind == kind && e.From == from && e.To == to {
			return
		}
	}
	w.f.Edges = append(w.f.Edges, graph.Edge{Kind: kind, From: from, To: to, Source: source, Origin: w.pos("code"), Attrs: attrs})
}

func (w *world) declared(kind graph.EdgeKind, from, to string) {
	w.edge(kind, from, to, graph.SourceDeclared, nil)
}

func (w *world) extracted(kind graph.EdgeKind, from, to string) {
	w.edge(kind, from, to, graph.SourceExtracted, nil)
}

// call records a call in the code, and the import it needs across packages.
func (w *world) call(from, to string) {
	w.edge(graph.EdgeKindCalls, from, to, graph.SourceExtracted, map[string][]string{"sites": {"src/x.go:1:1"}})
	if pkgOf(from) != pkgOf(to) {
		w.extracted(graph.EdgeKindImports, w.fileOfCode(from), pkgOf(to))
	}
}

// pkgOf is the package of a code identity go:<package>.<name>[.<method>].
func pkgOf(id string) string {
	slash := strings.LastIndex(id, "/")
	dot := strings.Index(id[slash:], ".")
	return id[:slash+dot]
}

func (w *world) fileOfCode(id string) string {
	for _, c := range w.comps {
		if c.pkg == pkgOf(id) {
			return c.file
		}
	}
	return ""
}

func (w *world) designFile(c *comp) string {
	if c == nil {
		return "design/system.uc"
	}
	return "design/" + strings.ReplaceAll(c.id, ".", "/") + ".uc"
}

func upper(s string) string { return strings.ToUpper(s[:1]) + s[1:] }

// failure is the error type that operations raise: k0's third type.
const failure = "k0.T2"

// typePair draws a type as the design writes it and as the Go backend may
// write it: an object type X is passed by pointer, so the code may write
// optional<X> for X; object is an object type of the design.
func (w *world) typePair(object string, depth int) (string, string) {
	options := 4
	if depth > 0 {
		options = 6
	}
	switch rapid.IntRange(0, options-1).Draw(w.t, "type") {
	case 0:
		b := rapid.SampledFrom([]string{"int", "string", "bool", "optional<int>", "list<string>"}).Draw(w.t, "builtin")
		return b, b
	case 1:
		return object, object
	case 2:
		return object, "optional<" + object + ">"
	case 3:
		return "optional<" + object + ">", "optional<" + object + ">"
	case 4:
		d, c := w.typePair(object, depth-1)
		return "list<" + d + ">", "list<" + c + ">"
	}
	d, c := w.typePair(object, depth-1)
	return "map<string, " + d + ">", "map<string, " + c + ">"
}

// signaturePair draws an operation's signature as the design writes it and
// as the Go backend writes it: errors become ` raises error`.
func (w *world) signaturePair(object string) (string, string) {
	var design, code []string
	for i := range rapid.IntRange(0, 2).Draw(w.t, "params") {
		d, c := w.typePair(object, 1)
		design, code = append(design, fmt.Sprintf("p%d: %s", i, d)), append(code, fmt.Sprintf("p%d: %s", i, c))
	}
	ds, cs := "("+strings.Join(design, ", ")+")", "("+strings.Join(code, ", ")+")"
	if rapid.Bool().Draw(w.t, "result") {
		d, c := w.typePair(object, 1)
		ds, cs = ds+" -> "+d, cs+" -> "+c
	}
	if rapid.Bool().Draw(w.t, "raises") {
		ds, cs = ds+" raises "+failure, cs+" raises error"
	}
	return ds, cs
}

// objectOf is the object type that a component's signatures and fields may
// name: its first type.
func objectOf(c *comp) string {
	if c == nil {
		return "k0.T0"
	}
	return c.id + ".T0"
}

// genSample draws a conforming graph.
//
//uc:generates check.accepts-conforming-graph.sample
func genSample() *rapid.Generator[Sample] {
	return rapid.Custom(func(t *rapid.T) Sample { return Sample{Fragment: newWorld(t).f} })
}

func newWorld(t *rapid.T) *world {
	w := &world{t: t}
	w.node("sys", graph.LayerDesign, graph.NodeKindContainer, "system", "design/system.uc", nil)
	w.node("user", graph.LayerDesign, graph.NodeKindActor, "person", "design/system.uc", nil)
	w.declared(graph.EdgeKindContains, "sys", "user")
	w.components()
	for _, c := range w.comps {
		w.componentCode(c)
		w.componentTypes(c)
	}
	w.externalSystem()
	w.relations()
	for _, c := range w.comps {
		w.checkables_(c)
	}
	return w
}

func (w *world) components() {
	for i := range rapid.IntRange(2, 4).Draw(w.t, "components") {
		c := &comp{id: fmt.Sprintf("k%d", i)}
		if i >= 2 {
			if p := rapid.IntRange(-1, i-1).Draw(w.t, "parent"); p >= 0 {
				c.parent = w.comps[p]
				c.id = c.parent.id + "." + c.id
			}
		}
		c.dir = "src/" + strings.ReplaceAll(c.id, ".", "/")
		for _, earlier := range w.comps {
			if rapid.IntRange(0, 2).Draw(w.t, "uses") == 0 {
				c.uses = append(c.uses, earlier.id)
			}
		}
		w.node(c.id, graph.LayerDesign, graph.NodeKindContainer, "component", w.designFile(c),
			map[string][]string{"focus": {"core"}, "paths": {c.dir + "/**"}})
		if c.parent == nil {
			w.declared(graph.EdgeKindContains, "sys", c.id)
		} else {
			w.declared(graph.EdgeKindContains, c.parent.id, c.id)
		}
		for _, u := range c.uses {
			w.declared(graph.EdgeKindUses, c.id, u)
		}
		w.comps = append(w.comps, c)
	}
}

func (w *world) componentCode(c *comp) {
	c.pkg = "go:m/" + c.dir
	c.file = "file:" + c.dir + "/code.go"
	w.node(c.pkg, graph.LayerCode, graph.NodeKindContainer, "package", c.dir, map[string][]string{"language": {"go"}, "path": {"m/" + c.dir}})
	w.node(c.file, graph.LayerCode, graph.NodeKindContainer, "file", c.dir+"/code.go", map[string][]string{"language": {"go"}, "path": {c.dir + "/code.go"}})
	w.extracted(graph.EdgeKindContains, c.pkg, c.file)
	c.helper = w.function(c.file, c.pkg+".helper", "helper", "()", "func")
}

func (w *world) function(parent, id, name, signature, form string) string {
	w.node(id, graph.LayerCode, graph.NodeKindOp, form, strings.TrimPrefix(w.fileOfCode(id), "file:"),
		map[string][]string{"name": {name}, "signature": {signature}, "testKind": {"none"}, "stub": {"false"}})
	w.extracted(graph.EdgeKindContains, parent, id)
	return id
}

// componentTypes declares a component's types and operations with the code
// that realizes them: an object with a run operation first, then maybe an
// event (always in k0), a record, an interface or an error; then maybe a load
// operation of the component itself.
func (w *world) componentTypes(c *comp) {
	forms := []string{"object"}
	switch {
	case c.id == "k0":
		forms = append(forms, "event", "error")
	case rapid.Bool().Draw(w.t, "second"):
		forms = append(forms, rapid.SampledFrom([]string{"event", "record", "error", "object"}).Draw(w.t, "form"))
	}
	if rapid.Bool().Draw(w.t, "interface") {
		forms = append(forms, "interface")
	}
	for j, form := range forms {
		var ops []string
		switch form {
		case "object":
			ops = append([]string{"run"}, w.subset("ops", []string{"stop"})...)
		case "interface":
			ops = []string{"run"}
		}
		w.designType(c, fmt.Sprintf("%s.T%d", c.id, j), form, c.pkg+fmt.Sprintf(".T%d", j), c.file, ops)
	}
	if rapid.Bool().Draw(w.t, "load") {
		w.directOp(c, c.id+".load", c.pkg+".Load", c.file, "load")
	}
}

func (w *world) subset(label string, options []string) []string {
	var out []string
	for _, o := range options {
		if rapid.Bool().Draw(w.t, label) {
			out = append(out, o)
		}
	}
	return out
}

func (w *world) designType(c *comp, id, form, code, file string, ops []string) *dtype {
	t := &dtype{id: id, form: form, code: code, home: c}
	if form != "interface" {
		for k := range rapid.IntRange(0, 2).Draw(w.t, "fields") {
			d, c := w.typePair(objectOf(c), 1)
			t.fields, t.codeFields = append(t.fields, fmt.Sprintf("f%d: %s", k, d)), append(t.codeFields, fmt.Sprintf("f%d: %s", k, c))
		}
	}
	parent := "x"
	if c != nil {
		parent = c.id
	}
	w.node(id, graph.LayerDesign, graph.NodeKindType, form, w.designFile(c), map[string][]string{"fields": t.fields})
	w.declared(graph.EdgeKindContains, parent, id)
	codeForm := "struct"
	if form == "interface" {
		codeForm = "interface"
	}
	w.node(code, graph.LayerCode, graph.NodeKindType, codeForm, strings.TrimPrefix(file, "file:"),
		map[string][]string{"name": {name(id)}, "fields": t.codeFields})
	w.extracted(graph.EdgeKindContains, file, code)
	w.declared(graph.EdgeKindRealizes, code, id)
	for _, op := range ops {
		o := &dop{id: id + "." + op, home: c, owner: t}
		o.signature, o.codeSignature = w.signaturePair(objectOf(c))
		w.node(o.id, graph.LayerDesign, graph.NodeKindOp, "op", w.designFile(c), map[string][]string{"signature": {o.signature}})
		w.declared(graph.EdgeKindContains, id, o.id)
		o.kind = methodForm(form)
		o.code = w.function(code, code+"."+upper(op), op, o.codeSignature, o.kind)
		w.edge(graph.EdgeKindRealizes, o.code, o.id, graph.SourceDerived, nil)
		t.ops = append(t.ops, o)
		w.ops = append(w.ops, o)
	}
	if rapid.Bool().Draw(w.t, "detailMethod") {
		// A method outside the design: its candidate names no operation.
		m := w.function(code, code+".Other", "other", "()", methodForm(form))
		w.edge(graph.EdgeKindRealizes, m, id+".other", graph.SourceDerived, nil)
	}
	w.types = append(w.types, t)
	return t
}

// methodForm is the form of the code methods of a type of the given design form.
func methodForm(form string) string {
	if form == "interface" {
		return "abstract"
	}
	return "method"
}

func (w *world) directOp(c *comp, id, code, file, opName string) *dop {
	o := &dop{id: id, home: c, kind: "func"}
	o.signature, o.codeSignature = w.signaturePair(objectOf(c))
	parent := "x"
	if c != nil {
		parent = c.id
	}
	w.node(id, graph.LayerDesign, graph.NodeKindOp, "op", w.designFile(c), map[string][]string{"signature": {o.signature}})
	w.declared(graph.EdgeKindContains, parent, id)
	o.code = w.function(file, code, opName, o.codeSignature, "func")
	w.declared(graph.EdgeKindRealizes, o.code, id)
	w.ops = append(w.ops, o)
	return o
}

// externalSystem maybe declares the external system x, with a record Api and
// an operation charge, which a client in a component that uses x realizes.
func (w *world) externalSystem() {
	if !rapid.Bool().Draw(w.t, "external") {
		return
	}
	w.external = true
	w.node("x", graph.LayerDesign, graph.NodeKindActor, "external", "design/system.uc", nil)
	w.declared(graph.EdgeKindContains, "sys", "x")
	w.client = rapid.SampledFrom(w.comps).Draw(w.t, "client")
	w.client.uses = append(w.client.uses, "x")
	w.declared(graph.EdgeKindUses, w.client.id, "x")
	w.designType(nil, "x.Api", "record", w.client.pkg+".Api", w.client.file, nil)
	w.directOp(nil, "x.charge", w.client.pkg+".Charge", w.client.file, "charge")
}

// relations maybe declares an implements or extends between two types, and
// the same between the code types that realize them.
func (w *world) relations() {
	for range rapid.IntRange(0, 2).Draw(w.t, "relations") {
		a, b := rapid.SampledFrom(w.types).Draw(w.t, "from"), rapid.SampledFrom(w.types).Draw(w.t, "to")
		kind := rapid.SampledFrom([]graph.EdgeKind{graph.EdgeKindImplements, graph.EdgeKindExtends}).Draw(w.t, "relation")
		if a != b {
			w.declared(kind, a.id, b.id)
			w.extracted(kind, a.code, b.code)
		}
	}
}

// checkables_ declares a component's property and flow, each with its
// predicate and the generators of its generated symbols; k0's property
// always has a generated symbol.
func (w *world) checkables_(c *comp) {
	d, code := w.typePair(objectOf(c), 1)
	symbols := [][2]string{{"v: " + d, "v: " + code}}
	if c.id == "k0" || rapid.Bool().Draw(w.t, "generated") {
		symbols = append(symbols, [2]string{"s: string generated", "s: string"})
	}
	w.checkable(c, c.id+".p", graph.NodeKindProperty, "invariant", symbols)
	var flowSymbols [][2]string
	if rapid.Bool().Draw(w.t, "flowSymbol") {
		flowSymbols = append(flowSymbols, [2]string{"n: int", "n: int"})
	}
	flow := w.checkable(c, c.id+".f", graph.NodeKindFlow, "flow", flowSymbols)
	n := 0
	if c.id == "k0" {
		w.callStep(c, flow.id, flow.id, &n, "call", c.id, c.helper, w.types[0].ops[0], 0)
		w.emitStep(c, flow.id, flow.id, &n)
	}
	for range rapid.IntRange(0, 3).Draw(w.t, "steps") {
		w.topStep(c, flow.id, flow.id, &n)
	}
}

// checkable declares a property or a flow with its symbols, each as the
// design writes it and as its predicate's parameter.
func (w *world) checkable(c *comp, id string, kind graph.NodeKind, form string, symbols [][2]string) *checkable {
	k := &checkable{id: id, home: c, generators: map[string]string{}}
	for _, s := range symbols {
		k.symbols = append(k.symbols, s[0])
	}
	w.node(id, graph.LayerDesign, kind, form, w.designFile(c), map[string][]string{"symbols": k.symbols})
	w.declared(graph.EdgeKindContains, c.id, id)
	params := []string{handle}
	for _, s := range symbols {
		params = append(params, s[1])
		if strings.HasSuffix(s[0], " generated") {
			n, _, _ := strings.Cut(s[1], ": ")
			gen := w.function(c.file, c.pkg+".gen"+upper(name(id))+upper(n), "gen", "()", "func")
			w.edge(graph.EdgeKindGenerates, gen, id, graph.SourceDeclared, map[string][]string{"symbols": {n}})
			k.generators[n] = gen
		}
	}
	k.pred = w.function(c.file, c.pkg+".check"+upper(name(id)), "check", "("+strings.Join(params, ", ")+")", "func")
	w.declared(graph.EdgeKindChecks, k.pred, id)
	w.checkables = append(w.checkables, k)
	return k
}

// allowed lists the components whose operations the code of c may call
// (rule 13), and the external system when c holds its client.
func (w *world) allowed(c *comp) []string {
	out := []string{c.id}
	for _, o := range w.comps {
		if o.parent == c {
			out = append(out, o.id)
		}
	}
	for a := c; a != nil; a = a.parent {
		out = append(out, a.uses...)
	}
	if w.client != c {
		out = slices.DeleteFunc(out, func(id string) bool { return id == "x" })
	}
	return out
}

// callable lists the operations that a flow step sent from c's code may
// call: those of allowed components and systems.
func (w *world) callable(c *comp) []*dop {
	allowed := w.allowed(c)
	var out []*dop
	for _, o := range w.ops {
		home := "x"
		if o.home != nil {
			home = o.home.id
		}
		if slices.Contains(allowed, home) {
			out = append(out, o)
		}
	}
	return out
}

func (w *world) newStep(flow, parent, form string, n *int) string {
	*n++
	id := fmt.Sprintf("%s#%d", flow, *n)
	w.node(id, graph.LayerDesign, graph.NodeKindStep, form, "design/flow.uc", map[string][]string{"index": {fmt.Sprint(*n)}})
	w.declared(graph.EdgeKindContains, parent, id)
	return id
}

// topStep adds an outermost step: an alt around outermost calls, an emit,
// or a call or send from the person, the component or one of its objects.
func (w *world) topStep(c *comp, flow, parent string, n *int) {
	switch rapid.IntRange(0, 3).Draw(w.t, "topStep") {
	case 0:
		alt := w.newStep(flow, parent, "alt", n)
		for range rapid.IntRange(1, 2).Draw(w.t, "branchSteps") {
			w.topStep(c, flow, alt, n)
		}
	case 1:
		w.emitStep(c, flow, parent, n)
	default:
		ops := w.callable(c)
		senders := []string{"user", c.id}
		for _, t := range w.types {
			if t.home == c && t.form == "object" {
				senders = append(senders, t.id)
			}
		}
		sender := rapid.SampledFrom(senders).Draw(w.t, "sender")
		var from string
		switch {
		case sender == c.id:
			from = c.helper
		case sender != "user":
			for _, t := range w.types {
				if t.id == sender {
					from = t.ops[0].code
				}
			}
		}
		form := rapid.SampledFrom([]string{"call", "send"}).Draw(w.t, "callForm")
		w.callStep(c, flow, parent, n, form, sender, from, rapid.SampledFrom(ops).Draw(w.t, "op"), 2)
	}
}

// callStep adds a call or send step to op, sent by sender; from is the code
// function of its context that calls op's function, or "" for the person.
// A call may nest steps that its receiver sends, unless it calls an
// interface: an abstract method calls nothing, so no step under it could be
// reached.
func (w *world) callStep(c *comp, flow, parent string, n *int, form, sender, from string, op *dop, depth int) {
	id := w.newStep(flow, parent, form, n)
	w.declared(graph.EdgeKindFrom, id, sender)
	w.declared(graph.EdgeKindCalls, id, op.id)
	s := &step{id: id, form: form, op: op.id, sender: sender, traced: op.code, home: c}
	if from != "" {
		w.call(from, op.code)
		s.context = []string{from}
	}
	w.steps = append(w.steps, s)
	if form != "call" || op.home == nil || op.kind == "abstract" || depth == 0 {
		return
	}
	receiver := op.home.id
	if op.owner != nil {
		receiver = op.owner.id
	}
	for range rapid.IntRange(0, 2).Draw(w.t, "subSteps") {
		w.callStep(op.home, flow, id, n, "call", receiver, op.code, rapid.SampledFrom(w.callable(op.home)).Draw(w.t, "subOp"), depth-1)
	}
}

// emitStep adds an emit step of an event that the component may use, sent
// by the component or one of its objects, whose code refers to the event's.
func (w *world) emitStep(c *comp, flow, parent string, n *int) {
	allowed := w.allowed(c)
	var events []*dtype
	for _, t := range w.types {
		if t.form == "event" && t.home != nil && slices.Contains(allowed, t.home.id) {
			events = append(events, t)
		}
	}
	if len(events) == 0 {
		return
	}
	event := rapid.SampledFrom(events).Draw(w.t, "event")
	id := w.newStep(flow, parent, "emit", n)
	s := &step{id: id, form: "emit", sender: c.id, event: event.code, home: c, senderCode: []string{c.helper}}
	if rapid.Bool().Draw(w.t, "objectSender") {
		object := w.types[slices.IndexFunc(w.types, func(t *dtype) bool { return t.home == c })]
		s.sender, s.senderCode = object.id, []string{object.code}
	}
	w.declared(graph.EdgeKindFrom, id, s.sender)
	w.declared(graph.EdgeKindEmits, id, event.id)
	w.extracted(graph.EdgeKindRefers, s.senderCode[0], event.code)
	w.steps = append(w.steps, s)
}
