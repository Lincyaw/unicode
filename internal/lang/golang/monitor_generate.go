package golang

import (
	"bytes"
	_ "embed"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"go/types"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
)

//go:embed monitor_runtime.go
var contractMonitorRuntime string

var _ = ucMonitorJudge
var _ = ucMonitorCollect

// monitorFileName is the name of the monitor file of a package, and
// monitorBuildTag the build constraint that builds it in (docs/design.md §6.6).
const (
	monitorFileName = "unicode_monitor_gen.go"
	monitorBuildTag = "unicode_monitor"
	monitorPackage  = "github.com/lincyaw/unicode/monitor"
)

// monitorJudge is a property judged at runtime, with the Go declaration that
// realizes its operation: a type for an operation of a type, a function for
// an operation of a container.
type monitorJudge struct {
	unit    *contractUnit
	call    lang.Monitored
	target  types.Object
	method  string
	deep    bool
	collect bool
}

// monitorFile is the monitor file of one package: the properties judged on
// the operations its declarations realize, and the interface wrappers and
// entry wrappers it declares.
type monitorFile struct {
	file     *contractFile
	judges   []*monitorJudge
	wrappers []*types.TypeName
	entries  []*monitorJudge
	// anchor is the property its diagnostics are reported on.
	anchor graph.Node
}

// monitorFiles plans the monitor files of the properties judged at runtime
// (lang.MonitoredCalls): a judge in the package of the operation's
// realization, an entry wrapper beside a realizing function, and an interface
// wrapper for each design interface the operation's type implements with an
// operation of that name. The monitor package itself is never monitored: its
// monitor file would import it.
func (c *contractCompiler) monitorFiles(units map[string]*contractUnit) map[string]*monitorFile {
	files := map[string]*monitorFile{}
	fileOf := func(pkg *packages.Package, anchor graph.Node) *monitorFile {
		if files[pkg.PkgPath] == nil {
			files[pkg.PkgPath] = &monitorFile{file: &contractFile{pkg: pkg, imports: map[string]string{}}, anchor: anchor}
		}
		return files[pkg.PkgPath]
	}
	wrapped := map[string]bool{}
	monitored := lang.MonitoredCalls(c.plan.Graph)
	for _, id := range slices.Sorted(maps.Keys(monitored)) {
		call, unit := monitored[id], units[id]
		// A derived input is not given by the call (docs/model.md §3.4).
		if unit == nil || unit.predicate != nil || len(unit.spec.Derived) > 0 || !c.monitorable(unit) {
			continue
		}
		op, ok := c.nodes[call.Op]
		if !ok {
			continue
		}
		judge := &monitorJudge{unit: unit, call: call, deep: first(op.Attrs["pure"]) == "true"}
		judge.collect = judge.deep && !c.reachesInterface(unit, call)
		owner := c.parents[call.Op]
		var d nativeDeclaration
		if call.Receiver != "" {
			if d, ok = c.bound(graph.EdgeKindRealizes, owner); !ok {
				continue
			}
			if _, isType := d.obj.(*types.TypeName); !isType {
				continue
			}
			judge.method = lastName(call.Op)
		} else {
			if d, ok = c.bound(graph.EdgeKindRealizes, call.Op); !ok {
				continue
			}
			if fn, isFunc := d.obj.(*types.Func); !isFunc || fn.Signature().Recv() != nil {
				continue
			}
		}
		if d.pkg.PkgPath == monitorPackage {
			continue
		}
		judge.target = d.obj
		f := fileOf(d.pkg, unit.node)
		f.judges = append(f.judges, judge)
		if call.Receiver == "" {
			if !slices.ContainsFunc(f.entries, func(e *monitorJudge) bool { return e.target == d.obj }) {
				f.entries = append(f.entries, judge)
			}
			continue
		}
		for _, edge := range c.graphEdges() {
			iface := c.nodes[edge.To]
			if edge.Kind != graph.EdgeKindImplements || edge.From != owner || iface.Form != "interface" || c.nodes[edge.To+"."+judge.method].Kind != graph.NodeKindOp || wrapped[edge.To] {
				continue
			}
			native, ok := c.bound(graph.EdgeKindRealizes, edge.To)
			if !ok || native.pkg.PkgPath == monitorPackage {
				continue
			}
			if name, isType := native.obj.(*types.TypeName); isType && types.IsInterface(name.Type()) {
				wrapped[edge.To] = true
				w := fileOf(native.pkg, unit.node)
				w.wrappers = append(w.wrappers, name)
			}
		}
	}
	return files
}

// monitorable tells whether a property's symbols are real values a monitored
// call gives: no call values, reply scripts or interfaces, which only test
// doubles and carriers give.
func (c *contractCompiler) monitorable(unit *contractUnit) bool {
	for _, symbol := range unit.spec.Symbols {
		if strings.Contains(symbol.Type, "call<") || strings.Contains(symbol.Type, "replies<") || unit.spec.Types[ucBaseType(symbol.Type)].Form == "interface" {
			return false
		}
	}
	return true
}

// reachesInterface tells whether the receiver or an argument of the call
// holds an interface value: such a call is not collected, since in a test
// its value is a reply script the real implementation has no model of.
func (c *contractCompiler) reachesInterface(unit *contractUnit, call lang.Monitored) bool {
	seen := map[string]bool{}
	var holds func(model string) bool
	holds = func(model string) bool {
		kind, args := ucSplitType(model)
		if len(args) > 0 && kind != "call" && kind != "replies" {
			return slices.ContainsFunc(args, holds)
		}
		ty, ok := unit.spec.Types[model]
		if !ok || seen[model] {
			return false
		}
		seen[model] = true
		if ty.Form == "interface" {
			return true
		}
		for _, field := range ucFields(unit.spec.Types, model) {
			if holds(field.Type) {
				return true
			}
		}
		return false
	}
	for _, symbol := range unit.spec.Symbols {
		if (symbol.Name == call.Receiver || slices.Contains(call.Args, symbol.Name)) && holds(symbol.Type) {
			return true
		}
	}
	return false
}

// judgeName is the name of a property's judge: judge and the property's
// name in upper camel case, a flow's with Flow before it.
func judgeName(n graph.Node) string {
	role := lang.NameRolePredicate
	if n.Kind == graph.NodeKindFlow {
		role = lang.NameRoleFlowPredicate
	}
	return "judge" + strings.TrimPrefix((&Backend{}).MapName(role, lastName(n.Id)), "check")
}

// renderMonitor writes a package's monitor file: the generated marker, the
// build constraint, the part of the runtime its judges reach, each judged
// property's contract and judge, the wrappers, and the init that registers
// them.
func (c *contractCompiler) renderMonitor(m *monitorFile) []byte {
	file := m.file
	c.literalImports = file.imports
	c.foreignNames, c.foreignOriginal = map[string]string{}, map[string]string{}
	file.imports[monitorPackage] = "monitor"
	file.imports["reflect"] = "reflect"
	var body strings.Builder
	var registrations []string
	owned := map[string]bool{}
	// The judges of one operation are registered together, as one operation.
	type registered struct {
		judge      *monitorJudge
		judges     []string
		collection string
	}
	var targets []*registered
	byTarget := map[types.Object]*registered{}
	for index, judge := range m.judges {
		id := strconv.Itoa(index)
		spec, unit := "ucMonitorSpec"+id, "ucMonitorUnitOf"+id
		owned[spec], owned[unit] = true, true
		c.renderSpec(&body, file, judge.unit, spec, true)
		var argTypes, resultTypes []string
		var signature *types.Signature
		if fn, ok := judge.target.(*types.Func); ok {
			signature = fn.Signature()
		} else if method := c.monitoredMethod(judge.target.(*types.TypeName), judge.unit.spec.Operations[judge.call.Op].GoName); method != nil {
			signature = method.Signature()
		}
		if signature == nil {
			c.diagnostic(judge.unit.node, "lang.golang.unsupported-contract", "the Go type realizing "+judge.call.Op+" has no method for it", "realize the operation with a method of the type")
			return nil
		}
		params, results := nativeParams(signature)
		for _, p := range params {
			argTypes = append(argTypes, "reflect.TypeFor["+c.nativeType(p.Type(), file.pkg, file.imports)+"]()")
		}
		for _, r := range results {
			resultTypes = append(resultTypes, "reflect.TypeFor["+c.nativeType(r.Type(), file.pkg, file.imports)+"]()")
		}
		var sensitive []string
		for _, n := range c.plan.Graph.Nodes() {
			if n.Layer == graph.LayerDesign && n.Kind == graph.NodeKindType && slices.Contains(n.Attrs["labels"], "sensitive") {
				sensitive = append(sensitive, strconv.Quote(n.Id))
			}
		}
		var args []string
		for _, a := range judge.call.Args {
			args = append(args, strconv.Quote(a))
		}
		fmt.Fprintf(&body, "var %s = ucMonitorUnit{Spec: %s, Op: %s, Receiver: %s, Args: []string{%s}, ArgTypes: []reflect.Type{%s}, Results: []reflect.Type{%s}, Deep: %t, Sensitive: []string{%s}}\n",
			unit, spec, strconv.Quote(judge.call.Op), strconv.Quote(judge.call.Receiver), strings.Join(args, ", "), strings.Join(argTypes, ", "), strings.Join(resultTypes, ", "), judge.deep, strings.Join(sensitive, ", "))
		name := judgeName(judge.unit.node)
		fmt.Fprintf(&body, "// %s judges %s on a monitored call (docs/language.md E26).\nfunc %s(observed monitor.Observed) monitor.Finish { return ucMonitorJudge(&%s, observed) }\n", name, judge.unit.node.Id, name, unit)
		r := byTarget[judge.target]
		if r == nil {
			r = &registered{judge: judge}
			byTarget[judge.target] = r
			targets = append(targets, r)
		}
		r.judges = append(r.judges, fmt.Sprintf("{Check: %s, Before: %s}", strconv.Quote(judge.unit.node.Id), name))
		if judge.collect && r.collection == "" {
			r.collection = fmt.Sprintf(", Collect: func(receiver any, args []any) (monitor.Collected, error) { return ucMonitorCollect(&%s, receiver, args) }", unit)
		}
	}
	for _, r := range targets {
		operation := fmt.Sprintf("monitor.Operation{Op: %s, Judges: []monitor.Judge{%s}%s}", strconv.Quote(r.judge.call.Op), strings.Join(r.judges, ", "), r.collection)
		if r.judge.method != "" {
			typeName := c.qualify(r.judge.target, file.pkg, file.imports)
			for _, receiver := range []string{typeName, "*" + typeName} {
				registrations = append(registrations, fmt.Sprintf("monitor.RegisterMethod(reflect.TypeFor[%s](), %s, %s)", receiver, strconv.Quote(r.judge.method), operation))
			}
		} else {
			registrations = append(registrations, fmt.Sprintf("monitor.RegisterFunction(%s)", operation))
		}
	}
	for _, entry := range m.entries {
		fn := entry.target.(*types.Func)
		name := "monitored" + upperFirst(fn.Name())
		c.renderForward(&body, file, "func "+name, fn.Signature(), entry.call.Op, "nil", fn.Name())
		registrations = append(registrations, fmt.Sprintf("monitor.RegisterEntry(%s, %s)", fn.Name(), name))
	}
	for _, iface := range m.wrappers {
		name := "monitored" + iface.Name()
		fmt.Fprintf(&body, "// %s wraps an implementation of %s: each call of an operation of the design is monitored.\ntype %s struct{ next %s }\n", name, iface.Name(), name, iface.Name())
		design := c.designOf(iface)
		methods := types.NewMethodSet(iface.Type())
		for i := range methods.Len() {
			method := methods.At(i).Obj().(*types.Func)
			op := design + "." + lowerFirstName(method.Name())
			if c.nodes[op].Kind != graph.NodeKindOp || c.parents[op] != design {
				op = ""
			}
			c.renderForward(&body, file, "func (w "+name+") "+method.Name(), method.Signature(), op, "w.next", "w.next."+method.Name())
		}
		registrations = append(registrations, fmt.Sprintf("monitor.RegisterWrapper(reflect.TypeFor[%s](), func(next any) any { return %s{next: next.(%s)} })", iface.Name(), name, iface.Name()))
	}
	fmt.Fprintf(&body, "func init() {\n%s\n}\n", strings.Join(registrations, "\n"))
	// Only judges need the runtime; a file of wrappers alone calls the monitor.
	runtime := ""
	if len(m.judges) > 0 {
		source, imports, runtimeOwned, err := monitorRuntime()
		if err != nil {
			c.diagnostic(m.anchor, "lang.golang.generate-failed", err.Error(), "correct the monitor runtime before generation")
			return nil
		}
		for path, alias := range imports {
			if _, ok := file.imports[path]; !ok {
				file.imports[path] = alias
			}
		}
		for name := range runtimeOwned {
			owned[name] = true
		}
		runtime = source
	}
	var out bytes.Buffer
	fmt.Fprintf(&out, "%s\n\n//go:build %s\n\npackage %s\n\nimport (\n", unicodeGeneratedHeader, monitorBuildTag, file.pkg.Name)
	for _, path := range slices.Sorted(maps.Keys(file.imports)) {
		alias := file.imports[path]
		if alias == filepath.Base(path) {
			alias = ""
		}
		fmt.Fprintf(&out, "%s %s\n", alias, strconv.Quote(path))
	}
	out.WriteString(")\n")
	out.WriteString(runtime)
	out.WriteString(body.String())
	fset := token.NewFileSet()
	tree, err := parser.ParseFile(fset, monitorFileName, out.Bytes(), parser.ParseComments)
	if err != nil {
		c.diagnostic(m.anchor, "lang.golang.generate-failed", err.Error(), "correct unsupported Go bindings before generation")
		return nil
	}
	ast.Inspect(tree, func(node ast.Node) bool {
		if id, ok := node.(*ast.Ident); ok {
			if owned[id.Name] {
				id.Name = "unicodeMonitor" + strings.TrimPrefix(strings.TrimPrefix(id.Name, "ucMonitor"), "uc")
			} else if original, ok := c.foreignOriginal[id.Name]; ok {
				id.Name = original
			}
		}
		return true
	})
	for _, decl := range tree.Decls {
		var names []*ast.Ident
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Recv == nil && d.Name.Name != "init" {
				names = append(names, d.Name)
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch spec := spec.(type) {
				case *ast.TypeSpec:
					names = append(names, spec.Name)
				case *ast.ValueSpec:
					names = append(names, spec.Names...)
				}
			}
		}
		for _, name := range names {
			if name.Name != "_" && file.pkg.Types.Scope().Lookup(name.Name) != nil {
				c.diagnostic(m.anchor, "lang.golang.driver-name-collision", "generated declaration "+name.Name+" of the monitor file conflicts with a declaration in package "+file.pkg.PkgPath, "rename the handwritten declaration")
				return nil
			}
		}
	}
	var formatted bytes.Buffer
	if err := format.Node(&formatted, fset, tree); err != nil {
		c.diagnostic(m.anchor, "lang.golang.generate-failed", err.Error(), "correct unsupported Go bindings before generation")
		return nil
	}
	return formatted.Bytes()
}

// monitoredMethod is the method name of a type or of a pointer to it.
func (c *contractCompiler) monitoredMethod(t *types.TypeName, name string) *types.Func {
	for _, typ := range []types.Type{t.Type(), types.NewPointer(t.Type())} {
		set := types.NewMethodSet(typ)
		for i := range set.Len() {
			if fn, ok := set.At(i).Obj().(*types.Func); ok && fn.Name() == name {
				return fn
			}
		}
	}
	return nil
}

// designOf is the design type a Go type realizes.
func (c *contractCompiler) designOf(t *types.TypeName) string {
	for design, bound := range c.bindings[graph.EdgeKindRealizes] {
		for _, code := range bound {
			if d, ok := c.declarations[code]; ok && d.obj == t {
				return design
			}
		}
	}
	return ""
}

// nativeParams are a signature's parameters without a leading
// context.Context, and its results without a trailing error.
func nativeParams(sig *types.Signature) (params, results []*types.Var) {
	for i := range sig.Params().Len() {
		if i == 0 && isContext(sig.Params().At(0).Type()) {
			continue
		}
		params = append(params, sig.Params().At(i))
	}
	for i := range sig.Results().Len() {
		if i == sig.Results().Len()-1 && types.Identical(sig.Results().At(i).Type(), errorType) {
			continue
		}
		results = append(results, sig.Results().At(i))
	}
	return params, results
}

// isContext tells whether t is context.Context.
func isContext(t types.Type) bool {
	named, ok := types.Unalias(t).(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == "context" && named.Obj().Name() == "Context"
}

// renderForward writes a function or method header of the signature sig
// whose body calls target with the same arguments and returns its results;
// for a design operation op, the call is monitored: begun with its receiver
// and its arguments without a leading context.Context, given the call's
// context, recovered when it panics and ended with its results.
func (c *contractCompiler) renderForward(body *strings.Builder, file *contractFile, header string, sig *types.Signature, op, receiver, target string) {
	var params, args, results, returned []string
	hasContext := false
	for i := range sig.Params().Len() {
		p := sig.Params().At(i)
		name := "p" + strconv.Itoa(i)
		t := c.nativeType(p.Type(), file.pkg, file.imports)
		if sig.Variadic() && i == sig.Params().Len()-1 {
			t = "..." + c.nativeType(p.Type().(*types.Slice).Elem(), file.pkg, file.imports)
			args = append(args, name+"...")
		} else {
			args = append(args, name)
		}
		params = append(params, name+" "+t)
		hasContext = hasContext || i == 0 && isContext(p.Type())
	}
	for i := range sig.Results().Len() {
		results = append(results, c.nativeType(sig.Results().At(i).Type(), file.pkg, file.imports))
		returned = append(returned, "r"+strconv.Itoa(i))
	}
	fmt.Fprintf(body, "%s(%s) (%s) {\n", header, strings.Join(params, ", "), strings.Join(results, ", "))
	if op == "" {
		call := target + "(" + strings.Join(args, ", ") + ")"
		if len(results) > 0 {
			call = "return " + call
		}
		body.WriteString(call + "\n}\n")
		return
	}
	ctx, begun := "nil", args
	if hasContext {
		ctx, begun = "p0", args[1:]
		args = append([]string{"call.Context()"}, args[1:]...)
	}
	var plain []string
	for _, a := range begun {
		plain = append(plain, strings.TrimSuffix(a, "..."))
	}
	fmt.Fprintf(body, "call := monitor.Begin(%s, %s, %s", ctx, strconv.Quote(op), receiver)
	if len(plain) > 0 {
		body.WriteString(", " + strings.Join(plain, ", "))
	}
	body.WriteString(")\ndefer call.Recover()\n")
	call := target + "(" + strings.Join(args, ", ") + ")"
	if len(returned) > 0 {
		call = strings.Join(returned, ", ") + " := " + call
	}
	body.WriteString(call + "\n")
	values, failure := returned, "nil"
	if n := sig.Results().Len(); n > 0 && types.Identical(sig.Results().At(n-1).Type(), errorType) {
		values, failure = returned[:n-1], returned[n-1]
	}
	fmt.Fprintf(body, "call.End([]any{%s}, %s)\n", strings.Join(values, ", "), failure)
	if len(returned) > 0 {
		body.WriteString("return " + strings.Join(returned, ", ") + "\n")
	}
	body.WriteString("}\n")
}

// lowerFirstName is the design name a Go method name maps back to.
func lowerFirstName(name string) string {
	if name == "" {
		return name
	}
	return strings.ToLower(name[:1]) + name[1:]
}

// monitorRuntime is the part of the contract runtime that monitor files
// embed: the declarations of monitor_runtime.go and every declaration of the
// runtime they reach, with the imports these use; owned are their names.
func monitorRuntime() (source string, imports map[string]string, owned map[string]bool, err error) {
	parts := append([]string{contractRuntime, contractMonitorRuntime}, contractRuntimeParts()...)
	fset := token.NewFileSet()
	declared := map[string][]ast.Decl{}
	methods := map[string][]ast.Decl{}
	importNames := map[string]string{}
	var roots []string
	var order []ast.Decl
	texts := map[ast.Decl]string{}
	for index, part := range parts {
		file, err := parser.ParseFile(fset, "runtime-part.go", part, 0)
		if err != nil {
			return "", nil, nil, err
		}
		for _, spec := range file.Imports {
			path, _ := strconv.Unquote(spec.Path.Value)
			name := filepath.Base(path)
			if spec.Name != nil {
				name = spec.Name.Name
			}
			importNames[name] = path
		}
		for _, decl := range file.Decls {
			if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.IMPORT {
				continue
			}
			texts[decl] = part[fset.Position(decl.Pos()).Offset:fset.Position(decl.End()).Offset]
			order = append(order, decl)
			var names []string
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Recv != nil {
					recv := d.Recv.List[0].Type
					if star, ok := recv.(*ast.StarExpr); ok {
						recv = star.X
					}
					if ident, ok := recv.(*ast.Ident); ok {
						methods[ident.Name] = append(methods[ident.Name], decl)
					}
					continue
				}
				names = append(names, d.Name.Name)
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch spec := spec.(type) {
					case *ast.TypeSpec:
						names = append(names, spec.Name.Name)
					case *ast.ValueSpec:
						for _, name := range spec.Names {
							names = append(names, name.Name)
						}
					}
				}
			}
			for _, name := range names {
				if name == "_" {
					continue
				}
				declared[name] = append(declared[name], decl)
				if index == 1 {
					roots = append(roots, name)
				}
			}
		}
	}
	kept := map[ast.Decl]bool{}
	seen := map[string]bool{}
	used := map[string]bool{}
	for len(roots) > 0 {
		name := roots[0]
		roots = roots[1:]
		if seen[name] {
			continue
		}
		seen[name] = true
		for _, decl := range append(slices.Clone(declared[name]), methods[name]...) {
			if kept[decl] {
				continue
			}
			kept[decl] = true
			ast.Inspect(decl, func(node ast.Node) bool {
				switch n := node.(type) {
				case *ast.SelectorExpr:
					if x, ok := n.X.(*ast.Ident); ok && importNames[x.Name] != "" && declared[x.Name] == nil {
						used[x.Name] = true
					}
				case *ast.Ident:
					if declared[n.Name] != nil || methods[n.Name] != nil {
						roots = append(roots, n.Name)
					}
				}
				return true
			})
		}
	}
	if used["rapid"] || used["testing"] {
		return "", nil, nil, fmt.Errorf("the monitor runtime reaches test code (rapid or testing)")
	}
	var out strings.Builder
	owned = map[string]bool{}
	for _, decl := range order {
		if kept[decl] {
			out.WriteString(texts[decl])
			out.WriteString("\n")
		}
	}
	for name := range seen {
		if strings.HasPrefix(name, "uc") && declared[name] != nil {
			owned[name] = true
		}
	}
	imports = map[string]string{}
	for name := range used {
		imports[importNames[name]] = name
	}
	return out.String(), imports, owned, nil
}
