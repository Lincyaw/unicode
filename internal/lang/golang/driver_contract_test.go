package golang

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/tools/go/packages"
	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
)

//uc:realizes lang.golang.DriverSample
type DriverSample struct {
	Name    string
	Symbols int
	Manual  bool
}

//uc:generates lang.golang.driver-modes-preserve-claims.sample
func genDriverSample() *rapid.Generator[DriverSample] {
	return rapid.Custom(func(t *rapid.T) DriverSample {
		parts := rapid.SliceOfN(rapid.SampledFrom([]string{"a", "bank", "same-name", "longer", "x9"}), 1, 4).Draw(t, "parts")
		return DriverSample{"driver." + strings.Join(parts, "."), rapid.IntRange(0, 3).Draw(t, "symbols"), rapid.Bool().Draw(t, "manual")}
	})
}

// Driver compilation imports are immutable and shared across randomized samples.
// Loading actual dependency types lets go/types check the full emitted runtime,
// including rapid's generic generators and its genuine *rapid.T signatures.
var driverDependencies struct {
	sync.Once
	packages map[string]*types.Package
	err      error
}

type driverImporter map[string]*types.Package

func (imports driverImporter) Import(path string) (*types.Package, error) {
	if pkg := imports[path]; pkg != nil {
		return pkg, nil
	}
	return nil, fmt.Errorf("no driver dependency type information for %s", path)
}

func driverImports(t rapid.TB) driverImporter {
	driverDependencies.Do(func() {
		file, err := parser.ParseFile(token.NewFileSet(), "runtime.go", contractRuntime, parser.ImportsOnly)
		if err != nil {
			driverDependencies.err = err
			return
		}
		paths := []string{"testing", "os"}
		for _, spec := range file.Imports {
			path, _ := strconv.Unquote(spec.Path.Value)
			paths = append(paths, path)
		}
		loaded, err := packages.Load(&packages.Config{Mode: packages.NeedName | packages.NeedTypes | packages.NeedImports | packages.NeedDeps}, paths...)
		if err != nil {
			driverDependencies.err = err
			return
		}
		driverDependencies.packages = map[string]*types.Package{}
		for _, pkg := range loaded {
			if len(pkg.Errors) > 0 {
				driverDependencies.err = pkg.Errors[0]
				return
			}
			driverDependencies.packages[pkg.PkgPath] = pkg.Types
		}
	})
	if driverDependencies.err != nil {
		t.Fatal(driverDependencies.err)
	}
	return driverDependencies.packages
}

func driverFixture(sample DriverSample) (*contractCompiler, *contractFile, string) {
	pkg := &packages.Package{PkgPath: "driverfixture", Name: "driverfixture", Types: types.NewPackage("driverfixture", "driverfixture")}
	unit := &contractUnit{node: graph.Node{Id: sample.Name, Origin: diag.Position{Path: "design/driver.uc", Line: 1, Column: 1}}, pkg: pkg, generators: map[string]*types.Func{}, generatorArgs: map[string][]string{}}
	unit.spec = ucContract{ID: sample.Name, Path: "design/driver.uc", Line: 1, Column: 1, Types: map[string]ucType{}, Operations: map[string]ucOperation{}, Ensures: []*ucExpr{{Kind: "bool", Text: "true"}}}
	var source strings.Builder
	source.WriteString("package driverfixture\nimport \"pgregory.net/rapid\"\n")
	var params, prior []string
	names := []string{"t", "s", "values", "reflect", "ucContext", "ucInputs0", "ucInvoke0", "unicodeGeneratedPredicate0"}
	if len(sample.Name)%2 == 1 {
		slices.Reverse(names)
	}
	names = append(names[len(sample.Name)%3:], names[:len(sample.Name)%3]...)
	for i := range sample.Symbols {
		name := names[i]
		unit.spec.Symbols = append(unit.spec.Symbols, ucSymbol{Name: name, Type: "int"})
		params = append(params, fmt.Sprintf("value%d int", i))
		generator := []string{"ucInputs0", "ucInvoke0", "ucRandom0"}[i]
		fn := types.NewFunc(token.NoPos, pkg.Types, generator, types.NewSignatureType(nil, nil, nil, nil, nil, false))
		pkg.Types.Scope().Insert(fn)
		unit.generators[name] = fn
		for j := range i {
			unit.generatorArgs[name] = append(unit.generatorArgs[name], names[j])
		}
		value := strconv.Itoa(i)
		if i > 0 {
			value = strings.Join(prior, "+") + "+1"
		}
		fmt.Fprintf(&source, "func %s(%s) *rapid.Generator[int] {return rapid.Just(%s)}\n", generator, strings.Join(params[:i], ","), value)
		prior = append(prior, fmt.Sprintf("value%d", i))
	}
	if sample.Manual {
		unit.predicate = types.NewFunc(token.NoPos, pkg.Types, "ucCheck", types.NewSignatureType(nil, nil, nil, nil, nil, false))
		pkg.Types.Scope().Insert(unit.predicate)
		source.WriteString("func ucCheck(t rapid.TB")
		if len(params) > 0 {
			source.WriteString("," + strings.Join(params, ","))
		}
		source.WriteString(") {}\n")
	} else if sample.Symbols == 0 {
		// Keep rapid imported for the same companion source in both variants.
		source.WriteString("var _ rapid.TB\n")
	}
	for i := range 1 + len(sample.Name)%3 {
		var values []string
		for j := range sample.Symbols {
			values = append(values, strconv.Itoa(i+j))
		}
		unit.cases = append(unit.cases, values)
	}
	c := &contractCompiler{foreignNames: map[string]string{}, foreignOriginal: map[string]string{}, bindings: map[graph.EdgeKind]map[string][]string{}}
	return c, &contractFile{pkg: pkg, units: []*contractUnit{unit}, imports: map[string]string{}}, source.String()
}

func driverCalls(fn *ast.FuncDecl, name string) []*ast.CallExpr {
	var calls []*ast.CallExpr
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		callee := ""
		switch x := call.Fun.(type) {
		case *ast.Ident:
			callee = x.Name
		case *ast.SelectorExpr:
			if pkg, ok := x.X.(*ast.Ident); ok {
				callee = pkg.Name + "." + x.Sel.Name
			}
		}
		if callee == name {
			calls = append(calls, call)
		}
		return true
	})
	return calls
}

func driverText(set *token.FileSet, source []byte, node ast.Node) string {
	return string(source[set.Position(node.Pos()).Offset:set.Position(node.End()).Offset])
}

//uc:checks lang.golang.driver-modes-preserve-claims
func checkDriverModesPreserveClaims(t rapid.TB, sample DriverSample) {
	c, fixture, source := driverFixture(sample)
	generated := c.render(fixture)
	if generated == nil || len(c.diagnostics) > 0 {
		t.Fatalf("driver generation failed for %+v: %v", sample, c.diagnostics)
	}
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, "unicode_contracts_gen_test.go", generated, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	handwritten, err := parser.ParseFile(set, "predicate_test.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	conf := types.Config{Importer: driverImports(t)}
	if _, err := conf.Check("driverfixture", set, []*ast.File{file, handwritten}, nil); err != nil {
		t.Fatalf("generated drivers do not compile for %+v: %v", sample, err)
	}
	functions := map[string]*ast.FuncDecl{}
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil {
			functions[fn.Name.Name] = fn
		}
	}
	fuzzName := fmt.Sprintf("FuzzUnicode_%x", sha256.Sum256([]byte(sample.Name)))
	fuzz := functions[fuzzName]
	if fuzz == nil || fuzz.Doc == nil || !strings.Contains(fuzz.Doc.Text(), sample.Name) || len(driverCalls(fuzz, "rapid.MakeFuzz")) != 1 {
		t.Fatalf("native fuzz entry is missing, unstable, or lacks its design identity: %s", fuzzName)
	}
	drive := functions["unicodeGeneratedDrive0"]
	random := functions["unicodeGeneratedRandom0"]
	inputs := functions["unicodeGeneratedInputs0"]
	invoke := functions["unicodeGeneratedInvoke0"]
	if drive == nil || random == nil || inputs == nil || invoke == nil {
		t.Fatal("the shared driver helpers are incomplete")
	}
	if len(driverCalls(fuzz, random.Name.Name)) != 1 || len(driverCalls(drive, random.Name.Name)) != 1 || len(driverCalls(random, inputs.Name.Name)) != 1 || len(driverCalls(random, invoke.Name.Name)) != 1 {
		t.Fatal("native fuzz and the ordinary driver do not share exactly one input/predicate path")
	}
	if len(driverCalls(fuzz, "unicodeGeneratedFinish")) != 0 || len(driverCalls(fuzz, "unicodeGeneratedContext")) != 1 {
		t.Fatal("native fuzz applies a batch time assertion or leaks its report context")
	}
	// Each literal case is an unconditional top-level invocation before the
	// random phase. No case can reuse a map constructed by another invocation.
	var replay int
	randomPhase := false
	for _, statement := range drive.Body.List {
		if expression, ok := statement.(*ast.ExprStmt); ok {
			if call, ok := expression.X.(*ast.CallExpr); ok {
				if name, ok := call.Fun.(*ast.Ident); ok && name.Name == invoke.Name.Name {
					if randomPhase {
						t.Fatal("a declared replay follows the random phase")
					}
					if _, ok := call.Args[2].(*ast.CompositeLit); !ok {
						t.Fatal("a replay shares a previously allocated input map")
					}
					replay++
				}
			}
		}
		if branch, ok := statement.(*ast.IfStmt); ok {
			randomPhase = true
			condition := driverText(set, generated, branch.Cond)
			if condition != "mode != \"replay\"" || len(driverCalls(&ast.FuncDecl{Body: branch.Body}, "rapid.Check")) != 1 {
				t.Fatalf("the random phase does not distinguish replay from batch checking: %s", condition)
			}
		}
	}
	if replay != len(fixture.units[0].cases) || !randomPhase {
		t.Fatalf("literal replay count or random phase changed: got %d cases", replay)
	}
	var guards int
	ast.Inspect(drive.Body, func(node ast.Node) bool {
		if branch, ok := node.(*ast.IfStmt); ok && strings.Contains(driverText(set, generated, branch.Cond), "time.Since") {
			guards++
			if driverText(set, generated, branch.Cond) != "mode == \"explore\" && time.Since(start) > budget" {
				t.Fatal("ordinary property checks can be truncated by wall clock time")
			}
		}
		return true
	})
	if guards != 1 {
		t.Fatal("the explicit exploration budget guard is missing or duplicated")
	}
	var drawn []string
	for _, statement := range inputs.Body.List {
		assignment, ok := statement.(*ast.AssignStmt)
		if !ok || len(assignment.Lhs) != 1 {
			continue
		}
		index, ok := assignment.Lhs[0].(*ast.IndexExpr)
		if !ok {
			continue
		}
		literal, ok := index.Index.(*ast.BasicLit)
		if !ok {
			t.Fatal("a symbol input is not indexed by its model name")
		}
		name, _ := strconv.Unquote(literal.Value)
		drawn = append(drawn, name)
	}
	var expected []string
	for _, symbol := range fixture.units[0].spec.Symbols {
		expected = append(expected, symbol.Name)
	}
	if !slices.Equal(drawn, expected) {
		t.Fatalf("input generation changed symbol order: %v != %v", drawn, expected)
	}
	if !sample.Manual {
		predicate := functions[(&Backend{}).MapName("predicate", lastName(sample.Name))]
		if predicate == nil {
			t.Fatal("the automatic checks predicate is missing")
		}
		var parameters []string
		for _, parameter := range predicate.Type.Params.List[1:] {
			for _, name := range parameter.Names {
				parameters = append(parameters, name.Name)
			}
		}
		if !slices.Equal(parameters, expected) {
			t.Fatalf("the automatic predicate changed forall names or order: %v != %v", parameters, expected)
		}
	}
	// Moving a unit within the output must retain its native corpus name.
	companion := &contractUnit{node: graph.Node{Id: "companion.stable"}, spec: ucContract{ID: "companion.stable", Ensures: []*ucExpr{{Kind: "bool", Text: "true"}}}}
	reordered := *fixture
	reordered.units = []*contractUnit{companion, fixture.units[0]}
	if other := c.render(&reordered); other == nil || !strings.Contains(string(other), "func "+fuzzName+"(") {
		t.Fatal("reordering declarations changed or removed the native corpus entry")
	}
	// Distinct full IDs can map to the same Go predicate name. Reject that
	// generated/generated collision as well as a handwritten declaration.
	first, second := *fixture.units[0], *fixture.units[0]
	first.predicate, second.predicate = nil, nil
	second.node.Id = "another." + lastName(sample.Name)
	second.spec.ID = second.node.Id
	colliding := *fixture
	colliding.units = []*contractUnit{&first, &second}
	c.diagnostics = nil
	if c.render(&colliding) != nil || len(c.diagnostics) != 1 || c.diagnostics[0].Code != "lang.golang.driver-name-collision" {
		t.Fatalf("colliding generated predicates did not stop generation: %v", c.diagnostics)
	}
	// Collision diagnostics must happen before any source is returned, and use
	// a source position, cause, and fix just like the compiler's other errors.
	fixture.pkg.Types.Scope().Insert(types.NewVar(token.NoPos, fixture.pkg.Types, fuzzName, types.Typ[types.Int]))
	c.diagnostics = nil
	if c.render(fixture) != nil || len(c.diagnostics) != 1 || c.diagnostics[0].Code != "lang.golang.driver-name-collision" || c.diagnostics[0].Position.Path == "" || c.diagnostics[0].Message == "" || c.diagnostics[0].Fix == "" {
		t.Fatalf("a handwritten/native-fuzz collision did not stop generation: %v", c.diagnostics)
	}
	checkDriverContext(t, sample)
}

type driverStop struct{}
type driverTB struct {
	rapid.TB
	failed bool
}

func (t *driverTB) Failed() bool          { return t.failed }
func (t *driverTB) Errorf(string, ...any) { t.failed = true }
func (t *driverTB) Fatalf(string, ...any) { t.failed = true; panic(driverStop{}) }
func (t *driverTB) SkipNow()              { panic(driverStop{}) }

func checkDriverContext(t rapid.TB, sample DriverSample) {
	previous, exploring := ucCurrent, ucExploring
	defer func() { ucCurrent, ucExploring = previous, exploring }()
	sentinel := &ucRunResult{Id: "outer", Accepted: len(sample.Name)}
	ucCurrent, ucExploring = sentinel, false
	for _, outcome := range []string{"pass", "skip", "fatal", "panic"} {
		result := &ucRunResult{Id: sample.Name, Passed: true}
		tb := &driverTB{}
		x := &ucExpr{Kind: "name", Text: "x"}
		spec := &ucContract{ID: sample.Name, Path: "design/driver.uc", Line: 1, Column: 1, Symbols: []ucSymbol{{"x", "int"}}, Types: map[string]ucType{}}
		if outcome == "skip" {
			spec.Requires = []*ucExpr{{Kind: "binary", Text: "<", Children: []*ucExpr{x, x}}}
		} else if outcome == "fatal" {
			spec.Ensures = []*ucExpr{{Kind: "binary", Text: "<", Children: []*ucExpr{x, x}}}
		}
		func() {
			defer func() {
				if p := recover(); p != nil {
					if _, ok := p.(driverStop); !ok {
						panic(p)
					}
				}
			}()
			restore := ucContext(result, true)
			defer restore()
			defer func() { ucFailedPredicate(tb, spec, result); ucFinish(tb, spec, result) }()
			if outcome == "panic" {
				panic(driverStop{})
			}
			ucCheck(tb, spec, map[string]reflect.Value{"x": reflect.ValueOf(sample.Symbols)})
		}()
		if ucCurrent != sentinel || ucExploring || sentinel.Accepted != len(sample.Name) {
			t.Fatalf("the %s invocation leaked report state into its caller", outcome)
		}
		if outcome == "skip" || outcome == "panic" {
			codes := map[string]bool{}
			for _, diagnostic := range result.Diagnostics {
				codes[diagnostic.Code] = true
			}
			if !codes["lang.golang.no-effective-input"] || result.Passed || !result.Infrastructure || result.Accepted != 0 {
				t.Fatalf("an aborted invocation hid its zero-effective-input error: %+v", result)
			}
		} else if result.Accepted != 1 || result.Passed != (outcome == "pass") {
			t.Fatalf("the %s result leaked another invocation's counters or failure: %+v", outcome, result)
		}
	}
}
