package golang

import (
	"errors"
	"go/ast"
	goParser "go/parser"
	"go/token"
	"go/types"
	"maps"
	"math/big"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"pgregory.net/rapid"

	"github.com/lincyaw/unicode/internal/syntax"
)

//uc:realizes lang.golang.ContractSample
type ContractSample struct {
	Balances             map[string]int
	FromId, ToId         string
	Amount, ForeignDelta int
	RaiseError           bool
}

//uc:generates lang.golang.executes-contracts.sample
func genContractSample() *rapid.Generator[ContractSample] {
	return rapid.Custom(func(t *rapid.T) ContractSample {
		balances := map[string]int{}
		for _, id := range []string{"a", "b", "c"} {
			if rapid.Bool().Draw(t, "present."+id) {
				balances[id] = rapid.IntRange(0, 200).Draw(t, "balance."+id)
			}
		}
		sample := ContractSample{balances, rapid.SampledFrom([]string{"a", "b", "missing"}).Draw(t, "from"), rapid.SampledFrom([]string{"a", "b", "missing"}).Draw(t, "to"), rapid.IntRange(1, 100).Draw(t, "amount"), rapid.IntRange(-2, 2).Draw(t, "foreign"), rapid.Bool().Draw(t, "error")}
		if rapid.Bool().Draw(t, "valid-domain") {
			sample.FromId = "a"
			sample.ToId = "b"
			sample.Balances["a"] = sample.Amount + rapid.IntRange(0, 100).Draw(t, "remaining")
			sample.Balances["b"] = rapid.IntRange(0, 100).Draw(t, "target")
		}
		return sample
	})
}

type contractBank struct {
	Balances map[string]int
	Other    int
	Port     any
}
type contractReceipt struct{ Amount int }

func (r *contractReceipt) Value() int { return r.Amount }

type contractStage string
type contractFundsError struct{ AccountId string }

func (e contractFundsError) Error() string { return "insufficient funds: " + e.AccountId }
func runtimeExpr(t rapid.TB, text string) *ucExpr {
	parsed := syntax.ParseExpr("design/runtime.uc", text)
	if len(parsed.Diagnostics) > 0 {
		t.Fatalf("cannot parse property expression %s: %v", text, parsed.Diagnostics)
	}
	var convert func(*syntax.Expr) *ucExpr
	convert = func(e *syntax.Expr) *ucExpr {
		out := &ucExpr{Kind: e.Kind, Text: e.Text, Line: 7, Column: 3}
		for _, child := range e.Children {
			out.Children = append(out.Children, convert(child))
		}
		return out
	}
	return convert(parsed.Expr)
}
func transferSpec(t rapid.TB, sample ContractSample, called *int, bank *contractBank) *ucContract {
	spec := &ucContract{ID: "bank.moves", Path: "design/runtime.uc", Line: 1, Column: 1, Symbols: []ucSymbol{{"bank", "Bank"}, {"fromId", "string"}, {"toId", "string"}, {"amount", "int"}}, Types: map[string]ucType{"Bank": {Form: "object", Fields: []ucField{{"balances", "Balances", "map<string, int>"}, {"other", "Other", "int"}}}, "Receipt": {Form: "record", Fields: []ucField{{"amount", "Amount", "int"}}}, "Funds": {Form: "error", Fields: []ucField{{"accountId", "AccountId", "string"}}}}, Operations: map[string]ucOperation{}, ErrorTypes: map[string]reflect.Type{"Funds": reflect.TypeFor[contractFundsError]()}, Observe: []string{"bank"}}
	spec.Requires = []*ucExpr{runtimeExpr(t, "fromId != toId"), runtimeExpr(t, "fromId in bank.balances and toId in bank.balances"), runtimeExpr(t, "amount <= bank.balances[fromId]")}
	spec.Changes = []*ucExpr{runtimeExpr(t, "bank.balances[fromId]"), runtimeExpr(t, "bank.balances[toId]")}
	spec.Call = runtimeExpr(t, "transfer(bank, fromId, toId, amount)")
	spec.Operations["transfer"] = ucOperation{Name: "transfer", Result: "Receipt", Raises: []string{"Funds"}, Function: reflect.ValueOf(func(b *contractBank, from, to string, amount int) (contractReceipt, error) {
		*called++
		b.Other += sample.ForeignDelta
		if sample.RaiseError {
			return contractReceipt{}, contractFundsError{from}
		}
		b.Balances[from] -= amount
		b.Balances[to] += amount
		return contractReceipt{amount}, nil
	})}
	spec.Ensures = []*ucExpr{runtimeExpr(t, "raised(Funds) implies failure(Funds).accountId == fromId"), runtimeExpr(t, "raised(Funds) implies bank.balances == old(bank.balances)"), runtimeExpr(t, "not raised(Funds) implies result.amount == amount"), runtimeExpr(t, "not raised(Funds) implies bank.balances[fromId] == old(bank.balances[fromId]) - amount"), runtimeExpr(t, "not raised(Funds) implies bank.balances[toId] == old(bank.balances[toId]) + amount")}
	return spec
}
func transferInputs(sample ContractSample, bank *contractBank) map[string]reflect.Value {
	return map[string]reflect.Value{"bank": reflect.ValueOf(bank), "fromId": reflect.ValueOf(sample.FromId), "toId": reflect.ValueOf(sample.ToId), "amount": reflect.ValueOf(sample.Amount)}
}

//uc:checks lang.golang.executes-contracts
func checkExecutesContracts(t rapid.TB, sample ContractSample) {
	checkIntegerRanges(t, sample)
	from, hasFrom := sample.Balances[sample.FromId]
	_, hasTo := sample.Balances[sample.ToId]
	accepted := sample.FromId != sample.ToId && hasFrom && hasTo && sample.Amount <= from
	bank := &contractBank{Balances: maps.Clone(sample.Balances)}
	called := 0
	spec := transferSpec(t, sample, &called, bank)
	spec.Ensures = append(spec.Ensures, runtimeExpr(t, "not raised(Funds) implies bank.balances != old(bank).balances"), runtimeExpr(t, "balanceOf(old(bank.balances), fromId) == old(bank.balances[fromId])"))
	spec.Operations["balanceOf"] = ucOperation{Name: "balanceOf", Result: "int", Params: []string{"map<string, int>", "string"}, Pure: true, Function: reflect.ValueOf(func(values map[string]int, id string) int { return values[id] })}
	out := ucExecute(spec, transferInputs(sample, bank), true)
	if out.Rejected == accepted || called != boolInt(accepted) {
		t.Fatalf("precondition/dispatch mismatch: sample=%+v outcome=%+v called=%d", sample, out, called)
	}
	if !accepted {
		if len(out.Diagnostics) > 0 || len(out.Differences) > 0 {
			t.Fatalf("a rejected candidate evaluated old or produced a business result: %+v", out)
		}
		explicit := ucExecute(spec, transferInputs(sample, bank), false)
		if !explicit.Infrastructure || len(explicit.Diagnostics) == 0 || called != 0 {
			t.Fatalf("a replay outside its domain was not a tooling error: %+v", explicit)
		}
		return
	}
	if out.Infrastructure {
		t.Fatalf("a valid model input produced an adapter error: %+v", out)
	}
	if (len(out.Differences) == 0) != (sample.ForeignDelta == 0) {
		t.Fatalf("frame result disagrees with the independently known outside change %d: %+v", sample.ForeignDelta, out)
	}
	// An observed alias has its own frame permission, even when bank permits the write.
	aliasBank := &contractBank{Balances: maps.Clone(sample.Balances)}
	aliasCalls := 0
	aliasSpec := transferSpec(t, sample, &aliasCalls, aliasBank)
	aliasSpec.Symbols = append(aliasSpec.Symbols, ucSymbol{"alias", "Bank"})
	aliasSpec.Observe = append(aliasSpec.Observe, "alias")
	inputs := transferInputs(sample, aliasBank)
	inputs["alias"] = reflect.ValueOf(aliasBank)
	alias := ucExecute(aliasSpec, inputs, true)
	if !sample.RaiseError && len(alias.Differences) == 0 {
		t.Fatalf("a write through one alias bypassed the unchanged observed alias: %+v", alias)
	}
	// A named observer must work around an interface-bearing service receiver.
	opaque := &contractBank{Balances: maps.Clone(sample.Balances), Port: make(chan int)}
	opaqueCalls := 0
	observed := transferSpec(t, sample, &opaqueCalls, opaque)
	ty := observed.Types["Bank"]
	ty.Fields = append(ty.Fields, ucField{"port", "Port", "Port"})
	observed.Types["Bank"] = ty
	observed.Types["Port"] = ucType{Form: "interface"}
	observed.Types["Snapshot"] = ucType{Form: "record", Fields: []ucField{{"other", "Other", "int"}}}
	observed.Observe = nil
	observed.Changes = nil
	observed.Observers = []ucObserver{{Name: "state", Type: "Snapshot", Inputs: []string{"bank"}, Read: reflect.ValueOf(func(b *contractBank) (struct{ Other int }, error) { return struct{ Other int }{b.Other}, nil })}}
	value := ucExecute(observed, transferInputs(sample, opaque), true)
	if value.Infrastructure || opaqueCalls != 1 {
		t.Fatalf("named observer could not inspect an opaque receiver: %+v", value)
	}
	observed.Observers[0].Read = reflect.ValueOf(func(*contractBank) (struct{ Other int }, error) {
		return struct{ Other int }{}, errors.New("observer unavailable")
	})
	failed := ucExecute(observed, transferInputs(sample, opaque), true)
	if !failed.Infrastructure || len(failed.Diagnostics) == 0 || opaqueCalls != 1 {
		t.Fatalf("an unavailable observer was replaced with a default snapshot: %+v", failed)
	}
	// Domain generators retain every declared enum member and use codepoint sizes.
	if rt, ok := t.(*rapid.T); ok {
		enumTypes := map[string]ucType{"Stage": {Form: "enum", Values: []string{"draft", "paid", "sent"}}}
		value := ucDraw(rt, reflect.TypeFor[contractStage](), "Stage", enumTypes, []string{"phase in {draft, paid}"}, "phase", 0).String()
		if value != "draft" && value != "paid" {
			t.Fatalf("the subset generator escaped its declared enum: %q", value)
		}
	}
	compiler := contractCompiler{}
	optional, err := compiler.valueLiteral(&syntax.NumberValue{Text: syntax.Number(strconv.Itoa(sample.Amount))}, "*int", nil)
	if err != nil {
		t.Fatal(err)
	}
	set, err := compiler.valueLiteral(&syntax.ListValue{Items: []syntax.Value{&syntax.NumberValue{Text: syntax.Number(strconv.Itoa(sample.Amount))}, &syntax.NumberValue{Text: syntax.Number(strconv.Itoa(sample.Amount + 1))}}}, "map[int]struct{}", nil)
	if err != nil {
		t.Fatal(err)
	}
	fs := token.NewFileSet()
	file, err := goParser.ParseFile(fs, "replay.go", "package replay\nvar optional *int = "+optional+"\nvar members map[int]struct{} = "+set, 0)
	if err != nil {
		t.Fatal(err)
	}
	conf := types.Config{}
	if _, err := conf.Check("replay", fs, []*ast.File{file}, nil); err != nil {
		t.Fatalf("a generated replay literal does not type-check: %v", err)
	}
	// Arbitrary precision evaluates a relation beyond native integer range.
	numbers := []float64{}
	if sample.ForeignDelta != 0 {
		numbers = []float64{float64(sample.Amount), -float64(sample.Amount)}
	}
	env := ucEnvironment{spec: spec, values: map[string]ucValue{"amount": ucNormalize(reflect.ValueOf(sample.Amount), "int", spec.Types)}}
	text := strings.Repeat("中éx", sample.Amount%5)
	env.values["text"] = ucNormalize(reflect.ValueOf(text), "string", spec.Types)
	if !ucTruth(env.eval(runtimeExpr(t, "size(text) == "+strconv.Itoa(utf8.RuneCountInString(text)))), nil) {
		t.Fatal("string size counted encoding bytes instead of codepoints")
	}
	optionalBytes := []byte(text)
	env.values["optionalBytes"] = ucNormalize(reflect.ValueOf(&optionalBytes), "optional<bytes>", spec.Types)
	if !ucTruth(env.eval(runtimeExpr(t, "size(optionalBytes) == "+strconv.Itoa(len(optionalBytes)))), nil) {
		t.Fatal("optional bytes size counted codepoints")
	}
	optionalNumbers := []*float64{}
	for _, number := range numbers {
		copy := number
		optionalNumbers = append(optionalNumbers, &copy)
	}
	env.values["optionalNumbers"] = ucNormalize(reflect.ValueOf(optionalNumbers), "list<optional<float>>", spec.Types)
	optionalCollection := numbers
	env.values["optionalCollection"] = ucNormalize(reflect.ValueOf(&optionalCollection), "optional<list<float>>", spec.Types)
	if !ucTruth(env.eval(runtimeExpr(t, "sum(value in optionalCollection: value) == 0.0")), nil) {
		t.Fatal("an optional float collection changed the sum type")
	}
	if !ucTruth(env.eval(runtimeExpr(t, "sum(value in optionalNumbers: value) == 0.0")), nil) {
		t.Fatal("an empty optional-float sum changed numeric type")
	}
	receipt := contractReceipt{sample.Amount}
	env.values["receipt"] = ucNormalize(reflect.ValueOf(&receipt).Elem(), "Receipt", spec.Types)
	oldReceipt := runtimeExpr(t, "old(receipt)")
	env.old = map[string]ucValue{ucExprKey(oldReceipt): ucDetach(env.values["receipt"])}
	receipt.Amount += sample.Amount
	spec.Operations["Receipt.value"] = ucOperation{Name: "Receipt.value", GoName: "Value", Owner: "Receipt", Result: "int", Pure: true}
	if !ucTruth(env.eval(runtimeExpr(t, "old(receipt).value() == amount")), nil) {
		t.Fatal("a pure method read the live record through old")
	}
	env.values["copy"] = ucNormalize(reflect.ValueOf(contractReceipt{sample.Amount}), "Receipt", spec.Types)
	if !ucTruth(env.eval(runtimeExpr(t, "copy.value() == amount")), nil) {
		t.Fatal("a pointer method could not read a returned record value")
	}
	env.values["numbers"] = ucNormalize(reflect.ValueOf(numbers), "list<float>", spec.Types)
	if !ucTruth(env.eval(runtimeExpr(t, "sum(value in numbers: value) == 0.0")), nil) {
		t.Fatal("empty or cancelling float sum changed its numeric type or exact value")
	}
	if !ucTruth(env.eval(runtimeExpr(t, "amount * 999999999999999999999999999999 / 999999999999999999999999999999 == amount")), nil) {
		t.Fatal("integer expressions overflowed native range")
	}
}

// integerBounds is an independent mathematical oracle for each actual Go width.
func integerBounds(typ reflect.Type) (*big.Int, *big.Int) {
	bits := uint(typ.Bits())
	limit := new(big.Int).Lsh(big.NewInt(1), bits)
	if typ.Kind() >= reflect.Int && typ.Kind() <= reflect.Int64 {
		half := new(big.Int).Rsh(limit, 1)
		return new(big.Int).Neg(new(big.Int).Set(half)), new(big.Int).Sub(half, big.NewInt(1))
	}
	return new(big.Int), new(big.Int).Sub(limit, big.NewInt(1))
}
func integerFits(number *big.Int, typ reflect.Type) bool {
	minimum, maximum := integerBounds(typ)
	return number.Cmp(minimum) >= 0 && number.Cmp(maximum) <= 0
}
func checkIntegerRanges(t rapid.TB, sample ContractSample) {
	widths := []reflect.Type{reflect.TypeFor[int](), reflect.TypeFor[int8](), reflect.TypeFor[int16](), reflect.TypeFor[int32](), reflect.TypeFor[int64](), reflect.TypeFor[uint](), reflect.TypeFor[uint8](), reflect.TypeFor[uint16](), reflect.TypeFor[uint32](), reflect.TypeFor[uint64](), reflect.TypeFor[uintptr]()}
	for _, target := range widths {
		minimum, maximum := integerBounds(target)
		offset := big.NewInt(int64(sample.ForeignDelta))
		large := new(big.Int).Lsh(big.NewInt(int64(sample.Amount)), uint(sample.Amount%80))
		if sample.RaiseError {
			large.Neg(large)
		}
		values := []*big.Int{new(big.Int).Add(minimum, offset), new(big.Int).Add(maximum, offset), large}
		for _, number := range values {
			fits := integerFits(number, target)
			adapt := func(value ucValue, to reflect.Type) reflect.Value {
				var got reflect.Value
				var failure any
				func() { defer func() { failure = recover() }(); got = ucNative(value, to) }()
				if !fits {
					issue, ok := failure.(ucIssue)
					if !ok || issue.code != "lang.golang.input-outside-domain" {
						t.Fatalf("integer %s to %s did not reject its native range: %T %v", number, to, failure, failure)
					}
					return reflect.Value{}
				}
				if failure != nil {
					t.Fatalf("integer %s in %s's range failed: %T %v", number, to, failure, failure)
				}
				actual := ucNormalize(got, "int", nil).Data.(*big.Int)
				if got.Type() != to || actual.Cmp(number) != 0 {
					t.Fatalf("integer %s changed when adapted to %s: %s", number, to, actual)
				}
				return got
			}
			detached := ucValue{Type: "int", Data: new(big.Int).Set(number)}
			adapt(detached, target)
			for _, source := range widths {
				if !integerFits(number, source) {
					continue
				}
				native := reflect.New(source).Elem()
				if source.Kind() >= reflect.Int && source.Kind() <= reflect.Int64 {
					native.SetInt(number.Int64())
				} else {
					native.SetUint(number.Uint64())
				}
				adapt(ucNormalize(native, "int", nil), target)
			}
			pointerType := reflect.PointerTo(target)
			if !fits {
				adapt(detached, pointerType)
				continue
			}
			pointer := reflect.New(target)
			pointer.Elem().Set(adapt(detached, target))
			live := ucNative(ucNormalize(pointer, "optional<int>", nil), pointerType)
			if live.Pointer() != pointer.Pointer() {
				t.Fatalf("adapting %s replaced a live pointer", pointerType)
			}
			frozen := adapt(ucDetach(ucNormalize(pointer, "optional<int>", nil)), pointerType)
			if frozen.Pointer() == pointer.Pointer() {
				t.Fatalf("adapting an old %s reused its live pointer", pointerType)
			}
			if target.Kind() >= reflect.Int && target.Kind() <= reflect.Int64 {
				pointer.Elem().SetInt(0)
				if number.Sign() == 0 {
					pointer.Elem().SetInt(1)
				}
			} else {
				pointer.Elem().SetUint(0)
				if number.Sign() == 0 {
					pointer.Elem().SetUint(1)
				}
			}
			if ucNormalize(frozen, "int", nil).Data.(*big.Int).Cmp(number) != 0 {
				t.Fatalf("a live %s write changed its detached integer", pointerType)
			}
		}
	}
}
func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
func TestExecutesContracts(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) { checkExecutesContracts(t, genContractSample().Draw(t, "sample")) })
}
func FuzzExecutesContracts(f *testing.F) {
	f.Fuzz(rapid.MakeFuzz(func(t *rapid.T) { checkExecutesContracts(t, genContractSample().Draw(t, "sample")) }))
}
