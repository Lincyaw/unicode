package golang

import (
	"fmt"
	"go/token"
	"go/types"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
	"github.com/lincyaw/unicode/internal/syntax"
)

// Formal expressions execute before or after the tested operation. Their
// external dependencies need scripts even when the predicate itself is manual.
func checkDoubleExpressionDependencies(t rapid.TB, sample DoubleSample) {
	g := graph.NewGraph()
	nodes := map[string]graph.Node{}
	add := func(id string, kind graph.NodeKind, form string, layer graph.Layer) {
		path := "design/domain.uc"
		if layer == graph.LayerCode {
			path = "domain/domain.go"
		}
		if id == "go:sample/domain.Install" {
			path = "domain/install.go"
		}
		n := graph.Node{Id: id, Kind: kind, Form: form, Layer: layer, Origin: diag.Position{Path: path, Line: 1, Column: 1}}
		if err := g.AddNode(n); err != nil {
			t.Fatal(err)
		}
		nodes[id] = n
	}
	edge := func(kind graph.EdgeKind, from, to string) {
		if err := g.AddEdge(graph.Edge{Kind: kind, From: from, To: to, Source: graph.SourceDeclared}); err != nil {
			t.Fatal(err)
		}
	}
	add("remote", graph.NodeKindActor, "external", graph.LayerDesign)
	add("domain", graph.NodeKindContainer, "component", graph.LayerDesign)
	add("other", graph.NodeKindContainer, "component", graph.LayerDesign)
	add("file:domain/domain.go", graph.NodeKindContainer, "file", graph.LayerCode)
	add("file:domain/install.go", graph.NodeKindContainer, "file", graph.LayerCode)
	edge(graph.EdgeKindOwns, "domain", "file:domain/domain.go")
	edge(graph.EdgeKindOwns, "domain", "file:domain/install.go")
	edge(graph.EdgeKindUses, "domain", "remote")
	edge(graph.EdgeKindUses, "other", "remote")
	add("remote.calc", graph.NodeKindOp, "op", graph.LayerDesign)
	add("domain.read", graph.NodeKindOp, "op", graph.LayerDesign)
	add("domain.check", graph.NodeKindProperty, "invariant", graph.LayerDesign)
	add("go:sample/domain.Calc", graph.NodeKindOp, "func", graph.LayerCode)
	add("go:sample/domain.Install", graph.NodeKindOp, "func", graph.LayerCode)
	edge(graph.EdgeKindRealizes, "go:sample/domain.Calc", "remote.calc")
	add("go:sample/domain.Read", graph.NodeKindOp, "func", graph.LayerCode)
	edge(graph.EdgeKindRealizes, "go:sample/domain.Read", "domain.read")
	edge(graph.EdgeKindRefers, "domain.check", "domain.read")
	previous := "go:sample/domain.Read"
	for i := 0; i < max(1, sample.Count); i++ {
		next := fmt.Sprintf("go:sample/domain.Helper%d", i)
		add(next, graph.NodeKindOp, "func", graph.LayerCode)
		edge(graph.EdgeKindCalls, previous, next)
		previous = next
	}
	edge(graph.EdgeKindCalls, previous, "go:sample/domain.Calc")
	pkg := types.NewPackage("sample/domain", "domain")
	param := types.NewTuple(types.NewVar(token.NoPos, pkg, "x", types.Typ[types.Int]))
	sig := types.NewSignatureType(nil, nil, nil, param, param, false)
	cleanup := types.NewSignatureType(nil, nil, nil, nil, nil, false)
	installSig := types.NewSignatureType(nil, nil, nil, types.NewTuple(types.NewVar(token.NoPos, pkg, "replacement", sig)), types.NewTuple(types.NewVar(token.NoPos, pkg, "cleanup", cleanup)), false)
	unit := &contractUnit{node: nodes["domain.check"], pkg: &packages.Package{PkgPath: pkg.Path(), Types: pkg}, spec: ucContract{Types: map[string]ucType{}, Symbols: []ucSymbol{{Name: "x", Type: "int"}}, Operations: map[string]ucOperation{"remote.calc": {Name: "remote.calc", Owner: "remote", Result: "int", Params: []string{"int"}, Pure: true}}}}
	if sample.Fail {
		unit.predicate = types.NewFunc(token.NoPos, pkg, "manual", cleanup)
	}
	c := contractCompiler{plan: lang.GenPlan{Graph: &g}, nodes: nodes, parents: map[string]string{"remote.calc": "remote"}, bindings: map[graph.EdgeKind]map[string][]string{graph.EdgeKindRealizes: {"remote.calc": {"go:sample/domain.Calc"}}, graph.EdgeKindSubstitutes: {"remote.calc": {"go:sample/domain.Install"}}}, declarations: map[string]nativeDeclaration{"go:sample/domain.Calc": {obj: types.NewFunc(token.NoPos, pkg, "Calc", sig)}, "go:sample/domain.Install": {obj: types.NewFunc(token.NoPos, pkg, "Install", installSig)}}}
	if c.configureDoubles(unit) {
		t.Fatal("an expression dependency reached an external client without a declared response script")
	}
	found := false
	for _, d := range c.diagnostics {
		found = found || d.Code == "lang.golang.missing-substitute"
	}
	if !found {
		t.Fatalf("missing dependency response has no diagnostic: %+v", c.diagnostics)
	}
	c.diagnostics = nil
	unit.spec.Symbols = append(unit.spec.Symbols, ucSymbol{Name: "answers", Type: "replies<remote.calc>"})
	if !c.configureDoubles(unit) {
		t.Fatalf("a complete expression dependency and installer were rejected: %+v", c.diagnostics)
	}
	otherGraph := graph.NewGraph()
	for _, n := range g.Nodes() {
		if err := otherGraph.AddNode(n); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range g.Edges() {
		if e.Kind == graph.EdgeKindOwns && e.To == "file:domain/install.go" {
			e.From = "other"
		}
		if err := otherGraph.AddEdge(e); err != nil {
			t.Fatal(err)
		}
	}
	c.plan.Graph = &otherGraph
	c.diagnostics = nil
	if c.configureDoubles(unit) {
		t.Fatal("an installer owned by a different permitted component was accepted for generation")
	}
}

// Go permits equal basenames in different import paths. Literal resolution
// follows the emitted alias and actual object, irrespective of map order.
func checkDoubleLiteralIdentities(t rapid.TB, count int) {
	pkg := types.NewPackage("sample/domain", "domain")
	current := &packages.Package{PkgPath: pkg.Path(), Types: pkg}
	g := graph.NewGraph()
	c := contractCompiler{plan: lang.GenPlan{Graph: &g}, nodes: map[string]graph.Node{}, bindings: map[graph.EdgeKind]map[string][]string{graph.EdgeKindRealizes: {}}, declarations: map[string]nativeDeclaration{}, literalImports: map[string]string{}, foreignOriginal: map[string]string{}}
	for i, side := range []string{"left", "right"} {
		nativePkg := types.NewPackage("sample/"+side, side)
		native := &packages.Package{PkgPath: nativePkg.Path(), Types: nativePkg}
		alias := "ucpkg" + strconv.Itoa(i)
		c.literalImports[nativePkg.Path()] = alias
		object := types.NewTypeName(token.NoPos, nativePkg, "Gateway", nil)
		types.NewNamed(object, types.NewInterfaceType(nil, nil).Complete(), nil)
		nativePkg.Scope().Insert(object)
		model := side + ".Gateway"
		id := typeID(object)
		c.nodes[model] = graph.Node{Id: model, Kind: graph.NodeKindType, Form: "interface", Layer: graph.LayerDesign}
		c.bindings[graph.EdgeKindRealizes][model] = []string{id}
		c.declarations[id] = nativeDeclaration{obj: object, pkg: native}
		gatewayAlias := types.NewTypeName(token.NoPos, nativePkg, "GatewayAlias", nil)
		types.NewAlias(gatewayAlias, object.Type())
		nativePkg.Scope().Insert(gatewayAlias)
		c.declarations[typeID(gatewayAlias)] = nativeDeclaration{obj: gatewayAlias, pkg: native}
		field := types.NewVar(token.NoPos, nativePkg, "Value"+strconv.Itoa(i), types.Typ[types.Int])
		record := types.NewTypeName(token.NoPos, nativePkg, "Payload", nil)
		types.NewNamed(record, types.NewStruct([]*types.Var{field}, nil), nil)
		nativePkg.Scope().Insert(record)
		c.declarations[typeID(record)] = nativeDeclaration{obj: record, pkg: native}
		recordAlias := types.NewTypeName(token.NoPos, nativePkg, "PayloadAlias", nil)
		types.NewAlias(recordAlias, record.Type())
		nativePkg.Scope().Insert(recordAlias)
		c.declarations[typeID(recordAlias)] = nativeDeclaration{obj: recordAlias, pkg: native}
	}
	for i, side := range []string{"left", "right"} {
		alias := "ucpkg" + strconv.Itoa(i)
		text, ok, err := c.doubleLiteral(&syntax.BraceValue{}, alias+".Gateway", current)
		if !ok || err != nil || !strings.Contains(text, strconv.Quote(side+".Gateway")) {
			t.Fatalf("qualified interface %s resolved incorrectly: %s %v", side, text, err)
		}
		text, ok, err = c.doubleLiteral(&syntax.BraceValue{}, alias+".GatewayAlias", current)
		if !ok || err != nil || !strings.Contains(text, strconv.Quote(side+".Gateway")) {
			t.Fatalf("qualified interface alias %s resolved incorrectly: %s %v", side, text, err)
		}
		key := syntax.Lower("value" + strconv.Itoa(i))
		value := &syntax.BraceValue{Entries: []syntax.Entry{{Key: syntax.Key{Name: &key}, Value: &syntax.NumberValue{Text: syntax.Number(strconv.Itoa(count))}}}}
		text, err = c.valueLiteral(value, alias+".Payload", current)
		if err != nil || !strings.Contains(text, alias+".Payload") || !strings.Contains(text, "Value"+strconv.Itoa(i)) {
			t.Fatalf("qualified record %s resolved incorrectly: %s %v", side, text, err)
		}
		text, err = c.valueLiteral(value, alias+".PayloadAlias", current)
		if err != nil || !strings.Contains(text, alias+".PayloadAlias") || !strings.Contains(text, "Value"+strconv.Itoa(i)) {
			t.Fatalf("qualified record alias %s resolved incorrectly: %s %v", side, text, err)
		}
	}
}
