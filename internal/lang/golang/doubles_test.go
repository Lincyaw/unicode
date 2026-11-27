package golang

import (
	"errors"
	"reflect"

	"pgregory.net/rapid"
)

//uc:realizes lang.golang.DoubleSample
type DoubleSample struct {
	Count int
	Fail  bool
}

//uc:generates lang.golang.scripted-doubles-follow-inputs.sample
func genDoubleSample() *rapid.Generator[DoubleSample] {
	return rapid.Custom(func(t *rapid.T) DoubleSample {
		return DoubleSample{Count: rapid.IntRange(1, 5).Draw(t, "count"), Fail: rapid.Bool().Draw(t, "fail")}
	})
}

type scriptedFailure struct{ Code int }

func (e *scriptedFailure) Error() string { return "scripted failure" }

type scriptedInterface interface {
	Fetch(int) (int, error)
	Notify()
}
type scriptedInterfaceValue struct {
	scripts map[string][]map[string]any
	cursors map[string]int
	ops     map[string]ucReplyOperation
	types   map[string]ucType
}

func (d *scriptedInterfaceValue) Fetch(int) (int, error) {
	cursor := d.cursors["fetch"]
	values := ucReplyValues(d.ops["fetch"], ucNextReply(d.scripts["fetch"], &cursor), d.types)
	d.cursors["fetch"] = cursor
	var err error
	if value := values[1].Interface(); value != nil {
		err = value.(error)
	}
	return values[0].Interface().(int), err
}
func (d *scriptedInterfaceValue) Notify() {
	cursor := d.cursors["notify"]
	ucReplyValues(d.ops["notify"], ucNextReply(d.scripts["notify"], &cursor), d.types)
	d.cursors["notify"] = cursor
}

//uc:checks lang.golang.scripted-doubles-follow-inputs
func checkScriptedDoublesFollowInputs(t rapid.TB, sample DoubleSample) {
	const operation = "remote.fetch"
	const errorID = "remote.Failure"
	const carrier = "replies<remote.fetch>"
	n := max(1, sample.Count)
	script := make([]map[string]any, n)
	for i := range script {
		if sample.Fail && i%2 == 0 {
			script[i] = map[string]any{"raises": errorID, "fields": &scriptedFailure{Code: i + 17}}
		} else {
			script[i] = map[string]any{"returns": i + 17}
		}
	}
	op := ucReplyOperation{Name: operation, Result: "int", Signature: reflect.TypeFor[func(int) (int, error)](), Errors: map[string]reflect.Type{errorID: reflect.TypeFor[*scriptedFailure]()}}
	reads := 0
	real := func(int) (int, error) { reads++; return -1, nil }
	current := real
	installed, cleaned := 0, 0
	installer := func(replacement func(int) (int, error)) func() {
		installed++
		current = replacement
		return func() { cleaned++; current = real }
	}
	s := &ucContract{Symbols: []ucSymbol{{Name: "answers", Type: carrier}}, Types: map[string]ucType{
		errorID: {Form: "error", Fields: []ucField{{Name: "code", GoName: "Code", Type: "int"}}},
		carrier: {Form: "replies", Double: &ucDoubleFactory{Operations: map[string]ucReplyOperation{operation: op}}},
	}, Installers: map[string]reflect.Value{operation: reflect.ValueOf(installer)}}
	input := struct{ Replies []map[string]any }{script}
	cleanup := ucInstallDoubles(s, map[string]reflect.Value{"answers": reflect.ValueOf(input)})
	if installed != 1 || cleaned != 0 {
		t.Fatalf("installer lifecycle before execution: installed=%d cleaned=%d", installed, cleaned)
	}
	for i := 0; i < n+3; i++ {
		index := min(i, n-1)
		got, err := current(7)
		if sample.Fail && index%2 == 0 {
			var failure *scriptedFailure
			if !errors.As(err, &failure) || failure.Code != index+17 {
				t.Fatalf("response %d: got result=%d error=%v", i, got, err)
			}
			failure.Code = -1
		} else if err != nil || got != index+17 {
			t.Fatalf("response %d: got result=%d error=%v", i, got, err)
		}
	}
	cleanup()
	cleanup()
	if installed != 1 || cleaned != 1 || reads != 0 {
		t.Fatalf("substitute lifecycle: installed=%d cleaned=%d realCalls=%d", installed, cleaned, reads)
	}
	if value, _ := current(0); value != -1 || reads != 1 {
		t.Fatalf("cleanup did not restore the original client")
	}
	// The saved error fields survive caller mutation and final-response reuse.
	for i, reply := range script {
		if failure, ok := reply["fields"].(*scriptedFailure); ok && failure.Code != i+17 {
			t.Fatalf("response %d was modified through a returned error", i)
		}
	}
	// Rejection, assertion failure and panic all leave the real client installed.
	inputs := map[string]reflect.Value{"answers": reflect.ValueOf(input)}
	s.Requires = []*ucExpr{{Kind: "bool", Text: "false"}}
	if outcome := ucExecute(s, inputs, true); !outcome.Rejected || installed != 2 || cleaned != 2 {
		t.Fatalf("rejected input leaked its substitute: outcome=%+v installed=%d cleaned=%d", outcome, installed, cleaned)
	}
	s.Requires = nil
	s.Ensures = []*ucExpr{{Kind: "bool", Text: "false"}}
	if outcome := ucExecute(s, inputs, false); len(outcome.Diagnostics) == 0 || cleaned != 3 {
		t.Fatalf("failed assertion leaked its substitute: %+v cleaned=%d", outcome, cleaned)
	}
	s.Ensures = nil
	s.Call = &ucExpr{Kind: "call", Children: []*ucExpr{{Kind: "name", Text: "local.run"}}}
	s.Operations = map[string]ucOperation{"local.run": {Name: "local.run", Function: reflect.ValueOf(func() { panic("business failure") })}}
	if outcome := ucExecute(s, inputs, false); len(outcome.Diagnostics) == 0 || cleaned != 4 {
		t.Fatalf("panicking call leaked its substitute: %+v cleaned=%d", outcome, cleaned)
	}
	if value, _ := current(0); value != -1 {
		t.Fatalf("client was not restored after a failed contract")
	}
	// Both methods keep independent positions, including the void method.
	var native scriptedInterface = &scriptedInterfaceValue{scripts: map[string][]map[string]any{"fetch": script, "notify": {{"returns": nil}}}, cursors: map[string]int{}, ops: map[string]ucReplyOperation{"fetch": op, "notify": {Name: "local.notify", Signature: reflect.TypeFor[func()]()}}, types: s.Types}
	for i := 0; i < n+2; i++ {
		native.Notify()
		index := min(i, n-1)
		value, err := native.Fetch(0)
		if sample.Fail && index%2 == 0 {
			var failure *scriptedFailure
			if !errors.As(err, &failure) || failure.Code != index+17 {
				t.Fatalf("interface error response %d differs: %v", i, err)
			}
		} else if err != nil || value != index+17 {
			t.Fatalf("interface response %d differs: %d %v", i, value, err)
		}
	}
}
