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

func (w *world) drawSig(from *gfile, name string) *gsig {
	sig := &gsig{pkg: from.pkg, name: name}
	unnamed := rapid.IntRange(0, 5).Draw(w.t, "unnamed") == 0
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
	return sig
}

func (w *world) drawType() {
	file := w.anyFile("typeFile")
	ty := &gtype{file: file, form: rapid.SampledFrom([]string{"struct", "interface", "defined"}).Draw(w.t, "form")}
	ty.name = w.fresh("typeName", w.exported("typeExported"))
	switch ty.form {
	case "struct":
		for range rapid.IntRange(0, 3).Draw(w.t, "fields") {
			field := gfield{name: w.fresh("field", w.exported("fieldExported")), typ: w.drawExpr(file, 0)}
			if rapid.IntRange(0, 5).Draw(w.t, "fieldDoc") == 0 {
				field.doc = w.misplaced("fieldBindings")
			}
			ty.fields = append(ty.fields, field)
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
	fn := &gfunc{file: file, recv: ty, ptrRecv: rapid.Bool().Draw(w.t, "ptrRecv"), sig: sig}
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
	fn := &gfunc{file: file, sig: sig, testName: testName}
	fn.doc = w.drawDoc(sig.name, "func")
	w.funcs = append(w.funcs, fn)
	file.decls = append(file.decls, fn)
}

// Bodies.

func (w *world) drawBody(fn *gfunc) {
	if rapid.IntRange(0, 4).Draw(w.t, "stub") == 0 {
		fn.terminal = marker
		return
	}
	for range rapid.IntRange(0, 3).Draw(w.t, "stmts") {
		if stmt, ok := w.drawStmt(fn, 0); ok {
			fn.body = append(fn.body, stmt)
		}
	}
	if len(fn.sig.results) > 0 || rapid.Bool().Draw(w.t, "terminal") {
		fn.terminal = rapid.SampledFrom([]string{marker, "done"}).Draw(w.t, "panic")
	}
}

func (w *world) drawStmt(fn *gfunc, depth int) (gstmt, bool) {
	kinds := []string{"call", "call", "call", "assign", "iface", "builtin", "ref", "comment"}
	if depth == 0 {
		kinds = append(kinds, "closure")
	}
	if fn.testName == "Test" || fn.testName == "Testlower" {
		kinds = append(kinds, "rapid", "rapid", "rapid")
	}
	switch kind := rapid.SampledFrom(kinds).Draw(w.t, "stmt"); kind {
	case "call":
		calls := w.callables(fn.file)
		if len(calls) == 0 {
			return gstmt{}, false
		}
		return rapid.SampledFrom(calls).Draw(w.t, "callee"), true
	case "assign":
		var calls []gstmt
		for _, c := range w.callables(fn.file) {
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
		calls := w.interfaceCalls(fn.file)
		if len(calls) == 0 {
			return gstmt{}, false
		}
		return rapid.SampledFrom(calls).Draw(w.t, "ifaceCall"), true
	case "ref":
		var refs []gstmt
		for _, c := range w.callables(fn.file) {
			if c.via == nil {
				refs = append(refs, gstmt{kind: "ref", target: c.target})
			}
		}
		if len(refs) == 0 {
			return gstmt{}, false
		}
		return rapid.SampledFrom(refs).Draw(w.t, "ref"), true
	case "closure":
		inner, ok := w.drawStmt(fn, depth+1)
		return gstmt{kind: kind, inner: &inner}, ok
	case "comment":
		return gstmt{kind: kind, comment: w.drawBinding("bodyBinding", "func")}, true
	default:
		return gstmt{kind: kind}, true
	}
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
		verb = rapid.SampledFrom([]string{"realizes", "checks", "generates", "observes"}).Draw(w.t, label+"Verb")
	}
	component := rapid.StringMatching(`[a-z]{1,3}(-[a-z]{1,2})?(\.[a-z]{1,3})?`).Draw(w.t, label+"Component")
	var id string
	switch {
	case verb == "realizes" && on == "type":
		id = component + "." + rapid.StringMatching(`[A-Z][a-zA-Z]{0,3}`).Draw(w.t, label+"Type")
	case verb == "realizes":
		id = component + "." + rapid.StringMatching(`[a-z][a-zA-Z]{0,3}`).Draw(w.t, label+"Op")
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

// drawLoose adds declarations that are no nodes, each carrying a binding.
func (w *world) drawLoose() {
	for range rapid.IntRange(0, 2).Draw(w.t, "loose") {
		file := w.anyFile("looseFile")
		kind := rapid.SampledFrom([]string{"const", "var", "trailing", "init", "floating"}).Draw(w.t, "looseKind")
		file.decls = append(file.decls, &gloose{
			kind: kind, name: w.fresh("loose", false), doc: []binding{w.drawBinding("looseBinding", "func")},
		})
	}
}

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
