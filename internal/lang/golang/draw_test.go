package golang

import (
	"cmp"
	"go/token"
	"maps"
	"slices"
	"strconv"

	"pgregory.net/rapid"
)

// This file draws the declarations of a random module: types, methods,
// functions, their bodies, and their doc comments with bindings.

var basics = []string{"bool", "int", "float64", "string", "int64"}

func (w *world) drawExpr(from *gfile, depth int) *gexpr {
	var named []*gtype
	for _, ty := range w.types {
		if typeVisible(from, ty) {
			named = append(named, ty)
		}
	}
	kinds := []string{"basic", "basic", "bytes", "time", "duration", "error", "tb", "testingT", "raw"}
	if len(named) > 0 {
		kinds = append(kinds, "named", "named", "named")
	}
	if depth < 2 {
		kinds = append(kinds, "pointer", "slice", "map", "set")
	}
	switch kind := rapid.SampledFrom(kinds).Draw(w.t, "exprKind"); kind {
	case "basic":
		return &gexpr{kind: kind, basic: rapid.SampledFrom(basics).Draw(w.t, "basic")}
	case "raw":
		return &gexpr{kind: kind, basic: rapid.SampledFrom([]string{"func()", "chan int"}).Draw(w.t, "raw")}
	case "named":
		return &gexpr{kind: kind, named: rapid.SampledFrom(named).Draw(w.t, "named")}
	case "tb":
		return &gexpr{kind: "ext", ext: [3]string{"pgregory.net/rapid", "rapid", "TB"}}
	case "testingT":
		return &gexpr{kind: "pointer", elem: &gexpr{kind: "ext", ext: [3]string{"testing", "testing", "T"}}}
	case "pointer", "slice":
		return &gexpr{kind: kind, elem: w.drawExpr(from, depth+1)}
	case "map":
		return &gexpr{kind: kind, basic: rapid.SampledFrom(basics[:4]).Draw(w.t, "key"), elem: w.drawExpr(from, depth+1)}
	case "set":
		return &gexpr{kind: kind, basic: rapid.SampledFrom(basics[:4]).Draw(w.t, "key")}
	default:
		return &gexpr{kind: kind}
	}
}

// contextExpr is context.Context.
func contextExpr() *gexpr {
	return &gexpr{kind: "ext", ext: [3]string{"context", "context", "Context"}}
}

func (w *world) drawSig(from *gfile, name string) *gsig {
	sig := &gsig{pkg: from.pkg, name: name}
	unnamed := rapid.IntRange(0, 5).Draw(w.t, "unnamed") == 0
	// A leading context.Context carries the call's trace, which the
	// normalized signature leaves out (docs/design.md §4).
	if rapid.IntRange(0, 4).Draw(w.t, "context") == 0 {
		p := gparam{typ: contextExpr()}
		if !unnamed {
			p.name = w.fresh("ctx", false)
		}
		sig.params = append(sig.params, p)
	}
	for range rapid.IntRange(0, 3).Draw(w.t, "params") {
		p := gparam{typ: w.drawExpr(from, 0)}
		if !unnamed {
			p.name = w.fresh("param", false)
			if rapid.IntRange(0, 6).Draw(w.t, "blank") == 0 {
				p.name = "_"
			}
		}
		sig.params = append(sig.params, p)
	}
	for range rapid.IntRange(0, 2).Draw(w.t, "results") {
		sig.results = append(sig.results, w.drawExpr(from, 0))
	}
	if rapid.Bool().Draw(w.t, "raises") {
		sig.results = append(sig.results, &gexpr{kind: "error"})
	}
	sig.resultNames = w.drawResultNames(len(sig.results))
	return sig
}

func (w *world) drawResultNames(count int) []string {
	if count == 0 || !rapid.Bool().Draw(w.t, "namedResults") {
		return nil
	}
	names := make([]string, count)
	for i := range names {
		names[i] = w.fresh("result", false)
	}
	return names
}

func (w *world) drawType() {
	file := w.anyFile("typeFile")
	ty := &gtype{file: file, form: rapid.SampledFrom([]string{"struct", "interface", "defined"}).Draw(w.t, "form")}
	ty.name = w.fresh("typeName", w.exported("typeExported"))
	switch ty.form {
	case "struct":
		if twins := w.structTwins(file); len(twins) > 0 && rapid.IntRange(0, 3).Draw(w.t, "structTwin") == 0 {
			w.copyStruct(ty, rapid.SampledFrom(twins).Draw(w.t, "structTwinOf"))
			break
		}
		for range rapid.IntRange(0, 3).Draw(w.t, "fields") {
			ty.fields = append(ty.fields, w.drawField(gfield{name: w.fresh("field", w.exported("fieldExported")), typ: w.drawExpr(file, 0)}))
		}
		if structs := w.visibleTypes(file, "struct"); len(structs) > 0 && rapid.Bool().Draw(w.t, "embeds") {
			ty.embed = rapid.SampledFrom(structs).Draw(w.t, "embedded")
			ty.embedPtr = rapid.Bool().Draw(w.t, "embedPtr")
		}
	case "interface":
		if ifaces := w.visibleTypes(file, "interface"); len(ifaces) > 0 {
			for range rapid.IntRange(0, 2).Draw(w.t, "embeds") {
				if e := rapid.SampledFrom(ifaces).Draw(w.t, "embeddedIface"); !slices.Contains(ty.embeds, e) {
					ty.embeds = append(ty.embeds, e)
				}
			}
		}
		for range rapid.IntRange(0, 2).Draw(w.t, "sigs") {
			sig := w.drawSig(file, w.fresh("method", w.exported("methodExported")))
			sig.doc = w.drawDoc(sig.name, "method")
			ty.sigs = append(ty.sigs, sig)
		}
	default:
		// Half of them are strings, the shape of an enum.
		ty.under = &gexpr{kind: "basic", basic: "string"}
		if rapid.Bool().Draw(w.t, "notEnum") {
			ty.under = &gexpr{kind: "basic", basic: rapid.SampledFrom(basics).Draw(w.t, "under")}
			if rapid.Bool().Draw(w.t, "underSlice") {
				ty.under = &gexpr{kind: "slice", elem: ty.under}
			}
		}
	}
	ty.doc = w.drawDoc(ty.name, "type")
	w.types = append(w.types, ty)
	if last, ok := lastDecl(file).(*gtype); ok && rapid.IntRange(0, 3).Draw(w.t, "group") == 0 {
		file.decls[len(file.decls)-1] = &ggroup{types: []*gtype{last, ty}, doc: w.misplaced("groupBindings")}
	} else if group, ok := lastDecl(file).(*ggroup); ok && rapid.Bool().Draw(w.t, "joinGroup") {
		group.types = append(group.types, ty)
	} else {
		file.decls = append(file.decls, ty)
	}
}

// drawField gives a field its doc comment and layout, which its type's
// normal form drops.
func (w *world) drawField(field gfield) gfield {
	field.doc = nil
	if rapid.IntRange(0, 5).Draw(w.t, "fieldDoc") == 0 {
		field.doc = w.misplaced("fieldBindings")
	}
	field.layout = w.drawLayout()
	return field
}

// structTwins are the structs that a struct in the file can copy: those of
// its package in files that are test files exactly when it is, which name
// the same types.
func (w *world) structTwins(file *gfile) []*gtype {
	var out []*gtype
	for _, ty := range w.types {
		if ty.form == "struct" && ty.file.pkg == file.pkg && ty.file.test == file.test {
			out = append(out, ty)
		}
	}
	return out
}

// copyStruct gives ty the fields and the embedded struct of another, with
// doc comments and layouts of its own, so that it differs from that one only
// in its name and in what the normal form drops.
func (w *world) copyStruct(ty, of *gtype) {
	for _, field := range of.fields {
		ty.fields = append(ty.fields, w.drawField(field))
	}
	ty.embed, ty.embedPtr = of.embed, of.embedPtr
}

func lastDecl(f *gfile) any {
	if len(f.decls) == 0 {
		return nil
	}
	return f.decls[len(f.decls)-1]
}

func (w *world) visibleTypes(from *gfile, form string) []*gtype {
	var out []*gtype
	for _, ty := range w.types {
		if ty.form == form && typeVisible(from, ty) {
			out = append(out, ty)
		}
	}
	return out
}

// drawMethods gives a struct or defined type the methods of some interfaces
// it can implement, and methods of its own. A type's methods are declared in
// files of its package that are test files exactly when its own file is, so
// that every variant of the package that has the type has all its methods.
func (w *world) drawMethods(ty *gtype) {
	if ty.form == "interface" {
		return
	}
	for _, iface := range w.types {
		if iface.form != "interface" || !w.implementable(ty, iface) || !rapid.Bool().Draw(w.t, "implement") {
			continue
		}
		have := ownMethods(ty)
		for _, sig := range sortedMethods(ifaceMethods(iface)) {
			if _, ok := have[methodID(sig)]; !ok {
				w.addMethod(ty, ty.file, sig)
			}
		}
	}
	var files []*gfile
	for _, f := range ty.file.pkg.files {
		if f.test == ty.file.test {
			files = append(files, f)
		}
	}
	for range rapid.IntRange(0, 2).Draw(w.t, "ownMethods") {
		file := rapid.SampledFrom(files).Draw(w.t, "methodFile")
		w.addMethod(ty, file, w.drawSig(file, w.fresh("method", w.exported("methodExported"))))
	}
}

// implementable tells whether ty can declare every method of iface: each is
// exported or declared in ty's package, and its types can be named in ty's file.
func (w *world) implementable(ty, iface *gtype) bool {
	methods := ifaceMethods(iface)
	if len(methods) == 0 {
		return false
	}
	for _, sig := range methods {
		if !token.IsExported(sig.name) && sig.pkg != ty.file.pkg || !sigVisible(ty.file, sig) {
			return false
		}
	}
	return true
}

// drawConsts gives a type whose underlying type is string constants, in files
// of its package that can name it: most named after the type and a value,
// some not.
func (w *world) drawConsts(ty *gtype) {
	if ty.form != "defined" || ty.under.kind != "basic" || ty.under.basic != "string" {
		return
	}
	var files []*gfile
	for _, f := range ty.file.pkg.files {
		if typeVisible(f, ty) {
			files = append(files, f)
		}
	}
	for range rapid.IntRange(0, 3).Draw(w.t, "consts") {
		name := ty.name + w.fresh("value", true)
		if rapid.IntRange(0, 3).Draw(w.t, "unprefixed") == 0 {
			name = w.fresh("const", w.exported("constExported"))
		}
		file := rapid.SampledFrom(files).Draw(w.t, "constFile")
		file.decls = append(file.decls, &gconst{typ: ty, name: name})
	}
}

func (w *world) addMethod(ty *gtype, file *gfile, sig *gsig) {
	// Implementation names vary independently from the interface's names.
	copy := *sig
	copy.pkg = file.pkg
	copy.params = slices.Clone(sig.params)
	copy.results = slices.Clone(sig.results)
	named := rapid.Bool().Draw(w.t, "methodNamedParams")
	for i := range copy.params {
		copy.params[i].name = ""
		if named {
			copy.params[i].name = w.fresh("methodParam", false)
		}
	}
	copy.resultNames = w.drawResultNames(len(copy.results))
	// A changed native type or arity must not become an implementation.
	if rapid.IntRange(0, 5).Draw(w.t, "methodTypeMismatch") == 0 {
		replacement := func(before *gexpr) *gexpr {
			basic := "int"
			if before.kind == "basic" && before.basic == "int" {
				basic = "string"
			}
			return &gexpr{kind: "basic", basic: basic}
		}
		switch {
		case len(copy.params) > 0:
			copy.params[0].typ = replacement(copy.params[0].typ)
		case len(copy.results) > 0:
			copy.results[0] = replacement(copy.results[0])
		default:
			copy.params = append(copy.params, gparam{typ: &gexpr{kind: "basic", basic: "int"}})
		}
	}
	sig = &copy
	fn := &gfunc{file: file, recv: ty, ptrRecv: rapid.Bool().Draw(w.t, "ptrRecv"), sig: sig, varFree: rapid.Bool().Draw(w.t, "varFree")}
	fn.doc = w.drawDoc(sig.name, "method")
	ty.methods = append(ty.methods, fn)
	w.funcs = append(w.funcs, fn)
	file.decls = append(file.decls, fn)
}

func (w *world) drawFunc(file *gfile, testName string) {
	var sig *gsig
	switch testName {
	case "Test", "Testlower":
		param := &gexpr{kind: "pointer", elem: &gexpr{kind: "ext", ext: [3]string{"testing", "testing", "T"}}}
		sig = &gsig{pkg: file.pkg, params: []gparam{{name: "t", typ: param}}}
	case "Fuzz":
		param := &gexpr{kind: "pointer", elem: &gexpr{kind: "ext", ext: [3]string{"testing", "testing", "F"}}}
		sig = &gsig{pkg: file.pkg, params: []gparam{{name: "f", typ: param}}}
	default:
		// Some are named as predicates, by one camel hump or two, of a property or a flow.
		name := w.fresh("func", w.exported("funcExported"))
		switch rapid.IntRange(0, 5).Draw(w.t, "predicateName") {
		case 0:
			name = "check" + w.fresh("hump", true)
		case 1:
			name = "checkFlow" + w.fresh("hump", true)
		case 2:
			name = "check" + w.fresh("hump", true) + w.fresh("hump", true)
		case 3:
			name = "check" + w.fresh("hump", false)
		}
		sig = w.drawSig(file, name)
	}
	switch testName {
	case "Test", "Fuzz":
		sig.name = testName + w.fresh("testName", true)
		if rapid.IntRange(0, 4).Draw(w.t, "bare") == 0 {
			sig.name = testName + strconv.Itoa(w.count)
		}
	case "Testlower":
		sig.name = "Test" + w.fresh("testName", false)
	}
	fn := &gfunc{file: file, sig: sig, testName: testName, varFree: rapid.Bool().Draw(w.t, "varFree")}
	fn.doc = w.drawDoc(sig.name, "func")
	w.funcs = append(w.funcs, fn)
	file.decls = append(file.decls, fn)
}

// Bodies.

func (w *world) drawBody(fn *gfunc) {
	if twins := w.bodyTwins(fn); len(twins) > 0 && rapid.IntRange(0, 3).Draw(w.t, "bodyTwin") == 0 {
		w.copyBody(fn, rapid.SampledFrom(twins).Draw(w.t, "bodyTwinOf"))
		return
	}
	if rapid.IntRange(0, 4).Draw(w.t, "stub") == 0 {
		fn.terminal = marker
		return
	}
	fn.body = w.drawStmts(w.scopeOf(fn))
	if len(fn.sig.results) > 0 || rapid.Bool().Draw(w.t, "terminal") {
		fn.terminal = rapid.SampledFrom([]string{marker, "done"}).Draw(w.t, "panic")
	}
}

// scopeOf is what a function's body may name: a var-free function calls
// only var-free functions and uses no variable.
func (w *world) scopeOf(fn *gfunc) scope {
	sc := scope{file: fn.file, freeOnly: fn.varFree, tests: fn.testName == "Test" || fn.testName == "Testlower"}
	if !fn.varFree {
		sc.vars = w.vars
	}
	return sc
}

func (w *world) drawStmts(sc scope) []gstmt {
	var stmts []gstmt
	for range rapid.IntRange(0, 3).Draw(w.t, "stmts") {
		if stmt, ok := w.drawStmt(sc, 0); ok {
			stmts = append(stmts, stmt)
		}
	}
	return stmts
}

// drawVar draws a package-level variable in any file: closures with
// statements of their own and references to var-free functions and methods,
// or a call that runs when the package is initialized, of a var-free
// function with one result.
func (w *world) drawVar() {
	file := w.anyFile("varFile")
	v := &gvar{file: file, name: w.fresh("var", w.exported("varExported")),
		form: rapid.SampledFrom([]string{"func", "map", "ref", "init"}).Draw(w.t, "varForm")}
	sc := scope{file: file, freeOnly: true, vars: slices.Clone(w.vars)}
	switch v.form {
	case "init":
		var calls []gstmt
		for _, c := range w.calls(sc) {
			if len(c.target.sig.results) == 1 {
				calls = append(calls, c)
			}
		}
		if len(calls) == 0 {
			return
		}
		c := rapid.SampledFrom(calls).Draw(w.t, "initCall")
		v.init = &c
	case "ref":
		refs := w.calls(sc)
		if len(refs) == 0 {
			return
		}
		ref := rapid.SampledFrom(refs).Draw(w.t, "stored")
		v.entries = []gentry{{ref: &ref}}
	case "func":
		v.entries = []gentry{{body: w.drawStmts(sc)}}
	default:
		for range rapid.IntRange(1, 3).Draw(w.t, "entries") {
			v.entries = append(v.entries, w.drawEntry(sc))
		}
	}
	w.vars = append(w.vars, v)
	file.decls = append(file.decls, v)
}

// drawEntry draws a value of a map: a reference, when there is a function it
// may refer to, or a function literal.
func (w *world) drawEntry(sc scope) gentry {
	if refs := w.calls(sc); len(refs) > 0 && rapid.Bool().Draw(w.t, "entryRef") {
		ref := rapid.SampledFrom(refs).Draw(w.t, "entry")
		return gentry{ref: &ref}
	}
	return gentry{body: w.drawStmts(sc)}
}

// bodyTwins are the functions whose bodies are drawn before fn's and that fn
// can copy: in its file, named like a test exactly when fn is, so that a call
// of rapid.Check finds its t, var-free when fn is, and ending in a panic when
// fn has results.
func (w *world) bodyTwins(fn *gfunc) []*gfunc {
	var out []*gfunc
	for _, other := range w.funcs[:slices.Index(w.funcs, fn)] {
		if other.file == fn.file && other.testName == fn.testName && (!fn.varFree || other.varFree) &&
			(len(fn.sig.results) == 0 || other.terminal != "") {
			out = append(out, other)
		}
	}
	return out
}

// copyBody gives fn the statements and the panic of another body, with
// layouts and comment lines of its own, so that the two differ only in what
// the normal form drops.
func (w *world) copyBody(fn, of *gfunc) {
	fn.terminal = of.terminal
	for _, s := range of.body {
		if s.kind != "comment" {
			fn.body = append(fn.body, w.relayout(s))
		}
	}
	if rapid.Bool().Draw(w.t, "twinComment") {
		at := rapid.IntRange(0, len(fn.body)).Draw(w.t, "twinCommentAt")
		fn.body = slices.Insert(fn.body, at, gstmt{kind: "comment", comment: w.drawBinding("bodyBinding", "func")})
	}
}

// relayout copies a statement with a layout of its own, its closure's too.
func (w *world) relayout(s gstmt) gstmt {
	s.layout = w.drawLayout()
	if s.inner != nil {
		inner := w.relayout(*s.inner)
		s.inner = &inner
	}
	return s
}

func (w *world) drawLayout() layout {
	return layout{
		blank: rapid.IntRange(0, 4).Draw(w.t, "blank") == 0,
		note:  rapid.IntRange(0, 4).Draw(w.t, "note") == 0,
		split: rapid.Bool().Draw(w.t, "split"),
	}
}

func (w *world) drawStmt(sc scope, depth int) (gstmt, bool) {
	s, ok := w.drawContent(sc, depth)
	s.layout = w.drawLayout()
	return s, ok
}

// drawContent draws what a statement does.
func (w *world) drawContent(sc scope, depth int) (gstmt, bool) {
	kinds := []string{"call", "call", "call", "assign", "iface", "builtin", "ref", "ref", "ref", "comment", "varCall", "varCall", "varRef", "varRef"}
	if depth == 0 {
		kinds = append(kinds, "closure")
	}
	if sc.tests {
		kinds = append(kinds, "rapid", "rapid", "rapid")
	}
	switch kind := rapid.SampledFrom(kinds).Draw(w.t, "stmt"); kind {
	case "call":
		calls := w.calls(sc)
		if len(calls) == 0 {
			return gstmt{}, false
		}
		return rapid.SampledFrom(calls).Draw(w.t, "callee"), true
	case "assign":
		var calls []gstmt
		for _, c := range w.calls(sc) {
			if len(c.target.sig.results) == 1 {
				calls = append(calls, gstmt{kind: kind, target: c.target, via: c.via})
			}
		}
		if len(calls) == 0 {
			return gstmt{}, false
		}
		call := rapid.SampledFrom(calls).Draw(w.t, "assigned")
		call.name = w.fresh("variable", false)
		return call, true
	case "iface":
		calls := w.interfaceCalls(sc.file)
		if len(calls) == 0 {
			return gstmt{}, false
		}
		return rapid.SampledFrom(calls).Draw(w.t, "ifaceCall"), true
	case "ref":
		refs := w.references(sc)
		if len(refs) == 0 {
			return gstmt{}, false
		}
		return rapid.SampledFrom(refs).Draw(w.t, "ref"), true
	case "closure":
		inner, ok := w.drawStmt(sc, depth+1)
		return gstmt{kind: kind, inner: &inner}, ok
	case "varCall", "varRef":
		var held []*gvar
		for _, v := range sc.vars {
			if visible(sc.file, v.file, v.name) && (kind == "varRef" || v.form == "func" || v.form == "map") {
				held = append(held, v)
			}
		}
		if len(held) == 0 {
			return gstmt{}, false
		}
		return gstmt{kind: kind, held: rapid.SampledFrom(held).Draw(w.t, "held")}, true
	case "comment":
		return gstmt{kind: kind, comment: w.drawBinding("bodyBinding", "func")}, true
	default:
		return gstmt{kind: kind}, true
	}
}

// calls are the calls a scope can write.
func (w *world) calls(sc scope) []gstmt {
	var out []gstmt
	for _, c := range w.callables(sc.file) {
		if !sc.freeOnly || c.target.varFree {
			out = append(out, c)
		}
	}
	return out
}

// callables are the calls a file can write: to every function and method it
// can name, a method also through a struct that embeds its type.
func (w *world) callables(from *gfile) []gstmt {
	var calls []gstmt
	for _, fn := range w.funcs {
		if !visible(from, fn.file, fn.sig.name) || !sigVisible(from, fn.sig) {
			continue
		}
		if fn.recv == nil {
			calls = append(calls, gstmt{kind: "call", target: fn})
			continue
		}
		if typeVisible(from, fn.recv) {
			calls = append(calls, gstmt{kind: "call", target: fn, via: fn.recv})
		}
		for _, outer := range w.types {
			if _, shadowed := ownMethods(outer)[methodID(fn.sig)]; outer.embed == fn.recv && !shadowed &&
				typeVisible(from, outer) {
				calls = append(calls, gstmt{kind: "call", target: fn, via: outer})
			}
		}
	}
	return calls
}

// references are the references outside a call a scope can write: to a
// function by its name, and to a method, an interface's too, as a method
// value or a method expression.
func (w *world) references(sc scope) []gstmt {
	var refs []gstmt
	for _, c := range w.calls(sc) {
		c.kind = "ref"
		refs = append(refs, c)
		if c.via != nil {
			c.onType = true
			refs = append(refs, c)
		}
	}
	for _, c := range w.interfaceCalls(sc.file) {
		c.kind = "ifaceRef"
		refs = append(refs, c)
		c.onType = true
		refs = append(refs, c)
	}
	return refs
}

func (w *world) interfaceCalls(from *gfile) []gstmt {
	var calls []gstmt
	for _, iface := range w.visibleTypes(from, "interface") {
		for _, sig := range sortedMethods(ifaceMethods(iface)) {
			if (token.IsExported(sig.name) || sig.pkg == from.pkg) && sigVisible(from, sig) {
				calls = append(calls, gstmt{kind: "iface", via: iface, method: sig})
			}
		}
	}
	return calls
}

// Method sets, keyed by the method's identity: its name, qualified by its
// package's path when it is not exported.

func methodID(sig *gsig) string {
	if token.IsExported(sig.name) {
		return sig.name
	}
	return sig.pkg.path + "." + sig.name
}

func ownMethods(ty *gtype) map[string]*gsig {
	out := map[string]*gsig{}
	for _, m := range ty.methods {
		out[methodID(m.sig)] = m.sig
	}
	return out
}

// methodSet is the method set of a pointer to ty: its own methods and those
// promoted from the struct it embeds, which its own shadow.
func methodSet(ty *gtype) map[string]*gsig {
	out := ownMethods(ty)
	if ty.embed != nil {
		for id, sig := range methodSet(ty.embed) {
			if _, ok := out[id]; !ok {
				out[id] = sig
			}
		}
	}
	return out
}

func ifaceMethods(ty *gtype) map[string]*gsig {
	out := map[string]*gsig{}
	for _, e := range ty.embeds {
		maps.Copy(out, ifaceMethods(e))
	}
	for _, sig := range ty.sigs {
		out[methodID(sig)] = sig
	}
	return out
}

func sortedMethods(set map[string]*gsig) []*gsig {
	return slices.SortedFunc(maps.Values(set), func(a, b *gsig) int { return cmp.Compare(methodID(a), methodID(b)) })
}

// Bindings and docs.

// The diagnostics of bindings that are not well-formed or not in place.
const (
	codeUnknownVerb = "lang.golang.unknown-binding-verb"
	codeMissingID   = "lang.golang.missing-binding-id"
	codeInvalidID   = "lang.golang.invalid-binding-id"
	codeMisplaced   = "lang.golang.misplaced-binding"
	codeDuplicate   = "lang.golang.duplicate-binding"
)

// malformed writes a binding that is not well-formed, whatever its place.
var malformed = []func(w *world, verb, id string) binding{
	func(w *world, _, id string) binding {
		verb := rapid.SampledFrom([]string{"realize", "Realizes", "check", "generate", "uses", ""}).Draw(w.t, "verb")
		return binding{text: "//uc:" + verb + " " + id, code: codeUnknownVerb}
	},
	func(_ *world, verb, id string) binding {
		return binding{text: "//uc:" + verb + "\t" + id, code: codeUnknownVerb}
	},
	func(_ *world, verb, _ string) binding { return binding{text: "//uc:" + verb, code: codeMissingID} },
	func(_ *world, verb, _ string) binding {
		return binding{text: "//uc:" + verb + " ", code: codeMissingID}
	},
	func(_ *world, verb, id string) binding {
		return binding{text: "//uc:" + verb + "  " + id, code: codeInvalidID}
	},
	func(_ *world, verb, id string) binding {
		return binding{text: "//uc:" + verb + " " + id + " ", code: codeInvalidID}
	},
	func(_ *world, verb, id string) binding {
		return binding{text: "//uc:" + verb + " " + id + " more", code: codeInvalidID}
	},
	func(w *world, verb, _ string) binding {
		id := rapid.SampledFrom([]string{"a..B", ".a", "a.", "a_b", "9a", "A-b", "a.-b"}).Draw(w.t, "badID")
		return binding{text: "//uc:" + verb + " " + id, code: codeInvalidID}
	},
	func(w *world, _, _ string) binding {
		id := rapid.SampledFrom([]string{"order", "order.Order", "order.total-is-sum"}).Draw(w.t, "badSymbol")
		return binding{text: "//uc:generates " + id, code: codeInvalidID}
	},
}

// binding is one line of a comment. A well-formed binding has its verb and
// target; one that is not has the code of its diagnostic.
type binding struct {
	text, verb, target, code string
}

// drawBinding draws a binding fit for a type or a function, sometimes one
// that is not well-formed.
func (w *world) drawBinding(label, on string) binding {
	verb := "realizes"
	if on != "type" {
		verb = rapid.SampledFrom([]string{"realizes", "checks", "generates", "observes", "publishes"}).Draw(w.t, label+"Verb")
	}
	component := rapid.StringMatching(`[a-z]{1,3}(-[a-z]{1,2})?(\.[a-z]{1,3})?`).Draw(w.t, label+"Component")
	var id string
	switch {
	case verb == "realizes" && on == "type":
		id = component + "." + rapid.StringMatching(`[A-Z][a-zA-Z]{0,3}`).Draw(w.t, label+"Type")
	case verb == "realizes":
		id = component + "." + rapid.StringMatching(`[a-z][a-zA-Z]{0,3}`).Draw(w.t, label+"Op")
	case verb == "publishes":
		id = component + "." + rapid.StringMatching(`[A-Z][a-zA-Z]{0,3}`).Draw(w.t, label+"Event")
	case verb == "checks":
		id = component + "." + rapid.StringMatching(`[a-z]{1,3}(-[a-z]{1,3}){0,2}`).Draw(w.t, label+"Property")
	default:
		id = component + "." + rapid.StringMatching(`[a-z]{1,3}(-[a-z]{1,3}){0,1}\.[a-z][a-zA-Z]{0,3}`).Draw(w.t, label+"Symbol")
	}
	if rapid.IntRange(0, 4).Draw(w.t, label+"Malformed") == 0 {
		return rapid.SampledFrom(malformed).Draw(w.t, label+"How")(w, verb, id)
	}
	return binding{text: "//uc:" + verb + " " + id, verb: verb, target: id}
}

// drawDoc draws a declaration's doc comment: maybe a sentence, then maybe
// bindings, some of which repeat an earlier one or are not well-formed. A
// doc with bindings always starts with the sentence, so that the summary is it.
func (w *world) drawDoc(name, on string) []binding {
	var doc []binding
	var bindings []binding
	for range rapid.IntRange(0, 3).Draw(w.t, "bindings") {
		b := w.drawBinding("binding", on)
		switch k := rapid.IntRange(0, 9).Draw(w.t, "bindingVariant"); {
		case k == 0 && len(bindings) > 0:
			b = rapid.SampledFrom(bindings).Draw(w.t, "repeated")
		case k == 1 && on == "type":
			b = w.drawBinding("misfit", "func")
		case k == 2:
			b = binding{text: rapid.SampledFrom([]string{"// uc:realizes a.B", "/*uc:realizes a.B*/"}).Draw(w.t, "notBinding")}
		}
		bindings = append(bindings, b)
	}
	if len(bindings) > 0 || rapid.Bool().Draw(w.t, "prose") {
		doc = append(doc, binding{text: "// " + name + w.words("words") + "."})
	}
	return append(doc, bindings...)
}

func (w *world) words(label string) string {
	return rapid.StringMatching(`( [a-z]{1,5}){0,3}`).Draw(w.t, label)
}

// misplaced draws bindings for a place that binds nothing.
func (w *world) misplaced(label string) []binding {
	var lines []binding
	for range rapid.IntRange(0, 1).Draw(w.t, label) {
		lines = append(lines, w.drawBinding(label, "func"))
	}
	return lines
}

// drawLoose adds declarations that are no nodes, each carrying a binding,
// then sometimes copies a file's to a file of another package.
func (w *world) drawLoose() {
	for range rapid.IntRange(0, 2).Draw(w.t, "loose") {
		file := w.anyFile("looseFile")
		kind := rapid.SampledFrom([]string{"const", "var", "trailing", "alias", "init", "blank", "floating"}).Draw(w.t, "looseKind")
		file.decls = append(file.decls, &gloose{
			kind: kind, name: w.fresh("loose", false), doc: []binding{w.drawBinding("looseBinding", "func")},
		})
	}
	w.copyLoose()
}

// copyLoose gives a file that has no globals the loose declarations of a
// file in another package whose globals are only loose ones, with comments of their own and a
// trailing comment or none, so that the two files' globals differ only in
// what the normal form drops.
func (w *world) copyLoose() {
	var from, to []*gfile
	for _, f := range w.files() {
		if !declaresGlobals(f) {
			to = append(to, f)
		} else if !slices.ContainsFunc(f.decls, func(d any) bool { _, loose := d.(*gloose); return !loose && isGlobal(d) }) {
			from = append(from, f)
		}
	}
	if len(from) == 0 || !rapid.Bool().Draw(w.t, "copyLoose") {
		return
	}
	source := rapid.SampledFrom(from).Draw(w.t, "looseFrom")
	var targets []*gfile
	for _, f := range to {
		if f.pkg != source.pkg {
			targets = append(targets, f)
		}
	}
	if len(targets) == 0 {
		return
	}
	target := rapid.SampledFrom(targets).Draw(w.t, "looseTo")
	for _, decl := range source.decls {
		if l, ok := decl.(*gloose); ok && l.kind != "floating" {
			kind := l.kind
			if kind == "var" || kind == "trailing" {
				kind = rapid.SampledFrom([]string{"var", "trailing"}).Draw(w.t, "copiedKind")
			}
			target.decls = append(target.decls, &gloose{kind: kind, name: l.name, doc: []binding{w.drawBinding("looseBinding", "func")}})
		}
	}
}

// isGlobal tells whether a declaration counts in its file's globals: it is
// top-level and no node.
func isGlobal(decl any) bool {
	switch decl := decl.(type) {
	case *gvar, *gconst:
		return true
	case *gloose:
		return decl.kind != "floating"
	}
	return false
}

func declaresGlobals(f *gfile) bool { return slices.ContainsFunc(f.decls, isGlobal) }

// bindingsOf returns a declaration's well-formed bindings that are in the
// right place, each once: on a type only realizes binds.
func bindingsOf(doc []binding, isType bool) []binding {
	var out []binding
	for _, b := range doc {
		if b.target != "" && (!isType || b.verb == "realizes") && !slices.Contains(out, b) {
			out = append(out, b)
		}
	}
	return out
}

// checks tells whether a doc comment carries a well-formed checks binding.
func checks(doc []binding) bool {
	return slices.ContainsFunc(bindingsOf(doc, false), func(b binding) bool { return b.verb == "checks" })
}
