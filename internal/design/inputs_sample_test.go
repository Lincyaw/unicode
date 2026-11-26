package design

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/syntax"
)

// This file draws the flows, properties and corpus files of a valid design:
// symbols of types that can be generated, limits at paths that resolve, steps
// that follow the rules of flows, and cases that fit their symbols.

// callableOps lists the operations whose parameters can all be generated.
func (d *designer) callableOps() []*gNode {
	var out []*gNode
	for _, op := range d.ops {
		if !slices.ContainsFunc(op.params, func(p gField) bool { return !d.simpleType(p.t) }) {
			out = append(out, op)
		}
	}
	return out
}

// callableTypes lists the types with operations, each of them callable.
func (d *designer) callableTypes() []*gNode {
	callable := d.callableOps()
	var out []*gNode
	for _, t := range d.types {
		if len(t.ops) > 0 && !slices.ContainsFunc(t.ops, func(op *gNode) bool { return !slices.Contains(callable, op) }) {
			out = append(out, t)
		}
	}
	return out
}

// symbolType draws a type whose values can be generated (docs/model.md §10 rule 23).
func (d *designer) symbolType(depth int) *gType {
	var simple, enums, trees []*gNode
	for _, t := range d.types {
		switch {
		case t.recursive:
			trees = append(trees, t)
		case t.simple && (t.form != "enum" || len(t.values) > 0):
			simple = append(simple, t)
			if t.form == "enum" {
				enums = append(enums, t)
			}
		}
	}
	kind := count(d, "symbolType", 0, 7)
	if depth <= 0 {
		kind %= 3
	}
	switch {
	case kind == 1 && len(simple) > 0:
		return &gType{named: pick(d, "simple", simple)}
	case kind == 2 && len(trees) > 0:
		return &gType{named: pick(d, "tree", trees)}
	case kind == 3 || kind == 4:
		return &gType{kind: pick(d, "elem", []string{"list", "set", "optional"}), elems: []*gType{d.symbolType(depth - 1)}}
	case kind == 5:
		key := &gType{builtin: pick(d, "key", []string{"string", "int"})}
		if len(enums) > 0 && rapid.Bool().Draw(d.t, "enumKey") {
			key = &gType{named: pick(d, "enumKey", enums)}
		}
		return &gType{kind: "map", elems: []*gType{key, d.symbolType(depth - 1)}}
	case kind == 6:
		if call := d.callType(); call != nil {
			return call
		}
	}
	return &gType{builtin: pick(d, "builtin", builtins)}
}

// callType draws call<…> of distinct types or of distinct operations, or nil.
func (d *designer) callType() *gType {
	targets := d.callableTypes()
	if rapid.Bool().Draw(d.t, "callOps") {
		targets = d.callableOps()
	}
	if len(targets) == 0 {
		return nil
	}
	t := &gType{kind: "call"}
	for range count(d, "callTargets", 1, 2) {
		if target := pick(d, "callTarget", targets); !slices.Contains(t.targets, target) {
			t.targets = append(t.targets, target)
		}
	}
	return t
}

// callOps lists the operations a value of call<…> may call.
func callOps(t *gType) []*gNode {
	var out []*gNode
	for _, target := range t.targets {
		if target.kind == "op" {
			out = append(out, target)
		} else {
			out = append(out, target.ops...)
		}
	}
	return out
}

// elemOf is what a limit passes through to: the element of a list, set or
// optional, the value of a map.
func elemOf(t *gType) *gType {
	switch t.kind {
	case "list", "set", "optional":
		return t.elems[0]
	case "map":
		return t.elems[1]
	}
	return nil
}

func fits(op string, t *gType) bool {
	switch op {
	case "in":
		return t.builtin == "int" || t.builtin == "float"
	case "size":
		return t.kind == "list" || t.kind == "set" || t.kind == "map" || t.builtin == "string" || t.builtin == "bytes"
	}
	return t.named != nil && t.named.form == "enum"
}

// site is where a limit may stand: a path, a kind of limit, and the type it applies to.
type site struct {
	path []string
	op   string
	at   *gType
}

// sites lists the places of t, at path, where each kind of limit applies:
// the first type along the elements that fits it; then the places of the
// fields of records and of the parameters of calls (docs/model.md §10 rule 25).
func sites(t *gType, path []string, depth int) []site {
	var out []site
	for _, op := range []string{"in", "size", "subset"} {
		for x := t; x != nil; x = elemOf(x) {
			if fits(op, x) {
				out = append(out, site{slices.Clone(path), op, x})
				break
			}
		}
	}
	if depth >= 3 {
		return out
	}
	base := t
	for elemOf(base) != nil {
		base = elemOf(base)
	}
	switch {
	case base.named != nil && !base.named.recursive:
		for _, f := range base.named.fields {
			out = append(out, sites(f.t, append(slices.Clone(path), f.name), depth+1)...)
		}
	case base.kind == "call":
		for _, op := range callOps(base) {
			for _, p := range op.params {
				out = append(out, sites(p.t, append(slices.Clone(path), op.name, p.name), depth+1)...)
			}
		}
	}
	return out
}

// inputs draws the symbols and limits of a flow or a property n; a
// skeleton's come first: an int limited to 0..10 and a generated string.
func (d *designer) inputs(n, c *gNode, skeleton bool) []any {
	var lines []any
	add := func(name string, t *gType, generated bool) {
		s := &syntax.Symbol{Name: syntax.Lower(name), Type: d.expr(t, c)}
		if generated {
			domain := d.summary("domain")
			s.Domain = &domain
		}
		n.symbols = append(n.symbols, gSymbol{name, t, generated, s})
		lines = append(lines, s)
		d.refersTo(n, t)
	}
	if skeleton {
		add(d.lower("symbol"), &gType{builtin: "int"}, false)
		if n.kind == "property" {
			add(d.lower("symbol"), &gType{named: d.skel.record}, false)
		}
		add(d.lower("symbol"), &gType{builtin: "string"}, true)
	}
	for range count(d, "symbols", 0, 2) {
		generated := rapid.IntRange(0, 3).Draw(d.t, "generated") == 0
		add(d.lower("symbol"), d.symbolType(2), generated)
	}
	if skeleton {
		d.addLimit(n, &lines, site{[]string{n.symbols[0].name}, "in", n.symbols[0].t}, 0, 10)
	}
	var all []site
	for _, s := range n.symbols {
		if !s.generated && (!skeleton || s.name != n.symbols[0].name) {
			all = append(all, sites(s.t, []string{s.name}, 0)...)
		}
	}
	var chosen []int
	for range count(d, "limits", 0, 3) {
		if len(all) == 0 {
			break
		}
		if i := rapid.IntRange(0, len(all)-1).Draw(d.t, "site"); !slices.Contains(chosen, i) {
			chosen = append(chosen, i)
			low := rapid.IntRange(-20, 20).Draw(d.t, "low")
			if all[i].op == "size" {
				low = rapid.IntRange(0, 2).Draw(d.t, "sizeLow")
			}
			d.addLimit(n, &lines, all[i], low, low+rapid.IntRange(0, 3).Draw(d.t, "span"))
		}
	}
	return lines
}

// maxKeys is how many distinct keys a map of key type k can have in a case.
func maxKeys(k *gType) int {
	if k.named != nil {
		return len(k.named.values)
	}
	return len(stringKeys)
}

func (d *designer) addLimit(n *gNode, lines *[]any, s site, low, high int) {
	lim := gLimit{path: s.path, op: s.op, low: low, high: high, at: s.at}
	switch s.op {
	case "subset":
		values := s.at.named.values
		for _, v := range values {
			if rapid.Bool().Draw(d.t, "member") || len(lim.members) == 0 && v == values[len(values)-1] {
				lim.members = append(lim.members, v)
			}
		}
		lim.decl = &syntax.SubsetLimit{Path: lowers(s.path), Members: lowers(lim.members)}
	default:
		if s.op == "size" && s.at.kind == "map" {
			lim.high = min(lim.high, maxKeys(s.at.elems[0]))
			lim.low = min(lim.low, lim.high)
		}
		lim.decl = &syntax.IntervalLimit{Path: lowers(s.path), Op: s.op,
			Low: syntax.Number(strconv.Itoa(lim.low)), High: syntax.Number(strconv.Itoa(lim.high))}
	}
	n.limits = append(n.limits, lim)
	*lines = append(*lines, lim.decl)
}

func lowers(ss []string) []syntax.Lower {
	out := make([]syntax.Lower, len(ss))
	for i, s := range ss {
		out[i] = syntax.Lower(s)
	}
	return out
}

var (
	stringKeys = []string{"a", "b", "中", "x y", `q"`, ""}
	times      = []string{"2026-11-26T08:00:00Z", "1999-01-01T00:00:00+08:00", "2026-02-28T23:59:59.5Z"}
	durations  = []string{"1h30m", "5s", "250ms", "-2m", "0s"}
	textRunes  = []rune("ab 中文\"\\\n")
)

// bounds returns the bounds of the limit of kind op that applies to t at path, or the defaults.
func bounds(lims []gLimit, t *gType, path []string, op string, low, high int) (int, int) {
	for _, l := range lims {
		if l.at == t && l.op == op && slices.Equal(l.path, path) {
			return l.low, l.high
		}
	}
	return low, high
}

// value draws a value of t at path that satisfies the limits (docs/model.md §10 rule 26).
func (d *designer) value(t *gType, path []string, lims []gLimit, depth int) syntax.Value {
	switch t.builtin {
	case "bool":
		return &syntax.BoolValue{Value: syntax.Boolean(rapid.Bool().Draw(d.t, "bool"))}
	case "int":
		low, high := bounds(lims, t, path, "in", -50, 50)
		return &syntax.NumberValue{Text: syntax.Number(strconv.Itoa(rapid.IntRange(low, high).Draw(d.t, "int")))}
	case "float":
		low, high := bounds(lims, t, path, "in", -50, 50)
		n := rapid.IntRange(low, high).Draw(d.t, "float")
		if n < high && rapid.Bool().Draw(d.t, "half") {
			return &syntax.NumberValue{Text: syntax.Number(strconv.FormatFloat(float64(n)+0.5, 'f', 1, 64))}
		}
		return &syntax.NumberValue{Text: syntax.Number(strconv.Itoa(n))}
	case "string":
		low, high := bounds(lims, t, path, "size", 0, 6)
		return &syntax.StringValue{Text: syntax.Text(rapid.StringOfN(rapid.SampledFrom(textRunes), low, high, -1).Draw(d.t, "string"))}
	case "bytes":
		low, high := bounds(lims, t, path, "size", 0, 6)
		return &syntax.StringValue{Text: syntax.Text(rapid.StringOfN(rapid.SampledFrom([]rune("ab \\")), low, high, high).Draw(d.t, "bytes"))}
	case "time":
		return &syntax.StringValue{Text: syntax.Text(pick(d, "time", times))}
	case "duration":
		return &syntax.StringValue{Text: syntax.Text(pick(d, "duration", durations))}
	}
	switch t.kind {
	case "list", "set":
		low, high := bounds(lims, t, path, "size", 0, 3)
		if depth > 2 {
			high = min(high, max(low, 1))
		}
		v := &syntax.ListValue{}
		for range rapid.IntRange(low, high).Draw(d.t, "items") {
			v.Items = append(v.Items, d.value(t.elems[0], path, lims, depth+1))
		}
		return v
	case "optional":
		if depth > 2 || rapid.IntRange(0, 2).Draw(d.t, "none?") == 0 {
			return &syntax.NoneValue{None: true}
		}
		return d.value(t.elems[0], path, lims, depth+1)
	case "map":
		return d.mapValue(t, path, lims, depth)
	case "call":
		op := pick(d, "callOp", callOps(t))
		v := &syntax.CallValue{Op: syntax.Lower(op.name)}
		for _, i := range rapid.Permutation(indexes(len(op.params))).Draw(d.t, "argOrder") {
			p := op.params[i]
			v.Args = append(v.Args, syntax.Arg{Name: syntax.Lower(p.name), Value: d.value(p.t, append(slices.Clone(path), op.name, p.name), lims, depth+1)})
		}
		return v
	}
	n := t.named
	if n.form == "enum" {
		members := n.values
		for _, l := range lims {
			if l.at == t && l.op == "subset" && slices.Equal(l.path, path) {
				members = l.members
			}
		}
		return &syntax.NameValue{Name: syntax.Lower(pick(d, "enumValue", members))}
	}
	v := &syntax.BraceValue{}
	for _, i := range rapid.Permutation(indexes(len(n.fields))).Draw(d.t, "fieldOrder") {
		f := n.fields[i]
		name := syntax.Lower(f.name)
		var fv syntax.Value
		switch {
		case n.recursive && f.t.kind == "list" && depth > 1:
			fv = &syntax.ListValue{}
		case n.recursive && f.t.kind == "optional" && depth > 1:
			fv = &syntax.NoneValue{None: true}
		default:
			fv = d.value(f.t, append(slices.Clone(path), f.name), lims, depth+1)
		}
		v.Entries = append(v.Entries, syntax.Entry{Key: syntax.Key{Name: &name}, Value: fv})
	}
	return v
}

func indexes(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

func (d *designer) mapValue(t *gType, path []string, lims []gLimit, depth int) syntax.Value {
	key := t.elems[0]
	low, high := bounds(lims, t, path, "size", 0, 3)
	high = min(high, maxKeys(key))
	v := &syntax.BraceValue{}
	var keys []syntax.Key
	switch {
	case key.named != nil:
		for _, name := range key.named.values {
			name := syntax.Lower(name)
			keys = append(keys, syntax.Key{Name: &name})
		}
	case key.builtin == "int":
		for i := range 6 {
			n := syntax.Number(strconv.Itoa(i*7 - 9))
			keys = append(keys, syntax.Key{Number: &n})
		}
	default:
		for _, s := range stringKeys {
			s := syntax.Text(s)
			keys = append(keys, syntax.Key{String: &s})
		}
	}
	order := rapid.Permutation(keys).Draw(d.t, "keys")
	for _, k := range order[:rapid.IntRange(min(low, len(order)), min(high, len(order))).Draw(d.t, "entries")] {
		v.Entries = append(v.Entries, syntax.Entry{Key: k, Value: d.value(t.elems[1], path, lims, depth+1)})
	}
	return v
}

// assignments draws a value for every symbol of n, in a random order.
func (d *designer) assignments(n *gNode) []syntax.CaseLine {
	var lines []syntax.CaseLine
	for _, i := range rapid.Permutation(indexes(len(n.symbols))).Draw(d.t, "symbolOrder") {
		s := n.symbols[i]
		lines = append(lines, &syntax.Assignment{Symbol: syntax.Lower(s.name), Value: d.value(s.t, []string{s.name}, n.limits, 0)})
	}
	return lines
}

// cases draws the declared cases of n.
func (d *designer) cases(n *gNode, at int) []*syntax.CaseDecl {
	var out []*syntax.CaseDecl
	for range count(d, "cases", at, 2) {
		c := &syntax.CaseDecl{Name: syntax.Kebab(d.kebab("case")), Summary: d.optSummary("caseSummary"), Lines: d.assignments(n)}
		d.nodes = append(d.nodes, n.id+"."+string(c.Name))
		d.edges = append(d.edges, "contains "+n.id+" "+n.id+"."+string(c.Name))
		out = append(out, c)
	}
	return out
}

func (d *designer) genFlows() {
	for _, c := range d.containers {
		n := count(d, "flows", 0, 2)
		if c == d.skel.top {
			d.skel.flow = d.newFlow(c, true)
		}
		for range n {
			d.newFlow(c, false)
		}
	}
}

func (d *designer) newFlow(c *gNode, skeleton bool) *gNode {
	fl := d.declare(c, &gNode{name: d.kebab("flow"), kind: "flow", form: "flow", file: c.file})
	d.flows = append(d.flows, fl)
	decl := &syntax.FlowDecl{Name: syntax.Kebab(fl.name), Summary: d.summary("summary")}
	fl.flowDecl = decl
	for _, line := range d.inputs(fl, c, skeleton) {
		decl.Lines = append(decl.Lines, line.(syntax.FlowLine))
	}
	g := &flowGen{d: d, flow: fl, scope: c, budget: 12}
	if skeleton {
		for _, s := range g.skeleton() {
			decl.Lines = append(decl.Lines, s.(syntax.FlowLine))
		}
	}
	for _, s := range g.list(fl.id, nil, 3) {
		decl.Lines = append(decl.Lines, s.(syntax.FlowLine))
	}
	for _, cd := range d.cases(fl, 0) {
		decl.Lines = append(decl.Lines, cd)
	}
	d.componentFile(c).Decls = append(d.componentFile(c).Decls, decl)
	return fl
}

func (d *designer) genProperties() {
	forms := []string{"invariant", "postcondition", "relation", "transition"}
	var abouts []*gNode
	abouts = append(abouts, d.containers...)
	abouts = append(abouts, d.types...)
	abouts = append(abouts, d.ops...)
	abouts = append(abouts, d.flows...)
	for _, c := range d.containers {
		n := count(d, "properties", 0, 2)
		if c == d.skel.top {
			n++
		}
		for i := range n {
			skeleton := c == d.skel.top && i == 0
			p := d.declare(c, &gNode{name: d.kebab("property"), kind: "property", form: pick(d, "form", forms), file: c.file})
			d.props = append(d.props, p)
			decl := &syntax.PropertyDecl{Name: syntax.Kebab(p.name), Form: p.form, Summary: d.summary("summary")}
			p.propDecl = decl
			about := &syntax.About{}
			var targets []*gNode
			if skeleton {
				targets = append(targets, d.skel.service)
			}
			for range count(d, "abouts", 1, 3) {
				if target := pick(d, "about", abouts); !slices.Contains(targets, target) {
					targets = append(targets, target)
				}
			}
			for _, target := range targets {
				about.Refs = append(about.Refs, syntax.Ref{Segments: d.refTo(target, c)})
				d.edge("about", p, target)
			}
			decl.Lines = append(decl.Lines, about)
			for _, line := range d.inputs(p, c, skeleton) {
				decl.Lines = append(decl.Lines, line.(syntax.PropertyLine))
			}
			at := 0
			if skeleton {
				at = 1
			}
			cases := d.cases(p, at)
			for _, cd := range cases {
				decl.Lines = append(decl.Lines, cd)
			}
			if skeleton {
				d.skel.prop = p
				d.skel.propCase = cases[0]
				for _, line := range cases[0].Lines {
					if a := line.(*syntax.Assignment); string(a.Symbol) == p.symbols[0].name {
						d.skel.number = a
					}
				}
			}
			d.componentFile(c).Decls = append(d.componentFile(c).Decls, decl)
		}
	}
}

// genCorpus writes the recorded cases of some flows and properties.
func (d *designer) genCorpus() {
	targets := append(slices.Clone(d.flows), d.props...)
	for _, n := range targets {
		skeleton := n == d.skel.prop
		if !skeleton && rapid.IntRange(0, 2).Draw(d.t, "corpus?") != 0 {
			continue
		}
		path := corpusPath(n.id)
		f := d.file(path, syntax.FileKindCorpus)
		f.Decls = append(f.Decls, &syntax.CorpusDecl{Target: syntax.Ref{Segments: strings.Split(n.id, ".")}})
		var names []string
		for i := range count(d, "recorded", 1, 2) {
			name := fmt.Sprintf("r-%08x", rapid.Uint32().Draw(d.t, "recordedName"))
			if slices.Contains(names, name) {
				continue
			}
			names = append(names, name)
			rc := &syntax.RecordedCase{Name: syntax.Kebab(name), Reason: pick(d, "reason", []string{"coverage", "counterexample"}), Lines: d.assignments(n)}
			f.Decls = append(f.Decls, rc)
			d.nodes = append(d.nodes, n.id+"."+name)
			d.edges = append(d.edges, "contains "+n.id+" "+n.id+"."+name)
			if skeleton && i == 0 {
				d.skel.recorded = rc
				d.skel.corpusPath = path
			}
		}
	}
}

// flowGen draws the steps of one flow, numbered in the order of the text.
type flowGen struct {
	d       *designer
	flow    *gNode
	scope   *gNode
	n       int
	emitted []*gNode
	sent    []*gNode
	budget  int
}

func (g *flowGen) newStep(parent string) string {
	g.n++
	id := g.flow.id + "#" + strconv.Itoa(g.n)
	g.d.nodes = append(g.d.nodes, id)
	g.d.edges = append(g.d.edges, "contains "+parent+" "+id)
	return id
}

// senderHome is the container whose dependencies a sender's calls follow:
// an actor's is the system.
func (g *flowGen) senderHome(s *gNode) *gNode {
	if s.kind == "person" || s.kind == "external" {
		return g.d.system
	}
	return home(s)
}

func (g *flowGen) callable(sender *gNode) []*gNode {
	var out []*gNode
	for _, op := range g.d.ops {
		if allowed(g.senderHome(sender), home(op)) {
			out = append(out, op)
		}
	}
	return out
}

func (g *flowGen) participant(n *gNode) syntax.Participant {
	return syntax.Participant{Segments: g.d.refTo(n, g.scope)}
}

func (g *flowGen) args(n int) []syntax.Lower {
	out := make([]syntax.Lower, n)
	for i := range out {
		out[i] = syntax.Lower(lowerNames.Filter(func(s string) bool { return s != "true" && s != "false" && s != "none" }).Draw(g.d.t, "arg"))
	}
	return out
}

// call writes sender -> op(args) with steps of its own, then on blocks for
// some of the errors op raises; receiver is who acts in the list it is part of.
func (g *flowGen) call(parent string, sender, op, receiver *gNode, depth int, inner func(id string) []syntax.Step) []syntax.Step {
	d := g.d
	id := g.newStep(parent)
	d.edges = append(d.edges, "from "+id+" "+sender.id, "calls "+id+" "+op.id)
	c := &syntax.Call{From: g.participant(sender), Op: syntax.OpRef{Segments: d.refTo(op, g.scope)}, Args: g.args(len(op.params))}
	if inner != nil {
		c.Lines = inner(id)
	} else if depth > 0 {
		c.Lines = g.list(id, op.parent, depth-1)
	}
	out := []syntax.Step{c}
	for _, e := range op.raises {
		if inner == nil && rapid.IntRange(0, 2).Draw(d.t, "on?") != 0 {
			continue
		}
		out = append(out, g.on(parent, e, receiver, depth, inner != nil))
	}
	return out
}

func (g *flowGen) on(parent string, e, receiver *gNode, depth int, empty bool) *syntax.OnBlock {
	id := g.newStep(parent)
	g.d.edges = append(g.d.edges, "handles "+id+" "+e.id)
	b := &syntax.OnBlock{Error: syntax.TypeRef{Segments: g.d.refTo(e, g.scope)}}
	if !empty && depth > 0 {
		b.Lines = g.list(id, receiver, depth-1)
	}
	return b
}

func (g *flowGen) send(parent string, sender, op *gNode) *syntax.Send {
	id := g.newStep(parent)
	g.d.edges = append(g.d.edges, "from "+id+" "+sender.id, "calls "+id+" "+op.id)
	g.sent = append(g.sent, op)
	return &syntax.Send{From: g.participant(sender), Op: syntax.OpRef{Segments: g.d.refTo(op, g.scope)}, Args: g.args(len(op.params))}
}

func (g *flowGen) emit(parent string, sender, event *gNode) *syntax.Emit {
	id := g.newStep(parent)
	g.d.edges = append(g.d.edges, "from "+id+" "+sender.id, "emits "+id+" "+event.id)
	g.emitted = append(g.emitted, event)
	return &syntax.Emit{From: g.participant(sender), Event: syntax.TypeRef{Segments: g.d.refTo(event, g.scope)}, Args: g.args(len(event.fields))}
}

func (g *flowGen) async(parent string, op *gNode, depth int) *syntax.Async {
	id := g.newStep(parent)
	g.d.edges = append(g.d.edges, "calls "+id+" "+op.id)
	a := &syntax.Async{Op: syntax.OpRef{Segments: g.d.refTo(op, g.scope)}}
	if depth > 0 {
		a.Lines = g.list(id, op.parent, depth-1)
	}
	return a
}

// skeleton writes the steps every rich design has: a person's call whose
// steps call, emit and send, an on block, and an async after the emit.
func (g *flowGen) skeleton() []syntax.Step {
	d, k := g.d, &g.d.skel
	steps := g.call(g.flow.id, d.persons[0], k.raising, nil, 0, func(id string) []syntax.Step {
		inner := g.call(id, k.service, k.helper, k.service, 0, func(id string) []syntax.Step {
			k.emit = g.emit(id, k.service, k.event)
			return []syntax.Step{k.emit}
		})
		k.inner = inner[0].(*syntax.Call)
		return append(inner, g.send(id, k.service, k.helper))
	})
	k.call = steps[0].(*syntax.Call)
	k.on = steps[1].(*syntax.OnBlock)
	k.async = g.async(g.flow.id, k.sub, 0)
	return append(steps, k.async)
}

// list draws a list of steps; receiver acts in it, or anyone at the top (nil).
func (g *flowGen) list(parent string, receiver *gNode, depth int) []syntax.Step {
	var out []syntax.Step
	for range count(g.d, "steps", 0, 3) {
		if g.budget <= 0 {
			break
		}
		g.budget--
		out = append(out, g.one(parent, receiver, depth)...)
	}
	return out
}

func (g *flowGen) one(parent string, receiver *gNode, depth int) []syntax.Step {
	d := g.d
	senders := []*gNode{receiver}
	if receiver == nil {
		senders = slices.Concat(d.persons, d.externals, d.containers, d.types)
	}
	if len(senders) == 0 {
		return nil
	}
	sender := pick(d, "sender", senders)
	switch count(d, "step", 0, 6) {
	case 0, 1:
		if ops := g.callable(sender); len(ops) > 0 {
			return g.call(parent, sender, pick(d, "op", ops), receiver, depth, nil)
		}
	case 2:
		if ops := g.callable(sender); len(ops) > 0 {
			return []syntax.Step{g.send(parent, sender, pick(d, "op", ops))}
		}
	case 3:
		events := slices.DeleteFunc(slices.Clone(d.types), func(t *gNode) bool { return t.form != "event" })
		if len(events) > 0 && (sender.kind == "component" || sender.kind == "type") {
			return []syntax.Step{g.emit(parent, sender, pick(d, "event", events))}
		}
	case 4:
		var triggered []*gNode
		for _, op := range d.ops {
			if slices.Contains(g.sent, op) || op.handles != nil && slices.Contains(g.emitted, op.handles) {
				triggered = append(triggered, op)
			}
		}
		if len(triggered) > 0 {
			return []syntax.Step{g.async(parent, pick(d, "async", triggered), depth)}
		}
	case 5:
		id := g.newStep(parent)
		a := &syntax.Alt{}
		for range count(d, "branches", 1, 3) {
			a.Branches = append(a.Branches, syntax.Branch{Label: d.summary("label"), Lines: g.list(id, receiver, depth-1)})
		}
		return []syntax.Step{a}
	default:
		id := g.newStep(parent)
		if rapid.Bool().Draw(d.t, "opt?") {
			return []syntax.Step{&syntax.Opt{Label: d.summary("label"), Lines: g.list(id, receiver, depth-1)}}
		}
		return []syntax.Step{&syntax.Loop{Label: d.summary("label"), Lines: g.list(id, receiver, depth-1)}}
	}
	return nil
}
