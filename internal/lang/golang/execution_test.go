package golang

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"reflect"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/lang"
	"github.com/lincyaw/unicode/internal/syntax"
)

//uc:realizes lang.golang.ExecutionSample
type ExecutionSample struct {
	Initial int
	Deltas  []int
	FailAt  int
	Impure  bool
}

//uc:generates lang.golang.executes-sequences-and-flows.sample
func genExecutionSample() *rapid.Generator[ExecutionSample] {
	return rapid.Custom(func(t *rapid.T) ExecutionSample {
		return ExecutionSample{rapid.IntRange(-20, 20).Draw(t, "initial"), rapid.SliceOfN(rapid.IntRange(-5, 5), 0, 8).Draw(t, "deltas"), rapid.IntRange(-1, 8).Draw(t, "failAt"), rapid.Bool().Draw(t, "impure")}
	})
}

type executionCounter struct{ Value, Calls, FailAt int }
type executionFailure struct{ Attempt int }

// executionReturnedItem keeps implementation data outside its modeled fields.
type ExecutionParent struct{ Value int }
type executionDerived struct {
	*ExecutionParent
	Delta int
}

type executionReturnedItem struct {
	Value int
	fee   int
}

func (item *executionReturnedItem) Total() int { return item.Value + item.fee }

func (f executionFailure) Error() string { return "attempt " + strconv.Itoa(f.Attempt) }
func (c *executionCounter) Apply(delta int) error {
	attempt := c.Calls
	c.Calls++
	if attempt == c.FailAt {
		return executionFailure{attempt}
	}
	c.Value += delta
	return nil
}

type executionCall struct {
	Operation string
	Arguments map[string]any
}

func executionSpec(t rapid.TB) *ucContract {
	return &ucContract{ID: "execution.sequence", Path: "design/execution.uc", Line: 1, Column: 1, Symbols: []ucSymbol{{"counter", "Counter"}, {"steps", "list<call<Counter>>"}}, Sequence: "steps", SequenceReceiver: "counter", Types: map[string]ucType{"Counter": {Form: "object", Fields: []ucField{{"value", "Value", "int"}}}, "Failure": {Form: "error", Fields: []ucField{{"attempt", "Attempt", "int"}}}}, Operations: map[string]ucOperation{"Counter.apply": {Name: "Counter.apply", Owner: "Counter", GoName: "Apply", Params: []string{"int"}, ParamNames: []string{"delta"}, Raises: []string{"Failure"}}}, ErrorTypes: map[string]reflect.Type{"Failure": reflect.TypeFor[executionFailure]()}, Ensures: []*ucExpr{runtimeExpr(t, "counter.value >= 0"), runtimeExpr(t, "counter.value >= old(counter.value) and counter.value - old(counter.value) <= 5")}}
}

//uc:checks lang.golang.executes-sequences-and-flows
func checkExecutesSequencesAndFlows(t rapid.TB, sample ExecutionSample) {
	// Built-in named types have no package and cannot be generated carriers.
	extractor := &extraction{}
	builtinError := types.Universe.Lookup("error").Type().(*types.Named)
	if extractor.carrierModel(builtinError) != "" || extractor.namedType(builtinError) != "error" {
		t.Fatal("the built-in error type was mistaken for a generated carrier")
	}
	// Manual flows own their execution boundary, including asynchronous first
	// steps; the same timed automatic flow still requires a synchronous entry.
	arrow := "app ->> app.work()"
	if sample.Impure {
		arrow = "app emits Ping()"
	}
	flowSource := fmt.Sprintf("component execution \"Execution flow fixture\" {\n  paths \"src/**\"\n}\nflow entry \"Manual boundary\" {\n  %s\n  within \"1s\"\n  manual \"The predicate prepares and times the asynchronous environment\"\n}\n", arrow)
	parsed := syntax.Parse("design/execution.uc", syntax.FileKindComponent, []byte(flowSource))
	if len(parsed.Diagnostics) > 0 {
		t.Fatalf("a timed manual flow was rejected as an automatic entry: %v", parsed.Diagnostics)
	}
	automatic := strings.Replace(flowSource, "  manual \"The predicate prepares and times the asynchronous environment\"\n", "", 1)
	checked := syntax.Parse("design/execution.uc", syntax.FileKindComponent, []byte(automatic))
	if len(checked.Diagnostics) != 1 || checked.Diagnostics[0].Code != "syntax.invalid-flow-call" {
		t.Fatalf("an automatic asynchronous entry escaped G17: %v", checked.Diagnostics)
	}

	initial := sample.Initial
	if initial < 0 {
		initial = -initial
	}
	spec := executionSpec(t)
	operation := spec.Operations["Counter.apply"]
	operation.ParamTypes = []reflect.Type{reflect.TypeFor[int]()}
	spec.Types["call<Counter>"] = ucType{Form: "call", Calls: []ucOperation{operation}}
	var calls []executionCall
	expected := initial
	for i, delta := range sample.Deltas {
		if delta < 0 {
			delta = -delta
		}
		calls = append(calls, executionCall{"Counter.apply", map[string]any{"delta": delta}})
		if i != sample.FailAt {
			expected += delta
		}
	}
	counter := &executionCounter{Value: initial, FailAt: sample.FailAt}
	inputs := map[string]reflect.Value{"counter": reflect.ValueOf(counter), "steps": reflect.ValueOf(calls)}
	result := ucExecute(spec, inputs, false)
	if !result.Executed || result.Infrastructure || len(result.Diagnostics) > 0 || counter.Calls != len(calls) || counter.Value != expected {
		t.Fatalf("sequence did not preserve its independent prefix oracle: sample=%+v counter=%+v outcome=%+v", sample, counter, result)
	}
	// An invalid initial state fails even when there are no steps, before calls.
	counter.Value = -1
	counter.Calls = 0
	bad := ucExecute(spec, inputs, false)
	if !bad.Executed || bad.Infrastructure || len(bad.Diagnostics) == 0 || counter.Calls != 0 {
		t.Fatalf("initial invariant was not checked before the sequence: %+v", bad)
	}
	// The operation script sampler must use model parameter limits, not Go zeroes.
	if rt, ok := t.(*rapid.T); ok {
		drawn := ucDraw(rt, reflect.TypeFor[executionCall](), "call<Counter>", spec.Types, []string{"steps.apply.delta in 1..3"}, "steps", 0)
		name, args := ucCallParts(drawn)
		if name != "Counter.apply" || args["delta"].Int() < 1 || args["delta"].Int() > 3 {
			t.Fatalf("generated call escaped its operation domain: %s %v", name, args)
		}
	}
	checkExecutionPurity(t, sample, initial)
	checkExecutionPromotedLiteral(t, sample)
	if rt, ok := t.(*rapid.T); ok {
		hierarchy := map[string]ucType{"Parent": {Form: "record", Fields: []ucField{{"value", "Value", "int"}}}, "Child": {Form: "record", Parent: "Parent", Fields: []ucField{{"delta", "Delta", "int"}}}}
		generated := ucDraw(rt, reflect.TypeFor[executionDerived](), "Child", hierarchy, []string{"item.value in 1..3", "item.delta in -5..5"}, "item", 0).Interface().(executionDerived)
		if generated.ExecutionParent == nil || generated.Value < 1 || generated.Value > 3 || generated.Delta < -5 || generated.Delta > 5 {
			t.Fatalf("inherited generated fields escaped their independent domain: %+v", generated)
		}
		before := generated.Value
		snapshot := ucDetach(ucNormalize(reflect.ValueOf(generated), "Child", hierarchy))
		generated.Value++
		if ucKey(ucFieldValue(snapshot, "value")) != "i:"+strconv.Itoa(before) {
			t.Fatal("a generated inherited field was not readable or frozen in the model snapshot")
		}
	}

	// Automatic flows share the tested-call runtime and retain their flow name.
	compiler, file, _ := driverFixture(DriverSample{Name: "execution.entry", Symbols: 0})
	file.units[0].node.Kind = graph.NodeKindFlow
	source := string(compiler.render(file))
	mapped := (&Backend{}).MapName(lang.NameRoleFlowPredicate, "entry")
	if len(compiler.diagnostics) > 0 || !strings.Contains(source, "func "+mapped+"(") {
		t.Fatalf("an automatic flow did not generate its mapped predicate: %v", compiler.diagnostics)
	}
	// Carrier metadata preserves exact model identity and is retained independently
	// of obsolete generated function bodies.
	model := "call<execution.Counter.apply>"
	carrier := "// Code generated by unicode. DO NOT EDIT.\npackage execution\n" + modelCarrierDeclaration(model)
	set := token.NewFileSet()
	tree, err := parser.ParseFile(set, "carrier.go", carrier, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	kept := string(retainGeneratedCarriers(tree, []byte(carrier), set))
	if !strings.Contains(kept, modelCarrierName(model)) || !strings.Contains(kept, "//uc:model-type "+model) {
		t.Fatalf("generation overlay lost a typed carrier: %s", kept)
	}
	for _, decl := range tree.Decls {
		if group, ok := decl.(*ast.GenDecl); ok {
			for _, item := range group.Specs {
				if typ, ok := item.(*ast.TypeSpec); ok {
					if carrierDeclarationModel(tree, typeDoc(group, typ), typ) != model {
						t.Fatalf("carrier model did not survive native extraction")
					}
				}
			}
		}
	}
	tree.Comments = nil
	if len(retainGeneratedCarriers(tree, []byte(carrier), set)) != 0 {
		t.Fatal("an unowned carrier was retained")
	}
	if carrierModelText("call<") != "" || carrierModelText("call<bad name>") != "" {
		t.Fatal("malformed carrier metadata was accepted")
	}
}
func checkExecutionPurity(t rapid.TB, sample ExecutionSample, initial int) {
	// Normal call results keep their actual native receiver so pure methods can
	// use private implementation data while modeled fields remain frozen values.
	fee := len(sample.Deltas) + 1
	returnedCalls := 0
	returnedSpec := &ucContract{ID: "execution.returned-value", Path: "design/execution.uc", Types: map[string]ucType{"Item": {Form: "record", Fields: []ucField{{"value", "Value", "int"}}}}, Operations: map[string]ucOperation{}, Call: runtimeExpr(t, "makeItem()"), Ensures: []*ucExpr{runtimeExpr(t, "result.value == "+strconv.Itoa(initial)), runtimeExpr(t, "result.total() == "+strconv.Itoa(initial+fee))}}
	returnedSpec.Operations["makeItem"] = ucOperation{Name: "makeItem", Result: "Item", Function: reflect.ValueOf(func() *executionReturnedItem {
		returnedCalls++
		return &executionReturnedItem{Value: initial, fee: fee}
	})}
	returnedSpec.Operations["Item.total"] = ucOperation{Name: "Item.total", Owner: "Item", GoName: "Total", Result: "int", Pure: true}
	returned := ucExecute(returnedSpec, nil, false)
	if returnedCalls != 1 || returned.Infrastructure || len(returned.Diagnostics) > 0 {
		t.Fatalf("a normal returned value lost its real receiver: calls=%d outcome=%+v", returnedCalls, returned)
	}
	counter := &executionCounter{Value: initial}
	calls := 0
	spec := &ucContract{ID: "execution.pure", Path: "design/execution.uc", Line: 1, Column: 1, Symbols: []ucSymbol{{"counter", "Counter"}}, Types: map[string]ucType{"Counter": {Form: "object", Fields: []ucField{{"value", "Value", "int"}}}}, Operations: map[string]ucOperation{}, Call: runtimeExpr(t, "touch(counter)"), Ensures: []*ucExpr{runtimeExpr(t, "true")}}
	spec.Operations["touch"] = ucOperation{Name: "touch", Pure: true, Params: []string{"Counter"}, Function: reflect.ValueOf(func(value *executionCounter) {
		calls++
		if sample.Impure {
			value.Value++
		}
	})}
	result := ucExecute(spec, map[string]reflect.Value{"counter": reflect.ValueOf(counter)}, false)
	if calls != 2 || result.Infrastructure || ((len(result.Diagnostics) > 0) != sample.Impure) {
		t.Fatalf("void pure operation was not repeated and compared: %+v calls=%d", result, calls)
	}
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code != "lang.golang.purity-violation" || diagnostic.Subject != "touch" {
			t.Fatalf("purity violation lost its operation identity: %+v", diagnostic)
		}
	}
	// A repeated call may mutate a shared returned pointer; ensures must still see
	// the frozen first value, including on a declared-error path.
	receipt := &contractReceipt{Amount: initial}
	calls = 0
	valueSpec := &ucContract{ID: "execution.alias", Path: "design/execution.uc", Types: map[string]ucType{"Receipt": {Form: "record", Fields: []ucField{{"amount", "Amount", "int"}}}}, Operations: map[string]ucOperation{}, Call: runtimeExpr(t, "read()"), Ensures: []*ucExpr{runtimeExpr(t, "result.amount == "+strconv.Itoa(initial))}}
	valueSpec.Operations["read"] = ucOperation{Name: "read", Pure: true, Result: "Receipt", Function: reflect.ValueOf(func() *contractReceipt {
		if calls > 0 {
			receipt.Amount++
		}
		calls++
		return receipt
	})}
	aliased := ucExecute(valueSpec, nil, false)
	if aliased.Infrastructure || len(aliased.Diagnostics) != 1 || aliased.Diagnostics[0].Code != "lang.golang.purity-violation" {
		t.Fatalf("first aliased result was not frozen: %+v", aliased)
	}
	failure := &executionFailure{Attempt: initial}
	calls = 0
	errorSpec := &ucContract{ID: "execution.error", Path: "design/execution.uc", Types: map[string]ucType{"Failure": {Form: "error", Fields: []ucField{{"attempt", "Attempt", "int"}}}}, Operations: map[string]ucOperation{}, ErrorTypes: map[string]reflect.Type{"Failure": reflect.TypeFor[*executionFailure]()}, Call: runtimeExpr(t, "reject()"), Ensures: []*ucExpr{runtimeExpr(t, "raised(Failure) and failure(Failure).attempt == "+strconv.Itoa(initial))}}
	errorSpec.Operations["reject"] = ucOperation{Name: "reject", Pure: true, Raises: []string{"Failure"}, Function: reflect.ValueOf(func() error {
		if calls > 0 {
			failure.Attempt++
		}
		calls++
		return failure
	})}
	rejected := ucExecute(errorSpec, nil, false)
	if rejected.Infrastructure || len(rejected.Diagnostics) != 1 || rejected.Diagnostics[0].Code != "lang.golang.purity-violation" {
		t.Fatalf("first declared failure was not frozen: %+v", rejected)
	}
}

func checkExecutionPromotedLiteral(t rapid.TB, sample ExecutionSample) {
	native := types.NewPackage("executionliteral", "executionliteral")
	parentObject := types.NewTypeName(token.NoPos, native, "Parent", nil)
	parent := types.NewNamed(parentObject, types.NewStruct([]*types.Var{types.NewField(token.NoPos, native, "Value", types.Typ[types.Int], false)}, nil), nil)
	childObject := types.NewTypeName(token.NoPos, native, "Child", nil)
	types.NewNamed(childObject, types.NewStruct([]*types.Var{types.NewField(token.NoPos, native, "Parent", types.NewPointer(parent), true), types.NewField(token.NoPos, native, "Delta", types.Typ[types.Int], false)}, nil), nil)
	pkg := &packages.Package{PkgPath: native.Path(), Name: native.Name(), Types: native}
	compiler := contractCompiler{declarations: map[string]nativeDeclaration{"Parent": {obj: parentObject, pkg: pkg}, "Child": {obj: childObject, pkg: pkg}}, literalImports: map[string]string{}}
	literal, err := compiler.literal(fmt.Sprintf("{value: %d, delta: %d}", sample.Initial, len(sample.Deltas)), "Child", pkg)
	if err != nil {
		t.Fatalf("a native promoted record field could not be replayed: %v", err)
	}
	// Type-check the actual complete literal against an independent native shape.
	source := "package executionliteral\ntype Parent struct{Value int}\ntype Child struct{*Parent;Delta int}\nvar supplied=" + literal
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, "literal.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	configuration := types.Config{}
	if _, err := configuration.Check("executionliteral", set, []*ast.File{file}, nil); err != nil {
		t.Fatalf("a promoted-field replay literal does not compile: %v", err)
	}
}
