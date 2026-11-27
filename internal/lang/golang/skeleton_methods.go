package golang

import (
	"fmt"
	"go/format"
	"go/types"
	"maps"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
)

// interfaceRealizers keeps native signatures, including promoted methods and
// interface details which the language-neutral signature deliberately omits.
func interfaceRealizers(plan lang.GenPlan, l loaded) map[string]nativeDeclaration {
	code := map[string]nativeDeclaration{}
	for _, pkg := range l.packages {
		for _, obj := range pkg.TypesInfo.Defs {
			if name, ok := obj.(*types.TypeName); ok {
				code[typeID(name)] = nativeDeclaration{obj: name, pkg: pkg}
			}
		}
	}
	realized := map[string]nativeDeclaration{}
	for _, edge := range plan.Graph.Edges() {
		if edge.Kind == graph.EdgeKindRealizes {
			if declaration, ok := code[edge.From]; ok {
				realized[edge.To] = declaration
			}
		}
	}
	return realized
}
func (p *skeletonPlanner) hasInterfaceImplementations() bool {
	for _, edge := range p.plan.Graph.Edges() {
		if edge.Kind == graph.EdgeKindImplements && p.nodes[edge.From].Layer == graph.LayerDesign {
			return true
		}
	}
	return false
}
func skeletonSignaturesEqual(a, b *types.Func) bool {
	identity := nativeIdentity{packages: map[string]*types.Package{}, named: map[string][]nativeNamed{}}
	return types.Identical(identity.typ(a.Signature()), identity.typ(b.Signature()))
}
func skeletonMethod(receiver types.Type, required *types.Func) *types.Func {
	methods := types.NewMethodSet(types.NewPointer(receiver))
	for i := 0; i < methods.Len(); i++ {
		fn := methods.At(i).Obj().(*types.Func)
		if fn.Name() != required.Name() {
			continue
		}
		if !fn.Exported() && fn.Pkg().Path() != required.Pkg().Path() {
			continue
		}
		return fn
	}
	return nil
}
func (p *skeletonPlanner) implementationEdits(plan lang.GenPlan, l loaded) []lang.Edit {
	native := interfaceRealizers(plan, l)
	byType := map[string][]*types.Func{}
	for _, edge := range plan.Graph.Edges() {
		if edge.Kind != graph.EdgeKindImplements || p.nodes[edge.From].Layer != graph.LayerDesign {
			continue
		}
		declaration, ok := native[edge.To]
		if !ok {
			continue
		}
		iface, ok := declaration.obj.Type().Underlying().(*types.Interface)
		if !ok {
			continue
		}
		iface.Complete()
		for i := 0; i < iface.NumMethods(); i++ {
			byType[edge.From] = append(byType[edge.From], iface.Method(i))
		}
	}
	// Parents precede children so a newly planned parent implementation also
	// satisfies its embedded child's requirements without a shadowing method.
	var depth func(string, map[string]bool) int
	depth = func(id string, seen map[string]bool) int {
		if seen[id] {
			return 0
		}
		seen[id] = true
		n := 0
		for _, edge := range plan.Graph.Edges() {
			if edge.Kind == graph.EdgeKindExtends && edge.From == id {
				n = max(n, 1+depth(edge.To, maps.Clone(seen)))
			}
		}
		return n
	}
	ids := slices.Sorted(maps.Keys(byType))
	slices.SortStableFunc(ids, func(a, b string) int { return depth(a, map[string]bool{}) - depth(b, map[string]bool{}) })
	planned := map[string]map[string]*types.Func{}
	var inherited func(string, *types.Func, map[string]bool) int
	inherited = func(id string, required *types.Func, seen map[string]bool) int {
		if seen[id] {
			return 0
		}
		seen[id] = true
		if fn := planned[id][required.Name()]; fn != nil {
			if skeletonSignaturesEqual(fn, required) {
				return 1
			}
			return 0
		}
		count := 0
		for _, edge := range plan.Graph.Edges() {
			if edge.Kind == graph.EdgeKindExtends && edge.From == id {
				branch := maps.Clone(seen)
				count += inherited(edge.To, required, branch)
			}
		}
		return count
	}
	var edits []lang.Edit
	for _, id := range ids {
		declaration, ok := native[id]
		if !ok {
			continue
		}
		named := namedOf(declaration.obj.Type())
		if named == nil {
			continue
		}
		methods := map[string]*types.Func{}
		for _, required := range byType[id] {
			if previous := methods[required.Name()]; previous != nil && !skeletonSignaturesEqual(previous, required) {
				p.fail(p.nodes[id], "the implemented interfaces require incompatible native method signatures", "reconcile the interfaces or declare separate implementations")
				continue
			}
			if provided := skeletonMethod(declaration.obj.Type(), required); provided != nil {
				if skeletonSignaturesEqual(provided, required) {
					continue
				}
				if receiver := namedOf(provided.Signature().Recv().Type()); receiver != nil && typeID(receiver.Obj()) == typeID(named.Obj()) {
					p.fail(p.nodes[id], "an existing method has an incompatible interface signature", "reconcile the handwritten method and interface without adding a duplicate method")
					continue
				}
			}
			if inherited(id, required, map[string]bool{}) == 1 {
				continue
			}
			if !required.Exported() && required.Pkg().Path() != declaration.pkg.Types.Path() {
				p.fail(p.nodes[id], "a foreign interface's private method cannot be implemented here", "implement the interface in its defining package or expose a public interface")
				continue
			}
			methods[required.Name()] = required
		}
		if len(methods) == 0 {
			continue
		}
		planned[id] = methods
		imports := map[string]string{}
		qualifier := func(pkg *types.Package) string {
			if pkg.Path() == declaration.pkg.Types.Path() {
				return ""
			}
			if alias := imports[pkg.Path()]; alias != "" {
				return alias
			}
			alias := "skeletonpkg" + strconv.Itoa(len(imports))
			imports[pkg.Path()] = alias
			return alias
		}
		var body strings.Builder
		receiver := declaration.obj.Name()
		if p.nodes[id].Form == "object" {
			receiver = "*" + receiver
		}
		for _, name := range slices.Sorted(maps.Keys(methods)) {
			signature := strings.TrimPrefix(types.TypeString(methods[name].Signature(), qualifier), "func")
			fmt.Fprintf(&body, "func (%s) %s%s { panic(%s) }\n", receiver, name, signature, strconv.Quote(unimplemented))
		}
		var source strings.Builder
		fmt.Fprintf(&source, "// Unicode skeleton. Implement the declarations in this file.\npackage %s\n", declaration.pkg.Name)
		if len(imports) > 0 {
			source.WriteString("import (\n")
			for _, key := range slices.Sorted(maps.Keys(imports)) {
				fmt.Fprintf(&source, "%s %s\n", imports[key], strconv.Quote(key))
			}
			source.WriteString(")\n")
		}
		source.WriteString(body.String())
		content, err := format.Source([]byte(source.String()))
		if err != nil {
			p.fail(p.nodes[id], err.Error(), "correct native interface declarations before generation")
			continue
		}
		filename := declaration.pkg.Fset.Position(declaration.obj.Pos()).Filename
		rel, err := filepath.Rel(l.root, filename)
		if err != nil || !filepath.IsLocal(rel) {
			p.fail(p.nodes[id], "the receiver source lies outside the repository", "realize the receiver inside its configured source roots")
			continue
		}
		test := strings.HasSuffix(rel, "_test.go")
		edits = append(edits, lang.Edit{Path: path.Join(path.Dir(filepath.ToSlash(rel)), skeletonFileName("implements:"+id, test)), Content: content, CreateOnly: true})
	}
	return edits
}
func (p *skeletonPlanner) checkNativeImplementations(plan lang.GenPlan, l loaded) {
	native := interfaceRealizers(plan, l)
	for _, edge := range plan.Graph.Edges() {
		if edge.Kind != graph.EdgeKindImplements || p.nodes[edge.From].Layer != graph.LayerDesign {
			continue
		}
		implementation, hasType := native[edge.From]
		declaration, hasInterface := native[edge.To]
		if !hasType || !hasInterface {
			continue
		}
		identity := nativeIdentity{packages: map[string]*types.Package{}, named: map[string][]nativeNamed{}}
		iface, ok := declaration.obj.Type().Underlying().(*types.Interface)
		valid := ok && iface.IsMethodSet()
		if valid {
			have := methodKeys(types.NewMethodSet(types.NewPointer(implementation.obj.Type())), &identity)
			want := methodKeys(types.NewMethodSet(declaration.obj.Type()), &identity)
			valid = implementsAll(have, want)
		}
		if !valid {
			p.fail(p.nodes[edge.From], "the planned receiver does not implement its native interface", "resolve incompatible or ambiguous embedded methods before generating source")
		}
	}
}
