package golang

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"go/types"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/tools/go/packages"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
	"github.com/lincyaw/unicode/internal/syntax"
)

//go:embed contract_runtime.go
var contractRuntime string

var _ lang.Generator = (*Backend)(nil)
var _ = ucCheck
var _ = ucDraw
var _ = ucParseTime
var _ = ucParseDuration
var _ = ucFinish
var _ = ucFailedPredicate

type nativeDeclaration struct {
	obj types.Object
	pkg *packages.Package
}
type contractCompiler struct {
	plan            lang.GenPlan
	loaded          loaded
	declarations    map[string]nativeDeclaration
	bindings        map[graph.EdgeKind]map[string][]string
	nodes           map[string]graph.Node
	parents         map[string]string
	diagnostics     []diag.Diagnostic
	literalImports  map[string]string
	foreignNames    map[string]string
	foreignOriginal map[string]string
}
type contractUnit struct {
	node          graph.Node
	pkg           *packages.Package
	spec          ucContract
	predicate     *types.Func
	cases         [][]string
	generators    map[string]*types.Func
	generatorArgs map[string][]string
	limits        []string
}
type contractFile struct {
	pkg     *packages.Package
	units   []*contractUnit
	imports map[string]string
}

// Generate builds standalone Go test files without changing the repository.
func (b *Backend) Generate(plan lang.GenPlan) lang.Generation {
	c := contractCompiler{foreignNames: map[string]string{}, foreignOriginal: map[string]string{}, plan: plan, declarations: map[string]nativeDeclaration{}, bindings: map[graph.EdgeKind]map[string][]string{}, nodes: map[string]graph.Node{}, parents: map[string]string{}}
	if plan.Graph == nil {
		return lang.Generation{Diagnostics: []diag.Diagnostic{{Code: "lang.golang.invalid-plan", Position: diag.Position{Path: "design/system.uc", Line: 1, Column: 1}, Message: "the generation plan has no graph", Fix: "build and link the graph before generating Go contracts"}}}
	}
	var skeletons []lang.Edit
	var preparationDiagnostics []diag.Diagnostic
	plan, c.loaded, skeletons, preparationDiagnostics = prepareSkeletons(plan)
	if len(preparationDiagnostics) > 0 {
		return lang.Generation{Diagnostics: preparationDiagnostics}
	}
	c.plan = plan

	for _, p := range c.loaded.packages {
		for _, obj := range p.TypesInfo.Defs {
			switch obj := obj.(type) {
			case *types.TypeName:
				c.declarations[typeID(obj)] = nativeDeclaration{obj, p}
			case *types.Func:
				c.declarations[funcID(obj)] = nativeDeclaration{obj, p}
			}
		}
	}
	for _, n := range plan.Graph.Nodes() {
		c.nodes[n.Id] = n
	}
	for _, e := range plan.Graph.Edges() {
		if e.Kind == graph.EdgeKindContains {
			c.parents[e.To] = e.From
		}
		if e.Kind == graph.EdgeKindRealizes || e.Kind == graph.EdgeKindChecks || e.Kind == graph.EdgeKindGenerates || e.Kind == graph.EdgeKindObserves {
			if c.bindings[e.Kind] == nil {
				c.bindings[e.Kind] = map[string][]string{}
			}
			c.bindings[e.Kind][e.To] = append(c.bindings[e.Kind][e.To], e.From)
			if e.Kind == graph.EdgeKindGenerates {
				for _, symbol := range e.Attrs["symbols"] {
					c.bindings[e.Kind][e.To+"."+symbol] = append(c.bindings[e.Kind][e.To+"."+symbol], e.From)
				}
			}
		}
	}
	files := map[string]*contractFile{}
	for _, n := range plan.Graph.Nodes() {
		if n.Layer != graph.LayerDesign || (n.Kind != graph.NodeKindProperty && n.Kind != graph.NodeKindFlow) {
			continue
		}
		unit := c.unit(n)
		if unit == nil {
			continue
		}
		key := unit.pkg.PkgPath
		if files[key] == nil {
			files[key] = &contractFile{pkg: unit.pkg, imports: map[string]string{}}
		}
		files[key].units = append(files[key].units, unit)
	}
	generation := lang.Generation{Edits: skeletons}
	outputs := map[string]bool{}
	for _, key := range slices.Sorted(maps.Keys(files)) {
		file := files[key]
		content := c.render(file)
		if content == nil {
			continue
		}
		dir := filepath.Dir(c.positionFile(file.pkg))
		path := filepath.ToSlash(filepath.Join(dir, "unicode_contracts_gen_test.go"))
		outputs[path] = true
		generation.Edits = append(generation.Edits, lang.Edit{Path: path, Content: content})
	}
	// Owned tests no longer emitted by this plan retain only their package and
	// build constraints, so removed or moved contracts cannot leave live drivers.
	overlay, err := generatedTestOverlay(c.loaded.root, plan.Roots)
	if err != nil {
		return lang.Generation{Diagnostics: append(c.diagnostics, runDiagnostic("lang.golang.generate-failed", err.Error(), "make owned generated tests readable before generation"))}
	}
	for _, name := range slices.Sorted(maps.Keys(overlay)) {
		rel, err := filepath.Rel(c.loaded.root, name)
		if err != nil || !filepath.IsLocal(rel) {
			continue
		}
		path := filepath.ToSlash(rel)
		if !outputs[path] {
			generation.Edits = append(generation.Edits, lang.Edit{Path: path, Content: overlay[name]})
		}
	}
	slices.SortFunc(generation.Edits, func(a, b lang.Edit) int { return strings.Compare(a.Path, b.Path) })
	generation.Diagnostics = c.diagnostics
	if len(generation.Diagnostics) == 0 && len(generation.Edits) > 0 {
		if _, err := loadPlannedGo(plan, generation.Edits, false); err != nil {
			generation.Diagnostics = append(generation.Diagnostics, runDiagnostic("lang.golang.generate-failed", err.Error(), "correct source bindings or unsupported native signatures before writing the generation plan"))
		}
	}
	return generation
}
func (c *contractCompiler) diagnostic(n graph.Node, code, message, fix string) {
	c.diagnostics = append(c.diagnostics, diag.Diagnostic{Code: code, Subject: n.Id, Position: n.Origin, Message: message, Fix: fix})
}
func (c *contractCompiler) bound(kind graph.EdgeKind, id string) (nativeDeclaration, bool) {
	ids := c.bindings[kind][id]
	if len(ids) != 1 {
		return nativeDeclaration{}, false
	}
	d, ok := c.declarations[ids[0]]
	return d, ok
}
func (c *contractCompiler) positionFile(p *packages.Package) string {
	for _, f := range filesUnder(c.loaded.root, p) {
		if !strings.HasSuffix(f.rel, "_test.go") {
			return f.rel
		}
	}
	fs := filesUnder(c.loaded.root, p)
	if len(fs) > 0 {
		return fs[0].rel
	}
	return ""
}
func (c *contractCompiler) packageFor(n graph.Node, call *ucExpr, symbols []ucSymbol) *packages.Package {
	if d, ok := c.bound(graph.EdgeKindChecks, n.Id); ok {
		return d.pkg
	}
	if call != nil {
		callee := call.Children[0]
		id := ucName(callee)
		if callee.Kind == "field" {
			root := ucName(callee.Children[0])
			for _, s := range symbols {
				if s.Name == root {
					id = s.Type + "." + callee.Text
				}
			}
		}
		if d, ok := c.bound(graph.EdgeKindRealizes, id); ok {
			return d.pkg
		}
	}
	component := c.nodes[c.parents[n.Id]]
	for _, p := range c.loaded.packages {
		for _, f := range filesUnder(c.loaded.root, p) {
			for _, pattern := range component.Attrs["paths"] {
				prefix, _, _ := strings.Cut(pattern, "*")
				if strings.HasPrefix(f.rel, prefix) {
					return p
				}
			}
		}
	}
	return nil
}
func (c *contractCompiler) expression(n graph.Node, text, clause string, ordinal int) *ucExpr {
	parsed := syntax.ParseExpr(n.Origin.Path, text)
	if len(parsed.Diagnostics) > 0 {
		c.diagnostics = append(c.diagnostics, parsed.Diagnostics...)
		return nil
	}
	line, column := n.Origin.Line, n.Origin.Column
	if data, err := os.ReadFile(filepath.Join(c.plan.Root, n.Origin.Path)); err == nil {
		source := syntax.Parse(n.Origin.Path, syntax.FileKindComponent, data)
		if source.File != nil {
			for _, decl := range source.File.Decls {
				property, ok := decl.(*syntax.PropertyDecl)
				if !ok || string(property.Name) != lastName(n.Id) {
					continue
				}
				index := 0
				for _, member := range property.Lines {
					var original *syntax.Expr
					switch member := member.(type) {
					case *syntax.Act:
						if clause == "call" {
							original = member.Expression.Expr()
						}
					case *syntax.Requires:
						if clause == "requires" {
							original = member.Expression.Expr()
						}
					case *syntax.Ensure:
						if clause == "ensure" {
							original = member.Expression.Expr()
						}
					case *syntax.Changes:
						if clause == "changes" && ordinal < len(member.Paths) {
							original = member.Paths[ordinal].Expr()
							index = ordinal
						}
					}
					if original != nil {
						if index == ordinal {
							line, column = original.Pos.Line, original.Pos.Column
							break
						}
						index++
					}
				}
			}
		}
	}
	known := map[string]bool{"result": true, "size": true}
	for _, s := range n.Attrs["symbols"] {
		name, _, _ := strings.Cut(s, ": ")
		known[name] = true
	}
	for _, node := range c.plan.Graph.Nodes() {
		if node.Kind == graph.NodeKindObservation && c.parents[node.Id] == n.Id {
			known[lastName(node.Id)] = true
		}
	}
	var convert func(*syntax.Expr) *ucExpr
	convert = func(e *syntax.Expr) *ucExpr {
		if e == nil {
			return nil
		}
		out := &ucExpr{Kind: e.Kind, Text: e.Text, Line: line + e.Pos.Line - 1, Column: column + e.Pos.Column - 1}
		if e.Kind == "all" || e.Kind == "any" || e.Kind == "count" || e.Kind == "sum" {
			out.Children = append(out.Children, convert(e.Children[0]))
			previous, present := known[e.Text]
			known[e.Text] = true
			out.Children = append(out.Children, convert(e.Children[1]))
			if present {
				known[e.Text] = previous
			} else {
				delete(known, e.Text)
			}
			return out
		}
		for _, child := range e.Children {
			out.Children = append(out.Children, convert(child))
		}
		if e.Kind == "raised" || e.Kind == "failure" {
			out.Text = c.resolve(n, e.Text)
		}
		name := ucName(out)
		root, _, _ := strings.Cut(name, ".")
		if name != "" && !known[root] {
			resolved := c.resolve(n, name)
			if resolved != name {
				out.Kind = "name"
				out.Text = resolved
				out.Children = nil
			}
		}
		return out
	}
	return convert(parsed.Expr)
}
func (c *contractCompiler) resolve(n graph.Node, name string) string {
	for scope := c.parents[n.Id]; scope != ""; scope = c.parents[scope] {
		candidate := scope + "." + name
		if _, ok := c.nodes[candidate]; ok {
			return candidate
		}
		if i := strings.LastIndexByte(candidate, '.'); i > 0 {
			if node, ok := c.nodes[candidate[:i]]; ok && node.Kind == graph.NodeKindType && node.Form == "enum" {
				return candidate
			}
		}
	}
	return name
}

func ucTypeList(text string) []string {
	depth, start := 0, 0
	var out []string
	for i, r := range text {
		if r == '<' {
			depth++
		}
		if r == '>' {
			depth--
		}
		if r == ',' && depth == 0 {
			out = append(out, strings.TrimSpace(text[start:i]))
			start = i + 1
		}
	}
	if strings.TrimSpace(text[start:]) != "" {
		out = append(out, strings.TrimSpace(text[start:]))
	}
	return out
}
func ucSignature(text string) ([]ucSymbol, string, []string) {
	i := strings.IndexByte(text, ')')
	if i < 0 {
		return nil, "", nil
	}
	var params []ucSymbol
	for _, p := range ucTypeList(strings.TrimPrefix(text[:i], "(")) {
		name, t, _ := strings.Cut(p, ": ")
		params = append(params, ucSymbol{name, t})
	}
	tail := strings.TrimSpace(text[i+1:])
	ret, raises, _ := strings.Cut(tail, " raises ")
	if strings.HasPrefix(tail, "raises ") {
		ret = ""
		raises = strings.TrimPrefix(tail, "raises ")
	}
	ret = strings.TrimSpace(strings.TrimPrefix(ret, "->"))
	var errors []string
	if raises != "" {
		errors = ucTypeList(raises)
	}
	return params, ret, errors
}
func (c *contractCompiler) unit(n graph.Node) *contractUnit {
	unit := &contractUnit{node: n, generators: map[string]*types.Func{}, generatorArgs: map[string][]string{}, limits: n.Attrs["limits"]}
	unit.spec = ucContract{ID: n.Id, Path: n.Origin.Path, Line: n.Origin.Line, Column: n.Origin.Column, Types: map[string]ucType{}, Operations: map[string]ucOperation{}}
	for _, text := range n.Attrs["symbols"] {
		name, t, _ := strings.Cut(text, ": ")
		t = strings.TrimSuffix(t, " generated")
		unit.spec.Symbols = append(unit.spec.Symbols, ucSymbol{name, t})
	}
	if len(n.Attrs["call"]) > 0 {
		unit.spec.Call = c.expression(n, n.Attrs["call"][0], "call", 0)
	}
	for i, e := range n.Attrs["requires"] {
		unit.spec.Requires = append(unit.spec.Requires, c.expression(n, e, "requires", i))
	}
	for i, e := range n.Attrs["ensure"] {
		unit.spec.Ensures = append(unit.spec.Ensures, c.expression(n, e, "ensure", i))
	}
	for _, observation := range n.Attrs["observe"] {
		if !strings.Contains(observation, ":") {
			unit.spec.Observe = append(unit.spec.Observe, observation)
		}
	}
	for i, text := range n.Attrs["changes"] {
		if text != "none" {
			unit.spec.Changes = append(unit.spec.Changes, c.expression(n, text, "changes", i))
		}
	}
	if len(n.Attrs["within"]) > 0 {
		duration, err := time.ParseDuration(n.Attrs["within"][0])
		if err != nil {
			c.diagnostic(n, "lang.golang.unsupported-contract", err.Error(), "write a valid within duration")
		} else {
			unit.spec.Within = duration
		}
	}
	unit.pkg = c.packageFor(n, unit.spec.Call, unit.spec.Symbols)
	if unit.pkg == nil {
		c.diagnostic(n, "lang.golang.missing-contract-package", "no Go package realizes the property or its tested operation", "realize the operation or bind the manual predicate in its component package")
		return nil
	}
	if len(n.Attrs["manual"]) > 0 || n.Kind == graph.NodeKindFlow {
		d, ok := c.bound(graph.EdgeKindChecks, n.Id)
		if !ok {
			c.diagnostic(n, "lang.golang.unsupported-contract", "a manual property or staged flow has no bound predicate", "bind a meaningful handwritten predicate until its generated execution is supported")
			return nil
		}
		unit.predicate, _ = d.obj.(*types.Func)
	} else if unit.spec.Call == nil && len(unit.spec.Ensures) == 0 {
		c.diagnostic(n, "lang.golang.unsupported-contract", "the property has neither executable expressions nor a manual predicate", "state atomic ensure expressions or declare a justified manual predicate")
		return nil
	}
	for _, node := range c.plan.Graph.Nodes() {
		if node.Layer != graph.LayerDesign {
			continue
		}
		if node.Kind == graph.NodeKindType {
			ty := ucType{Form: node.Form, Values: node.Attrs["values"]}
			for _, f := range node.Attrs["fields"] {
				name, t, _ := strings.Cut(f, ": ")
				ty.Fields = append(ty.Fields, ucField{name, upperFirst(name), t})
			}
			for _, e := range c.plan.Graph.Edges() {
				if e.Kind == graph.EdgeKindExtends && e.From == node.Id {
					ty.Parent = e.To
				}
			}
			unit.spec.Types[node.Id] = ty
		}
		if node.Kind == graph.NodeKindOp {
			parameters, ret, raises := ucSignature(first(node.Attrs["signature"]))
			op := ucOperation{Name: node.Id, GoName: upperFirst(lastName(node.Id)), Owner: c.parents[node.Id], Result: ret, Raises: raises, Pure: first(node.Attrs["pure"]) == "true"}
			for _, parameter := range parameters {
				op.Params = append(op.Params, parameter.Type)
			}
			if d, ok := c.bound(graph.EdgeKindRealizes, node.Id); ok {
				op.GoName = d.obj.Name()
			}
			unit.spec.Operations[node.Id] = op
		}
		if node.Kind == graph.NodeKindObservation && c.parents[node.Id] == n.Id {
			d, ok := c.bound(graph.EdgeKindObserves, node.Id)
			if !ok {
				c.diagnostic(n, "lang.golang.observation-failed", "observation "+node.Id+" has no unique Go adapter", "bind exactly one adapter using //uc:observes")
				return nil
			}
			if d.pkg.PkgPath != unit.pkg.PkgPath {
				c.diagnostic(n, "lang.golang.observation-failed", "the observation adapter is outside the generated predicate package", "place the adapter in the property's Go package")
				return nil
			}
			unit.spec.Observers = append(unit.spec.Observers, ucObserver{Name: lastName(node.Id), Type: first(node.Attrs["type"]), Inputs: node.Attrs["inputs"]})
		}
	}
	earlier := map[string]string{}
	for _, symbol := range unit.spec.Symbols {
		goType := c.goType(symbol.Type, unit.pkg, nil)
		if goType == "" {
			c.diagnostic(n, "lang.golang.unsupported-contract", "the Go type of symbol "+symbol.Name+" is unavailable", "realize the model type and use supported finite model values")
			return nil
		}
		if d, ok := c.bound(graph.EdgeKindGenerates, n.Id+"."+symbol.Name); ok {
			fn, isfn := d.obj.(*types.Func)
			if !isfn {
				continue
			}
			sig := fn.Signature()
			for i := 0; i < sig.Params().Len(); i++ {
				p := sig.Params().At(i)
				model, ok := earlier[p.Name()]
				if !ok || model == "" {
					c.diagnostic(n, "lang.golang.generator-dependency", "generator "+fn.Name()+" depends on undeclared or later symbol "+p.Name(), "bind a generator that accepts only preceding forall symbols")
					return nil
				}
				unit.generatorArgs[symbol.Name] = append(unit.generatorArgs[symbol.Name], p.Name())
			}
			unit.generators[symbol.Name] = fn
		}
		if strings.HasSuffix(n.Attrs["symbols"][len(earlier)], " generated") && unit.generators[symbol.Name] == nil {
			c.diagnostic(n, "lang.golang.missing-generator", "generated symbol "+symbol.Name+" has no unique bound Go generator", "bind exactly one generator with //uc:generates before executing this property")
			return nil
		}
		earlier[symbol.Name] = symbol.Type
	}
	for _, node := range c.plan.Graph.Nodes() {
		if node.Kind != graph.NodeKindCase || c.parents[node.Id] != n.Id {
			continue
		}
		values := map[string]string{}
		for _, value := range node.Attrs["values"] {
			name, text, _ := strings.Cut(value, " = ")
			values[name] = text
		}
		var expressions []string
		for _, symbol := range unit.spec.Symbols {
			expressions = append(expressions, values[symbol.Name])
		}
		unit.cases = append(unit.cases, expressions)
	}
	return unit
}
func first(xs []string) string {
	if len(xs) > 0 {
		return xs[0]
	}
	return ""
}
func lastName(id string) string { _, name, _ := cutLast(id, "."); return name }
func (c *contractCompiler) qualify(obj types.Object, pkg *packages.Package, imports map[string]string) string {
	if obj.Pkg() == nil || obj.Pkg().Path() == pkg.PkgPath {
		if strings.HasPrefix(obj.Name(), "uc") {
			return c.protectName(obj.Name(), pkg, pkg.PkgPath+"."+obj.Name())
		}
		return obj.Name()
	}
	if !obj.Exported() {
		return ""
	}
	path := obj.Pkg().Path()
	alias := "ucpkg" + strconv.Itoa(len(imports))
	if existing, ok := imports[path]; ok {
		alias = existing
	} else if imports != nil {
		imports[path] = alias
	}
	return alias + "." + obj.Name()
}
func (c *contractCompiler) protectName(name string, pkg *packages.Package, key string) string {
	if c.foreignNames == nil {
		c.foreignNames = map[string]string{}
		c.foreignOriginal = map[string]string{}
	}
	if alias, ok := c.foreignNames[key]; ok {
		return alias
	}
	alias := "unicodeContractForeign" + strconv.Itoa(len(c.foreignOriginal))
	for c.foreignOriginal[alias] != "" || pkg.Types.Scope().Lookup(alias) != nil {
		alias += "_"
	}
	c.foreignNames[key] = alias
	c.foreignOriginal[alias] = name
	return alias
}
func (c *contractCompiler) goType(text string, pkg *packages.Package, imports map[string]string) string {
	kind, args := ucSplitType(text)
	switch kind {
	case "bool", "int", "string":
		return kind
	case "float":
		return "float64"
	case "bytes":
		return "[]byte"
	case "time", "duration":
		if imports != nil {
			imports["time"] = "time"
		}
		if kind == "time" {
			return "time.Time"
		}
		return "time.Duration"
	case "list":
		return "[]" + c.goType(args[0], pkg, imports)
	case "set":
		return "map[" + c.goType(args[0], pkg, imports) + "]struct{}"
	case "map":
		return "map[" + c.goType(args[0], pkg, imports) + "]" + c.goType(args[1], pkg, imports)
	case "optional":
		inner := c.goType(args[0], pkg, imports)
		if strings.HasPrefix(inner, "*") {
			return inner
		}
		return "*" + inner
	case "call":
		return ""
	}
	d, ok := c.bound(graph.EdgeKindRealizes, text)
	if !ok {
		return ""
	}
	name := c.qualify(d.obj, pkg, imports)
	if c.nodes[text].Form == "object" {
		name = "*" + name
	}
	return name
}
func (c *contractCompiler) literal(text, goType string, pkg *packages.Package) (string, error) {
	parsed := syntax.Parse("design/contract-case.uc", syntax.FileKindComponent, []byte("component contract-case \"case\" {\n  paths \"sample/**\"\n}\nproperty p invariant \"case\" {\n  about target\n  forall x: int\n  manual \"Literal decoding\"\n  case c {\n    x = "+text+"\n  }\n}\n"))
	if len(parsed.Diagnostics) > 0 || parsed.File == nil {
		return "", fmt.Errorf("cannot parse the declared input %q: %v", text, parsed.Diagnostics)
	}
	var value syntax.Value
	for _, d := range parsed.File.Decls {
		if p, ok := d.(*syntax.PropertyDecl); ok {
			for _, line := range p.Lines {
				if ca, ok := line.(*syntax.CaseDecl); ok {
					for _, line := range ca.Lines {
						if a, ok := line.(*syntax.Assignment); ok {
							value = a.Value
						}
					}
				}
			}
		}
	}
	return c.valueLiteral(value, goType, pkg)
}
func (c *contractCompiler) valueLiteral(value syntax.Value, t string, pkg *packages.Package) (string, error) {
	if strings.HasPrefix(t, "*") {
		_, none := value.(*syntax.NoneValue)
		_, record := value.(*syntax.BraceValue)
		if !none && (!record || strings.HasPrefix(t, "*map[")) {
			inner, err := c.valueLiteral(value, t[1:], pkg)
			if err != nil {
				return "", err
			}
			return "func() " + t + " { value := " + inner + "; return &value }()", nil
		}
	}
	switch v := value.(type) {
	case *syntax.NumberValue:
		return t + "(" + string(v.Text) + ")", nil
	case *syntax.StringValue:
		if t == "[]byte" {
			return "[]byte(" + strconv.Quote(string(v.Text)) + ")", nil
		}
		if t == "time.Time" {
			return "ucParseTime(" + strconv.Quote(string(v.Text)) + ")", nil
		}
		if t == "time.Duration" {
			return "ucParseDuration(" + strconv.Quote(string(v.Text)) + ")", nil
		}
		return t + "(" + strconv.Quote(string(v.Text)) + ")", nil
	case *syntax.BoolValue:
		return strconv.FormatBool(bool(v.Value)), nil
	case *syntax.NoneValue:
		return "(" + t + ")(nil)", nil
	case *syntax.NameValue:
		return t + "(" + strconv.Quote(string(v.Name)) + ")", nil
	case *syntax.ListValue:
		if !strings.HasPrefix(t, "[]") {
			if strings.HasPrefix(t, "map[") && strings.HasSuffix(t, "]struct{}") {
				i := strings.IndexByte(t, ']')
				keyType := t[4:i]
				var items []string
				for _, item := range v.Items {
					x, err := c.valueLiteral(item, keyType, pkg)
					if err != nil {
						return "", err
					}
					items = append(items, x+":{}")
				}
				return t + "{" + strings.Join(items, ",") + "}", nil
			}
			return "", fmt.Errorf("a list case needs a Go slice or set")
		}
		var items []string
		for _, item := range v.Items {
			x, err := c.valueLiteral(item, t[2:], pkg)
			if err != nil {
				return "", err
			}
			items = append(items, x)
		}
		return t + "{" + strings.Join(items, ",") + "}", nil
	case *syntax.BraceValue:
		if strings.HasPrefix(t, "map[") {
			i := strings.IndexByte(t, ']')
			keyType, valueType := t[4:i], t[i+1:]
			var items []string
			for _, entry := range v.Entries {
				key := ""
				if entry.Key.String != nil {
					key = keyType + "(" + strconv.Quote(string(*entry.Key.String)) + ")"
				} else if entry.Key.Number != nil {
					key = keyType + "(" + string(*entry.Key.Number) + ")"
				} else if entry.Key.Name != nil {
					key = keyType + "(" + strconv.Quote(string(*entry.Key.Name)) + ")"
				}
				x, err := c.valueLiteral(entry.Value, valueType, pkg)
				if err != nil {
					return "", err
				}
				items = append(items, key+":"+x)
			}
			return t + "{" + strings.Join(items, ",") + "}", nil
		}
		name := strings.TrimPrefix(t, "*")
		if original, ok := c.foreignOriginal[name]; ok {
			name = original
		}
		var named *types.Named
		for _, d := range c.declarations {
			if ty, ok := d.obj.(*types.TypeName); ok && ty.Name() == lastGoName(name) && ((!strings.Contains(name, ".") && ty.Pkg().Path() == pkg.PkgPath) || (strings.Contains(name, ".") && c.literalImports[ty.Pkg().Path()] == strings.TrimSuffix(name, "."+ty.Name()))) {
				named, _ = ty.Type().(*types.Named)
				break
			}
		}
		if named == nil {
			return "", fmt.Errorf("cannot resolve record literal type %s", t)
		}
		structure, ok := named.Underlying().(*types.Struct)
		if !ok {
			return "", fmt.Errorf("%s is not a record type", t)
		}
		var items []string
		for _, entry := range v.Entries {
			field := upperFirst(string(*entry.Key.Name))
			var ft types.Type
			for i := 0; i < structure.NumFields(); i++ {
				if structure.Field(i).Name() == field {
					ft = structure.Field(i).Type()
				}
			}
			if ft == nil {
				return "", fmt.Errorf("record field %s is unavailable", field)
			}
			typeText := types.TypeString(ft, func(p *types.Package) string {
				if p.Path() == pkg.PkgPath {
					return ""
				}
				if alias, ok := c.literalImports[p.Path()]; ok {
					return alias
				}
				alias := "ucpkg" + strconv.Itoa(len(c.literalImports))
				c.literalImports[p.Path()] = alias
				return alias
			})
			x, err := c.valueLiteral(entry.Value, typeText, pkg)
			if err != nil {
				return "", err
			}
			items = append(items, field+":"+x)
		}
		prefix := ""
		if strings.HasPrefix(t, "*") {
			prefix = "&"
		}
		return prefix + name + "{" + strings.Join(items, ",") + "}", nil
	}
	return "", fmt.Errorf("the case value has an unsupported form")
}
func (c *contractCompiler) render(file *contractFile) []byte {
	c.literalImports = file.imports
	c.foreignNames, c.foreignOriginal = map[string]string{}, map[string]string{}
	var body strings.Builder
	var bridges []string
	for index, unit := range file.units {
		id := strconv.Itoa(index)
		specName := "ucSpec" + id
		raw, _ := json.Marshal(unit.spec)
		fmt.Fprintf(&body, "func %s() *ucContract { var s ucContract; if err := json.Unmarshal([]byte(%s), &s); err != nil { panic(err) }; ", specName, strconv.Quote(string(raw)))
		usedOps := map[string]bool{}
		usedErrors := map[string]bool{}
		expressions := append(append([]*ucExpr{}, unit.spec.Requires...), unit.spec.Ensures...)
		expressions = append(expressions, unit.spec.Call)
		for _, expr := range expressions {
			ucWalk(expr, func(e *ucExpr) {
				if e.Kind == "call" {
					name := ucName(e.Children[0])
					if _, ok := unit.spec.Operations[name]; !ok && e.Children[0].Kind == "field" {
						env := ucEnvironment{spec: &unit.spec, values: map[string]ucValue{}}
						for _, symbol := range unit.spec.Symbols {
							env.values[symbol.Name] = ucValue{Type: symbol.Type}
						}
						for _, observer := range unit.spec.Observers {
							env.values[observer.Name] = ucValue{Type: observer.Type}
						}
						receiverType := env.typeOf(e.Children[0].Children[0], nil)
						if kind, args := ucSplitType(receiverType); kind == "optional" {
							receiverType = args[0]
						}
						name = receiverType + "." + e.Children[0].Text
					}
					usedOps[name] = true
				}
				if e.Kind == "raised" || e.Kind == "failure" {
					usedErrors[e.Text] = true
				}
			})
		}
		for op := range usedOps {
			for _, errType := range unit.spec.Operations[op].Raises {
				usedErrors[errType] = true
			}
		}
		for id, ty := range unit.spec.Types {
			if ty.Form != "error" {
				continue
			}
			for parent := ty.Parent; parent != ""; parent = unit.spec.Types[parent].Parent {
				if usedErrors[parent] {
					usedErrors[id] = true
					break
				}
			}
		}
		for _, key := range slices.Sorted(maps.Keys(unit.spec.Operations)) {
			if !usedOps[key] {
				continue
			}
			op := unit.spec.Operations[key]
			d, ok := c.bound(graph.EdgeKindRealizes, key)
			if !ok {
				continue
			}
			fn, ok := d.obj.(*types.Func)
			if !ok || fn.Signature().Recv() != nil {
				continue
			}
			selector := c.qualify(fn, file.pkg, file.imports)
			if selector == "" {
				continue
			}
			fmt.Fprintf(&body, "{ op := s.Operations[%s]; op.Function = reflect.ValueOf(%s); s.Operations[%s] = op };", strconv.Quote(op.Name), selector, strconv.Quote(op.Name))
		}
		body.WriteString("s.ErrorTypes = map[string]reflect.Type{};")
		errorInterface := types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
		for _, key := range slices.Sorted(maps.Keys(unit.spec.Types)) {
			if unit.spec.Types[key].Form != "error" || !usedErrors[key] {
				continue
			}
			d, ok := c.bound(graph.EdgeKindRealizes, key)
			if !ok {
				continue
			}
			selector := c.qualify(d.obj, file.pkg, file.imports)
			if selector == "" {
				continue
			}
			typ := d.obj.Type()
			if !types.Implements(typ, errorInterface) {
				typ = types.NewPointer(typ)
				selector = "*" + selector
			}
			if !types.Implements(typ, errorInterface) {
				continue
			}
			fmt.Fprintf(&body, "s.ErrorTypes[%s]=reflect.TypeFor[%s]();", strconv.Quote(key), selector)
		}
		for i, o := range unit.spec.Observers {
			d, _ := c.bound(graph.EdgeKindObserves, unit.node.Id+"."+o.Name)
			fmt.Fprintf(&body, "s.Observers[%d].Read=reflect.ValueOf(%s);", i, c.qualify(d.obj, file.pkg, file.imports))
		}
		body.WriteString("return &s }\n")
		predicate := ""
		if unit.predicate != nil {
			predicate = c.qualify(unit.predicate, file.pkg, file.imports)
		} else {
			predicate = (&Backend{}).MapName(lang.NameRolePredicate, lastName(unit.node.Id))
			bridge := "ucPredicate" + id + "_"
			handle := "t"
			symbolNames := map[string]bool{}
			for _, symbol := range unit.spec.Symbols {
				symbolNames[symbol.Name] = true
			}
			for symbolNames[handle] {
				handle += "_"
			}
			for symbolNames["unicodeGenerated"+strings.TrimPrefix(bridge, "uc")] {
				bridge += "_"
			}
			bridges = append(bridges, bridge)
			var params, args, bridgeParams, values []string
			for i, symbol := range unit.spec.Symbols {
				t := c.goType(symbol.Type, file.pkg, file.imports)
				name := c.protectName(symbol.Name, file.pkg, unit.node.Id+"#symbol#"+symbol.Name)
				params = append(params, name+" "+t)
				args = append(args, name)
				local := "value" + strconv.Itoa(i)
				bridgeParams = append(bridgeParams, local+" "+t)
				values = append(values, strconv.Quote(symbol.Name)+":reflect.ValueOf(&"+local+").Elem()")
			}
			fmt.Fprintf(&body, "//uc:checks %s\nfunc %s(%s rapid.TB", unit.node.Id, predicate, handle)
			if len(params) > 0 {
				body.WriteString("," + strings.Join(params, ","))
			}
			fmt.Fprintf(&body, "){ %s(%s", bridge, handle)
			if len(args) > 0 {
				body.WriteString("," + strings.Join(args, ","))
			}
			body.WriteString(") }\n")
			fmt.Fprintf(&body, "func %s(t rapid.TB", bridge)
			if len(bridgeParams) > 0 {
				body.WriteString("," + strings.Join(bridgeParams, ","))
			}
			fmt.Fprintf(&body, "){ ucCheck(t,%s(),map[string]reflect.Value{%s}) }\n", specName, strings.Join(values, ","))
		}
		fmt.Fprintf(&body, "func ucInvoke%s(tb rapid.TB, s *ucContract, values map[string]reflect.Value){", id)
		if unit.predicate != nil {
			body.WriteString("pre:=*s;pre.Call=nil;pre.Ensures=nil;ucCheck(tb,&pre,values);")
		}
		var args []string
		for _, s := range unit.spec.Symbols {
			args = append(args, "values["+strconv.Quote(s.Name)+"].Interface().("+c.goType(s.Type, file.pkg, file.imports)+")")
		}
		fmt.Fprintf(&body, "%s(tb", predicate)
		if len(args) > 0 {
			body.WriteString("," + strings.Join(args, ","))
		}
		body.WriteString(")}\n")
		fmt.Fprintf(&body, "func ucInputs%s(rt *rapid.T,s *ucContract) map[string]reflect.Value{ values:=map[string]reflect.Value{};", id)
		for _, s := range unit.spec.Symbols {
			if fn := unit.generators[s.Name]; fn != nil {
				selector := c.qualify(fn, file.pkg, file.imports)
				if selector == "" {
					c.diagnostic(unit.node, "lang.golang.unsupported-contract", "a generator is inaccessible from the predicate package", "place it in the predicate package or export it")
					return nil
				}
				var args []string
				for _, name := range unit.generatorArgs[s.Name] {
					var model string
					for _, prior := range unit.spec.Symbols {
						if prior.Name == name {
							model = prior.Type
						}
					}
					args = append(args, "values["+strconv.Quote(name)+"].Interface().("+c.goType(model, file.pkg, file.imports)+")")
				}
				fmt.Fprintf(&body, "values[%s]=reflect.ValueOf(%s(%s).Draw(rt,%s));", strconv.Quote(s.Name), selector, strings.Join(args, ","), strconv.Quote(s.Name))
			} else {
				fmt.Fprintf(&body, "values[%s]=ucDraw(rt,reflect.TypeFor[%s](),%s,s.Types,%#v,%s,0);", strconv.Quote(s.Name), c.goType(s.Type, file.pkg, file.imports), strconv.Quote(s.Type), unit.limits, strconv.Quote(s.Name))
			}
		}
		body.WriteString("return values}\n")
		fmt.Fprintf(&body, "func ucRandom%s(rt *rapid.T,s *ucContract){ucInvoke%s(rt,s,ucInputs%s(rt,s))}\n", id, id, id)
		fmt.Fprintf(&body, "func ucDrive%s(t *testing.T, mode string, budget time.Duration, r *ucRunResult) { s:=%s(); *r=ucRunResult{Id:s.ID,Passed:true,Observations:append([]string{},s.Observe...)}; for _,o:=range s.Observers {r.Observations=append(r.Observations,o.Name)}; restore:=ucContext(r,false); defer restore(); defer func(){ucFailedPredicate(t,s,r);ucFinish(t,s,r)}();", id, specName)
		for _, ca := range unit.cases {
			var vals []string
			for i, text := range ca {
				x, err := c.literal(text, c.goType(unit.spec.Symbols[i].Type, file.pkg, file.imports), file.pkg)
				if err != nil {
					c.diagnostic(unit.node, "lang.golang.unsupported-case", err.Error(), "use a finite replay value supported by the Go backend")
					return nil
				}
				vals = append(vals, strconv.Quote(unit.spec.Symbols[i].Name)+":reflect.ValueOf("+x+")")
			}
			fmt.Fprintf(&body, "ucInvoke%s(t,s,map[string]reflect.Value{%s});", id, strings.Join(vals, ","))
		}
		fmt.Fprintf(&body, "if mode!=\"replay\" { restoreRandom:=ucContext(r,true); defer restoreRandom(); start:=time.Now(); rapid.Check(t,func(rt *rapid.T){if mode==\"explore\"&&time.Since(start)>budget{return};ucRandom%s(rt,s)})} }\n", id)
		fmt.Fprintf(&body, "// FuzzUnicode_%x checks %s.\nfunc FuzzUnicode_%x(f *testing.F){f.Fuzz(rapid.MakeFuzz(func(rt *rapid.T){restore:=ucContext(nil,true);defer restore();ucRandom%s(rt,%s())}))}\n", sha256.Sum256([]byte(unit.node.Id)), unit.node.Id, sha256.Sum256([]byte(unit.node.Id)), id, specName)
	}
	body.WriteString("func TestUnicodeContracts(t *testing.T){ var selected []string; _=json.Unmarshal([]byte(os.Getenv(\"UNICODE_CONTRACT_IDS\")),&selected); mode:=\"test\";if os.Getenv(\"UNICODE_CONTRACT_RESULTS\")!=\"\" {mode=\"replay\"};if os.Getenv(\"UNICODE_CONTRACT_EXPLORE\")!=\"\"{mode=\"explore\"}; budget,_:=time.ParseDuration(os.Getenv(\"UNICODE_CONTRACT_EXPLORE\")); results:=[]ucRunResult{}; defer func(){if path:=os.Getenv(\"UNICODE_CONTRACT_RESULTS\");path!=\"\"{data,err:=json.Marshal(results);if err!=nil{t.Error(err);return};if err=os.WriteFile(path,data,0600);err!=nil{t.Error(err)}}}();")
	for i, unit := range file.units {
		fmt.Fprintf(&body, "if len(selected)==0||slices.Contains(selected,%s){t.Run(%s,func(t *testing.T){var result ucRunResult;defer func(){if result.Id==\"\"{result=ucRunResult{Id:%s,Passed:false}};result.Passed=result.Passed&&!t.Failed();results=append(results,result)}();ucDrive%d(t,mode,budget,&result)})};", strconv.Quote(unit.node.Id), strconv.Quote(unit.node.Id), strconv.Quote(unit.node.Id), i)
	}
	body.WriteString("}\n")
	runtimeFile, err := parser.ParseFile(token.NewFileSet(), "runtime.go", contractRuntime, 0)
	if err != nil {
		panic(err)
	}
	imports := map[string]string{"testing": "", "os": ""}
	for _, spec := range runtimeFile.Imports {
		path, _ := strconv.Unquote(spec.Path.Value)
		imports[path] = ""
	}
	for path, alias := range file.imports {
		imports[path] = alias
	}
	var out bytes.Buffer
	fmt.Fprintf(&out, "// Code generated by unicode. DO NOT EDIT.\n\npackage %s\n\nimport (\n", file.pkg.Name)
	for _, path := range slices.Sorted(maps.Keys(imports)) {
		fmt.Fprintf(&out, "%s %s\n", imports[path], strconv.Quote(path))
	}
	out.WriteString(")\n")
	start := 0
	for _, decl := range runtimeFile.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.IMPORT {
			start = int(gen.End())
		}
	}
	out.WriteString(contractRuntime[start:])
	out.WriteString(body.String())
	fset := token.NewFileSet()
	tree, err := parser.ParseFile(fset, "unicode_contracts_gen_test.go", out.Bytes(), parser.ParseComments)
	if err != nil {
		c.diagnostic(file.units[0].node, "lang.golang.generate-failed", err.Error(), "correct unsupported Go bindings before generation")
		return nil
	}
	owned := map[string]bool{}
	for _, decl := range runtimeFile.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if strings.HasPrefix(d.Name.Name, "uc") {
				owned[d.Name.Name] = true
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch spec := spec.(type) {
				case *ast.TypeSpec:
					if strings.HasPrefix(spec.Name.Name, "uc") {
						owned[spec.Name.Name] = true
					}
				case *ast.ValueSpec:
					for _, name := range spec.Names {
						if strings.HasPrefix(name.Name, "uc") {
							owned[name.Name] = true
						}
					}
				}
			}
		}
	}
	for i := range file.units {
		owned["ucSpec"+strconv.Itoa(i)] = true
		owned["ucDrive"+strconv.Itoa(i)] = true
		owned["ucInputs"+strconv.Itoa(i)] = true
		owned["ucInvoke"+strconv.Itoa(i)] = true
		owned["ucRandom"+strconv.Itoa(i)] = true
	}
	for _, name := range bridges {
		owned[name] = true
	}
	ast.Inspect(tree, func(node ast.Node) bool {
		if id, ok := node.(*ast.Ident); ok && owned[id.Name] {
			id.Name = "unicodeGenerated" + strings.TrimPrefix(id.Name, "uc")
		}
		return true
	})
	ast.Inspect(tree, func(node ast.Node) bool {
		if id, ok := node.(*ast.Ident); ok {
			if original, ok := c.foreignOriginal[id.Name]; ok {
				id.Name = original
			}
		}
		return true
	})
	// Check the final emitted names, after helper names and protected source
	// identifiers have been restored. The loaded scope excludes owned outputs.
	declared := map[string]bool{}
	for _, decl := range tree.Decls {
		var names []*ast.Ident
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Recv == nil {
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
			if name.Name == "_" {
				continue
			}
			if declared[name.Name] || file.pkg.Types.Scope().Lookup(name.Name) != nil {
				c.diagnostic(file.units[0].node, "lang.golang.driver-name-collision", "generated declaration "+name.Name+" conflicts with another declaration in package "+file.pkg.PkgPath, "rename the handwritten declaration or give the conflicting properties distinct predicate names")
				return nil
			}
			declared[name.Name] = true
		}
	}
	var formatted bytes.Buffer
	if err := format.Node(&formatted, fset, tree); err != nil {
		c.diagnostic(file.units[0].node, "lang.golang.generate-failed", err.Error(), "correct unsupported Go bindings before generation")
		return nil
	}
	return formatted.Bytes()
}

func lastGoName(name string) string {
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		return name[i+1:]
	}
	return name
}
