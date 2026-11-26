package syntax

import (
	"bytes"
	"strings"
)

// maxWidth is the width a value line may take, in bytes as columns are counted (docs/format.md §3.1).
const maxWidth = 100

// Print writes the file in the canonical format of docs/format.md §3.
func (f *File) Print() []byte {
	pr := &printer{}
	switch {
	case f.System != nil:
		pr.systemFile(f.System)
	case f.Component != nil:
		pr.componentFile(f.Component)
	case f.Corpus != nil:
		pr.corpusFile(f.Corpus)
	}
	if len(f.Trailing) > 0 {
		pr.top(f.Trailing, func() {})
	}
	return pr.b.Bytes()
}

type printer struct {
	b      bytes.Buffer
	indent int
}

func (pr *printer) line(s string) {
	pr.b.WriteString(strings.Repeat("  ", pr.indent))
	pr.b.WriteString(s)
	pr.b.WriteByte('\n')
}

func (pr *printer) comments(cs []Comment) {
	for _, c := range cs {
		pr.line("//" + c.Text)
	}
}

// top writes one top-level item; items are separated by one blank line.
func (pr *printer) top(comments []Comment, body func()) {
	if pr.b.Len() > 0 {
		pr.b.WriteByte('\n')
	}
	pr.comments(comments)
	body()
}

// block writes `head {`, the members, the trailing comments and `}`.
func (pr *printer) block(head string, members func(), trailing []Comment) {
	pr.line(head + " {")
	pr.indent++
	members()
	pr.comments(trailing)
	pr.indent--
	pr.line("}")
}

// optBlock writes head alone when the block has nothing in it.
func (pr *printer) optBlock(head string, empty bool, members func(), trailing []Comment) {
	if empty && len(trailing) == 0 {
		pr.line(head)
		return
	}
	pr.block(head, members, trailing)
}

func (pr *printer) systemFile(sf *SystemFile) {
	pr.top(sf.System.Comments, func() {
		pr.line("system " + sf.System.Name + " " + quote(sf.System.Summary))
	})
	for _, n := range sf.Notations {
		pr.top(n.Comments, func() { pr.line("notation " + n.Name) })
	}
	for _, l := range sf.Languages {
		pr.top(l.Comments, func() {
			pr.block("language "+l.Name, func() {
				for _, r := range l.Roots {
					pr.comments(r.Comments)
					pr.line("root " + quote(r.Path))
				}
			}, l.Trailing)
		})
	}
	for _, a := range sf.Actors {
		pr.top(a.Comments, func() {
			head := "actor " + a.Name + " " + a.Form + " " + quote(a.Summary)
			pr.optBlock(head, len(a.Types) == 0 && len(a.Ops) == 0, func() {
				for _, t := range a.Types {
					pr.typeDecl(t)
				}
				for _, o := range a.Ops {
					pr.opDecl(o)
				}
			}, a.Trailing)
		})
	}
}

func (pr *printer) componentFile(cf *ComponentFile) {
	c := cf.Component
	pr.top(c.Comments, func() {
		pr.block("component "+strings.Join(c.Path, ".")+" "+quote(c.Summary), func() {
			pr.labels(c.Labels)
			if c.Focus != nil {
				pr.comments(c.Focus.Comments)
				pr.line("focus " + c.Focus.Level)
			}
			pr.comments(c.Paths.Comments)
			quoted := make([]string, len(c.Paths.Patterns))
			for i, s := range c.Paths.Patterns {
				quoted[i] = quote(s)
			}
			pr.line("paths " + strings.Join(quoted, ", "))
			if c.Uses != nil {
				pr.comments(c.Uses.Comments)
				pr.line("uses " + refs(c.Uses.Refs))
			}
		}, c.Trailing)
	})
	for _, t := range cf.Types {
		pr.top(t.Comments, func() { pr.typeBody(t) })
	}
	for _, o := range cf.Ops {
		pr.top(o.Comments, func() { pr.line(opLine(o)) })
	}
	for _, f := range cf.Flows {
		pr.top(f.Comments, func() { pr.flow(f) })
	}
	for _, p := range cf.Properties {
		pr.top(p.Comments, func() { pr.property(p) })
	}
}

func (pr *printer) labels(ls []Label) {
	for _, l := range ls {
		pr.comments(l.Comments)
		pr.line("label " + l.Name)
	}
}

func (pr *printer) typeDecl(t TypeDecl) {
	pr.comments(t.Comments)
	pr.typeBody(t)
}

func (pr *printer) typeBody(t TypeDecl) {
	head := "type " + t.Name + " " + t.Form + " " + quote(t.Summary)
	empty := len(t.Labels)+len(t.Extends)+len(t.Implements)+len(t.Fields)+len(t.Values)+len(t.Ops) == 0
	pr.optBlock(head, empty, func() {
		pr.labels(t.Labels)
		for _, s := range t.Extends {
			pr.comments(s.Comments)
			pr.line("extends " + ref(s.Type))
		}
		for _, s := range t.Implements {
			pr.comments(s.Comments)
			pr.line("implements " + ref(s.Type))
		}
		for _, f := range t.Fields {
			pr.comments(f.Comments)
			pr.line("field " + f.Name + ": " + typeExpr(f.Type))
		}
		for _, v := range t.Values {
			pr.comments(v.Comments)
			pr.line("value " + v.Name)
		}
		for _, o := range t.Ops {
			pr.opDecl(o)
		}
	}, t.Trailing)
}

func (pr *printer) opDecl(o OpDecl) {
	pr.comments(o.Comments)
	pr.line(opLine(o))
}

func opLine(o OpDecl) string {
	var b strings.Builder
	b.WriteString("op " + o.Name + "(")
	for i, p := range o.Params {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(p.Name + ": " + typeExpr(p.Type))
	}
	b.WriteString(")")
	if o.Result != nil {
		b.WriteString(" -> " + typeExpr(o.Result))
	}
	if len(o.Raises) > 0 {
		b.WriteString(" raises " + refs(o.Raises))
	}
	if o.Handles != nil {
		b.WriteString(" handles " + ref(*o.Handles))
	}
	if o.Summary != "" {
		b.WriteString(" " + quote(o.Summary))
	}
	return b.String()
}

func (pr *printer) flow(f FlowDecl) {
	pr.block("flow "+f.Name+" "+quote(f.Summary), func() {
		pr.symbols(f.Symbols)
		pr.limits(f.Limits)
		pr.steps(f.Steps)
		pr.cases(f.Cases)
	}, f.Trailing)
}

func (pr *printer) property(p PropertyDecl) {
	pr.block("property "+p.Name+" "+p.Form+" "+quote(p.Summary), func() {
		pr.comments(p.About.Comments)
		pr.line("about " + refs(p.About.Refs))
		pr.symbols(p.Symbols)
		pr.limits(p.Limits)
		pr.cases(p.Cases)
	}, p.Trailing)
}

func (pr *printer) symbols(ss []Symbol) {
	for _, s := range ss {
		pr.comments(s.Comments)
		l := "forall " + s.Name + ": " + typeExpr(s.Type)
		if s.Generated {
			l += " generated " + quote(s.Domain)
		}
		pr.line(l)
	}
}

func (pr *printer) limits(ls []Limit) {
	for _, l := range ls {
		pr.comments(l.Comments)
		head := "where " + strings.Join(l.Path, ".")
		switch l.Kind {
		case LimitRange:
			pr.line(head + " in " + l.Low + ".." + l.High)
		case LimitMembers:
			pr.line(head + " in {" + strings.Join(l.Members, ", ") + "}")
		case LimitSize:
			pr.line(head + " size " + l.Low + ".." + l.High)
		}
	}
}

func (pr *printer) cases(cs []CaseDecl) {
	for _, c := range cs {
		pr.comments(c.Comments)
		head := "case " + c.Name
		if c.Summary != "" {
			head += " " + quote(c.Summary)
		}
		pr.block(head, func() { pr.assignments(c.Assignments) }, c.Trailing)
	}
}

func (pr *printer) assignments(as []Assignment) {
	for _, a := range as {
		pr.comments(a.Comments)
		pr.value(a.Symbol+" = ", a.Value, "")
	}
}

func (pr *printer) steps(steps []Step) {
	for _, s := range steps {
		pr.step(s)
	}
}

func (pr *printer) step(s Step) {
	switch s := s.(type) {
	case *CallStep:
		pr.comments(s.Comments)
		head := ref(s.From) + " -> " + ref(s.Op) + args(s.Args)
		pr.optBlock(head, len(s.Steps) == 0, func() { pr.steps(s.Steps) }, s.Trailing)
		for _, h := range s.Handlers {
			pr.comments(h.Comments)
			pr.block("on "+ref(h.Error), func() { pr.steps(h.Steps) }, h.Trailing)
		}
	case *SendStep:
		pr.comments(s.Comments)
		pr.line(ref(s.From) + " ->> " + ref(s.Op) + args(s.Args))
	case *EmitStep:
		pr.comments(s.Comments)
		pr.line(ref(s.From) + " emits " + ref(s.Event) + args(s.Args))
	case *AsyncStep:
		pr.comments(s.Comments)
		pr.block("async "+ref(s.Op), func() { pr.steps(s.Steps) }, s.Trailing)
	case *AltStep:
		pr.comments(s.Comments)
		for i, br := range s.Branches {
			head := "alt " + quote(br.Label) + " {"
			if i > 0 {
				head = "} else " + quote(br.Label) + " {"
			}
			pr.line(head)
			pr.indent++
			pr.steps(br.Steps)
			pr.comments(br.Trailing)
			pr.indent--
		}
		pr.line("}")
	case *OptStep:
		pr.comments(s.Comments)
		pr.block("opt "+quote(s.Label), func() { pr.steps(s.Steps) }, s.Trailing)
	case *LoopStep:
		pr.comments(s.Comments)
		pr.block("loop "+quote(s.Label), func() { pr.steps(s.Steps) }, s.Trailing)
	}
}

func (pr *printer) corpusFile(cf *CorpusFile) {
	pr.top(cf.Corpus.Comments, func() { pr.line("corpus " + ref(cf.Corpus.Target)) })
	for _, c := range cf.Cases {
		pr.top(c.Comments, func() {
			pr.block("case "+c.Name+" "+c.Reason, func() { pr.assignments(c.Assignments) }, c.Trailing)
		})
	}
}

// value writes v after prefix, then suffix: on one line when that line fits in
// maxWidth, otherwise with each item on its own line and a comma after each.
func (pr *printer) value(prefix string, v Value, suffix string) {
	flat := flatValue(v)
	open, items, closer := parts(v)
	if len(items) == 0 || 2*pr.indent+len(prefix)+len(flat)+len(suffix) <= maxWidth {
		pr.line(prefix + flat + suffix)
		return
	}
	pr.line(prefix + open)
	pr.indent++
	for _, it := range items {
		pr.value(it.prefix, it.value, ",")
	}
	pr.indent--
	pr.line(closer + suffix)
}

// part is one item of a bracketed value: an element, or a key or name and its value.
type part struct {
	prefix string
	value  Value
}

func parts(v Value) (string, []part, string) {
	var items []part
	switch v := v.(type) {
	case *ListValue:
		for _, it := range v.Items {
			items = append(items, part{value: it})
		}
		return "[", items, "]"
	case *BraceValue:
		for _, e := range v.Entries {
			items = append(items, part{prefix: flatValue(e.Key) + ": ", value: e.Value})
		}
		return "{", items, "}"
	case *CallValue:
		for _, a := range v.Args {
			items = append(items, part{prefix: a.Name + ": ", value: a.Value})
		}
		return v.Op + "(", items, ")"
	}
	return "", nil, ""
}

func flatValue(v Value) string {
	switch v := v.(type) {
	case *NumberValue:
		return v.Text
	case *StringValue:
		return quote(v.Text)
	case *BoolValue:
		if v.Value {
			return "true"
		}
		return "false"
	case *NoneValue:
		return "none"
	case *NameValue:
		return v.Name
	}
	open, items, closer := parts(v)
	flat := make([]string, len(items))
	for i, it := range items {
		flat[i] = it.prefix + flatValue(it.value)
	}
	return open + strings.Join(flat, ", ") + closer
}

func typeExpr(t TypeExpr) string {
	switch t := t.(type) {
	case *BuiltinType:
		return t.Name
	case *NamedType:
		return ref(t.Ref)
	case *ElemType:
		return t.Kind + "<" + typeExpr(t.Elem) + ">"
	case *MapType:
		return "map<" + typeExpr(t.Key) + ", " + typeExpr(t.Value) + ">"
	case *CallType:
		return "call<" + refs(t.Targets) + ">"
	}
	return ""
}

func ref(r Ref) string { return strings.Join(r.Segments, ".") }

func refs(rs []Ref) string {
	s := make([]string, len(rs))
	for i, r := range rs {
		s[i] = ref(r)
	}
	return strings.Join(s, ", ")
}

func args(as []string) string { return "(" + strings.Join(as, ", ") + ")" }

var quoter = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\t", `\t`)

// quote writes s as a string literal with the four escapes of docs/format.md §2.
func quote(s string) string { return `"` + quoter.Replace(s) + `"` }
