package syntax

import (
	"bytes"
	"strings"

	"github.com/mattn/go-runewidth"
)

// maxWidth is the display width a value line may take (docs/format.md §3.1).
const maxWidth = 100

// widths measures display width with East Asian wide characters as two
// columns. It is fixed rather than taken from the locale, so that fmt prints
// the same bytes everywhere.
var widths = &runewidth.Condition{EastAsianWidth: false, StrictEmojiNeutral: true}

// Print writes the file in the canonical format of docs/format.md §3: one
// blank line before each top-level declaration or comment group, none inside
// blocks, two spaces of indentation per block, and one final newline.
func (f *File) Print() []byte {
	pr := &printer{}
	for i, d := range f.Decls {
		if i > 0 {
			if _, comment := f.Decls[i-1].(*Comment); !comment {
				pr.b.WriteByte('\n')
			}
		}
		pr.node(d)
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

// block writes `head {`, the lines and `}`.
func block[L any](pr *printer, head string, ls []L) {
	pr.line(head + " {")
	pr.indent++
	for _, l := range ls {
		pr.node(l)
	}
	pr.indent--
	pr.line("}")
}

// optBlock writes head alone when an optional block has no lines.
func optBlock[L any](pr *printer, head string, ls []L) {
	if len(ls) == 0 {
		pr.line(head)
		return
	}
	block(pr, head, ls)
}

func (pr *printer) node(n any) {
	switch n := n.(type) {
	case *Comment:
		pr.line("//" + string(n.Text))
	case *SystemDecl:
		pr.line("system " + string(n.Name) + " " + quote(string(n.Summary)))
	case *NotationDecl:
		pr.line("notation " + n.Name)
	case *LanguageDecl:
		block(pr, "language "+n.Name, n.Lines)
	case *Root:
		pr.line("root " + quote(string(n.Path)))
	case *PersonDecl:
		pr.line("actor " + string(n.Name) + " person " + quote(string(n.Summary)))
	case *ExternalDecl:
		optBlock(pr, "actor "+string(n.Name)+" external "+quote(string(n.Summary)), n.Lines)
	case *ComponentDecl:
		block(pr, "component "+ref(Ref(n.Path))+" "+quote(string(n.Summary)), n.Lines)
	case *Label:
		pr.line("label " + string(n.Name))
	case *Focus:
		pr.line("focus " + n.Level)
	case *Paths:
		quoted := make([]string, len(n.Patterns))
		for i, p := range n.Patterns {
			quoted[i] = quote(string(p))
		}
		pr.line("paths " + strings.Join(quoted, ", "))
	case *Uses:
		pr.line("uses " + refs(n.Refs))
	case *TypeDecl:
		optBlock(pr, "type "+string(n.Name)+" "+n.Form+" "+quote(string(n.Summary)), n.Lines)
	case *Extends:
		pr.line("extends " + ref(Ref(n.Type)))
	case *Implements:
		pr.line("implements " + ref(Ref(n.Type)))
	case *Field:
		pr.line("field " + string(n.Name) + ": " + typeExpr(n.Type))
	case *EnumValue:
		pr.line("value " + string(n.Name))
	case *OpDecl:
		pr.line(opLine(n))
	case *FlowDecl:
		block(pr, "flow "+string(n.Name)+" "+quote(string(n.Summary)), n.Lines)
	case *PropertyDecl:
		block(pr, "property "+string(n.Name)+" "+n.Form+" "+quote(string(n.Summary)), n.Lines)
	case *About:
		pr.line("about " + refs(n.Refs))
	case *Symbol:
		l := "forall " + string(n.Name) + ": " + typeExpr(n.Type)
		if n.Domain != nil {
			l += " generated " + quote(string(*n.Domain))
		}
		pr.line(l)
	case *SubsetLimit:
		pr.line("where " + names(n.Path, ".") + " in {" + names(n.Members, ", ") + "}")
	case *IntervalLimit:
		pr.line("where " + names(n.Path, ".") + " " + n.Op + " " + string(n.Low) + ".." + string(n.High))
	case *CaseDecl:
		head := "case " + string(n.Name)
		if n.Summary != nil {
			head += " " + quote(string(*n.Summary))
		}
		block(pr, head, n.Lines)
	case *Assignment:
		pr.value(string(n.Symbol)+" = ", n.Value, "")
	case *CorpusDecl:
		pr.line("corpus " + ref(n.Target))
	case *RecordedCase:
		block(pr, "case "+string(n.Name)+" "+n.Reason, n.Lines)
	case *Call:
		optBlock(pr, ref(Ref(n.From))+" -> "+ref(Ref(n.Op))+args(n.Args), n.Lines)
	case *Send:
		pr.line(ref(Ref(n.From)) + " ->> " + ref(Ref(n.Op)) + args(n.Args))
	case *Emit:
		pr.line(ref(Ref(n.From)) + " emits " + ref(Ref(n.Event)) + args(n.Args))
	case *OnBlock:
		block(pr, "on "+ref(Ref(n.Error)), n.Lines)
	case *Async:
		block(pr, "async "+ref(Ref(n.Op)), n.Lines)
	case *Alt:
		head := "alt "
		for _, br := range n.Branches {
			pr.line(head + quote(string(br.Label)) + " {")
			pr.indent++
			for _, l := range br.Lines {
				pr.node(l)
			}
			pr.indent--
			head = "} else "
		}
		pr.line("}")
	case *Opt:
		block(pr, "opt "+quote(string(n.Label)), n.Lines)
	case *Loop:
		block(pr, "loop "+quote(string(n.Label)), n.Lines)
	}
}

func opLine(o *OpDecl) string {
	params := make([]string, len(o.Params))
	for i, p := range o.Params {
		params[i] = string(p.Name) + ": " + typeExpr(p.Type)
	}
	l := "op " + string(o.Name) + "(" + strings.Join(params, ", ") + ")"
	if o.Result != nil {
		l += " -> " + typeExpr(o.Result)
	}
	if len(o.Raises) > 0 {
		raises := make([]string, len(o.Raises))
		for i, r := range o.Raises {
			raises[i] = ref(Ref(r))
		}
		l += " raises " + strings.Join(raises, ", ")
	}
	if o.Handles != nil {
		l += " handles " + ref(Ref(*o.Handles))
	}
	if o.Summary != nil {
		l += " " + quote(string(*o.Summary))
	}
	return l
}

func names(ns []Lower, sep string) string {
	s := make([]string, len(ns))
	for i, n := range ns {
		s[i] = string(n)
	}
	return strings.Join(s, sep)
}

// value writes v after prefix, then suffix: on one line when that line fits in
// maxWidth, otherwise with each item on its own line and a comma after each.
func (pr *printer) value(prefix string, v Value, suffix string) {
	flat := flatValue(v)
	open, items, closer := parts(v)
	if len(items) == 0 || 2*pr.indent+widths.StringWidth(prefix+flat+suffix) <= maxWidth {
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
			items = append(items, part{prefix: keyText(e.Key) + ": ", value: e.Value})
		}
		return "{", items, "}"
	case *CallValue:
		for _, a := range v.Args {
			items = append(items, part{prefix: string(a.Name) + ": ", value: a.Value})
		}
		return string(v.Op) + "(", items, ")"
	}
	return "", nil, ""
}

func keyText(k Key) string {
	switch {
	case k.Name != nil:
		return string(*k.Name)
	case k.String != nil:
		return quote(string(*k.String))
	}
	return string(*k.Number)
}

func flatValue(v Value) string {
	switch v := v.(type) {
	case *NumberValue:
		return string(v.Text)
	case *StringValue:
		return quote(string(v.Text))
	case *BoolValue:
		if v.Value {
			return "true"
		}
		return "false"
	case *NoneValue:
		return "none"
	case *NameValue:
		return string(v.Name)
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
	case *Builtin:
		return t.Name
	case *NamedType:
		return ref(Ref(t.Ref))
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

func args(as []Lower) string { return "(" + names(as, ", ") + ")" }
