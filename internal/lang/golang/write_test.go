package golang

import (
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/lincyaw/unicode/internal/diag"
)

// This file writes the model of a module as Go source, recording where each
// declaration's name and each call's callee are.

// writer writes one file and records where each declaration's name and
// each call's callee are, and which module packages the file imports. A
// plain writer writes no comments and no layout, as the normal form of
// docs/design.md §5.2 reads the source.
type writer struct {
	file      *gfile
	plain     bool
	b         strings.Builder
	line, col int
	imports   map[string]bool // by path
	at        map[any]diag.Position
}

func newWriter(f *gfile, plain bool) *writer {
	return &writer{file: f, plain: plain, line: 1, col: 1, imports: map[string]bool{}, at: map[any]diag.Position{}}
}

// write writes a file twice: once to learn its imports, and once with them.
func write(f *gfile) *writer {
	first := newWriter(f, false)
	first.decls()
	out := newWriter(f, false)
	if f.header != "" && !f.headerAfterPackage {
		out.put(f.header + "\n\n")
	}
	out.comments("", f.doc)
	out.at[f] = out.pos()
	out.put("package " + f.pkg.name + "\n")
	if f.header != "" && f.headerAfterPackage {
		out.put("\n" + f.header + "\n")
	}
	if len(first.imports) > 0 {
		out.put("\nimport (\n")
		for _, path := range slices.Sorted(maps.Keys(first.imports)) {
			out.put("\t" + strconv.Quote(path) + "\n")
		}
		out.put(")\n")
	}
	out.decls()
	return out
}

func (r *writer) put(s string) {
	r.b.WriteString(s)
	for _, c := range []byte(s) {
		if c == '\n' {
			r.line, r.col = r.line+1, 1
		} else {
			r.col++
		}
	}
}

func (r *writer) pos() diag.Position {
	return diag.Position{Path: r.file.path, Line: r.line, Column: r.col}
}

// plainBody writes a function's body as its normal form reads it, so that
// two bodies have one digest exactly when they write the same.
func plainBody(fn *gfunc) string {
	r := newWriter(fn.file, true)
	r.body(fn)
	return r.b.String()
}

// plainType writes what a type declares, without its name, as its normal
// form reads it.
func plainType(ty *gtype) string {
	r := newWriter(ty.file, true)
	r.typeBody(ty, "")
	return r.b.String()
}

// comments writes comment lines, recording where each starts.
func (r *writer) comments(indent string, lines []binding) {
	if r.plain {
		return
	}
	for i := range lines {
		r.put(indent)
		r.at[&lines[i]] = r.pos()
		r.put(lines[i].text + "\n")
	}
}

func (r *writer) decls() {
	for _, decl := range r.file.decls {
		r.put("\n")
		switch decl := decl.(type) {
		case *gtype:
			r.comments("", decl.doc)
			r.put("type ")
			r.typeSpec(decl, "")
		case *ggroup:
			r.comments("", decl.doc)
			r.put("type (\n")
			for _, ty := range decl.types {
				r.comments("\t", ty.doc)
				r.put("\t")
				r.typeSpec(ty, "\t")
			}
			r.put(")\n")
		case *gfunc:
			r.function(decl)
		case *gconst:
			r.put("const " + decl.name + " " + decl.typ.name + " = " + strconv.Quote(decl.name) + "\n")
		case *gloose:
			r.loose(decl)
		}
	}
}

func (r *writer) typeSpec(ty *gtype, indent string) {
	r.at[ty] = r.pos()
	r.put(ty.name + " ")
	r.typeBody(ty, indent)
}

func (r *writer) typeBody(ty *gtype, indent string) {
	switch {
	case ty.form == "struct" && len(ty.fields) == 0 && ty.embed == nil:
		r.put("struct{}\n")
	case ty.form == "struct":
		r.put("struct {\n")
		if ty.embed != nil {
			star := ""
			if ty.embedPtr {
				star = "*"
			}
			r.put(indent + "\t" + star + r.qualified(ty.embed.file.pkg, ty.embed.name) + "\n")
		}
		for _, field := range ty.fields {
			r.blank(field.layout)
			r.comments(indent+"\t", field.doc)
			r.put(indent + "\t" + field.name + " " + r.goType(field.typ))
			r.end(field.layout)
		}
		r.put(indent + "}\n")
	case ty.form == "interface" && len(ty.embeds) == 0 && len(ty.sigs) == 0:
		r.put("interface{}\n")
	case ty.form == "interface":
		r.put("interface {\n")
		for _, e := range ty.embeds {
			r.put(indent + "\t" + r.qualified(e.file.pkg, e.name) + "\n")
		}
		for _, sig := range ty.sigs {
			r.comments(indent+"\t", sig.doc)
			r.put(indent + "\t")
			r.at[sig] = r.pos()
			r.put(sig.name + r.goSig(sig) + "\n")
		}
		r.put(indent + "}\n")
	default:
		r.put(r.goType(ty.under) + "\n")
	}
}

func (r *writer) function(fn *gfunc) {
	r.comments("", fn.doc)
	r.put("func ")
	if fn.recv != nil {
		star := ""
		if fn.ptrRecv {
			star = "*"
		}
		r.put("(r " + star + fn.recv.name + ") ")
	}
	r.at[fn] = r.pos()
	r.put(fn.sig.name + r.goSig(fn.sig) + " {\n")
	r.body(fn)
	r.put("}\n")
}

func (r *writer) body(fn *gfunc) {
	for i := range fn.body {
		r.stmt(&fn.body[i], "\t")
	}
	if fn.terminal != "" {
		r.put("\tpanic(" + strconv.Quote(fn.terminal) + ")\n")
	}
}

func (r *writer) stmt(s *gstmt, indent string) {
	r.blank(s.layout)
	switch s.kind {
	case "call", "assign":
		r.put(indent)
		if s.kind == "assign" {
			r.put(s.name + " := ")
		}
		if s.via != nil {
			r.put("new(" + r.qualified(s.via.file.pkg, s.via.name) + ").")
		} else {
			r.put(r.qualified(s.target.file.pkg, ""))
		}
		r.at[s] = r.pos()
		r.put(s.target.sig.name + r.args(s.target.sig, s.layout, indent))
		r.end(s.layout)
		if s.kind == "assign" {
			r.put(indent + "_ = " + s.name + "\n")
		}
	case "iface":
		r.put(indent + "(*new(" + r.qualified(s.via.file.pkg, s.via.name) + ")).")
		r.at[s] = r.pos()
		r.put(s.method.name + r.args(s.method, s.layout, indent))
		r.end(s.layout)
	case "closure":
		r.put(indent + "func() {\n")
		r.stmt(s.inner, indent+"\t")
		r.put(indent + "}()")
		r.end(s.layout)
	case "builtin":
		r.put(indent + "_ = len(\"\")")
		r.end(s.layout)
	case "ref":
		r.put(indent + "_ = " + r.qualified(s.target.file.pkg, s.target.sig.name))
		r.end(s.layout)
	case "rapid":
		r.imports["pgregory.net/rapid"] = true
		r.put(indent + "rapid.Check(t, func(*rapid.T) {})")
		r.end(s.layout)
	case "comment":
		if r.plain {
			return
		}
		r.put(indent)
		r.at[&s.comment] = r.pos()
		r.put(s.comment.text + "\n")
	}
}

// blank writes the blank line a layout puts before its statement or field.
func (r *writer) blank(l layout) {
	if l.blank && !r.plain {
		r.put("\n")
	}
}

// end ends the line of a statement or a field, with the comment its layout
// puts after it.
func (r *writer) end(l layout) {
	if l.note && !r.plain {
		r.put(" // note")
	}
	r.put("\n")
}

func (r *writer) loose(l *gloose) {
	switch l.kind {
	case "const":
		r.comments("", l.doc)
		r.put("const " + l.name + " = 1\n")
	case "var":
		r.comments("", l.doc)
		r.put("var " + l.name + " = 1\n")
	case "trailing":
		r.put("var " + l.name + " = 1 ")
		r.at[&l.doc[0]] = r.pos()
		r.put(l.doc[0].text + "\n")
	case "init":
		r.comments("", l.doc)
		r.put("func init() {}\n")
	default:
		r.comments("", l.doc)
	}
}

// qualified writes a name declared in p as the file names it, importing p.
func (r *writer) qualified(p *gpkg, name string) string {
	if p == r.file.pkg {
		return name
	}
	r.imports[p.path] = true
	return p.name + "." + name
}

func (r *writer) goType(e *gexpr) string {
	switch e.kind {
	case "basic", "raw":
		return e.basic
	case "named":
		return r.qualified(e.named.file.pkg, e.named.name)
	case "ext":
		r.imports[e.ext[0]] = true
		return e.ext[1] + "." + e.ext[2]
	case "time", "duration":
		r.imports["time"] = true
		return map[string]string{"time": "time.Time", "duration": "time.Duration"}[e.kind]
	case "error":
		return "error"
	case "pointer":
		return "*" + r.goType(e.elem)
	case "slice":
		return "[]" + r.goType(e.elem)
	case "bytes":
		return "[]byte"
	case "map":
		return "map[" + e.basic + "]" + r.goType(e.elem)
	default:
		return "map[" + e.basic + "]struct{}"
	}
}

func (r *writer) goSig(sig *gsig) string {
	var params, results []string
	for _, p := range sig.params {
		params = append(params, strings.TrimPrefix(p.name+" "+r.goType(p.typ), " "))
	}
	for i, res := range sig.results {
		written := r.goType(res)
		if len(sig.resultNames) > 0 {
			written = sig.resultNames[i] + " " + written
		}
		results = append(results, written)
	}
	out := "(" + strings.Join(params, ", ") + ")"
	switch len(results) {
	case 0:
	case 1:
		if len(sig.resultNames) > 0 {
			out += " (" + results[0] + ")"
		} else {
			out += " " + results[0]
		}
	default:
		out += " (" + strings.Join(results, ", ") + ")"
	}
	return out
}

// args writes a call's arguments in parentheses, one to a line when the
// layout splits them.
func (r *writer) args(sig *gsig, l layout, indent string) string {
	var args []string
	for _, p := range sig.params {
		args = append(args, "*new("+r.goType(p.typ)+")")
	}
	if !l.split || r.plain || len(args) == 0 {
		return "(" + strings.Join(args, ", ") + ")"
	}
	inner := indent + "\t"
	return "(\n" + inner + strings.Join(args, ",\n"+inner) + ",\n" + indent + ")"
}
