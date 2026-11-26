package golang

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/doc"
	"go/token"
	"go/types"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/types/typeutil"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
)

// unimplemented is the message of the panic that is the whole body of a stub
// (docs/design.md §6.1).
const unimplemented = "unicode: unimplemented"

// extraction builds the code layer of the loaded packages: first a node for
// every package, file, type, function and method, then the edges between them.
type extraction struct {
	root     string
	nodes    map[string]graph.Node
	edges    map[edgeKey]*graph.Edge
	sites    map[edgeKey][]diag.Position
	declared []declared
}

type edgeKey struct {
	kind     graph.EdgeKind
	from, to string
}

// declared is a declaration that is a node: a type, a function or a method.
type declared struct {
	id   string
	pkg  *packages.Package
	file sourceFile
	spec *ast.TypeSpec   // a type's
	name *types.TypeName // a type's
	decl *ast.FuncDecl   // a function's or a method's
	fn   *types.Func     // a function's or a method's
}

func extract(l loaded) lang.Extracted {
	x := &extraction{
		root:  l.root,
		nodes: map[string]graph.Node{},
		edges: map[edgeKey]*graph.Edge{},
		sites: map[edgeKey][]diag.Position{},
	}
	for _, p := range l.packages {
		x.addPackage(p)
	}
	for _, d := range x.declared {
		if d.name != nil {
			x.addTypeEdges(d)
		} else {
			x.addFuncEdges(d)
		}
	}
	x.addImplements()
	return x.result()
}

func (x *extraction) addPackage(p *packages.Package) {
	files := filesUnder(x.root, p)
	pkgID := "go:" + p.PkgPath
	// The package is written where its doc comment is, or else in its first file.
	written := files[0].syntax
	for _, f := range files {
		if f.syntax.Doc != nil {
			written = f.syntax
			break
		}
	}
	x.addNode(graph.Node{
		Id: pkgID, Kind: graph.NodeKindContainer, Form: "package", Origin: x.position(p, written.Package),
		Summary: synopsis(written.Doc), Attrs: map[string][]string{"language": {language}, "path": {p.PkgPath}},
	})
	for _, f := range files {
		fileID := "file:" + f.rel
		x.addNode(graph.Node{
			Id: fileID, Kind: graph.NodeKindContainer, Form: "file", Origin: x.position(p, f.syntax.Package),
			Attrs: map[string][]string{"language": {language}, "path": {f.rel}},
		})
		x.addEdge(graph.EdgeKindContains, pkgID, fileID, x.position(p, f.syntax.Package))
		for _, spec := range f.syntax.Imports {
			if imported, err := strconv.Unquote(spec.Path.Value); err == nil {
				x.addEdge(graph.EdgeKindImports, fileID, "go:"+imported, x.position(p, spec.Pos()))
			}
		}
		for _, decl := range f.syntax.Decls {
			x.addDecl(p, f, fileID, decl)
		}
	}
}

func (x *extraction) addDecl(p *packages.Package, f sourceFile, fileID string, decl ast.Decl) {
	switch decl := decl.(type) {
	case *ast.GenDecl:
		if decl.Tok != token.TYPE {
			return
		}
		for _, s := range decl.Specs {
			spec := s.(*ast.TypeSpec)
			name, ok := p.TypesInfo.Defs[spec.Name].(*types.TypeName)
			if !ok || spec.Assign.IsValid() {
				continue
			}
			d := declared{id: typeID(name), pkg: p, file: f, spec: spec, name: name}
			x.addNode(graph.Node{
				Id: d.id, Kind: graph.NodeKindType, Form: typeForm(name), Origin: x.position(p, spec.Name.Pos()),
				Summary: synopsis(typeDoc(decl, spec)), Attrs: map[string][]string{"name": {name.Name()}},
			})
			x.addEdge(graph.EdgeKindContains, fileID, d.id, x.position(p, spec.Name.Pos()))
			x.declared = append(x.declared, d)
		}
	case *ast.FuncDecl:
		fn, ok := p.TypesInfo.Defs[decl.Name].(*types.Func)
		if !ok || decl.Name.Name == "_" || decl.Recv == nil && decl.Name.Name == "init" {
			// Neither has an identity: a package may declare several of each.
			return
		}
		d := declared{id: funcID(fn), pkg: p, file: f, decl: decl, fn: fn}
		form, parent := "func", fileID
		if decl.Recv != nil {
			form, parent = "method", typeID(receiver(fn).Obj())
		}
		x.addNode(graph.Node{
			Id: d.id, Kind: graph.NodeKindOp, Form: form, Origin: x.position(p, decl.Name.Pos()),
			Summary: synopsis(decl.Doc), Attrs: map[string][]string{
				"name":     {designName(fn.Name())},
				"testKind": {testKind(p.TypesInfo, f, decl, fn)},
				"stub":     {strconv.FormatBool(isStub(p.TypesInfo, decl.Body))},
			},
		})
		x.addEdge(graph.EdgeKindContains, parent, d.id, x.position(p, decl.Name.Pos()))
		x.declared = append(x.declared, d)
	}
}

// addTypeEdges adds the refers edges of a type to the types its declaration
// mentions, and its extends edges to the types it embeds.
func (x *extraction) addTypeEdges(d declared) {
	origin := x.position(d.pkg, d.spec.Name.Pos())
	refer := func(t *types.Named) { x.addEdge(graph.EdgeKindRefers, d.id, typeID(t.Obj()), origin) }
	embed := func(t types.Type) {
		if named := namedOf(t); named != nil {
			x.addEdge(graph.EdgeKindExtends, d.id, typeID(named.Obj()), origin)
		}
	}
	switch rhs := types.Unalias(d.pkg.TypesInfo.TypeOf(d.spec.Type)).(type) {
	case *types.Struct:
		for field := range rhs.Fields() {
			if field.Embedded() {
				embed(field.Type())
			} else {
				mentioned(field.Type(), refer)
			}
		}
	case *types.Interface:
		for m := range rhs.ExplicitMethods() {
			mentioned(m.Type(), refer)
		}
		for t := range rhs.EmbeddedTypes() {
			embed(t)
		}
	default:
		mentioned(rhs, refer)
	}
}

// addFuncEdges adds the refers edges of a function to the types of its
// parameters and results, and its calls edges.
func (x *extraction) addFuncEdges(d declared) {
	origin := x.position(d.pkg, d.decl.Name.Pos())
	sig := d.fn.Signature()
	for _, tuple := range []*types.Tuple{sig.Params(), sig.Results()} {
		for v := range tuple.Variables() {
			mentioned(v.Type(), func(t *types.Named) { x.addEdge(graph.EdgeKindRefers, d.id, typeID(t.Obj()), origin) })
		}
	}
	if d.decl.Body == nil {
		return
	}
	ast.Inspect(d.decl.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		callee, ok := typeutil.Callee(d.pkg.TypesInfo, call).(*types.Func)
		name := calleeName(call.Fun)
		if !ok || name == nil || !packageLevel(callee) {
			return true
		}
		site := x.position(d.pkg, name.Pos())
		key := x.addEdge(graph.EdgeKindCalls, d.id, funcID(callee.Origin()), site)
		x.sites[key] = append(x.sites[key], site)
		return true
	})
}

// addImplements adds an implements edge from every type that is not an
// interface to every interface with methods whose method set its pointer's
// method set holds. Methods are compared by their package-qualified names and
// signatures, so the variant a package was loaded in does not matter.
func (x *extraction) addImplements() {
	var concrete, interfaces []declared
	for _, d := range x.declared {
		if d.name == nil || d.name.Type().(*types.Named).TypeParams().Len() > 0 {
			continue
		}
		if iface, ok := d.name.Type().Underlying().(*types.Interface); !ok {
			concrete = append(concrete, d)
		} else if iface.IsMethodSet() && iface.NumMethods() > 0 {
			interfaces = append(interfaces, d)
		}
	}
	for _, c := range concrete {
		have := methodKeys(types.NewMethodSet(types.NewPointer(c.name.Type())))
		for _, i := range interfaces {
			want := methodKeys(types.NewMethodSet(i.name.Type()))
			if implementsAll(have, want) {
				x.addEdge(graph.EdgeKindImplements, c.id, i.id, x.position(c.pkg, c.spec.Name.Pos()))
			}
		}
	}
}

// methodKeys maps each method of a method set, by its package-qualified
// name, to its signature written with package paths.
func methodKeys(set *types.MethodSet) map[string]string {
	keys := map[string]string{}
	for sel := range set.Methods() {
		m := sel.Obj().(*types.Func)
		keys[m.Id()] = types.TypeString(m.Type(), pathQualifier)
	}
	return keys
}

func implementsAll(have, want map[string]string) bool {
	for id, sig := range want {
		if have[id] != sig {
			return false
		}
	}
	return true
}

func (x *extraction) addNode(n graph.Node) {
	n.Layer, n.Source = graph.LayerCode, graph.SourceExtracted
	x.nodes[n.Id] = n
}

// addEdge adds an extracted edge and returns its key; an edge already present
// stays as it is. The result keeps only the extracted edges whose ends are
// both nodes, so an edge may be added before its ends are.
func (x *extraction) addEdge(kind graph.EdgeKind, from, to string, origin diag.Position) edgeKey {
	key := edgeKey{kind, from, to}
	if _, ok := x.edges[key]; !ok {
		x.edges[key] = &graph.Edge{Kind: kind, From: from, To: to, Source: graph.SourceExtracted, Origin: origin}
	}
	return key
}

// result writes the nodes sorted by identity, the edges by kind, start and
// end, and each call's sites in the order of the source.
func (x *extraction) result() lang.Extracted {
	var out lang.Extracted
	for _, id := range slices.Sorted(maps.Keys(x.nodes)) {
		out.Fragment.Nodes = append(out.Fragment.Nodes, x.nodes[id])
	}
	keys := slices.SortedFunc(maps.Keys(x.edges), func(a, b edgeKey) int {
		return cmp.Or(cmp.Compare(a.kind, b.kind), cmp.Compare(a.from, b.from), cmp.Compare(a.to, b.to))
	})
	for _, key := range keys {
		e := *x.edges[key]
		_, fromNode := x.nodes[e.From]
		_, toNode := x.nodes[e.To]
		if e.Source == graph.SourceExtracted && (!fromNode || !toNode) {
			continue
		}
		if sites := x.sites[key]; len(sites) > 0 {
			slices.SortFunc(sites, comparePositions)
			e.Origin = sites[0]
			e.Attrs = map[string][]string{"sites": {}}
			for _, s := range sites {
				e.Attrs["sites"] = append(e.Attrs["sites"], fmt.Sprintf("%s:%d:%d", s.Path, s.Line, s.Column))
			}
		}
		out.Fragment.Edges = append(out.Fragment.Edges, e)
	}
	return out
}

func comparePositions(a, b diag.Position) int {
	return cmp.Or(cmp.Compare(a.Path, b.Path), cmp.Compare(a.Line, b.Line), cmp.Compare(a.Column, b.Column))
}

func (x *extraction) position(p *packages.Package, pos token.Pos) diag.Position {
	at := p.Fset.Position(pos)
	rel, err := filepath.Rel(x.root, at.Filename)
	if err != nil {
		rel = at.Filename
	}
	return diag.Position{Path: filepath.ToSlash(rel), Line: at.Line, Column: at.Column}
}

func typeID(name *types.TypeName) string { return "go:" + name.Pkg().Path() + "." + name.Name() }

// funcID is the identity of a package-level function, or of a method under
// its receiver's type.
func funcID(fn *types.Func) string {
	if recv := receiver(fn); recv != nil {
		return typeID(recv.Obj()) + "." + fn.Name()
	}
	return "go:" + fn.Pkg().Path() + "." + fn.Name()
}

// receiver is the named type a method is declared on, or nil for a function
// or a method of an unnamed interface.
func receiver(fn *types.Func) *types.Named {
	recv := fn.Signature().Recv()
	if recv == nil {
		return nil
	}
	return namedOf(recv.Type())
}

// namedOf is the named type t is, or points to, or nil.
func namedOf(t types.Type) *types.Named {
	if p, ok := types.Unalias(t).(*types.Pointer); ok {
		t = p.Elem()
	}
	named, _ := types.Unalias(t).(*types.Named)
	if named == nil {
		return nil
	}
	return named.Origin()
}

// packageLevel tells whether a function is declared at the package level,
// directly or as a method of a package-level type: a type declared in a
// function body shares its name's identity with one of the package.
func packageLevel(fn *types.Func) bool {
	if fn.Pkg() == nil {
		return false
	}
	if fn.Signature().Recv() == nil {
		return true
	}
	recv := receiver(fn)
	return recv != nil && recv.Obj().Parent() == recv.Obj().Pkg().Scope()
}

// mentioned calls visit with every named type that t mentions, without
// looking inside the named types themselves.
func mentioned(t types.Type, visit func(*types.Named)) {
	switch t := types.Unalias(t).(type) {
	case *types.Named:
		if t.Obj().Pkg() != nil {
			visit(t.Origin())
		}
		for arg := range t.TypeArgs().Types() {
			mentioned(arg, visit)
		}
	case *types.Pointer:
		mentioned(t.Elem(), visit)
	case *types.Slice:
		mentioned(t.Elem(), visit)
	case *types.Array:
		mentioned(t.Elem(), visit)
	case *types.Chan:
		mentioned(t.Elem(), visit)
	case *types.Map:
		mentioned(t.Key(), visit)
		mentioned(t.Elem(), visit)
	case *types.Signature:
		mentioned(t.Params(), visit)
		mentioned(t.Results(), visit)
	case *types.Tuple:
		for v := range t.Variables() {
			mentioned(v.Type(), visit)
		}
	case *types.Struct:
		for field := range t.Fields() {
			mentioned(field.Type(), visit)
		}
	case *types.Interface:
		for m := range t.Methods() {
			mentioned(m.Type(), visit)
		}
	}
}

// calleeName is the identifier that names the function a call calls, or nil.
func calleeName(fun ast.Expr) *ast.Ident {
	switch fun := ast.Unparen(fun).(type) {
	case *ast.Ident:
		return fun
	case *ast.SelectorExpr:
		return fun.Sel
	case *ast.IndexExpr:
		return calleeName(fun.X)
	case *ast.IndexListExpr:
		return calleeName(fun.X)
	}
	return nil
}

func typeForm(name *types.TypeName) string {
	switch name.Type().Underlying().(type) {
	case *types.Struct:
		return "struct"
	case *types.Interface:
		return "interface"
	}
	return "defined"
}

// typeDoc is a type's doc comment: its spec's in a parenthesized
// declaration, the declaration's otherwise.
func typeDoc(decl *ast.GenDecl, spec *ast.TypeSpec) *ast.CommentGroup {
	if decl.Lparen.IsValid() {
		return spec.Doc
	}
	return decl.Doc
}

// synopsis is the first sentence of a doc comment, without its directives.
func synopsis(group *ast.CommentGroup) string {
	if group == nil {
		return ""
	}
	return new(doc.Package).Synopsis(group.Text())
}

// testKind classifies a function the way go test does (docs/design.md §4):
// FuzzX(*testing.F) is a fuzz test; TestX(*testing.T) is a property when it
// calls rapid.Check and an example otherwise.
func testKind(info *types.Info, f sourceFile, decl *ast.FuncDecl, fn *types.Func) string {
	sig := fn.Signature()
	if decl.Recv != nil || !strings.HasSuffix(f.rel, "_test.go") || sig.TypeParams().Len() > 0 ||
		sig.Params().Len() != 1 || sig.Results().Len() != 0 {
		return "none"
	}
	param := types.TypeString(sig.Params().At(0).Type(), pathQualifier)
	switch {
	case isTestName(fn.Name(), "Fuzz") && param == "*testing.F":
		return "fuzz"
	case isTestName(fn.Name(), "Test") && param == "*testing.T" && callsRapidCheck(info, decl.Body):
		return "property"
	case isTestName(fn.Name(), "Test") && param == "*testing.T":
		return "example"
	}
	return "none"
}

// isTestName tells whether name is prefix followed by nothing or by a
// character that is not a lower-case letter, as go test requires.
func isTestName(name, prefix string) bool {
	rest, ok := strings.CutPrefix(name, prefix)
	if !ok {
		return false
	}
	r, _ := utf8.DecodeRuneInString(rest)
	return rest == "" || !unicode.IsLower(r)
}

func callsRapidCheck(info *types.Info, body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			fn, ok := typeutil.Callee(info, call).(*types.Func)
			found = found || ok && fn.Pkg() != nil && fn.Pkg().Path() == "pgregory.net/rapid" && fn.Name() == "Check"
		}
		return !found
	})
	return found
}

// isStub tells whether a body is exactly the unimplemented marker.
func isStub(info *types.Info, body *ast.BlockStmt) bool {
	if body == nil || len(body.List) != 1 {
		return false
	}
	stmt, ok := body.List[0].(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := stmt.X.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 || call.Ellipsis.IsValid() {
		return false
	}
	fun, ok := call.Fun.(*ast.Ident)
	if builtin, isBuiltin := info.Uses[fun].(*types.Builtin); !ok || !isBuiltin || builtin.Name() != "panic" {
		return false
	}
	lit, ok := call.Args[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return false
	}
	message, err := strconv.Unquote(lit.Value)
	return err == nil && message == unimplemented
}

// pathQualifier writes a package by its path.
func pathQualifier(p *types.Package) string { return p.Path() }
