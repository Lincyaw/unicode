package design

import (
	"slices"
	"strconv"
	"strings"

	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/syntax"
)

// expressionCheck resolves and types a clause in its property's input context.
type expressionCheck struct {
	l                    *loader
	owner, scope, tested *entity
	locals               map[string]*typ
	bound                map[string]bool
	observed             map[string]bool
	clause               string
	ordinal              int
	allowOld             bool
}

func builtin(name string) *typ { return &typ{builtin: name} }

func unwrap(t *typ) *typ {
	for t != nil && t.kind == "optional" {
		t = t.elems[0]
	}
	return t
}

func compatible(a, b *typ) bool {
	if a == nil || b == nil || a.builtin == "invalid" || b.builtin == "invalid" {
		return true
	}
	if a.builtin == "none" {
		return b.kind == "optional"
	}
	if b.builtin == "none" {
		return a.kind == "optional"
	}
	a, b = unwrap(a), unwrap(b)
	return a.text() == b.text()
}

func (c *expressionCheck) bad(e *syntax.Expr, message, fix string) *typ {
	c.l.report(c.owner.file, e.Pos, "design.expression-type", c.owner.id, message, fix)
	return builtin("invalid")
}

func (c *expressionCheck) reference(target *entity, expr *syntax.Expr) {
	if target == nil {
		return
	}
	c.l.depends(c.owner, []ref{{written: target.id, pos: expr.Pos, target: target}})
	key := edgeKey{graph.EdgeKindRefers, c.owner.id, target.id}
	entry := c.clause + "#" + strconv.Itoa(c.ordinal)
	path := ""
	if target.kind == graph.NodeKindType && target.form != "enum" && (expr.Kind == "field" || expr.Kind == "index") {
		path = syntax.ExprText(expr)
	}
	for i := range c.l.edges {
		e := &c.l.edges[i]
		if e.Kind == key.kind && e.From == key.from && e.To == key.to {
			if e.Attrs == nil {
				e.Attrs = map[string][]string{}
			}
			if !slices.Contains(e.Attrs["clauses"], entry) {
				e.Attrs["clauses"] = append(e.Attrs["clauses"], entry)
				slices.Sort(e.Attrs["clauses"])
			}
			if path != "" && !slices.Contains(e.Attrs["paths"], path) {
				e.Attrs["paths"] = append(e.Attrs["paths"], path)
				slices.Sort(e.Attrs["paths"])
			}
			return
		}
	}
	attrs := map[string][]string{"clauses": {entry}}
	if path != "" {
		attrs["paths"] = []string{path}
	}
	c.l.edge(graph.EdgeKindRefers, c.owner, target, c.owner.file, expr.Pos, graph.SourceDeclared, attrs)
}

func pathNames(e *syntax.Expr) ([]string, bool) {
	if e.Kind == "name" {
		return []string{e.Text}, true
	}
	if e.Kind == "field" {
		parts, ok := pathNames(e.Children[0])
		return append(parts, e.Text), ok
	}
	return nil, false
}

func (c *expressionCheck) localRoot(e *syntax.Expr) bool {
	for e.Kind == "field" || e.Kind == "index" {
		e = e.Children[0]
	}
	return e.Kind == "name" && (e.Text == "result" || c.locals[e.Text] != nil)
}

func (c *expressionCheck) infer(e *syntax.Expr) *typ {
	if e == nil {
		return builtin("invalid")
	}
	switch e.Kind {
	case "number":
		if strings.Contains(e.Text, ".") {
			return builtin("float")
		}
		return builtin("int")
	case "string", "bool", "none":
		return builtin(e.Kind)
	case "name":
		if e.Text == "result" {
			if c.tested == nil || c.tested.result == nil {
				return c.bad(e, "result needs a tested call with a return value", "use result only in a postcondition of an operation with a return value")
			}
			return c.tested.result
		}
		if t := c.locals[e.Text]; t != nil {
			return t
		}
		return c.bad(e, "the expression name "+e.Text+" names no input or observation", "declare the input or observation, or correct its name")
	case "field":
		if parts, ok := pathNames(e); ok && !c.localRoot(e) && len(parts) > 1 {
			if n := c.l.resolve(c.scope, parts[:len(parts)-1]); n.is(graph.NodeKindType, "enum") && slices.Contains(n.values, parts[len(parts)-1]) {
				c.reference(n, e)
				return &typ{refs: []ref{{written: n.id, target: n, pos: e.Pos}}}
			}
		}
		t := unwrap(c.infer(e.Children[0]))
		if t == nil || t.builtin == "invalid" {
			return builtin("invalid")
		}
		if n := t.named(); n != nil {
			if f := findMember(n.fields, e.Text); f != nil {
				c.reference(n, e)
				return f.typ
			}
		}
		return c.bad(e, "the expression's value has no declared field "+e.Text, "access a field declared on this type")
	case "index":
		collection, key := unwrap(c.infer(e.Children[0])), c.infer(e.Children[1])
		if collection != nil && collection.kind == "map" && compatible(collection.elems[0], key) {
			return collection.elems[1]
		}
		return c.bad(e, "an index needs a map and a key of its declared key type", "index a map with a matching key")
	case "call":
		return c.invoke(e, false)
	case "old":
		if !c.allowOld || forbidden(e.Children[0], map[string]bool{"old": true, "result": true, "raised": true, "failure": true}, nil) || constraintReference(e.Children[0], c.bound) {
			return c.bad(e, "old needs a tested call and an expression without old, results, errors or quantified variables", "snapshot an input or observation expression before the tested call")
		}
		return c.infer(e.Children[0])
	case "raised", "failure":
		n := c.l.refer(c.owner, c.scope, strings.Split(e.Text, "."), e.Pos, acceptError)
		if c.tested == nil || n == nil || !slices.Contains(c.tested.raises, n) {
			return c.bad(e, e.Kind+" must name an error declared by the tested call", "use an error in the tested operation's raises list")
		}
		c.reference(n, e)
		if e.Kind == "raised" {
			return builtin("bool")
		}
		return &typ{refs: []ref{{written: n.id, target: n, pos: e.Pos}}}
	case "unary":
		if compatible(c.infer(e.Children[0]), builtin("bool")) {
			return builtin("bool")
		}
		return c.bad(e, "not needs a bool", "use a boolean expression after not")
	case "binary":
		return c.binary(e)
	case "all", "any", "count", "sum":
		collection := unwrap(c.infer(e.Children[0]))
		if collection == nil || collection.kind != "list" && collection.kind != "set" && collection.kind != "map" {
			return c.bad(e, "a quantifier needs a list, set or map", "quantify over a declared collection")
		}
		if c.locals[e.Text] != nil {
			return c.bad(e, "the quantified variable "+e.Text+" shadows another symbol", "use a distinct quantified variable name")
		}
		c.locals[e.Text], c.bound[e.Text] = collection.elems[0], true
		body := c.infer(e.Children[1])
		delete(c.locals, e.Text)
		delete(c.bound, e.Text)
		if e.Kind == "sum" {
			body = unwrap(body)
			if body.builtin == "int" || body.builtin == "float" {
				return body
			}
			return c.bad(e, "sum needs an int or float body", "sum numeric expressions")
		}
		if !compatible(body, builtin("bool")) {
			return c.bad(e, "a quantifier predicate must be bool", "use a boolean predicate")
		}
		if e.Kind == "count" {
			return builtin("int")
		}
		return builtin("bool")
	}
	return c.bad(e, "the expression has an unknown construct", "use the expression constructs of language.md §4.4")
}

func (c *expressionCheck) binary(e *syntax.Expr) *typ {
	a, b := c.infer(e.Children[0]), c.infer(e.Children[1])
	boolResult := func(ok bool) *typ {
		if ok {
			return builtin("bool")
		}
		return c.bad(e, "the operands do not match operator "+e.Text, "use operands of the types allowed by language.md §4.4")
	}
	switch e.Text {
	case "and", "or", "implies":
		return boolResult(compatible(a, builtin("bool")) && compatible(b, builtin("bool")))
	case "==", "!=":
		return boolResult(compatible(a, b) && a.kind != "call" && b.kind != "call" && (a.builtin != "none" || b.builtin != "none"))
	case "<", "<=", ">", ">=":
		return boolResult(compatible(a, b) && slices.Contains([]string{"int", "float", "string", "time", "duration", "invalid"}, unwrap(a).builtin))
	case "in":
		b = unwrap(b)
		return boolResult((b.kind == "list" || b.kind == "set" || b.kind == "map") && compatible(a, b.elems[0]))
	default:
		a, b = unwrap(a), unwrap(b)
		if compatible(a, b) && (a.builtin == "int" || a.builtin == "float" || a.builtin == "invalid") {
			return a
		}
		return c.bad(e, "arithmetic needs two ints or two floats", "use operands of the same numeric type")
	}
}

func (c *expressionCheck) invoke(e *syntax.Expr, tested bool) *typ {
	callee := e.Children[0]
	args := e.Children[1:]
	if callee.Kind == "name" && callee.Text == "size" {
		if tested || len(args) != 1 {
			return c.bad(e, "size needs one collection argument and cannot be the tested call", "use size(collection) in a pure expression")
		}
		t := unwrap(c.infer(args[0]))
		if t.kind == "list" || t.kind == "set" || t.kind == "map" || t.builtin == "string" || t.builtin == "bytes" {
			return builtin("int")
		}
		return c.bad(e, "size needs a collection, string or bytes", "measure a collection, string or bytes")
	}
	var op *entity
	if parts, ok := pathNames(callee); ok && !c.localRoot(callee) {
		op = c.l.refer(c.owner, c.scope, parts, callee.Pos, acceptOp)
		if op != nil && op.parent.kind == graph.NodeKindType {
			return c.bad(e, "a type operation needs a receiver value", "call the operation on an input or result value")
		}
	} else if callee.Kind == "field" {
		receiver := unwrap(c.infer(callee.Children[0]))
		if n := receiver.named(); n != nil {
			op = findEntity(n.ops, callee.Text)
		}
		if tested {
			parts, ok := pathNames(callee.Children[0])
			if !ok || len(parts) == 0 || c.owner.symbol(parts[0]) == nil {
				return c.bad(e, "the tested receiver must be an input or its declared fields", "call an operation on an input field path")
			}
		}
	}
	if op == nil {
		return c.bad(e, "the expression calls no declared operation", "call an operation declared on this receiver or component")
	}
	c.reference(op, e)
	if !tested && (!op.pure || op.result == nil) {
		return c.bad(e, "an expression may call only a pure operation with a return value", "declare a pure result-bearing operation or use call for the tested effect")
	}
	if len(args) != len(op.params) {
		return c.bad(e, "the call's argument count differs from the operation's parameters", "supply one expression for each declared parameter")
	}
	for i, arg := range args {
		if tested && forbidden(arg, map[string]bool{"old": true, "result": true, "raised": true, "failure": true}, c.observed) {
			return c.bad(arg, "tested call arguments may depend only on input values", "remove references to observations, old, result or errors")
		}
		if !compatible(c.infer(arg), op.params[i].typ) {
			c.bad(arg, "the argument type differs from parameter "+op.params[i].name, "pass an expression of type "+op.params[i].typ.text())
		}
	}
	if tested {
		c.tested = op
	}
	if op.result != nil {
		return op.result
	}
	return builtin("void")
}

// forbidden finds phase-invalid special forms and references to disallowed locals.
func forbidden(e *syntax.Expr, kinds, names map[string]bool) bool {
	if kinds[e.Kind] || e.Kind == "name" && (kinds[e.Text] || names[e.Text]) {
		return true
	}
	for _, child := range e.Children {
		if forbidden(child, kinds, names) {
			return true
		}
	}
	return false
}

func constraintReference(e *syntax.Expr, names map[string]bool) bool {
	if e.Kind == "name" && names[e.Text] {
		return true
	}
	if e.Kind == "all" || e.Kind == "any" || e.Kind == "count" || e.Kind == "sum" {
		bound := map[string]bool{}
		for name, value := range names {
			bound[name] = value
		}
		bound[e.Text] = true
		return constraintReference(e.Children[0], names) || constraintReference(e.Children[1], bound)
	}
	for _, child := range e.Children {
		if constraintReference(child, names) {
			return true
		}
	}
	return false
}

func (l *loader) expressionContext(e, scope *entity) *expressionCheck {
	c := &expressionCheck{l: l, owner: e, scope: scope, locals: map[string]*typ{}, bound: map[string]bool{}, observed: map[string]bool{}}
	for _, s := range e.symbols {
		c.locals[s.name] = s.typ
	}
	for _, o := range e.observations {
		c.locals[o.name] = o.snapshot
		c.observed[o.name] = true
	}
	return c
}

func (l *loader) propertyExpressions(e, scope *entity, d *syntax.PropertyDecl) {
	for _, line := range d.Lines {
		if o, ok := line.(*syntax.Observe); ok && o.Named != nil {
			n := o.Named
			obs := &entity{name: string(n.Name), kind: graph.NodeKindObservation, form: "snapshot", file: e.file, pos: n.Pos}
			if e.symbol(obs.name) != nil {
				l.unique(e, map[string]bool{obs.name: true}, obs.name, obs.pos)
			}
			if !l.declare(e, obs, nil) {
				continue
			}
			e.observations = append(e.observations, obs)
			obs.setAttr("inputs", names(n.Inputs))
			l.signatures = append(l.signatures, func() {
				obs.snapshot = l.typeOf(obs, scope, n.Type)
				obs.setAttr("type", []string{obs.snapshot.text()})
				l.refers(obs, obs.snapshot)
				l.depends(obs, obs.snapshot.allRefs())
			})
		}
	}
	l.checks = append(l.checks, func() { l.checkPropertyExpressions(e, scope, d) })
}

func (l *loader) checkPropertyExpressions(e, scope *entity, d *syntax.PropertyDecl) {
	if e.attrs == nil {
		e.attrs = map[string][]string{}
	}
	c := l.expressionContext(e, scope)
	for _, o := range e.observations {
		if e.symbol(o.name) != nil {
			l.report(e.file, o.pos, "design.duplicate-name", e.id, "an observation shadows an input", "choose a unique observation name")
		}
		seen := map[string]bool{}
		for _, input := range o.attrs["inputs"] {
			if seen[input] || e.symbol(input) == nil {
				l.report(e.file, o.pos, "design.invalid-observation", o.id, "an observation input is repeated or is not a property input", "list distinct forall symbols in adapter parameter order")
			}
			seen[input] = true
		}
		if why := snapshotType(o.snapshot, map[*entity]bool{}); why != "" {
			l.report(e.file, o.pos, "design.invalid-observation", o.id, why, "return a finite snapshot of declared value fields")
		}
	}
	var sequence *entity
	sequences := 0
	for _, s := range e.symbols {
		if s.typ.kind == "list" && s.typ.elems[0].kind == "call" {
			sequences++
			for _, op := range s.typ.elems[0].ops() {
				if sequence == nil {
					sequence = op.parent
				} else if sequence != op.parent {
					sequence = nil
					sequences = 2
					break
				}
			}
		}
	}
	hasEnsure := false
	for _, line := range d.Lines {
		if _, ok := line.(*syntax.Ensure); ok {
			hasEnsure = true
		}
	}
	if hasEnsure && (sequences > 1 || e.form == "transition" && sequences != 1) {
		l.report(e.file, e.pos, "design.invalid-call-sequence", e.id, "a checked sequence must target one type and a transition needs one sequence", "declare at most one list<call<T>> targeting one type")
	}
	if hasEnsure && sequences == 1 && sequence != nil {
		count := 0
		for _, s := range e.symbols {
			if s.typ.named() == sequence {
				count++
			}
		}
		if count != 1 || sequence.kind != graph.NodeKindType {
			l.report(e.file, e.pos, "design.invalid-call-sequence", e.id, "a call sequence needs exactly one target input of its operation owner type", "declare exactly one input of the sequence's receiver type")
		}
		c.allowOld = true
	}
	for _, line := range d.Lines {
		if act, ok := line.(*syntax.Act); ok {
			c.clause, c.ordinal = "call", 1
			x := act.Expression.Expr()
			if x.Kind != "call" {
				c.bad(x, "the call clause must invoke one operation", "write call receiver.op(args) or call op(args)")
			} else {
				c.invoke(x, true)
			}
			c.allowOld = true
			e.setAttr("call", []string{syntax.ExprText(x)})
			if sequences != 0 {
				l.report(e.file, act.Pos, "design.invalid-call-sequence", e.id, "a call sequence cannot be combined with a call clause", "choose one tested call or a sequence")
			}
			if c.tested != nil {
				found := false
				for _, edge := range l.edges {
					if edge.Kind == graph.EdgeKindAbout && edge.From == e.id && edge.To == c.tested.id {
						found = true
					}
				}
				if !found {
					l.report(e.file, act.Pos, "design.call-not-about", e.id, "the tested operation is not listed in about", "include the tested operation in about")
				}
			}
		}
	}
	counts := map[string]int{}
	var observations []string
	for _, line := range d.Lines {
		switch x := line.(type) {
		case *syntax.Observe:
			counts["observe"]++
			c.clause, c.ordinal = "observe", counts["observe"]
			if x.None {
				observations = append(observations, "none")
			} else if x.Named != nil {
				if observation := e.children[string(x.Named.Name)]; observation != nil && observation.snapshot != nil {
					observations = append(observations, string(x.Named.Name)+": "+observation.snapshot.text()+" from "+strings.Join(names(x.Named.Inputs), ", "))
				}
			} else {
				for _, p := range x.Paths {
					expr := refExpression(p)
					if len(p.Segments) == 0 || e.symbol(p.Segments[0]) == nil {
						l.report(e.file, p.Pos, "design.invalid-observation", e.id, "a direct observation starts from a forall input", "observe an input or use a named adapter observation")
					}
					observedType := c.infer(expr)
					for _, ref := range observedType.allRefs() {
						c.reference(ref.target, expr)
					}
					if why := snapshotType(observedType, map[*entity]bool{}); why != "" {
						l.report(e.file, p.Pos, "design.invalid-observation", e.id, why, "observe finite declared value fields or use a snapshot adapter")
					}
					observations = append(observations, syntax.ExprText(expr))
				}
			}
		case *syntax.Requires:
			counts["requires"]++
			c.clause, c.ordinal = "requires", counts["requires"]
			expr := x.Expression.Expr()
			c.precondition(expr)
			e.attrs["requires"] = append(e.attrs["requires"], syntax.ExprText(expr))
		case *syntax.Changes:
			c.clause, c.ordinal = "changes", 1
			if x.None {
				e.setAttr("changes", []string{"none"})
			} else {
				var paths []string
				for _, p := range x.Paths {
					expr := p.Expr()
					l.changePath(c, expr, observations)
					paths = append(paths, syntax.ExprText(expr))
				}
				e.setAttr("changes", paths)
			}
		case *syntax.Ensure:
			counts["ensure"]++
			c.clause, c.ordinal = "ensure", counts["ensure"]
			expr := x.Expression.Expr()
			if !compatible(c.infer(expr), builtin("bool")) {
				c.bad(expr, "ensure must have type bool", "state a boolean relation")
			}
			e.attrs["ensure"] = append(e.attrs["ensure"], syntax.ExprText(expr))
		case *syntax.Within:
			e.setAttr("within", []string{string(x.Duration)})
		case *syntax.Manual:
			e.setAttr("manual", []string{string(x.Reason)})
		}
	}
	e.setAttr("observe", observations)
}

func (c *expressionCheck) precondition(expr *syntax.Expr) {
	if forbidden(expr, map[string]bool{"old": true, "result": true, "raised": true, "failure": true}, nil) {
		c.bad(expr, "a pre-call expression cannot read old, result or errors", "use only inputs, observations and pure operations")
	}
	if !compatible(c.infer(expr), builtin("bool")) {
		c.bad(expr, "requires must have type bool", "state a boolean precondition")
	}
}

func refExpression(r syntax.Ref) *syntax.Expr {
	e := &syntax.Expr{Pos: r.Pos, Kind: "name", Text: r.Segments[0]}
	for _, s := range r.Segments[1:] {
		e = &syntax.Expr{Pos: r.Pos, Kind: "field", Text: s, Children: []*syntax.Expr{e}}
	}
	return e
}

func snapshotType(t *typ, seen map[*entity]bool) string {
	if t == nil || t.builtin == "invalid" {
		return ""
	}
	if t.kind == "call" || t.kind == "replies" {
		return "call values and response scripts cannot be observed as snapshots"
	}
	for _, elem := range t.elems {
		if why := snapshotType(elem, seen); why != "" {
			return why
		}
	}
	n := t.named()
	if n == nil || seen[n] {
		return ""
	}
	if n.form == "interface" || n.form == "error" {
		return "interfaces and errors cannot be directly observed as snapshots"
	}
	seen[n] = true
	for _, field := range n.fields {
		if why := snapshotType(field.typ, seen); why != "" {
			return why
		}
	}
	return ""
}

func (l *loader) changePath(c *expressionCheck, expr *syntax.Expr, observations []string) {
	text := syntax.ExprText(expr)
	root := expr
	valid := true
	for root.Kind == "field" || root.Kind == "index" {
		if root.Kind == "index" && forbidden(root.Children[1], map[string]bool{"old": true, "result": true, "raised": true, "failure": true}, nil) {
			valid = false
		}
		root = root.Children[0]
	}
	valid = valid && root.Kind == "name"
	inside := false
	for _, observed := range observations {
		if observed == "none" {
			continue
		}
		if i := strings.Index(observed, ": "); i >= 0 {
			observed = observed[:i]
		}
		if text == observed || strings.HasPrefix(text, observed+".") || strings.HasPrefix(text, observed+"[") {
			inside = true
		}
	}
	if !valid || !inside {
		l.report(c.owner.file, expr.Pos, "design.invalid-change-path", c.owner.id, "the change path is not a field/map path inside an observation", "select a field or map entry below an observed root using pre-call keys")
	}
	c.infer(expr)
}

func (l *loader) flowExpressions(e, scope *entity, d *syntax.FlowDecl, steps []syntax.Step) {
	c := l.expressionContext(e, scope)
	manual := false
	for _, line := range d.Lines {
		if x, ok := line.(*syntax.Manual); ok {
			e.setAttr("manual", []string{string(x.Reason)})
			manual = true
		}
	}
	for _, step := range steps {
		if _, comment := step.(*syntax.Comment); comment {
			continue
		}
		if call, ok := step.(*syntax.Call); ok {
			c.tested = l.resolve(scope, call.Op.Segments)
			c.allowOld = true
			if !manual && c.tested != nil {
				callee := c.tested.id
				if c.tested.parent.kind == graph.NodeKindType {
					var receiver string
					count := 0
					for _, symbol := range e.symbols {
						if symbol.typ.named() == c.tested.parent {
							receiver = symbol.name
							count++
						}
					}
					if count != 1 {
						c.bad(&syntax.Expr{Pos: call.Pos}, "an automatic flow method needs exactly one receiver input", "declare exactly one forall input of the entry operation's owner type or declare manual")
					} else {
						callee = receiver + "." + c.tested.name
					}
				}
				expr := &syntax.Expr{Pos: call.Pos, Kind: "call", Children: []*syntax.Expr{refExpression(syntax.Ref{Pos: call.Pos, Segments: strings.Split(callee, ".")})}}
				if c.tested.parent.kind == graph.NodeKindType {
					if dot := strings.LastIndexByte(callee, '.'); dot >= 0 {
						expr.Children[0] = &syntax.Expr{Pos: call.Pos, Kind: "field", Text: callee[dot+1:], Children: []*syntax.Expr{{Pos: call.Pos, Kind: "name", Text: callee[:dot]}}}
					}
				}
				for _, arg := range call.Args {
					expr.Children = append(expr.Children, &syntax.Expr{Pos: call.Pos, Kind: "name", Text: string(arg)})
				}
				c.clause, c.ordinal = "call", 1
				c.invoke(expr, true)
				e.setAttr("call", []string{syntax.ExprText(expr)})
			}
		} else if !manual {
			c.bad(&syntax.Expr{Pos: e.pos}, "an automatic flow needs a first synchronous call", "start with a synchronous entry operation or declare manual")
		}
		break
	}
	for _, line := range d.Lines {
		switch x := line.(type) {
		case *syntax.Within:
			e.setAttr("within", []string{string(x.Duration)})
		case *syntax.Ensure:
			c.clause, c.ordinal = "ensure", len(e.attrs["ensure"])+1
			expr := x.Expression.Expr()
			if !compatible(c.infer(expr), builtin("bool")) {
				c.bad(expr, "ensure must have type bool", "state a boolean relation")
			}
			e.setAttr("ensure", append(e.attrs["ensure"], syntax.ExprText(expr)))
		}
	}
}
