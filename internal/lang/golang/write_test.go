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
// each call's callee are, and which module packages the file imports.
type writer struct {
	file      *gfile
	b         strings.Builder
	line, col int
	imports   map[string]bool // by path
	at        map[any]diag.Position
}

// write writes a file twice: once to learn its imports, and once with them.
func write(f *gfile) *writer {
	first := &writer{file: f, line: 1, col: 1, imports: map[string]bool{}, at: map[any]diag.Position{}}
	first.decls()
	out := &writer{file: f, line: 1, col: 1, imports: map[string]bool{}, at: map[any]diag.Position{}}
	out.comments("", f.doc)
	out.at[f] = out.pos()
	out.put("package " + f.pkg.name + "\n")
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

func (r *writer) comments(indent string, lines []string) {
	for _, line := range lines {
		r.put(indent + line + "\n")
	}
}

func (r *writer) decls() {
	for _, decl := range r.file.decls {
		r.put("\n")
		switch decl := decl.(type) {
		case *gtype:
			r.comments("", docLines(decl.doc))
			r.put("type ")
			r.typeSpec(decl, "")
		case *ggroup:
			r.comments("", decl.doc)
			r.put("type (\n")
			for _, ty := range decl.types {
				r.comments("\t", docLines(ty.doc))
				r.put("\t")
				r.typeSpec(ty, "\t")
			}
			r.put(")\n")
		case *gfunc:
			r.function(decl)
		case *gloose:
			r.loose(decl)
		}
	}
}

func (r *writer) typeSpec(ty *gtype, indent string) {
	r.at[ty] = r.pos()
	r.put(ty.name + " ")
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
			r.comments(indent+"\t", field.doc)
			r.put(indent + "\t" + field.name + " " + r.goType(field.typ) + "\n")
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
			r.put(indent + "\t" + sig.name + r.goSig(sig) + "\n")
		}
		r.put(indent + "}\n")
	default:
		r.put(r.goType(ty.under) + "\n")
	}
}

func (r *writer) function(fn *gfunc) {
	r.comments("", docLines(fn.doc))
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
	for i := range fn.body {
		r.stmt(&fn.body[i], "\t")
	}
	if fn.terminal != "" {
		r.put("\tpanic(" + strconv.Quote(fn.terminal) + ")\n")
	}
	r.put("}\n")
}

func (r *writer) stmt(s *gstmt, indent string) {
	switch s.kind {
	case "call":
		r.put(indent)
		if s.via != nil {
			r.put("new(" + r.qualified(s.via.file.pkg, s.via.name) + ").")
		} else {
			r.put(r.qualified(s.target.file.pkg, ""))
		}
		r.at[s] = r.pos()
		r.put(s.target.sig.name + "(" + r.args(s.target.sig) + ")\n")
	case "iface":
		r.put(indent + "(*new(" + r.qualified(s.via.file.pkg, s.via.name) + "))." + s.method.name + "(" + r.args(s.method) + ")\n")
	case "closure":
		r.put(indent + "func() {\n")
		r.stmt(s.inner, indent+"\t")
		r.put(indent + "}()\n")
	case "builtin":
		r.put(indent + "_ = len(\"\")\n")
	case "ref":
		r.put(indent + "_ = " + r.qualified(s.target.file.pkg, s.target.sig.name) + "\n")
	case "rapid":
		r.imports["pgregory.net/rapid"] = true
		r.put(indent + "rapid.Check(t, func(*rapid.T) {})\n")
	case "comment":
		r.put(indent + s.text + "\n")
	}
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
		r.put("var " + l.name + " = 1 " + l.doc[0] + "\n")
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
	for _, res := range sig.results {
		results = append(results, r.goType(res))
	}
	out := "(" + strings.Join(params, ", ") + ")"
	switch len(results) {
	case 0:
	case 1:
		out += " " + results[0]
	default:
		out += " (" + strings.Join(results, ", ") + ")"
	}
	return out
}

func (r *writer) args(sig *gsig) string {
	var args []string
	for _, p := range sig.params {
		args = append(args, "*new("+r.goType(p.typ)+")")
	}
	return strings.Join(args, ", ")
}
